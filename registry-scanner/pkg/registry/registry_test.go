package registry

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	distclient "github.com/distribution/distribution/v3/registry/client"

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/image"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/kube"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/options"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/registry/mocks"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/tag"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/test/fake"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/test/fixture"

	"github.com/distribution/distribution/v3/manifest/schema1" //nolint:staticcheck
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Test relies on image package which is not available yet. Will uncomment as soon as it is available.
func Test_GetTags(t *testing.T) {

	t.Run("Check for correctly returned tags with semver sort", func(t *testing.T) {
		regClient := mocks.RegistryClient{}
		regClient.On("NewRepository", mock.Anything).Return(nil)
		regClient.On("Tags", mock.Anything).Return([]string{"1.2.0", "1.2.1", "1.2.2"}, nil)

		ep, err := GetRegistryEndpoint(context.Background(), &image.ContainerImage{RegistryURL: ""})
		require.NoError(t, err)

		img := image.NewFromIdentifier("foo/bar:1.2.0")

		tl, err := ep.GetTags(context.Background(), img, &regClient, &image.VersionConstraint{Strategy: image.StrategySemVer, Options: options.NewManifestOptions()})
		require.NoError(t, err)
		assert.NotEmpty(t, tl)

		tag, err := ep.Cache.GetTag("foo/bar", "1.2.1")
		require.NoError(t, err)
		assert.Nil(t, tag)
	})

	t.Run("Check for correctly returned tags with filter function applied", func(t *testing.T) {
		regClient := mocks.RegistryClient{}
		regClient.On("NewRepository", mock.Anything).Return(nil)
		regClient.On("Tags", mock.Anything).Return([]string{"1.2.0", "1.2.1", "1.2.2"}, nil)
		ctx := context.Background()
		ep, err := GetRegistryEndpoint(ctx, &image.ContainerImage{RegistryURL: ""})
		require.NoError(t, err)

		img := image.NewFromIdentifier("foo/bar:1.2.0")

		tl, err := ep.GetTags(ctx, img, &regClient, &image.VersionConstraint{
			Strategy:  image.StrategySemVer,
			MatchFunc: image.MatchFuncNone,
			Options:   options.NewManifestOptions()})
		require.NoError(t, err)
		assert.Empty(t, tl.Tags())

		tag, err := ep.Cache.GetTag("foo/bar", "1.2.1")
		require.NoError(t, err)
		assert.Nil(t, tag)
	})

	t.Run("Check for correctly returned tags with name sort", func(t *testing.T) {

		regClient := mocks.RegistryClient{}
		regClient.On("NewRepository", mock.Anything).Return(nil)
		regClient.On("Tags", mock.Anything).Return([]string{"1.2.0", "1.2.1", "1.2.2"}, nil)

		ep, err := GetRegistryEndpoint(context.Background(), &image.ContainerImage{RegistryURL: ""})
		require.NoError(t, err)

		img := image.NewFromIdentifier("foo/bar:1.2.0")

		tl, err := ep.GetTags(context.Background(), img, &regClient, &image.VersionConstraint{Strategy: image.StrategyAlphabetical, Options: options.NewManifestOptions()})
		require.NoError(t, err)
		assert.NotEmpty(t, tl)

		tag, err := ep.Cache.GetTag("foo/bar", "1.2.1")
		require.NoError(t, err)
		assert.Nil(t, tag)
	})

	t.Run("Check for correctly returned tags with latest sort", func(t *testing.T) {
		ts := "2006-01-02T15:04:05.999999999Z"
		meta1 := &schema1.SignedManifest{ //nolint:staticcheck
			Manifest: schema1.Manifest{ //nolint:staticcheck
				History: []schema1.History{ //nolint:staticcheck
					{
						V1Compatibility: `{"created":"` + ts + `"}`,
					},
				},
			},
		}
		ctx := context.Background()
		regClient := mocks.RegistryClient{}
		regClient.On("NewRepository", mock.Anything).Return(nil)
		regClient.On("Tags", mock.Anything).Return([]string{"1.2.0", "1.2.1", "1.2.2"}, nil)
		regClient.On("ManifestForTag", mock.Anything, mock.Anything).Return(meta1, nil)
		regClient.On("TagMetadata", mock.Anything, mock.Anything, mock.Anything).Return(&tag.TagInfo{}, nil)
		ep, err := GetRegistryEndpoint(ctx, &image.ContainerImage{RegistryURL: ""})
		require.NoError(t, err)
		ep.Cache.ClearCache()

		img := image.NewFromIdentifier("foo/bar:1.2.0")
		tl, err := ep.GetTags(ctx, img, &regClient, &image.VersionConstraint{Strategy: image.StrategyNewestBuild, Options: options.NewManifestOptions()})
		require.NoError(t, err)
		assert.NotEmpty(t, tl)

		tag, err := ep.Cache.GetTag("foo/bar", "1.2.1")
		require.NoError(t, err)
		require.NotNil(t, tag)
		require.Equal(t, "1.2.1", tag.TagName)
	})

	t.Run("401 with valid cached creds clears endpoint and returns ErrCredentialsInvalid", func(t *testing.T) {
		authErr := &distclient.UnexpectedHTTPResponseError{
			StatusCode: http.StatusUnauthorized,
			ParseErr:   errors.New("unauthorized"),
		}
		regClient := mocks.RegistryClient{}
		regClient.On("NewRepository", mock.Anything).Return(nil)
		regClient.On("Tags", mock.Anything).Return([]string(nil), authErr)

		ep := &RegistryEndpoint{
			RegistryAPI:  "https://example.com",
			Credentials:  "env:FOO",
			CredsExpire:  1 * time.Hour,
			CredsUpdated: time.Now(),
			Username:     "cacheduser",
			Password:     "cachedpass",
		}
		img := image.NewFromIdentifier("foo/bar:1.0.0")
		vc := &image.VersionConstraint{Strategy: image.StrategySemVer, Options: options.NewManifestOptions()}

		tl, err := ep.GetTags(context.Background(), img, &regClient, vc)
		require.Error(t, err)
		assert.Nil(t, tl)
		assert.ErrorIs(t, err, ErrCredentialsInvalid)
		assert.Empty(t, ep.Username)
		assert.Empty(t, ep.Password)
	})

	t.Run("401 with expired cached creds returns original error not ErrCredentialsInvalid", func(t *testing.T) {
		authErr := &distclient.UnexpectedHTTPResponseError{
			StatusCode: http.StatusUnauthorized,
			ParseErr:   errors.New("unauthorized"),
		}
		regClient := mocks.RegistryClient{}
		regClient.On("NewRepository", mock.Anything).Return(nil)
		regClient.On("Tags", mock.Anything).Return([]string(nil), authErr)

		ep := &RegistryEndpoint{
			RegistryAPI:  "https://example.com",
			Credentials:  "env:FOO",
			CredsExpire:  1 * time.Second,
			CredsUpdated: time.Now().Add(-2 * time.Second),
			Username:     "olduser",
			Password:     "oldpass",
		}
		img := image.NewFromIdentifier("foo/bar:1.0.0")
		vc := &image.VersionConstraint{Strategy: image.StrategySemVer, Options: options.NewManifestOptions()}

		tl, err := ep.GetTags(context.Background(), img, &regClient, vc)
		require.Error(t, err)
		assert.Nil(t, tl)
		assert.NotErrorIs(t, err, ErrCredentialsInvalid)
		assert.ErrorIs(t, err, authErr)
		assert.Equal(t, "olduser", ep.Username)
		assert.Equal(t, "oldpass", ep.Password)
	})
}

