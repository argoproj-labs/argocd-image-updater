package tag

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewCalVerLayout(t *testing.T) {
	t.Run("Empty layout falls back to the default", func(t *testing.T) {
		l, err := NewCalVerLayout("")
		require.NoError(t, err)
		assert.Equal(t, DefaultCalVerLayout, l.String())
		assert.Equal(t, "YYYY.0M.0D", DefaultCalVerLayout, "the default is the canonical calver.org scheme")
	})

	t.Run("The default layout insists on the canonical spelling", func(t *testing.T) {
		l, err := NewCalVerLayout("")
		require.NoError(t, err)

		_, err = l.parse("2026.01.30")
		require.NoError(t, err)

		// Neither a different separator nor a missing zero is the canonical
		// scheme, so both are left for an explicit layout to pick up.
		for _, tagName := range []string{"2026-01-30", "2026.1.30"} {
			_, err = l.parse(tagName)
			assert.Error(t, err, "%s should not match the default layout", tagName)
		}
	})

	validLayouts := []string{
		"YYYY-0M-0D",
		"vYYYY-0M-0D",
		"vYYYY-0M-0D-MICRO",
		"vDD-MM-YYYY",
		"vDD-MM-YY",
		"vYY.0M.0D",
		"YYYY0M0D",
		"release-YYYY_0M_0D",
		"vYYYY-0M",
		"vYYYY",
		"vYY.MINOR.MICRO",
		"vYYYY.MAJOR.MINOR.MICRO",
		"vYYYY-0W",
		// The schemes below are the ones a fixed field ranking used to reject.
		"MAJOR.YY.0M",
		"MAJOR.MINOR.YYYY",
		"vYYYY-0M-0D-MODIFIER",
		"vYY.MINOR.MICRO-MODIFIER",
		"vYYYY.0M.MICRO_MODIFIER",
	}
	for _, layout := range validLayouts {
		t.Run("Valid layout "+layout, func(t *testing.T) {
			l, err := NewCalVerLayout(layout)
			require.NoError(t, err)
			assert.Equal(t, layout, l.String())
		})
	}

	invalidLayouts := map[string]string{
		"v0M-0D":                 "must contain a year field",
		"MAJOR.MINOR.MICRO":      "must contain a year field",
		"vYYYY-YY":               "defines the same field twice",
		"vYYYY-0M-MM":            "defines the same field twice",
		"vYYYY-0D":               "contains a day but no month",
		"vYYYY-0W-0M":            "must not combine a week field",
		"v1YYYY-0M-0D":           "contains '1' outside of a token",
		"nonsense":               "must contain a year field",
		"vYYYY-0M-D":             "contains 'D' outside of a token",
		"vYYYY-M-0D":             "contains 'M' outside of a token",
		"vYYYY-0M-DDD":           "contains 'D' outside of a token",
		"vYYYYMM0D":              "places the variable width MM directly before another field",
		"vYYMM":                  "places the variable width YY directly before another field",
		"vYYYY-0M-0D-MINORMICRO": "places the variable width MINOR directly before another field",
		// The date is compared by significance rather than in the order it is
		// written, which only works while it is one uninterrupted run.
		"vYYYY.MINOR.0M":       "splits the date around 0M",
		"vYYYY-MICRO0M":        "splits the date around 0M",
		"vMODIFIER-YYYY":       "MODIFIER matches the remainder of the tag and must come last",
		"vYYYY-MODIFIER-0M":    "splits the date around 0M",
		"vYYYY-0M-MODIFIER-0D": "splits the date around 0D",
		"vYYYY.MODIFIER.MICRO": "MODIFIER matches the remainder of the tag and must come last",
		// Where the numbered segments sit relative to the date is free, but
		// their order among themselves is not.
		"vYYYY.MICRO.MINOR": "places MICRO before MINOR, but MINOR is the more significant",
		"vYYYY.MINOR.MAJOR": "places MINOR before MAJOR, but MAJOR is the more significant",
		"MINOR.YYYY.MAJOR":  "places MINOR before MAJOR, but MAJOR is the more significant",
	}
	for layout, wantErr := range invalidLayouts {
		t.Run("Invalid layout "+layout, func(t *testing.T) {
			l, err := NewCalVerLayout(layout)
			require.Error(t, err)
			assert.Nil(t, l)
			assert.Contains(t, err.Error(), wantErr)
		})
	}
}

