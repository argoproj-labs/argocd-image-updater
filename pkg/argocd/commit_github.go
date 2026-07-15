package argocd

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-github/v69/github"

	"github.com/argoproj-labs/argocd-image-updater/ext/git"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
)

// graphQLEndpoint derives the GraphQL API endpoint from the REST API base URL
// carried by the credentials. An empty base URL means github.com. GitHub
// Enterprise serves REST under https://HOST/api/v3 and GraphQL under
// https://HOST/api/graphql.
func graphQLEndpoint(apiBaseURL string) string {
	if apiBaseURL == "" {
		return "https://api.github.com/graphql"
	}
	return strings.TrimSuffix(strings.TrimSuffix(apiBaseURL, "/"), "/v3") + "/graphql"
}

// splitCommitMessage splits a rendered commit message into the headline
// (first line) and body expected by the GraphQL commit message input. An
// empty message gets a generic headline so the mutation does not fail.
func splitCommitMessage(message string) (headline, body string) {
	message = strings.TrimSpace(message)
	if message == "" {
		return "Update container image versions", ""
	}
	parts := strings.SplitN(message, "\n", 2)
	headline = strings.TrimSpace(parts[0])
	if len(parts) == 2 {
		body = strings.TrimSpace(parts[1])
	}
	return headline, body
}

// commitOnBranchInput is the CreateCommitOnBranchInput for GitHub's GraphQL
// createCommitOnBranch mutation.
type commitOnBranchInput struct {
	Branch struct {
		RepositoryNameWithOwner string `json:"repositoryNameWithOwner"`
		BranchName              string `json:"branchName"`
	} `json:"branch"`
	ExpectedHeadOID string `json:"expectedHeadOid"`
	Message         struct {
		Headline string `json:"headline"`
		Body     string `json:"body,omitempty"`
	} `json:"message"`
	// The list fields carry omitempty because a nil slice would marshal as
	// JSON null, which passes GraphQL schema validation but makes GitHub's
	// resolver fail with an opaque "Something went wrong while executing
	// your query" error. GitHub's own examples omit unused lists entirely.
	FileChanges struct {
		Additions []graphQLFileAddition `json:"additions,omitempty"`
		Deletions []graphQLFileDeletion `json:"deletions,omitempty"`
	} `json:"fileChanges"`
}

// graphQLFileAddition is one added or modified file in the mutation's
// fileChanges input.
type graphQLFileAddition struct {
	Path string `json:"path"`
	// Contents is the base64-encoded file content.
	Contents string `json:"contents"`
}

// graphQLFileDeletion is one deleted path in the mutation's fileChanges input.
type graphQLFileDeletion struct {
	Path string `json:"path"`
}

// createCommitOnBranchMutation is the GraphQL mutation used to create a
// GitHub-signed commit on a branch.
const createCommitOnBranchMutation = `mutation($input: CreateCommitOnBranchInput!) { createCommitOnBranch(input: $input) { commit { oid } } }`

// githubGraphQLHTTPClient bounds the GraphQL request even when the caller's
// context carries no deadline; context cancellation still applies first.
var githubGraphQLHTTPClient = &http.Client{Timeout: 30 * time.Second}

