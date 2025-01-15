package argocd

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
	"time"

	"golang.org/x/exp/slices"

	"github.com/argoproj-labs/argocd-image-updater/ext/git"
	"github.com/argoproj-labs/argocd-image-updater/pkg/common"
	"github.com/argoproj-labs/argocd-image-updater/pkg/kube"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/image"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/registry"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/tag"

	"github.com/argoproj/argo-cd/v2/pkg/apiclient/application"
	"github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"gopkg.in/yaml.v2"
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

type UpdateConfiguration struct {
	NewRegFN               registry.NewRegistryClient
	ArgoClient             ArgoCD
	KubeClient             *kube.ImageUpdaterKubernetesClient
	UpdateApp              *ApplicationImages
	DryRun                 bool
	GitCommitUser          string
	GitCommitEmail         string
	GitCommitMessage       *template.Template
	GitCommitSigningKey    string
	GitCommitSigningMethod string
	GitCommitSignOff       bool
	DisableKubeEvents      bool
	IgnorePlatforms        bool
	GitCreds               git.CredsStore
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
	GitClient              git.Client
	GetCreds               GitCredsSource
	GitBranch              string
	GitWriteBranch         string
	GitCommitUser          string
	GitCommitEmail         string
	GitCommitMessage       string
	GitCommitSigningKey    string
	GitCommitSigningMethod string
	GitCommitSignOff       bool
	KustomizeBase          string
	Target                 string
	GitRepo                string
	GitCreds               git.CredsStore
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

// ChangeEntry represents an image that has been changed by Image Updater
type ChangeEntry struct {
	Image  *image.ContainerImage
	OldTag *tag.ImageTag
	NewTag *tag.ImageTag
}

// SyncIterationState holds shared state of a running update operation
type SyncIterationState struct {
	lock            sync.Mutex
	repositoryLocks map[string]*sync.Mutex
}

// NewSyncIterationState returns a new instance of SyncIterationState
func NewSyncIterationState() *SyncIterationState {
	return &SyncIterationState{
		repositoryLocks: make(map[string]*sync.Mutex),
	}
}

// GetRepositoryLock returns the lock for a specified repository
func (state *SyncIterationState) GetRepositoryLock(repository string) *sync.Mutex {
	state.lock.Lock()
	defer state.lock.Unlock()

	lock, exists := state.repositoryLocks[repository]
	if !exists {
		lock = &sync.Mutex{}
		state.repositoryLocks[repository] = lock
	}

	return lock
}

// RequiresLocking returns true if write-back method requires repository locking
func (wbc *WriteBackConfig) RequiresLocking() bool {
	switch wbc.Method {
	case WriteBackGit:
		return true
	default:
		return false
	}
}

// UpdateApplication update all images of a single application. Will run in a goroutine.
func UpdateApplication(updateConf *UpdateConfiguration, state *SyncIterationState) ImageUpdaterResult {
	var needUpdate bool = false

	result := ImageUpdaterResult{}
	app := updateConf.UpdateApp.Application.GetName()
	changeList := make([]ChangeEntry, 0)

	// Get all images that are deployed with the current application
	applicationImages := GetImagesFromApplication(&updateConf.UpdateApp.Application)

	result.NumApplicationsProcessed += 1

	// Loop through all images of current application, and check whether one of
	// its images is eligible for updating.
	//
	// Whether an image qualifies for update is dependent on semantic version
	// constraints which are part of the application's annotation values.
	//
	for _, applicationImage := range updateConf.UpdateApp.Images {
		updateableImage := applicationImages.ContainsImage(applicationImage, false)
		if updateableImage == nil {
			log.WithContext().AddField("application", app).Debugf("Image '%s' seems not to be live in this application, skipping", applicationImage.ImageName)
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

		imgCtx := log.WithContext().
			AddField("application", app).
			AddField("registry", updateableImage.RegistryURL).
			AddField("image_name", updateableImage.ImageName).
			AddField("image_tag", updateableImage.ImageTag).
			AddField("alias", applicationImage.ImageAlias)

		if updateableImage.KustomizeImage != nil {
			imgCtx.AddField("kustomize_image", updateableImage.KustomizeImage)
		}

		imgCtx.Debugf("Considering this image for update")

		rep, err := registry.GetRegistryEndpoint(applicationImage.RegistryURL)
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

		vc.Strategy = applicationImage.GetParameterUpdateStrategy(updateConf.UpdateApp.Application.Annotations, common.ImageUpdaterAnnotationPrefix)
		vc.MatchFunc, vc.MatchArgs = applicationImage.GetParameterMatch(updateConf.UpdateApp.Application.Annotations, common.ImageUpdaterAnnotationPrefix)
		vc.IgnoreList = applicationImage.GetParameterIgnoreTags(updateConf.UpdateApp.Application.Annotations, common.ImageUpdaterAnnotationPrefix)
		vc.Options = applicationImage.
			GetPlatformOptions(updateConf.UpdateApp.Application.Annotations, updateConf.IgnorePlatforms, common.ImageUpdaterAnnotationPrefix).
			WithMetadata(vc.Strategy.NeedsMetadata()).
			WithLogger(imgCtx.AddField("application", app))

		// If a strategy needs meta-data and tagsortmode is set for the
		// registry, let the user know.
		if rep.TagListSort > registry.TagListSortUnsorted && vc.Strategy.NeedsMetadata() {
			imgCtx.Infof("taglistsort is set to '%s' but update strategy '%s' requires metadata. Results may not be what you expect.", rep.TagListSort.String(), vc.Strategy.String())
		}

		// The endpoint can provide default credentials for pulling images
		err = rep.SetEndpointCredentials(updateConf.KubeClient.KubeClient)
		if err != nil {
			imgCtx.Errorf("Could not set registry endpoint credentials: %v", err)
			result.NumErrors += 1
			continue
		}

		imgCredSrc := applicationImage.GetParameterPullSecret(updateConf.UpdateApp.Application.Annotations, common.ImageUpdaterAnnotationPrefix)
		var creds *image.Credential = &image.Credential{}
		if imgCredSrc != nil {
			creds, err = imgCredSrc.FetchCredentials(rep.RegistryAPI, updateConf.KubeClient.KubeClient)
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

		// If the user has specified digest as update strategy, but the running
		// image is configured to use a tag and no digest, we need to set an
		// initial dummy digest, so that tag.Equals() will return false.
		// TODO: Fix this. This is just a workaround.
		if vc.Strategy == image.StrategyDigest {
			if !updateableImage.ImageTag.IsDigest() {
				log.Tracef("Setting dummy digest for image %s", updateableImage.GetFullNameWithTag())
				updateableImage.ImageTag.TagDigest = "dummy"
			}
		}

		if needsUpdate(updateableImage, applicationImage, latest) {
			appImageWithTag := applicationImage.WithTag(latest)
			appImageFullNameWithTag := appImageWithTag.GetFullNameWithTag()

			// Check if new image is already set in Application Spec when write back is set to argocd
			// and compare with new image
			appImageSpec, err := getAppImage(&updateConf.UpdateApp.Application, appImageWithTag)
			if err != nil {
				continue
			}
			if appImageSpec == appImageFullNameWithTag {
				imgCtx.Infof("New image %s already set in spec", appImageFullNameWithTag)
				continue
			}

			needUpdate = true
			imgCtx.Infof("Setting new image to %s", appImageFullNameWithTag)

			err = setAppImage(&updateConf.UpdateApp.Application, appImageWithTag)

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
			err = setAppImage(&updateConf.UpdateApp.Application, applicationImage.WithTag(updateableImage.ImageTag))
			if err != nil {
				imgCtx.Errorf("Error while trying to update image: %v", err)
				result.NumErrors += 1
			}
			imgCtx.Debugf("Image '%s' already on latest allowed version", updateableImage.GetFullNameWithTag())
		}
	}

	wbc, err := getWriteBackConfig(&updateConf.UpdateApp.Application, updateConf.KubeClient, updateConf.ArgoClient)
	if err != nil {
		return result
	}
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
			wbc.GitCommitMessage = TemplateCommitMessage(updateConf.GitCommitMessage, updateConf.UpdateApp.Application.Name, changeList)
		}
		if updateConf.GitCommitSigningKey != "" {
			wbc.GitCommitSigningKey = updateConf.GitCommitSigningKey
		}
		wbc.GitCommitSigningMethod = updateConf.GitCommitSigningMethod
		wbc.GitCommitSignOff = updateConf.GitCommitSignOff
	}

	if needUpdate {
		logCtx := log.WithContext().AddField("application", app)
		log.Debugf("Using commit message: %s", wbc.GitCommitMessage)
		if !updateConf.DryRun {
			logCtx.Infof("Committing %d parameter update(s) for application %s", result.NumImagesUpdated, app)
			err := commitChangesLocked(&updateConf.UpdateApp.Application, wbc, state, changeList)
			if err != nil {
				logCtx.Errorf("Could not update application spec: %v", err)
				result.NumErrors += 1
				result.NumImagesUpdated = 0
			} else {
				logCtx.Infof("Successfully updated the live application spec")
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
						logCtx.Warnf("Event could not be sent: %v", err)
					}
				}
			}
		} else {
			logCtx.Infof("Dry run - not committing %d changes to application", result.NumImagesUpdated)
		}
	}

	return result
}

