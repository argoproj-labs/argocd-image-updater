package argocd

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"github.com/argoproj/argo-cd/v3/pkg/apiclient/application"
	argocdapi "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	iuapi "github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"
	"github.com/argoproj-labs/argocd-image-updater/pkg/common"
	"github.com/argoproj-labs/argocd-image-updater/pkg/kube"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/image"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
)

// ArgoCDK8sClient is a controller-runtime based client specifically for ArgoCD application operations.
// It wraps ctrlclient.Client to provide ArgoCD-specific functionality like getting, listing,
// and updating ArgoCD Application resources.
type ArgoCDK8sClient struct {
	ctrlclient.Client
}

// NewArgoCDK8sClient creates a new ArgoCD-specific Kubernetes client for managing ArgoCD applications.
// This client is designed to work with controller-runtime and provides methods for
// ArgoCD Application CRUD operations.
func NewArgoCDK8sClient(ctrlClient ctrlclient.Client) (*ArgoCDK8sClient, error) {
	return &ArgoCDK8sClient{ctrlClient}, nil
}

// ArgoCD is the interface for accessing Argo CD functions we need
//
//go:generate mockery --name ArgoCD --output ./mocks --outpkg mocks
type ArgoCD interface {
	GetApplication(ctx context.Context, appNamespace string, appName string) (*argocdapi.Application, error)
	UpdateSpec(ctx context.Context, spec *application.ApplicationUpdateSpecRequest) (*argocdapi.ApplicationSpec, error)
}

// GetApplication retrieves a single application by its name and namespace.
func (client *ArgoCDK8sClient) GetApplication(ctx context.Context, appNamespace string, appName string) (*argocdapi.Application, error) {
	app := &argocdapi.Application{}

	if err := client.Get(ctx, types.NamespacedName{Namespace: appNamespace, Name: appName}, app); err != nil {
		return nil, err
	}
	return app, nil
}

// UpdateSpec updates the spec for given application
func (client *ArgoCDK8sClient) UpdateSpec(ctx context.Context, spec *application.ApplicationUpdateSpecRequest) (*argocdapi.ApplicationSpec, error) {
	log := log.LoggerFromContext(ctx)
	app := &argocdapi.Application{}
	var err error

	// Use RetryOnConflict to handle potential conflicts gracefully.
	err = retry.RetryOnConflict(retry.DefaultRetry, func() error {
		// Get the latest version of the Application within the retry loop.
		app, err = client.GetApplication(ctx, spec.GetAppNamespace(), spec.GetName())
		if err != nil {
			log.Errorf("could not get application: %s, error: %v", spec.GetName(), err)
			return err
		}

		app.Spec = *spec.Spec

		// Attempt to update the object. If there is a conflict,
		// RetryOnConflict will automatically re-fetch and re-apply the changes.
		return client.Update(ctx, app)
	})

	if err != nil {
		log.Errorf("could not update application spec for %s: %v", spec.GetName(), err)
		return nil, fmt.Errorf("failed to update application spec for %s after retries: %w", spec.GetName(), err)
	}

	log.Infof("Successfully updated application spec for %s", spec.GetName())
	return &app.Spec, nil
}

// nameMatchesPatterns Matches a name against a list of patterns
func nameMatchesPatterns(ctx context.Context, name string, patterns []string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, p := range patterns {
		if m, _ := nameMatchesPattern(ctx, name, p); m {
			return true
		}
	}
	return false
}

// nameMatchesPattern Matches a name against a pattern
func nameMatchesPattern(ctx context.Context, name string, pattern string) (bool, error) {
	log := log.LoggerFromContext(ctx)
	log.Tracef("Matching application name %s against pattern %s", name, pattern)

	m, err := filepath.Match(pattern, name)
	if err != nil {
		log.Warnf("Invalid application name pattern '%s': %v", pattern, err)
		return false, fmt.Errorf("could not compile name pattern '%s': %w", pattern, err)
	}
	log.Tracef("Matched application name %s against pattern %s: %v", name, pattern, m)
	return m, nil
}

// nameMatchesLabels checks if the given labels match the provided LabelSelector.
// It returns true if the selectors are nil (no filtering), or if all MatchLabels
// and MatchExpressions conditions are met.
func nameMatchesLabels(ctx context.Context, appLabels map[string]string, selectors *metav1.LabelSelector) bool {
	log := log.LoggerFromContext(ctx)
	if selectors == nil {
		return true // No selectors means no filtering by labels
	}

	selector, err := metav1.LabelSelectorAsSelector(selectors)
	if err != nil {
		// An invalid selector should not match anything.
		log.Warnf("Invalid label selector provided: %v", err)
		return false
	}

	result := selector.Matches(labels.Set(appLabels))
	log.Tracef("Matched labels %v against selector %v: %v", appLabels, selectors, result)
	return result
}

