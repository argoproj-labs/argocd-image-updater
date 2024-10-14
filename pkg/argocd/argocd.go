package argocd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/argoproj-labs/argocd-image-updater/pkg/common"
	"github.com/argoproj-labs/argocd-image-updater/pkg/env"
	"github.com/argoproj-labs/argocd-image-updater/pkg/image"
	"github.com/argoproj-labs/argocd-image-updater/pkg/kube"
	"github.com/argoproj-labs/argocd-image-updater/pkg/log"
	"github.com/argoproj-labs/argocd-image-updater/pkg/metrics"

	argocdclient "github.com/argoproj/argo-cd/v2/pkg/apiclient"
	"github.com/argoproj/argo-cd/v2/pkg/apiclient/application"
	"github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Kubernetes based client
type k8sClient struct {
	kubeClient *kube.KubernetesClient
}

// GetApplication retrieves an application by name across all namespaces.
func (client *k8sClient) GetApplication(ctx context.Context, appName string) (*v1alpha1.Application, error) {
	log.Debugf("Getting application %s across all namespaces", appName)

	// List all applications across all namespaces (using empty labelSelector)
	appList, err := client.ListApplications(v1.NamespaceAll)
	if err != nil {
		return nil, fmt.Errorf("error listing applications: %w", err)
	}

	// Filter applications by name using nameMatchesPattern
	app, err := findApplicationByName(appList, appName)
	if err != nil {
		log.Errorf("error getting application: %v", err)
		return nil, fmt.Errorf("error getting application: %w", err)
	}

	// Retrieve the application in the specified namespace
	return app, nil
}

// ListApplications lists all applications across all namespaces.
func (client *k8sClient) ListApplications(labelSelector string) ([]v1alpha1.Application, error) {
	list, err := client.kubeClient.ApplicationsClientset.ArgoprojV1alpha1().Applications(v1.NamespaceAll).List(context.TODO(), v1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, fmt.Errorf("error listing applications: %w", err)
	}
	log.Debugf("Applications listed: %d", len(list.Items))
	return list.Items, nil
}

// findApplicationByName filters the list of applications by name using nameMatchesPattern.
func findApplicationByName(appList []v1alpha1.Application, appName string) (*v1alpha1.Application, error) {
	var matchedApps []*v1alpha1.Application

	for _, app := range appList {
		log.Debugf("Found application: %s in namespace %s", app.Name, app.Namespace)
		if nameMatchesPattern(app.Name, []string{appName}) {
			log.Debugf("Application %s matches the pattern", app.Name)
			matchedApps = append(matchedApps, &app)
		}
	}

	if len(matchedApps) == 0 {
		return nil, fmt.Errorf("application %s not found", appName)
	}

	if len(matchedApps) > 1 {
		return nil, fmt.Errorf("multiple applications found matching %s", appName)
	}

	return matchedApps[0], nil
}

func (client *k8sClient) UpdateSpec(ctx context.Context, spec *application.ApplicationUpdateSpecRequest) (*v1alpha1.ApplicationSpec, error) {
	const defaultMaxRetries = 7
	const baseDelay = 100 * time.Millisecond // Initial delay before retrying

	// Allow overriding max retries for testing purposes
	maxRetries := env.ParseNumFromEnv("OVERRIDE_MAX_RETRIES", defaultMaxRetries, 0, 100)

	for attempts := 0; attempts < maxRetries; attempts++ {
		app, err := client.GetApplication(ctx, spec.GetName())
		if err != nil {
			log.Errorf("could not get application: %s, error: %v", spec.GetName(), err)
			return nil, fmt.Errorf("error getting application: %w", err)
		}
		app.Spec = *spec.Spec

		updatedApp, err := client.kubeClient.ApplicationsClientset.ArgoprojV1alpha1().Applications(app.Namespace).Update(ctx, app, v1.UpdateOptions{})
		if err != nil {
			if errors.IsConflict(err) {
				log.Warnf("conflict occurred while updating application: %s, retrying... (%d/%d)", spec.GetName(), attempts+1, maxRetries)
				time.Sleep(baseDelay * (1 << attempts)) // Exponential backoff, multiply baseDelay by 2^attempts
				continue
			}
			log.Errorf("could not update application: %s, error: %v", spec.GetName(), err)
			return nil, fmt.Errorf("error updating application: %w", err)
		}
		return &updatedApp.Spec, nil
	}
	return nil, fmt.Errorf("max retries(%d) reached while updating application: %s", maxRetries, spec.GetName())
}

// NewK8SClient creates a new kubernetes client to interact with kubernetes api-server.
func NewK8SClient(kubeClient *kube.KubernetesClient) (ArgoCD, error) {
	return &k8sClient{kubeClient: kubeClient}, nil
}