func Test_CalVerLayout_parse(t *testing.T) {
	tests := []struct {
		name     string
		layout   string
		tagName  string
		wantErr  bool
		year     int
		month    int
		week     int
		day      int
		major    int
		minor    int
		micro    int
		modifier string
	}{
		{name: "Zero padded date", layout: "vYYYY-0M-0D", tagName: "v2026-01-30", year: 2026, month: 1, day: 30},
		{name: "Zero padded tokens require the padding", layout: "vYYYY-0M-0D", tagName: "v2026-1-30", wantErr: true},
		{name: "Short tokens accept unpadded fields", layout: "vYYYY-MM-DD", tagName: "v2026-1-3", year: 2026, month: 1, day: 3},
		{name: "Short tokens also accept padded fields", layout: "vYYYY-MM-DD", tagName: "v2026-01-30", year: 2026, month: 1, day: 30},
		{name: "Two digit month", layout: "vYYYY-0M-0D", tagName: "v2026-10-03", year: 2026, month: 10, day: 3},
		{name: "Explicit micro segment", layout: "vYYYY-0M-0D-MICRO", tagName: "v2026-01-30-12", year: 2026, month: 1, day: 30, micro: 12},
		{name: "A trailing micro segment may be left out", layout: "vYYYY-0M-0D-MICRO", tagName: "v2026-01-30", year: 2026, month: 1, day: 30},
		{name: "A trailing separator without a segment is rejected", layout: "vYYYY-0M-0D-MICRO", tagName: "v2026-01-30-", wantErr: true},
		{name: "An undeclared trailing segment is rejected", layout: "vYYYY-0M-0D", tagName: "v2026-01-30-7", wantErr: true},
		{name: "Minor and micro segments", layout: "vYY.MINOR.MICRO", tagName: "v26.4.11", year: 2026, minor: 4, micro: 11},
		{name: "Minor and micro segments may both be left out", layout: "vYY.MINOR.MICRO", tagName: "v26", year: 2026},
		{name: "Only the micro segment is left out", layout: "vYY.MINOR.MICRO", tagName: "v26.4", year: 2026, minor: 4},
		{name: "Major segment", layout: "vYYYY.MAJOR.MINOR", tagName: "v2026.3.4", year: 2026, major: 3, minor: 4},
		{name: "A major segment is never optional", layout: "vYYYY.MAJOR", tagName: "v2026", wantErr: true},
		{name: "Major segment leading the date", layout: "MAJOR.YY.0M", tagName: "1.26.01", year: 2026, month: 1, major: 1},
		{name: "Day first", layout: "vDD-MM-YYYY", tagName: "v30-01-2026", year: 2026, month: 1, day: 30},
		{name: "Day first with short year", layout: "vDD-MM-YY", tagName: "v30-01-26", year: 2026, month: 1, day: 30},
		{name: "Short year counts from 2000", layout: "vYY-MM-DD", tagName: "v6-01-30", year: 2006, month: 1, day: 30},
		{name: "Short year reaches past 2099", layout: "vYY-MM-DD", tagName: "v106-01-30", year: 2106, month: 1, day: 30},
		{name: "Padded short year", layout: "v0Y-0M-0D", tagName: "v06-01-30", year: 2006, month: 1, day: 30},
		{name: "Padded short year requires the padding", layout: "v0Y-0M-0D", tagName: "v6-01-30", wantErr: true},
		{name: "Week based layout", layout: "vYYYY-0W", tagName: "v2026-05", year: 2026, week: 5},
		{name: "Week out of range", layout: "vYYYY-0W", tagName: "v2026-54", wantErr: true},
		{name: "Week zero is a valid week", layout: "vYYYY.0W", tagName: "v2026.00", year: 2026, week: 0},
		{name: "Dot separated", layout: "vYY.0M.0D", tagName: "v26.01.30", year: 2026, month: 1, day: 30},
		{name: "No separators", layout: "YYYY0M0D", tagName: "20260130", year: 2026, month: 1, day: 30},
		{name: "No separators requires padding", layout: "YYYY0M0D", tagName: "2026130", wantErr: true},
		{name: "Prefixless layout accepts a v prefix", layout: "YYYY-0M-0D", tagName: "v2026-01-30", year: 2026, month: 1, day: 30},
		{name: "Prefixless layout accepts no prefix", layout: "YYYY-0M-0D", tagName: "2026-01-30", year: 2026, month: 1, day: 30},
		{name: "A spelled out v prefix is required", layout: "vYYYY-0M-0D", tagName: "2026-01-30", wantErr: true},
		{name: "Literal prefix is required", layout: "release-YYYY-0M-0D", tagName: "v2026-01-30", wantErr: true},
		{name: "Literal prefix matches", layout: "release-YYYY-0M-0D", tagName: "release-2026-01-30", year: 2026, month: 1, day: 30},
		{name: "Month only layout", layout: "vYYYY-0M", tagName: "v2026-10", year: 2026, month: 10},
		{name: "Non matching tag", layout: "vYYYY-0M-0D", tagName: "latest", wantErr: true},
		{name: "Semver tag does not match", layout: "vYYYY-0M-0D", tagName: "v1.2.3", wantErr: true},
		{name: "Wrong year width", layout: "vYYYY-0M-0D", tagName: "v206-01-30", wantErr: true},
		{name: "Month out of range", layout: "vYYYY-0M-0D", tagName: "v2026-13-30", wantErr: true},
		{name: "Swapped month and day is rejected", layout: "vYYYY-0M-0D", tagName: "v2026-31-01", wantErr: true},
		{name: "Day out of range", layout: "vYYYY-0M-0D", tagName: "v2026-01-32", wantErr: true},
		{name: "Month zero is out of range", layout: "vYYYY-0M-0D", tagName: "v2026-00-30", wantErr: true},
		{name: "A day the month does not have is rejected", layout: "vYYYY-0M-0D", tagName: "v2026-02-31", wantErr: true},
		{name: "February 29 of a leap year is a date", layout: "vYYYY-0M-0D", tagName: "v2028-02-29", year: 2028, month: 2, day: 29},
		{name: "February 29 of a common year is not", layout: "vYYYY-0M-0D", tagName: "v2026-02-29", wantErr: true},

		// MODIFIER, the segment calver.org describes as "an optional text tag".
		{name: "Modifier is matched", layout: "vYYYY-0M-0D-MODIFIER", tagName: "v2026-01-30-rc1", year: 2026, month: 1, day: 30, modifier: "rc1"},
		{name: "Modifier may be left out", layout: "vYYYY-0M-0D-MODIFIER", tagName: "v2026-01-30", year: 2026, month: 1, day: 30},
		{name: "Modifier is rejected when the layout does not ask for it", layout: "vYYYY-0M-0D", tagName: "v2026-01-30-rc1", wantErr: true},
		{name: "Modifier with its own separator", layout: "vYYYY.0M.MICRO-MODIFIER", tagName: "v2026.01.3-beta", year: 2026, month: 1, micro: 3, modifier: "beta"},
		{name: "Modifier and micro may both be left out", layout: "vYYYY.0M.MICRO-MODIFIER", tagName: "v2026.01", year: 2026, month: 1},
		{name: "An empty modifier is rejected", layout: "vYYYY-0M-0D-MODIFIER", tagName: "v2026-01-30-", wantErr: true},
		{name: "A modifier may span separators", layout: "vYYYY-0M-0D-MODIFIER", tagName: "v2026-01-30-alpha.2", year: 2026, month: 1, day: 30, modifier: "alpha.2"},
		{name: "A modifier may not contain arbitrary characters", layout: "vYYYY-0M-0D-MODIFIER", tagName: "v2026-01-30-rc/1", wantErr: true},
		{name: "A segment too large to be a number is rejected", layout: "vYYYY.0M.MICRO", tagName: "v2026.01.99999999999999999999", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, err := NewCalVerLayout(tt.layout)
			require.NoError(t, err)

			v, err := l.parse(tt.tagName)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.year, v.fields[fieldYear], "year")
			assert.Equal(t, tt.month, v.fields[fieldMonth], "month")
			assert.Equal(t, tt.week, v.fields[fieldWeek], "week")
			assert.Equal(t, tt.day, v.fields[fieldDay], "day")
			assert.Equal(t, tt.major, v.fields[fieldMajor], "major")
			assert.Equal(t, tt.minor, v.fields[fieldMinor], "minor")
			assert.Equal(t, tt.micro, v.fields[fieldMicro], "micro")
			assert.Equal(t, tt.modifier, v.modifier, "modifier")
		})
	}
}

