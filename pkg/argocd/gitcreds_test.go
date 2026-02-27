package argocd

import (
	"context"
	"testing"

	"github.com/argoproj-labs/argocd-image-updater/ext/git"
	"github.com/argoproj-labs/argocd-image-updater/pkg/kube"
	registryKube "github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/kube"
	"github.com/argoproj-labs/argocd-image-updater/test/fake"
	"github.com/argoproj-labs/argocd-image-updater/test/fixture"

	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"

	"github.com/stretchr/testify/assert"
)

func TestGetCredsFromSecret(t *testing.T) {
	store := git.NoopCredsStore{}

	secret1 := fixture.NewSecret("foo", "bar", map[string][]byte{
		"username": []byte("myuser"),
		"password": []byte("mypass"),
	})
	secret2 := fixture.NewSecret("foo1", "bar1", map[string][]byte{
		"username": []byte("myuser"),
	})
	secret3 := fixture.NewSecret("ns", "ghapp", map[string][]byte{
		"githubAppID":                []byte("123"),
		"githubAppInstallationID":    []byte("456"),
		"githubAppPrivateKey":        []byte("appprivatekey"),
		"githubAppEnterpriseBaseUrl": []byte("https://ghe.example.com/api/v3"),
		"tlsClientCertData":          []byte("certdata"),
		"tlsClientCertKey":           []byte("certkey"),
		"insecure":                   []byte("true"),
		"proxy":                      []byte("https://proxy.example.com"),
	})
	secret4 := fixture.NewSecret("ns", "ghapp-minimal", map[string][]byte{
		"githubAppID":             []byte("789"),
		"githubAppInstallationID": []byte("101"),
		"githubAppPrivateKey":     []byte("minimalkey"),
	})
	secret5 := fixture.NewSecret("ns", "ghapp-bad-id", map[string][]byte{
		"githubAppID":             []byte("abc"),
		"githubAppInstallationID": []byte("456"),
		"githubAppPrivateKey":     []byte("key"),
	})
	secret6 := fixture.NewSecret("ns", "ghapp-bad-insecure", map[string][]byte{
		"githubAppID":             []byte("123"),
		"githubAppInstallationID": []byte("456"),
		"githubAppPrivateKey":     []byte("key"),
		"insecure":                []byte("maybe"),
	})

	kubeClient := kube.ImageUpdaterKubernetesClient{
		KubeClient: &registryKube.KubernetesClient{
			Clientset: fake.NewFakeClientsetWithResources(secret1, secret2, secret3, secret4, secret5, secret6),
		},
	}

	tests := []struct {
		name          string
		gitRepo       string
		secretRef     string
		expectedCreds git.Creds
		expectedErr   string
	}{
		{
			name:          "HTTPS credentials",
			gitRepo:       "https://github.com/example/repo.git",
			secretRef:     "foo/bar",
			expectedCreds: git.NewHTTPSCreds("myuser", "mypass", "", "", true, "", store, false),
		},
		{
			name:        "invalid secret reference format",
			gitRepo:     "https://github.com/example/repo.git",
			secretRef:   "invalid",
			expectedErr: "secret ref must be in format 'namespace/name', but is 'invalid'",
		},
		{
			name:        "missing password field",
			gitRepo:     "https://github.com/example/repo.git",
			secretRef:   "foo1/bar1",
			expectedErr: "invalid secret foo1/bar1: does not contain field password",
		},
		{
			name:        "unknown repository type",
			gitRepo:     "unknown://example.com/repo.git",
			secretRef:   "foo/bar",
			expectedErr: "unknown repository type",
		},
		{
			name:      "GitHub App with enterprise base URL",
			gitRepo:   "https://ghe.example.com/org/repo.git",
			secretRef: "ns/ghapp",
			expectedCreds: git.NewGitHubAppCreds(
				123, 456, "appprivatekey",
				"https://ghe.example.com/api/v3", "https://ghe.example.com/org/repo.git",
				"certdata", "certkey", true, "https://proxy.example.com", store,
			),
		},
		{
			name:      "GitHub App with absent optional fields defaults safely",
			gitRepo:   "https://github.com/org/repo.git",
			secretRef: "ns/ghapp-minimal",
			expectedCreds: git.NewGitHubAppCreds(
				789, 101, "minimalkey",
				"", "https://github.com/org/repo.git",
				"", "", false, "", store,
			),
		},
		{
			name:        "non-numeric githubAppID",
			gitRepo:     "https://github.com/org/repo.git",
			secretRef:   "ns/ghapp-bad-id",
			expectedErr: "invalid value in field githubAppID",
		},
		{
			name:      "malformed insecure value defaults to false",
			gitRepo:   "https://github.com/org/repo.git",
			secretRef: "ns/ghapp-bad-insecure",
			expectedCreds: git.NewGitHubAppCreds(
				123, 456, "key",
				"", "https://github.com/org/repo.git",
				"", "", false, "", store,
			),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			wbc := &WriteBackConfig{
				GitRepo:  tt.gitRepo,
				GitCreds: store,
			}
			creds, err := getCredsFromSecret(wbc, tt.secretRef, &kubeClient)
			if tt.expectedErr != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErr)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedCreds, creds)
			}
		})
	}
}

