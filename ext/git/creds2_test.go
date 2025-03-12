package git

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGitHubAppCreds_getBaseURL(t *testing.T) {
	g := GitHubAppCreds{
		appID:        123,
		appInstallId: 1234,
	}
	assert.Equal(t, defaultGithubApiUrl, g.getBaseURL())

	g.baseURL = "https://example.com/api"
	assert.Equal(t, g.baseURL, g.getBaseURL())

	g.baseURL = ""
	g.repoURL = "https://github.com/org/repo"
	assert.Equal(t, defaultGithubApiUrl, g.getBaseURL())

	g.repoURL = "http://github.com/org/repo"
	assert.Equal(t, defaultGithubApiUrl, g.getBaseURL())

	g.repoURL = "https://example.com/org/repo"
	assert.Equal(t, "https://example.com/api/v3", g.getBaseURL())

	g.repoURL = "http://example.com/org/repo"
	assert.Equal(t, "http://example.com/api/v3", g.getBaseURL())
}
