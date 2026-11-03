package git

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	gocache "github.com/patrickmn/go-cache"

	argoio "github.com/argoproj/gitops-engine/pkg/utils/io"
	"github.com/argoproj/gitops-engine/pkg/utils/text"
	"github.com/bradleyfalzon/ghinstallation/v2"
	"github.com/kballard/go-shellquote"
	"github.com/sirupsen/logrus"

	"github.com/argoproj/argo-cd/v3/common"
	certutil "github.com/argoproj/argo-cd/v3/util/cert"
	argoioutils "github.com/argoproj/argo-cd/v3/util/io"

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
)

var (
	// In memory cache for storing github APP api token credentials
	githubAppTokenCache *gocache.Cache
	// In memory cache for storing oauth2.TokenSource used to generate Google Cloud OAuth tokens
	googleCloudTokenSource *gocache.Cache
	// validProxyHostRegexp matches only safe hostname characters: alphanumeric, hyphen, dot, and brackets (for IPv6).
	validProxyHostRegexp = regexp.MustCompile(`^[a-zA-Z0-9.\-]+$`)
)

// githubAppInstallationTransport holds both the ghinstallation transport and
// the underlying HTTP transport, allowing proper cleanup when cache entries expire.
type githubAppInstallationTransport struct {
	transport     *ghinstallation.Transport
	httpTransport *http.Transport
}

const (
	// ASKPASS_NONCE_ENV is the environment variable that is used to pass the nonce to the askpass script
	ASKPASS_NONCE_ENV = "ARGOCD_GIT_ASKPASS_NONCE"
	// githubAccessTokenUsername is a username that is used to with the github access token
	githubAccessTokenUsername = "x-access-token"
	forceBasicAuthHeaderEnv   = "ARGOCD_GIT_AUTH_HEADER"
)

func init() {
	githubAppCredsExp := common.GithubAppCredsExpirationDuration
	if exp := os.Getenv(common.EnvGithubAppCredsExpirationDuration); exp != "" {
		if qps, err := strconv.Atoi(exp); err != nil {
			githubAppCredsExp = time.Duration(qps) * time.Minute
		}
	}

	githubAppTokenCache = gocache.New(githubAppCredsExp, 1*time.Minute)
	// Set up eviction callback to close idle HTTP connections when cache entries expire.
	// This prevents resource exhaustion from orphaned HTTP transports.
	githubAppTokenCache.OnEvicted(func(_ string, value interface{}) {
		if t, ok := value.(*githubAppInstallationTransport); ok && t.httpTransport != nil {
			t.httpTransport.CloseIdleConnections()
		}
	})
	// oauth2.TokenSource handles fetching new Tokens once they are expired. The oauth2.TokenSource itself does not expire.
	googleCloudTokenSource = gocache.New(gocache.NoExpiration, 0)
}

func validateProxyURL(rawURL string) error {
	return validateProxyURLWithSchemes(rawURL, "socks5", "socks5h")
}

// ValidateHTTPProxyURL validates a proxy URL for HTTP/HTTPS creds (GitHub App,
// Azure Service Principal, HTTPS creds). It accepts http, https, socks5 and
// socks5h schemes.
func ValidateHTTPProxyURL(rawURL string) error {
	return validateProxyURLWithSchemes(rawURL, "http", "https", "socks5", "socks5h")
}

func validateProxyURLWithSchemes(rawURL string, allowedSchemes ...string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid proxy URL: %w", err)
	}

	schemeOK := slices.Contains(allowedSchemes, parsed.Scheme)
	if !schemeOK {
		return fmt.Errorf("invalid proxy scheme %q: only %v are supported", parsed.Scheme, allowedSchemes)
	}

	host := parsed.Hostname()
	if host == "" {
		return errors.New("proxy URL has empty hostname")
	}
	if strings.Contains(host, ":") {
		// Treat as IPv6 — validate with net.ParseIP which understands all
		// valid IPv6 formats and rejects anything containing shell metacharacters.
		if net.ParseIP(host) == nil {
			return fmt.Errorf("proxy hostname %q is not a valid IPv6 address", host)
		}
	} else {
		// DNS name or IPv4 — allow only alphanumerics, hyphens, and dots.
		if !validProxyHostRegexp.MatchString(host) {
			return fmt.Errorf("proxy hostname %q contains unsafe characters: only alphanumerics, hyphens, and dots are allowed", host)
		}
	}

	portStr := parsed.Port()
	if portStr == "" {
		return errors.New("proxy URL has no port")
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("proxy port %q is not a valid TCP port (must be 1-65535)", portStr)
	}

	return nil
}

