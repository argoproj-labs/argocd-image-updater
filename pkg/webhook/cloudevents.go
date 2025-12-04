package webhook

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/sirupsen/logrus"

	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
)

// CloudEventsWebhook handles CloudEvents webhook events
// CloudEvents is a CNCF specification for describing event data in a common way
// This handler supports events transformed by AWS EventBridge from ECR push events.
type CloudEventsWebhook struct {
	secret string
}

// NewCloudEventsWebhook creates a new CloudEvents webhook handler
func NewCloudEventsWebhook(secret string) *CloudEventsWebhook {
	return &CloudEventsWebhook{
		secret: secret,
	}
}

// GetRegistryType returns the registry type this handler supports
func (c *CloudEventsWebhook) GetRegistryType() string {
	return "cloudevents"
}

// Validate validates the CloudEvents webhook payload
func (c *CloudEventsWebhook) Validate(r *http.Request) error {
	webhookLogger := log.Log().WithFields(logrus.Fields{
		"logger": "cloudevents-webhook",
	})
	ctx := log.ContextWithLogger(r.Context(), webhookLogger)
	logCtx := log.LoggerFromContext(ctx)

	logCtx.Tracef("Validating request: method=%s, content-type=%s", r.Method, r.Header.Get("Content-Type"))

	if r.Method != http.MethodPost {
		return fmt.Errorf("invalid HTTP method: %s", r.Method)
	}

	// CloudEvents can use either structured or binary content mode
	// For structured mode, check Content-Type
	contentType := r.Header.Get("Content-Type")
	if contentType != "" && !strings.Contains(contentType, "application/json") &&
		!strings.Contains(contentType, "application/cloudevents+json") {
		return fmt.Errorf("invalid content type: %s", contentType)
	}

	// If secret is configured, validate it
	// CloudEvents doesn't have a standard authentication mechanism,
	// so we support a simple query parameter or header-based secret
	if c.secret != "" {
		// Check for secret in query parameter
		secret := r.URL.Query().Get("secret")
		if secret == "" {
			// Check for secret in header
			secret = r.Header.Get("X-Webhook-Secret")
		}

		if secret == "" {
			return fmt.Errorf("missing webhook secret")
		}

		if subtle.ConstantTimeCompare([]byte(secret), []byte(c.secret)) != 1 {
			return fmt.Errorf("invalid webhook secret")
		}
	}

	return nil
}

