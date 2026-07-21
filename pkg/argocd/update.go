package argocd

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/exp/slices"
	yaml "sigs.k8s.io/yaml/goyaml.v3"

	iuapi "github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"
	"github.com/argoproj-labs/argocd-image-updater/ext/git"
	"github.com/argoproj-labs/argocd-image-updater/pkg/common"
	"github.com/argoproj-labs/argocd-image-updater/pkg/kube"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/image"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/registry"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/tag"

	"github.com/argoproj/argo-cd/v3/pkg/apiclient/application"
	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
)

// listElementPattern is a regular expression for searching for an element in a yaml array.
// example: any-string[1]
const listElementPattern = `^(.*)\[(.*)\]$`

var re = regexp.MustCompile(listElementPattern)

// UpdateApplication update all images of a single application. Will run in a goroutine.
func UpdateApplication(ctx context.Context, updateConf *UpdateConfiguration, state *SyncIterationState) ImageUpdaterResult {
	baseLogger := log.LoggerFromContext(ctx)

	var needUpdate bool = false
	result := ImageUpdaterResult{}
	app := updateConf.UpdateApp.Application.GetName()
	appNs := updateConf.UpdateApp.Application.GetNamespace()

	changeList := make([]ChangeEntry, 0)

	// Get all images that are deployed with the current application
	applicationImages := GetImagesFromApplication(updateConf.UpdateApp)

	result.NumApplicationsProcessed += 1

	// Loop through all images of current application, and check whether one of
	// its images is eligible for updating.
	//
	// Whether an image qualifies for update is dependent on semantic version
	// constraints which are part of the application's values.
	//
	for _, applicationImage := range updateConf.UpdateApp.Images {
		// updateableImage is the live image found in the cluster status
		updateableImage := applicationImages.ContainsImage(applicationImage.ContainerImage, false)
		if updateableImage == nil {
			baseLogger.Debugf("Image '%s' seems not to be live in this application, skipping", applicationImage.ImageName)
			result.NumSkipped += 1
			continue
		}

		// In some cases, the running image has no tag set. We create a dummy
		// tag, without name, digest and a timestamp of zero. This dummy tag
		// will trigger an update on the first run.
		if updateableImage.ImageTag == nil {
			updateableImage.ImageTag = tag.NewImageTag("", time.Unix(0, 0), "")
		}

		result.NumImagesConsidered += 1

		fields := updateableImage.GetLogFields(applicationImage.ImageAlias)
		imgCtx := baseLogger.WithFields(fields)
		imageOpCtx := log.ContextWithLogger(ctx, imgCtx)

		if updateableImage.KustomizeImage != nil {
			imgCtx = imgCtx.WithField("kustomize_image", updateableImage.KustomizeImage)
		}

		imgCtx.Debugf("Considering this image for update")

		rep, err := registry.GetRegistryEndpoint(imageOpCtx, applicationImage.ContainerImage)
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

		vc.Strategy = applicationImage.UpdateStrategy
		vc.MatchFunc, vc.MatchArgs = applicationImage.ParseMatch(imageOpCtx, applicationImage.AllowTags)
		vc.IgnoreList = applicationImage.IgnoreTags
		vc.Options = applicationImage.
			GetPlatformOptions(imageOpCtx, updateConf.IgnorePlatforms, applicationImage.Platforms).
			WithMetadata(vc.Strategy.NeedsMetadata())

		// If a strategy needs meta-data and tagsortmode is set for the
		// registry, let the user know.
		if rep.TagListSort > registry.TagListSortUnsorted && vc.Strategy.NeedsMetadata() {
			imgCtx.Infof("taglistsort is set to '%s' but update strategy '%s' requires metadata. Results may not be what you expect.", rep.TagListSort.String(), vc.Strategy.String())
		}

		// retrieves an image's pull secret credentials
		secretVal := applicationImage.PullSecret
		if secretVal == "" {
			imgCtx.Tracef("No pull secret configured for this image")
		}

		// reject cross-namespace secret references in commonUpdateSettings.pullSecret
		if strings.HasPrefix(secretVal, "pullsecret:") || strings.HasPrefix(secretVal, "secret:") {
			// Strip "pullsecret:" or "secret:" prefix before splitting on "/" to isolate the namespace.
			_, ref, _ := strings.Cut(secretVal, ":")
			s := strings.SplitN(ref, "/", 2)
			if len(s) == 2 {
				if s[0] != appNs {
					imgCtx.Errorf("commonUpdateSettings.pullSecret namespace '%s' differs from app namespace '%s'", s[0], appNs)
					result.NumErrors += 1
					continue
				}
			}
		}

		// The endpoint can provide default credentials for pulling images
		creds, err := rep.SetEndpointCredentials(imageOpCtx, updateConf.KubeClient.KubeClient, secretVal)
		if err != nil {
			imgCtx.Errorf("Could not set registry endpoint credentials: %v", err)
			result.NumErrors += 1
			continue
		}

		regClient, err := updateConf.NewRegFN(rep, creds.Username, creds.Password)
		if err != nil {
			imgCtx.Errorf("Could not create registry client: %v", err)
			result.NumErrors += 1
			continue
		}

		// Get list of available image tags from the repository
		// Load creds, create registry client, fetch tags (retry once on 401/403)
		tags, err := rep.GetTags(imageOpCtx, applicationImage.ContainerImage, regClient, &vc, secretVal == "")
		if err != nil {
			// Retry once on 401/403
			if errors.Is(err, registry.ErrCredentialsInvalid) {
				imgCtx.Infof("credentials invalid (401/403), refetching and retrying once")
				// The endpoint can provide default credentials for pulling images
				creds, err = rep.SetEndpointCredentials(imageOpCtx, updateConf.KubeClient.KubeClient, secretVal)
				if err != nil {
					imgCtx.Errorf("Could not set registry endpoint credentials: %v", err)
					result.NumErrors += 1
					continue
				}

				regClient, err = updateConf.NewRegFN(rep, creds.Username, creds.Password)
				if err != nil {
					imgCtx.Errorf("Could not create registry client: %v", err)
					result.NumErrors += 1
					continue
				}
				tags, err = rep.GetTags(imageOpCtx, applicationImage.ContainerImage, regClient, &vc, secretVal == "")
			}
			if err != nil {
				imgCtx.Errorf("Could not get tags from registry: %v", err)
				result.NumErrors += 1
				continue
			}
		}

		imgCtx.Tracef("List of available tags found: %v", tags.Tags())

		// Get the latest available tag matching any constraint that might be set
		// for allowed updates.
		latest, err := updateableImage.GetNewestVersionFromTags(imageOpCtx, &vc, tags)
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

		if needsUpdate(updateableImage, applicationImage.ContainerImage, latest, vc.Strategy) {
			appImageWithTag := applicationImage.WithTag(latest)
			appImageFullNameWithTag := appImageWithTag.GetFullNameWithTag()

			// Check if new image is already set in Application Spec when write back is set to argocd
			// and compare with new image
			appImageSpec, err := getAppImage(imageOpCtx, &updateConf.UpdateApp.Application, updateConf.UpdateApp.WriteBackConfig, applicationImage)
			if err != nil {
				continue
			}
			if appImageSpec == appImageFullNameWithTag {
				imgCtx.Infof("New image %s already set in spec", appImageFullNameWithTag)
				continue
			}

			// check signature for appImageWithTag using applicationImage.Verify
			if applicationImage.EnableVerification {
				switch {
				case applicationImage.Verify != nil && applicationImage.Verify.CosignKey != "":
					err := image.VerifyWithPublicKey(imageOpCtx, appImageWithTag, applicationImage.Verify, regClient)
					if err != nil {
						imgCtx.Errorf("Unable to verify image %s with public key: %v", appImageFullNameWithTag, err)
						result.NumErrors += 1
						continue
					}
				// additional verification methods will be added here
				default:
					imgCtx.Errorf("Image verification enabled but no verification method configured for %s", appImageFullNameWithTag)
					result.NumErrors += 1
					continue
				}
			} else {
				imgCtx.Debugf("Image verification not configured for %s, skipping", appImageFullNameWithTag)
			}

			needUpdate = true
			imgCtx.Infof("Setting new image to %s", appImageFullNameWithTag)

			err = setAppImage(imageOpCtx, &updateConf.UpdateApp.Application, appImageWithTag, updateConf.UpdateApp.WriteBackConfig, applicationImage)

			if err != nil {
				imgCtx.Errorf("Error while trying to update image: %v", err)
				result.NumErrors += 1
				continue
			} else {
				imgCtx.Infof("Successfully updated image '%s' to '%s', but pending spec update (dry run=%v)", updateableImage.GetFullNameWithTag(), appImageFullNameWithTag, updateConf.DryRun)
				changeList = append(changeList, ChangeEntry{appImageWithTag, updateableImage.ImageTag, appImageWithTag.ImageTag})
				result.NumImagesUpdated += 1
			}
		} else {
			// We need to explicitly set the up-to-date images in the spec too, so
			// that we correctly marshal out the parameter overrides to include all
			// images, regardless of those were updated or not.
			err = setAppImage(imageOpCtx, &updateConf.UpdateApp.Application, applicationImage.WithTag(updateableImage.ImageTag), updateConf.UpdateApp.WriteBackConfig, applicationImage)
			if err != nil {
				imgCtx.Errorf("Error while trying to update image: %v", err)
				result.NumErrors += 1
			}
			imgCtx.Debugf("Image '%s' already on latest allowed version", updateableImage.GetFullNameWithTag())
		}
	}

	wbc := updateConf.UpdateApp.WriteBackConfig
	wbc.ArgoClient = updateConf.ArgoClient

	if updateConf.GitCreds == nil {
		wbc.GitCreds = git.NoopCredsStore{}
	} else {
		wbc.GitCreds = updateConf.GitCreds
	}

	if wbc.Method == WriteBackGit {
		if updateConf.GitCommitUser != "" {
			wbc.GitCommitUser = updateConf.GitCommitUser
		}
		if updateConf.GitCommitEmail != "" {
			wbc.GitCommitEmail = updateConf.GitCommitEmail
		}
		if len(changeList) > 0 && updateConf.GitCommitMessage != nil {
			wbc.GitCommitMessage = TemplateCommitMessage(ctx, updateConf.GitCommitMessage, updateConf.UpdateApp.Application.Name, changeList)
		}
		if updateConf.GitCommitSigningKey != "" {
			wbc.GitCommitSigningKey = updateConf.GitCommitSigningKey
		}
		wbc.GitCommitSigningMethod = updateConf.GitCommitSigningMethod
		wbc.GitCommitSignOff = updateConf.GitCommitSignOff
	}

	if needUpdate {
		baseLogger.Debugf("Using commit message: %s", wbc.GitCommitMessage)
		if !updateConf.DryRun {
			baseLogger.Infof("Committing %d parameter update(s) for application %s", result.NumImagesUpdated, app)
			err := commitChangesLocked(ctx, updateConf.UpdateApp, state, changeList)
			if err != nil {
				baseLogger.Errorf("Could not update application spec: %v", err)
				result.NumErrors += 1
				result.NumImagesUpdated = 0
			} else {
				baseLogger.Infof("Successfully updated the live application spec")
				if !updateConf.DisableKubeEvents && updateConf.KubeClient != nil {
					annotations := map[string]string{}
					for i, c := range changeList {
						annotations[fmt.Sprintf("argocd-image-updater.image-%d/full-image-name", i)] = c.Image.GetFullNameWithoutTag()
						annotations[fmt.Sprintf("argocd-image-updater.image-%d/image-name", i)] = c.Image.ImageName
						annotations[fmt.Sprintf("argocd-image-updater.image-%d/old-tag", i)] = c.OldTag.String()
						annotations[fmt.Sprintf("argocd-image-updater.image-%d/new-tag", i)] = c.NewTag.String()
					}
					message := fmt.Sprintf("Successfully updated application '%s'", app)
					_, err = updateConf.KubeClient.CreateApplicationEvent(&updateConf.UpdateApp.Application, "ImagesUpdated", message, annotations)
					if err != nil {
						baseLogger.Warnf("Event could not be sent: %v", err)
					}
				} else {
					if updateConf.DisableKubeEvents {
						baseLogger.Debugf("Kubernetes events disabled for application '%s'", app)
					}
					if updateConf.KubeClient == nil {
						baseLogger.Debugf("KubeClient is nil, skipping Kubernetes event creation for application '%s'", app)
					}
				}
			}
		} else {
			baseLogger.Infof("Dry run - not committing %d changes to application", result.NumImagesUpdated)
		}
	}

	result.Changes = changeList
	return result
}

