package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd"
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

	// If secret is configured, validate the signature
	if h.secret != "" {
		signature := r.Header.Get("Authorization")
		if signature == "" {
			return fmt.Errorf("missing webhook signature")
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			return fmt.Errorf("failed to read request body: %w", err)
		}

		// Reset body for later reading
		r.Body = io.NopCloser(strings.NewReader(string(body)))

		if !h.validateSignature(body, signature) {
			return fmt.Errorf("invalid webhook signature")
		}
	}

	return nil
}

// Parse processes the Harbor webhook payload and returns a WebhookEvent
func (h *HarborWebhook) Parse(r *http.Request) (*argocd.WebhookEvent, error) {
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

	return &argocd.WebhookEvent{
		RegistryURL: registryURL,
		Repository:  repository,
		Tag:         resource.Tag,
		Digest:      resource.Digest,
	}, nil
}

// validateSignature validates the webhook signature using HMAC-SHA256
func (h *HarborWebhook) validateSignature(body []byte, signature string) bool {
	// Harbor signature format can vary, commonly uses sha256=<hex> in Authorization header
	var expectedSig string

	if strings.HasPrefix(signature, "sha256=") {
		expectedSig = signature[7:] // Remove "sha256=" prefix
	} else if strings.HasPrefix(signature, "Bearer ") {
		expectedSig = signature[7:] // Remove "Bearer " prefix
	} else {
		expectedSig = signature
	}

	mac := hmac.New(sha256.New, []byte(h.secret))
	mac.Write(body)
	calculatedSig := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(expectedSig), []byte(calculatedSig))
}