type NoopCredsStore struct {
}

func (d NoopCredsStore) Add(username string, password string) string {
	return ""
}

func (d NoopCredsStore) Remove(id string) {
}

type CredsStore interface {
	Add(username string, password string) string
	Remove(id string)
}

type Creds interface {
	Environ(ctx context.Context) (io.Closer, []string, error)
}

func getGitAskPassEnv(id string) []string {
	// TODO(jannfis): This change should go upstream into Argo CD. Calling the
	// full path to currently executing binary instead of relying on a binary
	// named "argocd" in the PATH has only benefits.
	cmd, err := os.Executable()
	if err != nil {
		return []string{}
	}
	return []string{
		fmt.Sprintf("GIT_ASKPASS=%s", cmd),
		fmt.Sprintf("%s=%s", ASKPASS_NONCE_ENV, id),
		"GIT_TERMINAL_PROMPT=0",
		"ARGOCD_BINARY_NAME=argocd-git-ask-pass",
	}
}

// nop implementation
type NopCloser struct {
}

func (c NopCloser) Close() error {
	return nil
}

var _ Creds = NopCreds{}

type NopCreds struct {
}

func (c NopCreds) Environ(context.Context) (io.Closer, []string, error) {
	return NopCloser{}, nil, nil
}

var _ io.Closer = NopCloser{}

type GenericHTTPSCreds interface {
	HasClientCert() bool
	GetClientCertData() string
	GetClientCertKey() string
	Environ(ctx context.Context) (io.Closer, []string, error)
}

var _ GenericHTTPSCreds = HTTPSCreds{}

// HTTPS creds implementation
type HTTPSCreds struct {
	// Username for authentication
	username string
	// Password for authentication
	password string
	// Whether to ignore invalid server certificates
	insecure bool
	// Client certificate to use
	clientCertData string
	// Client certificate key to use
	clientCertKey string
	// HTTP/HTTPS proxy used to access repository
	proxy string
	// temporal credentials store
	store CredsStore
	// whether to force usage of basic auth
	forceBasicAuth bool
}

func NewHTTPSCreds(ctx context.Context, username string, password string, clientCertData string, clientCertKey string, insecure bool, proxy string, store CredsStore, forceBasicAuth bool) GenericHTTPSCreds {
	log := log.LoggerFromContext(ctx)

	if proxy != "" {
		if err := ValidateHTTPProxyURL(proxy); err != nil {
			log.WithFields(logrus.Fields{
				common.SecurityField:    common.SecurityHigh,
				common.SecurityCWEField: 78,
			}).Warnf("NewHTTPSCreds: rejecting unsafe proxy URL: %v", err)
			proxy = ""
		}
	}

	return HTTPSCreds{
		username,
		password,
		insecure,
		clientCertData,
		clientCertKey,
		proxy,
		store,
		forceBasicAuth,
	}
}

func (c HTTPSCreds) BasicAuthHeader() string {
	h := "Authorization: Basic "
	t := c.username + ":" + c.password
	h += base64.StdEncoding.EncodeToString([]byte(t))
	return h
}