// needsUpdate determines if an image needs to be updated based on the provided
// updateableImage, applicationImage, latest available tag, and update strategy.
// It considers digest strategy, tag equality, and Kustomize image differences.
// Returns true if an update is required, false otherwise.
func needsUpdate(updateableImage *image.ContainerImage, applicationImage *image.ContainerImage, latest *tag.ImageTag, strategy image.UpdateStrategy) bool {
	if strategy == image.StrategyDigest {
		if updateableImage.ImageTag == nil {
			return true
		}
		// When using digest strategy, consider the digest even if the current image
		// was referenced by tag. If either digest is missing or differs, we want an update.
		if !updateableImage.ImageTag.IsDigest() || updateableImage.ImageTag.TagDigest != latest.TagDigest {
			return true
		}
	}
	// If the latest tag does not match image's current tag or the kustomize image is different, it means we have an update candidate.
	return !updateableImage.ImageTag.Equals(latest) || applicationImage.KustomizeImage != nil && applicationImage.DiffersFrom(updateableImage, false)
}

// getAppImage retrieves the current image string from an Argo CD application.
// It determines the application type (Kustomize or Helm) and calls the appropriate
// function to extract the image information.
func getAppImage(ctx context.Context, app *v1alpha1.Application, wbc *WriteBackConfig, applicationImage *Image) (string, error) {
	var err error
	if appType := GetApplicationType(app, wbc); appType == ApplicationTypeKustomize {
		return GetKustomizeImage(ctx, app, wbc, applicationImage)
	} else if appType == ApplicationTypeHelm {
		return GetHelmImage(ctx, app, wbc, applicationImage)
	} else {
		err = fmt.Errorf("could not update application %s - neither Helm nor Kustomize application", app)
		return "", err
	}
}

