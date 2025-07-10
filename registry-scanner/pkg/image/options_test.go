package image

import (
	"context"
	"fmt"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/common"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/options"
)

func Test_GetHelmOptions(t *testing.T) {
	t.Run("Get Helm parameter for configured application", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.Prefixed("", common.HelmParamImageNameAnnotationSuffix), "dummy"): "release.name",
			fmt.Sprintf(common.Prefixed("", common.HelmParamImageTagAnnotationSuffix), "dummy"):  "release.tag",
			fmt.Sprintf(common.Prefixed("", common.HelmParamImageSpecAnnotationSuffix), "dummy"): "release.image",
		}

		img := NewFromIdentifier("dummy=foo/bar:1.12")
		paramName := img.GetParameterHelmImageName(annotations, "")
		paramTag := img.GetParameterHelmImageTag(annotations, "")
		paramSpec := img.GetParameterHelmImageSpec(annotations, "")
		assert.Equal(t, "release.name", paramName)
		assert.Equal(t, "release.tag", paramTag)
		assert.Equal(t, "release.image", paramSpec)
	})

	t.Run("Get Helm parameter for non-configured application", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.Prefixed("", common.HelmParamImageNameAnnotationSuffix), "dummy"): "release.name",
			fmt.Sprintf(common.Prefixed("", common.HelmParamImageTagAnnotationSuffix), "dummy"):  "release.tag",
			fmt.Sprintf(common.Prefixed("", common.HelmParamImageSpecAnnotationSuffix), "dummy"): "release.image",
		}

		img := NewFromIdentifier("foo=foo/bar:1.12")
		paramName := img.GetParameterHelmImageName(annotations, "")
		paramTag := img.GetParameterHelmImageTag(annotations, "")
		paramSpec := img.GetParameterHelmImageSpec(annotations, "")
		assert.Equal(t, "", paramName)
		assert.Equal(t, "", paramTag)
		assert.Equal(t, "", paramSpec)
	})

	t.Run("Get Helm parameter for configured application with normalized name", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.Prefixed("", common.HelmParamImageNameAnnotationSuffix), "foo_dummy"): "release.name",
			fmt.Sprintf(common.Prefixed("", common.HelmParamImageTagAnnotationSuffix), "foo_dummy"):  "release.tag",
			fmt.Sprintf(common.Prefixed("", common.HelmParamImageSpecAnnotationSuffix), "foo_dummy"): "release.image",
		}

		img := NewFromIdentifier("foo/dummy=foo/bar:1.12")
		paramName := img.GetParameterHelmImageName(annotations, "")
		paramTag := img.GetParameterHelmImageTag(annotations, "")
		paramSpec := img.GetParameterHelmImageSpec(annotations, "")
		assert.Equal(t, "release.name", paramName)
		assert.Equal(t, "release.tag", paramTag)
		assert.Equal(t, "release.image", paramSpec)
	})
}

func Test_GetKustomizeOptions(t *testing.T) {
	t.Run("Get Kustomize parameter for configured application", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.Prefixed("", common.KustomizeApplicationNameAnnotationSuffix), "dummy"): "argoproj/argo-cd",
		}

		img := NewFromIdentifier("dummy=foo/bar:1.12")
		paramName := img.GetParameterKustomizeImageName(annotations, "")
		assert.Equal(t, "argoproj/argo-cd", paramName)

		img = NewFromIdentifier("dummy2=foo2/bar2:1.12")
		paramName = img.GetParameterKustomizeImageName(annotations, "")
		assert.Equal(t, "", paramName)
	})
}

