package image

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_ParseUpdateStrategy_CalVer(t *testing.T) {
	img := NewFromIdentifier("example/app:v2026-01-30")
	assert.Equal(t, StrategyCalVer, img.ParseUpdateStrategy(context.Background(), "calver"))
	assert.Equal(t, StrategyCalVer, img.ParseUpdateStrategy(context.Background(), "CalVer"))
}

func Test_CalVerLayoutSurvivesImageSpec(t *testing.T) {
	// The layout is written where a tag would be, so it has to survive being
	// parsed as an image reference before it ever reaches the layout parser.
	layouts := []string{
		"vYYYY-0M-0D",
		"vYYYY-0M-0D-MICRO",
		"vDD-MM-YY",
		"YYYY0M0D",
		"release-YYYY_0M_0D",
		"vYY.MINOR.MICRO",
		"vYYYY-0M-0D-MODIFIER",
		"MAJOR.YY.0M",
	}
	for _, layout := range layouts {
		t.Run(layout, func(t *testing.T) {
			img := NewFromIdentifier("myalias=example.com/some/app:" + layout)
			require.NotNil(t, img.ImageTag, "layout %s is not a valid image tag", layout)
			assert.Equal(t, layout, img.ImageTag.TagName)

			tagList := newImageTagList([]string{"v2026-01-30", "v2026-02-01"})
			vc := VersionConstraint{Strategy: StrategyCalVer, Constraint: img.ImageTag.TagName}
			_, err := img.GetNewestVersionFromTags(context.Background(), &vc, tagList)
			require.NoError(t, err, "layout %s reached the parser but was rejected", layout)
		})
	}
}

func Test_LatestVersion_CalVer(t *testing.T) {
	tests := []struct {
		name string
		// layout is the version constraint, i.e. the tag part of the image spec.
		layout   string
		tags     []string
		expected string
	}{
		{
			// The default is the canonical calver.org scheme, dot separated
			// and zero padded.
			name:     "Default layout when no constraint is given",
			tags:     []string{"v2026.01.30", "v2026.09.01", "v2026.10.03"},
			expected: "v2026.10.03",
		},
		{
			name:     "The default layout ignores tags of another spelling",
			tags:     []string{"v2026.01.30", "2026-02-01", "v2026.1.30"},
			expected: "v2026.01.30",
		},
		{
			name:     "An unpadded month does not outrank a later date",
			layout:   "vYYYY-MM-DD",
			tags:     []string{"v2026-1-30", "v2026-09-01"},
			expected: "v2026-09-01",
		},
		{
			name:     "A stale unpadded tag does not pin the image",
			layout:   "vYYYY-MM-DD",
			tags:     []string{"v2026-9-01", "v2026-10-03", "v2026-11-04", "v2026-12-25"},
			expected: "v2026-12-25",
		},
		{
			name:     "Micro segments are compared numerically",
			layout:   "vYYYY-0M-0D-MICRO",
			tags:     []string{"v2026-01-30", "v2026-01-30-2", "v2026-01-30-10"},
			expected: "v2026-01-30-10",
		},
		{
			name:     "Day first layout",
			layout:   "vDD-MM-YYYY",
			tags:     []string{"v30-01-2026", "v01-09-2026", "v03-10-2026"},
			expected: "v03-10-2026",
		},
		{
			name:     "Day first layout with a short year",
			layout:   "vDD-MM-YY",
			tags:     []string{"v03-10-26", "v31-12-26", "v01-01-27"},
			expected: "v01-01-27",
		},
		{
			name:     "Tags that do not match the layout are ignored",
			layout:   "vYYYY-0M-0D",
			tags:     []string{"latest", "v2026-01-30", "v1.2.3", "v2026-02-01", "v2026-02-01-rc1"},
			expected: "v2026-02-01",
		},
		{
			name:     "A zero padded layout ignores unpadded tags",
			layout:   "vYYYY-0M-0D",
			tags:     []string{"v2026-02-01", "v2026-3-01"},
			expected: "v2026-02-01",
		},
		{
			// A pre-release is only ever considered when the layout spells
			// MODIFIER out, and even then it ranks below the plain release.
			name:     "A release outranks its own pre-releases",
			layout:   "vYYYY-0M-0D-MODIFIER",
			tags:     []string{"v2026-02-01-rc1", "v2026-02-01-rc2", "v2026-02-01"},
			expected: "v2026-02-01",
		},
		{
			name:     "Pre-releases are ordered naturally",
			layout:   "vYYYY-0M-0D-MODIFIER",
			tags:     []string{"v2026-02-01-rc2", "v2026-02-01-rc10"},
			expected: "v2026-02-01-rc10",
		},
		{
			name:     "A leading major segment outranks the date",
			layout:   "MAJOR.YY.0M",
			tags:     []string{"1.26.11", "2.25.01"},
			expected: "2.25.01",
		},
		{
			name:     "Optional minor and micro segments",
			layout:   "vYY.MINOR.MICRO",
			tags:     []string{"v26", "v26.4", "v26.4.11"},
			expected: "v26.4.11",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tagList := newImageTagList(tt.tags)
			img := NewFromIdentifier("example/app:" + tt.tags[0])
			vc := VersionConstraint{Strategy: StrategyCalVer, Constraint: tt.layout}

			newTag, err := img.GetNewestVersionFromTags(context.Background(), &vc, tagList)
			require.NoError(t, err)
			require.NotNil(t, newTag)
			assert.Equal(t, tt.expected, newTag.TagName)
		})
	}

	t.Run("An invalid layout is an error", func(t *testing.T) {
		tagList := newImageTagList([]string{"v2026-01-30"})
		img := NewFromIdentifier("example/app:v2026-01-30")
		vc := VersionConstraint{Strategy: StrategyCalVer, Constraint: "v0M-0D"}

		newTag, err := img.GetNewestVersionFromTags(context.Background(), &vc, tagList)
		assert.ErrorContains(t, err, "must contain a year field")
		assert.Nil(t, newTag)
	})

	t.Run("A concrete tag is not a layout", func(t *testing.T) {
		// Writing the current tag where the layout belongs is the mistake this
		// strategy invites, so it has to fail loudly rather than match nothing.
		tagList := newImageTagList([]string{"v2026-01-30"})
		img := NewFromIdentifier("example/app:v2026-01-30")
		vc := VersionConstraint{Strategy: StrategyCalVer, Constraint: "v2026-01-30"}

		newTag, err := img.GetNewestVersionFromTags(context.Background(), &vc, tagList)
		assert.ErrorContains(t, err, "outside of a token")
		assert.Nil(t, newTag)
	})

	t.Run("No matching tag yields no update candidate", func(t *testing.T) {
		tagList := newImageTagList([]string{"latest", "stable"})
		img := NewFromIdentifier("example/app:latest")
		vc := VersionConstraint{Strategy: StrategyCalVer, Constraint: "vYYYY-0M-0D"}

		newTag, err := img.GetNewestVersionFromTags(context.Background(), &vc, tagList)
		require.NoError(t, err)
		assert.Nil(t, newTag)
	})
}
