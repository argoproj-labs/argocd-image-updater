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

// DockerHubWebhook handles Docker Hub webhook events
type DockerHubWebhook struct {
	secret string
}

// NewDockerHubWebhook creates a new Docker Hub webhook handler
func NewDockerHubWebhook(secret string) *DockerHubWebhook {
	return &DockerHubWebhook{
		secret: secret,
	}
}

// GetRegistryType returns the registry type this handler supports
func (d *DockerHubWebhook) GetRegistryType() string {
	return "docker.io"
}

// Validate validates the Docker Hub webhook payload
func (d *DockerHubWebhook) Validate(r *http.Request) error {
	if r.Method != http.MethodPost {
		return fmt.Errorf("invalid HTTP method: %s", r.Method)
	}

	// If secret is configured, validate the signature
	if d.secret != "" {
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

		if !d.validateSignature(body, signature) {
			return fmt.Errorf("invalid webhook signature")
		}
	}

	return nil
}

// Parse processes the Docker Hub webhook payload and returns a WebhookEvent
func (d *DockerHubWebhook) Parse(r *http.Request) (*WebhookEvent, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}

	var payload struct {
		Repository struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
			RepoName  string `json:"repo_name"`
		} `json:"repository"`
		PushData struct {
			Tag string `json:"tag"`
		} `json:"push_data"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse webhook payload: %w", err)
	}

	// Extract repository name - Docker Hub uses namespace/name format
	repository := payload.Repository.RepoName
	if repository == "" {
		if payload.Repository.Namespace != "" && payload.Repository.Name != "" {
			repository = fmt.Sprintf("%s/%s", payload.Repository.Namespace, payload.Repository.Name)
		} else {
			repository = payload.Repository.Name
		}
	}

	if repository == "" {
		return nil, fmt.Errorf("repository name not found in webhook payload")
	}

	if payload.PushData.Tag == "" {
		return nil, fmt.Errorf("tag not found in webhook payload")
	}

	return &WebhookEvent{
		RegistryURL: "docker.io",
		Repository:  repository,
		Tag:         payload.PushData.Tag,
	}, nil
}

// validateSignature validates the webhook signature using HMAC-SHA256
func (d *DockerHubWebhook) validateSignature(body []byte, signature string) bool {
	// Docker Hub signature format: sha256=<hex>
	if !strings.HasPrefix(signature, "sha256=") {
		return false
	}

	expectedSig := signature[7:] // Remove "sha256=" prefix
	mac := hmac.New(sha256.New, []byte(d.secret))
	mac.Write(body)
	calculatedSig := hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(expectedSig), []byte(calculatedSig))
}