func needsUpdate(updateableImage *image.ContainerImage, applicationImage *image.ContainerImage, latest *tag.ImageTag) bool {
	// If the latest tag does not match image's current tag or the kustomize image is different, it means we have an update candidate.
	return !updateableImage.ImageTag.Equals(latest) || applicationImage.KustomizeImage != nil && applicationImage.DiffersFrom(updateableImage, false)
}

func getAppImage(app *v1alpha1.Application, img *image.ContainerImage) (string, error) {
	var err error
	if appType := GetApplicationType(app); appType == ApplicationTypeKustomize {
		return GetKustomizeImage(app, img)
	} else if appType == ApplicationTypeHelm {
		return GetHelmImage(app, img)
	} else {
		err = fmt.Errorf("could not update application %s - neither Helm nor Kustomize application", app)
		return "", err
	}
}

func setAppImage(app *v1alpha1.Application, img *image.ContainerImage) error {
	var err error
	if appType := GetApplicationType(app); appType == ApplicationTypeKustomize {
		err = SetKustomizeImage(app, img)
	} else if appType == ApplicationTypeHelm {
		err = SetHelmImage(app, img)
	} else {
		err = fmt.Errorf("could not update application %s - neither Helm nor Kustomize application", app)
	}
	return err
}

// marshalParamsOverride marshals the parameter overrides of a given application
// into YAML bytes
func marshalParamsOverride(app *v1alpha1.Application, originalData []byte) ([]byte, error) {
	var override []byte
	var err error

	appType := GetApplicationType(app)
	appSource := getApplicationSource(app)

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
			override, err = yaml.Marshal(newParams)
			break
		}
		err = yaml.Unmarshal(originalData, &params)
		if err != nil {
			override, err = yaml.Marshal(newParams)
			break
		}
		mergeKustomizeOverride(&params, &newParams)
		override, err = yaml.Marshal(params)
	case ApplicationTypeHelm:
		if appSource.Helm == nil {
			return []byte{}, nil
		}

		if strings.HasPrefix(app.Annotations[common.WriteBackTargetAnnotation], common.HelmPrefix) {
			images := GetImagesAndAliasesFromApplication(app)

			helmNewValues := yaml.MapSlice{}
			err = yaml.Unmarshal(originalData, &helmNewValues)
			if err != nil {
				return nil, err
			}

			for _, c := range images {
				if c.ImageAlias == "" {
					continue
				}

				helmAnnotationParamName, helmAnnotationParamVersion := getHelmParamNamesFromAnnotation(app.Annotations, c)

				if helmAnnotationParamName == "" {
					return nil, fmt.Errorf("could not find an image-name annotation for image %s", c.ImageName)
				}
				// for image-spec annotation, helmAnnotationParamName holds image-spec annotation value,
				// and helmAnnotationParamVersion is empty
				if helmAnnotationParamVersion == "" {
					if c.GetParameterHelmImageSpec(app.Annotations, common.ImageUpdaterAnnotationPrefix) == "" {
						// not a full image-spec, so image-tag is required
						return nil, fmt.Errorf("could not find an image-tag annotation for image %s", c.ImageName)
					}
				} else {
					// image-tag annotation is present, so continue to process image-tag
					helmParamVersion := getHelmParam(appSource.Helm.Parameters, helmAnnotationParamVersion)
					if helmParamVersion == nil {
						return nil, fmt.Errorf("%s parameter not found", helmAnnotationParamVersion)
					}
					err = setHelmValue(&helmNewValues, helmAnnotationParamVersion, helmParamVersion.Value)
					if err != nil {
						return nil, fmt.Errorf("failed to set image parameter version value: %v", err)
					}
				}

				helmParamName := getHelmParam(appSource.Helm.Parameters, helmAnnotationParamName)
				if helmParamName == nil {
					return nil, fmt.Errorf("%s parameter not found", helmAnnotationParamName)
				}

				err = setHelmValue(&helmNewValues, helmAnnotationParamName, helmParamName.Value)
				if err != nil {
					return nil, fmt.Errorf("failed to set image parameter name value: %v", err)
				}
			}

			override, err = yaml.Marshal(helmNewValues)
		} else {
			var params helmOverride
			newParams := helmOverride{
				Helm: helmParameters{
					Parameters: appSource.Helm.Parameters,
				},
			}

			outputParams := appSource.Helm.ValuesYAML()
			log.WithContext().AddField("application", app).Debugf("values: '%s'", outputParams)

			if len(originalData) == 0 {
				override, err = yaml.Marshal(newParams)
				break
			}
			err = yaml.Unmarshal(originalData, &params)
			if err != nil {
				override, err = yaml.Marshal(newParams)
				break
			}
			mergeHelmOverride(&params, &newParams)
			override, err = yaml.Marshal(params)
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
	for _, image := range *o.Kustomize.Images {
		idx := t.Kustomize.Images.Find(image)
		if idx != -1 {
			(*t.Kustomize.Images)[idx] = image
			continue
		}
		*t.Kustomize.Images = append(*t.Kustomize.Images, image)
	}
}

