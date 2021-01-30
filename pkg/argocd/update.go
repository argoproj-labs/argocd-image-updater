package argocd

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strings"

	"github.com/argoproj-labs/argocd-image-updater/ext/git"
	"github.com/argoproj-labs/argocd-image-updater/pkg/common"
	"github.com/argoproj-labs/argocd-image-updater/pkg/image"
	"github.com/argoproj-labs/argocd-image-updater/pkg/kube"
	"github.com/argoproj-labs/argocd-image-updater/pkg/log"
	"github.com/argoproj-labs/argocd-image-updater/pkg/registry"

	"gopkg.in/yaml.v2"

	"github.com/argoproj/argo-cd/pkg/apiclient/application"
	"github.com/argoproj/argo-cd/pkg/apis/application/v1alpha1"
)

// Stores some statistics about the results of a run
type ImageUpdaterResult struct {
	NumApplicationsProcessed int
	NumImagesFound           int
	NumImagesUpdated         int
	NumImagesConsidered      int
	NumSkipped               int
	NumErrors                int
}

type GitCredsSource func(app *v1alpha1.Application) (git.Creds, error)

type WriteBackMethod int

const (
	WriteBackApplication WriteBackMethod = 0
	WriteBackGit         WriteBackMethod = 1
)

// WriteBackConfig holds information on how to write back the changes to an Application
type WriteBackConfig struct {
	Method     WriteBackMethod
	ArgoClient ArgoCD
	// If GitClient is not nil, the client will be used for updates. Otherwise, a new client will be created.
	GitClient git.Client
	GetCreds  GitCredsSource
	GitBranch string
}

// The following are helper structs to only marshal the fields we require
type kustomizeImages struct {
	Images *v1alpha1.KustomizeImages `json:"images"`
}

type kustomizeOverride struct {
	Kustomize kustomizeImages `json:"kustomize"`
}

type helmParameters struct {
	Parameters []v1alpha1.HelmParameter `json:"parameters"`
}

type helmOverride struct {
	Helm helmParameters `json:"helm"`
}

