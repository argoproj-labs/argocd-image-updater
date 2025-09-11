package webhook

import (
	"fmt"
	"net/http"

	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd"
)

// RegistryWebhook interface defines methods for handling registry webhooks
type RegistryWebhook interface {
	// Parse processes the webhook payload and returns a WebhookEvent
	Parse(r *http.Request) (*argocd.WebhookEvent, error)
	// Validate validates the webhook payload
	Validate(r *http.Request) error
	// GetRegistryType returns the type of registry this handler supports
	GetRegistryType() string
}

// WebhookHandler manages webhook handlers for different registry types
type WebhookHandler struct {
	handlers map[string]RegistryWebhook
}

// NewWebhookHandler creates a new webhook handler
func NewWebhookHandler() *WebhookHandler {
	return &WebhookHandler{
		handlers: make(map[string]RegistryWebhook),
	}
}

// RegisterHandler registers a webhook handler for a specific registry type
func (h *WebhookHandler) RegisterHandler(handler RegistryWebhook) {
	h.handlers[handler.GetRegistryType()] = handler
}

// ProcessWebhook processes an incoming webhook request and returns a WebhookEvent
func (h *WebhookHandler) ProcessWebhook(r *http.Request) (*argocd.WebhookEvent, error) {
	// Try to determine registry type from request headers or path
	registryType := h.detectRegistryType(r)

	if handler, exists := h.handlers[registryType]; exists {
		if err := handler.Validate(r); err != nil {
			return nil, err
		}
		return handler.Parse(r)
	}

	// If we can't determine the registry type, try each handler
	for _, handler := range h.handlers {
		if err := handler.Validate(r); err == nil {
			return handler.Parse(r)
		}
	}

	return nil, fmt.Errorf("no handler available for this webhook")
}

// detectRegistryType tries to determine the registry type from the request
func (h *WebhookHandler) detectRegistryType(r *http.Request) string {
	// Check for registry type in path or header
	registryType := r.URL.Query().Get("type")
	if registryType != "" {
		return registryType
	}

	registryType = r.Header.Get("X-Registry-Type")
	if registryType != "" {
		return registryType
	}

	return ""
}