// Parse processes the CloudEvents webhook payload and returns a WebhookEvent
func (c *CloudEventsWebhook) Parse(r *http.Request) (*argocd.WebhookEvent, error) {
	webhookLogger := log.Log().WithFields(logrus.Fields{
		"logger": "cloudevents-webhook",
	})
	ctx := log.ContextWithLogger(r.Context(), webhookLogger)
	logCtx := log.LoggerFromContext(ctx)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}

	logCtx.Tracef("Received payload: %s", string(body))

	// CloudEvents specification: https://github.com/cloudevents/spec/blob/v1.0.2/cloudevents/spec.md
	var payload struct {
		SpecVersion     string                 `json:"specversion"`
		Type            string                 `json:"type"`
		Source          string                 `json:"source"`
		Subject         string                 `json:"subject"`
		ID              string                 `json:"id"`
		Time            string                 `json:"time"`
		DataContentType string                 `json:"datacontenttype"`
		Data            map[string]interface{} `json:"data"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse webhook payload: %w", err)
	}

	logCtx.Tracef("Parsed CloudEvents: specversion=%s, type=%s, source=%s, subject=%s",
		payload.SpecVersion, payload.Type, payload.Source, payload.Subject)

	// Validate CloudEvents spec version
	if payload.SpecVersion == "" {
		return nil, fmt.Errorf("missing CloudEvents specversion")
	}

	if payload.Type == "" {
		return nil, fmt.Errorf("missing CloudEvents type")
	}

	// Parse the event based on the type
	return c.parseEvent(&payload, ctx)
}

// parseEvent extracts webhook event data from CloudEvents payload
func (c *CloudEventsWebhook) parseEvent(payload *struct {
	SpecVersion     string                 `json:"specversion"`
	Type            string                 `json:"type"`
	Source          string                 `json:"source"`
	Subject         string                 `json:"subject"`
	ID              string                 `json:"id"`
	Time            string                 `json:"time"`
	DataContentType string                 `json:"datacontenttype"`
	Data            map[string]interface{} `json:"data"`
}, ctx context.Context) (*argocd.WebhookEvent, error) {
	logCtx := log.LoggerFromContext(ctx)

	var repository, tag, digest, registryURL string

	// If subject is in format "repo:tag", parse it first as fallback
	// Use SplitN to handle tags containing colons (e.g., "repo:v1:hotfix")
	if parts := strings.SplitN(payload.Subject, ":", 2); len(parts) == 2 {
		repository = parts[0]
		tag = parts[1]
		logCtx.Tracef("Extracted from subject: repository=%s, tag=%s", repository, tag)
	}

	// Handle different event types
	switch {
	// AWS ECR push events
	case strings.HasPrefix(payload.Type, "com.amazon.ecr."):
		logCtx.Tracef("Processing AWS ECR event type: %s", payload.Type)

		// Extract from data (overrides subject if present)
		if repoName, ok := payload.Data["repositoryName"].(string); ok && repoName != "" {
			repository = repoName
		}
		if imageTag, ok := payload.Data["imageTag"].(string); ok && imageTag != "" {
			tag = imageTag
		}
		if imageDigest, ok := payload.Data["imageDigest"].(string); ok {
			digest = imageDigest
		}

		logCtx.Tracef("Extracted from ECR data: repository=%s, tag=%s, digest=%s", repository, tag, digest)

		// Extract registry URL from source
		// Source format: urn:aws:ecr:<region>:<account>:repository/<repo>
		registryURL = c.extractECRRegistryURL(payload.Source, payload.Data)
		logCtx.Tracef("Extracted ECR registry URL: %s", registryURL)

	// Generic container registry push events
	case strings.Contains(payload.Type, "container") || strings.Contains(payload.Type, "image"):
		logCtx.Tracef("Processing generic container/image event type: %s", payload.Type)

		// Try to extract from data (overrides subject if present)
		if repoName, ok := payload.Data["repositoryName"].(string); ok && repoName != "" {
			repository = repoName
		} else if repoName, ok := payload.Data["repository"].(string); ok && repoName != "" {
			repository = repoName
		}

		if imageTag, ok := payload.Data["imageTag"].(string); ok && imageTag != "" {
			tag = imageTag
		} else if imageTag, ok := payload.Data["tag"].(string); ok && imageTag != "" {
			tag = imageTag
		}

		if imageDigest, ok := payload.Data["imageDigest"].(string); ok {
			digest = imageDigest
		} else if imageDigest, ok := payload.Data["digest"].(string); ok {
			digest = imageDigest
		}

		logCtx.Tracef("Extracted from data: repository=%s, tag=%s, digest=%s", repository, tag, digest)

		// Try to extract registry URL from data or source
		if regURL, ok := payload.Data["registryUrl"].(string); ok {
			registryURL = regURL
		} else if regURL, ok := payload.Data["registry"].(string); ok {
			registryURL = regURL
		} else {
			registryURL = c.extractRegistryFromSource(payload.Source)
		}

		logCtx.Tracef("Extracted registry URL: %s", registryURL)

	default:
		return nil, fmt.Errorf("unsupported CloudEvents type: %s", payload.Type)
	}

	// Validate required fields
	if repository == "" {
		return nil, fmt.Errorf("repository name not found in CloudEvents payload")
	}

	if tag == "" {
		return nil, fmt.Errorf("tag not found in CloudEvents payload")
	}

	if registryURL == "" {
		return nil, fmt.Errorf("registry URL not found in CloudEvents payload")
	}

	logCtx.Tracef("Final webhook event: registry=%s, repository=%s, tag=%s, digest=%s",
		registryURL, repository, tag, digest)

	return &argocd.WebhookEvent{
		RegistryURL: registryURL,
		Repository:  repository,
		Tag:         tag,
		Digest:      digest,
	}, nil
}

// extractECRRegistryURL extracts ECR registry URL from CloudEvents source
// Source format: urn:aws:ecr:<region>:<account>:repository/<repo>
func (c *CloudEventsWebhook) extractECRRegistryURL(source string, data map[string]interface{}) string {
	// Try to extract from source URN
	if strings.HasPrefix(source, "urn:aws:ecr:") {
		parts := strings.Split(source, ":")
		if len(parts) >= 5 {
			region := parts[3]
			account := parts[4]

			// Use registryId from data if available (more reliable than source)
			if registryID, ok := data["registryId"].(string); ok && registryID != "" {
				account = registryID
			}

			return fmt.Sprintf("%s.dkr.ecr.%s.amazonaws.com", account, region)
		}
	}

	return ""
}

// extractRegistryFromSource extracts registry URL from CloudEvents source field
func (c *CloudEventsWebhook) extractRegistryFromSource(source string) string {
	// If source is a URL, extract the host
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		parts := strings.Split(source, "/")
		if len(parts) >= 3 {
			return parts[2]
		}
	}

	// If source contains a registry-like pattern (host.domain)
	if strings.Contains(source, ".") {
		parts := strings.Split(source, "/")
		if len(parts) > 0 && strings.Contains(parts[0], ".") {
			return parts[0]
		}
	}

	return ""
}