func TestGetGitCredsSource(t *testing.T) {
	ctx := context.Background()
	kubeClient := &kube.ImageUpdaterKubernetesClient{}
	wbc := &WriteBackConfig{
		GitRepo:  "https://github.com/example/repo.git",
		GitCreds: git.NoopCredsStore{},
	}

	// Test case 1: 'repocreds' credentials
	creds1, err := getGitCredsSource(ctx, "repocreds", kubeClient, wbc)
	assert.NoError(t, err)
	assert.NotNil(t, creds1)

	// Test case 2: 'secret:<namespace>/<secret>' credentials
	creds2, err := getGitCredsSource(ctx, "secret:foo/bar", kubeClient, wbc)
	assert.NoError(t, err)
	assert.NotNil(t, creds2)

	// Test case 3: Unexpected credentials format
	_, err = getGitCredsSource(ctx, "invalid", kubeClient, wbc)
	assert.Error(t, err)
	assert.EqualError(t, err, "unexpected credentials format. Expected 'repocreds' or 'secret:<namespace>/<secret>' but got 'invalid'")
}

func TestGetCAPath(t *testing.T) {
	ctx := context.Background()
	// Test case 1: HTTPS URL
	repoURL := "https://github.com/example/repo.git"
	expectedCAPath := ""
	caPath := getCAPath(ctx, repoURL)
	assert.Equal(t, expectedCAPath, caPath)

	// Test case 2: OCI URL
	repoURL = "oci://example.com/repo"
	expectedCAPath = ""
	caPath = getCAPath(ctx, repoURL)
	assert.Equal(t, expectedCAPath, caPath)

	// Test case 3: SSH URL
	repoURL = "git@github.com:example/repo.git"
	expectedCAPath = ""
	caPath = getCAPath(ctx, repoURL)
	assert.Equal(t, expectedCAPath, caPath)

	// Test case 4: Invalid URL
	repoURL = "invalid-url"
	expectedCAPath = ""
	caPath = getCAPath(ctx, repoURL)
	assert.Equal(t, expectedCAPath, caPath)
}

func TestGetGitCreds(t *testing.T) {
	ctx := context.Background()
	store := git.NoopCredsStore{}

	// Test case 1: HTTP credentials
	repo := &v1alpha1.Repository{
		Username: "myuser",
		Password: "mypass",
		Repo:     "https://github.com/example/repo.git",
	}
	expectedHTTPSCreds := git.NewHTTPSCreds("myuser", "mypass", "", "", false, "", store, false)
	httpCreds := GetGitCreds(ctx, repo, store)
	assert.Equal(t, expectedHTTPSCreds, httpCreds)

	// Test case 2: SSH credentials
	repo = &v1alpha1.Repository{
		Username:      "myuser",
		SSHPrivateKey: "privatekey",
		Repo:          "https://github.com/example/repo.git",
	}
	expectedSSHCreds := git.NewSSHCreds("privatekey", "", false, store, "")
	sshCreds := GetGitCreds(ctx, repo, store)
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
	githubAppCreds := GetGitCreds(ctx, repo, store)
	assert.Equal(t, expectedGitHubAppCreds, githubAppCreds)

	// Test case 4: Google Cloud credentials
	repo = &v1alpha1.Repository{
		Username:             "myuser",
		GCPServiceAccountKey: "serviceaccountkey",
	}
	expectedGoogleCloudCreds := git.NewGoogleCloudCreds("serviceaccountkey", store)
	googleCloudCreds := GetGitCreds(ctx, repo, store)
	repo.Password = ""
	repo.SSHPrivateKey = ""
	assert.Equal(t, expectedGoogleCloudCreds, googleCloudCreds)

	// Test case 5: No credentials
	expectedNopCreds := git.NopCreds{}
	nopCreds := GetGitCreds(ctx, nil, store)
	assert.Equal(t, expectedNopCreds, nopCreds)
}
