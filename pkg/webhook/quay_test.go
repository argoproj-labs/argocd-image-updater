package webhook

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewQuayWebhook(t *testing.T) {
	secret := "test"
	webhook := NewQuayWebhook(secret)

	assert.NotNil(t, webhook, "webhook was nil")
	assert.Equal(t, secret, webhook.secret, "Secret is not the same expected %s but got %s", secret, webhook.secret)
}

func TestQuayWebhook_GetRegistryType(t *testing.T) {
	webhook := NewQuayWebhook("")
	registryType := webhook.GetRegistryType()

	assert.NotNil(t, webhook, "Webhook was nil")
	assert.Equal(t, "quay.io", registryType, "Registry type is not quay.io got: %s", registryType)
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
		name             string
		payload          string
		expectedRegistry string
		expectedRepo     string
		expectedTag      string
		expectError      bool
	}{
		{
			name: "valid payload with quay.io docker_url",
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
			expectedRegistry: "quay.io",
			expectedRepo:     "mynamespace/repository",
			expectedTag:      "latest",
			expectError:      false,
		},
		{
			name: "private quay registry in docker_url",
			payload: `{
				"name": "repository",
				"repository": "mynamespace/repository",
				"namespace": "mynamespace",
				"docker_url": "quay.apps.example.com/mynamespace/repository",
				"homepage": "https://quay.apps.example.com/repository/mynamespace/repository",
				"updated_tags": [
					"dev"
				]
			}`,
			expectedRegistry: "quay.apps.example.com",
			expectedRepo:     "mynamespace/repository",
			expectedTag:      "dev",
			expectError:      false,
		},
		{
			name: "private quay registry with https scheme in docker_url",
			payload: `{
				"name": "repository",
				"repository": "ariss/alrl",
				"namespace": "ariss",
				"docker_url": "https://quay.internal.corp/ariss/alrl",
				"homepage": "https://quay.internal.corp/repository/ariss/alrl",
				"updated_tags": [
					"v1.0"
				]
			}`,
			expectedRegistry: "quay.internal.corp",
			expectedRepo:     "ariss/alrl",
			expectedTag:      "v1.0",
			expectError:      false,
		},
		{
			name: "empty docker_url falls back to quay.io",
			payload: `{
				"name": "repository",
				"repository": "mynamespace/repository",
				"namespace": "mynamespace",
				"docker_url": "",
				"updated_tags": [
					"latest"
				]
			}`,
			expectedRegistry: "quay.io",
			expectedRepo:     "mynamespace/repository",
			expectedTag:      "latest",
			expectError:      false,
		},
		{
			name: "missing docker_url falls back to quay.io",
			payload: `{
				"name": "repository",
				"repository": "mynamespace/repository",
				"namespace": "mynamespace",
				"updated_tags": [
					"latest"
				]
			}`,
			expectedRegistry: "quay.io",
			expectedRepo:     "mynamespace/repository",
			expectedTag:      "latest",
			expectError:      false,
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
			expectedRegistry: "quay.io",
			expectedRepo:     "mynamespace/repository",
			expectedTag:      "latest",
			expectError:      false,
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
			assert.Equal(t, tt.expectedRegistry, event.RegistryURL)
			assert.Equal(t, tt.expectedRepo, event.Repository)
			assert.Equal(t, tt.expectedTag, event.Tag)
		})
	}
}

func TestExtractRegistryFromDockerURL(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"public quay.io", "quay.io/ns/repo", "quay.io"},
		{"private hostname", "quay.apps.example.com/ns/repo", "quay.apps.example.com"},
		{"with https scheme", "https://quay.internal.corp/ns/repo", "quay.internal.corp"},
		{"with http scheme", "http://quay.internal.corp/ns/repo", "quay.internal.corp"},
		{"hostname with port", "quay.internal.corp:8443/ns/repo", "quay.internal.corp:8443"},
		{"bare hostname no path", "quay.apps.example.com", "quay.apps.example.com"},
		{"manual split fallback", "quay.custom.io:bad:port/ns/repo", "quay.custom.io:bad:port"},
		{"unparseable url falls back to quay.io", "/ns/repo", "quay.io"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractRegistryFromDockerURL(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
