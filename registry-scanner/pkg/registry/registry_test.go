package registry

import (
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/image"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/options"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/registry/mocks"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/tag"

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

		ep, err := GetRegistryEndpoint(&image.ContainerImage{RegistryURL: ""})
		require.NoError(t, err)

		img := image.NewFromIdentifier("foo/bar:1.2.0")

		tl, err := ep.GetTags(img, &regClient, &image.VersionConstraint{Strategy: image.StrategySemVer, Options: options.NewManifestOptions()})
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

		ep, err := GetRegistryEndpoint(&image.ContainerImage{RegistryURL: ""})
		require.NoError(t, err)

		img := image.NewFromIdentifier("foo/bar:1.2.0")

		tl, err := ep.GetTags(img, &regClient, &image.VersionConstraint{
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

		ep, err := GetRegistryEndpoint(&image.ContainerImage{RegistryURL: ""})
		require.NoError(t, err)

		img := image.NewFromIdentifier("foo/bar:1.2.0")

		tl, err := ep.GetTags(img, &regClient, &image.VersionConstraint{Strategy: image.StrategyAlphabetical, Options: options.NewManifestOptions()})
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

		regClient := mocks.RegistryClient{}
		regClient.On("NewRepository", mock.Anything).Return(nil)
		regClient.On("Tags", mock.Anything).Return([]string{"1.2.0", "1.2.1", "1.2.2"}, nil)
		regClient.On("ManifestForTag", mock.Anything, mock.Anything).Return(meta1, nil)
		regClient.On("TagMetadata", mock.Anything, mock.Anything).Return(&tag.TagInfo{}, nil)

		ep, err := GetRegistryEndpoint(&image.ContainerImage{RegistryURL: ""})
		require.NoError(t, err)
		ep.Cache.ClearCache()

		img := image.NewFromIdentifier("foo/bar:1.2.0")
		tl, err := ep.GetTags(img, &regClient, &image.VersionConstraint{Strategy: image.StrategyNewestBuild, Options: options.NewManifestOptions()})
		require.NoError(t, err)
		assert.NotEmpty(t, tl)

		tag, err := ep.Cache.GetTag("foo/bar", "1.2.1")
		require.NoError(t, err)
		require.NotNil(t, tag)
		require.Equal(t, "1.2.1", tag.TagName)
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
		err = AddRegistryEndpointFromConfig(epl.Items[0])
		require.NoError(t, err)
		ep, err := GetRegistryEndpoint(&image.ContainerImage{RegistryURL: "ghcr.io"})
		require.NoError(t, err)
		require.NotEqual(t, 0, ep.CredsExpire)

		// Initial creds
		os.Setenv("TEST_CREDS", "foo:bar")
		err = ep.SetEndpointCredentials(nil)
		assert.NoError(t, err)
		assert.Equal(t, "foo", ep.Username)
		assert.Equal(t, "bar", ep.Password)
		assert.False(t, ep.CredsUpdated.IsZero())

		// Creds should still be cached
		os.Setenv("TEST_CREDS", "bar:foo")
		err = ep.SetEndpointCredentials(nil)
		assert.NoError(t, err)
		assert.Equal(t, "foo", ep.Username)
		assert.Equal(t, "bar", ep.Password)

		// Pretend 5 minutes have passed - creds have expired and are re-read from env
		ep.CredsUpdated = ep.CredsUpdated.Add(time.Minute * -5)
		err = ep.SetEndpointCredentials(nil)
		assert.NoError(t, err)
		assert.Equal(t, "bar", ep.Username)
		assert.Equal(t, "foo", ep.Password)
	})

}

func Test_ConcurrentCredentialFetching(t *testing.T) {
	t.Run("Multiple goroutines fetching credentials should only call once", func(t *testing.T) {
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
		err = AddRegistryEndpointFromConfig(epl.Items[0])
		require.NoError(t, err)
		ep, err := GetRegistryEndpoint(&image.ContainerImage{RegistryURL: "123456789.dkr.ecr.us-east-1.amazonaws.com"})
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
				errors[idx] = ep.SetEndpointCredentials(nil)
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
		
		err = AddRegistryEndpointFromConfig(epl.Items[0])
		require.NoError(t, err)
		ep, err := GetRegistryEndpoint(&image.ContainerImage{RegistryURL: "test.registry.io"})
		require.NoError(t, err)

		// Set environment variable
		os.Setenv("TEST_CONCURRENT_CREDS", "user:pass")
		
		// First call to set credentials
		err = ep.SetEndpointCredentials(nil)
		require.NoError(t, err)
		atomic.AddInt32(&callCount, 1)

		// Launch concurrent calls - these should not refetch
		var wg sync.WaitGroup
		for i := 0; i < 10; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				err := ep.SetEndpointCredentials(nil)
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
