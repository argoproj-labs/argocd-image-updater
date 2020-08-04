package client

// Kubernetes client related code

import (
	"fmt"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type KubernetesClient struct {
	Clientset kubernetes.Interface
}

// NewKubernetesClient creates a new Kubernetes client object from given
// configuration file. If configuration file is the empty string, in-cluster
// client will be created.
func NewKubernetesClient(kubeconfig string) (*KubernetesClient, error) {
	kClient := KubernetesClient{}

	var config *rest.Config
	var err error

	if kubeconfig != "" {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	} else {
		config, err = rest.InClusterConfig()
	}
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	kClient.Clientset = clientset
	return &kClient, nil
}

// GetSecretData returns the raw data from named K8s secret in given namespace
func (client *KubernetesClient) GetSecretData(namespace string, secretName string) (map[string][]byte, error) {
	secret, err := client.Clientset.CoreV1().Secrets(namespace).Get(secretName, v1.GetOptions{})
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
