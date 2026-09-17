package argocd

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/argoproj-labs/argocd-image-updater/ext/git"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
)

// AzureDevOpsPRService implements PullRequestService for Azure Repos.
type AzureDevOpsPRService struct {
	client *http.Client
	apiURL *url.URL
	token  string
	pr     *PullRequest
}

var _ PullRequestService = (*AzureDevOpsPRService)(nil)

func (a *AzureDevOpsPRService) create(ctx context.Context) error {
	logCtx := log.LoggerFromContext(ctx)

	if a.pr == nil {
		return fmt.Errorf("cannot create PR: pull request metadata is nil")
	}
	description := a.pr.body
	if utf8.RuneCountInString(description) > 4000 {
		description = string([]rune(description)[:4000])
		logCtx.Warnf("Azure DevOps PR body exceeded 4000 characters and was truncated")
	}

	payload := map[string]any{
		"title":         a.pr.title,
		"description":   description,
		"sourceRefName": azureDevOpsBranchRef(a.pr.head),
		"targetRefName": azureDevOpsBranchRef(a.pr.base),
	}
	if len(a.pr.labels) > 0 {
		labels := make([]map[string]string, 0, len(a.pr.labels))
		for _, name := range a.pr.labels {
			labels = append(labels, map[string]string{"name": name})
		}
		payload["labels"] = labels
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("could not encode PR: %w", err)
	}
	var pr struct {
		ID  int    `json:"pullRequestId"`
		URL string `json:"url"`
	}
	if err := a.request(ctx, http.MethodPost, a.apiURL, bytes.NewReader(body), &pr); err != nil {
		if err == ErrPRAlreadyExists {
			logCtx.Infof("PR %q → %q already exists, skipping creation", a.pr.head, a.pr.base)
			return ErrPRAlreadyExists
		}
		return fmt.Errorf("could not create PR %q → %q: %w", a.pr.head, a.pr.base, err)
	}
	logCtx.Infof("created PR #%d %q → %q: %s", pr.ID, a.pr.head, a.pr.base, pr.URL)
	return nil
}

func (a *AzureDevOpsPRService) exists(ctx context.Context, checkOutBranch, pushBranch string) (bool, error) {
	u := *a.apiURL
	query := u.Query()
	query.Set("searchCriteria.sourceRefName", azureDevOpsBranchRef(pushBranch))
	query.Set("searchCriteria.targetRefName", azureDevOpsBranchRef(checkOutBranch))
	query.Set("searchCriteria.status", "active")
	query.Set("$top", "1")
	u.RawQuery = query.Encode()
	var prs struct {
		Value []json.RawMessage `json:"value"`
	}
	if err := a.request(ctx, http.MethodGet, &u, nil, &prs); err != nil {
		return false, fmt.Errorf("could not list Azure DevOps PRs: %w", err)
	}
	return len(prs.Value) > 0, nil
}

// request calls the Azure DevOps REST API using the PAT from the Git credentials.
func (a *AzureDevOpsPRService) request(ctx context.Context, method string, endpoint *url.URL, body io.Reader, result any) error {
	req, err := http.NewRequestWithContext(ctx, method, endpoint.String(), body)
	if err != nil {
		return err
	}
	req.SetBasicAuth("", a.token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		var apiErr struct {
			Message string `json:"message"`
			TypeKey string `json:"typeKey"`
		}
		// Non-JSON error responses still report their HTTP status below.
		_ = json.NewDecoder(resp.Body).Decode(&apiErr)
		if method == http.MethodPost && apiErr.TypeKey == "GitPullRequestExistsException" {
			return ErrPRAlreadyExists
		}
		return fmt.Errorf("Azure DevOps API returned %s: %s", resp.Status, apiErr.Message)
	}
	if err := json.NewDecoder(resp.Body).Decode(result); err != nil {
		return fmt.Errorf("could not decode Azure DevOps response: %w", err)
	}
	return nil
}

// NewAzureDevOpsPRService builds a service using the repository URL and a PAT.
func NewAzureDevOpsPRService(ctx context.Context, wbc *WriteBackConfig, tokenProvider git.SCMTokenProvider) (*AzureDevOpsPRService, error) {
	token, err := tokenProvider.SCMToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("could not obtain SCM token: %w", err)
	}
	if token == "" {
		return nil, fmt.Errorf("empty SCM token: Azure DevOps PR creation requires authentication")
	}
	u, err := url.Parse(wbc.GitRepo)
	if err != nil {
		return nil, fmt.Errorf("could not parse Azure DevOps repo URL: %w", err)
	}
	if (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return nil, fmt.Errorf("Azure DevOps PR creation requires an HTTP(S) repository URL")
	}
	prefix, repo, ok := strings.Cut(strings.TrimSuffix(u.Path, "/"), "/_git/")
	if !ok || repo == "" || strings.Contains(repo, "/") {
		return nil, fmt.Errorf("Azure DevOps repository URL must end with /_git/{repository}")
	}
	// Keep the organization/project or Server collection path from the Git URL.
	u.User = nil
	u.Path = prefix + "/_apis/git/repositories/" + repo + "/pullrequests"
	u.RawPath = ""
	u.RawQuery = "api-version=7.1"
	u.Fragment = ""
	log.LoggerFromContext(ctx).Infof("Azure DevOps PR service initialised for %s/_git/%s", prefix, repo)
	return &AzureDevOpsPRService{
		client: &http.Client{Timeout: 30 * time.Second},
		apiURL: u,
		token:  token,
		pr:     wbc.PullRequest,
	}, nil
}

func azureDevOpsBranchRef(branch string) string {
	return "refs/heads/" + strings.TrimPrefix(branch, "refs/heads/")
}