// Native
type argoCD struct {
	Client argocdclient.Client
}

// ArgoCD is the interface for accessing Argo CD functions we need
type ArgoCD interface {
	GetApplication(ctx context.Context, appName string) (*v1alpha1.Application, error)
	ListApplications(labelSelector string) ([]v1alpha1.Application, error)
	UpdateSpec(ctx context.Context, spec *application.ApplicationUpdateSpecRequest) (*v1alpha1.ApplicationSpec, error)
}

// Type of the application
type ApplicationType int

const (
	ApplicationTypeUnsupported ApplicationType = 0
	ApplicationTypeHelm        ApplicationType = 1
	ApplicationTypeKustomize   ApplicationType = 2
)

// Basic wrapper struct for ArgoCD client options
type ClientOptions struct {
	ServerAddr      string
	Insecure        bool
	Plaintext       bool
	Certfile        string
	GRPCWeb         bool
	GRPCWebRootPath string
	AuthToken       string
}

// NewAPIClient creates a new API client for ArgoCD and connects to the ArgoCD
// API server.
func NewAPIClient(opts *ClientOptions) (ArgoCD, error) {

	envAuthToken := os.Getenv("ARGOCD_TOKEN")
	if envAuthToken != "" && opts.AuthToken == "" {
		opts.AuthToken = envAuthToken
	}

	rOpts := argocdclient.ClientOptions{
		ServerAddr:      opts.ServerAddr,
		PlainText:       opts.Plaintext,
		Insecure:        opts.Insecure,
		CertFile:        opts.Certfile,
		GRPCWeb:         opts.GRPCWeb,
		GRPCWebRootPath: opts.GRPCWebRootPath,
		AuthToken:       opts.AuthToken,
	}
	client, err := argocdclient.NewClient(&rOpts)
	if err != nil {
		return nil, err
	}
	return &argoCD{Client: client}, nil
}

type ApplicationImages struct {
	Application v1alpha1.Application
	Images      image.ContainerImageList
}

// Will hold a list of applications with the images allowed to considered for
// update.
type ImageList map[string]ApplicationImages

// Match a name against a list of patterns
func nameMatchesPattern(name string, patterns []string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, p := range patterns {
		log.Tracef("Matching application name %s against pattern %s", name, p)
		if m, err := filepath.Match(p, name); err != nil {
			log.Warnf("Invalid application name pattern '%s': %v", p, err)
		} else if m {
			return true
		}
	}
	return false
}

// Retrieve a list of applications from ArgoCD that qualify for image updates
// Application needs either to be of type Kustomize or Helm and must have the
// correct annotation in order to be considered.
func FilterApplicationsForUpdate(apps []v1alpha1.Application, patterns []string) (map[string]ApplicationImages, error) {
	var appsForUpdate = make(map[string]ApplicationImages)

	for _, app := range apps {
		logCtx := log.WithContext().AddField("application", app.GetName()).AddField("namespace", app.GetNamespace())
		appNSName := fmt.Sprintf("%s/%s", app.GetNamespace(), app.GetName())
		sourceType := getApplicationSourceType(&app)

		// Check whether application has our annotation set
		annotations := app.GetAnnotations()
		if _, ok := annotations[common.ImageUpdaterAnnotation]; !ok {
			logCtx.Tracef("skipping app '%s' of type '%s' because required annotation is missing", appNSName, sourceType)
			continue
		}

		// Check for valid application type
		if !IsValidApplicationType(&app) {
			logCtx.Warnf("skipping app '%s' of type '%s' because it's not of supported source type", appNSName, sourceType)
			continue
		}

		// Check if application name matches requested patterns
		if !nameMatchesPattern(app.GetName(), patterns) {
			logCtx.Debugf("Skipping app '%s' because it does not match requested patterns", appNSName)
			continue
		}

		logCtx.Tracef("processing app '%s' of type '%v'", appNSName, sourceType)
		imageList := parseImageList(annotations)
		appImages := ApplicationImages{}
		appImages.Application = app
		appImages.Images = *imageList
		appsForUpdate[appNSName] = appImages
	}

	return appsForUpdate, nil
}

func parseImageList(annotations map[string]string) *image.ContainerImageList {
	results := make(image.ContainerImageList, 0)
	if updateImage, ok := annotations[common.ImageUpdaterAnnotation]; ok {
		splits := strings.Split(updateImage, ",")
		for _, s := range splits {
			img := image.NewFromIdentifier(strings.TrimSpace(s))
			if kustomizeImage := img.GetParameterKustomizeImageName(annotations); kustomizeImage != "" {
				img.KustomizeImage = image.NewFromIdentifier(kustomizeImage)
			}
			results = append(results, img)
		}
	}
	return &results
}

