package common

// This file contains a list of constants required by other packages

// Defaults for Helm parameter names
const (
	DefaultHelmImageName = "image.name"
	DefaultHelmImageTag  = "image.tag"
)

// DEPRECATED: These constants have been removed in the CRD branch and will be deprecated in a future release.
// The CRD branch introduces a new architecture that eliminates the need for these annotation-based constants.
// Helm related annotations
const (
	HelmParamImageNameAnnotationSuffix = "/%s.helm.image-name"
	HelmParamImageTagAnnotationSuffix  = "/%s.helm.image-tag"
	HelmParamImageSpecAnnotationSuffix = "/%s.helm.image-spec"
)

// DEPRECATED: These constants have been removed in the CRD branch and will be deprecated in a future release.
// The CRD branch introduces a new architecture that eliminates the need for these annotation-based constants.
// Kustomize related annotations
const (
	KustomizeApplicationNameAnnotationSuffix = "/%s.kustomize.image-name"
)

// DEPRECATED: These constants have been removed in the CRD branch and will be deprecated in a future release.
// The CRD branch introduces a new architecture that eliminates the need for these annotation-based constants.
// Image specific configuration annotations
const (
	OldMatchOptionAnnotationSuffix    = "/%s.tag-match" // Deprecated and will be removed
	AllowTagsOptionAnnotationSuffix   = "/%s.allow-tags"
	IgnoreTagsOptionAnnotationSuffix  = "/%s.ignore-tags"
	ForceUpdateOptionAnnotationSuffix = "/%s.force-update"
	UpdateStrategyAnnotationSuffix    = "/%s.update-strategy"
	PullSecretAnnotationSuffix        = "/%s.pull-secret"
	PlatformsAnnotationSuffix         = "/%s.platforms"
)

// DEPRECATED: These constants have been removed in the CRD branch and will be deprecated in a future release.
// The CRD branch introduces a new architecture that eliminates the need for these annotation-based constants.
// Application-wide update strategy related annotations
const (
	ApplicationWideAllowTagsOptionAnnotationSuffix   = "/allow-tags"
	ApplicationWideIgnoreTagsOptionAnnotationSuffix  = "/ignore-tags"
	ApplicationWideForceUpdateOptionAnnotationSuffix = "/force-update"
	ApplicationWideUpdateStrategyAnnotationSuffix    = "/update-strategy"
	ApplicationWidePullSecretAnnotationSuffix        = "/pull-secret"
)