// setAppImage updates the image in the application's manifest based on its type (Kustomize or Helm).
// It calls the appropriate function (SetKustomizeImage or SetHelmImage) to perform the update.
// Returns an error if the application type is neither Helm nor Kustomize, or if the update fails.
func setAppImage(ctx context.Context, app *v1alpha1.Application, img *image.ContainerImage, wbc *WriteBackConfig, applicationImage *Image) error {
	var err error
	if appType := GetApplicationType(app, wbc); appType == ApplicationTypeKustomize {
		err = SetKustomizeImage(ctx, app, img, wbc, applicationImage)
	} else if appType == ApplicationTypeHelm {
		err = SetHelmImage(ctx, app, img, wbc, applicationImage)
	} else {
		err = fmt.Errorf("could not update application %s - neither Helm nor Kustomize application", app)
	}
	return err
}

func marshalWithIndent(in interface{}, indent int) (out []byte, err error) {
	var b bytes.Buffer
	encoder := yaml.NewEncoder(&b)
	defer encoder.Close()
	// note: yaml.v3 will only respect indents from 1 to 9 inclusive.
	encoder.SetIndent(indent)
	encoder.CompactSeqIndent()
	if err = encoder.Encode(in); err != nil {
		return nil, err
	}
	if err = encoder.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// marshalParamsOverride marshals the parameter overrides of a given application
// into YAML bytes
func marshalParamsOverride(ctx context.Context, applicationImages *ApplicationImages, originalData []byte) ([]byte, error) {
	log := log.LoggerFromContext(ctx)
	var override []byte
	var err error
	app := &applicationImages.Application
	wbc := applicationImages.WriteBackConfig

	appType := GetApplicationType(app, wbc)
	appSource := GetApplicationSource(ctx, app, wbc)

	switch appType {
	case ApplicationTypeKustomize:
		if appSource.Kustomize == nil {
			return []byte{}, nil
		}

		var params kustomizeOverride
		newParams := kustomizeOverride{
			Kustomize: kustomizeImages{
				Images: &appSource.Kustomize.Images,
			},
		}

		if len(originalData) == 0 {
			override, err = marshalWithIndent(newParams, defaultIndent)
			break
		}
		err = yaml.Unmarshal(originalData, &params)
		if err != nil {
			override, err = marshalWithIndent(newParams, defaultIndent)
			break
		}
		mergeKustomizeOverride(&params, &newParams)
		override, err = marshalWithIndent(params, defaultIndent)
	case ApplicationTypeHelm:
		// Extract Helm parameters safely; Helm may be nil for SourceHydrator apps
		// where the source type is auto-detected from files rather than set in the spec.
		var helmParams []v1alpha1.HelmParameter
		if appSource.Helm != nil {
			helmParams = appSource.Helm.Parameters
		}

		if wbc != nil && !strings.HasPrefix(filepath.Base(wbc.Target), common.DefaultTargetFilePrefix) {
			images := GetImagesAndAliasesFromApplication(applicationImages)

			var helmNewValues yaml.Node
			emptyOriginalData := isOnlyWhitespace(originalData)
			if emptyOriginalData {
				// allow non-exists target file
				helmNewValues = yaml.Node{
					Kind:        yaml.DocumentNode,
					HeadComment: "auto generated by argocd image updater",
					Content: []*yaml.Node{
						{
							Kind:    yaml.MappingNode,
							Tag:     "!!map",
							Content: []*yaml.Node{},
							Style:   yaml.LiteralStyle,
						},
					},
				}
			} else {
				helmNewValues = yaml.Node{}
				err = yaml.Unmarshal(originalData, &helmNewValues)
				if err != nil {
					return nil, err
				}
			}

			var writes []helmValueWrite
			for _, c := range images {
				if c == nil || c.ImageAlias == "" {
					continue
				}

				helmParamName, helmParamVersion := getHelmParamNames(c)

				if helmParamName == "" {
					return nil, fmt.Errorf("could not find an image-name for image %s", c.ImageName)
				}
				// for image-spec, helmParamName holds image-spec value,
				// and helmParamVersion is empty
				if helmParamVersion == "" {
					if c.HelmImageSpec == "" {
						// not a full image-spec, so image-tag is required
						return nil, fmt.Errorf("could not find an image-tag for image %s", c.ImageName)
					}
				} else {
					// image-tag is present, so continue to process image-tag
					helmParamVer := getHelmParam(helmParams, helmParamVersion)
					var tagValue string
					if helmParamVer == nil {
						// Parameter not pre-defined in the Application - use the image's tag data as fallback
						log.Debugf("helm parameter %s not found in app spec, using image tag as fallback", helmParamVersion)
						tagValue = c.ContainerImage.GetTagWithDigest()
					} else {
						tagValue = helmParamVer.Value
					}
					//Write tag in helm value file only if tag is not empty
					if tagValue != "" {
						writes = append(writes, helmValueWrite{kind: "version", path: helmParamVersion, value: tagValue})
					}
				}

				helmParamN := getHelmParam(helmParams, helmParamName)
				// Determine which value to use for the image name parameter
				var valueToSet string
				if helmParamN == nil {
					// Parameter not pre-defined in the Application - use the image's name data as fallback
					log.Debugf("helm parameter %s not found in app spec, using image name as fallback", helmParamName)
					valueToSet = c.ContainerImage.GetFullNameWithoutTag()
				} else {
					valueToSet = helmParamN.Value
					if !emptyOriginalData && image.HasRegistryPrefix(valueToSet) {
						// helmParamN.Value is in long form (has registry URL)
						// Check the original value in helmNewValues to see if it's in short form
						// Skip this check if originalData is empty
						originalValue, err := getHelmValue(&helmNewValues, helmParamName)
						if err == nil {
							// Original value exists and was found
							if !image.HasRegistryPrefix(originalValue) {
								// Original value is in short form, use the short form of the value to set
								valueToSet = image.ExtractShortForm(valueToSet)
							}
							// If originalValue is also in long form, keep using helmParamN.Value
						}
						// If getHelmValue returns an error (key not found), use helmParamN.Value as-is
					}
					// If helmParamN.Value is already in short form or originalData is empty, use it as-is
				}

				writes = append(writes, helmValueWrite{kind: "name", path: helmParamName, value: valueToSet})
			}

			override, err = applyHelmValueWrites(&helmNewValues, originalData, writes)
		} else {
			if appSource.Helm == nil {
				return []byte{}, nil
			}
			var params helmOverride
			newParams := helmOverride{
				Helm: helmParameters{
					Parameters: appSource.Helm.Parameters,
				},
			}

			outputParams := appSource.Helm.ValuesYAML()
			log.Debugf("values: '%s'", outputParams)

			if len(originalData) == 0 {
				sortHelmParameters(newParams.Helm.Parameters)
				override, err = marshalWithIndent(newParams, defaultIndent)
				break
			}
			err = yaml.Unmarshal(originalData, &params)
			if err != nil {
				sortHelmParameters(newParams.Helm.Parameters)
				override, err = marshalWithIndent(newParams, defaultIndent)
				break
			}
			mergeHelmOverride(&params, &newParams)
			sortHelmParameters(params.Helm.Parameters)
			override, err = marshalWithIndent(params, defaultIndent)
		}
	default:
		err = fmt.Errorf("unsupported application type")
	}
	if err != nil {
		return nil, err
	}

	return override, nil
}

func sortHelmParameters(params []v1alpha1.HelmParameter) {
	slices.SortFunc(params, func(a, b v1alpha1.HelmParameter) int {
		return cmp.Compare(a.Name, b.Name)
	})
}

func mergeHelmOverride(t *helmOverride, o *helmOverride) {
	for _, param := range o.Helm.Parameters {
		idx := slices.IndexFunc(t.Helm.Parameters, func(tp v1alpha1.HelmParameter) bool { return tp.Name == param.Name })
		if idx != -1 {
			t.Helm.Parameters[idx] = param
			continue
		}
		t.Helm.Parameters = append(t.Helm.Parameters, param)
	}
}

func mergeKustomizeOverride(t *kustomizeOverride, o *kustomizeOverride) {
	if o.Kustomize.Images == nil {
		return
	}
	if t.Kustomize.Images == nil {
		emptyImages := make(v1alpha1.KustomizeImages, 0)
		t.Kustomize.Images = &emptyImages
	}
	for _, newImage := range *o.Kustomize.Images {
		found := false
		newContainerImage := image.NewFromIdentifier(string(newImage))
		for idx, existingImage := range *t.Kustomize.Images {
			existingContainerImage := image.NewFromIdentifier(string(existingImage))
			if sameImageNameAndRegistry(newContainerImage, existingContainerImage) {
				found = true
				if existingContainerImage.ImageTag == nil ||
					(newContainerImage.ImageTag != nil && !(existingContainerImage.ImageTag).Equals(newContainerImage.ImageTag)) {
					(*t.Kustomize.Images)[idx] = newImage
				}
				break
			}
		}
		if !found {
			*t.Kustomize.Images = append(*t.Kustomize.Images, newImage)
		}
	}
}

// sameImageNameAndRegistry checks if 2 ContainerImage have the same image name and registry url.
// When comparing registry url, the default registry url "docker.io" and empty string are considered the same.
func sameImageNameAndRegistry(img1 *image.ContainerImage, img2 *image.ContainerImage) bool {
	if img1.ImageName != img2.ImageName {
		return false
	}
	if img1.RegistryURL == img2.RegistryURL {
		return true
	}
	if img1.RegistryURL == "" && img2.RegistryURL == "docker.io" {
		return true
	}
	if img2.RegistryURL == "" && img1.RegistryURL == "docker.io" {
		return true
	}
	return false
}

// Check if a key exists in a MappingNode and return the index of its value
func findHelmValuesKey(m *yaml.Node, key string) (int, bool) {
	for i, item := range m.Content {
		if i%2 == 0 && item.Value == key {
			return i + 1, true
		}
	}
	return -1, false
}

func nodeKindString(k yaml.Kind) string {
	return map[yaml.Kind]string{
		yaml.DocumentNode: "DocumentNode",
		yaml.SequenceNode: "SequenceNode",
		yaml.MappingNode:  "MappingNode",
		yaml.ScalarNode:   "ScalarNode",
		yaml.AliasNode:    "AliasNode",
	}[k]
}

// setHelmValue sets value of the parameter passed from the CRD configuration.
func setHelmValue(currentValues *yaml.Node, key string, value interface{}) error {
	current := currentValues

	// an unmarshalled document has a DocumentNode at the root, but
	// we navigate from a MappingNode.
	if current.Kind == yaml.DocumentNode {
		current = current.Content[0]
	}

	if current.Kind != yaml.MappingNode {
		return fmt.Errorf("unexpected type %s for root", nodeKindString(current.Kind))
	}

	// Check if the full key exists
	if idx, found := findHelmValuesKey(current, key); found {
		(*current).Content[idx].Value = value.(string)
		return nil
	}

	var err error
	keys := strings.Split(key, ".")
	for i, k := range keys {
		// pointer is needed to determine that the id has indeed been passed.
		var idPtr *int
		// by default, the search is based on the key without changes, but
		// if string matches pattern, we consider it is an id in YAML list.
		key := k
		matches := re.FindStringSubmatch(k)
		if matches != nil {
			idStr := matches[2]
			id, err := strconv.Atoi(idStr)
			if err != nil {
				return fmt.Errorf("id \"%s\" in yaml array must match pattern ^(.*)\\[(.*)\\]$", idStr)
			}
			idPtr = &id
			key = matches[1]
		}
		if idx, found := findHelmValuesKey(current, key); found {
			// Navigate deeper into the map
			current = (*current).Content[idx]
			// unpack one level of alias; an alias of an alias is not supported
			if current.Kind == yaml.AliasNode {
				current = current.Alias
			}
			if current.Kind != yaml.SequenceNode && idPtr != nil {
				return fmt.Errorf("id %d provided when \"%s\" is not an yaml array", *idPtr, key)
			}
			if current.Kind == yaml.SequenceNode {
				if idPtr == nil {
					return fmt.Errorf("no id provided for yaml array \"%s\"", key)
				}
				currentContent := (*current).Content
				if *idPtr < 0 || *idPtr >= len(currentContent) {
					return fmt.Errorf("id %d is out of range [0, %d)", *idPtr, len(currentContent))
				}
				current = (*current).Content[*idPtr]
			}
			if i == len(keys)-1 {
				// If we're at the final key, set the value and return
				if current.Kind == yaml.ScalarNode {
					current.Value = value.(string)
					current.Tag = "!!str"
				} else {
					return fmt.Errorf("unexpected type %s for key %s", nodeKindString(current.Kind), k)
				}
				return nil
			} else if current.Kind != yaml.MappingNode {
				return fmt.Errorf("unexpected type %s for key %s", nodeKindString(current.Kind), k)
			}
		} else {
			if i == len(keys)-1 {
				current.Content = append(current.Content,
					&yaml.Node{
						Kind:  yaml.ScalarNode,
						Value: k,
						Tag:   "!!str",
					},
					&yaml.Node{
						Kind:  yaml.ScalarNode,
						Value: value.(string),
						Tag:   "!!str",
					},
				)
				return nil
			} else {
				current.Content = append(current.Content,
					&yaml.Node{
						Kind:  yaml.ScalarNode,
						Value: k,
						Tag:   "!!str",
					},
					&yaml.Node{
						Kind:    yaml.MappingNode,
						Content: []*yaml.Node{},
					},
				)
				current = current.Content[len(current.Content)-1]
			}
		}
	}

	return err
}

// getHelmValue retrieves a value from a yaml.Node using a key path.
// See resolveHelmScalarNode for how the key is interpreted.
func getHelmValue(values *yaml.Node, key string) (string, error) {
	node, err := resolveHelmScalarNode(values, key)
	if err != nil {
		return "", err
	}
	return node.Value, nil
}

// resolveHelmScalarNode resolves a key path to its scalar yaml.Node. The key
// can be in the form of "a.b.c" which can be:
// 1. A nested hierarchy where "a" has "b" which has "c"
// 2. A literal key "a.b.c" if the nested structure doesn't exist
// Returning the node (rather than just its value) lets callers read its source
// Line/Column for in-place patching. Returns an error if the key is not found.
func resolveHelmScalarNode(values *yaml.Node, key string) (*yaml.Node, error) {
	current := values

	// an unmarshalled document has a DocumentNode at the root, but
	// we navigate from a MappingNode.
	if current.Kind == yaml.DocumentNode {
		if len(current.Content) == 0 {
			return nil, fmt.Errorf("empty document node")
		}
		current = current.Content[0]
	}

	if current.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("unexpected type %s for root", nodeKindString(current.Kind))
	}

	// First, try to navigate as nested path (a.b.c)
	keys := strings.Split(key, ".")
	node := current

	for i, k := range keys {
		var idPtr *int
		// Handle array indexing pattern like "key[0]"
		keyPart := k
		if matches := re.FindStringSubmatch(k); matches != nil {
			id, err := strconv.Atoi(matches[2])
			if err != nil {
				return nil, fmt.Errorf("id \"%s\" in yaml array must match pattern ^(.*)\\[(.*)\\]$", matches[2])
			}
			idPtr = &id
			keyPart = matches[1]
		}

		idx, found := findHelmValuesKey(node, keyPart)
		if !found {
			break // fall through to literal check
		}
		node = node.Content[idx]
		// unpack one level of alias; an alias of an alias is not supported
		if node.Kind == yaml.AliasNode {
			node = node.Alias
		}
		if node.Kind == yaml.SequenceNode {
			if idPtr == nil || *idPtr < 0 || *idPtr >= len(node.Content) {
				break
			}
			node = node.Content[*idPtr]
		}

		if i == len(keys)-1 {
			if node.Kind == yaml.ScalarNode {
				return node, nil
			}
			break // not a scalar, fall through to literal check
		} else if node.Kind != yaml.MappingNode {
			break // can't navigate further
		}
	}

	// If nested path didn't work, try as a literal key "a.b.c"
	if idx, found := findHelmValuesKey(current, key); found {
		valueNode := current.Content[idx]
		if valueNode.Kind == yaml.AliasNode {
			valueNode = valueNode.Alias
		}
		if valueNode.Kind == yaml.ScalarNode {
			return valueNode, nil
		}
		return nil, fmt.Errorf("literal key \"%s\" found but is not a scalar value", key)
	}

	return nil, fmt.Errorf("key \"%s\" not found as nested path or literal key", key)
}

