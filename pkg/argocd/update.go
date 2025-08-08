package argocd

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

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

	"github.com/argoproj/argo-cd/v2/pkg/apiclient/application"
	"github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
)

// UpdateApplication update all images of a single application. Will run in a goroutine.
func UpdateApplication(ctx context.Context, updateConf *UpdateConfiguration, state *SyncIterationState) ImageUpdaterResult {
	baseLogger := log.LoggerFromContext(ctx)

	var needUpdate bool = false
	result := ImageUpdaterResult{}
	app := updateConf.UpdateApp.Application.GetName()
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

		rep, err := registry.GetRegistryEndpoint(imageOpCtx, applicationImage.RegistryURL)
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

		// The endpoint can provide default credentials for pulling images
		err = rep.SetEndpointCredentials(imageOpCtx, updateConf.KubeClient.KubeClient)
		if err != nil {
			imgCtx.Errorf("Could not set registry endpoint credentials: %v", err)
			result.NumErrors += 1
			continue
		}

		imgCredSrc := GetParameterPullSecret(imageOpCtx, applicationImage)
		var creds *image.Credential = &image.Credential{}
		if imgCredSrc != nil {
			creds, err = imgCredSrc.FetchCredentials(imageOpCtx, rep.RegistryAPI, updateConf.KubeClient.KubeClient)
			if err != nil {
				imgCtx.Warnf("Could not fetch credentials: %v", err)
				result.NumErrors += 1
				continue
			}
		}

		regClient, err := updateConf.NewRegFN(rep, creds.Username, creds.Password)
		if err != nil {
			imgCtx.Errorf("Could not create registry client: %v", err)
			result.NumErrors += 1
			continue
		}

		// Get list of available image tags from the repository
		tags, err := rep.GetTags(imageOpCtx, applicationImage.ContainerImage, regClient, &vc)
		if err != nil {
			imgCtx.Errorf("Could not get tags from registry: %v", err)
			result.NumErrors += 1
			continue
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
				}
			}
		} else {
			baseLogger.Infof("Dry run - not committing %d changes to application", result.NumImagesUpdated)
		}
	}

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
	appSource := GetApplicationSource(ctx, app)

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
		if appSource.Helm == nil {
			return []byte{}, nil
		}

		if wbc != nil && strings.HasPrefix(wbc.Target, common.HelmPrefix) {
			images := GetImagesAndAliasesFromApplication(applicationImages)

			helmNewValues := yaml.Node{}
			err = yaml.Unmarshal(originalData, &helmNewValues)
			if err != nil {
				return nil, err
			}

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
					helmParamVer := getHelmParam(appSource.Helm.Parameters, helmParamVersion)
					if helmParamVer == nil {
						return nil, fmt.Errorf("%s parameter not found", helmParamVersion)
					}
					err = setHelmValue(&helmNewValues, helmParamVersion, helmParamVer.Value)
					if err != nil {
						return nil, fmt.Errorf("failed to set image parameter version value: %v", err)
					}
				}

				helmParamN := getHelmParam(appSource.Helm.Parameters, helmParamName)
				if helmParamN == nil {
					return nil, fmt.Errorf("%s parameter not found", helmParamName)
				}

				err = setHelmValue(&helmNewValues, helmParamName, helmParamN.Value)
				if err != nil {
					return nil, fmt.Errorf("failed to set image parameter name value: %v", err)
				}
			}

			override, err = marshalWithIndent(&helmNewValues, defaultIndent)
		} else {
			var params helmOverride
			newParams := helmOverride{
				Helm: helmParameters{
					Parameters: appSource.Helm.Parameters,
				},
			}

			outputParams := appSource.Helm.ValuesYAML()
			log.Debugf("values: '%s'", outputParams)

			if len(originalData) == 0 {
				override, err = marshalWithIndent(newParams, defaultIndent)
				break
			}
			err = yaml.Unmarshal(originalData, &params)
			if err != nil {
				override, err = marshalWithIndent(newParams, defaultIndent)
				break
			}
			mergeHelmOverride(&params, &newParams)
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
	for _, newImage := range *o.Kustomize.Images {
		found := false
		newContainerImage := image.NewFromIdentifier(string(newImage))
		for idx, existingImage := range *t.Kustomize.Images {
			existingContainerImage := image.NewFromIdentifier(string(existingImage))
			if newContainerImage.ImageName == existingContainerImage.ImageName &&
				newContainerImage.RegistryURL == existingContainerImage.RegistryURL {
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
		if idx, found := findHelmValuesKey(current, k); found {
			// Navigate deeper into the map
			current = (*current).Content[idx]
			// unpack one level of alias; an alias of an alias is not supported
			if current.Kind == yaml.AliasNode {
				current = current.Alias
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
	wbc.GitRepo = getApplicationSource(ctx, app).RepoURL
	if settings.GitConfig != nil && settings.GitConfig.Repository != nil {
		repo := *settings.GitConfig.Repository
		wbc.GitRepo = repo
	}
	credsSource, err := getGitCredsSource(ctx, creds, kubeClient, wbc)
	if err != nil {
		return fmt.Errorf("invalid git credentials source: %v", err)
	}
	wbc.GetCreds = credsSource
	return nil
}

func commitChangesLocked(ctx context.Context, applicationImages *ApplicationImages, state *SyncIterationState, changeList []ChangeEntry) error {
	wbc := applicationImages.WriteBackConfig
	if wbc.RequiresLocking() {
		lock := state.GetRepositoryLock(wbc.GitRepo)
		lock.Lock()
		defer lock.Unlock()
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
