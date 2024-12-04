package cache

import (
	"testing"
	"time"

	memcache "github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/tag"
)

func Test_MemCache(t *testing.T) {
	imageName := "foo/bar"
	imageTag := "v1.0.0"
	t.Run("Cache hit", func(t *testing.T) {
		mc := NewMemCache()
		newTag := tag.NewImageTag(imageTag, time.Unix(0, 0), "")
		mc.SetTag(imageName, newTag)
		cachedTag, err := mc.GetTag(imageName, imageTag)
		require.NoError(t, err)
		require.NotNil(t, cachedTag)
		assert.Equal(t, imageTag, cachedTag.TagName)
		assert.True(t, mc.HasTag(imageName, imageTag))
		assert.Equal(t, 1, mc.NumEntries())
	})

	t.Run("Cache miss", func(t *testing.T) {
		mc := NewMemCache()
		newTag := tag.NewImageTag(imageTag, time.Unix(0, 0), "")
		mc.SetTag(imageName, newTag)
		assert.Equal(t, 1, mc.NumEntries())
		cachedTag, err := mc.GetTag(imageName, "v1.0.1")
		require.NoError(t, err)
		require.Nil(t, cachedTag)
		assert.False(t, mc.HasTag(imageName, "v1.0.1"))
	})

	t.Run("Cache clear", func(t *testing.T) {
		mc := NewMemCache()
		newTag := tag.NewImageTag(imageTag, time.Unix(0, 0), "")
		mc.SetTag(imageName, newTag)
		cachedTag, err := mc.GetTag(imageName, imageTag)
		require.NoError(t, err)
		require.NotNil(t, cachedTag)
		assert.Equal(t, imageTag, cachedTag.TagName)
		assert.True(t, mc.HasTag(imageName, imageTag))
		assert.Equal(t, 1, mc.NumEntries())
		mc.ClearCache()
		assert.Equal(t, 0, mc.NumEntries())
		cachedTag, err = mc.GetTag(imageName, imageTag)
		require.NoError(t, err)
		require.Nil(t, cachedTag)
	})
	t.Run("Image Cache Key", func(t *testing.T) {
		mc := MemCache{
			cache: memcache.New(0, 0),
		}
		application := "application1"
		key := imageCacheKey(imageName)
		mc.SetImage(imageName, application)
		app, b := mc.cache.Get(key)
		assert.True(t, b)
		assert.Equal(t, application, app)
		assert.Equal(t, 1, mc.NumEntries())
		mc.ClearCache()
		assert.Equal(t, 0, mc.NumEntries())
	})
}
