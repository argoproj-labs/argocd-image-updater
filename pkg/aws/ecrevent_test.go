package aws_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argoproj-labs/argocd-image-updater/pkg/aws"
)

func TestParseECREventBridgeMessage_PUSH(t *testing.T) {
	body := []byte(`{
		"version": "0",
		"id": "abc-123",
		"detail-type": "ECR Image Action",
		"source": "aws.ecr",
		"account": "123456789012",
		"time": "2024-01-15T12:00:00Z",
		"region": "us-east-1",
		"detail": {
			"result": "SUCCESS",
			"repository-name": "my-app",
			"image-digest": "sha256:abcdef123456",
			"action-type": "PUSH",
			"image-tag": "main-42"
		}
	}`)

	event, err := aws.ParseECREventBridgeMessage(body)
	require.NoError(t, err)
	assert.Equal(t, "123456789012.dkr.ecr.us-east-1.amazonaws.com", event.RegistryURL)
	assert.Equal(t, "my-app", event.Repository)
	assert.Equal(t, "main-42", event.Tag)
	assert.Equal(t, "sha256:abcdef123456", event.Digest)
}

func TestParseECREventBridgeMessage_nonPush(t *testing.T) {
	body := []byte(`{
		"version": "0",
		"detail-type": "ECR Image Action",
		"source": "aws.ecr",
		"account": "123456789012",
		"region": "us-east-1",
		"detail": {
			"result": "SUCCESS",
			"repository-name": "my-app",
			"image-digest": "sha256:abcdef123456",
			"action-type": "DELETE",
			"image-tag": "main-42"
		}
	}`)

	_, err := aws.ParseECREventBridgeMessage(body)
	require.Error(t, err)
	assert.ErrorIs(t, err, aws.ErrSkippedEvent)
}

func TestParseECREventBridgeMessage_failedPush(t *testing.T) {
	body := []byte(`{
		"version": "0",
		"detail-type": "ECR Image Action",
		"source": "aws.ecr",
		"account": "123456789012",
		"region": "us-east-1",
		"detail": {
			"result": "FAILED",
			"repository-name": "my-app",
			"image-digest": "sha256:abcdef123456",
			"action-type": "PUSH",
			"image-tag": "main-42"
		}
	}`)

	_, err := aws.ParseECREventBridgeMessage(body)
	require.Error(t, err)
	assert.ErrorIs(t, err, aws.ErrSkippedEvent)
}

func TestBuildECRRegistryURL(t *testing.T) {
	assert.Equal(t, "123456789012.dkr.ecr.us-east-1.amazonaws.com", aws.BuildECRRegistryURL("123456789012", "us-east-1"))
}

func TestIsECRRegistryURL(t *testing.T) {
	assert.True(t, aws.IsECRRegistryURL("123456789012.dkr.ecr.us-east-1.amazonaws.com"))
	assert.True(t, aws.IsECRRegistryURL("https://123456789012.dkr.ecr.eu-west-1.amazonaws.com"))
	assert.False(t, aws.IsECRRegistryURL("ghcr.io"))
}

func TestParseECRRegistryURL(t *testing.T) {
	account, region, ok := aws.ParseECRRegistryURL("123456789012.dkr.ecr.us-west-2.amazonaws.com")
	require.True(t, ok)
	assert.Equal(t, "123456789012", account)
	assert.Equal(t, "us-west-2", region)
}
