package kube

// Kubernetes client related code

import (
	"context"
	"fmt"
	"os"

	"github.com/argoproj-labs/argocd-image-updater/pkg/metrics"

	"github.com/argoproj/argo-cd/pkg/client/clientset/versioned"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

type KubernetesClient struct {
	Clientset             kubernetes.Interface
	ApplicationsClientset versioned.Interface
	Context               context.Context
	Namespace             string
}

func NewKubernetesClient(ctx context.Context, client kubernetes.Interface, applicationsClientset versioned.Interface, namespace string) *KubernetesClient {
	kc := &KubernetesClient{}
	kc.Context = ctx
	kc.Clientset = client
	kc.ApplicationsClientset = applicationsClientset
	kc.Namespace = namespace
	return kc
}

// NewKubernetesClient creates a new Kubernetes client object from given
// configuration file. If configuration file is the empty string, in-cluster
// client will be created.
func NewKubernetesClientFromConfig(ctx context.Context, namespace string, kubeconfig string) (*KubernetesClient, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	loadingRules.DefaultClientConfig = &clientcmd.DefaultClientConfig
	loadingRules.ExplicitPath = kubeconfig
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

	return NewKubernetesClient(ctx, clientset, applicationsClientset, namespace), nil
}

// GetSecretData returns the raw data from named K8s secret in given namespace
func (client *KubernetesClient) GetSecretData(namespace string, secretName string) (map[string][]byte, error) {
	secret, err := client.Clientset.CoreV1().Secrets(namespace).Get(client.Context, secretName, v1.GetOptions{})
	metrics.Clients().IncreaseK8sClientRequest(1)
	if err != nil {
		metrics.Clients().IncreaseK8sClientRequest(1)
		return nil, err
	}
	return secret.Data, nil
}

// GetSecretField returns the value of a field from named K8s secret in given namespace
func (client *KubernetesClient) GetSecretField(namespace string, secretName string, field string) (string, error) {
	secret, err := client.GetSecretData(namespace, secretName)
	metrics.Clients().IncreaseK8sClientRequest(1)
	if err != nil {
		metrics.Clients().IncreaseK8sClientRequest(1)
		return "", err
	}
	if data, ok := secret[field]; !ok {
		return "", fmt.Errorf("secret '%s/%s' does not have a field '%s'", namespace, secretName, field)
	} else {
		return string(data), nil
	}
}
