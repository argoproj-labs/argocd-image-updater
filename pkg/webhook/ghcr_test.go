package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewGHCRWebhook(t *testing.T) {
	secret := "test-secret"
	webhook := NewGHCRWebhook(secret)

	if webhook == nil {
		t.Fatal("expected webhook to be non-nil")
	}

	if webhook.secret != secret {
		t.Errorf("expected secret to be %q, got %q", secret, webhook.secret)
	}
}

func TestGHCRWebhook_GetRegistryType(t *testing.T) {
	webhook := NewGHCRWebhook("")
	registryType := webhook.GetRegistryType()

	expected := "ghcr.io"
	if registryType != expected {
		t.Errorf("expected registry type to be %q, got %q", expected, registryType)
	}
}

func TestGHCRWebhook_Validate(t *testing.T) {
	secret := "test-secret"
	webhook := NewGHCRWebhook(secret)

	tests := []struct {
		name        string
		method      string
		body        string
		signature   string
		eventType   string
		noSecret    bool
		expectError bool
	}{
		{
			name:        "valid POST request with correct signature",
			method:      "POST",
			body:        `{"test": "data"}`,
			signature:   generateGHCRSignature(secret, `{"test": "data"}`),
			eventType:   "package",
			expectError: false,
		},
		{
			name:        "valid POST request without secret validation",
			method:      "POST",
			body:        `{"test": "data"}`,
			eventType:   "package",
			noSecret:    true,
			expectError: false,
		},
		{
			name:        "invalid HTTP method",
			method:      "GET",
			body:        `{"test": "data"}`,
			signature:   generateGHCRSignature(secret, `{"test": "data"}`),
			eventType:   "package",
			expectError: true,
		},
		{
			name:        "missing X-GitHub-Event header",
			method:      "POST",
			body:        `{"test": "data"}`,
			signature:   generateGHCRSignature(secret, `{"test": "data"}`),
			expectError: true,
		},
		{
			name:        "unsupported event type",
			method:      "POST",
			body:        `{"test": "data"}`,
			signature:   generateGHCRSignature(secret, `{"test": "data"}`),
			eventType:   "push",
			expectError: true,
		},
		{
			name:        "missing signature when secret is configured",
			method:      "POST",
			body:        `{"test": "data"}`,
			eventType:   "package",
			signature:   "",
			expectError: true,
		},
		{
			name:        "invalid signature",
			method:      "POST",
			body:        `{"test": "data"}`,
			signature:   "sha256=invalid",
			eventType:   "package",
			expectError: true,
		},
		{
			name:        "signature for different body",
			method:      "POST",
			body:        `{"test": "data"}`,
			signature:   generateGHCRSignature(secret, `{"different": "data"}`),
			eventType:   "package",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testWebhook := webhook
			if tt.noSecret {
				testWebhook = NewGHCRWebhook("")
			}

			req := httptest.NewRequest(tt.method, "/webhook", strings.NewReader(tt.body))
			if tt.eventType != "" {
				req.Header.Set("X-GitHub-Event", tt.eventType)
			}
			if tt.signature != "" {
				req.Header.Set("X-Hub-Signature-256", tt.signature)
			}

			err := testWebhook.Validate(req)

			if tt.expectError && err == nil {
				t.Error("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}

func TestGHCRWebhook_Parse(t *testing.T) {
	webhook := NewGHCRWebhook("")

	tests := []struct {
		name         string
		payload      string
		expectedRepo string
		expectedTag  string
		expectError  bool
	}{
		{
			name: "valid registry package published event with lower case package_type",
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
			expectedRepo: "myuser/myapp",
			expectedTag:  "v1.0.0",
			expectError:  false,
		},
		{
			name: "another valid registry package published event with upper case package_type",
			payload: `{
				"action": "published",
				"package": {
					"name": "myapp",
					"package_type": "CONTAINER",
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
			expectedRepo: "myuser/myapp",
			expectedTag:  "v1.0.0",
			expectError:  false,
		},
		{
			name: "another valid registry package published event with camel case package_type",
			payload: `{
				"action": "published",
				"package": {
					"name": "myapp",
					"package_type": "Container",
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
			expectedRepo: "myuser/myapp",
			expectedTag:  "v1.0.0",
			expectError:  false,
		},
		{
			name: "valid registry package published event with latest tag",
			payload: `{
				"action": "published",
				"package": {
					"name": "my-service",
					"package_type": "container",
					"owner": {
						"login": "myorg"
					},
					"package_version": {
						"name": "latest",
						"container_metadata": {
							"tag": {
								"name": "latest"
							}
						}
					}
				}
			}`,
			expectedRepo: "myorg/my-service",
			expectedTag:  "latest",
			expectError:  false,
		},
		{
			name: "fallback to package version name when tag name is missing",
			payload: `{
				"action": "published",
				"package": {
					"name": "fallback-app",
					"package_type": "container",
					"owner": {
						"login": "testuser"
					},
					"package_version": {
						"name": "v2.0.0",
						"container_metadata": {}
					}
				}
			}`,
			expectedRepo: "testuser/fallback-app",
			expectedTag:  "v2.0.0",
			expectError:  false,
		},
		{
			name: "non-published action should be ignored",
			payload: `{
				"action": "updated",
				"package": {
					"name": "myapp",
					"package_type": "container",
					"owner": {
						"login": "myuser"
					},
					"package_version": {
						"name": "v1.0.0"
					}
				}
			}`,
			expectError: true,
		},
		{
			name: "non-container package type should be ignored",
			payload: `{
				"action": "published",
				"package": {
					"name": "myapp",
					"package_type": "npm",
					"owner": {
						"login": "myuser"
					},
					"package_version": {
						"name": "v1.0.0"
					}
				}
			}`,
			expectError: true,
		},
		{
			name: "missing package name",
			payload: `{
				"action": "published",
				"package": {
					"package_type": "container",
					"owner": {
						"login": "myuser"
					},
					"package_version": {
						"name": "v1.0.0"
					}
				}
			}`,
			expectError: true,
		},
		{
			name: "missing package owner",
			payload: `{
				"action": "published",
				"package": {
					"name": "myapp",
					"package_type": "container",
					"package_version": {
						"name": "v1.0.0"
					}
				}
			}`,
			expectError: true,
		},
		{
			name: "missing tag information",
			payload: `{
				"action": "published",
				"package": {
					"name": "myapp",
					"package_type": "container",
					"owner": {
						"login": "myuser"
					},
					"package_version": {}
				}
			}`,
			expectError: true,
		},
		{
			name:        "invalid JSON",
			payload:     `{"invalid": json}`,
			expectError: true,
		},
		{
			name:        "empty payload",
			payload:     `{}`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/webhook", strings.NewReader(tt.payload))

			event, err := webhook.Parse(req)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("expected no error but got: %v", err)
				return
			}

			if event == nil {
				t.Fatal("expected event to be non-nil")
			}

			if event.RegistryURL != "ghcr.io" {
				t.Errorf("expected registry URL to be 'ghcr.io', got %q", event.RegistryURL)
			}

			if event.Repository != tt.expectedRepo {
				t.Errorf("expected repository to be %q, got %q", tt.expectedRepo, event.Repository)
			}

			if event.Tag != tt.expectedTag {
				t.Errorf("expected tag to be %q, got %q", tt.expectedTag, event.Tag)
			}
		})
	}
}

func TestGHCRWebhook_validateSignature(t *testing.T) {
	secret := "test-secret"
	webhook := NewGHCRWebhook(secret)

	tests := []struct {
		name      string
		body      string
		signature string
		expected  bool
	}{
		{
			name:      "valid signature",
			body:      `{"test": "data"}`,
			signature: generateGHCRSignature(secret, `{"test": "data"}`),
			expected:  true,
		},
		{
			name:      "invalid signature",
			body:      `{"test": "data"}`,
			signature: "sha256=invalid",
			expected:  false,
		},
		{
			name:      "signature without prefix",
			body:      `{"test": "data"}`,
			signature: "invalid",
			expected:  false,
		},
		{
			name:      "empty signature",
			body:      `{"test": "data"}`,
			signature: "",
			expected:  false,
		},
		{
			name:      "signature for different body",
			body:      `{"test": "data"}`,
			signature: generateGHCRSignature(secret, `{"different": "data"}`),
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := webhook.validateSignature([]byte(tt.body), tt.signature)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestGHCRWebhook_ParseWithBodyReuse(t *testing.T) {
	// Test that body can be read multiple times (e.g., after validation)
	secret := "test-secret"
	webhook := NewGHCRWebhook(secret)

	payload := `{
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
	}`

	req := httptest.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("X-GitHub-Event", "package")
	req.Header.Set("X-Hub-Signature-256", generateGHCRSignature(secret, payload))

	// First, validate the request
	err := webhook.Validate(req)
	if err != nil {
		t.Fatalf("validation failed: %v", err)
	}

	// Then, parse the request - this should work even after validation read the body
	event, err := webhook.Parse(req)
	if err != nil {
		t.Fatalf("parsing failed: %v", err)
	}

	if event.Repository != "myuser/myapp" {
		t.Errorf("expected repository to be 'myuser/myapp', got %q", event.Repository)
	}

	if event.Tag != "v1.0.0" {
		t.Errorf("expected tag to be 'v1.0.0', got %q", event.Tag)
	}
}

// Helper function to generate HMAC-SHA256 signature for GHCR testing
func generateGHCRSignature(secret, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}
