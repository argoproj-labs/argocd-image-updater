package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// GHCRWebhook handles GitHub Container Registry webhook events
type GHCRWebhook struct {
	secret string
}

// NewGHCRWebhook creates a new GHCR webhook handler
func NewGHCRWebhook(secret string) *GHCRWebhook {
	return &GHCRWebhook{
		secret: secret,
	}
}

// GetRegistryType returns the registry type this handler supports
func (g *GHCRWebhook) GetRegistryType() string {
	return "ghcr.io"
}

// Validate validates the GHCR webhook payload
func (g *GHCRWebhook) Validate(r *http.Request) error {
	if r.Method != http.MethodPost {
		return fmt.Errorf("invalid HTTP method: %s", r.Method)
	}

	// Check for GitHub webhook headers
	eventType := r.Header.Get("X-GitHub-Event")
	if eventType == "" {
		return fmt.Errorf("missing X-GitHub-Event header")
	}

	// We're only interested in package events
	if eventType != "package" {
		return fmt.Errorf("unsupported event type: %s", eventType)
	}

	// If secret is configured, validate the signature
	if g.secret != "" {
		signature := r.Header.Get("X-Hub-Signature-256")
		if signature == "" {
			return fmt.Errorf("missing webhook signature")
		}

		body, err := io.ReadAll(r.Body)
		if err != nil {
			return fmt.Errorf("failed to read request body: %w", err)
		}

		// Reset body for later reading
		r.Body = io.NopCloser(strings.NewReader(string(body)))

		if !g.validateSignature(body, signature) {
			return fmt.Errorf("invalid webhook signature")
		}
	}

	return nil
}

// Parse processes the GHCR webhook payload and returns a WebhookEvent
func (g *GHCRWebhook) Parse(r *http.Request) (*WebhookEvent, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}

	var payload struct {
		Action  string `json:"action"`
		Package struct {
			Name        string `json:"name"`
			PackageType string `json:"package_type"`
			Owner       struct {
				Login string `json:"login"`
			} `json:"owner"`
			Registry struct {
				URL string `json:"url"`
			} `json:"registry"`
			PackageVersion struct {
				Version           string `json:"version"`
				Name              string `json:"name"`
				ContainerMetadata struct {
					Tag struct {
						Name string `json:"name"`
					} `json:"tag"`
				} `json:"container_metadata"`
			} `json:"package_version"`
		} `json:"package"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse webhook payload: %w", err)
	}

	// Only process published packages
	if payload.Action != "published" {
		return nil, fmt.Errorf("ignoring action: %s", payload.Action)
	}

	// Only process container packages
	if payload.Package.PackageType != "container" {
		return nil, fmt.Errorf("unsupported package type: %s", payload.Package.PackageType)
	}

	if payload.Package.Name == "" {
		return nil, fmt.Errorf("package name not found in webhook payload")
	}

	if payload.Package.Owner.Login == "" {
		return nil, fmt.Errorf("package owner not found in webhook payload")
	}

	// Extract tag name
	tagName := payload.Package.PackageVersion.ContainerMetadata.Tag.Name
	if tagName == "" {
		tagName = payload.Package.PackageVersion.Name
	}
	if tagName == "" {
		tagName = payload.Package.PackageVersion.Version
	}

	if tagName == "" {
		return nil, fmt.Errorf("tag not found in webhook payload")
	}

	// Construct repository name: owner/package
	repository := fmt.Sprintf("%s/%s", payload.Package.Owner.Login, payload.Package.Name)

	return &WebhookEvent{
		RegistryURL: "ghcr.io",
		Repository:  repository,
		Tag:         tagName,
	}, nil
}

// validateSignature validates the webhook signature using HMAC-SHA256
func (g *GHCRWebhook) validateSignature(body []byte, signature string) bool {
	// GitHub signature format: sha256=<hex>
	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}

	expectedSig := signature[7:] // Remove "sha256=" prefix
	mac := hmac.New(sha256.New, []byte(g.secret))
	mac.Write(body)
	calculatedSig := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(expectedSig), []byte(calculatedSig))
}
