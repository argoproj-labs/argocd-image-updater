package git

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/ssh"

	"github.com/argoproj/argo-cd/v3/common"
)

func TestIsCommitSHA(t *testing.T) {
	assert.True(t, IsCommitSHA("9d921f65f3c5373b682e2eb4b37afba6592e8f8b"))
	assert.True(t, IsCommitSHA("9D921F65F3C5373B682E2EB4B37AFBA6592E8F8B"))
	assert.False(t, IsCommitSHA("gd921f65f3c5373b682e2eb4b37afba6592e8f8b"))
	assert.False(t, IsCommitSHA("master"))
	assert.False(t, IsCommitSHA("HEAD"))
	assert.False(t, IsCommitSHA("9d921f6")) // only consider 40 characters hex strings as a commit-sha
	assert.True(t, IsTruncatedCommitSHA("9d921f6"))
	assert.False(t, IsTruncatedCommitSHA("9d921f")) // we only consider 7+ characters
	assert.False(t, IsTruncatedCommitSHA("branch-name"))
}

func TestEnsurePrefix(t *testing.T) {
	data := [][]string{
		{"world", "hello", "helloworld"},
		{"helloworld", "hello", "helloworld"},
		{"example.com", "https://", "https://example.com"},
		{"https://example.com", "https://", "https://example.com"},
		{"cd", "argo", "argocd"},
		{"argocd", "argo", "argocd"},
		{"", "argocd", "argocd"},
		{"argocd", "", "argocd"},
	}
	for _, table := range data {
		result := ensurePrefix(table[0], table[1])
		assert.Equal(t, table[2], result)
	}
}

func TestIsSSHURL(t *testing.T) {
	data := map[string]bool{
		"git://github.com/argoproj/test.git":     false,
		"git@GITHUB.com:argoproj/test.git":       true,
		"git@github.com:test":                    true,
		"git@github.com:test.git":                true,
		"https://github.com/argoproj/test":       false,
		"https://github.com/argoproj/test.git":   false,
		"ssh://git@GITHUB.com:argoproj/test":     true,
		"ssh://git@GITHUB.com:argoproj/test.git": true,
		"ssh://git@github.com:test.git":          true,
	}
	for k, v := range data {
		isSSH, _ := IsSSHURL(k)
		assert.Equal(t, v, isSSH)
	}
}

func TestIsSSHURLUserName(t *testing.T) {
	isSSH, user := IsSSHURL("ssh://john@john-server.org:29418/project")
	assert.True(t, isSSH)
	assert.Equal(t, "john", user)

	isSSH, user = IsSSHURL("john@john-server.org:29418/project")
	assert.True(t, isSSH)
	assert.Equal(t, "john", user)

	isSSH, user = IsSSHURL("john@doe.org@john-server.org:29418/project")
	assert.True(t, isSSH)
	assert.Equal(t, "john@doe.org", user)

	isSSH, user = IsSSHURL("ssh://john@doe.org@john-server.org:29418/project")
	assert.True(t, isSSH)
	assert.Equal(t, "john@doe.org", user)

	isSSH, user = IsSSHURL("john@doe.org@john-server.org:project")
	assert.True(t, isSSH)
	assert.Equal(t, "john@doe.org", user)

	isSSH, user = IsSSHURL("john@doe.org@john-server.org:29418/project")
	assert.True(t, isSSH)
	assert.Equal(t, "john@doe.org", user)

}

// ---------------------------------------------------------------------------
// NormalizeGitURL
// ---------------------------------------------------------------------------

