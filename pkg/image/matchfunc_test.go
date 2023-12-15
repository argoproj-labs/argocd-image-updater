package image

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_MatchFuncAny(t *testing.T) {
	result, ok := MatchFuncAny("whatever", nil)
	assert.True(t, ok)
	assert.Equal(t, "whatever", result)
}

func Test_MatchFuncNone(t *testing.T) {
	_, ok := MatchFuncNone("whatever", nil)
	assert.False(t, ok)
}

func Test_MatchFuncRegexp(t *testing.T) {
	t.Run("Test with valid expression", func(t *testing.T) {
		re := regexp.MustCompile("[a-z]+")
		result, ok := MatchFuncRegexp("lemon", re)
		assert.True(t, ok)
		assert.Equal(t, "lemon", result)

		_, ok = MatchFuncRegexp("31337", re)
		assert.False(t, ok)
	})
	t.Run("Test with invalid type", func(t *testing.T) {
		_, ok := MatchFuncRegexp("lemon", "[a-z]+")
		assert.False(t, ok)
	})
}
