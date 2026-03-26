package common

// This file contains a list of constants required by other packages

const ImageUpdaterAnnotationPrefix = "argocd-image-updater.argoproj.io"

// The annotation on the application resources to indicate the list of images
// allowed for updates.
const ImageUpdaterAnnotation = ImageUpdaterAnnotationPrefix + "/image-list"

// Application update configuration related annotations
const (
	WriteBackMethodAnnotation = ImageUpdaterAnnotationPrefix + "/write-back-method"
	GitBranchAnnotation       = ImageUpdaterAnnotationPrefix + "/git-branch"
	GitRepositoryAnnotation   = ImageUpdaterAnnotationPrefix + "/git-repository"
	WriteBackTargetAnnotation = ImageUpdaterAnnotationPrefix + "/write-back-target"
	KustomizationPrefix       = "kustomization"
	HelmPrefix                = "helmvalues"
)

// DefaultTargetFilePattern configurations related to the write-back functionality
const DefaultTargetFilePattern = ".argocd-source-%s_%s.yaml"
const DefaultTargetFilePatternWithoutNamespace = ".argocd-source-%s.yaml"
const DefaultHelmValuesFilename = "values.yaml"

// The default Git commit message's template
const DefaultGitCommitMessage = `build: automatic update of {{ .AppName }}

{{ range .AppChanges -}}
updates image {{ .Image }} tag '{{ .OldTag }}' to '{{ .NewTag }}'
{{ end -}}
`