// helmValueWrite is a single parameter value to write into a Helm values
// document. kind ("name" or "version") is used only for error context.
type helmValueWrite struct {
	kind  string
	path  string
	value string
}

// applyHelmValueWrites writes the given parameter values into a Helm values
// document. When every write updates a value that already exists in the
// original file as a plain scalar, it patches those values in place, preserving
// the file's exact formatting (comments, blank lines, indentation, anchors,
// inline-comment alignment). If any key must be created or cannot be safely
// rewritten, it falls back to mutating the YAML node tree and re-marshalling,
// which preserves comments and key order but not blank lines.
func applyHelmValueWrites(root *yaml.Node, originalData []byte, writes []helmValueWrite) ([]byte, error) {
	if patched, ok := patchHelmValuesInPlace(root, originalData, writes); ok {
		return patched, nil
	}
	for _, w := range writes {
		if err := setHelmValue(root, w.path, w.value); err != nil {
			return nil, fmt.Errorf("failed to set image parameter %s value: %v", w.kind, err)
		}
	}
	return marshalWithIndent(root, defaultIndent)
}

// patchHelmValuesInPlace edits value text directly in originalData. It succeeds
// only when every write targets an existing plain scalar that can be safely
// rewritten; otherwise it returns (nil, false) so the caller falls back to
// re-marshalling. Because it never re-serialises untouched lines, all original
// formatting outside the changed values is preserved byte-for-byte.
func patchHelmValuesInPlace(root *yaml.Node, originalData []byte, writes []helmValueWrite) ([]byte, bool) {
	if isOnlyWhitespace(originalData) {
		return nil, false
	}
	type patch struct {
		node  *yaml.Node
		value string
	}
	patches := make([]patch, 0, len(writes))
	for _, w := range writes {
		node, err := resolveHelmScalarNode(root, w.path)
		if err != nil {
			return nil, false // key must be created -> fall back
		}
		patches = append(patches, patch{node: node, value: w.value})
	}
	lines := strings.Split(string(originalData), "\n")
	for _, p := range patches {
		if !patchScalarValue(lines, p.node, p.value) {
			return nil, false // value not safely rewritable -> fall back
		}
	}
	return []byte(strings.Join(lines, "\n")), true
}

