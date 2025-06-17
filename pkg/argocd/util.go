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

	kubeClient, err := kube.NewKubernetesClient(ctx, kubeConfig, namespace)
	if err != nil {
		return nil, err
	}

	return kubeClient, nil
}