// GetApplication gets the application named appName from Argo CD API
func (client *argoCD) GetApplication(ctx context.Context, appName string) (*v1alpha1.Application, error) {
	conn, appClient, err := client.Client.NewApplicationClient()
	metrics.Clients().IncreaseArgoCDClientRequest(client.Client.ClientOptions().ServerAddr, 1)
	if err != nil {
		metrics.Clients().IncreaseArgoCDClientError(client.Client.ClientOptions().ServerAddr, 1)
		return nil, err
	}
	defer conn.Close()

	metrics.Clients().IncreaseArgoCDClientRequest(client.Client.ClientOptions().ServerAddr, 1)
	app, err := appClient.Get(ctx, &application.ApplicationQuery{Name: &appName})
	if err != nil {
		metrics.Clients().IncreaseArgoCDClientError(client.Client.ClientOptions().ServerAddr, 1)
		return nil, err
	}

	return app, nil
}

// ListApplications returns a list of all application names that the API user
// has access to.
func (client *argoCD) ListApplications(labelSelector string) ([]v1alpha1.Application, error) {
	conn, appClient, err := client.Client.NewApplicationClient()
	metrics.Clients().IncreaseArgoCDClientRequest(client.Client.ClientOptions().ServerAddr, 1)
	if err != nil {
		metrics.Clients().IncreaseArgoCDClientError(client.Client.ClientOptions().ServerAddr, 1)
		return nil, err
	}
	defer conn.Close()

	metrics.Clients().IncreaseArgoCDClientRequest(client.Client.ClientOptions().ServerAddr, 1)
	apps, err := appClient.List(context.TODO(), &application.ApplicationQuery{Selector: &labelSelector})
	if err != nil {
		metrics.Clients().IncreaseArgoCDClientError(client.Client.ClientOptions().ServerAddr, 1)
		return nil, err
	}

	return apps.Items, nil
}

// UpdateSpec updates the spec for given application
func (client *argoCD) UpdateSpec(ctx context.Context, in *application.ApplicationUpdateSpecRequest) (*v1alpha1.ApplicationSpec, error) {
	conn, appClient, err := client.Client.NewApplicationClient()
	metrics.Clients().IncreaseArgoCDClientRequest(client.Client.ClientOptions().ServerAddr, 1)
	if err != nil {
		metrics.Clients().IncreaseArgoCDClientError(client.Client.ClientOptions().ServerAddr, 1)
		return nil, err
	}
	defer conn.Close()

	metrics.Clients().IncreaseArgoCDClientRequest(client.Client.ClientOptions().ServerAddr, 1)
	spec, err := appClient.UpdateSpec(ctx, in)
	if err != nil {
		metrics.Clients().IncreaseArgoCDClientError(client.Client.ClientOptions().ServerAddr, 1)
		return nil, err
	}

	return spec, nil
}

// getHelmParamNamesFromAnnotation inspects the given annotations for whether
// the annotations for specifying Helm parameter names are being set and
// returns their values.
func getHelmParamNamesFromAnnotation(annotations map[string]string, img *image.ContainerImage) (string, string) {
	// Return default values without symbolic name given
	if img.ImageAlias == "" {
		return "image.name", "image.tag"
	}

	var annotationName, helmParamName, helmParamVersion string

	// Image spec is a full-qualified specifier, if we have it, we return early
	if param := img.GetParameterHelmImageSpec(annotations); param != "" {
		log.Tracef("found annotation %s", annotationName)
		return strings.TrimSpace(param), ""
	}

	if param := img.GetParameterHelmImageName(annotations); param != "" {
		log.Tracef("found annotation %s", annotationName)
		helmParamName = param
	}

	if param := img.GetParameterHelmImageTag(annotations); param != "" {
		log.Tracef("found annotation %s", annotationName)
		helmParamVersion = param
	}

	return helmParamName, helmParamVersion
}

// Get a named helm parameter from a list of parameters
func getHelmParam(params []v1alpha1.HelmParameter, name string) *v1alpha1.HelmParameter {
	for _, param := range params {
		if param.Name == name {
			return &param
		}
	}
	return nil
}

