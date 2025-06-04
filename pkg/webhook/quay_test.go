package webhook

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewQuayWebhook(t *testing.T) {
	secret := "test"
	webhook := NewDockerHubWebhook(secret)

	if webhook == nil {
		t.Fatal("expected webhook to not be nil")
	}

	if webhook.secret != secret {
		t.Errorf("expected secret to be %q, got %q", secret, webhook.secret)
	}
}

func TestQuayWebhook_GetRegistryType(t *testing.T) {
	webhook := NewQuayWebhook("")
	registryType := webhook.GetRegistryType()

	expected := "quay.io"
	if registryType != expected {
		t.Errorf("expected registry type to be %q, got %q", expected, registryType)
	}
}

func TestQuayWebhook_Validate(t *testing.T) {
	secret := "test-secret"
	webhook := NewQuayWebhook(secret)

	// TODO: once secret stuff is decided will need to update tests
	tests := []struct {
		name        string
		method      string
		body        string
		expectError bool
	}{
		{
			name:        "valid POST request",
			method:      "POST",
			body:        `{"test": "data"}`,
			expectError: false,
		},
		{
			name:        "invalid POST request",
			method:      "GET",
			body:        `{"test": "data"}`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testWebhook := webhook

			req := httptest.NewRequest(tt.method, "/webhook", strings.NewReader(tt.body))

			err := testWebhook.Validate(req)

			if tt.expectError && err == nil {
				t.Errorf("expected error but got none")
			}
			if !tt.expectError && err != nil {
				t.Errorf("expected no error but got: %v", err)
			}
		})
	}
}

func TestQuayWebhook_Parse(t *testing.T) {
	webhook := NewQuayWebhook("")

	tests := []struct {
		name         string
		payload      string
		expectedRepo string
		expectedTag  string
		expectError  bool
	}{
		{
			name: "valid payload",
			payload: `{
				"name": "repository",
				"repository": "mynamespace/repository",
				"namespace": "mynamespace",
				"docker_url": "quay.io/mynamespace/repository",
				"homepage": "https://quay.io/repository/mynamespace/repository",
				"updated_tags": [
					"latest"
				]
			}`,
			expectedRepo: "mynamespace/repository",
			expectedTag:  "latest",
			expectError:  false,
		},
		{
			name: "valid payload with multiple tags",
			payload: `{
				"name": "repository",
				"repository": "mynamespace/repository",
				"namespace": "mynamespace",
				"docker_url": "quay.io/mynamespace/repository",
				"homepage": "https://quay.io/repository/mynamespace/repository",
				"updated_tags": [
					"latest",
					"v1.0"
				]
			}`,
			expectedRepo: "mynamespace/repository",
			expectedTag:  "latest",
			expectError:  false,
		},
		{
			name: "valid payload with no tags",
			payload: `{
				"name": "repository",
				"repository": "mynamespace/repository",
				"namespace": "mynamespace",
				"docker_url": "quay.io/mynamespace/repository",
				"homepage": "https://quay.io/repository/mynamespace/repository",
				"updated_tags": [
				]
			}`,
			expectedRepo: "mynamespace/repository",
			expectedTag:  "",
			expectError:  true,
		},
		{
			name:         "invalid payload",
			payload:      `{"invalid": "data"}`,
			expectedRepo: "mynamespace/repository",
			expectedTag:  "latest",
			expectError:  true,
		},
		{
			name:         "empty JSON payload",
			payload:      `{}`,
			expectedRepo: "mynamespace/repository",
			expectedTag:  "latest",
			expectError:  true,
		},
		{
			name:         "empty payload",
			payload:      ``,
			expectedRepo: "mynamespace/repository",
			expectedTag:  "latest",
			expectError:  true,
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

			if event.RegistryURL != "quay.io" {
				t.Errorf("expected repository to be %q, got %q", tt.expectedRepo, event.Repository)
			}

			if event.Tag != tt.expectedTag {
				t.Errorf("expected tag to be %q, got %q", tt.expectedTag, event.Tag)
			}
		})
	}
}
