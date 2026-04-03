package webhook

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd"
)

// GitLabWebhook handles GitLab webhook events
type GitLabWebhook struct {
	secret string
}

var _ RegistryWebhook = &GitLabWebhook{}

// NewGitLabWebhook creates a new GitLab webhook handler
func NewGitLabWebhook(secret string) *GitLabWebhook {
	return &GitLabWebhook{
		secret: secret,
	}
}

// GetRegistryType returns the registry type this handler supports
func (g *GitLabWebhook) GetRegistryType() string {
	return "gitlab"
}

// Validate validates the GitLab webhook payload
func (g *GitLabWebhook) Validate(r *http.Request) error {
	if r.Method != http.MethodPost {
		return fmt.Errorf("invalid HTTP method: %s", r.Method)
	}

	// Check for GitLab webhook headers
	contentType := r.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/vnd.docker.distribution.events") {
		return fmt.Errorf("invalid content type: %s", contentType)
	}

	// If secret is configured, validate the secret
	if g.secret != "" {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			return errors.New("missing Authorization header when secret is configured")
		}

		// GitLab sends plain secret value directly in Authorization header for external webhooks
		if subtle.ConstantTimeCompare([]byte(authHeader), []byte(g.secret)) != 1 {
			return errors.New("incorrect webhook secret")
		}
	}

	return nil
}

// Parse processes the GitLab webhook payload and returns a slice of WebhookEvent
func (g *GitLabWebhook) Parse(r *http.Request) ([]*argocd.WebhookEvent, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}

	var payload struct {
		Events []struct {
			Action string `json:"action"`
			Target struct {
				Repository string `json:"repository"`
				Tag        string `json:"tag"`
				Digest     string `json:"digest"`
				MediaType  string `json:"mediaType"`
			}
			Request struct {
				Host string `json:"host"`
			}
		}
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse webhook payload: %w", err)
	}

	if len(payload.Events) == 0 {
		return nil, fmt.Errorf("no events found in webhook payload")
	}

	events := make([]*argocd.WebhookEvent, 0, len(payload.Events))

	for _, event := range payload.Events {
		if event.Action != "push" {
			continue
		}
		// GitLab will send events for blob pushes, which are generally not useful to us
		if event.Target.Tag == "" {
			continue
		}

		// source addr might not have a protocol scheme
		resourceURL := event.Request.Host

		// Add https:// scheme if missing for parsing
		if !strings.HasPrefix(resourceURL, "http://") && !strings.HasPrefix(resourceURL, "https://") {
			resourceURL = "https://" + resourceURL
		}

		if parsedURL, err := url.Parse(resourceURL); err == nil {
			resourceURL = parsedURL.Host
		} else {
			// Fallback: try to extract host manually by splitting on the first '/'
			parts := strings.Split(event.Request.Host, "/")
			if len(parts) > 0 && strings.Contains(parts[0], ".") {
				resourceURL = parts[0]
			}
		}

		events = append(events, &argocd.WebhookEvent{
			RegistryURL: resourceURL,
			Repository:  event.Target.Repository,
			Tag:         event.Target.Tag,
			Digest:      event.Target.Digest,
		})
	}

	return events, nil
}
