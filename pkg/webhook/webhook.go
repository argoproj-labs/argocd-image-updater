package webhook

import (
	"fmt"
	"net/http"
	"slices"
	"strings"

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

	// Get list of supported registry types from registered handlers
	registryTypes := h.getSupportedRegistryTypes()

	// Registry type is required
	if registryType == "" {
		return nil, fmt.Errorf("missing registry type parameter. Supported types: %s", strings.Join(registryTypes, ", "))
	}

	// Validate the registry type
	if !slices.Contains(registryTypes, registryType) {
		return nil, fmt.Errorf("invalid registry type: %s. Supported types: %s", registryType, strings.Join(registryTypes, ", "))
	}

	// If handler exists for the specified type, use it
	if handler, exists := h.handlers[registryType]; exists {
		if err := handler.Validate(r); err != nil {
			return nil, err
		}
		return handler.Parse(r)
	}

	// Valid type but no handler registered - return clear error
	return nil, fmt.Errorf("no handler registered for registry type: %s", registryType)
}

// getSupportedRegistryTypes returns a list of all supported registry types from registered handlers
func (h *WebhookHandler) getSupportedRegistryTypes() []string {
	types := make([]string, 0, len(h.handlers))
	for registryType := range h.handlers {
		types = append(types, registryType)
	}
	return types
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
