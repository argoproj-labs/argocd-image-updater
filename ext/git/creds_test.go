package git

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/kballard/go-shellquote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/argoproj/argo-cd/v3/util/cert"
	"github.com/argoproj/argo-cd/v3/util/io"
)

type cred struct {
	username string
	password string
}

type memoryCredsStore struct {
	creds map[string]cred
}

func (s *memoryCredsStore) Add(username string, password string) string {
	id := uuid.New().String()
	s.creds[id] = cred{
		username: username,
		password: password,
	}
	return id
}

func (s *memoryCredsStore) Remove(id string) {
	delete(s.creds, id)
}

func TestHTTPSCreds_Environ_no_cert_cleanup(t *testing.T) {
	ctx := context.Background()

	store := &memoryCredsStore{creds: make(map[string]cred)}
	creds := NewHTTPSCreds(ctx, "", "", "", "", true, "", store, false)
	closer, env, err := creds.Environ(ctx)
	require.NoError(t, err)
	var nonce string
	for _, envVar := range env {
		if strings.HasPrefix(envVar, ASKPASS_NONCE_ENV) {
			nonce = envVar[len(ASKPASS_NONCE_ENV)+1:]
			break
		}
	}
	assert.Contains(t, store.creds, nonce)
	io.Close(closer)
	assert.NotContains(t, store.creds, nonce)
}

func TestHTTPSCreds_Environ_insecure_true(t *testing.T) {
	ctx := context.Background()

	creds := NewHTTPSCreds(ctx, "", "", "", "", true, "", &NoopCredsStore{}, false)
	closer, env, err := creds.Environ(ctx)
	t.Cleanup(func() {
		io.Close(closer)
	})
	require.NoError(t, err)
	found := slices.Contains(env, "GIT_SSL_NO_VERIFY=true")
	assert.True(t, found)
}

func TestHTTPSCreds_Environ_insecure_false(t *testing.T) {
	ctx := context.Background()
	creds := NewHTTPSCreds(ctx, "", "", "", "", false, "", &NoopCredsStore{}, false)
	closer, env, err := creds.Environ(ctx)
	t.Cleanup(func() {
		io.Close(closer)
	})
	require.NoError(t, err)
	found := slices.Contains(env, "GIT_SSL_NO_VERIFY=true")
	assert.False(t, found)
}

func TestHTTPSCreds_Environ_forceBasicAuth(t *testing.T) {
	t.Run("Enabled and credentials set", func(t *testing.T) {
		ctx := context.Background()
		store := &memoryCredsStore{creds: make(map[string]cred)}
		creds := NewHTTPSCreds(ctx, "username", "password", "", "", false, "", store, true)
		closer, env, err := creds.Environ(ctx)
		require.NoError(t, err)
		defer closer.Close()
		var header string
		for _, envVar := range env {
			if strings.HasPrefix(envVar, fmt.Sprintf("%s=", forceBasicAuthHeaderEnv)) {
				header = envVar[len(forceBasicAuthHeaderEnv)+1:]
			}
			if header != "" {
				break
			}
		}
		b64enc := base64.StdEncoding.EncodeToString([]byte("username:password"))
		assert.Equal(t, "Authorization: Basic "+b64enc, header)
	})
	t.Run("Enabled but credentials not set", func(t *testing.T) {
		ctx := context.Background()
		store := &memoryCredsStore{creds: make(map[string]cred)}
		creds := NewHTTPSCreds(ctx, "", "", "", "", false, "", store, true)
		closer, env, err := creds.Environ(ctx)
		require.NoError(t, err)
		defer closer.Close()
		var header string
		for _, envVar := range env {
			if strings.HasPrefix(envVar, fmt.Sprintf("%s=", forceBasicAuthHeaderEnv)) {
				header = envVar[len(forceBasicAuthHeaderEnv)+1:]
			}
			if header != "" {
				break
			}
		}
		assert.Empty(t, header)
	})
	t.Run("Disabled with credentials set", func(t *testing.T) {
		ctx := context.Background()
		store := &memoryCredsStore{creds: make(map[string]cred)}
		creds := NewHTTPSCreds(ctx, "username", "password", "", "", false, "", store, false)
		closer, env, err := creds.Environ(ctx)
		require.NoError(t, err)
		defer closer.Close()
		var header string
		for _, envVar := range env {
			if strings.HasPrefix(envVar, fmt.Sprintf("%s=", forceBasicAuthHeaderEnv)) {
				header = envVar[len(forceBasicAuthHeaderEnv)+1:]
			}
			if header != "" {
				break
			}
		}
		assert.Empty(t, header)
	})

	t.Run("Disabled with credentials not set", func(t *testing.T) {
		ctx := context.Background()
		store := &memoryCredsStore{creds: make(map[string]cred)}
		creds := NewHTTPSCreds(ctx, "", "", "", "", false, "", store, false)
		closer, env, err := creds.Environ(ctx)
		require.NoError(t, err)
		defer closer.Close()
		var header string
		for _, envVar := range env {
			if strings.HasPrefix(envVar, fmt.Sprintf("%s=", forceBasicAuthHeaderEnv)) {
				header = envVar[len(forceBasicAuthHeaderEnv)+1:]
			}
			if header != "" {
				break
			}
		}
		assert.Empty(t, header)
	})
}

