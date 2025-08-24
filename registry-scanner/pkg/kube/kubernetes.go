package kube

// Kubernetes client related code

import (
	"context"
	"fmt"
	"os"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/tools/clientcmd"
)

type KubernetesClient struct {
	Clientset kubernetes.Interface
	Context   context.Context
	Namespace string
}

func NewKubernetesClient(ctx context.Context, client kubernetes.Interface, namespace string) *KubernetesClient {
	kc := &KubernetesClient{}
	kc.Context = ctx
	kc.Clientset = client
	kc.Namespace = namespace
	return kc
}

// NewKubernetesClientFromConfig creates a new Kubernetes client object from given
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

	return NewKubernetesClient(ctx, clientset, namespace), nil
}

// GetSecretData returns the raw data from named K8s secret in given namespace
func (client *KubernetesClient) GetSecretData(namespace string, secretName string) (map[string][]byte, error) {
	secret, err := client.Clientset.CoreV1().Secrets(namespace).Get(client.Context, secretName, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return secret.Data, nil
}

// GetSecretField returns the value of a field from named K8s secret in given namespace
func (client *KubernetesClient) GetSecretField(namespace string, secretName string, field string) (string, error) {
	secret, err := client.GetSecretData(namespace, secretName)
	if err != nil {
		return "", err
	}
	if data, ok := secret[field]; !ok {
		return "", fmt.Errorf("secret '%s/%s' does not have a field '%s'", namespace, secretName, field)
	} else {
		return string(data), nil
	}
}
