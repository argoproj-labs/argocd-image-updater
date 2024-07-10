package registry

import (
	"os"
	"testing"
	"time"

	"github.com/argoproj-labs/argocd-image-updater/pkg/image"
	"github.com/argoproj-labs/argocd-image-updater/pkg/options"
	"github.com/argoproj-labs/argocd-image-updater/pkg/registry/mocks"
	"github.com/argoproj-labs/argocd-image-updater/pkg/tag"

	"github.com/distribution/distribution/v3/manifest/schema1" //nolint:staticcheck
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_GetTags(t *testing.T) {

	t.Run("Check for correctly returned tags with semver sort", func(t *testing.T) {
		regClient := mocks.RegistryClient{}
		regClient.On("NewRepository", mock.Anything).Return(nil)
		regClient.On("Tags", mock.Anything).Return([]string{"1.2.0", "1.2.1", "1.2.2"}, nil)

		ep, err := GetRegistryEndpoint("")
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

		ep, err := GetRegistryEndpoint("")
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

		ep, err := GetRegistryEndpoint("")
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

		ep, err := GetRegistryEndpoint("")
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
		ep, err := GetRegistryEndpoint("ghcr.io")
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

func TestExpireCredentials(t *testing.T) {
	// Create a mock RegistryEndpoint instance
	ep := &RegistryEndpoint{
		Username:     "testuser",
		Password:     "testpassword",
		Credentials:  "mockcredentials",
		CredsExpire:  1 * time.Hour,                  // Example expiration time
		CredsUpdated: time.Now().Add(-2 * time.Hour), // Credentials updated more than expiry duration ago
	}

	// Test case where credentials should expire
	expired := ep.expireCredentials()
	if !expired {
		t.Error("Expected credentials to expire, but they did not")
	}

	// Verify that Username and Password have been cleared
	if ep.Username != "" || ep.Password != "" {
		t.Errorf("Expected Username and Password to be cleared, got Username: %s, Password: %s", ep.Username, ep.Password)
	}

	// Test case where credentials should not expire
	ep.Username = "testuser"
	ep.Password = "testpassword"
	ep.CredsUpdated = time.Now().Add(-30 * time.Minute) // Credentials updated within expiry duration
	expired = ep.expireCredentials()
	if expired {
		t.Error("Expected credentials not to expire, but they did")
	}

	// Verify that Username and Password have not been cleared
	if ep.Username == "" || ep.Password == "" {
		t.Error("Expected Username and Password not to be cleared, but they were")
	}

	// Test case where CredsExpire is not set
	ep.CredsExpire = 0
	expired = ep.expireCredentials()
	if expired {
		t.Error("Expected credentials not to expire when CredsExpire is not set, but they did")
	}

	// Test case where CredsUpdated is Zero
	ep.CredsExpire = 1 * time.Hour
	ep.CredsUpdated = time.Time{} // Zero value for time.Time
	expired = ep.expireCredentials()
	if expired {
		t.Error("Expected credentials not to expire when CredsUpdated is Zero, but they did")
	}

	// Test case where Credentials is empty
	ep.CredsUpdated = time.Now().Add(-2 * time.Hour)
	ep.Credentials = ""
	expired = ep.expireCredentials()
	if expired {
		t.Error("Expected credentials not to expire when Credentials is empty, but they did")
	}
}