func TestHTTPSCreds_Environ_clientCert(t *testing.T) {
	ctx := context.Background()

	store := &memoryCredsStore{creds: make(map[string]cred)}
	creds := NewHTTPSCreds(ctx, "", "", "clientCertData", "clientCertKey", false, "", store, false)
	closer, env, err := creds.Environ(ctx)
	require.NoError(t, err)
	var cert, key string
	for _, envVar := range env {
		if strings.HasPrefix(envVar, "GIT_SSL_CERT=") {
			cert = envVar[13:]
		} else if strings.HasPrefix(envVar, "GIT_SSL_KEY=") {
			key = envVar[12:]
		}
		if cert != "" && key != "" {
			break
		}
	}
	assert.NotEmpty(t, cert)
	assert.NotEmpty(t, key)

	certBytes, err := os.ReadFile(cert)
	assert.NoError(t, err)
	assert.Equal(t, "clientCertData", string(certBytes))
	keyBytes, err := os.ReadFile(key)
	assert.Equal(t, "clientCertKey", string(keyBytes))
	assert.NoError(t, err)

	io.Close(closer)

	_, err = os.Stat(cert)
	assert.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(key)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

func Test_SSHCreds_Environ(t *testing.T) {
	for _, insecureIgnoreHostKey := range []bool{false, true} {
		ctx := context.Background()

		tempDir := t.TempDir()
		caFile := path.Join(tempDir, "caFile")
		err := os.WriteFile(caFile, []byte(""), os.FileMode(0600))
		require.NoError(t, err)
		creds := NewSSHCreds(ctx, "sshPrivateKey", caFile, insecureIgnoreHostKey, &NoopCredsStore{}, "")
		closer, env, err := creds.Environ(ctx)
		require.NoError(t, err)
		require.Len(t, env, 2)

		assert.Equal(t, fmt.Sprintf("GIT_SSL_CAINFO=%s/caFile", tempDir), env[0], "CAINFO env var must be set")

		assert.True(t, strings.HasPrefix(env[1], "GIT_SSH_COMMAND="))

		if insecureIgnoreHostKey {
			assert.Contains(t, env[1], "-o StrictHostKeyChecking=no")
			assert.Contains(t, env[1], "-o UserKnownHostsFile=/dev/null")
		} else {
			assert.Contains(t, env[1], "-o StrictHostKeyChecking=yes")
			hostsPath := cert.GetSSHKnownHostsDataPath()
			assert.Contains(t, env[1], fmt.Sprintf("-o UserKnownHostsFile=%s", hostsPath))
		}

		envRegex := regexp.MustCompile("-i ([^ ]+)")
		assert.Regexp(t, envRegex, env[1])
		privateKeyFile := envRegex.FindStringSubmatch(env[1])[1]
		assert.FileExists(t, privateKeyFile)
		io.Close(closer)
		assert.NoFileExists(t, privateKeyFile)
	}
}

func Test_SSHCreds_Environ_WithProxy(t *testing.T) {
	for _, insecureIgnoreHostKey := range []bool{false, true} {
		ctx := context.Background()

		tempDir := t.TempDir()
		caFile := path.Join(tempDir, "caFile")
		err := os.WriteFile(caFile, []byte(""), os.FileMode(0600))
		require.NoError(t, err)
		creds := NewSSHCreds(ctx, "sshPrivateKey", caFile, insecureIgnoreHostKey, &NoopCredsStore{}, "socks5://127.0.0.1:1080")
		closer, env, err := creds.Environ(ctx)
		require.NoError(t, err)
		require.Len(t, env, 2)

		assert.Equal(t, fmt.Sprintf("GIT_SSL_CAINFO=%s/caFile", tempDir), env[0], "CAINFO env var must be set")

		assert.True(t, strings.HasPrefix(env[1], "GIT_SSH_COMMAND="))

		if insecureIgnoreHostKey {
			assert.Contains(t, env[1], "-o StrictHostKeyChecking=no")
			assert.Contains(t, env[1], "-o UserKnownHostsFile=/dev/null")
		} else {
			assert.Contains(t, env[1], "-o StrictHostKeyChecking=yes")
			hostsPath := cert.GetSSHKnownHostsDataPath()
			assert.Contains(t, env[1], fmt.Sprintf("-o UserKnownHostsFile=%s", hostsPath))
		}
		proxyScript := proxyScriptFromSSHCommand(t, env[1])
		assert.FileExists(t, proxyScript)

		scriptBytes, err := os.ReadFile(proxyScript)
		require.NoError(t, err)
		assert.Contains(t, string(scriptBytes), "127.0.0.1:1080")
		assert.Contains(t, string(scriptBytes), "connect-proxy")

		envRegex := regexp.MustCompile("-i ([^ ]+)")
		assert.Regexp(t, envRegex, env[1])
		privateKeyFile := envRegex.FindStringSubmatch(env[1])[1]
		assert.FileExists(t, privateKeyFile)
		io.Close(closer)
		assert.NoFileExists(t, privateKeyFile)
		assert.NoFileExists(t, proxyScript)
	}
}

func Test_SSHCreds_Environ_WithProxyUserNamePassword(t *testing.T) {
	for _, insecureIgnoreHostKey := range []bool{false, true} {
		ctx := context.Background()

		tempDir := t.TempDir()
		caFile := path.Join(tempDir, "caFile")
		err := os.WriteFile(caFile, []byte(""), os.FileMode(0600))
		require.NoError(t, err)
		creds := NewSSHCreds(ctx, "sshPrivateKey", caFile, insecureIgnoreHostKey, &NoopCredsStore{}, "socks5://user:password@127.0.0.1:1080")
		closer, env, err := creds.Environ(ctx)
		require.NoError(t, err)
		require.Len(t, env, 4)

		assert.Equal(t, fmt.Sprintf("GIT_SSL_CAINFO=%s/caFile", tempDir), env[0], "CAINFO env var must be set")

		assert.True(t, strings.HasPrefix(env[1], "GIT_SSH_COMMAND="))
		assert.Equal(t, "SOCKS5_USER=user", env[2], "SOCKS5 user env var must be set")
		assert.Equal(t, "SOCKS5_PASSWD=password", env[3], "SOCKS5 password env var must be set")

		if insecureIgnoreHostKey {
			assert.Contains(t, env[1], "-o StrictHostKeyChecking=no")
			assert.Contains(t, env[1], "-o UserKnownHostsFile=/dev/null")
		} else {
			assert.Contains(t, env[1], "-o StrictHostKeyChecking=yes")
			hostsPath := cert.GetSSHKnownHostsDataPath()
			assert.Contains(t, env[1], fmt.Sprintf("-o UserKnownHostsFile=%s", hostsPath))
		}
		proxyScript := proxyScriptFromSSHCommand(t, env[1])
		assert.FileExists(t, proxyScript)

		scriptBytes, err := os.ReadFile(proxyScript)
		require.NoError(t, err)
		assert.Contains(t, string(scriptBytes), "127.0.0.1:1080")

		envRegex := regexp.MustCompile("-i ([^ ]+)")
		assert.Regexp(t, envRegex, env[1])
		privateKeyFile := envRegex.FindStringSubmatch(env[1])[1]
		assert.FileExists(t, privateKeyFile)
		io.Close(closer)
		assert.NoFileExists(t, privateKeyFile)
		assert.NoFileExists(t, proxyScript)
	}
}

func proxyScriptFromSSHCommand(t *testing.T, gitSSHCommandEnv string) string {
	t.Helper()
	args, err := shellquote.Split(strings.TrimPrefix(gitSSHCommandEnv, "GIT_SSH_COMMAND="))
	require.NoError(t, err)
	for _, arg := range args {
		proxyCommand, ok := strings.CutPrefix(arg, "ProxyCommand=")
		if !ok {
			continue
		}
		fields, err := shellquote.Split(proxyCommand)
		require.NoError(t, err)
		require.Len(t, fields, 4, "ProxyCommand must be a single argument")
		assert.Equal(t, "/bin/sh", fields[0])
		assert.Equal(t, []string{"%h", "%p"}, fields[2:])
		return fields[1]
	}
	require.FailNow(t, "GIT_SSH_COMMAND has no ProxyCommand", gitSSHCommandEnv)
	return ""
}

func Test_SSHCreds_Environ_ProxyCommandPassesTargetToConnectProxy(t *testing.T) {
	if _, err := exec.LookPath("ssh"); err != nil {
		t.Skip("ssh is not installed")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}

	binDir := t.TempDir()
	argsFile := path.Join(t.TempDir(), "connect-proxy-args")
	fakeConnectProxy := "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$CONNECT_PROXY_ARGS_FILE\"\nexit 1\n"
	require.NoError(t, os.WriteFile(path.Join(binDir, "connect-proxy"), []byte(fakeConnectProxy), 0o755))

	ctx := context.Background()
	closer, env, err := NewSSHCreds(ctx, "sshPrivateKey", "", true, &NoopCredsStore{}, "socks5://127.0.0.1:1080").Environ(ctx)
	require.NoError(t, err)
	defer io.Close(closer)

	proxyScript := proxyScriptFromSSHCommand(t, env[0])
	info, err := os.Stat(proxyScript)
	require.NoError(t, err)
	assert.Equal(t, os.FileMode(0o600), info.Mode().Perm(), "the proxy script must run without the exec bit, /dev/shm is often mounted noexec")

	cmdCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(cmdCtx, "git", "ls-remote", "ssh://git@git.example.com:2222/org/repo.git")
	cmd.WaitDelay = 5 * time.Second
	cmd.Env = append(os.Environ(), env...)
	cmd.Env = append(cmd.Env,
		"PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"CONNECT_PROXY_ARGS_FILE="+argsFile,
		"GIT_TERMINAL_PROMPT=0",
	)
	out, err := cmd.CombinedOutput()
	require.Error(t, err, "the fake connect-proxy always fails")

	args, readErr := os.ReadFile(argsFile)
	require.NoError(t, readErr, "connect-proxy was never invoked: %s", out)
	assert.Equal(t, []string{"-5", "-S", "127.0.0.1:1080", "git.example.com", "2222"}, strings.Fields(string(args)))
}

func TestValidateProxyURL_RejectsUnsafeHostnames(t *testing.T) {
	t.Parallel()
	malicious := []struct {
		name string
		url  string
	}{
		{"single quote and command substitution", "socks5://evil'$(id>/tmp/x)'host:1080"},
		{"semicolon", "socks5://host;id:1080"},
		{"pipe", "socks5://host|id:1080"},
		{"ampersand", "socks5://host&id:1080"},
		{"dollar sign", "socks5://host$(x):1080"},
		{"backtick", "socks5://host`x`:1080"},
		{"redirect gt", "socks5://host>x:1080"},
		{"redirect lt", "socks5://host<x:1080"},
		{"whitespace in host", "socks5://ho st:1080"},
		{"newline in port", "socks5://host:\n1080"},
		{"port zero", "socks5://host:0"},
		{"port too high", "socks5://host:65536"},
		{"empty host", "socks5://:1080"},
		{"wrong scheme http", "http://host:1080"},
		{"wrong scheme https", "https://host:1080"},
		{"wrong scheme socks4", "socks4://host:1080"},
	}
	for _, tc := range malicious {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateProxyURL(tc.url)
			require.Error(t, err, "expected rejection of: %s", tc.url)
		})
	}
}

