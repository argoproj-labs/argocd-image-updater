package aws_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argoproj-labs/argocd-image-updater/pkg/aws"
)

func TestResolveEventTag_requiresRepositoryAndTag(t *testing.T) {
	_, err := aws.ResolveEventTag(context.Background(), aws.ResolveEventTagInput{})
	require.Error(t, err)

	_, err = aws.ResolveEventTag(context.Background(), aws.ResolveEventTagInput{
		Repository: "demo-app",
	})
	require.Error(t, err)
}

func TestResolveEventTag_fallbackUsesEventMetadata(t *testing.T) {
	when := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	tagInfo, err := aws.ResolveEventTag(context.Background(), aws.ResolveEventTagInput{
		ClientConfig: aws.ClientConfig{
			Region:      "us-east-1",
			EndpointURL: "http://localhost:4566",
		},
		Repository: "demo-app",
		Event: aws.PushEventInfo{
			Tag:      "dev-test",
			Digest:   "abc123",
			PushedAt: when,
		},
		FallbackOnDescribeError: true,
	})
	require.NoError(t, err)
	require.NotNil(t, tagInfo.TagDate)
	assert.Equal(t, when, tagInfo.TagDate.UTC())
	assert.Equal(t, "sha256:abc123", tagInfo.TagDigest)
}

func TestResolveEventTag_noFallbackReturnsDescribeError(t *testing.T) {
	_, err := aws.ResolveEventTag(context.Background(), aws.ResolveEventTagInput{
		ClientConfig: aws.ClientConfig{
			Region:      "us-east-1",
			EndpointURL: "http://localhost:4566",
		},
		Repository: "demo-app",
		Event: aws.PushEventInfo{
			Tag:    "dev-test",
			Digest: "abc123",
		},
		FallbackOnDescribeError: false,
	})
	require.Error(t, err)
}
