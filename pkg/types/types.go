package types

import (
	"context"

	argocdapi "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/image"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
)

// ApplicationImages holds an Argo CD application and a list of its images
// that are allowed to be considered for updates.
type ApplicationImages struct {
	Application argocdapi.Application
	Images      ImageList
}

// Image represents a container image and its update configuration.
// It embeds the neutral ContainerImage type and adds updater-specific
// configuration.
type Image struct {
	*image.ContainerImage

	// Update settings
	UpdateStrategy image.UpdateStrategy
	ForceUpdate    bool
	AllowTags      string
	IgnoreTags     []string
	PullSecret     string
	Platforms      []string
}

// ImageList is a list of Image objects that can be updated.
type ImageList []*Image

// NewImage creates a new Image object from a neutral ContainerImage
func NewImage(ci *image.ContainerImage) *Image {
	return &Image{
		ContainerImage: ci,
	}
}

// Clone creates a deep copy of the Image object.
func (i *Image) Clone() *Image {
	if i == nil {
		return nil
	}
	clone := &Image{
		ContainerImage: i.ContainerImage.Clone(),
		UpdateStrategy: i.UpdateStrategy,
		ForceUpdate:    i.ForceUpdate,
		AllowTags:      i.AllowTags,
		PullSecret:     i.PullSecret,
	}

	if i.IgnoreTags != nil {
		clone.IgnoreTags = make([]string, len(i.IgnoreTags))
		copy(clone.IgnoreTags, i.IgnoreTags)
	}

	if i.Platforms != nil {
		clone.Platforms = make([]string, len(i.Platforms))
		copy(clone.Platforms, i.Platforms)
	}

	return clone
}

// GetParameterPullSecret retrieves an image's pull secret credentials
func (i *Image) GetParameterPullSecret(ctx context.Context) *image.CredentialSource {
	log := log.LoggerFromContext(ctx)

	var pullSecretVal = i.PullSecret
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

// ToContainerImageList is a private helper that converts an ImageList to a
// neutral image.ContainerImageList. This allows us to reuse methods defined
// on ContainerImageList without duplicating code.
func (list ImageList) ToContainerImageList() image.ContainerImageList {
	cil := make(image.ContainerImageList, len(list))
	for i, img := range list {
		cil[i] = img.ContainerImage
	}
	return cil
}