func TestNormalizeGitURL(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		// SSH scp-style (git@host:org/repo) → colon replaced, ssh:// stripped off
		{
			name:  "SSH scp-style",
			input: "git@github.com:argoproj/argo-cd",
			want:  "git@github.com/argoproj/argo-cd",
		},
		{
			name:  "SSH scp-style with .git suffix",
			input: "git@github.com:argoproj/argo-cd.git",
			want:  "git@github.com/argoproj/argo-cd",
		},
		{
			name:  "SSH scp-style uppercase host",
			input: "git@GITHUB.COM:argoproj/argo-cd.git",
			want:  "git@github.com/argoproj/argo-cd",
		},
		{
			name:  "SSH scp-style with leading whitespace",
			input: "  git@github.com:argoproj/argo-cd.git  ",
			want:  "git@github.com/argoproj/argo-cd",
		},
		// ssh:// scheme stays consistent
		{
			name:  "ssh:// URL",
			input: "ssh://git@github.com/argoproj/argo-cd",
			want:  "git@github.com/argoproj/argo-cd",
		},
		{
			name:  "ssh:// URL with .git suffix",
			input: "ssh://git@github.com/argoproj/argo-cd.git",
			want:  "git@github.com/argoproj/argo-cd",
		},
		// HTTPS URLs
		{
			name:  "HTTPS URL",
			input: "https://github.com/argoproj/argo-cd",
			want:  "https://github.com/argoproj/argo-cd",
		},
		{
			name:  "HTTPS URL with .git suffix",
			input: "https://github.com/argoproj/argo-cd.git",
			want:  "https://github.com/argoproj/argo-cd",
		},
		{
			name:  "HTTPS URL uppercase host",
			input: "https://GITHUB.COM/argoproj/argo-cd.git",
			want:  "https://github.com/argoproj/argo-cd",
		},
		{
			name:  "HTTPS URL with port",
			input: "https://github.com:4443/argoproj/argo-cd.git",
			want:  "https://github.com:4443/argoproj/argo-cd",
		},
		{
			name:  "HTTPS URL with leading whitespace",
			input: "\thttps://github.com/argoproj/argo-cd.git\n",
			want:  "https://github.com/argoproj/argo-cd",
		},
		// Azure DevOps and VisualStudio URLs must NOT have .git stripped
		// (they don't end in .git so TrimSuffix is a no-op, but let's be explicit).
		{
			name:  "Azure DevOps URL",
			input: "https://dev.azure.com/org/proj/_git/repo",
			want:  "https://dev.azure.com/org/proj/_git/repo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeGitURL(tt.input)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// IsHTTPSURL
// ---------------------------------------------------------------------------

func TestIsHTTPSURL(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"https://github.com/org/repo", true},
		{"https://github.com/org/repo.git", true},
		{"https://user:pass@github.com/org/repo", true},
		{"http://github.com/org/repo", false},
		{"git@github.com:org/repo", false},
		{"ssh://git@github.com/org/repo", false},
		{"", false},
		{"ftp://example.com/repo", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, IsHTTPSURL(tt.input))
		})
	}
}

// ---------------------------------------------------------------------------
// IsHTTPURL
// ---------------------------------------------------------------------------

func TestIsHTTPURL(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"http://github.com/org/repo", true},
		{"http://github.com/org/repo.git", true},
		{"http://user:pass@github.com/org/repo", true},
		{"https://github.com/org/repo", false},
		{"git@github.com:org/repo", false},
		{"ssh://git@github.com/org/repo", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.want, IsHTTPURL(tt.input))
		})
	}
}

// ---------------------------------------------------------------------------
// TestRepo
// ---------------------------------------------------------------------------

func TestTestRepo(t *testing.T) {
	ctx := context.Background()

	t.Run("ValidLocalRepo", func(t *testing.T) {
		repoURL, _ := setupLocalRemoteRepo(t)
		err := TestRepo(ctx, repoURL, NopCreds{}, false, false, "")
		assert.NoError(t, err)
	})

	t.Run("InvalidURL", func(t *testing.T) {
		err := TestRepo(ctx, "ssh://bitbucket.org:org/repo", NopCreds{}, false, false, "")
		assert.Error(t, err)
	})

	t.Run("NonExistentRepo", func(t *testing.T) {
		err := TestRepo(ctx, "file:///this/path/does/not/exist", NopCreds{}, false, false, "")
		assert.Error(t, err)
	})
}

