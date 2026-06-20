package aws

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	eventSourceECR          = "aws.ecr"
	eventDetailTypeECRImage = "ECR Image Action"
	ecrActionPush           = "PUSH"
	ecrResultSuccess        = "SUCCESS"
)

// EventBridgeEnvelope is the top-level EventBridge event delivered to SQS.
type EventBridgeEnvelope struct {
	Version    string          `json:"version"`
	ID         string          `json:"id"`
	DetailType string          `json:"detail-type"`
	Source     string          `json:"source"`
	Account    string          `json:"account"`
	Time       string          `json:"time"`
	Region     string          `json:"region"`
	Detail     json.RawMessage `json:"detail"`
}

// ECRImageActionDetail is the detail payload for ECR Image Action events.
type ECRImageActionDetail struct {
	Result         string `json:"result"`
	RepositoryName string `json:"repository-name"`
	ImageDigest    string `json:"image-digest"`
	ActionType     string `json:"action-type"`
	ImageTag       string `json:"image-tag"`
}

// ParseECREventBridgeMessage parses an EventBridge ECR push event from an SQS message body.
func ParseECREventBridgeMessage(body []byte) (*ImagePushEvent, error) {
	var envelope EventBridgeEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("failed to parse EventBridge envelope: %w", err)
	}

	if envelope.Source != eventSourceECR {
		return nil, fmt.Errorf("%w: source %q", ErrSkippedEvent, envelope.Source)
	}
	if envelope.DetailType != eventDetailTypeECRImage {
		return nil, fmt.Errorf("%w: detail-type %q", ErrSkippedEvent, envelope.DetailType)
	}

	var detail ECRImageActionDetail
	if err := json.Unmarshal(envelope.Detail, &detail); err != nil {
		return nil, fmt.Errorf("failed to parse ECR event detail: %w", err)
	}

	if detail.ActionType != ecrActionPush {
		return nil, fmt.Errorf("%w: action-type %q", ErrSkippedEvent, detail.ActionType)
	}
	if detail.Result != ecrResultSuccess {
		return nil, fmt.Errorf("%w: result %q", ErrSkippedEvent, detail.Result)
	}
	if detail.RepositoryName == "" {
		return nil, fmt.Errorf("repository name not found in ECR event")
	}
	if detail.ImageTag == "" {
		return nil, fmt.Errorf("image tag not found in ECR event")
	}
	if envelope.Account == "" || envelope.Region == "" {
		return nil, fmt.Errorf("account or region missing from ECR event")
	}

	pushedAt := time.Time{}
	if envelope.Time != "" {
		parsed, err := time.Parse(time.RFC3339, envelope.Time)
		if err != nil {
			return nil, fmt.Errorf("failed to parse event time %q: %w", envelope.Time, err)
		}
		pushedAt = parsed.UTC()
	}

	return &ImagePushEvent{
		RegistryURL: BuildECRRegistryURL(envelope.Account, envelope.Region),
		Repository:  detail.RepositoryName,
		Tag:         detail.ImageTag,
		Digest:      detail.ImageDigest,
		PushedAt:    pushedAt,
	}, nil
}

// BuildECRRegistryURL returns the ECR registry host for an account and region.
func BuildECRRegistryURL(account, region string) string {
	return fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com", account, region)
}

// IsECRRegistryURL reports whether registryURL points at an AWS ECR registry.
func IsECRRegistryURL(registryURL string) bool {
	registryURL = strings.TrimPrefix(registryURL, "https://")
	registryURL = strings.TrimPrefix(registryURL, "http://")
	return strings.Contains(registryURL, ".dkr.ecr.") && strings.Contains(registryURL, "amazonaws.com")
}

// ParseECRRegistryURL extracts account ID and region from an ECR registry URL.
func ParseECRRegistryURL(registryURL string) (account, region string, ok bool) {
	registryURL = strings.TrimPrefix(registryURL, "https://")
	registryURL = strings.TrimPrefix(registryURL, "http://")
	parts := strings.Split(registryURL, ".")
	if len(parts) < 6 || parts[1] != "dkr" || parts[2] != "ecr" {
		return "", "", false
	}
	return parts[0], parts[3], true
}
