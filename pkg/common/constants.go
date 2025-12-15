package common

import (
	"github.com/sirupsen/logrus"

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
)

// Application update configurations
const (
	KustomizationPrefix = "kustomization"
	HelmPrefix          = "helmvalues"
)

// Defaults for Helm parameter names
const (
	DefaultHelmImageName = "image.name"
	DefaultHelmImageTag  = "image.tag"
)

// DefaultTargetFilePrefix the prefix of the default git write-back target
const DefaultTargetFilePrefix = ".argocd-source-"

// DefaultTargetFilePattern configurations related to the write-back functionality
const DefaultTargetFilePattern = ".argocd-source-%s_%s.yaml"
const DefaultTargetFilePatternWithoutNamespace = ".argocd-source-%s.yaml"
const DefaultHelmValuesFilename = "values.yaml"

// DefaultGitCommitMessage the default Git commit message's template
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

// ControllerLogFields contains the constant, structured context for the ImageUpdater controller.
// It's defined as logrus.Fields (map[string]interface{}) to be used directly with logrus loggers.
var ControllerLogFields = logrus.Fields{
	"controller":      "imageupdater",
	"controllerGroup": "argocd-image-updater.argoproj.io",
	"controllerKind":  "ImageUpdater",
}

func LogFields(fields logrus.Fields) *logrus.Entry {
	return log.Log().WithFields(fields).WithFields(ControllerLogFields)
}