// processApplicationForUpdate checks if an application is of a supported type,
// and if so, creates an ApplicationImages struct and adds it to the update map.
func processApplicationForUpdate(ctx context.Context, app *argocdapi.Application, appRef iuapi.ApplicationRef, appCommonUpdateSettings *iuapi.CommonUpdateSettings, appWBCSettings *WriteBackConfig, appNSName string, appsForUpdate map[string]ApplicationImages, webhookEvent *WebhookEvent) {
	log := log.LoggerFromContext(ctx)
	sourceType := getApplicationSourceType(app, appWBCSettings)

	// Check for valid application type
	if !IsValidApplicationType(app, appWBCSettings) {
		log.Warnf("skipping app '%s' of type '%s' because it's not of supported source type", appNSName, sourceType)
		return
	}
	log.Tracef("processing app '%s' of type '%v'", appNSName, sourceType)

	imageList := parseImageList(ctx, appRef.Images, appCommonUpdateSettings, webhookEvent)

	if imageList == nil || len(*imageList) == 0 {
		return
	}

	appImages := ApplicationImages{
		Application:     *app,
		WriteBackConfig: appWBCSettings,
		Images:          *imageList,
	}
	appsForUpdate[appNSName] = appImages
}

// stripBracketsRegex is used to remove character set wildcards like [a-z]
// from a pattern before calculating the number of literal characters for specificity.
// It is compiled once at the package level for performance.
var stripBracketsRegex = regexp.MustCompile(`\[.*?]`)

// calculateSpecificity computes a numerical score for an ApplicationRef to determine
// its precedence. A higher score means higher specificity.
func calculateSpecificity(applicationRef iuapi.ApplicationRef) int {
	score := 0
	pattern := applicationRef.NamePattern

	// 1. Check for an exact name match (highest precedence).
	// We define an exact match as not containing any glob wildcards.
	if !strings.ContainsAny(pattern, "*?[]") {
		score += 1_000_000
	}

	// 2. Add points for the number of literal characters in the pattern.
	// This makes "app-prod-*" more specific than "app-*".

	// First, remove character set wildcards like [a-z] entirely.
	patternWithoutSets := stripBracketsRegex.ReplaceAllString(pattern, "")

	// Then, remove the other wildcards and count the length of what's left.
	literals := strings.NewReplacer("*", "", "?", "").Replace(patternWithoutSets)
	score += len(literals)

	// 3. Add a significant bonus if a label selector is present.
	if applicationRef.LabelSelectors != nil {
		score += 10_000

		// 4. Add smaller points for each label/expression in the selector.
		// This makes a more complex selector win over a simpler one.
		if applicationRef.LabelSelectors.MatchLabels != nil {
			score += len(applicationRef.LabelSelectors.MatchLabels) * 100
		}
		if applicationRef.LabelSelectors.MatchExpressions != nil {
			score += len(applicationRef.LabelSelectors.MatchExpressions) * 100
		}
	}
	return score
}

// sortApplicationRefs sorts a slice of ApplicationRef objects from most specific
// to least specific based on their calculated specificity score. This ensures
// that more specific rules are applied before broader ones.
func sortApplicationRefs(applicationRefs []iuapi.ApplicationRef) []iuapi.ApplicationRef {
	if applicationRefs == nil {
		return []iuapi.ApplicationRef{}
	}
	// Create a copy of the slice to avoid modifying the original.
	sortedRefs := slices.Clone(applicationRefs)

	// Sort the slice from most specific to least specific.
	slices.SortStableFunc(sortedRefs, func(a, b iuapi.ApplicationRef) int {
		// We want descending order (higher score first), so we compare B to A.
		return cmp.Compare(calculateSpecificity(b), calculateSpecificity(a))
	})
	return sortedRefs
}

