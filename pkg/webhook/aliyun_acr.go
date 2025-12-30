package webhook

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd"
)

// AliyunACRWebhook handles Aliyun Container Registry webhook events
type AliyunACRWebhook struct {
	secret string
}

// NewAliyunACRWebhook creates a new Aliyun ACR webhook handler
func NewAliyunACRWebhook(secret string) *AliyunACRWebhook {
	return &AliyunACRWebhook{
		secret: secret,
	}
}

// GetRegistryType returns the registry type this handler supports
func (a *AliyunACRWebhook) GetRegistryType() string {
	return "aliyun-acr"
}

// Validate validates the Aliyun ACR webhook payload
func (a *AliyunACRWebhook) Validate(r *http.Request) error {
	if r.Method != http.MethodPost {
		return fmt.Errorf("invalid HTTP method: %s", r.Method)
	}

	// If secret is configured, validate it from query parameter
	if a.secret != "" {
		secret := r.URL.Query().Get("secret")
		if secret == "" {
			return fmt.Errorf("missing webhook secret")
		}

		if subtle.ConstantTimeCompare([]byte(secret), []byte(a.secret)) != 1 {
			return fmt.Errorf("invalid webhook secret")
		}
	}

	return nil
}

// Parse processes the Aliyun ACR webhook payload and returns a WebhookEvent
func (a *AliyunACRWebhook) Parse(r *http.Request) (*argocd.WebhookEvent, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body: %w", err)
	}

	// Aliyun ACR payload structure for push events. reference: https://www.alibabacloud.com/help/en/acr/user-guide/manage-webhooks
	var payload struct {
		PushData struct {
			Digest   string `json:"digest"`
			PushedAt string `json:"pushed_at"`
			Tag      string `json:"tag"`
		} `json:"push_data"`
		Repository struct {
			DateCreated            string `json:"date_created"`
			Name                   string `json:"name"`
			Namespace              string `json:"namespace"`
			Region                 string `json:"region"`
			RepoAuthenticationType string `json:"repo_authentication_type"`
			RepoFullName           string `json:"repo_full_name"`
			RepoOriginType         string `json:"repo_origin_type"`
			RepoType               string `json:"repo_type"`
		} `json:"repository"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse webhook payload: %w", err)
	}

	// Extract repository name - Aliyun ACR uses namespace/name format
	repository := payload.Repository.RepoFullName
	if repository == "" {
		repository = payload.Repository.Name
		if ns := payload.Repository.Namespace; ns != "" && repository != "" {
			repository = ns + "/" + repository
		}
	}

	if repository == "" {
		return nil, fmt.Errorf("repository name not found in webhook payload")
	}

	if payload.PushData.Tag == "" {
		return nil, fmt.Errorf("tag not found in webhook payload")
	}
	// ACR Enterprise Edition instance name is usually <instance-name>-registry(-vpc).<region>.cr.aliyuncs.com
	// payload does not contain the instance name, we support override registry URL through the query parameters in the URL of the webhook
	registryURL := ""
	if queryRegistry := r.URL.Query().Get("registry_url"); queryRegistry != "" {
		registryURL = queryRegistry

	} else if payload.Repository.Region != "" {
		// Aliyun ACR registry URLs usually follow: registry.<region>.aliyuncs.com
		// Note: VPC URLs usually follow: registry-vpc.<region>.aliyuncs.com, should be overridden by the query parameters in the URL of the webhook
		registryURL = fmt.Sprintf("registry.%s.aliyuncs.com", payload.Repository.Region)
	}

	return &argocd.WebhookEvent{
		RegistryURL: registryURL,
		Repository:  repository,
		Tag:         payload.PushData.Tag,
		Digest:      payload.PushData.Digest,
	}, nil
}
