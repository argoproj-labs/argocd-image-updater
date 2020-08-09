package cache

import (
	"github.com/argoproj-labs/argocd-image-updater/pkg/tag"
)

type ImageTagCache interface {
	HasTag(imageName string, imageTag string) bool
	GetTag(imageName string, imageTag string) (*tag.ImageTag, error)
	SetTag(imageName string, imgTag *tag.ImageTag)
}