// FilterApplicationsForUpdate Retrieve a list of applications from ArgoCD that qualify for image updates
// Application needs either to be of type Kustomize or Helm.
func FilterApplicationsForUpdate(ctx context.Context, ctrlClient *ArgoCDK8sClient, kubeClient *kube.ImageUpdaterKubernetesClient, cr *iuapi.ImageUpdater, webhookEvent *WebhookEvent) (map[string]ApplicationImages, error) {
	log := log.LoggerFromContext(ctx)

	// Validate CR configuration
	if len(cr.Spec.ApplicationRefs) == 0 {
		return nil, fmt.Errorf("no application references defined in ImageUpdater CR")
	}

	// Pre-validate all name patterns in the CR to fail fast on misconfiguration.
	for _, appRef := range cr.Spec.ApplicationRefs {
		if _, err := filepath.Match(appRef.NamePattern, "validation"); err != nil {
			// Wrap the error to provide context about which pattern is invalid.
			return nil, fmt.Errorf("invalid application name pattern '%s': %w", appRef.NamePattern, err)
		}
	}

	allAppsInNamespace := &argocdapi.ApplicationList{}
	listOpts := []ctrlclient.ListOption{
		ctrlclient.InNamespace(cr.Spec.Namespace),
	}

	// Perform the app list operation in the target namespace cr.Spec.Namespace.
	log.Infof("Listing all applications in target namespace: %s", cr.Spec.Namespace)
	if err := ctrlClient.List(ctx, allAppsInNamespace, listOpts...); err != nil {
		log.Errorf("Failed to list applications in namespace: %s, error: %v", cr.Spec.Namespace, err)
		return nil, err
	}

	if len(allAppsInNamespace.Items) == 0 {
		log.Infof("No applications found in target namespace: %s", cr.Spec.Namespace)
		return nil, nil
	}

	var appsForUpdate = make(map[string]ApplicationImages)

	// Sort namePatterns in applicationRefs from most specific to least specific.
	applicationRefsSorted := sortApplicationRefs(cr.Spec.ApplicationRefs)

	// Establish the base global settings
	globalUpdateSettings := cr.Spec.CommonUpdateSettings

	// For each app in the list, find its best matching rule from the CR.
	for _, app := range allAppsInNamespace.Items {
		// Find the first matching rule for this application
		for _, applicationRef := range applicationRefsSorted {
			// We can ignore the error here because we pre-validated all patterns above.
			// An error from filepath.Match is the only error condition.
			matches, _ := nameMatchesPattern(ctx, app.Name, applicationRef.NamePattern)
			if matches && nameMatchesLabels(ctx, app.Labels, applicationRef.LabelSelectors) {
				localAppRef := applicationRef
				var mergedCommonUpdateSettings *iuapi.CommonUpdateSettings
				var appWBCSettings *WriteBackConfig
				var err error
				// When ReadFromApplicationAnnotations is true, we ignore all CR-based configuration
				// (Images, CommonUpdateSettings, WriteBackConfig) and instead read everything from
				// the Application's legacy argocd-image-updater.argoproj.io/* annotations.
				if applicationRef.ReadFromApplicationAnnotations != nil && *applicationRef.ReadFromApplicationAnnotations {
					log.Debugf("Read settings from application Annotations for app %s/%s", app.Namespace, app.Name)

					appRefImages, err := getImagesFromAnnotations(&app)
					if err != nil {
						log.Warnf("Could not create image list for app %s/%s, skipping: %v", app.Namespace, app.Name, err)
						continue
					}

					appRefWBC := getWriteBackConfigFromAnnotations(&app)
					appWBCSettings, err = newWBCFromSettings(ctx, &app, kubeClient, appRefWBC)
					if err != nil {
						log.Warnf("Could not create write-back config for app %s/%s, skipping: %v", app.Namespace, app.Name, err)
						continue
					}

					// Empty alias means we're reading application-wide annotations (not image-specific)
					updateStrategyAnnotations := getImageUpdateStrategyAnnotations("")
					mergedCommonUpdateSettings, err = getCommonUpdateSettingsFromAnnotations(&app, updateStrategyAnnotations)
					if err != nil {
						log.Warnf("Could not create common update settings for app %s/%s, skipping: %v", app.Namespace, app.Name, err)
						continue
					}

					// Create a local copy of applicationRef with annotation-derived values
					localAppRef.Images = appRefImages
					localAppRef.WriteBackConfig = appRefWBC
				} else {
					// Calculate the effective settings for this ApplicationRef by layering on top of global.
					log.Debugf("Read settings from Image Updater CR for app %s/%s", app.Namespace, app.Name)
					mergedCommonUpdateSettings = mergeCommonUpdateSettings(globalUpdateSettings, applicationRef.CommonUpdateSettings)
					mergedWBCSettings := mergeWBCSettings(cr.Spec.WriteBackConfig, applicationRef.WriteBackConfig)
					appWBCSettings, err = newWBCFromSettings(ctx, &app, kubeClient, mergedWBCSettings)
					if err != nil {
						log.Warnf("Could not create write-back config for app %s/%s, skipping: %v", app.Namespace, app.Name, err)
						continue
					}
				}

				appRefJSON, err := json.MarshalIndent(localAppRef, "", "  ")
				if err != nil {
					log.Warnf("Could not marshal application reference for app %s/%s", app.Namespace, app.Name)
				} else {
					log.Tracef("Resulted Image Updater object for app %s/%s: %s", app.Namespace, app.Name, string(appRefJSON))
				}
				appNSName := fmt.Sprintf("%s/%s", cr.Spec.Namespace, app.Name)
				processApplicationForUpdate(ctx, &app, localAppRef, mergedCommonUpdateSettings, appWBCSettings, appNSName, appsForUpdate, webhookEvent)
				break // Found the best match, move to the next app
			}
		}
	}
	return appsForUpdate, nil
}

// mergeCommonUpdateSettings merges a list of CommonUpdateSettings.
// The later settings in the list take precedence.
func mergeCommonUpdateSettings(settings ...*iuapi.CommonUpdateSettings) *iuapi.CommonUpdateSettings {
	merged := &iuapi.CommonUpdateSettings{}
	for _, s := range settings {
		if s == nil {
			continue
		}
		if s.UpdateStrategy != nil {
			merged.UpdateStrategy = s.UpdateStrategy
		}
		if s.AllowTags != nil {
			merged.AllowTags = s.AllowTags
		}
		if s.IgnoreTags != nil {
			merged.IgnoreTags = s.IgnoreTags
		}
		if s.PullSecret != nil {
			merged.PullSecret = s.PullSecret
		}
		if s.ForceUpdate != nil {
			merged.ForceUpdate = s.ForceUpdate
		}
		if s.Platforms != nil {
			merged.Platforms = s.Platforms
		}
	}
	return merged
}

