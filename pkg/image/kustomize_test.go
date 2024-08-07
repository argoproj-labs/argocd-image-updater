package image

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
	for _, image := range []string{"a/b:2", "x/y=foo.bar"} {
		assert.True(t, images.Find(KustomizeImage(image)) >= 0)
	}
	for _, image := range []string{"a/b", "x", "x/y"} {
		assert.Equal(t, -1, images.Find(KustomizeImage(image)))
	}
}