func TestSameURL(t *testing.T) {
	data := map[string]string{
		"git@GITHUB.com:argoproj/test":                     "git@github.com:argoproj/test.git",
		"git@GITHUB.com:argoproj/test.git":                 "git@github.com:argoproj/test.git",
		"git@GITHUB.com:test":                              "git@github.com:test.git",
		"git@GITHUB.com:test.git":                          "git@github.com:test.git",
		"https://GITHUB.com/argoproj/test":                 "https://github.com/argoproj/test.git",
		"https://GITHUB.com/argoproj/test.git":             "https://github.com/argoproj/test.git",
		"https://github.com/FOO":                           "https://github.com/foo",
		"https://github.com/TEST":                          "https://github.com/TEST.git",
		"https://github.com/TEST.git":                      "https://github.com/TEST.git",
		"https://github.com:4443/TEST":                     "https://github.com:4443/TEST.git",
		"https://github.com:4443/TEST.git":                 "https://github.com:4443/TEST",
		"ssh://git@GITHUB.com/argoproj/test":               "git@github.com:argoproj/test.git",
		"ssh://git@GITHUB.com/argoproj/test.git":           "git@github.com:argoproj/test.git",
		"ssh://git@GITHUB.com/test.git":                    "git@github.com:test.git",
		"ssh://git@github.com/test":                        "git@github.com:test.git",
		" https://github.com/argoproj/test ":               "https://github.com/argoproj/test.git",
		"\thttps://github.com/argoproj/test\n":             "https://github.com/argoproj/test.git",
		"https://1234.visualstudio.com/myproj/_git/myrepo": "https://1234.visualstudio.com/myproj/_git/myrepo",
		"https://dev.azure.com/1234/myproj/_git/myrepo":    "https://dev.azure.com/1234/myproj/_git/myrepo",
	}
	for k, v := range data {
		assert.True(t, SameURL(k, v))
	}
}

