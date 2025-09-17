package webhook

import (
    "encoding/json"
    "fmt"
    "io"
    "net/http"
    "net/url"
    "strings"
)

// GitLabWebhook handles GitLab Container Registry webhook events
type GitLabWebhook struct {
    secret string
}

// NewGitLabWebhook creates a new GitLab webhook handler
func NewGitLabWebhook(secret string) *GitLabWebhook {
    return &GitLabWebhook{
        secret: secret,
    }
}

// GetRegistryType returns the registry type this handler supports
func (g *GitLabWebhook) GetRegistryType() string {
    return "gitlab"
}

// Validate validates the GitLab webhook payload
func (g *GitLabWebhook) Validate(r *http.Request) error {
    if r.Method != http.MethodPost {
        return fmt.Errorf("invalid HTTP method: %s", r.Method)
    }

    // GitLab sends an event header; we accept any event for now, focusing on registry pushes
    event := r.Header.Get("X-Gitlab-Event")
    if event == "" {
        return fmt.Errorf("missing X-Gitlab-Event header")
    }

    // If secret is configured, validate token header
    if g.secret != "" {
        token := r.Header.Get("X-Gitlab-Token")
        if token == "" {
            return fmt.Errorf("missing webhook token")
        }
        if token != g.secret {
            return fmt.Errorf("invalid webhook token")
        }
    }

    return nil
}

// Parse processes the GitLab webhook payload and returns a WebhookEvent
func (g *GitLabWebhook) Parse(r *http.Request) (*WebhookEvent, error) {
    body, err := io.ReadAll(r.Body)
    if err != nil {
        return nil, fmt.Errorf("failed to read request body: %w", err)
    }

    // GitLab Container Registry push event payloads can vary slightly.
    // We try to support common fields used by GitLab for registry events.
    var payload map[string]interface{}
    if err := json.Unmarshal(body, &payload); err != nil {
        return nil, fmt.Errorf("failed to parse webhook payload: %w", err)
    }

    // Try to extract repository and tag.
    // Common structure: payload["repository"]["name"] and payload["project"]["path_with_namespace"],
    // or payload under event["target_tag"], event["name"], etc.
    var repository string
    var tag string
    var registryURL string

    // Helper to get nested string
    getString := func(m map[string]interface{}, keys ...string) string {
        cur := m
        for i, k := range keys {
            v, ok := cur[k]
            if !ok {
                return ""
            }
            if i == len(keys)-1 {
                if s, ok := v.(string); ok {
                    return s
                }
                return ""
            }
            mv, ok := v.(map[string]interface{})
            if !ok {
                return ""
            }
            cur = mv
        }
        return ""
    }

    // Prefer repository from project path_with_namespace
    if p := getString(payload, "project", "path_with_namespace"); p != "" {
        repository = p
    }
    // Some payloads carry repository/name
    if repository == "" {
        if rname := getString(payload, "repository", "name"); rname != "" {
            if ns := getString(payload, "repository", "namespace"); ns != "" {
                repository = ns + "/" + rname
            } else {
                repository = rname
            }
        }
    }

    // Tag candidates
    if t := getString(payload, "target_tag"); t != "" {
        tag = t
    }
    if tag == "" {
        if t := getString(payload, "tag"); t != "" {
            tag = t
        }
    }
    // Some payloads list tags under object_attributes or other fields
    if tag == "" {
        if t := getString(payload, "object_attributes", "tag"); t != "" {
            tag = t
        }
    }

    // Registry host: try request Host header first (if GitLab sends its registry),
    // else look for registry info in payload URLs and parse host.
    registryURL = r.Host
    if registryURL == "" {
        if u := getString(payload, "repository", "homepage"); u != "" {
            if h := parseHost(u); h != "" {
                registryURL = h
            }
        }
    }
    if registryURL == "" {
        if u := getString(payload, "homepage"); u != "" {
            if h := parseHost(u); h != "" {
                registryURL = h
            }
        }
    }

    if repository == "" || tag == "" {
        return nil, fmt.Errorf("repository or tag not found in webhook payload")
    }

    // If host still empty, default to "gitlab" (matching handler type); matching will likely use explicit registry in image-list
    if registryURL == "" {
        registryURL = "gitlab"
    }

    return &WebhookEvent{
        RegistryURL: registryURL,
        Repository:  repository,
        Tag:         tag,
    }, nil
}

func parseHost(u string) string {
    if u == "" {
        return ""
    }
    if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
        u = "https://" + u
    }
    if pu, err := url.Parse(u); err == nil {
        return pu.Host
    }
    return ""
}