func TestValidateProxyURL_AcceptsValidURLs(t *testing.T) {
	t.Parallel()
	valid := []struct {
		name string
		url  string
	}{
		{"IPv4", "socks5://127.0.0.1:1080"},
		{"hostname", "socks5://proxy.example.com:1080"},
		{"IPv6 bracketed", "socks5://[::1]:1080"},
		{"IPv6 full address", "socks5h://proxy.internal:9050"},
		{"port 1", "socks5://proxy.example.com:1"},
		{"port 65535", "socks5://proxy.example.com:65535"},
	}
	for _, tc := range valid {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateProxyURL(tc.url)
			require.NoError(t, err)
		})
	}
}

func TestSSHCreds_Environ_RejectsMaliciousProxyURL(t *testing.T) {
	ctx := context.Background()
	t.Parallel()
	// Verify that the malicious PoC proxy URL from the advisory is rejected
	// at construction time (NewSSHCreds), so it never flows into Environ(),
	// GIT_SSH_COMMAND, or any persisted creds state.
	creds := NewSSHCreds(ctx, "sshPrivateKey", "", false, &NoopCredsStore{},
		"socks5://evil'$(id>/tmp/PWNED)'host:1080")
	require.Empty(t, creds.proxy, "malicious proxy must not be stored on the creds struct")

	closer, env, err := creds.Environ(ctx)
	require.NoError(t, err)
	if closer != nil {
		defer closer.Close()
	}
	for _, e := range env {
		assert.NotContains(t, e, "ProxyCommand", "no ProxyCommand must be set for a rejected proxy")
		assert.NotContains(t, e, "PWNED", "no part of the malicious proxy must leak into the environment")
	}
}

