package webhook

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewACRWebhook(t *testing.T) {
	secret := "test-secret"
	webhook := NewACRWebhook(secret)

	assert.NotNil(t, webhook, "webhook was nil")
	assert.Equal(t, secret, webhook.secret, "Secret is not the same expected %s but got %s", secret, webhook.secret)
}

func TestACRWebhook_GetRegistryType(t *testing.T) {
	webhook := NewACRWebhook("")
	registryType := webhook.GetRegistryType()

	assert.NotNil(t, webhook, "Webhook was nil")
	assert.Equal(t, "acr", registryType, "Registry type is not acr got: %s", registryType)
}

func TestACRWebhook_Validate(t *testing.T) {
	secret := "test-secret"
	webhook := NewACRWebhook(secret)

	tests := []struct {
		name           string
		method         string
		contentType    string
		authHeader     string
		noSecret       bool
		expectError    bool
		expectedErrMsg string
	}{
		{
			name:        "valid POST request with correct secret",
			method:      "POST",
			contentType: "application/json",
			authHeader:  "test-secret",
			expectError: false,
		},
		{
			name:        "valid POST request without secret",
			method:      "POST",
			contentType: "application/json",
			noSecret:    true,
			expectError: false,
		},
		{
			name:        "invalid HTTP method",
			method:      "GET",
			contentType: "application/json",
			authHeader:  "test-secret",
			expectError: true,
		},
		{
			name:        "invalid content type",
			method:      "POST",
			contentType: "text/plain",
			authHeader:  "test-secret",
			expectError: true,
		},
		{
			name:           "missing Authorization header when secret is configured",
			method:         "POST",
			contentType:    "application/json",
			authHeader:     "",
			expectError:    true,
			expectedErrMsg: "missing Authorization header when secret is configured",
		},
		{
			name:           "incorrect secret",
			method:         "POST",
			contentType:    "application/json",
			authHeader:     "not-the-secret",
			expectError:    true,
			expectedErrMsg: "invalid webhook secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testWebhook := webhook
			if tt.noSecret {
				testWebhook = NewACRWebhook("")
			}

			req := httptest.NewRequest(tt.method, "/webhook", nil)
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			err := testWebhook.Validate(req)

			if tt.expectError {
				assert.Error(t, err)
				if tt.expectedErrMsg != "" {
					assert.EqualError(t, err, tt.expectedErrMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestACRWebhook_Parse(t *testing.T) {
	tests := []struct {
		name                string
		payload             string
		expectedRepo        string
		expectedTag         string
		expectedDigest      string
		expectedRegistryURL string
		expectError         bool
	}{
		{
			name: "valid push payload",
			payload: `{
				"id": "ea889308-3834-4a1d-a631-cadc572dab91",
				"timestamp": "2026-06-01T12:22:07.0958813Z",
				"action": "push",
				"target": {
					"mediaType": "application/vnd.docker.distribution.manifest.v2+json",
					"size": 524,
					"digest": "sha256:c766679d161d4ffe3dc4503b4c9f90b978f0d363fcedb02d1ae0cd271e645c0a",
					"length": 524,
					"repository": "hello-world",
					"tag": "test-v1"
				},
				"request": {
					"id": "9ad4af4e-95b2-4787-8f28-7f817b05a56a",
					"host": "mojrodevops.azurecr.io",
					"method": "PUT"
				}
			}`,
			expectedRepo:        "hello-world",
			expectedTag:         "test-v1",
			expectedDigest:      "sha256:c766679d161d4ffe3dc4503b4c9f90b978f0d363fcedb02d1ae0cd271e645c0a",
			expectedRegistryURL: "mojrodevops.azurecr.io",
			expectError:         false,
		},
		{
			name: "digest-only push with empty tag",
			payload: `{
				"action": "push",
				"target": {
					"digest": "sha256:c766679d161d4ffe3dc4503b4c9f90b978f0d363fcedb02d1ae0cd271e645c0a",
					"repository": "hello-world",
					"tag": ""
				},
				"request": {
					"host": "mojrodevops.azurecr.io"
				}
			}`,
			expectedRepo:        "hello-world",
			expectedTag:         "",
			expectedDigest:      "sha256:c766679d161d4ffe3dc4503b4c9f90b978f0d363fcedb02d1ae0cd271e645c0a",
			expectedRegistryURL: "mojrodevops.azurecr.io",
			expectError:         false,
		},
		{
			name: "non-push action is ignored",
			payload: `{
				"action": "delete",
				"target": {
					"repository": "hello-world",
					"tag": "test-v1"
				},
				"request": {
					"host": "mojrodevops.azurecr.io"
				}
			}`,
			expectError: true,
		},
		{
			name: "missing repository",
			payload: `{
				"action": "push",
				"target": {
					"tag": "test-v1"
				},
				"request": {
					"host": "mojrodevops.azurecr.io"
				}
			}`,
			expectError: true,
		},
		{
			name:        "malformed JSON",
			payload:     `{"action": "push", "target": }`,
			expectError: true,
		},
		{
			name:        "empty payload",
			payload:     ``,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			webhook := NewACRWebhook("")
			req := httptest.NewRequest("POST", "/webhook", strings.NewReader(tt.payload))

			event, err := webhook.Parse(req)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.NotNil(t, event, "Event was nil")
			assert.Equal(t, tt.expectedRegistryURL, event.RegistryURL, "Expected registry url to be %s, but got %s", tt.expectedRegistryURL, event.RegistryURL)
			assert.Equal(t, tt.expectedRepo, event.Repository, "Expected repository to be %s, but got %s", tt.expectedRepo, event.Repository)
			assert.Equal(t, tt.expectedTag, event.Tag, "Expected tag to be %s, but got %s", tt.expectedTag, event.Tag)
			assert.Equal(t, tt.expectedDigest, event.Digest, "Expected digest to be %s, but got %s", tt.expectedDigest, event.Digest)
		})
	}
}