// Get additional required environment variables for executing git client to
// access specific repository via HTTPS.
func (c HTTPSCreds) Environ(ctx context.Context) (io.Closer, []string, error) {
	log := log.LoggerFromContext(ctx)

	var env []string

	httpCloser := authFilePaths(make([]string, 0))

	// GIT_SSL_NO_VERIFY is used to tell git not to validate the server's cert at
	// all.
	if c.insecure {
		env = append(env, "GIT_SSL_NO_VERIFY=true")
	}

	// In case the repo is configured for using a TLS client cert, we need to make
	// sure git client will use it. The certificate's key must not be password
	// protected.
	if c.HasClientCert() {
		var certFile, keyFile *os.File

		// We need to actually create two temp files, one for storing cert data and
		// another for storing the key. If we fail to create second fail, the first
		// must be removed.
		certFile, err := os.CreateTemp(argoio.TempDir, "")
		if err == nil {
			defer certFile.Close()
			keyFile, err = os.CreateTemp(argoio.TempDir, "")
			if err != nil {
				removeErr := os.Remove(certFile.Name())
				if removeErr != nil {
					log.Errorf("Could not remove previously created tempfile %s: %v", certFile.Name(), removeErr)
				}
				return NopCloser{}, nil, err
			}
			defer keyFile.Close()
		} else {
			return NopCloser{}, nil, err
		}

		// We should have both temp files by now
		httpCloser = authFilePaths([]string{certFile.Name(), keyFile.Name()})

		_, err = certFile.WriteString(c.clientCertData)
		if err != nil {
			httpCloser.Close(ctx)
			return NopCloser{}, nil, err
		}
		// GIT_SSL_CERT is the full path to a client certificate to be used
		env = append(env, fmt.Sprintf("GIT_SSL_CERT=%s", certFile.Name()))

		_, err = keyFile.WriteString(c.clientCertKey)
		if err != nil {
			httpCloser.Close(ctx)
			return NopCloser{}, nil, err
		}
		// GIT_SSL_KEY is the full path to a client certificate's key to be used
		env = append(env, fmt.Sprintf("GIT_SSL_KEY=%s", keyFile.Name()))
	}
	// If at least password is set, we will set ARGOCD_BASIC_AUTH_HEADER to
	// hold the HTTP authorization header, so auth mechanism negotiation is
	// skipped. This is insecure, but some environments may need it.
	if c.password != "" && c.forceBasicAuth {
		env = append(env, fmt.Sprintf("%s=%s", forceBasicAuthHeaderEnv, c.BasicAuthHeader()))
	}
	nonce := c.store.Add(text.FirstNonEmpty(c.username, githubAccessTokenUsername), c.password)
	env = append(env, getGitAskPassEnv(nonce)...)
	return argoioutils.NewCloser(func() error {
		c.store.Remove(nonce)
		return httpCloser.Close(ctx)
	}), env, nil
}

func (g HTTPSCreds) HasClientCert() bool {
	return g.clientCertData != "" && g.clientCertKey != ""
}

func (c HTTPSCreds) GetClientCertData() string {
	return c.clientCertData
}

func (c HTTPSCreds) GetClientCertKey() string {
	return c.clientCertKey
}

// SSH implementation
type SSHCreds struct {
	sshPrivateKey string
	caPath        string
	insecure      bool
	store         CredsStore
	proxy         string
}

func NewSSHCreds(ctx context.Context, sshPrivateKey string, caPath string, insecureIgnoreHostKey bool, store CredsStore, proxy string) SSHCreds {
	log := log.LoggerFromContext(ctx)

	if proxy != "" {
		if err := validateProxyURL(proxy); err != nil {
			log.WithFields(logrus.Fields{
				common.SecurityField:    common.SecurityHigh,
				common.SecurityCWEField: 78,
			}).Warnf("NewSSHCreds: rejecting unsafe proxy URL: %v", err)
			proxy = ""
		}
	}
	return SSHCreds{sshPrivateKey, caPath, insecureIgnoreHostKey, store, proxy}
}

type sshPrivateKeyFile string

type authFilePaths []string

func (f sshPrivateKeyFile) Close() error {
	return os.Remove(string(f))
}

// Remove a list of files that have been created as temp files while creating
// HTTPCreds object above.
func (f authFilePaths) Close(ctx context.Context) error {
	log := log.LoggerFromContext(ctx)

	var retErr error = nil
	for _, path := range f {
		err := os.Remove(path)
		if err != nil {
			log.Errorf("HTTPSCreds.Close(): Could not remove temp file %s: %v", path, err)
			retErr = err
		}
	}
	return retErr
}