func Test_SortByCalVer(t *testing.T) {
	newList := func(tagNames []string) *ImageTagList {
		tl := NewImageTagList()
		for _, tagName := range tagNames {
			tl.Add(NewImageTag(tagName, time.Unix(0, 0), ""))
		}
		return tl
	}

	tests := []struct {
		name   string
		layout string
		tags   []string
		want   []string
	}{
		{
			name:   "Dates are ordered chronologically, not lexically",
			layout: "vYYYY-MM-DD",
			tags:   []string{"v2026-10-03", "v2026-1-30", "v2026-09-01"},
			want:   []string{"v2026-1-30", "v2026-09-01", "v2026-10-03"},
		},
		{
			name:   "Micro segments are ordered numerically",
			layout: "vYYYY-0M-0D-MICRO",
			tags:   []string{"v2026-01-30-10", "v2026-01-30-2", "v2026-01-30"},
			want:   []string{"v2026-01-30", "v2026-01-30-2", "v2026-01-30-10"},
		},
		{
			name:   "Equivalent micro segments are ordered by name",
			layout: "vYYYY-0M-0D-MICRO",
			tags:   []string{"v2026-01-30-1", "v2026-01-30-01"},
			want:   []string{"v2026-01-30-01", "v2026-01-30-1"},
		},
		{
			name:   "Day first layout",
			layout: "vDD-MM-YYYY",
			tags:   []string{"v03-10-2026", "v30-01-2026", "v01-09-2026"},
			want:   []string{"v30-01-2026", "v01-09-2026", "v03-10-2026"},
		},
		{
			name:   "Short year layout crosses the year boundary",
			layout: "vDD-MM-YY",
			tags:   []string{"v01-01-27", "v31-12-26", "v03-10-26"},
			want:   []string{"v03-10-26", "v31-12-26", "v01-01-27"},
		},
		{
			name:   "Short years count from 2000 and beyond 2099",
			layout: "vDD-MM-YY",
			tags:   []string{"v31-12-99", "v01-01-0", "v03-10-26", "v01-01-106"},
			want:   []string{"v01-01-0", "v03-10-26", "v31-12-99", "v01-01-106"},
		},
		{
			name:   "Minor and micro segments",
			layout: "vYY.MINOR.MICRO",
			tags:   []string{"v26.4.11", "v26.10.1", "v26.4.2"},
			want:   []string{"v26.4.2", "v26.4.11", "v26.10.1"},
		},
		{
			name:   "Week based layout",
			layout: "vYYYY-0W",
			tags:   []string{"v2026-10", "v2026-02", "v2027-01"},
			want:   []string{"v2026-02", "v2026-10", "v2027-01"},
		},
		{
			// The layout decides what outranks what, so a scheme that leads
			// with MAJOR keeps an older date on a newer major above a newer
			// date on an older one.
			name:   "A leading major segment outranks the date",
			layout: "MAJOR.YY.0M",
			tags:   []string{"1.26.03", "2.25.01", "1.26.11"},
			want:   []string{"1.26.03", "1.26.11", "2.25.01"},
		},
		{
			name:   "A trailing major segment ranks below the date",
			layout: "YY.0M.MAJOR",
			tags:   []string{"26.03.1", "25.01.2", "26.11.1"},
			want:   []string{"25.01.2", "26.03.1", "26.11.1"},
		},
		{
			name:   "Modifiers rank below the release they belong to",
			layout: "vYYYY-0M-0D-MODIFIER",
			tags:   []string{"v2026-01-30", "v2026-01-30-rc1", "v2026-01-30-alpha"},
			want:   []string{"v2026-01-30-alpha", "v2026-01-30-rc1", "v2026-01-30"},
		},
		{
			name:   "Modifiers are ordered naturally",
			layout: "vYYYY-0M-0D-MODIFIER",
			tags:   []string{"v2026-01-30-rc10", "v2026-01-30-rc2", "v2026-01-30-rc1"},
			want:   []string{"v2026-01-30-rc1", "v2026-01-30-rc2", "v2026-01-30-rc10"},
		},
		{
			// MODIFIER cannot tell a variant from a pre-release, so the docs
			// point variant users at a literal suffix instead. This pins that
			// advice: alpine tags are ranked among themselves and the plain
			// tags never enter the list.
			name:   "A variant is tracked with a literal suffix, not MODIFIER",
			layout: "vYYYY-0M-0D-alpine",
			tags:   []string{"v2026-02-01", "v2026-01-30-alpine", "v2026-02-01-alpine", "v2026-02-01-slim"},
			want:   []string{"v2026-01-30-alpine", "v2026-02-01-alpine"},
		},
		{
			// The counterpart of the above: with MODIFIER the plain release
			// outranks the variant, which is what the docs warn about.
			name:   "MODIFIER lets a plain release outrank a variant",
			layout: "vYYYY-0M-0D-MODIFIER",
			tags:   []string{"v2026-01-30-alpine", "v2026-01-30"},
			want:   []string{"v2026-01-30-alpine", "v2026-01-30"},
		},
		{
			name:   "Tags not matching the layout are dropped",
			layout: "vYYYY-0M-0D",
			tags:   []string{"latest", "v2026-01-30", "1.2.3", "v2026-13-01", "v2026-02-01"},
			want:   []string{"v2026-01-30", "v2026-02-01"},
		},
		{
			name:   "Nothing matches",
			layout: "vYYYY-0M-0D",
			tags:   []string{"latest", "stable"},
			want:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l, err := NewCalVerLayout(tt.layout)
			require.NoError(t, err)
			assert.Equal(t, tt.want, newList(tt.tags).SortByCalVer(context.Background(), l).Tags())
		})
	}
}