// patchScalarValue replaces the value text of a scalar node at its recorded
// Line/Column in lines, leaving the remainder of the line (trailing comments
// and their alignment) untouched. It returns false when the raw text at that
// position does not match the node's decoded value (e.g. quoted or block
// scalars) or when the replacement would not be a safe plain scalar.
func patchScalarValue(lines []string, node *yaml.Node, newValue string) bool {
	if node == nil || node.Line < 1 || node.Line > len(lines) {
		return false
	}
	line := lines[node.Line-1]
	col := node.Column - 1
	if col < 0 || col > len(line) {
		return false
	}
	old := node.Value
	if old == "" || !strings.HasPrefix(line[col:], old) {
		return false
	}
	if !isSafePlainScalar(newValue) {
		return false
	}
	lines[node.Line-1] = line[:col] + newValue + line[col+len(old):]
	return true
}

// isSafePlainScalar reports whether s can be written as an unquoted YAML plain
// scalar without changing tokenisation. Conservative: anything that could need
// quoting returns false, causing a fall back to re-marshalling.
func isSafePlainScalar(s string) bool {
	if s == "" || strings.ContainsAny(s, "\n\t") {
		return false
	}
	if strings.Contains(s, ": ") || strings.Contains(s, " #") {
		return false
	}
	if strings.HasPrefix(s, " ") || strings.HasSuffix(s, " ") || strings.HasSuffix(s, ":") {
		return false
	}
	switch s[0] {
	case '!', '&', '*', '?', '|', '>', '%', '@', '`', '"', '\'', '#', ',', '[', ']', '{', '}', '-':
		return false
	}
	return true
}