func TestCustomHTTPClient(t *testing.T) {
	ctx := context.Background()

	// Generate client and server certs on the fly — no static files on disk.
	clientCert := generateTestTLSCert(t)
	assert.NotEmpty(t, clientCert.CertPEM)
	assert.NotEmpty(t, clientCert.KeyPEM)

	// Get HTTPSCreds with client cert creds specified, and insecure connection
	creds := NewHTTPSCreds("test", "test", string(clientCert.CertPEM), string(clientCert.KeyPEM), false, "http://proxy:5000", &NoopCredsStore{}, false)
	client := GetRepoHTTPClient(ctx, "https://localhost:9443/foo/bar", false, creds, "http://proxy:5000")
	assert.NotNil(t, client)
	assert.NotNil(t, client.Transport)
	if client.Transport != nil {
		transport := client.Transport.(*http.Transport)
		assert.NotNil(t, transport.TLSClientConfig)
		assert.Equal(t, true, transport.DisableKeepAlives)
		assert.Equal(t, false, transport.TLSClientConfig.InsecureSkipVerify)
		assert.NotNil(t, transport.TLSClientConfig.GetClientCertificate)
		assert.Nil(t, transport.TLSClientConfig.RootCAs)
		if transport.TLSClientConfig.GetClientCertificate != nil {
			cert, err := transport.TLSClientConfig.GetClientCertificate(nil)
			assert.NoError(t, err)
			if err == nil {
				assert.NotNil(t, cert)
				assert.NotEqual(t, 0, len(cert.Certificate))
				assert.NotNil(t, cert.PrivateKey)
			}
		}
		proxy, err := transport.Proxy(nil)
		assert.Nil(t, err)
		assert.Equal(t, "http://proxy:5000", proxy.String())
	}

	t.Setenv("http_proxy", "http://proxy-from-env:7878")

	// Get HTTPSCreds without client cert creds, but insecure connection
	creds = NewHTTPSCreds("test", "test", "", "", true, "", &NoopCredsStore{}, false)
	client = GetRepoHTTPClient(ctx, "https://localhost:9443/foo/bar", true, creds, "")
	assert.NotNil(t, client)
	assert.NotNil(t, client.Transport)
	if client.Transport != nil {
		transport := client.Transport.(*http.Transport)
		assert.NotNil(t, transport.TLSClientConfig)
		assert.Equal(t, true, transport.DisableKeepAlives)
		assert.Equal(t, true, transport.TLSClientConfig.InsecureSkipVerify)
		assert.NotNil(t, transport.TLSClientConfig.GetClientCertificate)
		assert.Nil(t, transport.TLSClientConfig.RootCAs)
		if transport.TLSClientConfig.GetClientCertificate != nil {
			cert, err := transport.TLSClientConfig.GetClientCertificate(nil)
			assert.NoError(t, err)
			if err == nil {
				assert.NotNil(t, cert)
				assert.Equal(t, 0, len(cert.Certificate))
				assert.Nil(t, cert.PrivateKey)
			}
		}
		req, err := http.NewRequest(http.MethodGet, "http://proxy-from-env:7878", nil)
		assert.Nil(t, err)
		proxy, err := transport.Proxy(req)
		assert.Nil(t, err)
		assert.Equal(t, "http://proxy-from-env:7878", proxy.String())
	}
	// GetRepoHTTPClient with root ca — use a freshly generated server cert.
	serverCert := generateTestTLSCert(t)
	temppath := t.TempDir()
	defer os.RemoveAll(temppath)
	err := os.WriteFile(filepath.Join(temppath, "127.0.0.1"), serverCert.CertPEM, 0600)
	assert.NoError(t, err)
	t.Setenv(common.EnvVarTLSDataPath, temppath)
	client = GetRepoHTTPClient(ctx, "https://127.0.0.1", false, creds, "")
	assert.NotNil(t, client)
	assert.NotNil(t, client.Transport)
	if client.Transport != nil {
		transport := client.Transport.(*http.Transport)
		assert.NotNil(t, transport.TLSClientConfig)
		assert.Equal(t, true, transport.DisableKeepAlives)
		assert.Equal(t, false, transport.TLSClientConfig.InsecureSkipVerify)
		assert.NotNil(t, transport.TLSClientConfig.RootCAs)
	}
}

func TestLsRemote(t *testing.T) {
	ctx := context.Background()

	repoURL, fullSHA := setupLocalRemoteRepo(t)
	clnt, err := NewClientExt(repoURL, t.TempDir(), NopCreds{}, false, false, "")
	assert.NoError(t, err)

	xpass := []string{
		"HEAD",
		"master",
		"release-0.8",
		"v0.8.0",
		fullSHA, // exact 40-char SHA must resolve to itself
	}
	for _, revision := range xpass {
		commitSHA, err := clnt.LsRemote(ctx, revision)
		assert.NoError(t, err)
		assert.True(t, IsCommitSHA(commitSHA))
	}

	// Truncated SHA (7 chars) is returned as-is, not fully resolved.
	truncated := fullSHA[:7]
	commitSHA, err := clnt.LsRemote(ctx, truncated)
	assert.NoError(t, err)
	assert.False(t, IsCommitSHA(commitSHA))
	assert.True(t, IsTruncatedCommitSHA(commitSHA))

	xfail := []string{
		"unresolvable",
		fullSHA[:6], // too short (6 characters)
	}
	for _, revision := range xfail {
		_, err := clnt.LsRemote(ctx, revision)
		assert.Error(t, err)
	}
}