const gcpServiceAccountKeyJSON = `{
  "type": "service_account",
  "project_id": "my-google-project",
  "private_key_id": "REDACTED",
  "private_key": "-----BEGIN PRIVATE KEY-----\nREDACTED\n-----END PRIVATE KEY-----\n",
  "client_email": "argocd-service-account@my-google-project.iam.gserviceaccount.com",
  "client_id": "REDACTED",
  "auth_uri": "https://accounts.google.com/o/oauth2/auth",
  "token_uri": "https://oauth2.googleapis.com/token",
  "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
  "client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/argocd-service-account%40my-google-project.iam.gserviceaccount.com"
}`

const invalidJSON = `{
  "type": "service_account",
  "project_id": "my-google-project",
`

func TestNewGoogleCloudCreds(t *testing.T) {
	store := &memoryCredsStore{creds: make(map[string]cred)}
	googleCloudCreds := NewGoogleCloudCreds(gcpServiceAccountKeyJSON, store)
	assert.NotNil(t, googleCloudCreds)
}

func TestNewGoogleCloudCreds_invalidJSON(t *testing.T) {
	ctx := context.Background()

	store := &memoryCredsStore{creds: make(map[string]cred)}
	googleCloudCreds := NewGoogleCloudCreds(invalidJSON, store)
	assert.Nil(t, googleCloudCreds.creds)

	token, err := googleCloudCreds.getAccessToken()
	assert.Equal(t, "", token)
	assert.NotNil(t, err)

	username, err := googleCloudCreds.getUsername()
	assert.Equal(t, "", username)
	assert.NotNil(t, err)

	closer, envStringSlice, err := googleCloudCreds.Environ(ctx)
	assert.Equal(t, NopCloser{}, closer)
	assert.Equal(t, []string(nil), envStringSlice)
	assert.NotNil(t, err)
}