// newImageFromCommonUpdateSettings creates a new Image from a final, merged set of settings.
func newImageFromCommonUpdateSettings(ctx context.Context, settings *iuapi.CommonUpdateSettings) *Image {
	// Start with defaults
	img := &Image{
		ContainerImage:     &image.ContainerImage{},
		UpdateStrategy:     image.StrategySemVer,
		ForceUpdate:        false,
		AllowTags:          "",
		PullSecret:         "",
		IgnoreTags:         []string{},
		Platforms:          []string{},
		HelmImageName:      "",
		HelmImageTag:       "",
		HelmImageSpec:      "",
		KustomizeImageName: "",
	}

	if settings == nil {
		return img
	}

	// Apply the final settings.
	if settings.UpdateStrategy != nil {
		img.UpdateStrategy = img.ParseUpdateStrategy(ctx, *settings.UpdateStrategy)
	}
	if settings.ForceUpdate != nil {
		img.ForceUpdate = *settings.ForceUpdate
	}
	if settings.AllowTags != nil {
		img.AllowTags = *settings.AllowTags
	}
	if settings.PullSecret != nil {
		img.PullSecret = *settings.PullSecret
	}
	if settings.IgnoreTags != nil {
		img.IgnoreTags = settings.IgnoreTags
	}
	if settings.Platforms != nil {
		img.Platforms = settings.Platforms
	}

	return img
}

// mergeWBCSettings merges global and app-specific WriteBackConfig settings.
// App-specific settings take precedence over global settings.
func mergeWBCSettings(global *iuapi.WriteBackConfig, appWBC *iuapi.WriteBackConfig) *iuapi.WriteBackConfig {
	if global == nil && appWBC == nil {
		return &iuapi.WriteBackConfig{}
	}

	// Start with a clone of global to prevent modification
	merged := &iuapi.WriteBackConfig{}
	if global != nil {
		merged = global.DeepCopy()
	}

	if appWBC == nil {
		return merged
	}

	if appWBC.Method != nil {
		merged.Method = appWBC.Method
	}

	if appWBC.GitConfig != nil {
		if merged.GitConfig == nil {
			merged.GitConfig = &iuapi.GitConfig{}
		}
		if appWBC.GitConfig.Repository != nil {
			merged.GitConfig.Repository = appWBC.GitConfig.Repository
		}
		if appWBC.GitConfig.Branch != nil {
			merged.GitConfig.Branch = appWBC.GitConfig.Branch
		}
		if appWBC.GitConfig.WriteBackTarget != nil {
			merged.GitConfig.WriteBackTarget = appWBC.GitConfig.WriteBackTarget
		}
	}
	return merged
}

// newWBCFromSettings creates a new WriteBackConfig from a given, final set of
// settings within the context of a specific application. It is responsible for
// resolving all app-dependent fields, like target paths.
func newWBCFromSettings(ctx context.Context, app *argocdapi.Application, kubeClient *kube.ImageUpdaterKubernetesClient, settings *iuapi.WriteBackConfig) (*WriteBackConfig, error) {
	wbc := &WriteBackConfig{
		Method:                 WriteBackApplication,
		ArgoClient:             nil,
		GitClient:              nil,
		GetCreds:               nil,
		GitBranch:              "",
		GitWriteBranch:         "",
		GitCommitUser:          "",
		GitCommitEmail:         "",
		GitCommitMessage:       "",
		GitCommitSigningKey:    "",
		GitCommitSigningMethod: "",
		GitCommitSignOff:       false,
		KustomizeBase:          "",
		Target:                 "", // Will be set by parseDefaultTarget
		GitRepo:                "",
		GitCreds:               nil,
	}

	appSource := getApplicationSource(ctx, app, nil)
	if appSource == nil {
		return nil, fmt.Errorf("application source is not defined for %s/%s", app.Namespace, app.Name)
	}

	// Set a default target. This will be used by the ArgoCD method, or by the Git method if no explicit target is given.
	wbc.Target = parseDefaultTarget(app.GetNamespace(), app.Name, appSource.Path, kubeClient)

	if settings == nil {
		return wbc, nil
	}

	// If no method is specified, or it's explicitly 'argocd', we are done.
	if settings.Method == nil || strings.TrimSpace(*settings.Method) == "argocd" {
		return wbc, nil
	}

	// Determine method and credentials from the method string
	method := strings.TrimSpace(*settings.Method)
	creds := "repocreds"
	if index := strings.Index(method, ":"); index > 0 {
		creds = method[index+1:]
		method = method[:index]
	}

	if method == "git" {
		wbc.Method = WriteBackGit
		// If an explicit write-back target is given, parse and apply it. Otherwise,
		// the default target set above will be used.
		if settings.GitConfig != nil && settings.GitConfig.WriteBackTarget != nil {
			target := *settings.GitConfig.WriteBackTarget
			if strings.HasPrefix(target, common.KustomizationPrefix) {
				wbc.KustomizeBase = parseKustomizeBase(target, appSource.Path)
			} else if strings.HasPrefix(target, common.HelmPrefix) {
				wbc.Target = parseTarget(target, appSource.Path)
			} else {
				wbc.Target = target
			}
		}
		// Parse all other git-related configurations
		if err := parseGitConfig(ctx, app, kubeClient, settings, wbc, creds); err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("invalid update mechanism: %s", *settings.Method)
	}

	return wbc, nil
}