func parseDefaultTarget(appNamespace string, appName string, path string, kubeClient *kube.ImageUpdaterKubernetesClient) string {
	// when running from command line and argocd-namespace is not set, e.g., via --argocd-namespace option,
	// kubeClient.Namespace may be resolved to "default". In this case, also use the file name without namespace
	if appNamespace == kubeClient.KubeClient.Namespace || kubeClient.KubeClient.Namespace == "default" || appNamespace == "" {
		defaultTargetFile := fmt.Sprintf(common.DefaultTargetFilePatternWithoutNamespace, appName)
		return filepath.Join(path, defaultTargetFile)
	} else {
		defaultTargetFile := fmt.Sprintf(common.DefaultTargetFilePattern, appNamespace, appName)
		return filepath.Join(path, defaultTargetFile)
	}
}

func parseKustomizeBase(target string, sourcePath string) (kustomizeBase string) {
	if target == common.KustomizationPrefix {
		return filepath.Join(sourcePath, ".")
	} else if base := target[len(common.KustomizationPrefix)+1:]; strings.HasPrefix(base, "/") {
		return base[1:]
	} else {
		return filepath.Join(sourcePath, base)
	}
}

// parseTarget extracts the target path to set in the writeBackConfig configuration
func parseTarget(writeBackTarget string, sourcePath string) string {
	if writeBackTarget == common.HelmPrefix {
		return filepath.Join(sourcePath, "./", common.DefaultHelmValuesFilename)
	} else if base := writeBackTarget[len(common.HelmPrefix)+1:]; strings.HasPrefix(base, "/") {
		return base[1:]
	} else {
		return filepath.Join(sourcePath, base)
	}
}

