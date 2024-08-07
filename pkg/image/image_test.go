package image

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/exp/slices"

	"github.com/argoproj-labs/argocd-image-updater/pkg/tag"
)

func Test_ParseImageTags(t *testing.T) {
	t.Run("Parse valid image name without registry info", func(t *testing.T) {
		image := NewFromIdentifier("jannfis/test-image:0.1")
		assert.Empty(t, image.RegistryURL)
		assert.Empty(t, image.ImageAlias)
		assert.Equal(t, "jannfis/test-image", image.ImageName)
		require.NotNil(t, image.ImageTag)
		assert.Equal(t, "0.1", image.ImageTag.TagName)
		assert.Equal(t, "jannfis/test-image:0.1", image.GetFullNameWithTag())
		assert.Equal(t, "jannfis/test-image", image.GetFullNameWithoutTag())
	})

	t.Run("Single element image name is unmodified", func(t *testing.T) {
		image := NewFromIdentifier("test-image")
		assert.Empty(t, image.RegistryURL)
		assert.Empty(t, image.ImageAlias)
		assert.Equal(t, "test-image", image.ImageName)
		require.Nil(t, image.ImageTag)
		assert.Equal(t, "test-image", image.GetFullNameWithTag())
		assert.Equal(t, "test-image", image.GetFullNameWithoutTag())
	})

	t.Run("library image name is unmodified", func(t *testing.T) {
		image := NewFromIdentifier("library/test-image")
		assert.Empty(t, image.RegistryURL)
		assert.Empty(t, image.ImageAlias)
		assert.Equal(t, "library/test-image", image.ImageName)
		require.Nil(t, image.ImageTag)
		assert.Equal(t, "library/test-image", image.GetFullNameWithTag())
		assert.Equal(t, "library/test-image", image.GetFullNameWithoutTag())
	})

	t.Run("Parse valid image name with registry info", func(t *testing.T) {
		image := NewFromIdentifier("gcr.io/jannfis/test-image:0.1")
		assert.Equal(t, "gcr.io", image.RegistryURL)
		assert.Empty(t, image.ImageAlias)
		assert.Equal(t, "jannfis/test-image", image.ImageName)
		require.NotNil(t, image.ImageTag)
		assert.Equal(t, "0.1", image.ImageTag.TagName)
		assert.Equal(t, "gcr.io/jannfis/test-image:0.1", image.GetFullNameWithTag())
		assert.Equal(t, "gcr.io/jannfis/test-image", image.GetFullNameWithoutTag())
	})

	t.Run("Parse valid image name with default registry info", func(t *testing.T) {
		image := NewFromIdentifier("docker.io/jannfis/test-image:0.1")
		assert.Equal(t, "docker.io", image.RegistryURL)
		assert.Empty(t, image.ImageAlias)
		assert.Equal(t, "jannfis/test-image", image.ImageName)
		require.NotNil(t, image.ImageTag)
		assert.Equal(t, "0.1", image.ImageTag.TagName)
		assert.Equal(t, "docker.io/jannfis/test-image:0.1", image.GetFullNameWithTag())
		assert.Equal(t, "docker.io/jannfis/test-image", image.GetFullNameWithoutTag())
	})

	t.Run("Parse valid image name with digest tag", func(t *testing.T) {
		image := NewFromIdentifier("gcr.io/jannfis/test-image@sha256:abcde")
		assert.Equal(t, "gcr.io", image.RegistryURL)
		assert.Empty(t, image.ImageAlias)
		assert.Equal(t, "jannfis/test-image", image.ImageName)
		require.NotNil(t, image.ImageTag)
		assert.Empty(t, image.ImageTag.TagName)
		assert.Equal(t, "sha256:abcde", image.ImageTag.TagDigest)
		assert.Equal(t, "latest@sha256:abcde", image.GetTagWithDigest())
		assert.Equal(t, "gcr.io/jannfis/test-image@sha256:abcde", image.GetFullNameWithTag())
		assert.Equal(t, "gcr.io/jannfis/test-image", image.GetFullNameWithoutTag())
	})

	t.Run("Parse valid image name with tag and digest", func(t *testing.T) {
		image := NewFromIdentifier("gcr.io/jannfis/test-image:test-tag@sha256:abcde")
		require.NotNil(t, image.ImageTag)
		assert.Empty(t, image.ImageTag.TagName)
		assert.Equal(t, "sha256:abcde", image.ImageTag.TagDigest)
		assert.Equal(t, "latest@sha256:abcde", image.GetTagWithDigest())
		assert.Equal(t, "gcr.io/jannfis/test-image:test-tag@sha256:abcde", image.GetFullNameWithTag())
	})

	t.Run("Parse valid image name with source name and registry info", func(t *testing.T) {
		image := NewFromIdentifier("jannfis/orig-image=gcr.io/jannfis/test-image:0.1")
		assert.Equal(t, "gcr.io", image.RegistryURL)
		assert.Equal(t, "jannfis/orig-image", image.ImageAlias)
		assert.Equal(t, "jannfis/test-image", image.ImageName)
		require.NotNil(t, image.ImageTag)
		assert.Equal(t, "0.1", image.ImageTag.TagName)
	})

	t.Run("Parse valid image name with source name and registry info with port", func(t *testing.T) {
		image := NewFromIdentifier("ghcr.io:4567/jannfis/orig-image=gcr.io:1234/jannfis/test-image:0.1")
		assert.Equal(t, "gcr.io:1234", image.RegistryURL)
		assert.Equal(t, "ghcr.io:4567/jannfis/orig-image", image.ImageAlias)
		assert.Equal(t, "jannfis/test-image", image.ImageName)
		require.NotNil(t, image.ImageTag)
		assert.Equal(t, "0.1", image.ImageTag.TagName)
	})

	t.Run("Parse image without version source name and registry info", func(t *testing.T) {
		image := NewFromIdentifier("jannfis/orig-image=gcr.io/jannfis/test-image")
		assert.Equal(t, "gcr.io", image.RegistryURL)
		assert.Equal(t, "jannfis/orig-image", image.ImageAlias)
		assert.Equal(t, "jannfis/test-image", image.ImageName)
		assert.Nil(t, image.ImageTag)
	})
	t.Run("#273 classic-web=registry:5000/classic-web", func(t *testing.T) {
		image := NewFromIdentifier("classic-web=registry:5000/classic-web")
		assert.Equal(t, "registry:5000", image.RegistryURL)
		assert.Equal(t, "classic-web", image.ImageAlias)
		assert.Equal(t, "classic-web", image.ImageName)
		assert.Nil(t, image.ImageTag)
	})
}

