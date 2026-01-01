package webhook

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd"
)

func TestParseArtifactRegistryMessage(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		wantEvent   bool
		wantErr     bool
		errContains string
		registry    string
		repository  string
		tag         string
		digest      string
	}{
		{
			name:       "valid INSERT with digest from data",
			data:       []byte(`{"action":"INSERT","digest":"us-docker.pkg.dev/my-project/my-repo/my-image@sha256:abc123","tag":"v1.0.0"}`),
			wantEvent:  true,
			wantErr:    false,
			registry:   "us-docker.pkg.dev",
			repository: "my-project/my-repo/my-image",
			tag:        "v1.0.0",
			digest:     "sha256:abc123",
		},
		{
			name:        "DELETE action is ignored",
			data:        []byte(`{"action":"DELETE","digest":"us-docker.pkg.dev/project/repo/image@sha256:abc123"}`),
			wantEvent:   false,
			wantErr:     true,
			errContains: "ignoring non-INSERT action",
		},
		{
			name:        "empty data",
			data:        nil,
			wantEvent:   false,
			wantErr:     true,
			errContains: "empty message data",
		},
		{
			name:       "GCR registry format",
			data:       []byte(`{"action":"INSERT","digest":"gcr.io/my-project/my-image@sha256:xyz789","tag":"v2.0.0"}`),
			wantEvent:  true,
			wantErr:    false,
			registry:   "gcr.io",
			repository: "my-project/my-image",
			tag:        "v2.0.0",
			digest:     "sha256:xyz789",
		},
		{
			name:       "regional GCR registry format",
			data:       []byte(`{"action":"INSERT","digest":"us.gcr.io/my-project/my-image@sha256:xyz789","tag":"v2.0.0"}`),
			wantEvent:  true,
			wantErr:    false,
			registry:   "us.gcr.io",
			repository: "my-project/my-image",
			tag:        "v2.0.0",
			digest:     "sha256:xyz789",
		},
		{
			name:       "data is parsed",
			data:       []byte(`{"action":"INSERT","digest":"us-docker.pkg.dev/old/old/old@sha256:old","tag":"old"}`),
			wantEvent:  true,
			wantErr:    false,
			registry:   "us-docker.pkg.dev",
			repository: "old/old/old",
			tag:        "old",
			digest:     "sha256:old",
		},
		{
			name:        "invalid JSON data",
			data:        []byte(`{invalid json}`),
			wantEvent:   false,
			wantErr:     true,
			errContains: "failed to unmarshal",
		},
		{
			name:        "non-GCP registry",
			data:        []byte(`{"action":"INSERT","digest":"docker.io/library/nginx@sha256:abc123"}`),
			wantEvent:   false,
			wantErr:     true,
			errContains: "not a recognized GCP container registry",
		},
		{
			name:       "digest without tag",
			data:       []byte(`{"action":"INSERT","digest":"us-docker.pkg.dev/project/repo/image@sha256:abc123"}`),
			wantEvent:  true,
			wantErr:    false,
			registry:   "us-docker.pkg.dev",
			repository: "project/repo/image",
			tag:        "",
			digest:     "sha256:abc123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := ParseArtifactRegistryMessage(tt.data)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
				assert.Nil(t, event)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, event)

			assert.Equal(t, tt.registry, event.RegistryURL)
			assert.Equal(t, tt.repository, event.Repository)
			assert.Equal(t, tt.tag, event.Tag)
			assert.Equal(t, tt.digest, event.Digest)
		})
	}
}

