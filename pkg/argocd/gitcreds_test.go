package argocd

import (
	"testing"

	"github.com/argoproj-labs/argocd-image-updater/ext/git"
	"github.com/argoproj-labs/argocd-image-updater/pkg/kube"
	"github.com/argoproj-labs/argocd-image-updater/test/fake"
	"github.com/argoproj-labs/argocd-image-updater/test/fixture"

	"github.com/stretchr/testify/assert"
)

func TestGetCredsFromSecret(t *testing.T) {
	wbc := &WriteBackConfig{
		GitRepo:  "https://github.com/example/repo.git",
		GitCreds: git.NoopCredsStore{},
	}

	secret1 := fixture.NewSecret("foo", "bar", map[string][]byte{"username": []byte("myuser"), "password": []byte("mypass")})
	secret2 := fixture.NewSecret("foo1", "bar1", map[string][]byte{"username": []byte("myuser")})
	kubeClient := kube.KubernetesClient{
		Clientset: fake.NewFakeClientsetWithResources(secret1, secret2),
	}

	// Test case 1: Valid secret reference
	credentialsSecret := "foo/bar"
	expectedCreds := git.NewHTTPSCreds("myuser", "mypass", "", "", true, "", wbc.GitCreds, false)
	creds, err := getCredsFromSecret(wbc, credentialsSecret, &kubeClient)
	assert.NoError(t, err)
	assert.Equal(t, expectedCreds, creds)

	// Test case 2: Invalid secret reference
	credentialsSecret = "invalid"
	_, err = getCredsFromSecret(wbc, credentialsSecret, &kubeClient)
	assert.Error(t, err)
	assert.EqualError(t, err, "secret ref must be in format 'namespace/name', but is 'invalid'")

	// Test case 3: Missing field in secret
	credentialsSecret = "foo1/bar1"
	_, err = getCredsFromSecret(wbc, credentialsSecret, &kubeClient)
	assert.Error(t, err)
	assert.EqualError(t, err, "invalid secret foo1/bar1: does not contain field password")

	// Test case 4: Unknown repository type
	credentialsSecret = "foo/bar"
	wbc.GitRepo = "unknown://example.com/repo.git"
	_, err = getCredsFromSecret(wbc, credentialsSecret, &kubeClient)
	assert.Error(t, err)
	assert.EqualError(t, err, "unknown repository type")
}
