package argocd

import (
	"context"
	"fmt"
	"time"

	"github.com/argoproj-labs/argocd-image-updater/pkg/kube"
)

func GetPrintableInterval(interval time.Duration) string {
	if interval == 0 {
		return "once"
	} else {
		return interval.String()
	}
}

func GetPrintableHealthPort(port int) string {
	if port == 0 {
		return "off"
	} else {
		return fmt.Sprintf("%d", port)
	}
}

func GetKubeConfig(ctx context.Context, namespace string, kubeConfig string) (*kube.ImageUpdaterKubernetesClient, error) {
	kubeClient, err := kube.NewImageUpdaterKubernetesClient(ctx, kubeConfig, namespace)
	if err != nil {
		return nil, err
	}

	return kubeClient, nil
}

// Infer the type of the application based on the image's manifest target fields.
// If no type can be inferred, return an error.
func (image Image) GetType() (ApplicationType, error) {
	if image.HelmImageName != "" || image.HelmImageTag != "" || image.HelmImageSpec != "" {
		return ApplicationTypeHelm, nil
	}
	if image.KustomizeImageName != "" {
		return ApplicationTypeKustomize, nil
	}
	if image.PluginEnvName != "" || image.PluginEnvTag != "" || image.PluginEnvSpec != "" {
		return ApplicationTypePlugin, nil
	}
	return ApplicationTypeUnsupported, fmt.Errorf("no manifest target type found for image %s", image.String())
}