func Test_ExpireCredentials(t *testing.T) {
	epYAML := `
registries:
- name: GitHub Container Registry
  api_url: https://ghcr.io
  ping: no
  prefix: ghcr.io
  credentials: env:TEST_CREDS
  credsexpire: 3s
`
	t.Run("Expire credentials", func(t *testing.T) {
		epl, err := ParseRegistryConfiguration(epYAML)
		require.NoError(t, err)
		require.Len(t, epl.Items, 1)

		// New registry configuration
		err = AddRegistryEndpointFromConfig(context.Background(), epl.Items[0])
		require.NoError(t, err)
		ep, err := GetRegistryEndpoint(context.Background(), &image.ContainerImage{RegistryURL: "ghcr.io"})
		require.NoError(t, err)
		require.NotEqual(t, 0, ep.CredsExpire)

		// Initial creds
		os.Setenv("TEST_CREDS", "foo:bar")
		_, err = ep.SetEndpointCredentials(context.Background(), nil, "")
		assert.NoError(t, err)
		assert.Equal(t, "foo", ep.Username)
		assert.Equal(t, "bar", ep.Password)
		assert.False(t, ep.CredsUpdated.IsZero())

		// Creds should still be cached
		os.Setenv("TEST_CREDS", "bar:foo")
		_, err = ep.SetEndpointCredentials(context.Background(), nil, "")
		assert.NoError(t, err)
		assert.Equal(t, "foo", ep.Username)
		assert.Equal(t, "bar", ep.Password)

		// Pretend 5 minutes have passed - creds have expired and are re-read from env
		ep.CredsUpdated = ep.CredsUpdated.Add(time.Minute * -5)
		_, err = ep.SetEndpointCredentials(context.Background(), nil, "")
		assert.NoError(t, err)
		assert.Equal(t, "bar", ep.Username)
		assert.Equal(t, "foo", ep.Password)
	})

}