func Test_ImageToString(t *testing.T) {
	t.Run("Get string representation of full-qualified image name", func(t *testing.T) {
		imageName := "jannfis/argocd=jannfis/orig-image:0.1"
		img := NewFromIdentifier(imageName)
		assert.Equal(t, imageName, img.String())
	})
	t.Run("Get string representation of full-qualified image name with registry", func(t *testing.T) {
		imageName := "jannfis/argocd=gcr.io/jannfis/orig-image:0.1"
		img := NewFromIdentifier(imageName)
		assert.Equal(t, imageName, img.String())
	})
	t.Run("Get string representation of full-qualified image name with registry", func(t *testing.T) {
		imageName := "jannfis/argocd=gcr.io/jannfis/orig-image"
		img := NewFromIdentifier(imageName)
		assert.Equal(t, imageName, img.String())
	})
	t.Run("Get original value", func(t *testing.T) {
		imageName := "invalid==foo"
		img := NewFromIdentifier(imageName)
		assert.Equal(t, imageName, img.Original())
	})
}

func Test_WithTag(t *testing.T) {
	t.Run("Get string representation of full-qualified image name", func(t *testing.T) {
		imageName := "jannfis/argocd=jannfis/orig-image:0.1"
		nimageName := "jannfis/argocd=jannfis/orig-image:0.2"
		oImg := NewFromIdentifier(imageName)
		nImg := oImg.WithTag(tag.NewImageTag("0.2", time.Unix(0, 0), ""))
		assert.Equal(t, nimageName, nImg.String())
	})
}

