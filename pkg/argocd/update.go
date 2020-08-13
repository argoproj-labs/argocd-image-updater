package argocd

import (
	"fmt"

	"github.com/argoproj-labs/argocd-image-updater/pkg/client"
	"github.com/argoproj-labs/argocd-image-updater/pkg/image"
	"github.com/argoproj-labs/argocd-image-updater/pkg/log"
	"github.com/argoproj-labs/argocd-image-updater/pkg/registry"
)

// Stores some statistics about the results of a run
type ImageUpdaterResult struct {
	NumApplicationsProcessed int
	NumImagesUpdated         int
	NumImagesConsidered      int
	NumSkipped               int
	NumErrors                int
}

// UpdateApplication update all images of a single application. Will run in a goroutine.
func UpdateApplication(newRegFn registry.NewRegistryClient, argoClient ArgoCD, kubeClient *client.KubernetesClient, curApplication *ApplicationImages, dryRun bool) ImageUpdaterResult {
	result := ImageUpdaterResult{}
	app := curApplication.Application.GetName()

	// Get all images that are deployed with the current application
	applicationImages := GetImagesFromApplication(&curApplication.Application)

	result.NumApplicationsProcessed += 1

	// Loop through all images of current application, and check whether one of
	// its images is eligible for updating.
	//
	// Whether an image qualifies for update is dependent on semantic version
	// constraints which are part of the application's annotation values.
	//
	for _, applicationImage := range applicationImages {
		updateableImage := curApplication.Images.ContainsImage(applicationImage, false)
		if updateableImage == nil {
			log.WithContext().AddField("application", app).Debugf("Image %s not in list of allowed images, skipping", applicationImage.ImageName)
			result.NumSkipped += 1
			continue
		}

		result.NumImagesConsidered += 1

		imgCtx := log.WithContext().
			AddField("application", app).
			AddField("registry", applicationImage.RegistryURL).
			AddField("image_name", applicationImage.ImageName).
			AddField("image_tag", applicationImage.ImageTag)

		imgCtx.Debugf("Considering this image for update")

		rep, err := registry.GetRegistryEndpoint(applicationImage.RegistryURL)
		if err != nil {
			imgCtx.Errorf("Could not get registry endpoint from configuration: %v", err)
			result.NumErrors += 1
			continue
		}

		var vc image.VersionConstraint
		if updateableImage.ImageTag != nil {
			vc.Constraint = updateableImage.ImageTag.TagName
			imgCtx.Debugf("Using version constraint '%s' when looking for a new tag", vc.Constraint)
		} else {
			imgCtx.Debugf("Using no version constraint when looking for a new tag")
		}

		vc.SortMode = updateableImage.GetParameterUpdateStrategy(curApplication.Application.Annotations)
		vc.MatchFunc, vc.MatchArgs = updateableImage.GetParameterMatch(curApplication.Application.Annotations)

		// The endpoint can provide default credentials for pulling images
		err = rep.SetEndpointCredentials(kubeClient)
		if err != nil {
			imgCtx.Errorf("Could not set registry endpoint credentials: %v", err)
			result.NumErrors += 1
			continue
		}

		imgCredSrc := updateableImage.GetParameterPullSecret(curApplication.Application.Annotations)
		var creds *image.Credential = &image.Credential{}
		if imgCredSrc != nil {
			creds, err = imgCredSrc.FetchCredentials(rep.RegistryAPI, kubeClient)
			if err != nil {
				imgCtx.Warnf("Could not fetch credentials: %v", err)
				result.NumErrors += 1
				continue
			}
		}

		regClient, err := newRegFn(rep, creds.Username, creds.Password)
		if err != nil {
			imgCtx.Errorf("Could not create registry client: %v", err)
			result.NumErrors += 1
			continue
		}

		// Get list of available image tags from the repository
		tags, err := rep.GetTags(applicationImage, regClient, &vc)
		if err != nil {
			imgCtx.Errorf("Could not get tags from registry: %v", err)
			result.NumErrors += 1
			continue
		}

		imgCtx.Tracef("List of available tags found: %v", tags.Tags())

		// Get the latest available tag matching any constraint that might be set
		// for allowed updates.
		latest, err := applicationImage.GetNewestVersionFromTags(&vc, tags)
		if err != nil {
			imgCtx.Errorf("Unable to find newest version from available tags: %v", err)
			result.NumErrors += 1
			continue
		}

		// If we have no latest tag information, it means there was no tag which
		// has met our version constraint (or there was no semantic versioned tag
		// at all in the repository)
		if latest == nil {
			imgCtx.Debugf("No suitable image tag for upgrade found in list of available tags.")
			result.NumSkipped += 1
			continue
		}

		// If the latest tag does not match image's current tag, it means we have
		// an update candidate.
		if applicationImage.ImageTag.TagName != latest.TagName {
			if dryRun {
				imgCtx.Infof("Would upgrade image to %s, but this is a dry run. Skipping.", applicationImage.WithTag(latest).String())
				continue
			}

			imgCtx.Infof("Upgrading image to %s", applicationImage.WithTag(latest).String())

			if appType := GetApplicationType(&curApplication.Application); appType == ApplicationTypeKustomize {
				err = SetKustomizeImage(argoClient, &curApplication.Application, updateableImage.WithTag(latest))
			} else if appType == ApplicationTypeHelm {
				err = SetHelmImage(argoClient, &curApplication.Application, updateableImage.WithTag(latest))
			} else {
				result.NumErrors += 1
				err = fmt.Errorf("Could not update application %s - neither Helm nor Kustomize application", app)
			}

			if err != nil {
				imgCtx.Errorf("Error while trying to update image: %v", err)
				result.NumErrors += 1
				continue
			} else {
				imgCtx.Infof("Successfully updated image '%s' to '%s'", applicationImage.GetFullNameWithTag(), applicationImage.WithTag(latest).GetFullNameWithTag())
				result.NumImagesUpdated += 1
			}
		} else {
			imgCtx.Debugf("Image '%s' already on latest allowed version", applicationImage.GetFullNameWithTag())
		}
	}

	return result
}
