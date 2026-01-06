package image

import (
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
		newTag, err := img.GetNewestVersionFromTags(&vc, tagList)
		require.NoError(t, err)
		require.NotNil(t, newTag)
		assert.Equal(t, "2.0.3", newTag.TagName)
	})

	t.Run("Find the latest version with a semver constraint on major", func(t *testing.T) {
		tagList := newImageTagList([]string{"0.1", "0.5.1", "0.9", "1.0", "1.0.1", "1.1.2", "2.0.3"})
		img := NewFromIdentifier("jannfis/test:1.0")
		vc := VersionConstraint{Constraint: "^1.0"}
		newTag, err := img.GetNewestVersionFromTags(&vc, tagList)
		require.NoError(t, err)
		require.NotNil(t, newTag)
		assert.Equal(t, "1.1.2", newTag.TagName)
	})

	t.Run("Find the latest version with a semver constraint on patch", func(t *testing.T) {
		tagList := newImageTagList([]string{"0.1", "0.5.1", "0.9", "1.0", "1.0.1", "1.1.2", "2.0.3"})
		img := NewFromIdentifier("jannfis/test:1.0")
		vc := VersionConstraint{Constraint: "~1.0"}
		newTag, err := img.GetNewestVersionFromTags(&vc, tagList)
		require.NoError(t, err)
		require.NotNil(t, newTag)
		assert.Equal(t, "1.0.1", newTag.TagName)
	})

	t.Run("Find the latest version with a semver constraint that has no match", func(t *testing.T) {
		tagList := newImageTagList([]string{"0.1", "0.5.1", "0.9", "2.0.3"})
		img := NewFromIdentifier("jannfis/test:1.0")
		vc := VersionConstraint{Constraint: "~1.0"}
		newTag, err := img.GetNewestVersionFromTags(&vc, tagList)
		require.NoError(t, err)
		require.Nil(t, newTag)
	})

	t.Run("Find the latest version with a semver constraint that is invalid", func(t *testing.T) {
		tagList := newImageTagList([]string{"0.1", "0.5.1", "0.9", "2.0.3"})
		img := NewFromIdentifier("jannfis/test:1.0")
		vc := VersionConstraint{Constraint: "latest"}
		newTag, err := img.GetNewestVersionFromTags(&vc, tagList)
		assert.Error(t, err)
		assert.Nil(t, newTag)
	})

	t.Run("Find the latest version with a calver constraint that is valid", func(t *testing.T) {
		tagList := newImageTagList([]string{"2021.01.01", "2022.02.02", "2023.05.01", "2025.01.25"})
		img := NewFromIdentifier("jannfis/test:2021.01.01")
		vc := VersionConstraint{Constraint: "2022.01.01", Strategy: StrategyCalVer, MatchArgs: "YYYY.MM.DD"}
		newTag, err := img.GetNewestVersionFromTags(&vc, tagList)
		assert.NoError(t, err)
		assert.NotNil(t, newTag)
		assert.Equal(t, "2025.01.25", newTag.TagName)
	})

	t.Run("Find latest version with YYYY.MM calver format", func(t *testing.T) {
		tagList := newImageTagList([]string{"2021.01", "2022.02", "2023.05", "2025.01"})
		img := NewFromIdentifier("jannfis/test:2021.01")
		vc := VersionConstraint{Constraint: "2022.01", Strategy: StrategyCalVer, MatchArgs: "YYYY.MM"}
		newTag, err := img.GetNewestVersionFromTags(&vc, tagList)
		assert.NoError(t, err)
		assert.NotNil(t, newTag)
		assert.Equal(t, "2025.01", newTag.TagName)
	})

	t.Run("Find latest version with YY.MM.DD calver format", func(t *testing.T) {
		tagList := newImageTagList([]string{"21.01.01", "22.02.02", "23.05.01", "25.01.25"})
		img := NewFromIdentifier("jannfis/test:21.01.01")
		vc := VersionConstraint{Constraint: "22.01.01", Strategy: StrategyCalVer, MatchArgs: "YY.MM.DD"}
		newTag, err := img.GetNewestVersionFromTags(&vc, tagList)
		assert.NoError(t, err)
		assert.NotNil(t, newTag)
		assert.Equal(t, "25.01.25", newTag.TagName)
	})

	t.Run("Invalid calver format should return error", func(t *testing.T) {
		tagList := newImageTagList([]string{"2021.01.01", "2022.02.02"})
		img := NewFromIdentifier("jannfis/test:2021.01.01")
		vc := VersionConstraint{Constraint: "2022.01.01", Strategy: StrategyCalVer, MatchArgs: "invalid-format"}
		newTag, err := img.GetNewestVersionFromTags(&vc, tagList)
		assert.Error(t, err)
		assert.Nil(t, newTag)
	})

	t.Run("Tags not matching calver format should be ignored", func(t *testing.T) {
		tagList := newImageTagList([]string{"2021.01.01", "invalid", "2023.05.01", "not-a-date"})
		img := NewFromIdentifier("jannfis/test:2021.01.01")
		vc := VersionConstraint{Constraint: "2022.01.01", Strategy: StrategyCalVer, MatchArgs: "YYYY.MM.DD"}
		newTag, err := img.GetNewestVersionFromTags(&vc, tagList)
		assert.NoError(t, err)
		assert.NotNil(t, newTag)
		assert.Equal(t, "2023.05.01", newTag.TagName)
	})

	t.Run("Empty tag list with calver should return nil", func(t *testing.T) {
		tagList := newImageTagList([]string{})
		img := NewFromIdentifier("jannfis/test:2021.01.01")
		vc := VersionConstraint{Constraint: "2022.01.01", Strategy: StrategyCalVer, MatchArgs: "YYYY.MM.DD"}
		newTag, err := img.GetNewestVersionFromTags(&vc, tagList)
		assert.NoError(t, err)
		assert.Nil(t, newTag)
	})

	t.Run("Missing constraint with calver should use current date", func(t *testing.T) {
		tagList := newImageTagList([]string{"2021.01.01", "2022.02.02", "2023.05.01"})
		img := NewFromIdentifier("jannfis/test:2021.01.01")
		vc := VersionConstraint{Strategy: StrategyCalVer, MatchArgs: "YYYY.MM.DD"}
		newTag, err := img.GetNewestVersionFromTags(&vc, tagList)
		assert.NoError(t, err)
		assert.NotNil(t, newTag)
		assert.Equal(t, "2023.05.01", newTag.TagName)
	})

	t.Run("Find the latest version with no tags", func(t *testing.T) {
		tagList := newImageTagList([]string{})
		img := NewFromIdentifier("jannfis/test:1.0")
		vc := VersionConstraint{Constraint: "~1.0"}
		newTag, err := img.GetNewestVersionFromTags(&vc, tagList)
		require.NoError(t, err)
		require.NotNil(t, newTag)
		assert.Equal(t, "1.0", newTag.TagName)
	})

	t.Run("Find the latest version using latest sortmode", func(t *testing.T) {
		tagList := newImageTagListWithDate([]string{"zz", "bb", "yy", "cc", "yy", "aa", "ll"})
		img := NewFromIdentifier("jannfis/test:bb")
		vc := VersionConstraint{Strategy: StrategyNewestBuild}
		newTag, err := img.GetNewestVersionFromTags(&vc, tagList)
		require.NoError(t, err)
		require.NotNil(t, newTag)
		assert.Equal(t, "ll", newTag.TagName)
	})

	t.Run("Find the latest version using latest sortmode, invalid tags", func(t *testing.T) {
		tagList := newImageTagListWithDate([]string{"zz", "bb", "yy", "cc", "yy", "aa", "ll"})
		img := NewFromIdentifier("jannfis/test:bb")
		vc := VersionConstraint{Strategy: StrategySemVer}
		newTag, err := img.GetNewestVersionFromTags(&vc, tagList)
		require.NoError(t, err)
		require.NotNil(t, newTag)
		assert.Equal(t, "bb", newTag.TagName)
	})

	t.Run("Find the latest version using VersionConstraint StrategyAlphabetical", func(t *testing.T) {
		tagList := newImageTagListWithDate([]string{"zz", "bb", "yy", "cc", "yy", "aa", "ll"})
		img := NewFromIdentifier("jannfis/test:bb")
		vc := VersionConstraint{Strategy: StrategyAlphabetical}
		newTag, err := img.GetNewestVersionFromTags(&vc, tagList)
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
		newTag, err := img.GetNewestVersionFromTags(&vc, tagList)
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
		{"StrategyCalVer", StrategyCalVer, "calver"},
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
	assert.True(t, versionConstraint.IsTagIgnored("tag1"))
	assert.True(t, versionConstraint.IsTagIgnored("tag2"))
	assert.False(t, versionConstraint.IsTagIgnored("tag3"))
	versionConstraint.IgnoreList = []string{"tag?", "foo"}
	assert.True(t, versionConstraint.IsTagIgnored("tag1"))
	assert.True(t, versionConstraint.IsTagIgnored("foo"))
	assert.False(t, versionConstraint.IsTagIgnored("tag10"))
}

func Test_UpdateStrategy_IsCacheable(t *testing.T) {
	assert.True(t, StrategySemVer.IsCacheable())
	assert.True(t, StrategyNewestBuild.IsCacheable())
	assert.True(t, StrategyAlphabetical.IsCacheable())
	assert.True(t, StrategyCalVer.IsCacheable())
	assert.False(t, StrategyDigest.IsCacheable())
}

func Test_UpdateStrategy_NeedsMetadata(t *testing.T) {
	assert.False(t, StrategySemVer.NeedsMetadata())
	assert.True(t, StrategyNewestBuild.NeedsMetadata())
	assert.False(t, StrategyAlphabetical.NeedsMetadata())
	assert.False(t, StrategyCalVer.NeedsMetadata())
	assert.False(t, StrategyDigest.NeedsMetadata())
}

func Test_UpdateStrategy_NeedsVersionConstraint(t *testing.T) {
	assert.False(t, StrategySemVer.NeedsVersionConstraint())
	assert.False(t, StrategyNewestBuild.NeedsVersionConstraint())
	assert.False(t, StrategyAlphabetical.NeedsVersionConstraint())
	assert.True(t, StrategyCalVer.NeedsVersionConstraint())
	assert.True(t, StrategyDigest.NeedsVersionConstraint())
}

func Test_UpdateStrategy_WantsOnlyConstraintTag(t *testing.T) {
	assert.False(t, StrategySemVer.WantsOnlyConstraintTag())
	assert.False(t, StrategyNewestBuild.WantsOnlyConstraintTag())
	assert.False(t, StrategyAlphabetical.WantsOnlyConstraintTag())
	assert.False(t, StrategyCalVer.WantsOnlyConstraintTag())
	assert.True(t, StrategyDigest.WantsOnlyConstraintTag())
}