func (c SSHCreds) Environ(ctx context.Context) (io.Closer, []string, error) {
	log := log.LoggerFromContext(ctx)

	// use the SHM temp dir from util, more secure
	file, err := os.CreateTemp(argoio.TempDir, "")
	if err != nil {
		return nil, nil, err
	}

	sshCloser := sshPrivateKeyFile(file.Name())

	defer func() {
		if err = file.Close(); err != nil {
			log.WithFields(logrus.Fields{
				common.SecurityField:    common.SecurityMedium,
				common.SecurityCWEField: common.SecurityCWEMissingReleaseOfFileDescriptor,
			}).Errorf("error closing file %q: %v", file.Name(), err)
		}
	}()

	_, err = file.WriteString(c.sshPrivateKey + "\n")
	if err != nil {
		sshCloser.Close()
		return nil, nil, err
	}

	args := []string{"ssh", "-i", file.Name()}
	var env []string
	if c.caPath != "" {
		env = append(env, fmt.Sprintf("GIT_SSL_CAINFO=%s", c.caPath))
	}
	if c.insecure {
		log.Warn("temporarily disabling strict host key checking (i.e. '-o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null'), please don't use in production")
		// StrictHostKeyChecking will add the host to the knownhosts file,  we don't want that - a security issue really,
		// UserKnownHostsFile=/dev/null is therefore used so we write the new insecure host to /dev/null
		args = append(args, "-o", "StrictHostKeyChecking=no", "-o", "UserKnownHostsFile=/dev/null")
	} else {
		knownHostsFile := certutil.GetSSHKnownHostsDataPath()
		args = append(args, "-o", "StrictHostKeyChecking=yes", "-o", fmt.Sprintf("UserKnownHostsFile=%s", knownHostsFile))
	}
	// Handle SSH socks5 proxy settings
	var proxyEnv []string
	var proxyScriptPath string

	if c.proxy != "" {
		if err := validateProxyURL(c.proxy); err != nil {
			sshCloser.Close()
			return nil, nil, fmt.Errorf("unsafe proxy URL rejected: %w", err)
		}

		parsedProxyURL, err := url.Parse(c.proxy)
		if err != nil {
			sshCloser.Close()
			return nil, nil, fmt.Errorf("failed to set environment variables related to socks5 proxy, could not parse proxy URL '%s': %w", c.proxy, err)
		}

		// Write a temporary wrapper script instead of embedding the hostname and port directly into the
		// shell-interpreted GIT_SSH_COMMAND string.
		proxyScriptContent := fmt.Sprintf(
			"#!/bin/sh\nexec connect-proxy -5 -S %s:%s \"$@\"\n",
			parsedProxyURL.Hostname(),
			parsedProxyURL.Port(),
		)

		proxyScript, err := os.CreateTemp(argoio.TempDir, "argocd-proxy-cmd-*")
		if err != nil {
			sshCloser.Close()
			return nil, nil, fmt.Errorf("failed to create proxy command script: %w", err)
		}
		proxyScriptPath = proxyScript.Name()

		if _, err := proxyScript.WriteString(proxyScriptContent); err != nil {
			_ = os.Remove(proxyScriptPath)
			sshCloser.Close()
			return nil, nil, fmt.Errorf("failed to write proxy command script: %w", err)
		}
		if err := proxyScript.Close(); err != nil {
			_ = os.Remove(proxyScriptPath)
			sshCloser.Close()
			return nil, nil, fmt.Errorf("failed to close proxy command script: %w", err)
		}

		args = append(args, "-o", "ProxyCommand="+shellquote.Join("/bin/sh", proxyScriptPath)+" %h %p")

		if parsedProxyURL.User != nil {
			proxyEnv = append(proxyEnv, fmt.Sprintf("SOCKS5_USER=%s", parsedProxyURL.User.Username()))
			if socks5_passwd, isPasswdSet := parsedProxyURL.User.Password(); isPasswdSet {
				proxyEnv = append(proxyEnv, fmt.Sprintf("SOCKS5_PASSWD=%s", socks5_passwd))
			}
		}
	}
	env = append(env, []string{"GIT_SSH_COMMAND=" + shellquote.Join(args...)}...)
	env = append(env, proxyEnv...)

	// We need a combined closer that removes both the SSH private key tempfile
	// and the proxy script tempfile (if one was created).
	combinedCloser := argoioutils.NewCloser(func() error {
		keyErr := sshCloser.Close()
		var proxyErr error
		if proxyScriptPath != "" {
			proxyErr = os.Remove(proxyScriptPath)
			if os.IsNotExist(proxyErr) {
				proxyErr = nil
			}
		}
		if keyErr != nil {
			return keyErr
		}
		return proxyErr
	})

	return combinedCloser, env, nil
}

// GitHubAppCreds to authenticate as GitHub application
type GitHubAppCreds struct {
	appID          int64
	appInstallId   int64
	privateKey     string
	baseURL        string
	repoURL        string
	clientCertData string
	clientCertKey  string
	insecure       bool
	proxy          string
	store          CredsStore
}