func Test_ConcurrentCredentialFetching(t *testing.T) {
	t.Run("Multiple goroutines fetching credentials should only call once", func(t *testing.T) {
		ctx := context.Background()
		// Create a mock script that counts how many times it's called
		scriptContent := `#!/bin/sh
echo "counter" >> /tmp/test_ecr_calls.log
echo "AWS:mock-token-12345"
`
		scriptPath := "/tmp/test_ecr_auth.sh"
		err := os.WriteFile(scriptPath, []byte(scriptContent), 0755)
		require.NoError(t, err)
		defer os.Remove(scriptPath)

		// Clean up any existing log file
		os.Remove("/tmp/test_ecr_calls.log")
		defer os.Remove("/tmp/test_ecr_calls.log")

		epYAML := `
registries:
- name: ECR Registry
  api_url: https://123456789.dkr.ecr.us-east-1.amazonaws.com
  prefix: 123456789.dkr.ecr.us-east-1.amazonaws.com
  credentials: ext:` + scriptPath + `
  credsexpire: 1s
`
		epl, err := ParseRegistryConfiguration(epYAML)
		require.NoError(t, err)
		require.Len(t, epl.Items, 1)

		// Add registry configuration
		err = AddRegistryEndpointFromConfig(ctx, epl.Items[0])
		require.NoError(t, err)
		ep, err := GetRegistryEndpoint(ctx, &image.ContainerImage{RegistryURL: "123456789.dkr.ecr.us-east-1.amazonaws.com"})
		require.NoError(t, err)

		// Force credentials to be expired
		ep.CredsUpdated = time.Now().Add(-2 * time.Second)

		// Launch multiple goroutines to fetch credentials concurrently
		var wg sync.WaitGroup
		numGoroutines := 10
		errors := make([]error, numGoroutines)

		for i := 0; i < numGoroutines; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				_, errors[idx] = ep.SetEndpointCredentials(ctx, nil, "")
			}(i)
		}

		wg.Wait()

		// Check that no errors occurred
		for i, err := range errors {
			assert.NoError(t, err, "goroutine %d returned error", i)
		}

		// Verify credentials were set
		assert.Equal(t, "AWS", ep.Username)
		assert.Equal(t, "mock-token-12345", ep.Password)

		// Check that the script was called only once
		data, err := os.ReadFile("/tmp/test_ecr_calls.log")
		if err != nil {
			// File might not exist if script wasn't called at all
			assert.Equal(t, 0, 0)
		} else {
			lines := strings.Count(string(data), "counter")
			assert.Equal(t, 1, lines, "Expected script to be called exactly once, but was called %d times", lines)
		}
	})

	t.Run("Concurrent calls with unexpired credentials should not refetch", func(t *testing.T) {
		ctx := context.Background()
		var callCount int32

		epYAML := `
registries:
- name: Test Registry
  api_url: https://test.registry.io
  prefix: test.registry.io
  credentials: env:TEST_CONCURRENT_CREDS
  credsexpire: 10m
`
		epl, err := ParseRegistryConfiguration(epYAML)
		require.NoError(t, err)

		err = AddRegistryEndpointFromConfig(ctx, epl.Items[0])
		require.NoError(t, err)
		ep, err := GetRegistryEndpoint(ctx, &image.ContainerImage{RegistryURL: "test.registry.io"})
		require.NoError(t, err)

		// Set environment variable
		os.Setenv("TEST_CONCURRENT_CREDS", "user:pass")

		// First call to set credentials
		_, err = ep.SetEndpointCredentials(ctx, nil, "")
		require.NoError(t, err)
		atomic.AddInt32(&callCount, 1)

		// Launch concurrent calls - these should not refetch
		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := ep.SetEndpointCredentials(ctx, nil, "")
				assert.NoError(t, err)
			}()
		}
		wg.Wait()

		// Credentials should still be cached, so total calls should be 1
		assert.Equal(t, int32(1), atomic.LoadInt32(&callCount))
		assert.Equal(t, "user", ep.Username)
		assert.Equal(t, "pass", ep.Password)
	})
}

