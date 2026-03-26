package webhook

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// HarborWebhook handles Harbor Registry webhook events
type HarborWebhook struct {
	secret string
}

// NewHarborWebhook creates a new Harbor webhook handler
func NewHarborWebhook(secret string) *HarborWebhook {
	return &HarborWebhook{
		secret: secret,
	}
}

// GetRegistryType returns the registry type this handler supports
func (h *HarborWebhook) GetRegistryType() string {
	return "harbor"
}

// Validate validates the Harbor webhook payload
func (h *HarborWebhook) Validate(r *http.Request) error {
	if r.Method != http.MethodPost {
		return fmt.Errorf("invalid HTTP method: %s", r.Method)
	}

	// Check for Harbor webhook headers
	eventType := r.Header.Get("Content-Type")
	if !strings.Contains(eventType, "application/json") {
		return fmt.Errorf("invalid content type: %s", eventType)
	}

	// If secret is configured, validate the secret
	// See: https://github.com/akuity/kargo/blob/main/pkg/webhook/external/harbor.go
	if h.secret != "" {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			return fmt.Errorf("missing Authorization header when secret is configured")
		}

		// Harbor sends plain secret value directly in Authorization header for external webhooks
		if subtle.ConstantTimeCompare([]byte(authHeader), []byte(h.secret)) != 1 {
			return fmt.Errorf("incorrect webhook secret")
		}
	}

	return nil
}

// Parse processes the Harbor webhook payload and returns a WebhookEvent
func (h *HarborWebhook) Parse(r *http.Request) (*WebhookEvent, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}

	var payload struct {
		Type      string `json:"type"`
		OccurAt   int64  `json:"occur_at"`
		Operator  string `json:"operator"`
		EventData struct {
			Resources []struct {
				Digest      string `json:"digest"`
				Tag         string `json:"tag"`
				ResourceURL string `json:"resource_url"`
			} `json:"resources"`
			Repository struct {
				DateCreated  int64  `json:"date_created"`
				Name         string `json:"name"`
				Namespace    string `json:"namespace"`
				RepoFullName string `json:"repo_full_name"`
				RepoType     string `json:"repo_type"`
			} `json:"repository"`
			CustomAttributes map[string]interface{} `json:"custom_attributes"`
		} `json:"event_data"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse webhook payload: %w", err)
	}

	// Only process PUSH_ARTIFACT events
	if payload.Type != "PUSH_ARTIFACT" {
		return nil, fmt.Errorf("unsupported event type: %s", payload.Type)
	}

	if len(payload.EventData.Resources) == 0 {
		return nil, fmt.Errorf("no resources found in webhook payload")
	}

	resource := payload.EventData.Resources[0]
	if resource.Tag == "" {
		return nil, fmt.Errorf("tag not found in webhook payload")
	}

	// Extract repository name
	repository := payload.EventData.Repository.RepoFullName
	if repository == "" {
		if payload.EventData.Repository.Namespace != "" && payload.EventData.Repository.Name != "" {
			repository = fmt.Sprintf("%s/%s", payload.EventData.Repository.Namespace, payload.EventData.Repository.Name)
		} else {
			repository = payload.EventData.Repository.Name
		}
	}

	if repository == "" {
		return nil, fmt.Errorf("repository name not found in webhook payload")
	}

	// Extract registry URL from resource URL
	registryURL := "harbor" // Default fallback value
	if resource.ResourceURL != "" {
		// Harbor resource_url might not have a protocol scheme
		// e.g., "registry.entrade.com.vn/private/dnse/krx-derivative-asset-service:tag"
		resourceURL := resource.ResourceURL

		// Add https:// scheme if missing for parsing
		if !strings.HasPrefix(resourceURL, "http://") && !strings.HasPrefix(resourceURL, "https://") {
			resourceURL = "https://" + resourceURL
		}

		if parsedURL, err := url.Parse(resourceURL); err == nil {
			registryURL = parsedURL.Host
		} else {
			// Fallback: try to extract host manually by splitting on the first '/'
			parts := strings.Split(resource.ResourceURL, "/")
			if len(parts) > 0 && strings.Contains(parts[0], ".") {
				registryURL = parts[0]
			}
			// If manual extraction fails, keep the default "harbor" value
		}
	}

	return &WebhookEvent{
		RegistryURL: registryURL,
		Repository:  repository,
		Tag:         resource.Tag,
		Digest:      resource.Digest,
	}, nil
}