func Test_ContainerList(t *testing.T) {
	t.Run("Test whether image is contained in list", func(t *testing.T) {
		images := make(ContainerImageList, 0)
		image_names := []string{"a/a:0.1", "a/b:1.2", "x/y=foo.bar/a/c:0.23"}
		for _, n := range image_names {
			images = append(images, NewFromIdentifier(n))
		}
		withKustomizeOverride := NewFromIdentifier("k1/k2:k3")
		withKustomizeOverride.KustomizeImage = images[0]
		images = append(images, withKustomizeOverride)

		assert.NotNil(t, images.ContainsImage(NewFromIdentifier(image_names[0]), false))
		assert.NotNil(t, images.ContainsImage(NewFromIdentifier(image_names[1]), false))
		assert.NotNil(t, images.ContainsImage(NewFromIdentifier(image_names[2]), false))
		assert.Nil(t, images.ContainsImage(NewFromIdentifier("foo/bar"), false))

		imageMatch := images.ContainsImage(withKustomizeOverride, false)
		assert.Equal(t, images[0], imageMatch)
	})
}

func Test_getImageDigestFromTag(t *testing.T) {
	tagAndDigest := "test-tag@sha256:abcde"
	tagName, tagDigest := getImageDigestFromTag(tagAndDigest)
	assert.Equal(t, "test-tag", tagName)
	assert.Equal(t, "sha256:abcde", tagDigest)

	tagAndDigest = "test-tag"
	tagName, tagDigest = getImageDigestFromTag(tagAndDigest)
	assert.Equal(t, "test-tag", tagName)
	assert.Empty(t, tagDigest)
}

func Test_ContainerImageList_String_Originals(t *testing.T) {
	images := make(ContainerImageList, 0)
	originals := []string{}

	assert.Equal(t, "", images.String())
	assert.True(t, slices.Equal(originals, images.Originals()))

	images = append(images, NewFromIdentifier("foo/bar:0.1"))
	originals = append(originals, "foo/bar:0.1")
	assert.Equal(t, "foo/bar:0.1", images.String())
	assert.True(t, slices.Equal(originals, images.Originals()))

	images = append(images, NewFromIdentifier("alias=foo/bar:0.2"))
	originals = append(originals, "alias=foo/bar:0.2")
	assert.Equal(t, "foo/bar:0.1,alias=foo/bar:0.2", images.String())
	assert.True(t, slices.Equal(originals, images.Originals()))
}

func TestContainerImage_DiffersFrom(t *testing.T) {
	foo1 := NewFromIdentifier("x/foo:1")
	foo2 := NewFromIdentifier("x/foo:2")
	bar1 := NewFromIdentifier("x/bar:1")
	bar1WithRegistry := NewFromIdentifier("docker.io/x/bar:1")

	assert.False(t, foo1.DiffersFrom(foo1, true))
	assert.False(t, foo1.DiffersFrom(foo2, false))
	assert.True(t, foo1.DiffersFrom(foo2, true))

	assert.True(t, foo1.DiffersFrom(bar1, false))
	assert.True(t, bar1.DiffersFrom(foo1, false))
	assert.True(t, foo1.DiffersFrom(bar1, true))
	assert.True(t, bar1.DiffersFrom(foo1, true))
	assert.True(t, bar1.DiffersFrom(bar1WithRegistry, false))

	assert.False(t, foo1.IsUpdatable("0.1", "^1.0"))
}
