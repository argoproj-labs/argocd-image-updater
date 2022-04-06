package image

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_MatchFuncAny(t *testing.T) {
	assert.True(t, MatchFuncAny("whatever"))
}

func Test_MatchFuncNone(t *testing.T) {
	assert.False(t, MatchFuncNone("whatever"))
}

func Test_MatchFuncRegexp(t *testing.T) {
	t.Run("Test with valid expression", func(t *testing.T) {
		re := regexp.MustCompile("[a-z]+")
		assert.True(t, MatchFuncRegexpFactory(re)("lemon"))
		assert.False(t, MatchFuncRegexpFactory(re)("31337"))
	})
}
