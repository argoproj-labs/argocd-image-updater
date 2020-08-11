package argocd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/argoproj-labs/argocd-image-updater/pkg/common"
	"github.com/argoproj-labs/argocd-image-updater/pkg/image"
	"github.com/argoproj-labs/argocd-image-updater/pkg/log"

	argocdclient "github.com/argoproj/argo-cd/pkg/apiclient"
	"github.com/argoproj/argo-cd/pkg/apiclient/application"
	"github.com/argoproj/argo-cd/pkg/apis/application/v1alpha1"
)

type ArgoCD struct {
	Client argocdclient.Client
}

// Interface that we need mocks for
type ArgoCDClient interface {
	ListApplications() ([]v1alpha1.Application, error)
	SetHelmImage(appName string, newImage *image.ContainerImage) error
	GetImagesFromApplication(appName string) (image.ContainerImageList, error)
	GetApplicationTypeByName(appName string) (ApplicationType, error)
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

// NewClient creates a new API client for ArgoCD and connects to the ArgoCD
// API server.
func NewClient(opts *ClientOptions) (*ArgoCD, error) {

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
	return &ArgoCD{Client: client}, nil
}

type ApplicationImages struct {
	Application v1alpha1.Application
	Images      image.ContainerImageList
}

// Will hold a list of applications with the images allowed to considered for
// update.
type ImageList map[string]ApplicationImages

// Retrieve a list of applications from ArgoCD that qualify for image updates
// Application needs either to be of type Kustomize or Helm and must have the
// correct annotation in order to be considered.
func FilterApplicationsForUpdate(apps []v1alpha1.Application) (map[string]ApplicationImages, error) {
	var appsForUpdate = make(map[string]ApplicationImages)

	for _, app := range apps {
		if !IsValidApplicationType(&app) {
			log.Tracef("skipping app '%s' of type '%s' because it's not of supported source type", app.GetName(), app.Status.SourceType)
			continue
		}
		annotations := app.GetAnnotations()
		if updateImage, ok := annotations[common.ImageUpdaterAnnotation]; !ok {
			log.Tracef("skipping app '%s' of type '%s' because required annotation is missing", app.GetName(), app.Status.SourceType)
			continue
		} else {
			log.Tracef("processing app '%s' of type '%v'", app.GetName(), app.Status.SourceType)
			imageList := make(image.ContainerImageList, 0)
			for _, imageName := range strings.Split(updateImage, ",") {
				allowed := image.NewFromIdentifier(strings.TrimSpace(imageName))
				imageList = append(imageList, allowed)
			}
			appImages := ApplicationImages{}
			appImages.Application = app
			appImages.Images = imageList
			appsForUpdate[app.GetName()] = appImages
		}
	}

	return appsForUpdate, nil
}

// ListApplications returns a list of all application names that the API user
// has access to.
func (client *ArgoCD) ListApplications() ([]v1alpha1.Application, error) {
	conn, appClient, err := client.Client.NewApplicationClient()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	apps, err := appClient.List(context.TODO(), &application.ApplicationQuery{})
	if err != nil {
		return nil, err
	}

	return apps.Items, nil
}

// getHelmParamNamesFromAnnotation inspects the given annotations for whether
// the annotations for specifying Helm parameter names are being set and
// returns their values.
func getHelmParamNamesFromAnnotation(annotations map[string]string, symbolicName string) (string, string) {
	// Return default values without symbolic name given
	if symbolicName == "" {
		return "image.name", "image.tag"
	}

	var annotationName, helmParamName, helmParamVersion string

	// Image spec is a full-qualified specifier, if we have it, we return early
	annotationName = fmt.Sprintf(common.HelmParamImageSpecAnnotation, symbolicName)
	if param, ok := annotations[annotationName]; ok {
		log.Tracef("found annotation %s", annotationName)
		return strings.TrimSpace(param), ""
	}

	annotationName = fmt.Sprintf(common.HelmParamImageNameAnnotation, symbolicName)
	if param, ok := annotations[annotationName]; ok {
		log.Tracef("found annotation %s", annotationName)
		helmParamName = param
	}

	annotationName = fmt.Sprintf(common.HelmParamImageTagAnnotation, symbolicName)
	if param, ok := annotations[annotationName]; ok {
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
func (client *ArgoCD) SetHelmImage(app *v1alpha1.Application, newImage *image.ContainerImage) error {
	conn, appClient, err := client.Client.NewApplicationClient()
	if err != nil {
		return err
	}
	defer conn.Close()

	if appType := getApplicationType(app); appType != ApplicationTypeHelm {
		return fmt.Errorf("cannot set Helm params on non-Helm application")
	}

	appName := app.GetName()

	var hpImageName, hpImageTag, hpImageSpec string

	hpImageSpec = newImage.GetParameterHelmImageSpec(app.Annotations)
	hpImageName = newImage.GetParameterHelmImageName(app.Annotations)
	hpImageTag = newImage.GetParameterHelmImageTag(app.Annotations)

	log.WithContext().
		AddField("application", appName).
		AddField("image", newImage.GetFullNameWithoutTag()).
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
			p := v1alpha1.HelmParameter{Name: hpImageTag, Value: newImage.ImageTag.TagName, ForceString: true}
			mergeParams = append(mergeParams, p)
		}
	}

	if app.Spec.Source.Helm == nil {
		app.Spec.Source.Helm = &v1alpha1.ApplicationSourceHelm{}
	}

	if app.Spec.Source.Helm.Parameters == nil {
		app.Spec.Source.Helm.Parameters = make([]v1alpha1.HelmParameter, 0)
	}

	app.Spec.Source.Helm.Parameters = mergeHelmParams(app.Spec.Source.Helm.Parameters, mergeParams)

	_, err = appClient.UpdateSpec(context.TODO(), &application.ApplicationUpdateSpecRequest{Name: &appName, Spec: app.Spec})
	if err != nil {
		return err
	}

	return nil
}

// SetKustomizeImage sets a Kustomize image for given application
func (client *ArgoCD) SetKustomizeImage(app *v1alpha1.Application, newImage *image.ContainerImage) error {
	conn, appClient, err := client.Client.NewApplicationClient()
	if err != nil {
		return err
	}
	defer conn.Close()

	appName := app.GetName()
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

	log.Tracef("Setting Kustomize parameter %s", ksImageParam)

	if app.Spec.Source.Kustomize == nil {
		app.Spec.Source.Kustomize = &v1alpha1.ApplicationSourceKustomize{}
	}

	app.Spec.Source.Kustomize.MergeImage(v1alpha1.KustomizeImage(ksImageParam))

	_, err = appClient.UpdateSpec(context.TODO(), &application.ApplicationUpdateSpecRequest{Name: &appName, Spec: app.Spec})
	if err != nil {
		return err
	}

	return nil
}

// GetImagesFromApplication returns the list of known images for the given application
func GetImagesFromApplication(app *v1alpha1.Application) image.ContainerImageList {
	images := make(image.ContainerImageList, 0)

	for _, imageStr := range app.Status.Summary.Images {
		image := image.NewFromIdentifier(imageStr)
		images = append(images, image)
	}

	return images
}

// GetApplicationTypeByName first retrieves application with given appName and
// returns its application type
func (client *ArgoCD) GetApplicationTypeByName(appName string) (ApplicationType, error) {
	conn, appClient, err := client.Client.NewApplicationClient()
	if err != nil {
		return ApplicationTypeUnsupported, err
	}
	defer conn.Close()

	app, err := appClient.Get(context.TODO(), &application.ApplicationQuery{Name: &appName})
	if err != nil {
		return ApplicationTypeUnsupported, err
	}
	return getApplicationType(app), nil
}

// GetApplicationType returns the type of the ArgoCD application
func GetApplicationType(app *v1alpha1.Application) ApplicationType {
	return getApplicationType(app)
}

// IsValidApplicationType returns true if we can update the application
func IsValidApplicationType(app *v1alpha1.Application) bool {
	return getApplicationType(app) != ApplicationTypeUnsupported
}

// getApplicationType returns the type of the application
func getApplicationType(app *v1alpha1.Application) ApplicationType {
	if app.Status.SourceType == v1alpha1.ApplicationSourceTypeKustomize {
		return ApplicationTypeKustomize
	} else if app.Status.SourceType == v1alpha1.ApplicationSourceTypeHelm {
		return ApplicationTypeHelm
	} else {
		return ApplicationTypeUnsupported
	}
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
