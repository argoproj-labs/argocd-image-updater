package webhook

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd"
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

	// If secret is configured, validate it
	// !! this is not that secure, docker does not have native secrets!
	if d.secret != "" {
		secret := r.URL.Query().Get("secret")
		if secret == "" {
			return fmt.Errorf("missing webhook secret")
		}

		if secret != d.secret {
			return fmt.Errorf("invalid webhook secret")
		}
	}

	return nil
}

// Parse processes the Docker Hub webhook payload and returns a WebhookEvent
func (d *DockerHubWebhook) Parse(r *http.Request) (*argocd.WebhookEvent, error) {
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

	return &argocd.WebhookEvent{
		RegistryURL: "docker.io",
		Repository:  repository,
		Tag:         payload.PushData.Tag,
	}, nil
}
