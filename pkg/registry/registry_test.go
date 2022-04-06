package registry

import (
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/argoproj-labs/argocd-image-updater/pkg/image"
	"github.com/argoproj-labs/argocd-image-updater/pkg/options"
	"github.com/argoproj-labs/argocd-image-updater/pkg/registry/mocks"
	"github.com/argoproj-labs/argocd-image-updater/pkg/tag"

	"github.com/distribution/distribution/v3/manifest/schema1"
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

		tl, err := ep.GetTags(img, &regClient, &image.VersionConstraint{Strategy: image.StrategyName, Options: options.NewManifestOptions()})
		require.NoError(t, err)
		assert.NotEmpty(t, tl)

		tag, err := ep.Cache.GetTag("foo/bar", "1.2.1")
		require.NoError(t, err)
		assert.Nil(t, tag)
	})

	t.Run("Check for correctly returned tags with latest sort", func(t *testing.T) {
		ts := "2006-01-02T15:04:05.999999999Z"
		meta1 := &schema1.SignedManifest{
			Manifest: schema1.Manifest{
				History: []schema1.History{
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
		tl, err := ep.GetTags(img, &regClient, &image.VersionConstraint{Strategy: image.StrategyLatest, Options: options.NewManifestOptions()})
		require.NoError(t, err)
		assert.NotEmpty(t, tl)

		tag, err := ep.Cache.GetTag("foo/bar", "1.2.1")
		require.NoError(t, err)
		require.NotNil(t, tag)
		require.Equal(t, "1.2.1", tag.TagName)
	})

	t.Run("Check for correctly returned tags with transform", func(t *testing.T) {
		ts := "2006-01-02T15:04:05.999999999Z"
		meta1 := &schema1.SignedManifest{
			Manifest: schema1.Manifest{
				History: []schema1.History{
					{
						V1Compatibility: `{"created":"` + ts + `"}`,
					},
				},
			},
		}

		img := image.NewFromIdentifier("foo/bar:1.24.0.4930-ab6e1a058")

		ep, err := GetRegistryEndpoint("")
		require.NoError(t, err)

		pattern := regexp.MustCompile(`\d+\.\d+\.\d+`)

		vc := image.VersionConstraint{
			Strategy:            image.StrategyLatest,
			Options:             options.NewManifestOptions(),
			SemVerTransformFunc: image.SemVerTransformerFuncRegexpFactory(pattern),
		}

		regClient := mocks.RegistryClient{}
		regClient.On("NewRepository", mock.Anything).Return(nil)
		regClient.On("Tags", mock.Anything).Return([]string{"latest", "1.24.2.4973-2b1b51db9", "1.25.0.5282-2edd3c44d", "1.25.3.5409-f11334058"}, nil)
		regClient.On("ManifestForTag", mock.Anything, mock.Anything).Return(meta1, nil)
		regClient.On("TagMetadata", mock.Anything, mock.Anything).Return(&tag.TagInfo{}, nil)

		tl, err := ep.GetTags(img, &regClient, &vc)
		require.NotEmpty(t, tl)
		require.Len(t, tl.Tags(), 3)
		sorted := tl.SortBySemVer()
		require.Equal(t, sorted.Len(), 3)
		assert.Equal(t, "1.24.2", sorted[0].TagVersion.String())
		assert.Equal(t, "1.24.2.4973-2b1b51db9", sorted[0].TagName)
		assert.Equal(t, "1.25.0", sorted[1].TagVersion.String())
		assert.Equal(t, "1.25.0.5282-2edd3c44d", sorted[1].TagName)
		assert.Equal(t, "1.25.3", sorted[2].TagVersion.String())
		assert.Equal(t, "1.25.3.5409-f11334058", sorted[2].TagName)

		tag, err := ep.Cache.GetTag("foo/bar", "1.25.3.5409-f11334058")
		require.NoError(t, err)
		require.NotNil(t, tag)
		assert.Equal(t, "1.25.3.5409-f11334058", tag.TagName)
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