func Test_GetSortOption(t *testing.T) {
	t.Run("Get update strategy semver for configured application", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.UpdateStrategyAnnotationSuffix, "dummy"): "semver",
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		sortMode := img.GetParameterUpdateStrategy(annotations, "")
		assert.Equal(t, StrategySemVer, sortMode)
	})

	t.Run("Use update strategy newest-build for configured application", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.UpdateStrategyAnnotationSuffix, "dummy"): "newest-build",
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		sortMode := img.GetParameterUpdateStrategy(annotations, "")
		assert.Equal(t, StrategyNewestBuild, sortMode)
	})

	t.Run("Get update strategy date for configured application", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.UpdateStrategyAnnotationSuffix, "dummy"): "latest",
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		sortMode := img.GetParameterUpdateStrategy(annotations, "")
		assert.Equal(t, StrategyNewestBuild, sortMode)
	})

	t.Run("Get update strategy name for configured application", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.UpdateStrategyAnnotationSuffix, "dummy"): "name",
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		sortMode := img.GetParameterUpdateStrategy(annotations, "")
		assert.Equal(t, StrategyAlphabetical, sortMode)
	})

	t.Run("Use update strategy alphabetical for configured application", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.UpdateStrategyAnnotationSuffix, "dummy"): "alphabetical",
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		sortMode := img.GetParameterUpdateStrategy(annotations, "")
		assert.Equal(t, StrategyAlphabetical, sortMode)
	})

	t.Run("Get update strategy option configured application because of invalid option", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.UpdateStrategyAnnotationSuffix, "dummy"): "invalid",
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		sortMode := img.GetParameterUpdateStrategy(annotations, "")
		assert.Equal(t, StrategySemVer, sortMode)
	})

	t.Run("Get update strategy option configured application because of option not set", func(t *testing.T) {
		annotations := map[string]string{}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		sortMode := img.GetParameterUpdateStrategy(annotations, "")
		assert.Equal(t, StrategySemVer, sortMode)
	})

	t.Run("Prefer update strategy option from image-specific annotation", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.UpdateStrategyAnnotationSuffix, "dummy"): "alphabetical",
			common.ApplicationWideUpdateStrategyAnnotationSuffix:        "newest-build",
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		sortMode := img.GetParameterUpdateStrategy(annotations, "")
		assert.Equal(t, StrategyAlphabetical, sortMode)
	})

	t.Run("Get update strategy option from application-wide annotation", func(t *testing.T) {
		annotations := map[string]string{
			common.ApplicationWideUpdateStrategyAnnotationSuffix: "newest-build",
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		sortMode := img.GetParameterUpdateStrategy(annotations, "")
		assert.Equal(t, StrategyNewestBuild, sortMode)
	})

	t.Run("Get update strategy option digest from application-wide annotation", func(t *testing.T) {
		annotations := map[string]string{
			common.ApplicationWideUpdateStrategyAnnotationSuffix: "digest",
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		sortMode := img.GetParameterUpdateStrategy(annotations, "")
		assert.Equal(t, StrategyDigest, sortMode)
	})
}

func Test_GetMatchOption(t *testing.T) {
	t.Run("Get regexp match option for configured application", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.AllowTagsOptionAnnotationSuffix, "dummy"): "regexp:a-z",
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		matchFunc, matchArgs := img.GetParameterMatch(annotations, "")
		require.NotNil(t, matchFunc)
		require.NotNil(t, matchArgs)
		assert.IsType(t, &regexp.Regexp{}, matchArgs)
	})

	t.Run("Get regexp match option for configured application with invalid expression", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.AllowTagsOptionAnnotationSuffix, "dummy"): `regexp:/foo\`,
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		matchFunc, matchArgs := img.GetParameterMatch(annotations, "")
		require.NotNil(t, matchFunc)
		require.Nil(t, matchArgs)
	})

	t.Run("Get invalid match option for configured application", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.AllowTagsOptionAnnotationSuffix, "dummy"): "invalid",
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		matchFunc, matchArgs := img.GetParameterMatch(annotations, "")
		require.NotNil(t, matchFunc)
		require.Equal(t, false, matchFunc("", nil))
		assert.Nil(t, matchArgs)
	})

	t.Run("No match option for configured application", func(t *testing.T) {
		annotations := map[string]string{}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		matchFunc, matchArgs := img.GetParameterMatch(annotations, "")
		require.NotNil(t, matchFunc)
		require.Equal(t, true, matchFunc("", nil))
		assert.Equal(t, "", matchArgs)
	})

	t.Run("Prefer match option from image-specific annotation", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.AllowTagsOptionAnnotationSuffix, "dummy"): "regexp:^[0-9]",
			common.ApplicationWideAllowTagsOptionAnnotationSuffix:        "regexp:^v",
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		matchFunc, matchArgs := img.GetParameterMatch(annotations, "")
		require.NotNil(t, matchFunc)
		require.NotNil(t, matchArgs)
		assert.IsType(t, &regexp.Regexp{}, matchArgs)
		assert.True(t, matchFunc("0.0.1", matchArgs))
		assert.False(t, matchFunc("v0.0.1", matchArgs))
	})

	t.Run("Get match option from application-wide annotation", func(t *testing.T) {
		annotations := map[string]string{
			common.ApplicationWideAllowTagsOptionAnnotationSuffix: "regexp:^v",
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		matchFunc, matchArgs := img.GetParameterMatch(annotations, "")
		require.NotNil(t, matchFunc)
		require.NotNil(t, matchArgs)
		assert.IsType(t, &regexp.Regexp{}, matchArgs)
		assert.False(t, matchFunc("0.0.1", matchArgs))
		assert.True(t, matchFunc("v0.0.1", matchArgs))
	})
}

func Test_GetIgnoreTags(t *testing.T) {
	t.Run("Get list of tags to ignore from image-specific annotation", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.IgnoreTagsOptionAnnotationSuffix, "dummy"): "tag1, ,tag2,  tag3  , tag4",
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		tags := img.GetParameterIgnoreTags(annotations, "")
		require.Len(t, tags, 4)
		assert.Equal(t, "tag1", tags[0])
		assert.Equal(t, "tag2", tags[1])
		assert.Equal(t, "tag3", tags[2])
		assert.Equal(t, "tag4", tags[3])
	})

	t.Run("No tags to ignore from image-specific annotation", func(t *testing.T) {
		annotations := map[string]string{}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		tags := img.GetParameterIgnoreTags(annotations, "")
		require.Nil(t, tags)
	})

	t.Run("Prefer list of tags to ignore from image-specific annotation", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.IgnoreTagsOptionAnnotationSuffix, "dummy"): "tag1, tag2",
			common.ApplicationWideIgnoreTagsOptionAnnotationSuffix:        "tag3, tag4",
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		tags := img.GetParameterIgnoreTags(annotations, "")
		require.Len(t, tags, 2)
		assert.Equal(t, "tag1", tags[0])
		assert.Equal(t, "tag2", tags[1])
	})

	t.Run("Get list of tags to ignore from application-wide annotation", func(t *testing.T) {
		annotations := map[string]string{
			common.ApplicationWideIgnoreTagsOptionAnnotationSuffix: "tag3, tag4",
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		tags := img.GetParameterIgnoreTags(annotations, "")
		require.Len(t, tags, 2)
		assert.Equal(t, "tag3", tags[0])
		assert.Equal(t, "tag4", tags[1])
	})
}

func Test_HasForceUpdateOptionAnnotation(t *testing.T) {
	t.Run("Get force-update option from image-specific annotation", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.ForceUpdateOptionAnnotationSuffix, "dummy"): "true",
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		forceUpdate := img.HasForceUpdateOptionAnnotation(annotations, "")
		assert.True(t, forceUpdate)
	})

	t.Run("Prefer force-update option from image-specific annotation", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.ForceUpdateOptionAnnotationSuffix, "dummy"): "true",
			common.ApplicationWideForceUpdateOptionAnnotationSuffix:        "false",
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		forceUpdate := img.HasForceUpdateOptionAnnotation(annotations, "")
		assert.True(t, forceUpdate)
	})

	t.Run("Get force-update option from application-wide annotation", func(t *testing.T) {
		annotations := map[string]string{
			common.ApplicationWideForceUpdateOptionAnnotationSuffix: "false",
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		forceUpdate := img.HasForceUpdateOptionAnnotation(annotations, "")
		assert.False(t, forceUpdate)
	})
}

func Test_GetPlatformOptions(t *testing.T) {
	t.Run("Empty platform options with restriction", func(t *testing.T) {
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		ctx := context.Background()
		opts := img.GetPlatformOptions(ctx, false, nil)
		os := runtime.GOOS
		arch := runtime.GOARCH
		platform := opts.Platforms()[0]
		slashCount := strings.Count(platform, "/")
		if slashCount == 1 {
			assert.True(t, opts.WantsPlatform(os, arch, ""))
			assert.True(t, opts.WantsPlatform(os, arch, "invalid"))
		} else if slashCount == 2 {
			assert.False(t, opts.WantsPlatform(os, arch, ""))
			assert.False(t, opts.WantsPlatform(os, arch, "invalid"))
		} else {
			t.Fatal("invalid platform options ", platform)
		}
	})
	t.Run("Empty platform options without restriction", func(t *testing.T) {
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		ctx := context.Background()
		opts := img.GetPlatformOptions(ctx, true, nil)
		os := runtime.GOOS
		arch := runtime.GOARCH
		assert.True(t, opts.WantsPlatform(os, arch, ""))
		assert.True(t, opts.WantsPlatform(os, arch, "invalid"))
		assert.True(t, opts.WantsPlatform("windows", "amd64", ""))
	})
	t.Run("Single platform without variant requested", func(t *testing.T) {
		os := "linux"
		arch := "arm64"
		variant := "v8"
		platforms := []string{options.PlatformKey(os, arch, variant)}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		ctx := context.Background()
		opts := img.GetPlatformOptions(ctx, false, platforms)
		assert.True(t, opts.WantsPlatform(os, arch, variant))
		assert.False(t, opts.WantsPlatform(os, arch, "invalid"))
	})
	t.Run("Single platform with variant requested", func(t *testing.T) {
		os := "linux"
		arch := "arm"
		variant := "v6"

		platforms := []string{options.PlatformKey(os, arch, variant)}

		img := NewFromIdentifier("dummy=foo/bar:1.12")
		ctx := context.Background()
		opts := img.GetPlatformOptions(ctx, false, platforms)
		assert.True(t, opts.WantsPlatform(os, arch, variant))
		assert.False(t, opts.WantsPlatform(os, arch, ""))
		assert.False(t, opts.WantsPlatform(runtime.GOOS, runtime.GOARCH, ""))
		assert.False(t, opts.WantsPlatform(runtime.GOOS, runtime.GOARCH, variant))
	})
	t.Run("Multiple platforms requested", func(t *testing.T) {
		os := "linux"
		arch := "arm"
		variant := "v6"
		platforms := []string{options.PlatformKey(os, arch, variant),
			options.PlatformKey(runtime.GOOS, runtime.GOARCH, "")}

		img := NewFromIdentifier("dummy=foo/bar:1.12")
		ctx := context.Background()
		opts := img.GetPlatformOptions(ctx, false, platforms)
		assert.True(t, opts.WantsPlatform(os, arch, variant))
		assert.True(t, opts.WantsPlatform(runtime.GOOS, runtime.GOARCH, ""))
		assert.False(t, opts.WantsPlatform(os, arch, ""))
		assert.True(t, opts.WantsPlatform(runtime.GOOS, runtime.GOARCH, variant))
	})
	t.Run("Invalid platform requested", func(t *testing.T) {
		os := "linux"
		arch := "arm"
		variant := "v6"
		platforms := []string{"invalid"}

		img := NewFromIdentifier("dummy=foo/bar:1.12")
		ctx := context.Background()
		opts := img.GetPlatformOptions(ctx, false, platforms)
		assert.False(t, opts.WantsPlatform(os, arch, variant))
		assert.False(t, opts.WantsPlatform(runtime.GOOS, runtime.GOARCH, ""))
		assert.False(t, opts.WantsPlatform(os, arch, ""))
		assert.False(t, opts.WantsPlatform(runtime.GOOS, runtime.GOARCH, variant))
	})
}

func Test_ContainerImage_ParseMatch(t *testing.T) {
	ctx := context.Background()
	img := NewFromIdentifier("dummy=foo/bar:1.12")
	matchFunc, pattern := img.ParseMatch(ctx, "any")
	assert.True(t, matchFunc("MatchFuncAny any tag name", pattern))
	assert.Nil(t, pattern)

	matchFunc, pattern = img.ParseMatch(ctx, "ANY")
	assert.True(t, matchFunc("MatchFuncAny any tag name", pattern))
	assert.Nil(t, pattern)

	matchFunc, pattern = img.ParseMatch(ctx, "other")
	assert.False(t, matchFunc("MatchFuncNone any tag name", pattern))
	assert.Nil(t, pattern)

	matchFunc, pattern = img.ParseMatch(ctx, "not-regexp:a-z")
	assert.False(t, matchFunc("MatchFuncNone any tag name", pattern))
	assert.Nil(t, pattern)

	matchFunc, pattern = img.ParseMatch(ctx, "regexp:[aA-zZ]")
	assert.True(t, matchFunc("MatchFuncRegexp-tag-name", pattern))
	compiledRegexp, _ := regexp.Compile("[aA-zZ]")
	assert.Equal(t, compiledRegexp, pattern)

	matchFunc, pattern = img.ParseMatch(ctx, "RegExp:[aA-zZ]")
	assert.True(t, matchFunc("MatchFuncRegexp-tag-name", pattern))
	compiledRegexp, _ = regexp.Compile("[aA-zZ]")
	assert.Equal(t, compiledRegexp, pattern)

	matchFunc, pattern = img.ParseMatch(ctx, "regexp:[aA-zZ") //invalid regexp: missing end ]
	assert.False(t, matchFunc("MatchFuncNone-tag-name", pattern))
	assert.Nil(t, pattern)
}
