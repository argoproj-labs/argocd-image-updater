package types

import (
	argocdapi "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/image"
)

// ApplicationImages holds an Argo CD application and a list of its images
// that are allowed to be considered for updates.
type ApplicationImages struct {
	Application argocdapi.Application
	Images      image.ContainerImageList
}
