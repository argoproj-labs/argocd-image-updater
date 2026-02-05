package webhook

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewAliyunACRWebhook(t *testing.T) {
	secret := "test-secret"
	webhook := NewAliyunACRWebhook(secret)

	if webhook == nil {
		t.Fatal("expected webhook to be non-nil")
	} else if webhook.secret != secret {
		t.Errorf("expected secret to be %q, got %q", secret, webhook.secret)
	}
}

func TestAliyunACRWebhook_GetRegistryType(t *testing.T) {
	webhook := NewAliyunACRWebhook("")
	registryType := webhook.GetRegistryType()

	expected := "aliyun-acr"
	if registryType != expected {
		t.Errorf("expected registry type to be %q, got %q", expected, registryType)
	}
}

func TestAliyunACRWebhook_Validate(t *testing.T) {
	secret := "test-secret"
	webhook := NewAliyunACRWebhook(secret)

	tests := []struct {
		name        string
		method      string
		secret      string
		noSecret    bool
		expectError bool
	}{
		{
			name:        "valid POST request with correct secret",
			method:      "POST",
			secret:      "test-secret",
			expectError: false,
		},
		{
			name:        "valid POST request without secret",
			method:      "POST",
			noSecret:    true,
			expectError: false,
		},
		{
			name:        "invalid HTTP method",
			method:      "GET",
			secret:      "test-secret",
			expectError: true,
		},
		{
			name:        "missing secret when secret is configured",
			method:      "POST",
			secret:      "",
			expectError: true,
		},
		{
			name:        "invalid secret",
			method:      "POST",
			secret:      "not-the-secret",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			testWebhook := webhook
			if tt.noSecret {
				testWebhook = NewAliyunACRWebhook("")
			}

			req := httptest.NewRequest(tt.method, "/webhook", nil)
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

func TestAliyunACRWebhook_Parse(t *testing.T) {
	tests := []struct {
		name                string
		payload             string
		queryRegistry       string
		expectedRepo        string
		expectedTag         string
		expectedRegistryURL string
		expectError         bool
	}{
		{
			name: "priority: query parameter",
			payload: `{
				"repository": { "repo_full_name": "ns/app", "region": "cn-shanghai" },
				"push_data": { "tag": "v1" }
			}`,
			queryRegistry:       "query.cr.com",
			expectedRepo:        "ns/app",
			expectedTag:         "v1",
			expectedRegistryURL: "query.cr.com",
			expectError:         false,
		},
		{
			name: "priority: region-based",
			payload: `{
				"repository": { "repo_full_name": "ns/app", "region": "cn-beijing" },
				"push_data": { "tag": "v1" }
			}`,
			expectedRepo:        "ns/app",
			expectedTag:         "v1",
			expectedRegistryURL: "registry.cn-beijing.aliyuncs.com",
			expectError:         false,
		},
		{
			name: "full payload from user example",
			payload: `{
				"push_data": {
					"digest": "sha256:457f4aa83fc9a6663ab9d1b0a6e2dce25a12a943ed5bf2c1747c58d48bbb4917", 
					"pushed_at": "2016-11-29 12:25:46", 
					"tag": "latest"
				}, 
				"repository": {
					"date_created": "2016-10-28 21:31:42", 
					"name": "repoTest", 
					"namespace": "namespace", 
					"region": "cn-hangzhou", 
					"repo_authentication_type": "NO_CERTIFIED", 
					"repo_full_name": "namespace/repoTest", 
					"repo_origin_type": "NO_CERTIFIED", 
					"repo_type": "PUBLIC"
				}
			}`,
			expectedRepo:        "namespace/repoTest",
			expectedTag:         "latest",
			expectedRegistryURL: "registry.cn-hangzhou.aliyuncs.com",
			expectError:         false,
		},
		{
			name: "missing region without query parameter",
			payload: `{
				"repository": { "repo_full_name": "ns/app" },
				"push_data": { "tag": "v1" }
			}`,
			expectedRepo:        "ns/app",
			expectedTag:         "v1",
			expectedRegistryURL: "",
			expectError:         false,
		},
		{
			name: "missing tag",
			payload: `{
				"repository": {
					"repo_full_name": "myuser/myapp"
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			webhook := NewAliyunACRWebhook("")
			req := httptest.NewRequest("POST", "/webhook", strings.NewReader(tt.payload))

			if tt.queryRegistry != "" {
				query := req.URL.Query()
				query.Set("registry_url", tt.queryRegistry)
				req.URL.RawQuery = query.Encode()
			}

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
			if event.RegistryURL != tt.expectedRegistryURL {
				t.Errorf("expected registry URL to be %q, got %q", tt.expectedRegistryURL, event.RegistryURL)
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