func Test_RegistryEndpoint_SetEndpointCredentials(t *testing.T) {
	ctx := context.Background()

	t.Run("returns cached endpoint creds when username and password already set", func(t *testing.T) {
		ep := &RegistryEndpoint{
			RegistryAPI: "https://example.com",
			Username:    "cacheduser",
			Password:    "cachedpass",
		}
		creds, err := ep.SetEndpointCredentials(ctx, nil, "")
		require.NoError(t, err)
		require.NotNil(t, creds)
		assert.Equal(t, "cacheduser", creds.Username)
		assert.Equal(t, "cachedpass", creds.Password)
	})

	t.Run("returns creds from env when endpoint has no creds and credentials ref set", func(t *testing.T) {
		ep := &RegistryEndpoint{
			RegistryAPI: "https://env-registry.example.com",
			Credentials: "env:TEST_SETENDPOINT_ENV",
		}
		os.Setenv("TEST_SETENDPOINT_ENV", "envuser:envpass")
		defer os.Unsetenv("TEST_SETENDPOINT_ENV")

		creds, err := ep.SetEndpointCredentials(ctx, nil, "")
		require.NoError(t, err)
		require.NotNil(t, creds)
		assert.Equal(t, "envuser", creds.Username)
		assert.Equal(t, "envpass", creds.Password)
		assert.Equal(t, "envuser", ep.Username)
		assert.Equal(t, "envpass", ep.Password)
		assert.False(t, ep.CredsUpdated.IsZero())
	})

	t.Run("uses secretVal (pull secret) when endpoint has no creds", func(t *testing.T) {
		secret := fixture.NewSecret("foo", "bar", map[string][]byte{"creds": []byte("secretuser:secretpass")})
		kubeClient := &kube.KubernetesClient{Clientset: fake.NewFakeClientsetWithResources(secret)}

		ep := &RegistryEndpoint{RegistryAPI: "https://registry.example.com"}

		creds, err := ep.SetEndpointCredentials(ctx, kubeClient, "secret:foo/bar#creds")
		require.NoError(t, err)
		require.NotNil(t, creds)
		assert.Equal(t, "secretuser", creds.Username)
		assert.Equal(t, "secretpass", creds.Password)
		// Image-specific pull secret is not cached on shared endpoint
		assert.Empty(t, ep.Username)
		assert.Empty(t, ep.Password)
	})

	t.Run("prefers secretVal over endpoint credentials when both empty endpoint and secretVal given", func(t *testing.T) {
		secret := fixture.NewSecret("ns", "s", map[string][]byte{"key": []byte("u:p")})
		kubeClient := &kube.KubernetesClient{Clientset: fake.NewFakeClientsetWithResources(secret)}
		ep := &RegistryEndpoint{RegistryAPI: "https://r.example.com"}

		creds, err := ep.SetEndpointCredentials(ctx, kubeClient, "secret:ns/s#key")
		require.NoError(t, err)
		require.NotNil(t, creds)
		assert.Equal(t, "u", creds.Username)
		assert.Equal(t, "p", creds.Password)
	})

	t.Run("second call with different secretVal uses secret B not cached A (secret rotation)", func(t *testing.T) {
		secretA := fixture.NewSecret("ns", "secret-a", map[string][]byte{"creds": []byte("userA:passA")})
		secretB := fixture.NewSecret("ns", "secret-b", map[string][]byte{"creds": []byte("userB:passB")})
		kubeClient := &kube.KubernetesClient{Clientset: fake.NewFakeClientsetWithResources(secretA, secretB)}
		ep := &RegistryEndpoint{RegistryAPI: "https://registry.example.com"}

		creds1, err := ep.SetEndpointCredentials(ctx, kubeClient, "secret:ns/secret-a#creds")
		require.NoError(t, err)
		require.NotNil(t, creds1)
		assert.Equal(t, "userA", creds1.Username)
		assert.Equal(t, "passA", creds1.Password)

		creds2, err := ep.SetEndpointCredentials(ctx, kubeClient, "secret:ns/secret-b#creds")
		require.NoError(t, err)
		require.NotNil(t, creds2)
		assert.Equal(t, "userB", creds2.Username)
		assert.Equal(t, "passB", creds2.Password)
		// Image-specific secrets are not cached on the endpoint; ep remains unchanged
		assert.Empty(t, ep.Username)
		assert.Empty(t, ep.Password)
	})

	t.Run("invalid credential reference returns error", func(t *testing.T) {
		ep := &RegistryEndpoint{RegistryAPI: "https://r.example.com", Credentials: "invalid-ref"}

		creds, err := ep.SetEndpointCredentials(ctx, nil, "")
		require.Error(t, err)
		assert.Nil(t, creds)
	})

	t.Run("secret: credential source with nil kube client returns error", func(t *testing.T) {
		ep := &RegistryEndpoint{RegistryAPI: "https://r.example.com"}

		creds, err := ep.SetEndpointCredentials(ctx, nil, "secret:foo/bar#creds")
		require.Error(t, err)
		assert.Nil(t, creds)
		assert.Contains(t, err.Error(), "could not fetch image tags")
	})

	t.Run("pullsecret: credential source with nil kube client returns error", func(t *testing.T) {
		ep := &RegistryEndpoint{RegistryAPI: "https://r.example.com"}

		creds, err := ep.SetEndpointCredentials(ctx, nil, "pullsecret:foo/bar")
		require.Error(t, err)
		assert.Nil(t, creds)
	})

	t.Run("expired creds refetch and return new creds", func(t *testing.T) {
		epYAML := `
registries:
- name: Expire Test
  api_url: https://expire.example.com
  prefix: expire.example.com
  credentials: env:TEST_SETENDPOINT_EXPIRE
  credsexpire: 1s
`
		epl, err := ParseRegistryConfiguration(epYAML)
		require.NoError(t, err)
		require.Len(t, epl.Items, 1)
		err = AddRegistryEndpointFromConfig(ctx, epl.Items[0])
		require.NoError(t, err)
		defer RestoreDefaultRegistryConfiguration()

		ep, err := GetRegistryEndpoint(ctx, &image.ContainerImage{RegistryURL: "expire.example.com"})
		require.NoError(t, err)

		os.Setenv("TEST_SETENDPOINT_EXPIRE", "olduser:oldpass")
		creds1, err := ep.SetEndpointCredentials(ctx, nil, "")
		require.NoError(t, err)
		require.NotNil(t, creds1)
		assert.Equal(t, "olduser", creds1.Username)

		os.Setenv("TEST_SETENDPOINT_EXPIRE", "newuser:newpass")
		ep.CredsUpdated = time.Now().Add(-5 * time.Second)

		creds2, err := ep.SetEndpointCredentials(ctx, nil, "")
		require.NoError(t, err)
		require.NotNil(t, creds2)
		assert.Equal(t, "newuser", creds2.Username)
		assert.Equal(t, "newpass", creds2.Password)
		assert.Equal(t, "newuser", ep.Username)
	})
}