func TestParseArtifactRegistryDigest(t *testing.T) {
	tests := []struct {
		name       string
		digest     string
		wantReg    string
		wantRepo   string
		wantErr    bool
		errContain string
	}{
		{
			name:     "Artifact Registry with digest",
			digest:   "us-docker.pkg.dev/my-project/my-repo/my-image@sha256:abc123",
			wantReg:  "us-docker.pkg.dev",
			wantRepo: "my-project/my-repo/my-image",
			wantErr:  false,
		},
		{
			name:     "Artifact Registry with tag",
			digest:   "us-docker.pkg.dev/my-project/my-repo/my-image:v1.0.0",
			wantReg:  "us-docker.pkg.dev",
			wantRepo: "my-project/my-repo/my-image",
			wantErr:  false,
		},
		{
			name:     "GCR with digest",
			digest:   "gcr.io/my-project/my-image@sha256:abc123",
			wantReg:  "gcr.io",
			wantRepo: "my-project/my-image",
			wantErr:  false,
		},
		{
			name:     "regional GCR",
			digest:   "eu.gcr.io/my-project/my-image@sha256:abc123",
			wantReg:  "eu.gcr.io",
			wantRepo: "my-project/my-image",
			wantErr:  false,
		},
		{
			name:       "empty digest",
			digest:     "",
			wantErr:    true,
			errContain: "empty digest",
		},
		{
			name:       "invalid format",
			digest:     "invalid",
			wantErr:    true,
			errContain: "invalid image path format",
		},
		{
			name:       "non-GCP registry",
			digest:     "docker.io/library/nginx@sha256:abc123",
			wantErr:    true,
			errContain: "not a recognized GCP container registry",
		},
		{
			name:     "nested repo path",
			digest:   "us-docker.pkg.dev/project/repo/path/to/image@sha256:abc",
			wantReg:  "us-docker.pkg.dev",
			wantRepo: "project/repo/path/to/image",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			reg, repo, err := parseArtifactRegistryDigest(tt.digest)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errContain != "" {
					assert.Contains(t, err.Error(), tt.errContain)
				}
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.wantReg, reg)
			assert.Equal(t, tt.wantRepo, repo)
		})
	}
}

func TestDefaultPubSubSubscriberConfig(t *testing.T) {
	cfg := DefaultPubSubSubscriberConfig()

	assert.False(t, cfg.Enabled)
	assert.Equal(t, "", cfg.ProjectID)
	assert.Equal(t, "", cfg.SubscriptionID)
	assert.Equal(t, "", cfg.CredentialsFile)
	assert.Equal(t, 100, cfg.MaxOutstandingMessages)
	assert.Equal(t, 100*1024*1024, cfg.MaxOutstandingBytes)
	assert.Equal(t, 4, cfg.NumGoroutines)
}

