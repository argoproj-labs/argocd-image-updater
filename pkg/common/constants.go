package common

// This file contains a list of constants required by other packages

const ImageUpdaterAnnotationPrefix = "argocd-image-updater.argoproj.io"

// The annotation on the application resources to indicate the list of images
// allowed for updates.
const ImageUpdaterAnnotation = ImageUpdaterAnnotationPrefix + "/image-list"

// Defaults for Helm parameter names
const (
	DefaultHelmImageName = "image.name"
	DefaultHelmImageTag  = "image.tag"
)

// Helm related annotations
const (
	HelmParamImageNameAnnotation = ImageUpdaterAnnotationPrefix + "/%s.helm.image-name"
	HelmParamImageTagAnnotation  = ImageUpdaterAnnotationPrefix + "/%s.helm.image-tag"
	HelmParamImageSpecAnnotation = ImageUpdaterAnnotationPrefix + "/%s.helm.image-spec"
)

// Kustomize related annotations
const (
	KustomizeApplicationNameAnnotation = ImageUpdaterAnnotationPrefix + "/%s.kustomize.image-name"
)

// Upgrade strategy related annotations
const (
	MatchOptionAnnotation      = ImageUpdaterAnnotationPrefix + "/%s.tag-match"
	IngoreTagsOptionAnnotation = ImageUpdaterAnnotationPrefix + "/%s.ignore-tags"
	UpdateStrategyAnnotation   = ImageUpdaterAnnotationPrefix + "/%s.update-strategy"
)

// Image pull secret related annotations
const (
	SecretListAnnotation = ImageUpdaterAnnotationPrefix + "/%s.pull-secret"
)