// newImageFromManifestTargetSettings creates a new Image and populates it
// by layering the given Manifest target settings.
func newImageFromManifestTargetSettings(settings *iuapi.ManifestTarget, img *Image) (*Image, error) {
	if settings == nil {
		return img, nil
	}

	if settings.Helm != nil && settings.Kustomize != nil {
		return nil, fmt.Errorf("only one of the fields (Helm, Kustomize) should be set, dictating the update method")
	}

	// Layer the new settings on top, only if they are explicitly set (non-nil).
	if settings.Helm != nil && settings.Helm.Spec != nil {
		img.HelmImageSpec = *settings.Helm.Spec
	} else {
		if settings.Helm != nil && settings.Helm.Name != nil {
			img.HelmImageName = *settings.Helm.Name
		}
		if settings.Helm != nil && settings.Helm.Tag != nil {
			img.HelmImageTag = *settings.Helm.Tag
		}
	}
	if settings.Kustomize != nil && settings.Kustomize.Name != nil {
		img.KustomizeImageName = *settings.Kustomize.Name
	}

	return img, nil
}

// parseImageList parses a list of ImageConfig objects from the ImageUpdater CR
// into a ImageList, which is used internally for image management.
func parseImageList(ctx context.Context, images []iuapi.ImageConfig, appSettings *iuapi.CommonUpdateSettings, webhookEvent *WebhookEvent) *ImageList {
	log := log.LoggerFromContext(ctx)
	results := make(ImageList, 0)
	for _, im := range images {
		// For each image, calculate its final settings by layering its specific
		// settings on top of the application-level settings.
		finalCommonUpdateSettings := mergeCommonUpdateSettings(appSettings, im.CommonUpdateSettings)
		img := newImageFromCommonUpdateSettings(ctx, finalCommonUpdateSettings)

		img, err := newImageFromManifestTargetSettings(im.ManifestTarget, img)
		if err != nil {
			log.Warnf("Could not set manifest target config for image %s, skipping: %v", im.ImageName, err)
			continue
		}

		img.ContainerImage = image.NewFromIdentifier(im.Alias + "=" + im.ImageName)

		// Check if any of the images match the webhook event
		if webhookEvent != nil {
			log.Debugf("Checking webhook match for image `%s`: event=(%s/%s), image=(%s/%s)",
				im.Alias, webhookEvent.RegistryURL, webhookEvent.Repository,
				img.ContainerImage.RegistryURL, img.ContainerImage.ImageName)

			// Skip if registry doesn't match
			if img.ContainerImage.RegistryURL != "" && img.ContainerImage.RegistryURL != webhookEvent.RegistryURL {
				log.Debugf("Registry mismatch for image `%s`: %s != %s", im.Alias, img.ContainerImage.RegistryURL, webhookEvent.RegistryURL)
				continue
			}

			// Check if repository matches
			if img.ContainerImage.ImageName != webhookEvent.Repository {
				log.Debugf("Repository mismatch for image `%s`: %s != %s", im.Alias, img.ContainerImage.ImageName, webhookEvent.Repository)
				continue
			}

			log.Infof("Image `%s` matches webhook event=(%s/%s)", im.Alias, webhookEvent.RegistryURL, webhookEvent.Repository)
		}

		if im.ManifestTarget != nil && im.ManifestTarget.Kustomize != nil && im.ManifestTarget.Kustomize.Name != nil {
			if kustomizeImage := im.ManifestTarget.Kustomize.Name; *kustomizeImage != "" {
				img.ContainerImage.KustomizeImage = image.NewFromIdentifier(*kustomizeImage)
			}
		}
		results = append(results, img)
	}

	return &results
}

// getHelmParamNames inspects the given image for whether
// the Helm parameter names are being set and
// returns their values.
func getHelmParamNames(img *Image) (string, string) {
	// Return default values without symbolic name given
	if img == nil || img.ImageAlias == "" {
		return "image.name", "image.tag"
	}

	var helmParamName, helmParamVersion string

	// Image spec is a full-qualified specifier, if we have it, we return early
	if param := img.HelmImageSpec; param != "" {
		return strings.TrimSpace(param), ""
	}

	if param := img.HelmImageName; param != "" {
		helmParamName = param
	}

	if param := img.HelmImageTag; param != "" {
		helmParamVersion = param
	}

	return helmParamName, helmParamVersion
}

