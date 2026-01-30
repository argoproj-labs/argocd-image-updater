package webhook

import (
	"bytes"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewHarborWebhook(t *testing.T) {
	secret := "test-secret"
	webhook := NewHarborWebhook(secret)

	if webhook == nil {
		t.Fatal("expected webhook to be non-nil")
	}

	if webhook.secret != secret {
		t.Errorf("expected secret to be %q, got %q", secret, webhook.secret)
	}
}

func TestHarborWebhook_GetRegistryType(t *testing.T) {
	webhook := NewHarborWebhook("")
	registryType := webhook.GetRegistryType()

	expected := "harbor"
	if registryType != expected {
		t.Errorf("expected registry type to be %q, got %q", expected, registryType)
	}
}

func TestHarborWebhook_Validate(t *testing.T) {
	secret := "test-secret"
	webhook := NewHarborWebhook(secret)

	tests := []struct {
		name           string
		method         string
		contentType    string
		body           string
		authHeader     string
		noSecret       bool
		expectError    bool
		expectedErrMsg string
	}{
		{
			name:        "valid POST request with correct secret",
			method:      "POST",
			contentType: "application/json",
			body:        `{"test": "data"}`,
			authHeader:  secret,
			expectError: false,
		},
		{
			name:        "valid POST request without secret validation",
			method:      "POST",
			contentType: "application/json",
			body:        `{"test": "data"}`,
			noSecret:    true,
			expectError: false,
		},
		{
			name:        "invalid HTTP method",
			method:      "GET",
			contentType: "application/json",
			body:        `{"test": "data"}`,
			authHeader:  secret,
			expectError: true,
		},
		{
			name:        "invalid content type",
			method:      "POST",
			contentType: "text/plain",
			body:        `{"test": "data"}`,
			authHeader:  secret,
			expectError: true,
		},
		{
			name:           "missing Authorization header when secret is configured",
			method:         "POST",
			contentType:    "application/json",
			body:           `{"test": "data"}`,
			authHeader:     "",
			expectError:    true,
			expectedErrMsg: "missing Authorization header when secret is configured",
		},
		{
			name:           "incorrect secret",
			method:         "POST",
			contentType:    "application/json",
			body:           `{"test": "data"}`,
			authHeader:     "wrong-secret",
			expectError:    true,
			expectedErrMsg: "incorrect webhook secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testWebhook := webhook
			if tt.noSecret {
				testWebhook = NewHarborWebhook("")
			}

			req := httptest.NewRequest(tt.method, "/webhook", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", tt.contentType)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			err := testWebhook.Validate(req)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got none")
					return
				}
				if tt.expectedErrMsg != "" && err.Error() != tt.expectedErrMsg {
					t.Errorf("expected error message %q, got %q", tt.expectedErrMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestHarborWebhook_Parse(t *testing.T) {
	webhook := NewHarborWebhook("")

	tests := []struct {
		name         string
		payload      string
		expectedRepo string
		expectedTag  string
		expectedURL  string
		expectError  bool
	}{
		{
			name: "valid PUSH_ARTIFACT payload with repo_full_name",
			payload: `{
				"type": "PUSH_ARTIFACT",
				"occur_at": 1640995200,
				"operator": "admin",
				"event_data": {
					"resources": [{
						"digest": "sha256:abc123",
						"tag": "v1.0.0",
						"resource_url": "https://harbor.example.com/library/myapp:v1.0.0"
					}],
					"repository": {
						"name": "myapp",
						"namespace": "library", 
						"repo_full_name": "library/myapp",
						"repo_type": "public"
					}
				}
			}`,
			expectedRepo: "library/myapp",
			expectedTag:  "v1.0.0",
			expectedURL:  "harbor.example.com",
			expectError:  false,
		},
		{
			name: "valid PUSH_ARTIFACT payload matching actual Harbor format",
			payload: `{
				"type": "PUSH_ARTIFACT",
				"occur_at": 1749023740,
				"operator": "somebody",
				"event_data": {
					"resources": [{
						"digest": "sha256:3e5e4ce59b8390414cf58806692fb716cc02d71f8a53b35ddeffdeb0c9aaf100",
						"tag": "tag",
						"resource_url": "harbor.example.com/image-name:tag"
					}],
					"repository": {
						"date_created": 1693889800,
						"name": "dnse/krx-derivative-asset-service",
						"namespace": "private",
						"repo_full_name": "image-name",
						"repo_type": "private"
					}
				}
			}`,
			expectedRepo: "image-name",
			expectedTag:  "tag",
			expectedURL:  "harbor.example.com",
			expectError:  false,
		},
		{
			name: "valid PUSH_ARTIFACT payload with namespace and name fallback",
			payload: `{
				"type": "PUSH_ARTIFACT",
				"occur_at": 1640995200,
				"operator": "admin",
				"event_data": {
					"resources": [{
						"digest": "sha256:def456",
						"tag": "latest",
						"resource_url": "registry.example.com/myproject/myapp:latest"
					}],
					"repository": {
						"name": "myapp",
						"namespace": "myproject",
						"repo_type": "private"
					}
				}
			}`,
			expectedRepo: "myproject/myapp",
			expectedTag:  "latest",
			expectedURL:  "registry.example.com",
			expectError:  false,
		},
		{
			name: "valid PUSH_ARTIFACT payload with only name",
			payload: `{
				"type": "PUSH_ARTIFACT",
				"occur_at": 1640995200,
				"operator": "admin",
				"event_data": {
					"resources": [{
						"digest": "sha256:ghi789",
						"tag": "v2.0.0"
					}],
					"repository": {
						"name": "myapp",
						"repo_type": "public"
					}
				}
			}`,
			expectedRepo: "myapp",
			expectedTag:  "v2.0.0",
			expectedURL:  "harbor",
			expectError:  false,
		},
		{
			name: "unsupported event type",
			payload: `{
				"type": "DELETE_ARTIFACT",
				"occur_at": 1640995200,
				"operator": "admin",
				"event_data": {
					"resources": [{
						"tag": "v1.0.0"
					}],
					"repository": {
						"repo_full_name": "library/myapp"
					}
				}
			}`,
			expectError: true,
		},
		{
			name: "missing resources",
			payload: `{
				"type": "PUSH_ARTIFACT",
				"occur_at": 1640995200,
				"operator": "admin",
				"event_data": {
					"resources": [],
					"repository": {
						"repo_full_name": "library/myapp"
					}
				}
			}`,
			expectError: true,
		},
		{
			name: "missing tag",
			payload: `{
				"type": "PUSH_ARTIFACT",
				"occur_at": 1640995200,
				"operator": "admin",
				"event_data": {
					"resources": [{
						"digest": "sha256:abc123"
					}],
					"repository": {
						"repo_full_name": "library/myapp"
					}
				}
			}`,
			expectError: true,
		},
		{
			name: "missing repository name",
			payload: `{
				"type": "PUSH_ARTIFACT",
				"occur_at": 1640995200,
				"operator": "admin",
				"event_data": {
					"resources": [{
						"tag": "v1.0.0"
					}],
					"repository": {}
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
			req.Header.Set("Content-Type", "application/json")

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

			if event.RegistryURL != tt.expectedURL {
				t.Errorf("expected registry URL to be %q, got %q", tt.expectedURL, event.RegistryURL)
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

func TestHarborWebhook_ParseWithBodyReuse(t *testing.T) {
	// Test that body can be read multiple times (e.g., after validation)
	secret := "test-secret"
	webhook := NewHarborWebhook(secret)

	payload := `{
		"type": "PUSH_ARTIFACT",
		"occur_at": 1640995200,
		"operator": "admin",
		"event_data": {
			"resources": [{
				"digest": "sha256:abc123",
				"tag": "v1.0.0"
			}],
			"repository": {
				"repo_full_name": "library/myapp"
			}
		}
	}`

	req := httptest.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", secret)

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

	if event.Repository != "library/myapp" {
		t.Errorf("expected repository to be 'library/myapp', got %q", event.Repository)
	}

	if event.Tag != "v1.0.0" {
		t.Errorf("expected tag to be 'v1.0.0', got %q", event.Tag)
	}

	if event.RegistryURL != "harbor" {
		t.Errorf("expected registry URL to be 'harbor', got %q", event.RegistryURL)
	}
}

// Test helper to simulate reading request body multiple times
func TestHarborBodyReusability(t *testing.T) {
	originalBody := `{"type": "PUSH_ARTIFACT"}`
	req := httptest.NewRequest("POST", "/webhook", strings.NewReader(originalBody))

	// First read
	body1, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("first read failed: %v", err)
	}

	// Reset body for second read
	req.Body = io.NopCloser(bytes.NewReader(body1))

	// Second read
	body2, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("second read failed: %v", err)
	}

	if string(body1) != originalBody {
		t.Errorf("first read: expected %q, got %q", originalBody, string(body1))
	}

	if string(body2) != originalBody {
		t.Errorf("second read: expected %q, got %q", originalBody, string(body2))
	}
}
