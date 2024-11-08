package tag

import (
	"fmt"
	"testing"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_NewImageTag(t *testing.T) {
	t.Run("New image tag from valid Time type", func(t *testing.T) {
		tagDate := time.Now()
		tag := NewImageTag("v1.0.0", tagDate, "")
		require.NotNil(t, tag)
		assert.Equal(t, "v1.0.0", tag.TagName)
		assert.Equal(t, &tagDate, tag.TagDate)
	})
}

func Test_ImageTagEqual(t *testing.T) {
	t.Run("Versions are similar", func(t *testing.T) {
		tag1 := NewImageTag("v1.0.0", time.Now(), "")
		tag2 := NewImageTag("v1.0.0", time.Now(), "")
		assert.True(t, tag1.Equals(tag2))
	})

	t.Run("Digests are similar but version is not", func(t *testing.T) {
		tag1 := NewImageTag("v1.0.0", time.Now(), "abcdef")
		tag2 := NewImageTag("v1.0.1", time.Now(), "abcdef")
		assert.True(t, tag1.Equals(tag2))
	})

	t.Run("Digests and versions are similar", func(t *testing.T) {
		tag1 := NewImageTag("v1.0.0", time.Now(), "abcdef")
		tag2 := NewImageTag("v1.0.0", time.Now(), "abcdef")
		assert.True(t, tag1.Equals(tag2))
	})

	t.Run("Versions are not similar", func(t *testing.T) {
		tag1 := NewImageTag("v1.0.0", time.Now(), "")
		tag2 := NewImageTag("v1.0.1", time.Now(), "")
		assert.False(t, tag1.Equals(tag2))
	})

	t.Run("Versions are not similar because digest is different", func(t *testing.T) {
		tag1 := NewImageTag("v1.0.0", time.Now(), "abc")
		tag2 := NewImageTag("v1.0.0", time.Now(), "def")
		assert.False(t, tag1.Equals(tag2))
	})

	t.Run("Versions are not similar because digest is missing", func(t *testing.T) {
		tag1 := NewImageTag("v1.0.0", time.Now(), "abc")
		tag2 := NewImageTag("v1.0.0", time.Now(), "")
		assert.False(t, tag1.Equals(tag2))
	})
}

func Test_AppendToImageTagList(t *testing.T) {
	t.Run("Append single entry to ImageTagList", func(t *testing.T) {
		il := NewImageTagList()
		tag := NewImageTag("v1.0.0", time.Now(), "")
		il.Add(tag)
		assert.Len(t, il.items, 1)
		assert.True(t, il.Contains(tag))
	})

	t.Run("Append two same entries to ImageTagList", func(t *testing.T) {
		il := NewImageTagList()
		tag := NewImageTag("v1.0.0", time.Now(), "")
		il.Add(tag)
		tag = NewImageTag("v1.0.0", time.Now(), "")
		il.Add(tag)
		assert.True(t, il.Contains(tag))
		assert.Len(t, il.items, 1)
	})

	t.Run("Append two distinct entries to ImageTagList", func(t *testing.T) {
		il := NewImageTagList()
		tag1 := NewImageTag("v1.0.0", time.Now(), "")
		il.Add(tag1)
		tag2 := NewImageTag("v1.0.1", time.Now(), "")
		il.Add(tag2)
		assert.True(t, il.Contains(tag1))
		assert.True(t, il.Contains(tag2))
		assert.Len(t, il.items, 2)
	})
}

func Test_SortableImageTagList(t *testing.T) {
	t.Run("Sort by name", func(t *testing.T) {
		names := []string{"wohoo", "bazar", "alpha", "jesus", "zebra"}
		il := NewImageTagList()
		for _, name := range names {
			tag := NewImageTag(name, time.Now(), "")
			il.Add(tag)
		}
		sil := il.SortAlphabetically()
		require.Len(t, sil, len(names))
		assert.Equal(t, "alpha", sil[0].TagName)
		assert.Equal(t, "bazar", sil[1].TagName)
		assert.Equal(t, "jesus", sil[2].TagName)
		assert.Equal(t, "wohoo", sil[3].TagName)
		assert.Equal(t, "zebra", sil[4].TagName)
	})

	t.Run("Sort by semver", func(t *testing.T) {
		names := []string{"v2.0.2", "v1.0", "v2.0.0", "v1.0.1", "v2.0.3", "v2.0"}
		il := NewImageTagList()
		for _, name := range names {
			tag := NewImageTag(name, time.Now(), "")
			il.Add(tag)
		}
		sil := il.SortBySemVer()
		require.Len(t, sil, len(names))
		assert.Equal(t, "v1.0", sil[0].TagName)
		assert.Equal(t, "v1.0.1", sil[1].TagName)
		assert.Equal(t, "v2.0", sil[2].TagName)
		assert.Equal(t, "v2.0.0", sil[3].TagName)
		assert.Equal(t, "v2.0.2", sil[4].TagName)
		assert.Equal(t, "v2.0.3", sil[5].TagName)
	})

	t.Run("Sort by date", func(t *testing.T) {
		names := []string{"v2.0.2", "v1.0", "v1.0.1", "v2.0.3", "v2.0"}
		dates := []int64{4, 1, 0, 3, 2}
		il := NewImageTagList()
		for i, name := range names {
			tag := NewImageTag(name, time.Unix(dates[i], 0), "")
			il.Add(tag)
		}
		sil := il.SortByDate()
		require.Len(t, sil, len(names))
		assert.Equal(t, "v1.0.1", sil[0].TagName)
		assert.Equal(t, "v1.0", sil[1].TagName)
		assert.Equal(t, "v2.0", sil[2].TagName)
		assert.Equal(t, "v2.0.3", sil[3].TagName)
		assert.Equal(t, "v2.0.2", sil[4].TagName)
	})

	t.Run("Sort by date with same dates", func(t *testing.T) {
		names := []string{"v2.0.2", "v1.0", "v1.0.1", "v2.0.3", "v2.0"}
		date := time.Unix(0, 0)
		il := NewImageTagList()
		for _, name := range names {
			tag := NewImageTag(name, date, "")
			il.Add(tag)
		}
		sil := il.SortByDate()
		require.Len(t, sil, len(names))
		assert.Equal(t, "v1.0", sil[0].TagName)
		assert.Equal(t, "v1.0.1", sil[1].TagName)
		assert.Equal(t, "v2.0", sil[2].TagName)
		assert.Equal(t, "v2.0.2", sil[3].TagName)
		assert.Equal(t, "v2.0.3", sil[4].TagName)
	})
}

func Test_TagsFromTagList(t *testing.T) {
	t.Run("Get list of tags from ImageTagList", func(t *testing.T) {
		names := []string{"wohoo", "bazar", "alpha", "jesus", "zebra"}
		il := NewImageTagList()
		for _, name := range names {
			tag := NewImageTag(name, time.Now(), "")
			il.Add(tag)
		}
		tl := il.Tags()
		assert.NotEmpty(t, tl)
		assert.Len(t, tl, len(names))
	})

	t.Run("Get list of tags from SortableImageTagList", func(t *testing.T) {
		names := []string{"wohoo", "bazar", "alpha", "jesus", "zebra"}
		sil := SortableImageTagList{}
		for _, name := range names {
			tag := NewImageTag(name, time.Now(), "")
			sil = append(sil, tag)
		}
		tl := sil.Tags()
		assert.NotEmpty(t, tl)
		assert.Len(t, tl, len(names))
	})
}

func Test_modifiedSemverCollection_Less(t *testing.T) {
	tests := []struct {
		name string
		s    modifiedSemverCollection
		want bool
	}{
		{
			name: "hf/hf",
			s:    newTestCollection(t, "hfHF"),
			want: true,
		}, {
			name: "hf/rc",
			s:    newTestCollection(t, "hfRC"),
			want: false,
		}, {
			name: "hf/pr",
			s:    newTestCollection(t, "hfpr"),
			want: false,
		}, {
			name: "pr/pr",
			s:    newTestCollection(t, "prPR"),
			want: true,
		}, {
			name: "pr/hf",
			s:    newTestCollection(t, "prhf"),
			want: true,
		}, {
			name: "pr/rc",
			s:    newTestCollection(t, "prrc"),
			want: true,
		}, {
			name: "rc/hf",
			s:    newTestCollection(t, "RChf"),
			want: true,
		}, {
			name: "rc/pr",
			s:    newTestCollection(t, "rcpr"),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.s.Less(0, 1); got != tt.want {
				t.Errorf("modifiedSemverCollection.Less() %v should be less than, %v but got result %v", tt.s[0].Original(), tt.s[0].Original(), got)
			}
		})
	}
}