// UpdateApplication update all images of a single application. Will run in a goroutine.
func UpdateApplication(newRegFn registry.NewRegistryClient, argoClient ArgoCD, kubeClient *kube.KubernetesClient, curApplication *ApplicationImages, dryRun bool) ImageUpdaterResult {
	var needUpdate bool = false

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
	for _, applicationImage := range curApplication.Images {
		updateableImage := applicationImages.ContainsImage(applicationImage, false)
		if updateableImage == nil {
			log.WithContext().AddField("application", app).Debugf("Image '%s' seems not to be live in this application, skipping", applicationImage.ImageName)
			result.NumSkipped += 1
			continue
		}

		result.NumImagesConsidered += 1

		imgCtx := log.WithContext().
			AddField("application", app).
			AddField("registry", updateableImage.RegistryURL).
			AddField("image_name", updateableImage.ImageName).
			AddField("image_tag", updateableImage.ImageTag).
			AddField("alias", applicationImage.ImageAlias)

		imgCtx.Debugf("Considering this image for update")

		rep, err := registry.GetRegistryEndpoint(updateableImage.RegistryURL)
		if err != nil {
			imgCtx.Errorf("Could not get registry endpoint from configuration: %v", err)
			result.NumErrors += 1
			continue
		}

		var vc image.VersionConstraint
		if applicationImage.ImageTag != nil {
			vc.Constraint = applicationImage.ImageTag.TagName
			imgCtx.Debugf("Using version constraint '%s' when looking for a new tag", vc.Constraint)
		} else {
			imgCtx.Debugf("Using no version constraint when looking for a new tag")
		}

		vc.SortMode = applicationImage.GetParameterUpdateStrategy(curApplication.Application.Annotations)
		vc.MatchFunc, vc.MatchArgs = applicationImage.GetParameterMatch(curApplication.Application.Annotations)
		vc.IgnoreList = applicationImage.GetParameterIgnoreTags(curApplication.Application.Annotations)

		// The endpoint can provide default credentials for pulling images
		err = rep.SetEndpointCredentials(kubeClient)
		if err != nil {
			imgCtx.Errorf("Could not set registry endpoint credentials: %v", err)
			result.NumErrors += 1
			continue
		}

		imgCredSrc := applicationImage.GetParameterPullSecret(curApplication.Application.Annotations)
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
		latest, err := updateableImage.GetNewestVersionFromTags(&vc, tags)
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
		if updateableImage.ImageTag.TagName != latest.TagName {

			imgCtx.Infof("Setting new image to %s", updateableImage.WithTag(latest).String())
			needUpdate = true

			if appType := GetApplicationType(&curApplication.Application); appType == ApplicationTypeKustomize {
				err = SetKustomizeImage(&curApplication.Application, applicationImage.WithTag(latest))
			} else if appType == ApplicationTypeHelm {
				err = SetHelmImage(&curApplication.Application, applicationImage.WithTag(latest))
			} else {
				result.NumErrors += 1
				err = fmt.Errorf("Could not update application %s - neither Helm nor Kustomize application", app)
			}

			if err != nil {
				imgCtx.Errorf("Error while trying to update image: %v", err)
				result.NumErrors += 1
				continue
			} else {
				imgCtx.Infof("Successfully updated image '%s' to '%s', but pending spec update (dry run=%v)", updateableImage.GetFullNameWithTag(), updateableImage.WithTag(latest).GetFullNameWithTag(), dryRun)
				result.NumImagesUpdated += 1
			}
		} else {
			imgCtx.Debugf("Image '%s' already on latest allowed version", updateableImage.GetFullNameWithTag())
		}
	}

	wbc, err := getWriteBackConfig(&curApplication.Application, kubeClient, argoClient)
	if err != nil {
		return result
	}

	if needUpdate {
		logCtx := log.WithContext().AddField("application", app)
		if !dryRun {
			logCtx.Infof("Committing %d parameter update(s) for application %s", result.NumImagesUpdated, app)
			err := commitChanges(&curApplication.Application, wbc)
			if err != nil {
				logCtx.Errorf("Could not update application spec: %v", err)
				result.NumErrors += 1
				result.NumImagesUpdated = 0
			} else {
				logCtx.Infof("Successfully updated the live application spec")
			}
		} else {
			logCtx.Infof("Dry run - not commiting %d changes to application", result.NumImagesUpdated)
		}
	}

	return result
}

// marshalParamsOverride marshals the parameter overrides of a given application
// into YAML bytes
func marshalParamsOverride(app *v1alpha1.Application) ([]byte, error) {
	var override []byte
	var err error

	appType := GetApplicationType(app)
	switch appType {
	case ApplicationTypeKustomize:
		if app.Spec.Source.Kustomize == nil {
			return []byte{}, nil
		}
		params := kustomizeOverride{
			Kustomize: kustomizeImages{
				Images: &app.Spec.Source.Kustomize.Images,
			},
		}
		override, err = yaml.Marshal(params)
	case ApplicationTypeHelm:
		if app.Spec.Source.Helm == nil {
			return []byte{}, nil
		}
		params := helmOverride{
			Helm: helmParameters{
				Parameters: app.Spec.Source.Helm.Parameters,
			},
		}
		override, err = yaml.Marshal(params)
	default:
		err = fmt.Errorf("unsupported application type")
	}
	if err != nil {
		return nil, err
	}

	return override, nil
}

func getWriteBackConfig(app *v1alpha1.Application, kubeClient *kube.KubernetesClient, argoClient ArgoCD) (*WriteBackConfig, error) {
	wbc := &WriteBackConfig{}
	// Default write-back is to use Argo CD API
	wbc.Method = WriteBackApplication
	wbc.ArgoClient = argoClient

	// If we have no update method, just return our default
	method, ok := app.Annotations[common.WriteBackMethodAnnotation]
	if !ok || strings.TrimSpace(method) == "argocd" {
		return wbc, nil
	}
	method = strings.TrimSpace(method)

	creds := "repocreds"
	if index := strings.Index(method, ":"); index > 0 {
		creds = method[index+1:]
		method = method[:index]
	}

	// We might support further methods later
	switch strings.TrimSpace(method) {
	case "git":
		wbc.Method = WriteBackGit
		branch, ok := app.Annotations[common.GitBranchAnnotation]
		if ok {
			wbc.GitBranch = strings.TrimSpace(branch)
		}
		credsSource, err := getGitCredsSource(creds, kubeClient)
		if err != nil {
			return nil, fmt.Errorf("invalid git credentials source: %v", err)
		}
		wbc.GetCreds = credsSource
	default:
		return nil, fmt.Errorf("invalid update mechanism: %s", method)
	}

	return wbc, nil
}

