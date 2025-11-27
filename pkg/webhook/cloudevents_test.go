package webhook

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCloudEventsWebhook_GetRegistryType(t *testing.T) {
	webhook := NewCloudEventsWebhook("")
	assert.Equal(t, "cloudevents", webhook.GetRegistryType())
}

func TestCloudEventsWebhook_Validate(t *testing.T) {
	tests := []struct {
		name         string
		method       string
		contentType  string
		secret       string
		querySecret  string
		headerSecret string
		wantErr      bool
		errContains  string
	}{
		{
			name:        "valid request without secret",
			method:      http.MethodPost,
			contentType: "application/json",
			secret:      "",
			wantErr:     false,
		},
		{
			name:        "valid request with CloudEvents content type",
			method:      http.MethodPost,
			contentType: "application/cloudevents+json",
			secret:      "",
			wantErr:     false,
		},
		{
			name:        "valid request with query secret",
			method:      http.MethodPost,
			contentType: "application/json",
			secret:      "test-secret",
			querySecret: "test-secret",
			wantErr:     false,
		},
		{
			name:         "valid request with header secret",
			method:       http.MethodPost,
			contentType:  "application/json",
			secret:       "test-secret",
			headerSecret: "test-secret",
			wantErr:      false,
		},
		{
			name:        "invalid HTTP method",
			method:      http.MethodGet,
			contentType: "application/json",
			wantErr:     true,
			errContains: "invalid HTTP method",
		},
		{
			name:        "invalid content type",
			method:      http.MethodPost,
			contentType: "text/plain",
			wantErr:     true,
			errContains: "invalid content type",
		},
		{
			name:        "missing secret",
			method:      http.MethodPost,
			contentType: "application/json",
			secret:      "test-secret",
			wantErr:     true,
			errContains: "missing webhook secret",
		},
		{
			name:        "invalid secret",
			method:      http.MethodPost,
			contentType: "application/json",
			secret:      "test-secret",
			querySecret: "wrong-secret",
			wantErr:     true,
			errContains: "invalid webhook secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			webhook := NewCloudEventsWebhook(tt.secret)

			req := httptest.NewRequest(tt.method, "/webhook", nil)
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			if tt.querySecret != "" {
				q := req.URL.Query()
				q.Set("secret", tt.querySecret)
				req.URL.RawQuery = q.Encode()
			}
			if tt.headerSecret != "" {
				req.Header.Set("X-Webhook-Secret", tt.headerSecret)
			}

			err := webhook.Validate(req)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestCloudEventsWebhook_Parse_ECR(t *testing.T) {
	tests := []struct {
		name            string
		payload         map[string]interface{}
		wantRegistryURL string
		wantRepository  string
		wantTag         string
		wantDigest      string
		wantErr         bool
		errContains     string
	}{
		{
			name: "valid ECR push event",
			payload: map[string]interface{}{
				"specversion":     "1.0",
				"id":              "12345678-1234-1234-1234-123456789012",
				"type":            "com.amazon.ecr.image.push",
				"source":          "urn:aws:ecr:us-east-1:123456789012:repository/my-repo",
				"subject":         "my-repo:v1.0.0",
				"time":            "2025-11-27T10:00:00Z",
				"datacontenttype": "application/json",
				"data": map[string]interface{}{
					"repositoryName": "my-repo",
					"imageDigest":    "sha256:abcdef1234567890",
					"imageTag":       "v1.0.0",
					"registryId":     "123456789012",
				},
			},
			wantRegistryURL: "123456789012.dkr.ecr.us-east-1.amazonaws.com",
			wantRepository:  "my-repo",
			wantTag:         "v1.0.0",
			wantDigest:      "sha256:abcdef1234567890",
			wantErr:         false,
		},
		{
			name: "ECR event with namespace in repository name",
			payload: map[string]interface{}{
				"specversion":     "1.0",
				"id":              "12345678-1234-1234-1234-123456789012",
				"type":            "com.amazon.ecr.image.push",
				"source":          "urn:aws:ecr:eu-west-1:987654321098:repository/team/my-app",
				"subject":         "team/my-app:latest",
				"time":            "2025-11-27T10:00:00Z",
				"datacontenttype": "application/json",
				"data": map[string]interface{}{
					"repositoryName": "team/my-app",
					"imageDigest":    "sha256:1234567890abcdef",
					"imageTag":       "latest",
					"registryId":     "987654321098",
				},
			},
			wantRegistryURL: "987654321098.dkr.ecr.eu-west-1.amazonaws.com",
			wantRepository:  "team/my-app",
			wantTag:         "latest",
			wantDigest:      "sha256:1234567890abcdef",
			wantErr:         false,
		},
		{
			name: "ECR event with subject fallback",
			payload: map[string]interface{}{
				"specversion":     "1.0",
				"id":              "12345678-1234-1234-1234-123456789012",
				"type":            "com.amazon.ecr.image.push",
				"source":          "urn:aws:ecr:us-west-2:111111111111:repository/myrepo",
				"subject":         "myrepo:v2.1.0",
				"time":            "2025-11-27T10:00:00Z",
				"datacontenttype": "application/json",
				"data": map[string]interface{}{
					"imageDigest": "sha256:fedcba0987654321",
					"registryId":  "111111111111",
				},
			},
			wantRegistryURL: "111111111111.dkr.ecr.us-west-2.amazonaws.com",
			wantRepository:  "myrepo",
			wantTag:         "v2.1.0",
			wantDigest:      "sha256:fedcba0987654321",
			wantErr:         false,
		},
		{
			name: "missing repository name",
			payload: map[string]interface{}{
				"specversion":     "1.0",
				"id":              "12345678-1234-1234-1234-123456789012",
				"type":            "com.amazon.ecr.image.push",
				"source":          "urn:aws:ecr:us-east-1:123456789012:repository/test",
				"time":            "2025-11-27T10:00:00Z",
				"datacontenttype": "application/json",
				"data": map[string]interface{}{
					"imageTag":   "v1.0.0",
					"registryId": "123456789012",
				},
			},
			wantErr:     true,
			errContains: "repository name not found",
		},
		{
			name: "missing tag",
			payload: map[string]interface{}{
				"specversion":     "1.0",
				"id":              "12345678-1234-1234-1234-123456789012",
				"type":            "com.amazon.ecr.image.push",
				"source":          "urn:aws:ecr:us-east-1:123456789012:repository/test",
				"time":            "2025-11-27T10:00:00Z",
				"datacontenttype": "application/json",
				"data": map[string]interface{}{
					"repositoryName": "test",
					"registryId":     "123456789012",
				},
			},
			wantErr:     true,
			errContains: "tag not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			webhook := NewCloudEventsWebhook("")

			body, err := json.Marshal(tt.payload)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			event, err := webhook.Parse(req)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, event)
				assert.Equal(t, tt.wantRegistryURL, event.RegistryURL)
				assert.Equal(t, tt.wantRepository, event.Repository)
				assert.Equal(t, tt.wantTag, event.Tag)
				assert.Equal(t, tt.wantDigest, event.Digest)
			}
		})
	}
}

