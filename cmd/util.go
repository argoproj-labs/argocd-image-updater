package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/argoproj-labs/argocd-image-updater/pkg/kube"
	"github.com/argoproj/argo-cd/v2/pkg/client/clientset/versioned"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

func getPrintableInterval(interval time.Duration) string {
	if interval == 0 {
		return "once"
	} else {
		return interval.String()
	}
}

func getPrintableHealthPort(port int) string {
	if port == 0 {
		return "off"
	} else {
		return fmt.Sprintf("%d", port)
	}
}

func getKubeConfig(ctx context.Context, namespace string, kubeConfig string) (*kube.ImageUpdaterKubernetesClient, error) {
	var kubeClient *kube.ImageUpdaterKubernetesClient

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.DefaultClientConfig = &clientcmd.DefaultClientConfig
	loadingRules.ExplicitPath = kubeConfig
	overrides := clientcmd.ConfigOverrides{}
	clientConfig := clientcmd.NewInteractiveDeferredLoadingClientConfig(loadingRules, &overrides, os.Stdin)

	config, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, err
	}

	if namespace == "" {
		namespace, _, err = clientConfig.Namespace()
		if err != nil {
			return nil, err
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	applicationsClientset, err := versioned.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	kubeClient = kube.NewKubernetesClient(ctx, clientset, applicationsClientset, namespace)

	return kubeClient, nil
}
