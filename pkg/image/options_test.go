package image

import (
	"fmt"
	"regexp"
	"runtime"
	"testing"

	"github.com/argoproj-labs/argocd-image-updater/pkg/common"
	"github.com/argoproj-labs/argocd-image-updater/pkg/options"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_GetHelmOptions(t *testing.T) {
	t.Run("Get Helm parameter for configured application", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.HelmParamImageNameAnnotation, "dummy"): "release.name",
			fmt.Sprintf(common.HelmParamImageTagAnnotation, "dummy"):  "release.tag",
			fmt.Sprintf(common.HelmParamImageSpecAnnotation, "dummy"): "release.image",
		}

		img := NewFromIdentifier("dummy=foo/bar:1.12")
		paramName := img.GetParameterHelmImageName(annotations)
		paramTag := img.GetParameterHelmImageTag(annotations)
		paramSpec := img.GetParameterHelmImageSpec(annotations)
		assert.Equal(t, "release.name", paramName)
		assert.Equal(t, "release.tag", paramTag)
		assert.Equal(t, "release.image", paramSpec)
	})

	t.Run("Get Helm parameter for non-configured application", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.HelmParamImageNameAnnotation, "dummy"): "release.name",
			fmt.Sprintf(common.HelmParamImageTagAnnotation, "dummy"):  "release.tag",
			fmt.Sprintf(common.HelmParamImageSpecAnnotation, "dummy"): "release.image",
		}

		img := NewFromIdentifier("foo=foo/bar:1.12")
		paramName := img.GetParameterHelmImageName(annotations)
		paramTag := img.GetParameterHelmImageTag(annotations)
		paramSpec := img.GetParameterHelmImageSpec(annotations)
		assert.Equal(t, "", paramName)
		assert.Equal(t, "", paramTag)
		assert.Equal(t, "", paramSpec)
	})

	t.Run("Get Helm parameter for configured application with normalized name", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.HelmParamImageNameAnnotation, "foo_dummy"): "release.name",
			fmt.Sprintf(common.HelmParamImageTagAnnotation, "foo_dummy"):  "release.tag",
			fmt.Sprintf(common.HelmParamImageSpecAnnotation, "foo_dummy"): "release.image",
		}

		img := NewFromIdentifier("foo/dummy=foo/bar:1.12")
		paramName := img.GetParameterHelmImageName(annotations)
		paramTag := img.GetParameterHelmImageTag(annotations)
		paramSpec := img.GetParameterHelmImageSpec(annotations)
		assert.Equal(t, "release.name", paramName)
		assert.Equal(t, "release.tag", paramTag)
		assert.Equal(t, "release.image", paramSpec)
	})
}

func Test_GetKustomizeOptions(t *testing.T) {
	t.Run("Get Helm parameter for configured application", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.KustomizeApplicationNameAnnotation, "dummy"): "argoproj/argo-cd",
		}

		img := NewFromIdentifier("dummy=foo/bar:1.12")
		paramName := img.GetParameterKustomizeImageName(annotations)
		assert.Equal(t, "argoproj/argo-cd", paramName)
	})
}

func Test_GetSortOption(t *testing.T) {

	t.Run("Get update strategy semver for configured application", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.UpdateStrategyAnnotation, "dummy"): "semver",
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		sortMode := img.GetParameterUpdateStrategy(annotations)
		assert.Equal(t, StrategySemVer, sortMode)
	})

	t.Run("Get update strategy date for configured application", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.UpdateStrategyAnnotation, "dummy"): "latest",
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		sortMode := img.GetParameterUpdateStrategy(annotations)
		assert.Equal(t, StrategyLatest, sortMode)
	})

	t.Run("Get update strategy name for configured application", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.UpdateStrategyAnnotation, "dummy"): "name",
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		sortMode := img.GetParameterUpdateStrategy(annotations)
		assert.Equal(t, StrategyName, sortMode)
	})

	t.Run("Get update strategy option configured application because of invalid option", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.UpdateStrategyAnnotation, "dummy"): "invalid",
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		sortMode := img.GetParameterUpdateStrategy(annotations)
		assert.Equal(t, StrategySemVer, sortMode)
	})

	t.Run("Get update strategy option configured application because of option not set", func(t *testing.T) {
		annotations := map[string]string{}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		sortMode := img.GetParameterUpdateStrategy(annotations)
		assert.Equal(t, StrategySemVer, sortMode)
	})
}