func TestCloudEventsWebhook_Parse_Generic(t *testing.T) {
	tests := []struct {
		name            string
		payload         map[string]interface{}
		wantRegistryURL string
		wantRepository  string
		wantTag         string
		wantDigest      string
		wantErr         bool
		errContains     string
	}{
		{
			name: "generic container push event",
			payload: map[string]interface{}{
				"specversion":     "1.0",
				"id":              "event-123",
				"type":            "com.example.container.push",
				"source":          "https://registry.example.com",
				"subject":         "myapp:v1.2.3",
				"time":            "2025-11-27T10:00:00Z",
				"datacontenttype": "application/json",
				"data": map[string]interface{}{
					"repository":  "myapp",
					"tag":         "v1.2.3",
					"digest":      "sha256:abc123",
					"registryUrl": "registry.example.com",
				},
			},
			wantRegistryURL: "registry.example.com",
			wantRepository:  "myapp",
			wantTag:         "v1.2.3",
			wantDigest:      "sha256:abc123",
			wantErr:         false,
		},
		{
			name: "generic image event with alternate field names",
			payload: map[string]interface{}{
				"specversion":     "1.0",
				"id":              "event-456",
				"type":            "org.container.image.published",
				"source":          "https://registry.company.com",
				"subject":         "project/app:2.0.0",
				"time":            "2025-11-27T10:00:00Z",
				"datacontenttype": "application/json",
				"data": map[string]interface{}{
					"repositoryName": "project/app",
					"imageTag":       "2.0.0",
					"imageDigest":    "sha256:def456",
					"registry":       "registry.company.com",
				},
			},
			wantRegistryURL: "registry.company.com",
			wantRepository:  "project/app",
			wantTag:         "2.0.0",
			wantDigest:      "sha256:def456",
			wantErr:         false,
		},
		{
			name: "event with source URL extraction",
			payload: map[string]interface{}{
				"specversion":     "1.0",
				"id":              "event-789",
				"type":            "container.push",
				"source":          "https://ghcr.io/owner/repo",
				"subject":         "owner/repo:v3.0.0",
				"time":            "2025-11-27T10:00:00Z",
				"datacontenttype": "application/json",
				"data": map[string]interface{}{
					"repository": "owner/repo",
					"tag":        "v3.0.0",
				},
			},
			wantRegistryURL: "ghcr.io",
			wantRepository:  "owner/repo",
			wantTag:         "v3.0.0",
			wantErr:         false,
		},
		{
			name: "missing specversion",
			payload: map[string]interface{}{
				"id":              "event-123",
				"type":            "container.push",
				"source":          "https://registry.example.com",
				"datacontenttype": "application/json",
				"data": map[string]interface{}{
					"repository": "myapp",
					"tag":        "v1.0.0",
				},
			},
			wantErr:     true,
			errContains: "missing CloudEvents specversion",
		},
		{
			name: "missing type",
			payload: map[string]interface{}{
				"specversion":     "1.0",
				"id":              "event-123",
				"source":          "https://registry.example.com",
				"datacontenttype": "application/json",
				"data": map[string]interface{}{
					"repository": "myapp",
					"tag":        "v1.0.0",
				},
			},
			wantErr:     true,
			errContains: "missing CloudEvents type",
		},
		{
			name: "unsupported event type",
			payload: map[string]interface{}{
				"specversion":     "1.0",
				"id":              "event-123",
				"type":            "com.example.database.updated",
				"source":          "https://db.example.com",
				"datacontenttype": "application/json",
				"data": map[string]interface{}{
					"table": "users",
				},
			},
			wantErr:     true,
			errContains: "unsupported CloudEvents type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			webhook := NewCloudEventsWebhook("")

			body, err := json.Marshal(tt.payload)
			require.NoError(t, err)

			req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
			req.Header.Set("Content-Type", "application/json")

			event, err := webhook.Parse(req)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				require.NotNil(t, event)
				assert.Equal(t, tt.wantRegistryURL, event.RegistryURL)
				assert.Equal(t, tt.wantRepository, event.Repository)
				assert.Equal(t, tt.wantTag, event.Tag)
				if tt.wantDigest != "" {
					assert.Equal(t, tt.wantDigest, event.Digest)
				}
			}
		})
	}
}

