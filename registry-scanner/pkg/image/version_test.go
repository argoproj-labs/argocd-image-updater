package image

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/options"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/tag"
)

func newImageTagList(tagNames []string) *tag.ImageTagList {
	tagList := tag.NewImageTagList()
	for _, tagName := range tagNames {
		tagList.Add(tag.NewImageTag(tagName, time.Unix(0, 0), ""))
	}
	return tagList
}

func newImageTagListWithDate(tagNames []string) *tag.ImageTagList {
	tagList := tag.NewImageTagList()
	for i, t := range tagNames {
		tagList.Add(tag.NewImageTag(t, time.Unix(int64(i*5), 0), ""))
	}
	return tagList
}

func Test_LatestVersion(t *testing.T) {
	t.Run("Find the latest version without any constraint", func(t *testing.T) {
		tagList := newImageTagList([]string{"0.1", "0.5.1", "0.9", "1.0", "1.0.1", "1.1.2", "2.0.3"})
		img := NewFromIdentifier("jannfis/test:1.0")
		vc := VersionConstraint{}
		newTag, err := img.GetNewestVersionFromTags(context.Background(), &vc, tagList)
		require.NoError(t, err)
		require.NotNil(t, newTag)
		assert.Equal(t, "2.0.3", newTag.TagName)
	})

	t.Run("Find the latest version with a semver constraint on major", func(t *testing.T) {
		tagList := newImageTagList([]string{"0.1", "0.5.1", "0.9", "1.0", "1.0.1", "1.1.2", "2.0.3"})
		img := NewFromIdentifier("jannfis/test:1.0")
		vc := VersionConstraint{Constraint: "^1.0"}
		newTag, err := img.GetNewestVersionFromTags(context.Background(), &vc, tagList)
		require.NoError(t, err)
		require.NotNil(t, newTag)
		assert.Equal(t, "1.1.2", newTag.TagName)
	})

	t.Run("Find the latest version with a semver constraint on patch", func(t *testing.T) {
		tagList := newImageTagList([]string{"0.1", "0.5.1", "0.9", "1.0", "1.0.1", "1.1.2", "2.0.3"})
		img := NewFromIdentifier("jannfis/test:1.0")
		vc := VersionConstraint{Constraint: "~1.0"}
		newTag, err := img.GetNewestVersionFromTags(context.Background(), &vc, tagList)
		require.NoError(t, err)
		require.NotNil(t, newTag)
		assert.Equal(t, "1.0.1", newTag.TagName)
	})

	t.Run("Find the latest version with a non-semver current tag and semver constraint", func(t *testing.T) {
		tagList := newImageTagList([]string{"0.1", "0.5.1", "0.9", "1.0", "1.0.1", "1.1.2", "2.0.3"})
		img := NewFromIdentifier("christianschlichtherle/test:latest")
		vc := VersionConstraint{Constraint: "^1.0"}
		newTag, err := img.GetNewestVersionFromTags(context.Background(), &vc, tagList)
		require.NoError(t, err)
		require.NotNil(t, newTag)
		assert.Equal(t, "1.1.2", newTag.TagName)
	})

	t.Run("Find the latest version with a non-semver current tag without any constraint", func(t *testing.T) {
		tagList := newImageTagList([]string{"0.1", "0.5.1", "0.9", "1.0", "1.0.1", "1.1.2", "2.0.3"})
		img := NewFromIdentifier("christianschlichtherle/test:latest")
		vc := VersionConstraint{}
		newTag, err := img.GetNewestVersionFromTags(context.Background(), &vc, tagList)
		require.NoError(t, err)
		require.NotNil(t, newTag)
		assert.Equal(t, "2.0.3", newTag.TagName)
	})

	t.Run("Find the latest version with a semver constraint that has no match", func(t *testing.T) {
		tagList := newImageTagList([]string{"0.1", "0.5.1", "0.9", "2.0.3"})
		img := NewFromIdentifier("jannfis/test:1.0")
		vc := VersionConstraint{Constraint: "~1.0"}
		newTag, err := img.GetNewestVersionFromTags(context.Background(), &vc, tagList)
		require.NoError(t, err)
		require.Nil(t, newTag)
	})

	t.Run("Find the latest version with a semver constraint that is invalid", func(t *testing.T) {
		tagList := newImageTagList([]string{"0.1", "0.5.1", "0.9", "2.0.3"})
		img := NewFromIdentifier("jannfis/test:1.0")
		vc := VersionConstraint{Constraint: "latest"}
		newTag, err := img.GetNewestVersionFromTags(context.Background(), &vc, tagList)
		assert.Error(t, err)
		assert.Nil(t, newTag)
	})

	t.Run("Find the latest version with no tags returns nil", func(t *testing.T) {
		tagList := newImageTagList([]string{})
		img := NewFromIdentifier("jannfis/test:1.0")
		vc := VersionConstraint{Constraint: "~1.0"}
		newTag, err := img.GetNewestVersionFromTags(context.Background(), &vc, tagList)
		require.NoError(t, err)
		require.Nil(t, newTag)
	})

	t.Run("Find the latest version using latest sortmode", func(t *testing.T) {
		tagList := newImageTagListWithDate([]string{"zz", "bb", "yy", "cc", "yy", "aa", "ll"})
		img := NewFromIdentifier("jannfis/test:bb")
		vc := VersionConstraint{Strategy: StrategyNewestBuild}
		newTag, err := img.GetNewestVersionFromTags(context.Background(), &vc, tagList)
		require.NoError(t, err)
		require.NotNil(t, newTag)
		assert.Equal(t, "ll", newTag.TagName)
	})

	t.Run("Find the latest version using semver sortmode with invalid tags returns nil", func(t *testing.T) {
		tagList := newImageTagListWithDate([]string{"zz", "bb", "yy", "cc", "yy", "aa", "ll"})
		img := NewFromIdentifier("jannfis/test:bb")
		vc := VersionConstraint{Strategy: StrategySemVer}
		newTag, err := img.GetNewestVersionFromTags(context.Background(), &vc, tagList)
		require.NoError(t, err)
		require.Nil(t, newTag)
	})

	t.Run("Find the latest version using VersionConstraint StrategyAlphabetical", func(t *testing.T) {
		tagList := newImageTagListWithDate([]string{"zz", "bb", "yy", "cc", "yy", "aa", "ll"})
		img := NewFromIdentifier("jannfis/test:bb")
		vc := VersionConstraint{Strategy: StrategyAlphabetical}
		newTag, err := img.GetNewestVersionFromTags(context.Background(), &vc, tagList)
		require.NoError(t, err)
		require.NotNil(t, newTag)
		assert.Equal(t, "zz", newTag.TagName)
	})

	t.Run("Find the latest version using VersionConstraint StrategyDigest", func(t *testing.T) {
		tagList := tag.NewImageTagList()
		newDigest := "latest@sha:abcdefg"
		tagList.Add(tag.NewImageTag("latest", time.Unix(int64(6), 0), newDigest))
		img := NewFromIdentifier("jannfis/test:latest@sha:1234567")
		vc := VersionConstraint{Strategy: StrategyDigest, Constraint: "latest"}
		newTag, err := img.GetNewestVersionFromTags(context.Background(), &vc, tagList)
		require.NoError(t, err)
		assert.Equal(t, "latest", newTag.TagName)
		assert.Equal(t, newDigest, newTag.TagDigest)
	})

}