// createCommitOnBranchResponse is the GraphQL response envelope for the
// createCommitOnBranch mutation.
type createCommitOnBranchResponse struct {
	Data struct {
		CreateCommitOnBranch struct {
			Commit struct {
				OID string `json:"oid"`
			} `json:"commit"`
		} `json:"createCommitOnBranch"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors"`
}

// createCommitOnBranch executes the createCommitOnBranch mutation and returns
// the OID of the created commit. Commits created this way are constructed
// server-side by GitHub and signed with GitHub's key; with a GitHub App
// installation token they are authored as the App's bot user.
func createCommitOnBranch(ctx context.Context, endpoint, token string, input *commitOnBranchInput) (string, error) {
	payload, err := json.Marshal(map[string]interface{}{
		"query":     createCommitOnBranchMutation,
		"variables": map[string]interface{}{"input": input},
	})
	if err != nil {
		return "", fmt.Errorf("could not marshal createCommitOnBranch request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := githubGraphQLHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("createCommitOnBranch request failed: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("could not read createCommitOnBranch response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("createCommitOnBranch returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var parsed createCommitOnBranchResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("could not parse createCommitOnBranch response: %w", err)
	}
	if len(parsed.Errors) > 0 {
		msgs := make([]string, len(parsed.Errors))
		for i, e := range parsed.Errors {
			msgs[i] = e.Message
		}
		return "", fmt.Errorf("createCommitOnBranch failed: %s", strings.Join(msgs, "; "))
	}
	if parsed.Data.CreateCommitOnBranch.Commit.OID == "" {
		return "", fmt.Errorf("createCommitOnBranch returned no commit OID")
	}
	return parsed.Data.CreateCommitOnBranch.Commit.OID, nil
}

// githubAppCredsProvider returns the credentials as an SCMTokenProvider if
// and only if they are GitHub App credentials — the only credential type
// whose API-created commits are signed by GitHub as the App's bot user.
func githubAppCredsProvider(creds git.Creds) (git.SCMTokenProvider, bool) {
	appCreds, ok := creds.(git.GitHubAppCreds)
	if !ok {
		return nil, false
	}
	return appCreds, true
}

// commitChangesGithubAPI creates the write-back commit through the GitHub
// GraphQL API instead of the local git command line. The working tree
// prepared by commitChangesGit (files already mutated by the change writer)
// is the source of the file contents. If branchCreated is true, pushBranch
// did not exist on the remote and is first created at the base head SHA.
func commitChangesGithubAPI(ctx context.Context, wbc *WriteBackConfig, gitC git.Client, tokenProvider git.SCMTokenProvider, pushBranch string, branchCreated bool) error {
	logCtx := log.LoggerFromContext(ctx)

	changes, err := gitC.WorkingTreeChanges(ctx)
	if err != nil {
		return fmt.Errorf("could not determine changed files: %w", err)
	}
	if len(changes) == 0 {
		return fmt.Errorf("no file changes in working tree to commit via GitHub API for repo '%s'", wbc.GitRepo)
	}

	// The checked-out HEAD is the remote head of pushBranch (or of the base
	// branch the push branch was just created from), so it is both the ref
	// base and the expectedHeadOid guard — the API equivalent of a
	// non-fast-forward push check.
	headOID, err := gitC.CommitSHA(ctx)
	if err != nil {
		return fmt.Errorf("could not determine HEAD commit SHA: %w", err)
	}

	token, err := tokenProvider.SCMToken(ctx)
	if err != nil {
		return fmt.Errorf("could not obtain SCM token: %w", err)
	}
	apiBaseURL := ""
	if p, ok := tokenProvider.(git.SCMAPIBaseURLProvider); ok {
		apiBaseURL = p.SCMAPIBaseURL()
	}

	owner, repoName, err := parseGitHubOwnerRepo(wbc.GitRepo)
	if err != nil {
		return fmt.Errorf("could not parse owner/repo from %q: %w", wbc.GitRepo, err)
	}

	if branchCreated {
		restClient, err := newGithubRESTClient(token, apiBaseURL)
		if err != nil {
			return err
		}
		ref := &github.Reference{
			Ref:    github.Ptr("refs/heads/" + pushBranch),
			Object: &github.GitObject{SHA: github.Ptr(headOID)},
		}
		if _, _, err := restClient.Git.CreateRef(ctx, owner, repoName, ref); err != nil {
			if !isRefAlreadyExistsError(err) {
				return fmt.Errorf("could not create remote branch %q: %w", pushBranch, err)
			}
			logCtx.Debugf("remote branch %q already exists", pushBranch)
		} else {
			logCtx.Debugf("created remote branch %q at %s", pushBranch, headOID)
		}
	}

	input := &commitOnBranchInput{}
	input.Branch.RepositoryNameWithOwner = owner + "/" + repoName
	input.Branch.BranchName = pushBranch
	input.ExpectedHeadOID = headOID
	input.Message.Headline, input.Message.Body = splitCommitMessage(wbc.GitCommitMessage)
	root := gitC.Root()
	for _, c := range changes {
		if c.Deleted {
			input.FileChanges.Deletions = append(input.FileChanges.Deletions, graphQLFileDeletion{Path: c.Path})
			continue
		}
		data, err := os.ReadFile(filepath.Join(root, c.Path))
		if err != nil {
			return fmt.Errorf("could not read changed file %q: %w", c.Path, err)
		}
		input.FileChanges.Additions = append(input.FileChanges.Additions, graphQLFileAddition{
			Path:     c.Path,
			Contents: base64.StdEncoding.EncodeToString(data),
		})
	}

	logCtx.Debugf("committing via GitHub API: commit author/committer and local signing settings are determined by GitHub (App bot user)")
	commitOID, err := createCommitOnBranch(ctx, graphQLEndpoint(apiBaseURL), token, input)
	if err != nil {
		return err
	}
	logCtx.Infof("committed %d file change(s) to branch %q via GitHub API (commit %s)", len(changes), pushBranch, commitOID)
	return nil
}

// isRefAlreadyExistsError reports whether err is GitHub's 422 "Reference
// already exists" response to a ref creation.
func isRefAlreadyExistsError(err error) bool {
	ghErr := unprocessableEntity(err)
	return ghErr != nil && strings.Contains(ghErr.Message, "Reference already exists")
}