func TestCloudEventsWebhook_ExtractECRRegistryURL(t *testing.T) {
	tests := []struct {
		name   string
		source string
		data   map[string]interface{}
		want   string
	}{
		{
			name:   "valid ECR URN",
			source: "urn:aws:ecr:us-east-1:123456789012:repository/myrepo",
			data:   map[string]interface{}{},
			want:   "123456789012.dkr.ecr.us-east-1.amazonaws.com",
		},
		{
			name:   "ECR URN with registryId fallback",
			source: "urn:aws:ecr:eu-west-1:987654321098:repository/test",
			data: map[string]interface{}{
				"registryId": "987654321098",
			},
			want: "987654321098.dkr.ecr.eu-west-1.amazonaws.com",
		},
		{
			name:   "invalid source format",
			source: "invalid-source",
			data:   map[string]interface{}{},
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			webhook := NewCloudEventsWebhook("")
			got := webhook.extractECRRegistryURL(tt.source, tt.data)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestCloudEventsWebhook_ExtractRegistryFromSource(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "https URL",
			source: "https://registry.example.com/repo",
			want:   "registry.example.com",
		},
		{
			name:   "http URL",
			source: "http://localhost:5000/myrepo",
			want:   "localhost:5000",
		},
		{
			name:   "domain pattern",
			source: "ghcr.io/owner/repo",
			want:   "ghcr.io",
		},
		{
			name:   "no domain pattern",
			source: "myrepo",
			want:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			webhook := NewCloudEventsWebhook("")
			got := webhook.extractRegistryFromSource(tt.source)
			assert.Equal(t, tt.want, got)
		})
	}
}