func parseGitConfig(ctx context.Context, app *v1alpha1.Application, kubeClient *kube.ImageUpdaterKubernetesClient, settings *iuapi.WriteBackConfig, wbc *WriteBackConfig, creds string) error {
	if settings.GitConfig != nil && settings.GitConfig.Branch != nil {
		branch := *settings.GitConfig.Branch

		branches := strings.Split(strings.TrimSpace(branch), ":")
		if len(branches) > 2 {
			return fmt.Errorf("invalid format for git-branch: %v", branch)
		}
		wbc.GitBranch = branches[0]
		if len(branches) == 2 {
			wbc.GitWriteBranch = branches[1]
		}

	}
	wbc.GitRepo = getApplicationSource(ctx, app, wbc).RepoURL
	if settings.GitConfig != nil && settings.GitConfig.Repository != nil {
		repo := *settings.GitConfig.Repository
		wbc.GitRepo = repo
	}
	credsSource, err := getGitCredsSource(ctx, creds, kubeClient, wbc, app.GetNamespace())
	if err != nil {
		return fmt.Errorf("invalid git credentials source: %v", err)
	}
	wbc.GetCreds = credsSource
	return nil
}

func commitChangesLocked(ctx context.Context, applicationImages *ApplicationImages, state *SyncIterationState, changeList []ChangeEntry) error {
	logCtx := log.LoggerFromContext(ctx)
	wbc := applicationImages.WriteBackConfig
	if wbc.RequiresLocking() {
		lock := state.GetRepositoryLock(wbc.GitRepo)
		lock.Lock()
		defer lock.Unlock()
	}

	if wbc.PRProvider > 0 {
		targetKey := wbc.WriteBackTargetKey()
		if !state.MarkPRCreated(targetKey) {
			logCtx.Infof("Skipping PR creation: another application already created a PR for the same write-back target in this cycle")
			return nil
		}
	}

	return commitChanges(ctx, applicationImages, changeList)
}

