package argocd

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/tag"
)

func Test_WriteBackTargetKey(t *testing.T) {
	t.Run("Same repo, branch, and target produce the same key", func(t *testing.T) {
		wbc1 := &WriteBackConfig{GitRepo: "https://github.com/org/repo.git", GitBranch: "main", Target: "overlays/dev/values.yaml"}
		wbc2 := &WriteBackConfig{GitRepo: "https://github.com/org/repo.git", GitBranch: "main", Target: "overlays/dev/values.yaml"}
		assert.Equal(t, wbc1.WriteBackTargetKey(), wbc2.WriteBackTargetKey())
	})

	t.Run("Different repo produces a different key", func(t *testing.T) {
		wbc1 := &WriteBackConfig{GitRepo: "https://github.com/org/repo-a.git", GitBranch: "main", Target: "values.yaml"}
		wbc2 := &WriteBackConfig{GitRepo: "https://github.com/org/repo-b.git", GitBranch: "main", Target: "values.yaml"}
		assert.NotEqual(t, wbc1.WriteBackTargetKey(), wbc2.WriteBackTargetKey())
	})

	t.Run("Different branch produces a different key", func(t *testing.T) {
		wbc1 := &WriteBackConfig{GitRepo: "https://github.com/org/repo.git", GitBranch: "main", Target: "values.yaml"}
		wbc2 := &WriteBackConfig{GitRepo: "https://github.com/org/repo.git", GitBranch: "staging", Target: "values.yaml"}
		assert.NotEqual(t, wbc1.WriteBackTargetKey(), wbc2.WriteBackTargetKey())
	})

	t.Run("Different target produces a different key", func(t *testing.T) {
		wbc1 := &WriteBackConfig{GitRepo: "https://github.com/org/repo.git", GitBranch: "main", Target: "overlays/dev/values.yaml"}
		wbc2 := &WriteBackConfig{GitRepo: "https://github.com/org/repo.git", GitBranch: "main", Target: "overlays/prod/values.yaml"}
		assert.NotEqual(t, wbc1.WriteBackTargetKey(), wbc2.WriteBackTargetKey())
	})

	t.Run("KustomizeBase used when Target is empty", func(t *testing.T) {
		wbc1 := &WriteBackConfig{GitRepo: "https://github.com/org/repo.git", GitBranch: "main", KustomizeBase: "overlays/dev"}
		wbc2 := &WriteBackConfig{GitRepo: "https://github.com/org/repo.git", GitBranch: "main", KustomizeBase: "overlays/dev"}
		assert.Equal(t, wbc1.WriteBackTargetKey(), wbc2.WriteBackTargetKey())
	})

	t.Run("Key is 8 hex characters", func(t *testing.T) {
		wbc := &WriteBackConfig{GitRepo: "https://github.com/org/repo.git", GitBranch: "main", Target: "values.yaml"}
		key := wbc.WriteBackTargetKey()
		assert.Len(t, key, 8)
	})
}

func Test_ChangeEntry_FormatTag(t *testing.T) {
	t.Run("Pinning disabled preserves digest-only reporting", func(t *testing.T) {
		change := ChangeEntry{}
		version := tag.NewImageTag("1.0", time.Now(), "sha256:abc")
		assert.Equal(t, "sha256:abc", change.FormatTag(version))
	})

	t.Run("Pinning enabled reports tag and digest", func(t *testing.T) {
		change := ChangeEntry{PinDigest: true}
		version := tag.NewImageTag("1.0", time.Now(), "sha256:abc")
		assert.Equal(t, "1.0@sha256:abc", change.FormatTag(version))
	})

	t.Run("Pinning enabled preserves partial version information", func(t *testing.T) {
		change := ChangeEntry{PinDigest: true}
		assert.Equal(t, "1.0", change.FormatTag(tag.NewImageTag("1.0", time.Now(), "")))
		assert.Equal(t, "sha256:abc", change.FormatTag(tag.NewImageTag("", time.Now(), "sha256:abc")))
		assert.Empty(t, change.FormatTag(tag.NewImageTag("", time.Now(), "")))
	})
}

func Test_MarkPRCreated(t *testing.T) {
	t.Run("First call returns true, second returns false", func(t *testing.T) {
		state := NewSyncIterationState()
		assert.True(t, state.MarkPRCreated("target-a"))
		assert.False(t, state.MarkPRCreated("target-a"))
	})

	t.Run("Different keys both return true", func(t *testing.T) {
		state := NewSyncIterationState()
		assert.True(t, state.MarkPRCreated("target-a"))
		assert.True(t, state.MarkPRCreated("target-b"))
	})

	t.Run("Thread safety", func(t *testing.T) {
		state := NewSyncIterationState()
		var wg sync.WaitGroup
		results := make([]bool, 10)
		for i := range 10 {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				results[idx] = state.MarkPRCreated("same-key")
			}(i)
		}
		wg.Wait()

		trueCount := 0
		for _, r := range results {
			if r {
				trueCount++
			}
		}
		assert.Equal(t, 1, trueCount, "exactly one goroutine should get true")
	})
}

func Test_ReleasePRReservation(t *testing.T) {
	state := NewSyncIterationState()
	assert.True(t, state.MarkPRCreated("target-a"))
	assert.True(t, state.MarkPRCreated("target-b"))
	state.ReleasePRReservation("target-a")
	assert.True(t, state.MarkPRCreated("target-a"))
	assert.False(t, state.MarkPRCreated("target-b"))
}

func Test_IsValidGitCommitMethod(t *testing.T) {
	assert.True(t, IsValidGitCommitMethod(""))
	assert.True(t, IsValidGitCommitMethod("git"))
	assert.True(t, IsValidGitCommitMethod("api"))
	assert.False(t, IsValidGitCommitMethod("API"))
	assert.False(t, IsValidGitCommitMethod("graphql"))
}
