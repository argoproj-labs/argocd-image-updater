package main

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/argoproj-labs/argocd-image-updater/pkg/kube"
	registryKube "github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/kube"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
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
	var fullKubeConfigPath string
	var kubeClient *kube.ImageUpdaterKubernetesClient
	var err error

	if kubeConfig != "" {
		fullKubeConfigPath, err = filepath.Abs(kubeConfig)
		if err != nil {
			return nil, fmt.Errorf("cannot expand path %s: %v", kubeConfig, err)
		}
	}

	if fullKubeConfigPath != "" {
		log.Debugf("Creating Kubernetes client from %s", fullKubeConfigPath)
	} else {
		log.Debugf("Creating in-cluster Kubernetes client")
	}

	kubernetesClient, err := registryKube.NewKubernetesClientFromConfig(ctx, namespace, fullKubeConfigPath)
	if err != nil {
		return nil, err
	}
	kubeClient = &kube.ImageUpdaterKubernetesClient{
		KubeClient: kubernetesClient,
	}

	return kubeClient, nil
}