// Get a named helm parameter from a list of parameters
func getHelmParam(params []argocdapi.HelmParameter, name string) *argocdapi.HelmParameter {
	for _, param := range params {
		if param.Name == name {
			return &param
		}
	}
	return nil
}

// mergeHelmParams merges a list of Helm parameters specified by merge into the
// Helm parameters given as src.
func mergeHelmParams(src []argocdapi.HelmParameter, merge []argocdapi.HelmParameter) []argocdapi.HelmParameter {
	retParams := make([]argocdapi.HelmParameter, 0)
	merged := make(map[string]interface{})

	// first look for params that need replacement
	for _, srcParam := range src {
		found := false
		for _, mergeParam := range merge {
			if srcParam.Name == mergeParam.Name {
				retParams = append(retParams, mergeParam)
				merged[mergeParam.Name] = true
				found = true
				break
			}
		}
		if !found {
			retParams = append(retParams, srcParam)
		}
	}

	// then check which we still need in dest list and merge those, too
	for _, mergeParam := range merge {
		if _, ok := merged[mergeParam.Name]; !ok {
			retParams = append(retParams, mergeParam)
		}
	}

	return retParams
}

// GetHelmImage gets the image set in Application source matching new image
// or an empty string if match is not found
func GetHelmImage(ctx context.Context, app *argocdapi.Application, wbc *WriteBackConfig, applicationImage *Image) (string, error) {

	if appType := getApplicationType(app, wbc); appType != ApplicationTypeHelm {
		return "", fmt.Errorf("cannot set Helm params on non-Helm application")
	}

	var hpImageName, hpImageTag, hpImageSpec string

	hpImageSpec = applicationImage.HelmImageSpec
	hpImageName = applicationImage.HelmImageName
	hpImageTag = applicationImage.HelmImageTag

	if hpImageSpec == "" {
		if hpImageName == "" {
			hpImageName = common.DefaultHelmImageName
		}
		if hpImageTag == "" {
			hpImageTag = common.DefaultHelmImageTag
		}
	}

	appSource := getApplicationSource(ctx, app, wbc)

	if appSource.Helm == nil {
		return "", nil
	}

	if appSource.Helm.Parameters == nil {
		return "", nil
	}

	if hpImageSpec != "" {
		if p := getHelmParam(appSource.Helm.Parameters, hpImageSpec); p != nil {
			return p.Value, nil
		}
	} else {
		imageName := getHelmParam(appSource.Helm.Parameters, hpImageName)
		imageTag := getHelmParam(appSource.Helm.Parameters, hpImageTag)
		if imageName == nil || imageTag == nil {
			return "", nil
		}
		return imageName.Value + ":" + imageTag.Value, nil
	}

	return "", nil
}

// SetHelmImage sets image parameters for a Helm application
func SetHelmImage(ctx context.Context, app *argocdapi.Application, newImage *image.ContainerImage, wbc *WriteBackConfig, applicationImage *Image) error {
	log := log.LoggerFromContext(ctx)
	if appType := getApplicationType(app, wbc); appType != ApplicationTypeHelm {
		return fmt.Errorf("cannot set Helm params on non-Helm application")
	}

	var hpImageName, hpImageTag, hpImageSpec string

	hpImageSpec = applicationImage.HelmImageSpec
	hpImageName = applicationImage.HelmImageName
	hpImageTag = applicationImage.HelmImageTag

	if hpImageSpec == "" {
		if hpImageName == "" {
			hpImageName = common.DefaultHelmImageName
		}
		if hpImageTag == "" {
			hpImageTag = common.DefaultHelmImageTag
		}
	}

	log.Debugf("target parameters: image-spec=%s image-name=%s, image-tag=%s", hpImageSpec, hpImageName, hpImageTag)

	mergeParams := make([]argocdapi.HelmParameter, 0)

	// The logic behind this is that image-spec is an override - if this is set,
	// we simply ignore any image-name and image-tag parameters that might be
	// there.
	if hpImageSpec != "" {
		p := argocdapi.HelmParameter{Name: hpImageSpec, Value: newImage.GetFullNameWithTag(), ForceString: true}
		mergeParams = append(mergeParams, p)
	} else {
		mergeParams = append(mergeParams,
			argocdapi.HelmParameter{Name: hpImageName, Value: newImage.GetFullNameWithoutTag(), ForceString: true},
		)
		// Only set the tag parameter if we have a non-empty tag value.
		// When forceUpdate is enabled and no tag is specified, the tag can be empty.
		// Setting an empty tag would overwrite existing tag values and cause invalid image references.
		if tagValue := newImage.GetTagWithDigest(); tagValue != "" {
			mergeParams = append(mergeParams,
				argocdapi.HelmParameter{Name: hpImageTag, Value: tagValue, ForceString: true},
			)
		}
	}

	appSource := getApplicationSource(ctx, app, wbc)

	if appSource.Helm == nil {
		appSource.Helm = &argocdapi.ApplicationSourceHelm{}
	}

	if appSource.Helm.Parameters == nil {
		appSource.Helm.Parameters = make([]argocdapi.HelmParameter, 0)
	}

	appSource.Helm.Parameters = mergeHelmParams(appSource.Helm.Parameters, mergeParams)

	return nil
}

