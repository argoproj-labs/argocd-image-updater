package image

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestKustomizeImage_Match(t *testing.T) {
	// no prefix
	assert.False(t, KustomizeImage("foo=1").Match("bar=1"))
	// mismatched delimiter
	assert.False(t, KustomizeImage("foo=1").Match("bar:1"))
	assert.False(t, KustomizeImage("foo:1").Match("bar=1"))
	assert.False(t, KustomizeImage("foobar:2").Match("foo:2"))
	assert.False(t, KustomizeImage("foobar@2").Match("foo@2"))
	// matches
	assert.True(t, KustomizeImage("foo=1").Match("foo=2"))
	assert.True(t, KustomizeImage("foo:1").Match("foo:2"))
	assert.True(t, KustomizeImage("foo@1").Match("foo@2"))
	assert.True(t, KustomizeImage("nginx").Match("nginx"))
}

func Test_KustomizeImages_Find(t *testing.T) {
	images := KustomizeImages{
		"a/b:1.0",
		"a/b@sha256:aabb",
		"a/b:latest@sha256:aabb",
		"x/y=busybox",
		"x/y=foo.bar/a/c:0.23",
	}
	for _, image := range images {
		assert.True(t, images.Find(image) >= 0)
	}
	for _, image := range []string{"a/b", "a/b:2", "x/y=foo.bar"} {
		assert.True(t, images.Find(KustomizeImage(image)) >= 0)
	}
	for _, image := range []string{"x", "x/y"} {
		assert.Equal(t, -1, images.Find(KustomizeImage(image)))
	}
}
