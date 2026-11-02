package git

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"
	"testing"

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
	creds := NewHTTPSCreds(ctx, "", "", "", "", false, "", &NoopCredsStore{}, false)
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
		proxyRegex := regexp.MustCompile(`-o ProxyCommand=([^ ]+)`)
		assert.Regexp(t, proxyRegex, env[1])
		proxyScript := proxyRegex.FindStringSubmatch(env[1])[1]
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
		proxyRegex := regexp.MustCompile(`-o ProxyCommand=([^ ]+)`)
		assert.Regexp(t, proxyRegex, env[1])
		proxyScript := proxyRegex.FindStringSubmatch(env[1])[1]
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