// Running this test requires git-lfs to be installed on your machine.
func TestLFSClient(t *testing.T) {

	// temporary disable LFS test
	// TODO(alexmt): dockerize tests in and enabled it
	t.Skip()

	tempDir := t.TempDir()

	client, err := NewClientExt("https://github.com/argoproj-labs/argocd-testrepo-lfs", tempDir, NopCreds{}, false, true, "")
	assert.NoError(t, err)

	ctx := context.Background()
	commitSHA, err := client.LsRemote(ctx, "HEAD")
	assert.NoError(t, err)
	assert.NotEqual(t, "", commitSHA)

	err = client.Init(ctx)
	assert.NoError(t, err)

	err = client.Fetch(ctx, "")
	assert.NoError(t, err)

	err = client.Checkout(ctx, commitSHA, true)
	assert.NoError(t, err)

	largeFiles, err := client.LsLargeFiles(ctx)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(largeFiles))

	fileHandle, err := os.Open(fmt.Sprintf("%s/test3.yaml", tempDir))
	assert.NoError(t, err)
	if err == nil {
		defer func() {
			if err = fileHandle.Close(); err != nil {
				assert.NoError(t, err)
			}
		}()
		text, err := io.ReadAll(fileHandle)
		assert.NoError(t, err)
		if err == nil {
			assert.Equal(t, "This is not a YAML, sorry.\n", string(text))
		}
	}
}

func TestVerifyCommitSignature(t *testing.T) {
	addGitVerifyWrapperToPath(t)

	ctx := context.Background()

	repoPath, signedSHA, unsignedSHA := setupRepoWithSignedCommit(t)
	client, err := NewClientExt("", repoPath, NopCreds{}, false, false, "")
	assert.NoError(t, err)

	// SSH-signed commit must produce non-empty verification output.
	{
		out, err := client.VerifyCommitSignature(ctx, signedSHA)
		assert.NoError(t, err)
		assert.NotEmpty(t, out)
	}

	// Unsigned commit must produce empty output.
	{
		out, err := client.VerifyCommitSignature(ctx, unsignedSHA)
		assert.NoError(t, err)
		assert.Empty(t, out)
	}
}

func TestVerifyShallowFetchCheckout(t *testing.T) {
	ctx := context.Background()

	repoURL, _ := setupLocalRemoteRepo(t)
	p := t.TempDir()

	client, err := NewClientExt(repoURL, p, NopCreds{}, false, false, "")
	assert.NoError(t, err)

	err = client.Init(ctx)
	assert.NoError(t, err)

	err = client.ShallowFetch(ctx, "HEAD", 1)
	assert.NoError(t, err)

	commitSHA, err := client.LsRemote(ctx, "HEAD")
	assert.NoError(t, err)

	err = client.Checkout(ctx, commitSHA, true)
	assert.NoError(t, err)
}

func TestNewFactory(t *testing.T) {
	ctx := context.Background()

	// Build a local bare repository with two commits:
	//   - first commit carries a tag (so HEAD has no tags → Tags len == 0)
	//   - second commit is untagged and becomes HEAD
	workDir := t.TempDir()
	bareDir := t.TempDir()

	require.NoError(t, runCmd(workDir, "git", "init", "-b", "master"))
	require.NoError(t, runCmd(workDir, "git", "config", "user.email", "test@example.com"))
	require.NoError(t, runCmd(workDir, "git", "config", "user.name", "Test User"))
	require.NoError(t, runCmd(workDir, "git", "commit", "--allow-empty", "-m", "first commit"))
	require.NoError(t, runCmd(workDir, "git", "tag", "v1.0.0"))
	require.NoError(t, runCmd(workDir, "git", "commit", "--allow-empty", "-m", "second commit"))

	require.NoError(t, runCmd(bareDir, "git", "init", "--bare"))
	require.NoError(t, runCmd(workDir, "git", "remote", "add", "origin", bareDir))
	require.NoError(t, runCmd(workDir, "git", "push", "origin", "master", "--tags"))

	repoURL := "file://" + bareDir
	dirName := t.TempDir()

	client, err := NewClientExt(repoURL, dirName, NopCreds{}, false, false, "")
	assert.NoError(t, err)

	commitSHA, err := client.LsRemote(ctx, "HEAD")
	assert.NoError(t, err)

	err = client.Init(ctx)
	assert.NoError(t, err)

	err = client.Fetch(ctx, "")
	assert.NoError(t, err)

	// Second fetch must treat "already up-to-date" as success.
	err = client.Fetch(ctx, "")
	assert.NoError(t, err)

	err = client.Checkout(ctx, commitSHA, true)
	assert.NoError(t, err)

	revisionMetadata, err := client.RevisionMetadata(ctx, commitSHA)
	assert.NoError(t, err)
	assert.NotNil(t, revisionMetadata)
	assert.Regexp(t, "^.*<.*>$", revisionMetadata.Author)
	assert.Len(t, revisionMetadata.Tags, 0)
	assert.NotEmpty(t, revisionMetadata.Date)
	assert.NotEmpty(t, revisionMetadata.Message)

	commitSHA2, err := client.CommitSHA(ctx)
	assert.NoError(t, err)
	assert.Equal(t, commitSHA, commitSHA2)
}

