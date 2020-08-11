package registry

import (
	"testing"

	"github.com/argoproj-labs/argocd-image-updater/pkg/image"
	"github.com/argoproj-labs/argocd-image-updater/pkg/registry/mocks"

	"github.com/docker/distribution/manifest/schema1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func Test_GetTags(t *testing.T) {
	t.Run("Check for correctly returned tags with semver sort", func(t *testing.T) {
		regClient := mocks.RegistryClient{}
		regClient.On("Tags", mock.Anything).Return([]string{"1.2.0", "1.2.1", "1.2.2"}, nil)
		ep, err := GetRegistryEndpoint("")
		require.NoError(t, err)
		img := image.NewFromIdentifier("foo/bar:1.2.0")
		tl, err := ep.GetTags(img, &regClient, &image.VersionConstraint{SortMode: image.VersionSortSemVer})
		require.NoError(t, err)
		assert.NotEmpty(t, tl)
		tag, err := ep.Cache.GetTag("foo/bar", "1.2.1")
		require.NoError(t, err)
		assert.Nil(t, tag)
	})
	t.Run("Check for correctly returned tags with name sort", func(t *testing.T) {
		regClient := mocks.RegistryClient{}
		regClient.On("Tags", mock.Anything).Return([]string{"1.2.0", "1.2.1", "1.2.2"}, nil)
		ep, err := GetRegistryEndpoint("")
		require.NoError(t, err)
		img := image.NewFromIdentifier("foo/bar:1.2.0")
		tl, err := ep.GetTags(img, &regClient, &image.VersionConstraint{SortMode: image.VersionSortName})
		require.NoError(t, err)
		assert.NotEmpty(t, tl)
		tag, err := ep.Cache.GetTag("foo/bar", "1.2.1")
		require.NoError(t, err)
		assert.Nil(t, tag)
	})
	t.Run("Check for correctly returned tags with latest sort", func(t *testing.T) {
		ts := "2006-01-02T15:04:05.999999999Z"
		meta := &schema1.SignedManifest{
			Manifest: schema1.Manifest{
				History: []schema1.History{
					{
						V1Compatibility: `{"created":"` + ts + `"}`,
					},
				},
			},
		}
		regClient := mocks.RegistryClient{}
		regClient.On("Tags", mock.Anything).Return([]string{"1.2.0", "1.2.1", "1.2.2"}, nil)
		regClient.On("ManifestV1", mock.Anything, mock.Anything).Return(meta, nil)
		ep, err := GetRegistryEndpoint("")
		require.NoError(t, err)
		img := image.NewFromIdentifier("foo/bar:1.2.0")
		tl, err := ep.GetTags(img, &regClient, &image.VersionConstraint{SortMode: image.VersionSortLatest})
		require.NoError(t, err)
		assert.NotEmpty(t, tl)
		tag, err := ep.Cache.GetTag("foo/bar", "1.2.1")
		require.NoError(t, err)
		require.NotNil(t, tag)
		require.Equal(t, "1.2.1", tag.TagName)
	})
}
