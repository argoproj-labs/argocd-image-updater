package image

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTransformFuncNone(t *testing.T) {
	t.Run("always returns the string passed in", func(t *testing.T) {
		result, err := SemVerTransformFuncNone("1.2.3")
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.Equal(t, "1.2.3", result.String())
	})

	t.Run("fail on malformed semver", func(t *testing.T) {
		result, err := SemVerTransformFuncNone("blah")
		require.Error(t, err)
		require.Nil(t, result)
	})
}

func TestTransformFuncRegexpFactory(t *testing.T) {
	pattern := regexp.MustCompile(`\d+\.\d+`)
	match := SemVerTransformerFuncRegexpFactory(pattern)

	t.Run("returns part of tag that matches", func(t *testing.T) {
		result, err := match("version-1.22+abcdef")
		require.NoError(t, err)
		assert.Equal(t, "1.22", result.Original())
	})

	t.Run("returns empty string if nothing matches", func(t *testing.T) {
		result, err := match("abc123blah")
		require.Error(t, err)
		require.Nil(t, result)
	})
}
