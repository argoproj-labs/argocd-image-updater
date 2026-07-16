package argocd

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
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
		for i := 0; i < 10; i++ {
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
