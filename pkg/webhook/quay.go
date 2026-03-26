package webhook

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

type QuayWebhook struct {
	secret string
}

func NewQuayWebhook(secret string) *QuayWebhook {
	return &QuayWebhook{
		secret: secret,
	}
}

// GetRegistryType returns the type this handler supports
func (q *QuayWebhook) GetRegistryType() string {
	return "quay.io"
}

// Validates checks the Quay webhook payload to see if its valid
func (q *QuayWebhook) Validate(r *http.Request) error {
	if r.Method != http.MethodPost {
		return fmt.Errorf("invalid HTTP method: %s", r.Method)
	}

	// Quay at the moment does not support secrets
	// !! This query param method is NOT secure use at own risk
	if q.secret != "" {
		secret := r.URL.Query().Get("secret")
		if secret == "" {
			return fmt.Errorf("Missing webhook secret")
		}

		if secret != q.secret {
			return fmt.Errorf("Incorrect webhook secret")
		}
	}

	return nil
}

// Parse process the Quay webhook and returns a Webhook event from the event
func (q *QuayWebhook) Parse(r *http.Request) (*WebhookEvent, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}

	// Quay Repository Push Event
	// https://docs.quay.io/guides/notifications.html
	var payload struct {
		Name        string   `json:"name"`
		Repository  string   `json:"repository"`
		Namespace   string   `json:"namespace"`
		DockerUrl   string   `json:"docker_url"`
		Homepage    string   `json:"homepage"`
		UpdatedTags []string `json:"updated_tags"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse webhook payload: %w", err)
	}

	// Check updated tags for now just take first one
	var tag string
	if len(payload.UpdatedTags) == 0 {
		return nil, fmt.Errorf("no tags in the payload")
	}
	tag = payload.UpdatedTags[0]

	return &WebhookEvent{
		RegistryURL: "quay.io",
		Repository:  payload.Repository,
		Tag:         tag,
	}, nil
}
