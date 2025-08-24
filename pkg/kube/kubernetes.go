package kube

// Kubernetes client related code

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/kube"

	appv1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/argoproj/argo-cd/v2/pkg/client/clientset/versioned"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/clientcmd"
)

type ImageUpdaterKubernetesClient struct {
	ApplicationsClientset versioned.Interface
	KubeClient            *kube.KubernetesClient
}

func NewImageUpdaterKubernetesClient(ctx context.Context, kubeConfig, namespace string) (*ImageUpdaterKubernetesClient, error) {
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

	kc := &ImageUpdaterKubernetesClient{}
	kc.KubeClient = kube.NewKubernetesClient(ctx, clientset, namespace)
	kc.ApplicationsClientset = applicationsClientset
	return kc, nil
}

// CreateApplicationEvent creates a kubernetes event with a custom reason and message for an application.
func (client *ImageUpdaterKubernetesClient) CreateApplicationEvent(app *appv1alpha1.Application, reason string, message string, annotations map[string]string) (*v1.Event, error) {
	t := metav1.Time{Time: time.Now()}

	event := v1.Event{
		ObjectMeta: metav1.ObjectMeta{
			Name:        fmt.Sprintf("%v.%x", app.ObjectMeta.Name, t.UnixNano()),
			Namespace:   app.ObjectMeta.Namespace,
			Annotations: annotations,
		},
		Source: v1.EventSource{
			Component: "ArgocdImageUpdater",
		},
		InvolvedObject: v1.ObjectReference{
			Kind:            app.Kind,
			APIVersion:      app.APIVersion,
			Name:            app.ObjectMeta.Name,
			Namespace:       app.ObjectMeta.Namespace,
			ResourceVersion: app.ObjectMeta.ResourceVersion,
			UID:             app.ObjectMeta.UID,
		},
		FirstTimestamp: t,
		LastTimestamp:  t,
		Count:          1,
		Message:        message,
		Type:           v1.EventTypeNormal,
		Reason:         reason,
	}

	result, err := client.KubeClient.Clientset.CoreV1().Events(app.ObjectMeta.Namespace).Create(client.KubeClient.Context, &event, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}
	return result, nil
}
