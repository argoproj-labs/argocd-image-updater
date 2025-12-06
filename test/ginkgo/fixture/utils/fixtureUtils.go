package utils

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	argocdv1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"

	admissionv1 "k8s.io/api/admissionregistration/v1"
	apps "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	crdv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	imageUpdater "github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"

	argov1alpha1api "github.com/argoproj-labs/argocd-operator/api/v1alpha1"
	argov1beta1api "github.com/argoproj-labs/argocd-operator/api/v1beta1"

	//lint:ignore ST1001 "This is a common practice in Gomega tests for readability."
	. "github.com/onsi/gomega" //nolint:all
)

func GetE2ETestKubeClient() (client.Client, *runtime.Scheme) {
	config, err := getSystemKubeConfig()
	Expect(err).ToNot(HaveOccurred())

	k8sClient, scheme, err := getKubeClient(config)
	Expect(err).ToNot(HaveOccurred())

	return k8sClient, scheme
}

func GetE2ETestKubeClientWithError() (client.Client, *runtime.Scheme, error) {
	config, err := getSystemKubeConfig()
	if err != nil {
		return nil, nil, err
	}

	k8sClient, scheme, err := getKubeClient(config)
	if err != nil {
		return nil, nil, err
	}

	return k8sClient, scheme, nil
}

// getKubeClient returns a controller-runtime Client for accessing K8s API resources used by the controller.
func getKubeClient(config *rest.Config) (client.Client, *runtime.Scheme, error) {

	scheme := runtime.NewScheme()

	if err := corev1.AddToScheme(scheme); err != nil {
		return nil, nil, err
	}

	if err := apps.AddToScheme(scheme); err != nil {
		return nil, nil, err
	}
	if err := rbacv1.AddToScheme(scheme); err != nil {
		return nil, nil, err
	}

	if err := admissionv1.AddToScheme(scheme); err != nil {
		return nil, nil, err
	}

	if err := crdv1.AddToScheme(scheme); err != nil {
		return nil, nil, err
	}

	if err := argov1beta1api.AddToScheme(scheme); err != nil {
		return nil, nil, err
	}

	if err := argocdv1alpha1.AddToScheme(scheme); err != nil {
		return nil, nil, err
	}

	if err := argov1alpha1api.AddToScheme(scheme); err != nil {
		return nil, nil, err
	}

	if err := networkingv1.AddToScheme(scheme); err != nil {
		return nil, nil, err
	}

	if err := autoscalingv2.AddToScheme(scheme); err != nil {
		return nil, nil, err
	}

	if err := batchv1.AddToScheme(scheme); err != nil {
		return nil, nil, err
	}

	if err := imageUpdater.AddToScheme(scheme); err != nil {
		return nil, nil, err
	}

	k8sClient, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		return nil, nil, err
	}

	return k8sClient, scheme, nil

}

// Retrieve the system-level Kubernetes config (e.g. ~/.kube/config or service account config from volume)
func getSystemKubeConfig() (*rest.Config, error) {

	overrides := clientcmd.ConfigOverrides{}

	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, &overrides)

	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, err
	}

	// Increase QPS and Burst to avoid rate limiting issues during cleanup.
	// The default values (5 QPS, 10 Burst) can cause "client rate limiter Wait returned an error"
	// errors in CI environments with limited resources where cleanup operations are slower.
	restConfig.QPS = 50
	restConfig.Burst = 100

	return restConfig, nil
}
