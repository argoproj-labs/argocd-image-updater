package webhook

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewQuayWebhook(t *testing.T) {
	secret := "test"
	webhook := NewDockerHubWebhook(secret)

	assert.NotNil(t, webhook, "webhook was nil")
	assert.Equal(t, webhook.secret, secret, "Secret is not the same expected %s but got %s", secret, webhook.secret)
}

func TestQuayWebhook_GetRegistryType(t *testing.T) {
	webhook := NewQuayWebhook("")
	registryType := webhook.GetRegistryType()

	assert.NotNil(t, webhook, "Webhook was nil")
	assert.Equal(t, registryType, "quay.io", "Registry type is not quay.io got: %s", registryType)
}

func TestQuayWebhook_Validate(t *testing.T) {
	secret := "test-secret"
	webhook := NewQuayWebhook(secret)

	tests := []struct {
		name        string
		method      string
		body        string
		secret      string
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
			name:        "valid POST request with incorrect secret",
			method:      "POST",
			body:        `{"test": "data"}`,
			secret:      "this-is-not-the-secret",
			expectError: true,
		},
		{
			name:        "incorrect method",
			method:      "GET",
			body:        `{"test": "data"}`,
			secret:      "test-secret",
			expectError: true,
		},
		{
			name:        "empty secret when secret is set",
			method:      "POST",
			body:        `{"test": "data"}`,
			secret:      "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testWebhook := webhook

			req := httptest.NewRequest(tt.method, "/webhook", strings.NewReader(tt.body))
			if tt.secret != "" {
				query := req.URL.Query()
				query.Set("secret", tt.secret)
				req.URL.RawQuery = query.Encode()
			}

			err := testWebhook.Validate(req)

			if tt.expectError {
				assert.Error(t, err)
			}
			if !tt.expectError {
				assert.NoError(t, err)
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
				assert.Error(t, err)
				return
			}

			if err != nil {
				assert.NoError(t, err)
				return
			}

			assert.NotNil(t, event, "Event was nil")
			assert.Equal(t, event.RegistryURL, "quay.io", "Expected repository url to be %s, but got %s", "quay.io", event.RegistryURL)
			assert.Equal(t, event.Repository, tt.expectedRepo, "Expect repository to be %s, but got %s", tt.expectedRepo, event.Repository)
			assert.Equal(t, event.Tag, tt.expectedTag, "Expected tag to be %s, but got %s", tt.expectedTag, event.Tag)
		})
	}
}