func TestGoogleCloudCreds_Environ_cleanup(t *testing.T) {
	ctx := context.Background()

	store := &memoryCredsStore{creds: make(map[string]cred)}
	staticToken := &oauth2.Token{AccessToken: "token"}
	googleCloudCreds := GoogleCloudCreds{&google.Credentials{
		ProjectID:   "my-google-project",
		TokenSource: oauth2.StaticTokenSource(staticToken),
		JSON:        []byte(gcpServiceAccountKeyJSON),
	}, store}

	closer, env, err := googleCloudCreds.Environ(ctx)
	assert.NoError(t, err)
	var nonce string
	for _, envVar := range env {
		if strings.HasPrefix(envVar, ASKPASS_NONCE_ENV) {
			nonce = envVar[len(ASKPASS_NONCE_ENV)+1:]
			break
		}
	}
	assert.Contains(t, store.creds, nonce)
	io.Close(closer)
	assert.NotContains(t, store.creds, nonce)
}

// ---------------------------------------------------------------------------
// GitHubAppCreds helpers
// ---------------------------------------------------------------------------

// generateRSAPrivateKeyPEM generates a 2048-bit RSA private key in PKCS#1 PEM
// format, which is the format GitHub App private keys use.
func generateRSAPrivateKeyPEM(t *testing.T) []byte {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
}

