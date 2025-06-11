package common

import "github.com/sirupsen/logrus"

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

// DefaultArgoCDServerAddr is a Default ArgoCD server address when running in same cluster as ArgoCD
const DefaultArgoCDServerAddr = "argocd-server.argocd"

// DefaultRegistriesConfPath is a Default path to registry configuration
const DefaultRegistriesConfPath = "/app/config/registries.conf"

// DefaultCommitTemplatePath is a Default path to Git commit message template
const DefaultCommitTemplatePath = "/app/config/commit.template"

const ApplicationsAPIKindK8S = "kubernetes"
const ApplicationsAPIKindArgoCD = "argocd"

// ControllerLogFields contains the constant, structured context for the ImageUpdater controller.
// It's defined as logrus.Fields (map[string]interface{}) to be used directly with logrus loggers.
var ControllerLogFields = logrus.Fields{
	"controller":      "imageupdater",
	"controllerGroup": ImageUpdaterAnnotationPrefix,
	"controllerKind":  "ImageUpdater",
}
