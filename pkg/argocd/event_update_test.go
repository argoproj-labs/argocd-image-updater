package argocd

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/tag"
)

func Test_eventCandidateIsNotNewerThan(t *testing.T) {
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
			name:            "same digest is stale",
			candidate:       tag.NewImageTag("main-42", newer, "sha256:abc"),
			currentTag:      tag.NewImageTag("main-43", older, "sha256:abc"),
			currentPushedAt: older,
			wantStale:       true,
		},
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
			got := eventCandidateIsNotNewerThan(tt.candidate, tt.currentTag, tt.currentPushedAt)
			assert.Equal(t, tt.wantStale, got)
		})
	}
}
