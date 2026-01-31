package webhook

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewWebhookHandler(t *testing.T) {
	handler := NewWebhookHandler()

	if handler == nil {
		t.Fatal("expected handler to be non-nil")
	} else if handler.handlers == nil {
		t.Fatal("expected handlers map to be initialized")
	} else if len(handler.handlers) != 0 {
		t.Errorf("expected handlers map to be empty, got %d handlers", len(handler.handlers))
	}
}

func TestWebhookHandler_RegisterHandler(t *testing.T) {
	handler := NewWebhookHandler()

	// Create mock webhook handlers
	dockerHandler := NewDockerHubWebhook("secret")
	ghcrHandler := NewGHCRWebhook("secret")

	// Register handlers
	handler.RegisterHandler(dockerHandler)
	handler.RegisterHandler(ghcrHandler)

	if len(handler.handlers) != 2 {
		t.Errorf("expected 2 handlers, got %d", len(handler.handlers))
	}

	// Check if handlers are registered with correct registry types
	if _, exists := handler.handlers["docker.io"]; !exists {
		t.Error("expected docker.io handler to be registered")
	}

	if _, exists := handler.handlers["ghcr.io"]; !exists {
		t.Error("expected ghcr.io handler to be registered")
	}
}

func TestWebhookHandler_ProcessWebhook(t *testing.T) {
	handler := NewWebhookHandler()

	// Register handlers
	dockerHandler := NewDockerHubWebhook("")
	ghcrHandler := NewGHCRWebhook("")
	handler.RegisterHandler(dockerHandler)
	handler.RegisterHandler(ghcrHandler)

	tests := []struct {
		name         string
		registryType string
		payload      string
		headers      map[string]string
		expectedRepo string
		expectedTag  string
		expectError  bool
	}{
		{
			name:         "valid Docker Hub webhook with type parameter",
			registryType: "docker.io",
			payload: `{
				"repository": {
					"repo_name": "myuser/myapp"
				},
				"push_data": {
					"tag": "v1.0.0"
				}
			}`,
			expectedRepo: "myuser/myapp",
			expectedTag:  "v1.0.0",
			expectError:  false,
		},
		{
			name:         "valid GHCR webhook with type parameter",
			registryType: "ghcr.io",
			payload: `{
				"action": "published",
				"package": {
					"name": "myapp",
					"package_type": "container",
					"owner": {
						"login": "myuser"
					},
					"package_version": {
						"name": "v1.0.0",
						"container_metadata": {
							"tag": {
								"name": "v1.0.0"
							}
						}
					}
				}
			}`,
			headers: map[string]string{
				"X-GitHub-Event": "package",
			},
			expectedRepo: "myuser/myapp",
			expectedTag:  "v1.0.0",
			expectError:  false,
		},
		{
			name:         "missing registry type parameter",
			registryType: "",
			payload: `{
				"repository": {
					"repo_name": "myuser/myapp"
				},
				"push_data": {
					"tag": "latest"
				}
			}`,
			expectError: true,
		},
		{
			name:         "invalid registry type",
			registryType: "unsupported.io",
			payload:      `{"test": "data"}`,
			expectError:  true,
		},
		{
			name:         "invalid payload with valid type",
			registryType: "docker.io",
			payload:      `{"invalid": "payload"}`,
			expectError:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/webhook"
			if tt.registryType != "" {
				url += "?type=" + tt.registryType
			}

			req := httptest.NewRequest("POST", url, strings.NewReader(tt.payload))

			// Set any required headers
			for key, value := range tt.headers {
				req.Header.Set(key, value)
			}

			event, err := handler.ProcessWebhook(req)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				} else {
					// Verify error message for specific test cases
					if tt.name == "missing registry type parameter" {
						if !strings.Contains(err.Error(), "missing registry type parameter") {
							t.Errorf("expected error about missing registry type parameter, got: %v", err)
						}
					} else if tt.name == "invalid registry type" {
						if !strings.Contains(err.Error(), "invalid registry type") {
							t.Errorf("expected error about invalid registry type, got: %v", err)
						}
						if !strings.Contains(err.Error(), "unsupported.io") {
							t.Errorf("expected error to mention the invalid type, got: %v", err)
						}
						if !strings.Contains(err.Error(), "Should be one of") {
							t.Errorf("expected error to list valid types, got: %v", err)
						}
					}
				}
				return
			}

			if err != nil {
				t.Errorf("expected no error but got: %v", err)
				return
			}

			if event == nil {
				t.Fatal("expected event to be non-nil")
			} else if event.Repository != tt.expectedRepo {
				t.Errorf("expected repository to be %q, got %q", tt.expectedRepo, event.Repository)
			} else if event.Tag != tt.expectedTag {
				t.Errorf("expected tag to be %q, got %q", tt.expectedTag, event.Tag)
			}
		})
	}
}

func TestWebhookHandler_ProcessWebhookWithHeader(t *testing.T) {
	handler := NewWebhookHandler()

	// Register Docker Hub handler
	dockerHandler := NewDockerHubWebhook("")
	handler.RegisterHandler(dockerHandler)

	payload := `{
		"repository": {
			"repo_name": "myuser/myapp"
		},
		"push_data": {
			"tag": "v2.0.0"
		}
	}`

	req := httptest.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("X-Registry-Type", "docker.io")

	event, err := handler.ProcessWebhook(req)
	if err != nil {
		t.Fatalf("expected no error but got: %v", err)
	}

	if event == nil {
		t.Fatal("expected event to be non-nil")
	} else if event.Repository != "myuser/myapp" {
		t.Errorf("expected repository to be 'myuser/myapp', got %q", event.Repository)
	} else if event.Tag != "v2.0.0" {
		t.Errorf("expected tag to be 'v2.0.0', got %q", event.Tag)
	}
}

func TestWebhookHandler_detectRegistryType(t *testing.T) {
	handler := NewWebhookHandler()

	tests := []struct {
		name         string
		queryParam   string
		header       string
		expectedType string
	}{
		{
			name:         "registry type from query parameter",
			queryParam:   "docker.io",
			expectedType: "docker.io",
		},
		{
			name:         "registry type from header",
			header:       "ghcr.io",
			expectedType: "ghcr.io",
		},
		{
			name:         "query parameter takes precedence over header",
			queryParam:   "docker.io",
			header:       "ghcr.io",
			expectedType: "docker.io",
		},
		{
			name:         "no registry type specified",
			expectedType: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			url := "/webhook"
			if tt.queryParam != "" {
				url += "?type=" + tt.queryParam
			}

			req := httptest.NewRequest("POST", url, nil)
			if tt.header != "" {
				req.Header.Set("X-Registry-Type", tt.header)
			}

			registryType := handler.detectRegistryType(req)
			if registryType != tt.expectedType {
				t.Errorf("expected registry type to be %q, got %q", tt.expectedType, registryType)
			}
		})
	}
}