func Test_GetMatchOption(t *testing.T) {

	t.Run("Get regexp match option for configured application", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.AllowTagsOptionAnnotation, "dummy"): "regexp:a-z",
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		matchFunc, matchArgs := img.GetParameterMatch(annotations)
		require.NotNil(t, matchFunc)
		require.NotNil(t, matchArgs)
		assert.IsType(t, &regexp.Regexp{}, matchArgs)
	})

	t.Run("Get regexp match option for configured application with invalid expression", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.AllowTagsOptionAnnotation, "dummy"): `regexp:/foo\`,
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		matchFunc, matchArgs := img.GetParameterMatch(annotations)
		require.NotNil(t, matchFunc)
		require.Nil(t, matchArgs)
	})

	t.Run("Get invalid match option for configured application", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.AllowTagsOptionAnnotation, "dummy"): "invalid",
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		matchFunc, matchArgs := img.GetParameterMatch(annotations)
		require.NotNil(t, matchFunc)
		require.Equal(t, false, matchFunc("", nil))
		assert.Nil(t, matchArgs)
	})

}

func Test_GetSecretOption(t *testing.T) {
	t.Run("Get cred source from annotation", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.SecretListAnnotation, "dummy"): "pullsecret:foo/bar",
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		credSrc := img.GetParameterPullSecret(annotations)
		require.NotNil(t, credSrc)
		assert.Equal(t, CredentialSourcePullSecret, credSrc.Type)
		assert.Equal(t, "foo", credSrc.SecretNamespace)
		assert.Equal(t, "bar", credSrc.SecretName)
		assert.Equal(t, ".dockerconfigjson", credSrc.SecretField)
	})

	t.Run("Invalid reference in annotation", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.SecretListAnnotation, "dummy"): "foo/bar",
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		credSrc := img.GetParameterPullSecret(annotations)
		require.Nil(t, credSrc)
	})
}

func Test_GetIgnoreTags(t *testing.T) {
	t.Run("Get list of tags to ignore from annotation", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.IgnoreTagsOptionAnnotation, "dummy"): "tag1, ,tag2,  tag3  , tag4",
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		tags := img.GetParameterIgnoreTags(annotations)
		require.Len(t, tags, 4)
		assert.Equal(t, "tag1", tags[0])
		assert.Equal(t, "tag2", tags[1])
		assert.Equal(t, "tag3", tags[2])
		assert.Equal(t, "tag4", tags[3])
	})
}

func Test_GetPlatformOptions(t *testing.T) {
	t.Run("Empty platform options with restriction", func(t *testing.T) {
		annotations := map[string]string{}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		opts := img.GetPlatformOptions(annotations, false)
		os := runtime.GOOS
		arch := runtime.GOARCH
		assert.True(t, opts.WantsPlatform(os, arch, ""))
		assert.False(t, opts.WantsPlatform(os, arch, "invalid"))
	})
	t.Run("Empty platform options without restriction", func(t *testing.T) {
		annotations := map[string]string{}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		opts := img.GetPlatformOptions(annotations, true)
		os := runtime.GOOS
		arch := runtime.GOARCH
		assert.True(t, opts.WantsPlatform(os, arch, ""))
		assert.True(t, opts.WantsPlatform(os, arch, "invalid"))
		assert.True(t, opts.WantsPlatform("windows", "amd64", ""))
	})
	t.Run("Single platform without variant requested", func(t *testing.T) {
		os := "linux"
		arch := "arm64"
		variant := ""
		annotations := map[string]string{
			fmt.Sprintf(common.PlatformsAnnotation, "dummy"): options.PlatformKey(os, arch, variant),
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		opts := img.GetPlatformOptions(annotations, false)
		assert.True(t, opts.WantsPlatform(os, arch, variant))
		assert.False(t, opts.WantsPlatform(os, arch, "invalid"))
	})
	t.Run("Single platform with variant requested", func(t *testing.T) {
		os := "linux"
		arch := "arm"
		variant := "v6"
		annotations := map[string]string{
			fmt.Sprintf(common.PlatformsAnnotation, "dummy"): options.PlatformKey(os, arch, variant),
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		opts := img.GetPlatformOptions(annotations, false)
		assert.True(t, opts.WantsPlatform(os, arch, variant))
		assert.False(t, opts.WantsPlatform(os, arch, ""))
		assert.False(t, opts.WantsPlatform(runtime.GOOS, runtime.GOARCH, ""))
		assert.False(t, opts.WantsPlatform(runtime.GOOS, runtime.GOARCH, variant))
	})
	t.Run("Multiple platforms requested", func(t *testing.T) {
		os := "linux"
		arch := "arm"
		variant := "v6"
		annotations := map[string]string{
			fmt.Sprintf(common.PlatformsAnnotation, "dummy"): options.PlatformKey(os, arch, variant) + ", " + options.PlatformKey(runtime.GOOS, runtime.GOARCH, ""),
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		opts := img.GetPlatformOptions(annotations, false)
		assert.True(t, opts.WantsPlatform(os, arch, variant))
		assert.True(t, opts.WantsPlatform(runtime.GOOS, runtime.GOARCH, ""))
		assert.False(t, opts.WantsPlatform(os, arch, ""))
		assert.False(t, opts.WantsPlatform(runtime.GOOS, runtime.GOARCH, variant))
	})
	t.Run("Invalid platform requested", func(t *testing.T) {
		os := "linux"
		arch := "arm"
		variant := "v6"
		annotations := map[string]string{
			fmt.Sprintf(common.PlatformsAnnotation, "dummy"): "invalid",
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		opts := img.GetPlatformOptions(annotations, false)
		assert.False(t, opts.WantsPlatform(os, arch, variant))
		assert.False(t, opts.WantsPlatform(runtime.GOOS, runtime.GOARCH, ""))
		assert.False(t, opts.WantsPlatform(os, arch, ""))
		assert.False(t, opts.WantsPlatform(runtime.GOOS, runtime.GOARCH, variant))
	})
}
