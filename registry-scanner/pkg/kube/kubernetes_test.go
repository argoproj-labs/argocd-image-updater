package kube

import (
	"context"
	"testing"

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/test/fake"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/test/fixture"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewKubernetesClient(t *testing.T) {
	t.Run("Get new K8s client for remote cluster instance", func(t *testing.T) {
		client, err := NewKubernetesClientFromConfig(context.TODO(), "", "../../test/testdata/kubernetes/config")
		require.NoError(t, err)
		assert.NotNil(t, client)
		assert.Equal(t, "default", client.Namespace)
	})

	t.Run("Get new K8s client for remote cluster instance specified namespace", func(t *testing.T) {
		client, err := NewKubernetesClientFromConfig(context.TODO(), "argocd", "../../test/testdata/kubernetes/config")
		require.NoError(t, err)
		assert.NotNil(t, client)
		assert.Equal(t, "argocd", client.Namespace)
	})
}

func Test_GetDataFromSecrets(t *testing.T) {
	t.Run("Get all data from dummy secret", func(t *testing.T) {
		secret := fixture.MustCreateSecretFromFile("../../test/testdata/resources/dummy-secret.json")
		clientset := fake.NewFakeClientsetWithResources(secret)
		client := &KubernetesClient{Clientset: clientset}
		data, err := client.GetSecretData("test-namespace", "test-secret")
		require.NoError(t, err)
		require.NotNil(t, data)
		assert.Len(t, data, 1)
		assert.Equal(t, "argocd", string(data["namespace"]))
	})

	t.Run("Get string data from dummy secret existing field", func(t *testing.T) {
		secret := fixture.MustCreateSecretFromFile("../../test/testdata/resources/dummy-secret.json")
		clientset := fake.NewFakeClientsetWithResources(secret)
		client := &KubernetesClient{Clientset: clientset}
		data, err := client.GetSecretField("test-namespace", "test-secret", "namespace")
		require.NoError(t, err)
		assert.Equal(t, "argocd", data)
	})

	t.Run("Get string data from dummy secret non-existing field", func(t *testing.T) {
		secret := fixture.MustCreateSecretFromFile("../../test/testdata/resources/dummy-secret.json")
		clientset := fake.NewFakeClientsetWithResources(secret)
		client := &KubernetesClient{Clientset: clientset}
		data, err := client.GetSecretField("test-namespace", "test-secret", "nonexisting")
		require.Error(t, err)
		require.Empty(t, data)
	})

	t.Run("Get string data from non-existing secret non-existing field", func(t *testing.T) {
		secret := fixture.MustCreateSecretFromFile("../../test/testdata/resources/dummy-secret.json")
		clientset := fake.NewFakeClientsetWithResources(secret)
		client := &KubernetesClient{Clientset: clientset}
		data, err := client.GetSecretField("test-namespace", "test", "namespace")
		require.Error(t, err)
		require.Empty(t, data)
	})
}
