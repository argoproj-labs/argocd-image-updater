package image

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_LatestVersion(t *testing.T) {
	t.Run("Find the latest version without any constraint", func(t *testing.T) {
		tagList := []string{"0.1", "0.5.1", "0.9", "1.0", "1.0.1", "1.1.2", "2.0.3"}
		img := NewFromIdentifier("jannfis/test:1.0")
		newTag, err := img.GetNewestVersionFromTags("", tagList)
		require.NoError(t, err)
		assert.Equal(t, "2.0.3", newTag)
	})

	t.Run("Find the latest version with a semver constraint on major", func(t *testing.T) {
		tagList := []string{"0.1", "0.5.1", "0.9", "1.0", "1.0.1", "1.1.2", "2.0.3"}
		img := NewFromIdentifier("jannfis/test:1.0")
		newTag, err := img.GetNewestVersionFromTags("^1.0", tagList)
		require.NoError(t, err)
		assert.Equal(t, "1.1.2", newTag)
	})

	t.Run("Find the latest version with a semver constraint on patch", func(t *testing.T) {
		tagList := []string{"0.1", "0.5.1", "0.9", "1.0", "1.0.1", "1.1.2", "2.0.3"}
		img := NewFromIdentifier("jannfis/test:1.0")
		newTag, err := img.GetNewestVersionFromTags("~1.0", tagList)
		require.NoError(t, err)
		assert.Equal(t, "1.0.1", newTag)
	})

	t.Run("Find the latest version with a semver constraint that has no match", func(t *testing.T) {
		tagList := []string{"0.1", "0.5.1", "0.9", "2.0.3"}
		img := NewFromIdentifier("jannfis/test:1.0")
		newTag, err := img.GetNewestVersionFromTags("~1.0", tagList)
		require.NoError(t, err)
		assert.Equal(t, "1.0", newTag)
	})

	t.Run("Find the latest version with a semver constraint that is invalid", func(t *testing.T) {
		tagList := []string{"0.1", "0.5.1", "0.9", "2.0.3"}
		img := NewFromIdentifier("jannfis/test:1.0")
		newTag, err := img.GetNewestVersionFromTags("latest", tagList)
		assert.Error(t, err)
		assert.Equal(t, "", newTag)
	})

	t.Run("Find the latest version with no tags", func(t *testing.T) {
		tagList := []string{}
		img := NewFromIdentifier("jannfis/test:1.0")
		newTag, err := img.GetNewestVersionFromTags("~1.0", tagList)
		require.NoError(t, err)
		assert.Equal(t, "1.0", newTag)
	})

}