// GetKustomizeImage gets the image set in Application source matching new image
// or an empty string if match is not found
func GetKustomizeImage(ctx context.Context, app *argocdapi.Application, wbc *WriteBackConfig, applicationImage *Image) (string, error) {
	if appType := getApplicationType(app, wbc); appType != ApplicationTypeKustomize {
		return "", fmt.Errorf("cannot set Kustomize image on non-Kustomize application")
	}

	ksImageName := applicationImage.KustomizeImageName

	appSource := getApplicationSource(ctx, app, wbc)

	if appSource.Kustomize == nil {
		return "", nil
	}

	ksImages := appSource.Kustomize.Images

	if ksImages == nil {
		return "", nil
	}

	for _, a := range ksImages {
		if a.Match(argocdapi.KustomizeImage(ksImageName)) {
			return string(a), nil
		}
	}

	return "", nil
}

// SetKustomizeImage sets a Kustomize image for given application
func SetKustomizeImage(ctx context.Context, app *argocdapi.Application, newImage *image.ContainerImage, wbc *WriteBackConfig, applicationImage *Image) error {
	log := log.LoggerFromContext(ctx)
	if appType := getApplicationType(app, wbc); appType != ApplicationTypeKustomize {
		return fmt.Errorf("cannot set Kustomize image on non-Kustomize application")
	}

	var ksImageParam string
	ksImageName := applicationImage.KustomizeImageName
	if ksImageName != "" {
		ksImageParam = fmt.Sprintf("%s=%s", ksImageName, newImage.GetFullNameWithTag())
	} else {
		ksImageParam = newImage.GetFullNameWithTag()
	}

	log.Tracef("Setting Kustomize parameter %s", ksImageParam)

	appSource := getApplicationSource(ctx, app, wbc)

	if appSource.Kustomize == nil {
		appSource.Kustomize = &argocdapi.ApplicationSourceKustomize{}
	}

	for i, kImg := range appSource.Kustomize.Images {
		curr := image.NewFromIdentifier(string(kImg))
		override := image.NewFromIdentifier(ksImageParam)

		if curr.ImageName == override.ImageName {
			curr.ImageAlias = override.ImageAlias
			appSource.Kustomize.Images[i] = argocdapi.KustomizeImage(override.String())
		}

	}

	appSource.Kustomize.MergeImage(argocdapi.KustomizeImage(ksImageParam))

	return nil
}

// GetImagesFromApplication returns the list of known images for the given application
func GetImagesFromApplication(applicationImages *ApplicationImages) image.ContainerImageList {
	images := make(image.ContainerImageList, 0)
	app := applicationImages.Application

	// Get images deployed with the current ArgoCD app.
	for _, imageStr := range app.Status.Summary.Images {
		image := image.NewFromIdentifier(imageStr)
		images = append(images, image)
	}

	for _, img := range applicationImages.Images {
		if img.ForceUpdate {
			// Create a copy of the container image with nil tag to add to the live images list.
			// The tag from the image list will be a version constraint, which isn't a valid tag.
			// We preserve the original img.ContainerImage so the constraint is available later.
			imgCopy := img.ContainerImage.WithTag(nil)
			// Avoid duplicates if an entry already exists for this image (ignore tag for match).
			if images.ContainsImage(img.ContainerImage, false) == nil {
				images = append(images, imgCopy)
			}
		}
	}

	return images
}

// GetImagesAndAliasesFromApplication returns the list of known images for the given application
// TODO: this function together with GetImagesFromApplication should be refactored. We iterate through
// applicationImages.Images 3 times in one place (2 in functions and in containerImages.ContainsImage).
// Also the 4th loop is in marshalParamsOverride. See GITOPS-7415
func GetImagesAndAliasesFromApplication(applicationImages *ApplicationImages) ImageList {
	containerImages := GetImagesFromApplication(applicationImages)

	// We iterate through the list of images with alias information.
	for _, aliasedImage := range applicationImages.Images {
		// For each one, we find its corresponding entry in the list of images found in the app source.
		if sourceImage := containerImages.ContainsImage(aliasedImage.ContainerImage, false); sourceImage != nil {
			if sourceImage.ImageAlias != "" {
				// this image has already been matched to an alias, so create a copy
				// and assign this alias to the image copy to avoid overwriting the existing alias association
				imageCopy := *sourceImage
				if aliasedImage.ImageAlias == "" {
					imageCopy.ImageAlias = aliasedImage.ImageName
				} else {
					imageCopy.ImageAlias = aliasedImage.ImageAlias
				}
				// We update the aliasedImage to point to this new copy.
				aliasedImage.ContainerImage = &imageCopy
			} else {
				// This is the first alias for this image. We can modify it in place.
				if aliasedImage.ImageAlias == "" {
					sourceImage.ImageAlias = aliasedImage.ImageName
				} else {
					sourceImage.ImageAlias = aliasedImage.ImageAlias
				}
				// We update the aliasedImage to point to the now-aliased source image.
				aliasedImage.ContainerImage = sourceImage
			}
		}
	}

	// The applicationImages.Images list is now correctly aliased.
	return applicationImages.Images
}