func TestListRevisions(t *testing.T) {
	ctx := context.Background()

	repoURL, _ := setupLocalRemoteRepo(t)
	client, err := NewClientExt(repoURL, t.TempDir(), NopCreds{}, false, false, "")
	assert.NoError(t, err)

	lsResult, err := client.LsRefs(ctx)
	assert.NoError(t, err)

	assert.Contains(t, lsResult.Branches, "master")
	assert.Contains(t, lsResult.Branches, "release-0.8")
	assert.Contains(t, lsResult.Tags, "v0.8.0")
	assert.NotContains(t, lsResult.Branches, "v0.8.0")
	assert.NotContains(t, lsResult.Tags, "master")
}

func TestLsFiles(t *testing.T) {
	ctx := context.Background()

	// Resolve symlinks so that paths are consistent with what filepath.Abs
	// returns inside LsFiles after os.Chdir (on macOS /var → /private/var).
	tmpDir1, err := filepath.EvalSymlinks(t.TempDir())
	assert.NoError(t, err)
	tmpDir2, err := filepath.EvalSymlinks(t.TempDir())
	assert.NoError(t, err)

	client, err := NewClientExt("", tmpDir1, NopCreds{}, false, false, "")
	assert.NoError(t, err)

	err = runCmd(tmpDir1, "git", "init")
	assert.NoError(t, err)

	// Prepare files
	a, err := os.Create(filepath.Join(tmpDir1, "a.yaml"))
	assert.NoError(t, err)
	a.Close()
	err = os.MkdirAll(filepath.Join(tmpDir1, "subdir"), 0755)
	assert.NoError(t, err)
	b, err := os.Create(filepath.Join(tmpDir1, "subdir", "b.yaml"))
	assert.NoError(t, err)
	b.Close()
	err = os.MkdirAll(filepath.Join(tmpDir2, "subdir"), 0755)
	assert.NoError(t, err)
	c, err := os.Create(filepath.Join(tmpDir2, "c.yaml"))
	assert.NoError(t, err)
	c.Close()
	err = os.Symlink(filepath.Join(tmpDir2, "c.yaml"), filepath.Join(tmpDir1, "link.yaml"))
	assert.NoError(t, err)

	err = runCmd(tmpDir1, "git", "add", ".")
	assert.NoError(t, err)
	err = runCmd(tmpDir1, "git", "commit", "-m", "Initial commit")
	assert.NoError(t, err)

	// Old and default globbing
	expectedResult := []string{"a.yaml", "link.yaml", "subdir/b.yaml"}
	lsResult, err := client.LsFiles(ctx, "*.yaml", false)
	assert.NoError(t, err)
	assert.Equal(t, lsResult, expectedResult)

	// New and safer globbing, do not return symlinks resolving outside of the repo
	expectedResult = []string{"a.yaml"}
	lsResult, err = client.LsFiles(ctx, "*.yaml", true)
	assert.NoError(t, err)
	assert.Equal(t, lsResult, expectedResult)

	// New globbing, do not return files outside of the repo
	var nilResult []string
	lsResult, err = client.LsFiles(ctx, filepath.Join(tmpDir2, "*.yaml"), true)
	assert.NoError(t, err)
	assert.Equal(t, lsResult, nilResult)
}