func Test_SortByCalVer_NilLayout(t *testing.T) {
	tl := NewImageTagList()
	for _, tagName := range []string{"latest", "v2026.01.30", "v2026.02.01"} {
		tl.Add(NewImageTag(tagName, time.Unix(0, 0), ""))
	}

	assert.Equal(t, []string{"v2026.01.30", "v2026.02.01"}, tl.SortByCalVer(context.Background(), nil).Tags())
}

func Test_SortByCalVer_PreservesTagMetadata(t *testing.T) {
	tl := NewImageTagList()
	tl.Add(NewImageTagWithLabels("v2026-01-30", time.Unix(0, 0), "sha256:abc", map[string]string{"foo": "bar"}))

	l, err := NewCalVerLayout("vYYYY-0M-0D")
	require.NoError(t, err)

	sorted := tl.SortByCalVer(context.Background(), l)
	require.Len(t, sorted, 1)
	assert.Equal(t, "sha256:abc", sorted[0].TagDigest)
	assert.Equal(t, map[string]string{"foo": "bar"}, sorted[0].Labels)
}

func Test_compareNatural(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"rc1", "rc2", -1},
		{"rc2", "rc10", -1},
		{"rc10", "rc2", 1},
		{"rc1", "rc1", 0},
		{"rc01", "rc1", 0},
		{"alpha", "beta", -1},
		{"alpha.2", "alpha.10", -1},
		{"rc", "rc1", -1},
		// Numbers no int could hold are still ordered by their value.
		{"rc99999999999999999999", "rc100000000000000000000", -1},
	}
	for _, tt := range tests {
		t.Run(tt.a+" vs "+tt.b, func(t *testing.T) {
			assert.Equal(t, tt.want, compareNatural(tt.a, tt.b))
		})
	}
}