// NewGitHubAppCreds provide github app credentials
func NewGitHubAppCreds(ctx context.Context, appID int64, appInstallId int64, privateKey string, baseURL string, repoURL string, clientCertData string, clientCertKey string, insecure bool, proxy string, store CredsStore) GenericHTTPSCreds {
	log := log.LoggerFromContext(ctx)

	if proxy != "" {
		if err := ValidateHTTPProxyURL(proxy); err != nil {
			log.WithFields(logrus.Fields{
				common.SecurityField:    common.SecurityHigh,
				common.SecurityCWEField: 78,
			}).Warnf("NewGitHubAppCreds: rejecting unsafe proxy URL: %v", err)
			proxy = ""
		}
	}
	return GitHubAppCreds{appID: appID, appInstallId: appInstallId, privateKey: privateKey, baseURL: baseURL, repoURL: repoURL, clientCertData: clientCertData, clientCertKey: clientCertKey, insecure: insecure, proxy: proxy, store: store}
}

func (g GitHubAppCreds) Environ(ctx context.Context) (io.Closer, []string, error) {
	log := log.LoggerFromContext(ctx)

	token, err := g.getAccessToken(ctx)
	if err != nil {
		return NopCloser{}, nil, err
	}
	var env []string
	httpCloser := authFilePaths(make([]string, 0))

	// GIT_SSL_NO_VERIFY is used to tell git not to validate the server's cert at
	// all.
	if g.insecure {
		env = append(env, "GIT_SSL_NO_VERIFY=true")
	}

	// In case the repo is configured for using a TLS client cert, we need to make
	// sure git client will use it. The certificate's key must not be password
	// protected.
	if g.HasClientCert() {
		var certFile, keyFile *os.File

		// We need to actually create two temp files, one for storing cert data and
		// another for storing the key. If we fail to create second fail, the first
		// must be removed.
		certFile, err := os.CreateTemp(argoio.TempDir, "")
		if err == nil {
			defer certFile.Close()
			keyFile, err = os.CreateTemp(argoio.TempDir, "")
			if err != nil {
				removeErr := os.Remove(certFile.Name())
				if removeErr != nil {
					log.Errorf("Could not remove previously created tempfile %s: %v", certFile.Name(), removeErr)
				}
				return NopCloser{}, nil, err
			}
			defer keyFile.Close()
		} else {
			return NopCloser{}, nil, err
		}

		// We should have both temp files by now
		httpCloser = []string{certFile.Name(), keyFile.Name()}

		_, err = certFile.WriteString(g.clientCertData)
		if err != nil {
			httpCloser.Close(ctx)
			return NopCloser{}, nil, err
		}
		// GIT_SSL_CERT is the full path to a client certificate to be used
		env = append(env, fmt.Sprintf("GIT_SSL_CERT=%s", certFile.Name()))

		_, err = keyFile.WriteString(g.clientCertKey)
		if err != nil {
			httpCloser.Close(ctx)
			return NopCloser{}, nil, err
		}
		// GIT_SSL_KEY is the full path to a client certificate's key to be used
		env = append(env, fmt.Sprintf("GIT_SSL_KEY=%s", keyFile.Name()))

	}
	nonce := g.store.Add(githubAccessTokenUsername, token)
	env = append(env, getGitAskPassEnv(nonce)...)
	return argoioutils.NewCloser(func() error {
		g.store.Remove(nonce)
		return httpCloser.Close(ctx)
	}), env, nil
}

// getAccessToken fetches GitHub token using the app id, install id, and private key.
// the token is then cached for re-use.
func (g GitHubAppCreds) getAccessToken(ctx context.Context) (string, error) {
	// Timeout
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// Compute hash of creds for lookup in cache
	h := sha256.New()
	_, err := h.Write([]byte(fmt.Sprintf("%s %d %d %s", g.privateKey, g.appID, g.appInstallId, g.baseURL)))
	if err != nil {
		return "", err
	}
	key := fmt.Sprintf("%x", h.Sum(nil))

	// Check cache for GitHub transport which helps fetch an API token
	t, found := githubAppTokenCache.Get(key)
	if found {
		cachedTransport := t.(*githubAppInstallationTransport)
		// This method caches the token and if it's expired retrieves a new one
		return cachedTransport.transport.Token(ctx)
	}

	// GitHub API url
	baseUrl := "https://api.github.com"
	if g.baseURL != "" {
		baseUrl = strings.TrimSuffix(g.baseURL, "/")
	}

	// Create a new GitHub transport
	c := GetRepoHTTPClient(ctx, baseUrl, g.insecure, g, g.proxy)
	// Store reference to the underlying HTTP transport for cleanup on eviction
	httpTransport, _ := c.Transport.(*http.Transport)
	itr, err := ghinstallation.New(c.Transport,
		g.appID,
		g.appInstallId,
		[]byte(g.privateKey),
	)
	if err != nil {
		return "", err
	}

	itr.BaseURL = baseUrl

	// Add transport to cache with both the ghinstallation transport and underlying HTTP transport
	// The HTTP transport reference allows proper cleanup when the cache entry is evicted
	githubAppTokenCache.Set(key, &githubAppInstallationTransport{
		transport:     itr,
		httpTransport: httpTransport,
	}, time.Minute*60)

	return itr.Token(ctx)
}

