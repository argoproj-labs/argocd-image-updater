package git

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	defaultGithubApiUrl = "https://api.github.com"
)

func (g GitHubAppCreds) getBaseURL() string {
	if g.baseURL != "" {
		return strings.TrimSuffix(g.baseURL, "/")
	}
	if g.repoURL == "" {
		return defaultGithubApiUrl
	}

	repoUrl, err := url.Parse(g.repoURL)
	if err != nil || repoUrl.Hostname() == "github.com" {
		return defaultGithubApiUrl
	}

	// GitHub Enterprise
	scheme := repoUrl.Scheme
	if scheme == "" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s/api/v3", scheme, repoUrl.Host)
}
