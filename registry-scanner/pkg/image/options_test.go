package image

import (
	"context"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/options"
)

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
