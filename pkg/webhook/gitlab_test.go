package webhook

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewGitLabWebhook(t *testing.T) {
	secret := "test-secret"
	webhook := NewGitLabWebhook(secret)

	if webhook == nil {
		t.Fatal("expected webhook to be non-nil")
	}
	if webhook.secret != secret {
		t.Errorf("expected secret to be %q, got %q", secret, webhook.secret)
	}
}

func TestGitLabWebhook_GetRegistryType(t *testing.T) {
	webhook := NewGitLabWebhook("")
	registryType := webhook.GetRegistryType()

	expected := "gitlab"
	if registryType != expected {
		t.Errorf("expected registry type to be %q, got %q", expected, registryType)
	}
}

func TestGitLabWebhook_Validate(t *testing.T) {
	secret := "test-secret"
	webhook := NewGitLabWebhook(secret)

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
			contentType: "application/vnd.docker.distribution.events.v1+json",
			body:        `{"events": []}`,
			authHeader:  secret,
			expectError: false,
		},
		{
			name:        "valid POST request without secret validation",
			method:      "POST",
			contentType: "application/vnd.docker.distribution.events.v1+json",
			body:        `{"events": []}`,
			noSecret:    true,
			expectError: false,
		},
		{
			name:        "invalid HTTP method",
			method:      "GET",
			contentType: "application/vnd.docker.distribution.events.v1+json",
			body:        `{"events": []}`,
			authHeader:  secret,
			expectError: true,
		},
		{
			name:        "invalid content type",
			method:      "POST",
			contentType: "application/json",
			body:        `{"events": []}`,
			authHeader:  secret,
			expectError: true,
		},
		{
			name:           "missing Authorization header when secret is configured",
			method:         "POST",
			contentType:    "application/vnd.docker.distribution.events.v1+json",
			body:           `{"events": []}`,
			authHeader:     "",
			expectError:    true,
			expectedErrMsg: "missing Authorization header when secret is configured",
		},
		{
			name:           "incorrect secret",
			method:         "POST",
			contentType:    "application/vnd.docker.distribution.events.v1+json",
			body:           `{"events": []}`,
			authHeader:     "wrong-secret",
			expectError:    true,
			expectedErrMsg: "incorrect webhook secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testWebhook := webhook
			if tt.noSecret {
				testWebhook = NewGitLabWebhook("")
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

func TestGitLabWebhook_Parse(t *testing.T) {
	webhook := NewGitLabWebhook("")

	tests := []struct {
		name           string
		payload        string
		expectedCount  int
		expectedRepo   string
		expectedTag    string
		expectedURL    string
		expectedDigest string
		expectError    bool
	}{
		{
			name: "valid push event with host and port in request host",
			payload: `{
				"events": [{
					"action": "push",
					"target": {
						"repository": "mygroup/myimage",
						"tag": "latest",
						"digest": "sha256:abc123",
						"mediaType": "application/vnd.docker.distribution.manifest.v2+json"
					},
					"request": { "host": "registry.example.com:5000" },
					"source": { "addr": "gitlab:5000" }
				}]
			}`,
			expectedCount:  1,
			expectedRepo:   "mygroup/myimage",
			expectedTag:    "latest",
			expectedURL:    "registry.example.com:5000",
			expectedDigest: "sha256:abc123",
			expectError:    false,
		},
		{
			name: "valid push event with plain host in request host",
			payload: `{
				"events": [{
					"action": "push",
					"target": {
						"repository": "mygroup/myimage",
						"tag": "v1.0.0",
						"digest": "sha256:def456"
					},
					"request": { "host": "registry.example.com" },
					"source": { "addr": "gitlab:5000" }
				}]
			}`,
			expectedCount:  1,
			expectedRepo:   "mygroup/myimage",
			expectedTag:    "v1.0.0",
			expectedURL:    "registry.example.com",
			expectedDigest: "sha256:def456",
			expectError:    false,
		},
		{
			name: "blob push events without tag are filtered out",
			payload: `{
				"events": [
					{
						"action": "push",
						"target": {
							"repository": "mygroup/myimage",
							"tag": "",
							"digest": "sha256:abc123",
							"mediaType": "application/vnd.docker.container.image.rootfs.diff+x-gtar"
						},
						"request": { "host": "registry.example.com" },
						"source": { "addr": "gitlab:5000" }
					},
					{
						"action": "push",
						"target": {
							"repository": "mygroup/myimage",
							"tag": "latest",
							"digest": "sha256:def456",
							"mediaType": "application/vnd.docker.distribution.manifest.v2+json"
						},
						"request": { "host": "registry.example.com" },
						"source": { "addr": "gitlab:5000" }
					}
				]
			}`,
			expectedCount:  1,
			expectedRepo:   "mygroup/myimage",
			expectedTag:    "latest",
			expectedURL:    "registry.example.com",
			expectedDigest: "sha256:def456",
			expectError:    false,
		},
		{
			name: "multiple push events in one payload",
			payload: `{
				"events": [
					{
						"action": "push",
						"target": {
							"repository": "group/imageA",
							"tag": "v1.0.0",
							"digest": "sha256:aaa"
						},
						"request": { "host": "registry.example.com" },
						"source": { "addr": "gitlab:5000" }
					},
					{
						"action": "push",
						"target": {
							"repository": "group/imageB",
							"tag": "v2.0.0",
							"digest": "sha256:bbb"
						},
						"request": { "host": "registry.example.com" },
						"source": { "addr": "gitlab:5000" }
					}
				]
			}`,
			expectedCount: 2,
			expectError:   false,
		},
		{
			name: "mixed push and non-push events - only push returned",
			payload: `{
				"events": [
					{
						"action": "push",
						"target": {
							"repository": "group/myimage",
							"tag": "v1.0.0",
							"digest": "sha256:aaa"
						},
						"request": { "host": "registry.example.com" },
						"source": { "addr": "gitlab:5000" }
					},
					{
						"action": "delete",
						"target": {
							"repository": "group/myimage",
							"tag": "old-tag",
							"digest": "sha256:bbb"
						},
						"request": { "host": "registry.example.com" },
						"source": { "addr": "gitlab:5000" }
					},
					{
						"action": "pull",
						"target": {
							"repository": "group/myimage",
							"tag": "v1.0.0",
							"digest": "sha256:aaa"
						},
						"request": { "host": "registry.example.com" },
						"source": { "addr": "gitlab:5000" }
					}
				]
			}`,
			expectedCount: 1,
			expectedRepo:  "group/myimage",
			expectedTag:   "v1.0.0",
			expectedURL:   "registry.example.com",
			expectError:   false,
		},
		{
			name: "all non-push events returns empty result",
			payload: `{
				"events": [
					{
						"action": "delete",
						"target": {
							"repository": "group/myimage",
							"tag": "v1.0.0"
						},
						"request": { "host": "registry.example.com" },
						"source": { "addr": "gitlab:5000" }
					},
					{
						"action": "pull",
						"target": {
							"repository": "group/myimage",
							"tag": "v1.0.0"
						},
						"request": { "host": "registry.example.com" },
						"source": { "addr": "gitlab:5000" }
					}
				]
			}`,
			expectedCount: 0,
			expectError:   false,
		},
		{
			name:        "empty events array",
			payload:     `{"events": []}`,
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
			req.Header.Set("Content-Type", "application/vnd.docker.distribution.events.v2+json")

			events, err := webhook.Parse(req)

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

			if len(events) != tt.expectedCount {
				t.Fatalf("expected %d events, got %d", tt.expectedCount, len(events))
			}

			if tt.expectedCount == 0 {
				return
			}

			if tt.expectedCount == 1 {
				if events[0].RegistryURL != tt.expectedURL {
					t.Errorf("expected registry URL to be %q, got %q", tt.expectedURL, events[0].RegistryURL)
				}
				if events[0].Repository != tt.expectedRepo {
					t.Errorf("expected repository to be %q, got %q", tt.expectedRepo, events[0].Repository)
				}
				if events[0].Tag != tt.expectedTag {
					t.Errorf("expected tag to be %q, got %q", tt.expectedTag, events[0].Tag)
				}
				if tt.expectedDigest != "" && events[0].Digest != tt.expectedDigest {
					t.Errorf("expected digest to be %q, got %q", tt.expectedDigest, events[0].Digest)
				}
			}

			if tt.expectedCount == 2 {
				repos := map[string]bool{}
				for _, e := range events {
					repos[e.Repository] = true
				}
				if !repos["group/imageA"] || !repos["group/imageB"] {
					t.Errorf("expected events for group/imageA and group/imageB, got %v", repos)
				}
			}
		})
	}
}

func TestGitLabWebhook_ParseWithBodyReuse(t *testing.T) {
	secret := "test-secret"
	webhook := NewGitLabWebhook(secret)

	payload := `{
		"events": [{
			"action": "push",
			"target": {
				"repository": "mygroup/myimage",
				"tag": "v1.0.0",
				"digest": "sha256:abc123"
			},
			"request": { "host": "registry.example.com:5000" },
			"source": { "addr": "gitlab:5000" }
		}]
	}`

	req := httptest.NewRequest("POST", "/webhook", strings.NewReader(payload))
	req.Header.Set("Content-Type", "application/vnd.docker.distribution.events.v1+json")
	req.Header.Set("Authorization", secret)

	err := webhook.Validate(req)
	if err != nil {
		t.Fatalf("validation failed: %v", err)
	}

	events, err := webhook.Parse(req)
	if err != nil {
		t.Fatalf("parsing failed: %v", err)
	}

	if len(events) == 0 {
		t.Fatal("expected at least one event")
	}
	if events[0].Repository != "mygroup/myimage" {
		t.Errorf("expected repository to be 'mygroup/myimage', got %q", events[0].Repository)
	}
	if events[0].Tag != "v1.0.0" {
		t.Errorf("expected tag to be 'v1.0.0', got %q", events[0].Tag)
	}
	if events[0].RegistryURL != "registry.example.com:5000" {
		t.Errorf("expected registry URL to be 'registry.example.com:5000', got %q", events[0].RegistryURL)
	}
}