// githubAppMockServer starts an httptest.Server that responds to the GitHub App
// installation token endpoint. It returns the server and a pointer to a request
// counter (incremented on every token request).
func githubAppMockServer(t *testing.T, token string) (server *httptest.Server, tokenRequests *int) {
	t.Helper()
	n := 0
	tokenRequests = &n
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/access_tokens") {
			n++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"token":      token,
				"expires_at": time.Now().Add(time.Hour).Format(time.RFC3339),
			})
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	return server, tokenRequests
}

// newTestGitHubAppCreds creates a GitHubAppCreds backed by the given mock
// server so no real GitHub API calls are made.
//
// It also pre-seeds Go's http.ProxyFromEnvironment cache with
// "http://proxy-from-env:7878" so that TestCustomHTTPClient (which sets
// http_proxy and expects that value to be returned) is not affected by the
// sync.Once caching that happens when the GitHub App tests call
// GetRepoHTTPClient with an empty proxy URL. Requests to 127.0.0.1 (the
// mock server) bypass this proxy via Go's built-in loopback exclusion.
func newTestGitHubAppCreds(t *testing.T, privateKeyPEM []byte, serverURL string, store CredsStore, opts ...func(*GitHubAppCreds)) GitHubAppCreds {
	t.Helper()
	// Seed the proxy env var so the http.ProxyFromEnvironment sync.Once cache
	// is populated with the right value on first call.
	t.Setenv("http_proxy", "http://proxy-from-env:7878")
	c := GitHubAppCreds{
		appID:        1,
		appInstallId: 2,
		privateKey:   string(privateKeyPEM),
		baseURL:      serverURL,
		store:        store,
	}
	for _, o := range opts {
		o(&c)
	}
	return c
}

// ---------------------------------------------------------------------------
// GitHubAppCreds constructor / getters
// ---------------------------------------------------------------------------

