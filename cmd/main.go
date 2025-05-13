/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"k8s.io/apimachinery/pkg/runtime"
	"os"
	"text/template"
	"time"

	"github.com/argoproj-labs/argocd-image-updater/ext/git"
	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd"
	"github.com/argoproj-labs/argocd-image-updater/pkg/kube"

	"github.com/spf13/cobra"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	argocdimageupdaterv1alpha1 "github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	// +kubebuilder:scaffold:imports
)

var (
	scheme = runtime.NewScheme()
)

// Default ArgoCD server address when running in same cluster as ArgoCD
const defaultArgoCDServerAddr = "argocd-server.argocd"

// Default path to registry configuration
const defaultRegistriesConfPath = "/app/config/registries.conf"

// Default path to Git commit message template
const defaultCommitTemplatePath = "/app/config/commit.template"

const applicationsAPIKindK8S = "kubernetes"
const applicationsAPIKindArgoCD = "argocd"

// ImageUpdaterConfig contains global configuration and required runtime data
type ImageUpdaterConfig struct {
	ApplicationsAPIKind    string
	ClientOpts             argocd.ClientOptions
	ArgocdNamespace        string
	AppNamespace           string
	DryRun                 bool
	CheckInterval          time.Duration
	ArgoClient             argocd.ArgoCD
	LogLevel               string
	KubeClient             *kube.ImageUpdaterKubernetesClient
	MaxConcurrency         int
	HealthPort             int
	MetricsPort            int
	RegistriesConf         string
	AppNamePatterns        []string
	AppLabel               string
	GitCommitUser          string
	GitCommitMail          string
	GitCommitMessage       *template.Template
	GitCommitSigningKey    string
	GitCommitSigningMethod string
	GitCommitSignOff       bool
	DisableKubeEvents      bool
	GitCreds               git.CredsStore
	EnableWebhook          bool
}

// newRootCommand implements the root command of argocd-image-updater
func newRootCommand() error {
	var rootCmd = &cobra.Command{
		Use:   "argocd-image-updater",
		Short: "Automatically update container images with ArgoCD",
	}
	rootCmd.AddCommand(newRunCommand())
	rootCmd.AddCommand(newVersionCommand())
	rootCmd.AddCommand(newTestCommand())
	rootCmd.AddCommand(newTemplateCommand())
	rootCmd.AddCommand(NewWebhookCommand())
	err := rootCmd.Execute()
	return err
}

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(argocdimageupdaterv1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	var err error

	// FIXME(jannfis):
	// This is a workaround for supporting the Argo CD askpass implementation.
	// When the environment ARGOCD_BINARY_NAME is set to argocd-git-ask-pass,
	// we divert from the main path of execution to become a git credentials
	// helper.
	cmdName := os.Getenv("ARGOCD_BINARY_NAME")
	if cmdName == "argocd-git-ask-pass" {
		cmd := NewAskPassCommand()
		err = cmd.Execute()
	} else {
		err = newRootCommand()
	}
	if err != nil {
		os.Exit(1)
	}

	os.Exit(0)
}
