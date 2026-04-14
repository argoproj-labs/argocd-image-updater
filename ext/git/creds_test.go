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
	"path"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
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
	creds := NewHTTPSCreds("", "", "", "", true, "", store, false)
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

	creds := NewHTTPSCreds("", "", "", "", true, "", &NoopCredsStore{}, false)
	closer, env, err := creds.Environ(ctx)
	t.Cleanup(func() {
		io.Close(closer)
	})
	require.NoError(t, err)
	found := false
	for _, envVar := range env {
		if envVar == "GIT_SSL_NO_VERIFY=true" {
			found = true
			break
		}
	}
	assert.True(t, found)
}

func TestHTTPSCreds_Environ_insecure_false(t *testing.T) {
	ctx := context.Background()
	creds := NewHTTPSCreds("", "", "", "", false, "", &NoopCredsStore{}, false)
	closer, env, err := creds.Environ(ctx)
	t.Cleanup(func() {
		io.Close(closer)
	})
	require.NoError(t, err)
	found := false
	for _, envVar := range env {
		if envVar == "GIT_SSL_NO_VERIFY=true" {
			found = true
			break
		}
	}
	assert.False(t, found)
}

func TestHTTPSCreds_Environ_forceBasicAuth(t *testing.T) {
	t.Run("Enabled and credentials set", func(t *testing.T) {
		ctx := context.Background()
		store := &memoryCredsStore{creds: make(map[string]cred)}
		creds := NewHTTPSCreds("username", "password", "", "", false, "", store, true)
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
		creds := NewHTTPSCreds("", "", "", "", false, "", store, true)
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
		creds := NewHTTPSCreds("username", "password", "", "", false, "", store, false)
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
		creds := NewHTTPSCreds("", "", "", "", false, "", store, false)
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
	creds := NewHTTPSCreds("", "", "clientCertData", "clientCertKey", false, "", store, false)
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
		creds := NewSSHCreds("sshPrivateKey", caFile, insecureIgnoreHostKey, &NoopCredsStore{}, "")
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
		creds := NewSSHCreds("sshPrivateKey", caFile, insecureIgnoreHostKey, &NoopCredsStore{}, "socks5://127.0.0.1:1080")
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
		assert.Contains(t, env[1], "-o ProxyCommand='connect-proxy -S 127.0.0.1:1080 -5 %h %p'")

		envRegex := regexp.MustCompile("-i ([^ ]+)")
		assert.Regexp(t, envRegex, env[1])
		privateKeyFile := envRegex.FindStringSubmatch(env[1])[1]
		assert.FileExists(t, privateKeyFile)
		io.Close(closer)
		assert.NoFileExists(t, privateKeyFile)
	}
}

func Test_SSHCreds_Environ_WithProxyUserNamePassword(t *testing.T) {
	for _, insecureIgnoreHostKey := range []bool{false, true} {
		ctx := context.Background()

		tempDir := t.TempDir()
		caFile := path.Join(tempDir, "caFile")
		err := os.WriteFile(caFile, []byte(""), os.FileMode(0600))
		require.NoError(t, err)
		creds := NewSSHCreds("sshPrivateKey", caFile, insecureIgnoreHostKey, &NoopCredsStore{}, "socks5://user:password@127.0.0.1:1080")
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
		assert.Contains(t, env[1], "-o ProxyCommand='connect-proxy -S 127.0.0.1:1080 -5 %h %p'")

		envRegex := regexp.MustCompile("-i ([^ ]+)")
		assert.Regexp(t, envRegex, env[1])
		privateKeyFile := envRegex.FindStringSubmatch(env[1])[1]
		assert.FileExists(t, privateKeyFile)
		io.Close(closer)
		assert.NoFileExists(t, privateKeyFile)
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
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
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

func TestNewGitHubAppCreds(t *testing.T) {
	key := generateRSAPrivateKeyPEM(t)
	creds := NewGitHubAppCreds(42, 7, string(key), "https://github.example.com", "https://github.example.com/org/repo",
		"certdata", "keydata", false, "", &NoopCredsStore{})
	assert.NotNil(t, creds)

	_, isHTTPS := creds.(GenericHTTPSCreds)
	assert.True(t, isHTTPS)
	_, isSCMToken := creds.(SCMTokenProvider)
	assert.True(t, isSCMToken)
	_, isSCMBase := creds.(SCMAPIBaseURLProvider)
	assert.True(t, isSCMBase)
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