// Compile-time checks: GitHubAppCreds must satisfy the SCM provider interfaces.
var _ SCMTokenProvider = GitHubAppCreds{}
var _ SCMAPIBaseURLProvider = GitHubAppCreds{}

func TestNewGitHubAppCreds(t *testing.T) {
	ctx := context.Background()
	key := generateRSAPrivateKeyPEM(t)
	// NewGitHubAppCreds already returns GenericHTTPSCreds; assert the value is
	// non-nil and that the underlying concrete type also satisfies the two SCM
	// provider interfaces (verified at compile time above).
	creds := NewGitHubAppCreds(ctx, 42, 7, string(key), "https://github.example.com", "https://github.example.com/org/repo", "certdata", "keydata", false, "", &NoopCredsStore{})
	assert.NotNil(t, creds)
	assert.Implements(t, (*SCMTokenProvider)(nil), creds)
	assert.Implements(t, (*SCMAPIBaseURLProvider)(nil), creds)
}

func TestGitHubAppCreds_HasClientCert(t *testing.T) {
	t.Run("WithCert", func(t *testing.T) {
		c := GitHubAppCreds{clientCertData: "cert", clientCertKey: "key"}
		assert.True(t, c.HasClientCert())
	})
	t.Run("NoCertData", func(t *testing.T) {
		c := GitHubAppCreds{clientCertData: "", clientCertKey: "key"}
		assert.False(t, c.HasClientCert())
	})
	t.Run("NoKeyData", func(t *testing.T) {
		c := GitHubAppCreds{clientCertData: "cert", clientCertKey: ""}
		assert.False(t, c.HasClientCert())
	})
	t.Run("NoCertOrKey", func(t *testing.T) {
		c := GitHubAppCreds{}
		assert.False(t, c.HasClientCert())
	})
}

func TestGitHubAppCreds_GetClientCertData(t *testing.T) {
	c := GitHubAppCreds{clientCertData: "my-cert"}
	assert.Equal(t, "my-cert", c.GetClientCertData())
}

func TestGitHubAppCreds_GetClientCertKey(t *testing.T) {
	c := GitHubAppCreds{clientCertKey: "my-key"}
	assert.Equal(t, "my-key", c.GetClientCertKey())
}

func TestGitHubAppCreds_SCMAPIBaseURL(t *testing.T) {
	t.Run("EnterpriseURL", func(t *testing.T) {
		c := GitHubAppCreds{baseURL: "https://github.example.com"}
		assert.Equal(t, "https://github.example.com", c.SCMAPIBaseURL())
	})
	t.Run("EmptyURL", func(t *testing.T) {
		c := GitHubAppCreds{}
		assert.Equal(t, "", c.SCMAPIBaseURL())
	})
}

// ---------------------------------------------------------------------------
// GitHubAppCreds Environ
// ---------------------------------------------------------------------------

func TestGitHubAppCreds_Environ(t *testing.T) {
	ctx := context.Background()

	key := generateRSAPrivateKeyPEM(t)
	server, _ := githubAppMockServer(t, "ghs_test_token")
	store := &memoryCredsStore{creds: make(map[string]cred)}
	creds := newTestGitHubAppCreds(t, key, server.URL, store)

	closer, env, err := creds.Environ(ctx)
	require.NoError(t, err)
	defer io.Close(closer)

	var nonce string
	for _, e := range env {
		if strings.HasPrefix(e, ASKPASS_NONCE_ENV+"=") {
			nonce = e[len(ASKPASS_NONCE_ENV)+1:]
		}
	}
	assert.NotEmpty(t, nonce)
	assert.Contains(t, store.creds, nonce)
	assert.Equal(t, githubAccessTokenUsername, store.creds[nonce].username)

	io.Close(closer)
	assert.NotContains(t, store.creds, nonce)
}