// tlsTestCert holds PEM-encoded certificate and private key for use in tests.
type tlsTestCert struct {
	CertPEM []byte
	KeyPEM  []byte
}

// generateTestTLSCert generates an in-memory self-signed ECDSA certificate
// valid for localhost / 127.0.0.1. Both client-auth and server-auth extended
// key usages are set so the same helper covers both roles in tests.
func generateTestTLSCert(t *testing.T) tlsTestCert {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"ArgoCD Image Updater Test"},
		},
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	require.NoError(t, err)

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	keyDER, err := x509.MarshalECPrivateKey(key)
	require.NoError(t, err)
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return tlsTestCert{CertPEM: certPEM, KeyPEM: keyPEM}
}

// gitVerifyWrapperScript is the content of git-verify-wrapper.sh from the
// upstream ArgoCD project (hack/git-verify-wrapper.sh). It is reproduced here
// so that tests in this package can run without the script being pre-installed
// on the system PATH.
const gitVerifyWrapperScript = `#!/bin/sh
# Wrapper script to perform GPG signature validation on git commit SHAs and
# annotated tags.
#
# We capture stderr to stdout, so we can have the output in the logs. Also,
# we ignore error codes that are emitted if signature verification failed.
#
if test "$1" = ""; then
	echo "Wrong usage of git-verify-wrapper.sh" >&2
	exit 1
fi

REVISION="$1"
TYPE=

# Figure out we have an annotated tag or a commit SHA
if git describe --exact-match "${REVISION}" >/dev/null 2>&1; then
	IFS=''
	TYPE=tag
	OUTPUT=$(git verify-tag "$REVISION" 2>&1)
	RET=$?
else
	IFS=''
	TYPE=commit
	OUTPUT=$(git verify-commit "$REVISION" 2>&1)
	RET=$?
fi

case "$RET" in
0)
	echo "$OUTPUT"
	;;
1)
	# git verify-tag emits error messages if no signature is found on tag,
	# which we don't want in the output.
	if test "$TYPE" = "tag" -a "${OUTPUT%%:*}" = "error"; then
		OUTPUT=""
	fi
	echo "$OUTPUT"
	RET=0
	;;
*)
	echo "$OUTPUT" >&2
	;;
esac
exit $RET
`

// addGitVerifyWrapperToPath writes git-verify-wrapper.sh into a temporary
// directory and prepends that directory to PATH for the duration of t.
// The PATH is restored automatically when t completes via t.Setenv.
func addGitVerifyWrapperToPath(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "git-verify-wrapper.sh")

	require.NoError(t, os.WriteFile(scriptPath, []byte(gitVerifyWrapperScript), 0755))

	originalPath := os.Getenv("PATH")
	t.Setenv("PATH", fmt.Sprintf("%s:%s", dir, originalPath))
}

// runCmdOut runs a command in workingDir and returns its trimmed stdout.
func runCmdOut(workingDir, name string, args ...string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = workingDir
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}

