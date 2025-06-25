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
	"os"

	argocdapplicationv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/spf13/cobra"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	argocdimageupdaterv1alpha1 "github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"
	// +kubebuilder:scaffold:imports
)

var (
	scheme = runtime.NewScheme()
)

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
	err := rootCmd.Execute()
	return err
}

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(argocdimageupdaterv1alpha1.AddToScheme(scheme))
	utilruntime.Must(argocdapplicationv1alpha1.AddToScheme(scheme))
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