// commitChanges commits any changes required for updating one or more images
// after the UpdateApplication cycle has finished.
func commitChanges(app *v1alpha1.Application, wbc *WriteBackConfig) error {
	switch wbc.Method {
	case WriteBackApplication:
		_, err := wbc.ArgoClient.UpdateSpec(context.TODO(), &application.ApplicationUpdateSpecRequest{
			Name: &app.Name,
			Spec: app.Spec,
		})
		if err != nil {
			return err
		}
	case WriteBackGit:
		creds, err := wbc.GetCreds(app)
		if err != nil {
			return fmt.Errorf("could not get creds for repo '%s': %v", app.Spec.Source.RepoURL, err)
		}
		tempRoot, err := ioutil.TempDir(os.TempDir(), fmt.Sprintf("git-%s", app.Name))
		if err != nil {
			return err
		}
		defer func() {
			err := os.RemoveAll(tempRoot)
			if err != nil {
				log.Errorf("could not remove temp dir: %v", err)
			}
		}()
		var gitC git.Client
		if wbc.GitClient == nil {
			gitC, err = git.NewClientExt(app.Spec.Source.RepoURL, tempRoot, creds, false, false)
			if err != nil {
				return err
			}
		} else {
			gitC = wbc.GitClient
		}
		err = gitC.Init()
		if err != nil {
			return err
		}
		err = gitC.Fetch()
		if err != nil {
			return err
		}

		// The branch to checkout is either a configured branch in the write-back
		// config, or taken from the application spec's targetRevision. If the
		// target revision is set to the special value HEAD, or is the empty
		// string, we'll try to resolve it to a branch name.
		checkOutBranch := app.Spec.Source.TargetRevision
		if wbc.GitBranch != "" {
			checkOutBranch = wbc.GitBranch
		}
		log.Tracef("targetRevision for update is '%s'", checkOutBranch)
		if checkOutBranch == "" || checkOutBranch == "HEAD" {
			checkOutBranch, err = gitC.SymRefToBranch(checkOutBranch)
			log.Infof("resolved remote default branch to '%s' and using that for operations", checkOutBranch)
			if err != nil {
				return err
			}
		}

		err = gitC.Checkout(checkOutBranch)
		if err != nil {
			return err
		}
		targetExists := true
		targetFile := path.Join(tempRoot, app.Spec.Source.Path, fmt.Sprintf(".argocd-source-%s.yaml", app.Name))
		_, err = os.Stat(targetFile)
		if err != nil {
			if !os.IsNotExist(err) {
				return err
			} else {
				targetExists = false
			}
		}

		override, err := marshalParamsOverride(app)
		if err != nil {
			return fmt.Errorf("could not marshal parameters: %v", err)
		}

		// If the target file already exist in the repository, we will check whether
		// our generated new file is the same as the existing one, and if yes, we
		// don't proceed further for commit.
		if targetExists {
			data, err := ioutil.ReadFile(targetFile)
			if err != nil {
				return err
			}
			if string(data) == string(override) {
				log.Debugf("target parameter file and marshaled data are the same, skipping commit.")
				return nil
			}
		}

		err = ioutil.WriteFile(targetFile, override, 0600)
		if err != nil {
			return err
		}

		if !targetExists {
			err = gitC.Add(targetFile)
			if err != nil {
				return err
			}
		}

		err = gitC.Commit("", "Update to new image versions", "")
		if err != nil {
			return err
		}
		err = gitC.Push("origin", checkOutBranch, false)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown write back method set: %d", wbc.Method)
	}
	return nil
}