// setupLocalRemoteRepo creates a local bare git repository that acts as a
// remote, containing:
//   - a "master" branch with one empty commit
//   - a "release-0.8" branch pointing to the same commit
//   - a "v0.8.0" lightweight tag pointing to the same commit
//
// Returns a file:// URL for the bare repo and the full 40-char commit SHA.
func setupLocalRemoteRepo(t *testing.T) (repoURL, commitSHA string) {
	t.Helper()

	workDir := t.TempDir()
	bareDir := t.TempDir()

	require.NoError(t, runCmd(workDir, "git", "init", "-b", "master"))
	require.NoError(t, runCmd(workDir, "git", "config", "user.email", "test@example.com"))
	require.NoError(t, runCmd(workDir, "git", "config", "user.name", "Test"))
	require.NoError(t, runCmd(workDir, "git", "commit", "--allow-empty", "-m", "initial"))

	var err error
	commitSHA, err = runCmdOut(workDir, "git", "rev-parse", "HEAD")
	require.NoError(t, err)

	require.NoError(t, runCmd(workDir, "git", "checkout", "-b", "release-0.8"))
	require.NoError(t, runCmd(workDir, "git", "checkout", "master"))
	require.NoError(t, runCmd(workDir, "git", "tag", "v0.8.0"))

	require.NoError(t, runCmd(bareDir, "git", "init", "--bare"))
	require.NoError(t, runCmd(workDir, "git", "remote", "add", "origin", bareDir))
	require.NoError(t, runCmd(workDir, "git", "push", "origin", "master", "release-0.8"))
	require.NoError(t, runCmd(workDir, "git", "push", "origin", "--tags"))

	return "file://" + bareDir, commitSHA
}

// setupRepoWithSignedCommit creates a local git repository containing:
//   - one SSH-signed commit (using an Ed25519 key generated in-memory)
//   - one unsigned commit
//
// Git's local config is used for all signing/verification settings so the
// test works even when HOME=/dev/null (as set by nativeGitClient.runCmdOutput).
// Returns the repo path, signed commit SHA, and unsigned commit SHA.
func setupRepoWithSignedCommit(t *testing.T) (repoPath, signedSHA, unsignedSHA string) {
	t.Helper()

	// Generate an Ed25519 key pair entirely in Go – no external keygen tool.
	pubKeyRaw, privKeyRaw, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	sshPubKey, err := ssh.NewPublicKey(pubKeyRaw)
	require.NoError(t, err)

	privPEM, err := ssh.MarshalPrivateKey(privKeyRaw, "")
	require.NoError(t, err)

	keyDir := t.TempDir()
	privKeyFile := filepath.Join(keyDir, "id_ed25519")
	require.NoError(t, os.WriteFile(privKeyFile, pem.EncodeToMemory(privPEM), 0600))

	// allowed_signers format required by ssh-keygen -Y verify and git verify-commit.
	pubKeyLine := strings.TrimSuffix(string(ssh.MarshalAuthorizedKey(sshPubKey)), "\n")
	allowedSigners := fmt.Sprintf("test@example.com namespaces=\"git\" %s\n", pubKeyLine)
	allowedSignersFile := filepath.Join(keyDir, "allowed_signers")
	require.NoError(t, os.WriteFile(allowedSignersFile, []byte(allowedSigners), 0644))

	// Set up the repo. All signing config goes into local (.git/config) so it
	// is respected even when HOME=/dev/null is set by nativeGitClient.
	repoPath = t.TempDir()
	require.NoError(t, runCmd(repoPath, "git", "init", "-b", "master"))
	require.NoError(t, runCmd(repoPath, "git", "config", "user.email", "test@example.com"))
	require.NoError(t, runCmd(repoPath, "git", "config", "user.name", "Test"))
	require.NoError(t, runCmd(repoPath, "git", "config", "gpg.format", "ssh"))
	require.NoError(t, runCmd(repoPath, "git", "config", "user.signingkey", privKeyFile))
	require.NoError(t, runCmd(repoPath, "git", "config", "gpg.ssh.allowedSignersFile", allowedSignersFile))

	require.NoError(t, runCmd(repoPath, "git", "commit", "--allow-empty", "-S", "-m", "signed commit"))
	signedSHA, err = runCmdOut(repoPath, "git", "rev-parse", "HEAD")
	require.NoError(t, err)

	require.NoError(t, runCmd(repoPath, "git", "commit", "--allow-empty", "-m", "unsigned commit"))
	unsignedSHA, err = runCmdOut(repoPath, "git", "rev-parse", "HEAD")
	require.NoError(t, err)

	return repoPath, signedSHA, unsignedSHA
}