// Check if a key exists in a MapSlice and return its index and value
func findHelmValuesKey(m yaml.MapSlice, key string) (int, bool) {
	for i, item := range m {
		if item.Key == key {
			return i, true
		}
	}
	return -1, false
}

// set value of the parameter passed from the annotations.
func setHelmValue(currentValues *yaml.MapSlice, key string, value interface{}) error {
	// Check if the full key exists
	if idx, found := findHelmValuesKey(*currentValues, key); found {
		(*currentValues)[idx].Value = value
		return nil
	}

	var err error
	keys := strings.Split(key, ".")
	current := currentValues
	var parent *yaml.MapSlice
	parentIdx := -1

	for i, k := range keys {
		if idx, found := findHelmValuesKey(*current, k); found {
			if i == len(keys)-1 {
				// If we're at the final key, set the value and return
				(*current)[idx].Value = value
				return nil
			} else {
				// Navigate deeper into the map
				if nestedMap, ok := (*current)[idx].Value.(yaml.MapSlice); ok {
					parent = current
					parentIdx = idx
					current = &nestedMap
				} else {
					return fmt.Errorf("unexpected type %T for key %s", (*current)[idx].Value, k)
				}
			}
		} else {
			newCurrent := yaml.MapSlice{}
			var newParent yaml.MapSlice

			if i == len(keys)-1 {
				newParent = append(*current, yaml.MapItem{Key: k, Value: value})
			} else {
				newParent = append(*current, yaml.MapItem{Key: k, Value: newCurrent})
			}

			if parent == nil {
				*currentValues = newParent
			} else {
				// if parentIdx has not been set (parent element is also new), set it to the last element
				if parentIdx == -1 {
					parentIdx = len(*parent) - 1
					if parentIdx < 0 {
						parentIdx = 0
					}
				}
				(*parent)[parentIdx].Value = newParent
			}

			parent = &newParent
			current = &newCurrent
			parentIdx = -1
		}
	}

	return err
}