// commitChanges commits any changes required for updating one or more images
// after the UpdateApplication cycle has finished.
func commitChanges(ctx context.Context, applicationImages *ApplicationImages, changeList []ChangeEntry) error {
	app := applicationImages.Application
	wbc := applicationImages.WriteBackConfig
	if wbc == nil {
		return fmt.Errorf("write back method is not defined")
	}
	switch wbc.Method {
	case WriteBackApplication:
		_, err := wbc.ArgoClient.UpdateSpec(ctx, &application.ApplicationUpdateSpecRequest{
			Name:         &app.Name,
			AppNamespace: &app.Namespace,
			Spec:         &app.Spec,
		})
		if err != nil {
			return err
		}
	case WriteBackGit:
		if wbc.PRProvider > 0 {
			// create a Pull Request if provider was set
			// if the kustomize base is set, the target is a kustomization
			if wbc.KustomizeBase != "" {
				return commitChangesPR(ctx, applicationImages, changeList, writeKustomization)
			}
			return commitChangesPR(ctx, applicationImages, changeList, writeOverrides)
		}
		// if the kustomize base is set, the target is a kustomization
		if wbc.KustomizeBase != "" {
			return commitChangesGit(ctx, applicationImages, changeList, writeKustomization)
		}
		return commitChangesGit(ctx, applicationImages, changeList, writeOverrides)
	default:
		return fmt.Errorf("unknown write back method set: %d", wbc.Method)
	}
	return nil
}

func isOnlyWhitespace(data []byte) bool {
	if len(data) == 0 {
		return true
	}
	for i := 0; i < len(data); {
		r, size := utf8.DecodeRune(data[i:])
		if !unicode.IsSpace(r) {
			return false
		}
		i += size
	}
	return true
}
