package aws

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/tag"
)

// PushEventInfo is the image metadata carried by an ECR push notification.
type PushEventInfo struct {
	Tag      string
	Digest   string
	PushedAt time.Time
}

// ResolveEventTagInput configures single-tag resolution for event-driven updates.
type ResolveEventTagInput struct {
	ClientConfig            ClientConfig
	Repository              string
	Event                   PushEventInfo
	FallbackOnDescribeError bool
}

// ResolveEventTag resolves digest and push time for one known tag without listing
// the repository. When DescribeImages fails and FallbackOnDescribeError is set,
// metadata is taken from the push event payload.
func ResolveEventTag(ctx context.Context, in ResolveEventTagInput) (*tag.ImageTag, error) {
	if in.Repository == "" {
		return nil, fmt.Errorf("repository name is required")
	}
	if in.Event.Tag == "" {
		return nil, fmt.Errorf("event tag is required")
	}

	imgTag, err := DescribeImageTag(ctx, in.ClientConfig, in.Repository, in.Event.Tag)
	if err == nil {
		return imgTag, nil
	}
	if errors.Is(err, ErrSkippedMediaType) {
		return nil, err
	}
	if !in.FallbackOnDescribeError {
		return nil, err
	}
	if in.Event.Digest == "" {
		return nil, fmt.Errorf("describe failed and event digest is empty: %w", err)
	}

	pushedAt := in.Event.PushedAt
	if pushedAt.IsZero() {
		pushedAt = time.Now().UTC()
	}
	return tag.NewImageTag(in.Event.Tag, pushedAt, NormalizeDigest(in.Event.Digest)), nil
}