func getWriteBackConfig(app *v1alpha1.Application, kubeClient *kube.ImageUpdaterKubernetesClient, argoClient ArgoCD) (*WriteBackConfig, error) {
	wbc := &WriteBackConfig{}
	// Default write-back is to use Argo CD API
	wbc.Method = WriteBackApplication
	wbc.ArgoClient = argoClient
	wbc.Target = parseDefaultTarget(app.GetNamespace(), app.Name, getApplicationSource(app).Path, kubeClient)

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
		target, ok := app.Annotations[common.WriteBackTargetAnnotation]
		if ok && strings.HasPrefix(target, common.KustomizationPrefix) {
			wbc.KustomizeBase = parseKustomizeBase(target, getApplicationSource(app).Path)
		} else if ok && strings.HasPrefix(target, common.HelmPrefix) { // This keeps backward compatibility
			wbc.Target = parseTarget(target, getApplicationSource(app).Path)
		} else if ok { // This keeps backward compatibility
			wbc.Target = app.Annotations[common.WriteBackTargetAnnotation]
		}
		if err := parseGitConfig(app, kubeClient, wbc, creds); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("invalid update mechanism: %s", method)
	}

	return wbc, nil
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

func parseGitConfig(app *v1alpha1.Application, kubeClient *kube.ImageUpdaterKubernetesClient, wbc *WriteBackConfig, creds string) error {
	branch, ok := app.Annotations[common.GitBranchAnnotation]
	if ok {
		branches := strings.Split(strings.TrimSpace(branch), ":")
		if len(branches) > 2 {
			return fmt.Errorf("invalid format for git-branch annotation: %v", branch)
		}
		wbc.GitBranch = branches[0]
		if len(branches) == 2 {
			wbc.GitWriteBranch = branches[1]
		}
	}
	wbc.GitRepo = getApplicationSource(app).RepoURL
	repo, ok := app.Annotations[common.GitRepositoryAnnotation]
	if ok {
		wbc.GitRepo = repo
	}
	credsSource, err := getGitCredsSource(creds, kubeClient, wbc)
	if err != nil {
		return fmt.Errorf("invalid git credentials source: %v", err)
	}
	wbc.GetCreds = credsSource
	return nil
}

func commitChangesLocked(app *v1alpha1.Application, wbc *WriteBackConfig, state *SyncIterationState, changeList []ChangeEntry) error {
	if wbc.RequiresLocking() {
		lock := state.GetRepositoryLock(wbc.GitRepo)
		lock.Lock()
		defer lock.Unlock()
	}

	return commitChanges(app, wbc, changeList)
}

// commitChanges commits any changes required for updating one or more images
// after the UpdateApplication cycle has finished.
func commitChanges(app *v1alpha1.Application, wbc *WriteBackConfig, changeList []ChangeEntry) error {
	switch wbc.Method {
	case WriteBackApplication:
		_, err := wbc.ArgoClient.UpdateSpec(context.TODO(), &application.ApplicationUpdateSpecRequest{
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
			return commitChangesGit(app, wbc, changeList, writeKustomization)
		}
		return commitChangesGit(app, wbc, changeList, writeOverrides)
	default:
		return fmt.Errorf("unknown write back method set: %d", wbc.Method)
	}
	return nil
}