func (g GitHubAppCreds) HasClientCert() bool {
	return g.clientCertData != "" && g.clientCertKey != ""
}

func (g GitHubAppCreds) GetClientCertData() string {
	return g.clientCertData
}

func (g GitHubAppCreds) GetClientCertKey() string {
	return g.clientCertKey
}

// GoogleCloudCreds to authenticate to Google Cloud Source repositories
type GoogleCloudCreds struct {
	creds *google.Credentials
	store CredsStore
}

func NewGoogleCloudCreds(jsonData string, store CredsStore) GoogleCloudCreds {
	creds, err := google.CredentialsFromJSON(context.Background(), []byte(jsonData), "https://www.googleapis.com/auth/cloud-platform")
	if err != nil {
		// Invalid JSON
		log.Errorf("Failed reading credentials from JSON: %+v", err)
	}
	return GoogleCloudCreds{creds, store}
}

func (c GoogleCloudCreds) Environ(context.Context) (io.Closer, []string, error) {
	username, err := c.getUsername()
	if err != nil {
		return NopCloser{}, nil, fmt.Errorf("failed to get username from creds: %w", err)
	}
	token, err := c.getAccessToken()
	if err != nil {
		return NopCloser{}, nil, fmt.Errorf("failed to get access token from creds: %w", err)
	}

	nonce := c.store.Add(username, token)
	env := getGitAskPassEnv(nonce)

	return argoioutils.NewCloser(func() error {
		c.store.Remove(nonce)
		return NopCloser{}.Close()
	}), env, nil
}

func (c GoogleCloudCreds) getUsername() (string, error) {
	type googleCredentialsFile struct {
		Type string `json:"type"`

		// Service Account fields
		ClientEmail  string `json:"client_email"`
		PrivateKeyID string `json:"private_key_id"`
		PrivateKey   string `json:"private_key"`
		AuthURL      string `json:"auth_uri"`
		TokenURL     string `json:"token_uri"`
		ProjectID    string `json:"project_id"`
	}

	if c.creds == nil {
		return "", errors.New("credentials for Google Cloud Source repositories are invalid")
	}

	var f googleCredentialsFile
	if err := json.Unmarshal(c.creds.JSON, &f); err != nil {
		return "", fmt.Errorf("failed to unmarshal Google Cloud credentials: %w", err)
	}
	return f.ClientEmail, nil
}

func (c GoogleCloudCreds) getAccessToken() (string, error) {
	if c.creds == nil {
		return "", errors.New("credentials for Google Cloud Source repositories are invalid")
	}

	// Compute hash of creds for lookup in cache
	h := sha256.New()
	_, err := h.Write(c.creds.JSON)
	if err != nil {
		return "", err
	}
	key := fmt.Sprintf("%x", h.Sum(nil))

	t, found := googleCloudTokenSource.Get(key)
	if found {
		ts := t.(*oauth2.TokenSource)
		token, err := (*ts).Token()
		if err != nil {
			return "", fmt.Errorf("failed to get token from Google Cloud token source: %w", err)
		}
		return token.AccessToken, nil
	}

	ts := c.creds.TokenSource

	// Add TokenSource to cache
	// As TokenSource handles refreshing tokens once they expire itself, TokenSource itself can be reused. Hence, no expiration.
	googleCloudTokenSource.Set(key, &ts, gocache.NoExpiration)

	token, err := ts.Token()
	if err != nil {
		return "", fmt.Errorf("failed to get get SHA256 hash for Google Cloud credentials: %w", err)
	}

	return token.AccessToken, nil
}