func TestGitHubAppCreds_Environ_insecure(t *testing.T) {
	ctx := context.Background()

	key := generateRSAPrivateKeyPEM(t)
	server, _ := githubAppMockServer(t, "ghs_insecure_token")
	creds := newTestGitHubAppCreds(t, key, server.URL, &NoopCredsStore{}, func(c *GitHubAppCreds) {
		c.insecure = true
	})

	closer, env, err := creds.Environ(ctx)
	require.NoError(t, err)
	defer io.Close(closer)

	found := false
	for _, e := range env {
		if e == "GIT_SSL_NO_VERIFY=true" {
			found = true
		}
	}
	assert.True(t, found, "GIT_SSL_NO_VERIFY=true must be set for insecure creds")
}

func TestGitHubAppCreds_Environ_clientCert(t *testing.T) {
	ctx := context.Background()

	clientCert := generateTestTLSCert(t)
	key := generateRSAPrivateKeyPEM(t)
	server, _ := githubAppMockServer(t, "ghs_cert_token")
	creds := newTestGitHubAppCreds(t, key, server.URL, &NoopCredsStore{}, func(c *GitHubAppCreds) {
		c.clientCertData = string(clientCert.CertPEM)
		c.clientCertKey = string(clientCert.KeyPEM)
	})

	closer, env, err := creds.Environ(ctx)
	require.NoError(t, err)

	var certPath, keyPath string
	for _, e := range env {
		if strings.HasPrefix(e, "GIT_SSL_CERT=") {
			certPath = e[len("GIT_SSL_CERT="):]
		} else if strings.HasPrefix(e, "GIT_SSL_KEY=") {
			keyPath = e[len("GIT_SSL_KEY="):]
		}
	}
	assert.NotEmpty(t, certPath, "GIT_SSL_CERT must be set")
	assert.NotEmpty(t, keyPath, "GIT_SSL_KEY must be set")
	assert.FileExists(t, certPath)
	assert.FileExists(t, keyPath)

	io.Close(closer)
	assert.NoFileExists(t, certPath, "cert temp file must be removed after close")
	assert.NoFileExists(t, keyPath, "key temp file must be removed after close")
}

func TestGitHubAppCreds_Environ_tokenCaching(t *testing.T) {
	ctx := context.Background()

	key := generateRSAPrivateKeyPEM(t)
	server, requestCount := githubAppMockServer(t, "ghs_cached_token")
	store := &memoryCredsStore{creds: make(map[string]cred)}
	creds := newTestGitHubAppCreds(t, key, server.URL, store)

	closer1, _, err := creds.Environ(ctx)
	require.NoError(t, err)
	io.Close(closer1)

	closer2, _, err := creds.Environ(ctx)
	require.NoError(t, err)
	io.Close(closer2)

	assert.Equal(t, 1, *requestCount, "token endpoint must be called exactly once due to caching")
}

// ---------------------------------------------------------------------------
// GitHubAppCreds SCMToken
// ---------------------------------------------------------------------------

func TestGitHubAppCreds_SCMToken(t *testing.T) {
	ctx := context.Background()

	key := generateRSAPrivateKeyPEM(t)
	server, _ := githubAppMockServer(t, "ghs_scm_token")
	creds := newTestGitHubAppCreds(t, key, server.URL, &NoopCredsStore{})

	token, err := creds.SCMToken(ctx)
	require.NoError(t, err)
	assert.Equal(t, "ghs_scm_token", token)
}

// ---------------------------------------------------------------------------
// GitHubAppCreds error paths
// ---------------------------------------------------------------------------

func TestGitHubAppCreds_Environ_invalidPrivateKey(t *testing.T) {
	ctx := context.Background()

	creds := GitHubAppCreds{
		appID:        1,
		appInstallId: 2,
		privateKey:   "this-is-not-a-valid-rsa-key",
		baseURL:      "http://localhost:1",
		store:        &NoopCredsStore{},
	}

	_, _, err := creds.Environ(ctx)
	assert.Error(t, err, "invalid private key must produce an error")
}