// GetApplicationType returns the type of the ArgoCD application
func GetApplicationType(app *argocdapi.Application, wbc *WriteBackConfig) ApplicationType {
	return getApplicationType(app, wbc)
}

// GetApplicationSourceType returns the source type of the ArgoCD application
func GetApplicationSourceType(app *argocdapi.Application, wbc *WriteBackConfig) argocdapi.ApplicationSourceType {
	return getApplicationSourceType(app, wbc)
}

// GetApplicationSource returns the main source of a Helm or Kustomize type of the ArgoCD application
func GetApplicationSource(ctx context.Context, app *argocdapi.Application, wbc *WriteBackConfig) *argocdapi.ApplicationSource {
	return getApplicationSource(ctx, app, wbc)
}

// IsValidApplicationType returns true if we can update the application
func IsValidApplicationType(app *argocdapi.Application, wbc *WriteBackConfig) bool {
	return getApplicationType(app, wbc) != ApplicationTypeUnsupported
}

// getApplicationType returns the type of the application
func getApplicationType(app *argocdapi.Application, wbc *WriteBackConfig) ApplicationType {
	sourceType := getApplicationSourceType(app, wbc)

	if sourceType == argocdapi.ApplicationSourceTypeKustomize {
		return ApplicationTypeKustomize
	} else if sourceType == argocdapi.ApplicationSourceTypeHelm {
		return ApplicationTypeHelm
	} else {
		return ApplicationTypeUnsupported
	}
}

// getApplicationSourceType returns the source type of the application
func getApplicationSourceType(app *argocdapi.Application, wbc *WriteBackConfig) argocdapi.ApplicationSourceType {
	if wbc != nil {
		if wbc.KustomizeBase != "" {
			return argocdapi.ApplicationSourceTypeKustomize
		}
		// Check if the target is a helmvalues path (ends with .yaml or .yml)
		// but exclude the default override file format (.argocd-source-*.yaml)
		if wbc.Target != "" && (strings.HasSuffix(wbc.Target, ".yaml") || strings.HasSuffix(wbc.Target, ".yml")) {
			targetBase := filepath.Base(wbc.Target)
			if !strings.HasPrefix(targetBase, common.DefaultTargetFilePrefix) {
				return argocdapi.ApplicationSourceTypeHelm
			}
		}
	}
	if app.Spec.HasMultipleSources() {
		for _, st := range app.Status.SourceTypes {
			if st == argocdapi.ApplicationSourceTypeHelm {
				return argocdapi.ApplicationSourceTypeHelm
			} else if st == argocdapi.ApplicationSourceTypeKustomize {
				return argocdapi.ApplicationSourceTypeKustomize
			} else if st == argocdapi.ApplicationSourceTypePlugin {
				return argocdapi.ApplicationSourceTypePlugin
			}
		}
		return argocdapi.ApplicationSourceTypeDirectory
	}

	return app.Status.SourceType
}

// getApplicationSource returns the main source of a Helm or Kustomize type of the application
func getApplicationSource(ctx context.Context, app *argocdapi.Application, wbc *WriteBackConfig) *argocdapi.ApplicationSource {
	log := log.LoggerFromContext(ctx)
	if app.Spec.HasMultipleSources() {
		// Determine the source type from WriteBackConfig
		sourceType := getApplicationSourceType(app, wbc)

		for i := range app.Spec.Sources {
			s := &app.Spec.Sources[i]
			// If source type is Helm, look for Helm source
			if sourceType == argocdapi.ApplicationSourceTypeHelm && s.Helm != nil {
				return s
			}
			// If source type is Kustomize, look for Kustomize source
			if sourceType == argocdapi.ApplicationSourceTypeKustomize && s.Kustomize != nil {
				return s
			}
		}

		// Fallback: look for any Helm or Kustomize source
		for i := range app.Spec.Sources {
			s := &app.Spec.Sources[i]
			if s.Helm != nil || s.Kustomize != nil {
				return s
			}
		}

		log.Tracef("Could not get Source of type Helm or Kustomize from multisource configuration. Returning first source from the list")
		return &app.Spec.Sources[0]
	}

	return app.Spec.Source
}

// GetParameterPullSecret retrieves an image's pull secret credentials
func GetParameterPullSecret(ctx context.Context, img *Image) *image.CredentialSource {
	log := log.LoggerFromContext(ctx)

	var pullSecretVal = img.PullSecret
	if pullSecretVal == "" {
		log.Tracef("No pull secret configured for this image")
		return nil
	}
	credSrc, err := image.ParseCredentialSource(pullSecretVal, false)
	if err != nil {
		log.Warnf("Invalid credential reference specified: %s", pullSecretVal)
		return nil
	}
	return credSrc
}
