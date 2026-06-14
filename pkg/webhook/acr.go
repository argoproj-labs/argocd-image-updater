package webhook

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd"
)

// ACRWebhook handles Azure Container Registry webhook events
type ACRWebhook struct {
	secret string
}

// NewACRWebhook creates a new Azure ACR webhook handler
func NewACRWebhook(secret string) *ACRWebhook {
	return &ACRWebhook{
		secret: secret,
	}
}

// GetRegistryType returns the registry type this handler supports
func (a *ACRWebhook) GetRegistryType() string {
	return "acr"
}

// Validate validates the Azure ACR webhook payload
func (a *ACRWebhook) Validate(r *http.Request) error {
	if r.Method != http.MethodPost {
		return fmt.Errorf("invalid HTTP method: %s", r.Method)
	}

	// ACR sends a Content-Type of application/json unless a custom Content-Type
	// header is configured for the webhook.
	// See: https://learn.microsoft.com/en-us/azure/container-registry/container-registry-webhook-reference#http-headers
	contentType := r.Header.Get("Content-Type")
	if !strings.Contains(contentType, "application/json") {
		return fmt.Errorf("invalid content type: %s", contentType)
	}

	// ACR has no built-in HMAC signing, but it lets you attach arbitrary custom
	// headers to webhook notifications via the Azure CLI, e.g.:
	//   az acr webhook update -n <webhook> -r <registry> --headers "Authorization=Basic <token>"
	// We therefore validate the configured secret against the Authorization
	// header value. The secret configured in argocd-image-updater must match the full Authorization header value.
	// See: https://learn.microsoft.com/en-us/cli/azure/acr/webhook?view=azure-cli-latest#az-acr-webhook-update-examples
	if a.secret != "" {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			return fmt.Errorf("missing Authorization header when secret is configured")
		}

		if subtle.ConstantTimeCompare([]byte(authHeader), []byte(a.secret)) != 1 {
			return fmt.Errorf("incorrect webhook secret")
		}
	}

	return nil
}

// Parse processes the Azure ACR webhook payload and returns a WebhookEvent
func (a *ACRWebhook) Parse(r *http.Request) (*argocd.WebhookEvent, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}

	// Azure ACR payload structure for push events. reference: https://learn.microsoft.com/en-us/azure/container-registry/container-registry-webhook
	var payload struct {
		Action string `json:"action"`
		Target struct {
			MediaType  string `json:"mediaType"`
			Size       int64  `json:"size"`
			Digest     string `json:"digest"`
			Length     int64  `json:"length"`
			Repository string `json:"repository"`
			Tag        string `json:"tag"`
		} `json:"target"`
		Request struct {
			ID     string `json:"id"`
			Host   string `json:"host"`
			Method string `json:"method"`
		} `json:"request"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse webhook payload: %w", err)
	}

	// Only process push events
	if payload.Action != "push" {
		return nil, fmt.Errorf("ignoring action: %s", payload.Action)
	}

	if payload.Target.Repository == "" {
		return nil, fmt.Errorf("repository name not found in webhook payload")
	}

	// A tag may be empty for a digest-only push. Handle this gracefully by
	// returning the event with the digest set instead of erroring out.
	return &argocd.WebhookEvent{
		RegistryURL: payload.Request.Host,
		Repository:  payload.Target.Repository,
		Tag:         payload.Target.Tag,
		Digest:      payload.Target.Digest,
	}, nil
}