func Test_UpdateStrategy_String(t *testing.T) {
	tests := []struct {
		name string
		us   UpdateStrategy
		want string
	}{
		{"StrategySemVer", StrategySemVer, "semver"},
		{"StrategyNewestBuild", StrategyNewestBuild, "newest-build"},
		{"StrategyAlphabetical", StrategyAlphabetical, "alphabetical"},
		{"StrategyDigest", StrategyDigest, "digest"},
		{"unknown", UpdateStrategy(-1), "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.us.String())
		})
	}
}

func Test_NewVersionConstraint(t *testing.T) {
	constraint := NewVersionConstraint()
	assert.Equal(t, StrategySemVer, constraint.Strategy)
	assert.Equal(t, options.NewManifestOptions(), constraint.Options)
	assert.False(t, constraint.MatchFunc("", ""))
}

func Test_VersionConstraint_IsTagIgnored(t *testing.T) {
	versionConstraint := VersionConstraint{IgnoreList: []string{"tag1", "tag2"}}
	ctx := context.Background()
	assert.True(t, versionConstraint.IsTagIgnored(ctx, "tag1"))
	assert.True(t, versionConstraint.IsTagIgnored(ctx, "tag2"))
	assert.False(t, versionConstraint.IsTagIgnored(ctx, "tag3"))
	versionConstraint.IgnoreList = []string{"tag?", "foo"}
	assert.True(t, versionConstraint.IsTagIgnored(ctx, "tag1"))
	assert.True(t, versionConstraint.IsTagIgnored(ctx, "foo"))
	assert.False(t, versionConstraint.IsTagIgnored(ctx, "tag10"))
}

func Test_UpdateStrategy_IsCacheable(t *testing.T) {
	assert.True(t, StrategySemVer.IsCacheable())
	assert.True(t, StrategyNewestBuild.IsCacheable())
	assert.True(t, StrategyAlphabetical.IsCacheable())
	assert.False(t, StrategyDigest.IsCacheable())
}

func Test_UpdateStrategy_NeedsMetadata(t *testing.T) {
	assert.False(t, StrategySemVer.NeedsMetadata())
	assert.True(t, StrategyNewestBuild.NeedsMetadata())
	assert.False(t, StrategyAlphabetical.NeedsMetadata())
	assert.False(t, StrategyDigest.NeedsMetadata())
}

func Test_UpdateStrategy_NeedsVersionConstraint(t *testing.T) {
	assert.False(t, StrategySemVer.NeedsVersionConstraint())
	assert.False(t, StrategyNewestBuild.NeedsVersionConstraint())
	assert.False(t, StrategyAlphabetical.NeedsVersionConstraint())
	assert.True(t, StrategyDigest.NeedsVersionConstraint())
}

func Test_UpdateStrategy_WantsOnlyConstraintTag(t *testing.T) {
	assert.False(t, StrategySemVer.WantsOnlyConstraintTag())
	assert.False(t, StrategyNewestBuild.WantsOnlyConstraintTag())
	assert.False(t, StrategyAlphabetical.WantsOnlyConstraintTag())
	assert.True(t, StrategyDigest.WantsOnlyConstraintTag())
}