func TestIsArtifactRegistryIgnorableError(t *testing.T) {
	tests := []struct {
		name     string
		errStr   string
		expected bool
	}{
		{
			name:     "non-INSERT action",
			errStr:   "ignoring non-INSERT action: DELETE",
			expected: true,
		},
		{
			name:     "empty message",
			errStr:   "empty message data",
			expected: true,
		},
		{
			name:     "parse error",
			errStr:   "failed to unmarshal Artifact Registry message",
			expected: false,
		},
		{
			name:     "other error",
			errStr:   "network timeout",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := fmt.Errorf("%s", tt.errStr)
			result := isArtifactRegistryIgnorableError(err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsArtifactRegistryIgnorableError_Nil(t *testing.T) {
	assert.False(t, isArtifactRegistryIgnorableError(nil))
}

func TestNewArtifactRegistrySubscriber(t *testing.T) {
	cfg := &PubSubSubscriberConfig{
		Enabled:        true,
		ProjectID:      "test-project",
		SubscriptionID: "test-subscription",
	}

	handler := func(ctx context.Context, event *argocd.WebhookEvent) error {
		return nil
	}

	sub := NewArtifactRegistrySubscriber(cfg, handler)

	assert.NotNil(t, sub)
	assert.Equal(t, cfg, sub.config)
	assert.NotNil(t, sub.eventHandler)
	assert.Nil(t, sub.client)
}

func TestArtifactRegistrySubscriber_NeedLeaderElection(t *testing.T) {
	sub := &ArtifactRegistrySubscriber{}
	assert.True(t, sub.NeedLeaderElection())
}

func TestNewArtifactRegistryWebhook(t *testing.T) {
	webhook := NewArtifactRegistryWebhook("test-secret")
	assert.NotNil(t, webhook)
	assert.Equal(t, "test-secret", webhook.secret)
}

func TestArtifactRegistryWebhook_GetRegistryType(t *testing.T) {
	webhook := NewArtifactRegistryWebhook("")
	assert.Equal(t, "artifact-registry", webhook.GetRegistryType())
}

func TestArtifactRegistryWebhook_Parse(t *testing.T) {
	webhook := NewArtifactRegistryWebhook("")

	// Create a Pub/Sub push envelope with base64-encoded Artifact Registry message
	arMessage := `{"action":"INSERT","digest":"us-docker.pkg.dev/my-project/my-repo/my-image@sha256:abc123","tag":"v1.0.0"}`
	encodedData := base64.StdEncoding.EncodeToString([]byte(arMessage))

	pushEnvelope := fmt.Sprintf(`{
		"message": {
			"data": "%s",
			"messageId": "123",
			"publishTime": "2026-01-01T00:00:00Z"
		},
		"subscription": "projects/my-project/subscriptions/my-subscription"
	}`, encodedData)

	req := httptest.NewRequest(http.MethodPost, "/webhook?type=artifact-registry", strings.NewReader(pushEnvelope))

	event, err := webhook.Parse(req)
	require.NoError(t, err)
	require.NotNil(t, event)

	assert.Equal(t, "us-docker.pkg.dev", event.RegistryURL)
	assert.Equal(t, "my-project/my-repo/my-image", event.Repository)
	assert.Equal(t, "v1.0.0", event.Tag)
	assert.Equal(t, "sha256:abc123", event.Digest)
}

func TestArtifactRegistryWebhook_Parse_Unwrapped(t *testing.T) {
	webhook := NewArtifactRegistryWebhook("")

	// Unwrapped push delivers raw message data as the HTTP body.
	arMessage := `{"action":"INSERT","digest":"us-docker.pkg.dev/my-project/my-repo/my-image@sha256:abc123","tag":"v1.0.0"}`
	req := httptest.NewRequest(http.MethodPost, "/webhook?type=artifact-registry", strings.NewReader(arMessage))

	event, err := webhook.Parse(req)
	require.NoError(t, err)
	require.NotNil(t, event)

	assert.Equal(t, "us-docker.pkg.dev", event.RegistryURL)
	assert.Equal(t, "my-project/my-repo/my-image", event.Repository)
	assert.Equal(t, "v1.0.0", event.Tag)
	assert.Equal(t, "sha256:abc123", event.Digest)
}

func TestArtifactRegistryWebhook_Parse_IgnoredAction(t *testing.T) {
	webhook := NewArtifactRegistryWebhook("")

	// Non-INSERT actions are intentionally ignored (ACK semantics).
	arMessage := `{"action":"DELETE","digest":"us-docker.pkg.dev/my-project/my-repo/my-image@sha256:abc123","tag":"v1.0.0"}`
	encodedData := base64.StdEncoding.EncodeToString([]byte(arMessage))

	pushEnvelope := fmt.Sprintf(`{
		"message": {
			"data": "%s",
			"messageId": "123",
			"publishTime": "2026-01-01T00:00:00Z"
		},
		"subscription": "projects/my-project/subscriptions/my-subscription"
	}`, encodedData)

	req := httptest.NewRequest(http.MethodPost, "/webhook?type=artifact-registry", strings.NewReader(pushEnvelope))

	event, err := webhook.Parse(req)
	require.Nil(t, event)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrWebhookIgnored)
}

func TestArtifactRegistryWebhook_Parse_IgnoredAction_Unwrapped(t *testing.T) {
	webhook := NewArtifactRegistryWebhook("")

	// Unwrapped non-INSERT actions are ignored (ACK semantics).
	arMessage := `{"action":"DELETE","digest":"us-docker.pkg.dev/my-project/my-repo/my-image@sha256:abc123","tag":"v1.0.0"}`
	req := httptest.NewRequest(http.MethodPost, "/webhook?type=artifact-registry", strings.NewReader(arMessage))

	event, err := webhook.Parse(req)
	require.Nil(t, event)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrWebhookIgnored)
}

func TestArtifactRegistryWebhook_Validate(t *testing.T) {
	tests := []struct {
		name      string
		secret    string
		urlSecret string
		method    string
		wantErr   bool
	}{
		{
			name:    "valid POST without secret",
			secret:  "",
			method:  http.MethodPost,
			wantErr: false,
		},
		{
			name:      "valid POST with matching secret",
			secret:    "my-secret",
			urlSecret: "my-secret",
			method:    http.MethodPost,
			wantErr:   false,
		},
		{
			name:      "invalid secret",
			secret:    "my-secret",
			urlSecret: "wrong-secret",
			method:    http.MethodPost,
			wantErr:   true,
		},
		{
			name:    "missing secret when required",
			secret:  "my-secret",
			method:  http.MethodPost,
			wantErr: true,
		},
		{
			name:    "invalid method",
			secret:  "",
			method:  http.MethodGet,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			webhook := NewArtifactRegistryWebhook(tt.secret)

			url := "/webhook?type=artifact-registry"
			if tt.urlSecret != "" {
				url += "&secret=" + tt.urlSecret
			}

			req := httptest.NewRequest(tt.method, url, nil)
			err := webhook.Validate(req)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
