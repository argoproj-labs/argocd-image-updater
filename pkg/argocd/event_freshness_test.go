package argocd

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/tag"
)

func Test_eventIsNotNewerThan(t *testing.T) {
	older := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	newer := time.Date(2024, 1, 2, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name            string
		candidate       *tag.ImageTag
		currentTag      *tag.ImageTag
		currentPushedAt time.Time
		wantStale       bool
	}{
		{
			name:            "older candidate push time is stale",
			candidate:       tag.NewImageTag("main-42", older, "sha256:old"),
			currentTag:      tag.NewImageTag("main-43", newer, "sha256:new"),
			currentPushedAt: newer,
			wantStale:       true,
		},
		{
			name:            "newer candidate push time is not stale",
			candidate:       tag.NewImageTag("main-44", newer, "sha256:newer"),
			currentTag:      tag.NewImageTag("main-43", older, "sha256:old"),
			currentPushedAt: older,
			wantStale:       false,
		},
		{
			name:            "equal push time is stale",
			candidate:       tag.NewImageTag("main-42", older, "sha256:other"),
			currentTag:      tag.NewImageTag("main-43", older, "sha256:current"),
			currentPushedAt: older,
			wantStale:       true,
		},
		{
			name:            "missing candidate push time does not block",
			candidate:       tag.NewImageTag("main-42", time.Time{}, "sha256:other"),
			currentTag:      tag.NewImageTag("main-43", older, "sha256:current"),
			currentPushedAt: older,
			wantStale:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := eventIsNotNewerThan(tt.candidate, tt.currentTag, tt.currentPushedAt, time.Time{})
			assert.Equal(t, tt.wantStale, got)
		})
	}
}

func TestEventFreshnessStore_IsOutOfOrder(t *testing.T) {
	store := NewEventFreshnessStore()
	older := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	newer := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)

	store.RecordApplied("demo-app", "dev-stale-newer", newer)

	assert.True(t, store.IsOutOfOrder("demo-app", "dev-stale-older", older))
	assert.False(t, store.IsOutOfOrder("demo-app", "dev-stale-newer", older))
	assert.False(t, store.IsOutOfOrder("demo-app", "dev-stale-newer", newer))
}

func TestEventFreshnessStore_TagPushTime(t *testing.T) {
	store := NewEventFreshnessStore()
	when := time.Date(2026, 6, 3, 12, 0, 0, 0, time.UTC)
	store.RecordApplied("demo-app", "dev-stale-newer", when)

	got, ok := store.TagPushTime("demo-app", "dev-stale-newer")
	assert.True(t, ok)
	assert.Equal(t, when, got)
}