func Test_extractBaseAndSuffix(t *testing.T) {
	tests := []struct {
		name     string
		version  string
		wantBase string
		wantEnv  string
		wantSuf  int
	}{
		{
			name:     "prod",
			version:  "1.2.3-prod",
			wantBase: "1.2.3",
			wantEnv:  "prod",
			wantSuf:  0,
		}, {
			name:     "hotfix",
			version:  "1.2.3-hotfix.123",
			wantBase: "1.2.3",
			wantEnv:  "hotfix",
			wantSuf:  123,
		}, {
			name:     "release-candidate",
			version:  "1.2.3-release-candidate.123",
			wantBase: "1.2.3",
			wantEnv:  "release-candidate",
			wantSuf:  123,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, env, suf := extractBaseAndSuffix(tt.version)
			if base != tt.wantBase {
				t.Errorf("extractBaseAndSuffix() got = %v, want %v", base, tt.wantBase)
			}
			if env != tt.wantEnv {
				t.Errorf("extractBaseAndSuffix() got = %v, want %v", env, tt.wantEnv)
			}
			if suf != tt.wantSuf {
				t.Errorf("extractBaseAndSuffix() got1 = %v, want %v", suf, tt.wantSuf)
			}
		})
	}
}

func newTestCollection(t *testing.T, kind string) modifiedSemverCollection {
	t.Helper()
	prdH, _ := semver.NewVersion("1.2.5-prod")
	prdL, _ := semver.NewVersion("1.2.3-prod")
	hfH, _ := semver.NewVersion("1.2.3-hotfix.2")
	hfL, _ := semver.NewVersion("1.2.3-hotfix.1")
	rcH, _ := semver.NewVersion("1.2.3-release-candidate.3")
	rcL, _ := semver.NewVersion("1.2.3-release-candidate.1")

	fmt.Printf("creating kind: %v\n", kind)
	switch kind {
	case "hfHF":
		return modifiedSemverCollection{hfL, hfH}
	case "HFhf":
		return modifiedSemverCollection{hfH, hfL}
	case "rcRC":
		return modifiedSemverCollection{rcL, hfH}
	case "RCrc":
		return modifiedSemverCollection{rcH, hfL}
	case "PRpr":
		return modifiedSemverCollection{prdH, prdL}
	case "prPR":
		return modifiedSemverCollection{prdL, prdH}
	case "prhf":
		return modifiedSemverCollection{prdL, hfL}
	case "prrc":
		return modifiedSemverCollection{prdL, rcL}
	case "rcpr":
		return modifiedSemverCollection{rcL, prdL}
	case "RCpr":
		return modifiedSemverCollection{rcH, prdL}
	case "RChf":
		return modifiedSemverCollection{rcH, hfL}
	case "hfRC":
		return modifiedSemverCollection{hfL, rcH}
	case "hfpr":
		return modifiedSemverCollection{hfL, prdL}
	default:
		return nil
	}
}
