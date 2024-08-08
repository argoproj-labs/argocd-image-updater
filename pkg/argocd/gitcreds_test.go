package argocd

import (
	"testing"

	"github.com/argoproj-labs/argocd-image-updater/ext/git"
	"github.com/argoproj-labs/argocd-image-updater/pkg/kube"
	"github.com/argoproj-labs/argocd-image-updater/test/fake"
	"github.com/argoproj-labs/argocd-image-updater/test/fixture"

	"github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"

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

func TestGetGitCredsSource(t *testing.T) {
	kubeClient := &kube.KubernetesClient{}
	wbc := &WriteBackConfig{
		GitRepo:  "https://github.com/example/repo.git",
		GitCreds: git.NoopCredsStore{},
	}

	// Test case 1: 'repocreds' credentials
	creds1, err := getGitCredsSource("repocreds", kubeClient, wbc)
	assert.NoError(t, err)
	assert.NotNil(t, creds1)

	// Test case 2: 'secret:<namespace>/<secret>' credentials
	creds2, err := getGitCredsSource("secret:foo/bar", kubeClient, wbc)
	assert.NoError(t, err)
	assert.NotNil(t, creds2)

	// Test case 3: Unexpected credentials format
	_, err = getGitCredsSource("invalid", kubeClient, wbc)
	assert.Error(t, err)
	assert.EqualError(t, err, "unexpected credentials format. Expected 'repocreds' or 'secret:<namespace>/<secret>' but got 'invalid'")
}

func TestGetCAPath(t *testing.T) {
	// Test case 1: HTTPS URL
	repoURL := "https://github.com/example/repo.git"
	expectedCAPath := ""
	caPath := getCAPath(repoURL)
	assert.Equal(t, expectedCAPath, caPath)

	// Test case 2: OCI URL
	repoURL = "oci://example.com/repo"
	expectedCAPath = ""
	caPath = getCAPath(repoURL)
	assert.Equal(t, expectedCAPath, caPath)

	// Test case 3: SSH URL
	repoURL = "git@github.com:example/repo.git"
	expectedCAPath = ""
	caPath = getCAPath(repoURL)
	assert.Equal(t, expectedCAPath, caPath)

	// Test case 4: Invalid URL
	repoURL = "invalid-url"
	expectedCAPath = ""
	caPath = getCAPath(repoURL)
	assert.Equal(t, expectedCAPath, caPath)
}

func TestGetGitCreds(t *testing.T) {

	store := git.NoopCredsStore{}

	// Test case 1: HTTP credentials
	repo := &v1alpha1.Repository{
		Username: "myuser",
		Password: "mypass",
		Repo:     "https://github.com/example/repo.git",
	}
	expectedHTTPSCreds := git.NewHTTPSCreds("myuser", "mypass", "", "", false, "", store, false)
	httpCreds := GetGitCreds(repo, store)
	assert.Equal(t, expectedHTTPSCreds, httpCreds)

	// Test case 2: SSH credentials
	repo = &v1alpha1.Repository{
		Username:      "myuser",
		SSHPrivateKey: "privatekey",
		Repo:          "https://github.com/example/repo.git",
	}
	expectedSSHCreds := git.NewSSHCreds("privatekey", "", false, store, "")
	sshCreds := GetGitCreds(repo, store)
	assert.Equal(t, expectedSSHCreds, sshCreds)

	// Test case 3: GitHub App credentials
	repo = &v1alpha1.Repository{
		Username:                   "myuser",
		GithubAppPrivateKey:        "appprivatekey",
		GithubAppId:                123,
		GithubAppInstallationId:    456,
		GitHubAppEnterpriseBaseURL: "enterpriseurl",
		Repo:                       "https://github.com/example/repo.git",
		TLSClientCertData:          "certdata",
		TLSClientCertKey:           "certkey",
		Insecure:                   true,
		Proxy:                      "proxy",
	}
	expectedGitHubAppCreds := git.NewGitHubAppCreds(123, 456, "appprivatekey", "enterpriseurl", "https://github.com/example/repo.git", "certdata", "certkey", true, "proxy", store)
	githubAppCreds := GetGitCreds(repo, store)
	assert.Equal(t, expectedGitHubAppCreds, githubAppCreds)

	// Test case 4: Google Cloud credentials
	repo = &v1alpha1.Repository{
		Username:             "myuser",
		GCPServiceAccountKey: "serviceaccountkey",
	}
	expectedGoogleCloudCreds := git.NewGoogleCloudCreds("serviceaccountkey", store)
	googleCloudCreds := GetGitCreds(repo, store)
	repo.Password = ""
	repo.SSHPrivateKey = ""
	assert.Equal(t, expectedGoogleCloudCreds, googleCloudCreds)

	// Test case 5: No credentials
	expectedNopCreds := git.NopCreds{}
	nopCreds := GetGitCreds(nil, store)
	assert.Equal(t, expectedNopCreds, nopCreds)
}