// mergeHelmParams merges a list of Helm parameters specified by merge into the
// Helm parameters given as src.
func mergeHelmParams(src []v1alpha1.HelmParameter, merge []v1alpha1.HelmParameter) []v1alpha1.HelmParameter {
	retParams := make([]v1alpha1.HelmParameter, 0)
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

// SetHelmImage sets image parameters for a Helm application
func SetHelmImage(app *v1alpha1.Application, newImage *image.ContainerImage) error {
	if appType := getApplicationType(app); appType != ApplicationTypeHelm {
		return fmt.Errorf("cannot set Helm params on non-Helm application")
	}

	appName := app.GetName()
	appNamespace := app.GetNamespace()

	var hpImageName, hpImageTag, hpImageSpec string

	hpImageSpec = newImage.GetParameterHelmImageSpec(app.Annotations)
	hpImageName = newImage.GetParameterHelmImageName(app.Annotations)
	hpImageTag = newImage.GetParameterHelmImageTag(app.Annotations)

	if hpImageSpec == "" {
		if hpImageName == "" {
			hpImageName = common.DefaultHelmImageName
		}
		if hpImageTag == "" {
			hpImageTag = common.DefaultHelmImageTag
		}
	}

	log.WithContext().
		AddField("application", appName).
		AddField("image", newImage.GetFullNameWithoutTag()).
		AddField("namespace", appNamespace).
		Debugf("target parameters: image-spec=%s image-name=%s, image-tag=%s", hpImageSpec, hpImageName, hpImageTag)

	mergeParams := make([]v1alpha1.HelmParameter, 0)

	// The logic behind this is that image-spec is an override - if this is set,
	// we simply ignore any image-name and image-tag parameters that might be
	// there.
	if hpImageSpec != "" {
		p := v1alpha1.HelmParameter{Name: hpImageSpec, Value: newImage.GetFullNameWithTag(), ForceString: true}
		mergeParams = append(mergeParams, p)
	} else {
		if hpImageName != "" {
			p := v1alpha1.HelmParameter{Name: hpImageName, Value: newImage.GetFullNameWithoutTag(), ForceString: true}
			mergeParams = append(mergeParams, p)
		}
		if hpImageTag != "" {
			p := v1alpha1.HelmParameter{Name: hpImageTag, Value: newImage.GetTagWithDigest(), ForceString: true}
			mergeParams = append(mergeParams, p)
		}
	}

	appSource := getApplicationSource(app)

	if appSource.Helm == nil {
		appSource.Helm = &v1alpha1.ApplicationSourceHelm{}
	}

	if appSource.Helm.Parameters == nil {
		appSource.Helm.Parameters = make([]v1alpha1.HelmParameter, 0)
	}

	appSource.Helm.Parameters = mergeHelmParams(appSource.Helm.Parameters, mergeParams)

	return nil
}

// SetKustomizeImage sets a Kustomize image for given application
func SetKustomizeImage(app *v1alpha1.Application, newImage *image.ContainerImage) error {
	if appType := getApplicationType(app); appType != ApplicationTypeKustomize {
		return fmt.Errorf("cannot set Kustomize image on non-Kustomize application")
	}

	var ksImageParam string
	ksImageName := newImage.GetParameterKustomizeImageName(app.Annotations)
	if ksImageName != "" {
		ksImageParam = fmt.Sprintf("%s=%s", ksImageName, newImage.GetFullNameWithTag())
	} else {
		ksImageParam = newImage.GetFullNameWithTag()
	}

	log.WithContext().AddField("application", app.GetName()).Tracef("Setting Kustomize parameter %s", ksImageParam)

	appSource := getApplicationSource(app)

	if appSource.Kustomize == nil {
		appSource.Kustomize = &v1alpha1.ApplicationSourceKustomize{}
	}

	for i, kImg := range appSource.Kustomize.Images {
		curr := image.NewFromIdentifier(string(kImg))
		override := image.NewFromIdentifier(ksImageParam)

		if curr.ImageName == override.ImageName {
			curr.ImageAlias = override.ImageAlias
			appSource.Kustomize.Images[i] = v1alpha1.KustomizeImage(override.String())
		}

	}

	appSource.Kustomize.MergeImage(v1alpha1.KustomizeImage(ksImageParam))

	return nil
}

// GetImagesFromApplication returns the list of known images for the given application
func GetImagesFromApplication(app *v1alpha1.Application) image.ContainerImageList {
	images := make(image.ContainerImageList, 0)

	for _, imageStr := range app.Status.Summary.Images {
		image := image.NewFromIdentifier(imageStr)
		images = append(images, image)
	}

	// The Application may wish to update images that don't create a container we can detect.
	// Check the image list for images with a force-update annotation, and add them if they are not already present.
	annotations := app.Annotations
	for _, img := range *parseImageList(annotations) {
		if img.HasForceUpdateOptionAnnotation(annotations) {
			img.ImageTag = nil // the tag from the image list will be a version constraint, which isn't a valid tag
			images = append(images, img)
		}
	}

	return images
}

// GetImagesFromApplicationImagesAnnotation returns the list of known images for the given application from the images annotation
func GetImagesAndAliasesFromApplication(app *v1alpha1.Application) image.ContainerImageList {
	images := GetImagesFromApplication(app)

	// We update the ImageAlias field of the Images found in the app.Status.Summary.Images list.
	for _, img := range *parseImageList(app.Annotations) {
		if image := images.ContainsImage(img, false); image != nil {
			if image.ImageAlias != "" {
				// this image has already been matched to an alias, so create a copy
				// and assign this alias to the image copy to avoid overwriting the existing alias association
				imageCopy := *image
				if img.ImageAlias == "" {
					imageCopy.ImageAlias = img.ImageName
				} else {
					imageCopy.ImageAlias = img.ImageAlias
				}
				images = append(images, &imageCopy)
			} else {
				if img.ImageAlias == "" {
					image.ImageAlias = img.ImageName
				} else {
					image.ImageAlias = img.ImageAlias
				}
			}
		}
	}

	return images
}

// GetApplicationTypeByName first retrieves application with given appName and
// returns its application type
func GetApplicationTypeByName(client ArgoCD, appName string) (ApplicationType, error) {
	app, err := client.GetApplication(context.TODO(), appName)
	if err != nil {
		return ApplicationTypeUnsupported, err
	}
	return getApplicationType(app), nil
}

// GetApplicationType returns the type of the ArgoCD application
func GetApplicationType(app *v1alpha1.Application) ApplicationType {
	return getApplicationType(app)
}

// GetApplicationSourceType returns the source type of the ArgoCD application
func GetApplicationSourceType(app *v1alpha1.Application) v1alpha1.ApplicationSourceType {
	return getApplicationSourceType(app)
}

// GetApplicationSource returns the main source of a Helm or Kustomize type of the ArgoCD application
func GetApplicationSource(app *v1alpha1.Application) *v1alpha1.ApplicationSource {
	return getApplicationSource(app)
}

// IsValidApplicationType returns true if we can update the application
func IsValidApplicationType(app *v1alpha1.Application) bool {
	return getApplicationType(app) != ApplicationTypeUnsupported
}

// getApplicationType returns the type of the application
func getApplicationType(app *v1alpha1.Application) ApplicationType {
	sourceType := getApplicationSourceType(app)

	if sourceType == v1alpha1.ApplicationSourceTypeKustomize {
		return ApplicationTypeKustomize
	} else if sourceType == v1alpha1.ApplicationSourceTypeHelm {
		return ApplicationTypeHelm
	} else {
		return ApplicationTypeUnsupported
	}
}

// getApplicationSourceType returns the source type of the application
func getApplicationSourceType(app *v1alpha1.Application) v1alpha1.ApplicationSourceType {

	if st, set := app.Annotations[common.WriteBackTargetAnnotation]; set &&
		strings.HasPrefix(st, common.KustomizationPrefix) {
		return v1alpha1.ApplicationSourceTypeKustomize
	}

	if app.Spec.HasMultipleSources() {
		for _, st := range app.Status.SourceTypes {
			if st == v1alpha1.ApplicationSourceTypeHelm {
				return v1alpha1.ApplicationSourceTypeHelm
			} else if st == v1alpha1.ApplicationSourceTypeKustomize {
				return v1alpha1.ApplicationSourceTypeKustomize
			} else if st == v1alpha1.ApplicationSourceTypePlugin {
				return v1alpha1.ApplicationSourceTypePlugin
			}
		}
		return v1alpha1.ApplicationSourceTypeDirectory
	}

	return app.Status.SourceType
}

// getApplicationSource returns the main source of a Helm or Kustomize type of the application
func getApplicationSource(app *v1alpha1.Application) *v1alpha1.ApplicationSource {

	if app.Spec.HasMultipleSources() {
		for _, s := range app.Spec.Sources {
			if s.Helm != nil || s.Kustomize != nil {
				return &s
			}
		}

		log.WithContext().AddField("application", app.GetName()).Tracef("Could not get Source of type Helm or Kustomize from multisource configuration. Returning first source from the list")
		return &app.Spec.Sources[0]
	}

	return app.Spec.Source
}

// String returns a string representation of the application type
func (a ApplicationType) String() string {
	switch a {
	case ApplicationTypeKustomize:
		return "Kustomize"
	case ApplicationTypeHelm:
		return "Helm"
	case ApplicationTypeUnsupported:
		return "Unsupported"
	default:
		return "Unknown"
	}
}
