package webhook

import (
	"bytes"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewDockerHubWebhook(t *testing.T) {
	secret := "test-secret"
	webhook := NewDockerHubWebhook(secret)

	if webhook == nil {
		t.Fatal("expected webhook to be non-nil")
	}

	if webhook.secret != secret {
		t.Errorf("expected secret to be %q, got %q", secret, webhook.secret)
	}
}

func TestDockerHubWebhook_GetRegistryType(t *testing.T) {
	webhook := NewDockerHubWebhook("")
	registryType := webhook.GetRegistryType()

	expected := "docker.io"
	if registryType != expected {
		t.Errorf("expected registry type to be %q, got %q", expected, registryType)
	}
}

func TestDockerHubWebhook_Validate(t *testing.T) {
	secret := "test-secret"
	webhook := NewDockerHubWebhook(secret)

	tests := []struct {
		name        string
		method      string
		body        string
		secret      string
		noSecret    bool
		expectError bool
	}{
		{
			name:        "valid POST request with correct secret",
			method:      "POST",
			body:        `{"test": "data"}`,
			secret:      "test-secret",
			expectError: false,
		},
		{
			name:        "valid POST request without secret",
			method:      "POST",
			body:        `{"test": "data"}`,
			noSecret:    true,
			expectError: false,
		},
		{
			name:        "invalid HTTP method",
			method:      "GET",
			body:        `{"test": "data"}`,
			secret:      "test-secret",
			expectError: true,
		},
		{
			name:        "missing secret when secret is configured",
			method:      "POST",
			body:        `{"test": "data"}`,
			secret:      "",
			expectError: true,
		},
		{
			name:        "invalid secret",
			method:      "POST",
			body:        `{"test": "data"}`,
			secret:      "not-the-secret",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testWebhook := webhook
			if tt.noSecret {
				testWebhook = NewDockerHubWebhook("")
			}

			req := httptest.NewRequest(tt.method, "/webhook", strings.NewReader(tt.body))
			if tt.secret != "" {
				query := req.URL.Query()
				query.Set("secret", tt.secret)
				req.URL.RawQuery = query.Encode()
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

func TestDockerHubWebhook_Parse(t *testing.T) {
	webhook := NewDockerHubWebhook("")

	tests := []struct {
		name         string
		payload      string
		expectedRepo string
		expectedTag  string
		expectError  bool
	}{
		{
			name: "valid payload with repo_name",
			payload: `{
				"repository": {
					"repo_name": "myuser/myapp",
					"name": "myapp",
					"namespace": "myuser"
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
			name: "valid payload with namespace and name",
			payload: `{
				"repository": {
					"name": "myapp",
					"namespace": "myuser"
				},
				"push_data": {
					"tag": "latest"
				}
			}`,
			expectedRepo: "myuser/myapp",
			expectedTag:  "latest",
			expectError:  false,
		},
		{
			name: "valid payload with only name",
			payload: `{
				"repository": {
					"name": "myapp"
				},
				"push_data": {
					"tag": "v2.0.0"
				}
			}`,
			expectedRepo: "myapp",
			expectedTag:  "v2.0.0",
			expectError:  false,
		},
		{
			name: "missing repository name",
			payload: `{
				"repository": {},
				"push_data": {
					"tag": "v1.0.0"
				}
			}`,
			expectError: true,
		},
		{
			name: "missing tag",
			payload: `{
				"repository": {
					"repo_name": "myuser/myapp"
				},
				"push_data": {}
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

			if event.RegistryURL != "docker.io" {
				t.Errorf("expected registry URL to be 'docker.io', got %q", event.RegistryURL)
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

func TestDockerHubWebhook_ParseWithBodyReuse(t *testing.T) {
	// Test that body can be read multiple times (e.g., after validation)
	secret := "test-secret"
	webhook := NewDockerHubWebhook(secret)

	payload := `{
		"repository": {
			"repo_name": "myuser/myapp"
		},
		"push_data": {
			"tag": "v1.0.0"
		}
	}`

	req := httptest.NewRequest("POST", "/webhook", strings.NewReader(payload))
	query := req.URL.Query()
	query.Set("secret", "test-secret")
	req.URL.RawQuery = query.Encode()

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

// Test helper to simulate reading request body multiple times
func TestBodyReusability(t *testing.T) {
	originalBody := `{"test": "data"}`
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
