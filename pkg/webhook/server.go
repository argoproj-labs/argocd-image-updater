package webhook

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"go.uber.org/ratelimit"

	api "github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"
	"github.com/argoproj-labs/argocd-image-updater/internal/controller"
	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
)

const (
	// DefaultTLSCertPath is the default path to the TLS certificate file
	DefaultTLSCertPath = "/app/config/tls/tls.crt"
	// DefaultTLSKeyPath is the default path to the TLS private key file
	DefaultTLSKeyPath = "/app/config/tls/tls.key"
	// DefaultTLSMinVersion is the default minimum TLS version
	DefaultTLSMinVersion = "1.3"
	// DefaultTLSMaxVersion is the default maximum TLS version
	DefaultTLSMaxVersion = "1.3"
)

// TLSConfig holds TLS configuration for the server
type TLSConfig struct {
	// CertFile is the path to the TLS certificate file
	CertFile string
	// KeyFile is the path to the TLS private key file
	KeyFile string
	// MinVersion is the minimum TLS version (e.g. "1.2", "1.3")
	MinVersion string
	// MaxVersion is the maximum TLS version (e.g. "1.2", "1.3")
	MaxVersion string
	// Ciphers is a colon-separated list of TLS cipher suite names
	Ciphers string
}

// WebhookServer manages webhook endpoints and triggers update checks
type WebhookServer struct {
	// We pass the whole Reconciler struct here, since it now holds all dependencies.
	Reconciler *controller.ImageUpdaterReconciler
	// Port is the port number to listen on
	Port int
	// Handler is the webhook handler
	Handler *WebhookHandler
	// Server is the HTTP server
	Server *http.Server
	// rate limiter to limit requests in an interval
	RateLimiter ratelimit.Limiter
	// TLS holds TLS configuration for the server
	TLS *TLSConfig
	// DisableTLS disables TLS and runs plain HTTP
	DisableTLS bool
}

// NewWebhookServer creates a new webhook server
func NewWebhookServer(port int, handler *WebhookHandler, reconciler *controller.ImageUpdaterReconciler) *WebhookServer {
	return &WebhookServer{
		Reconciler:  reconciler,
		Port:        port,
		Handler:     handler,
		RateLimiter: nil,
	}
}

// tlsVersionMap maps version strings to tls version constants.
// TLS 1.0 is not supported as it is considered insecure.
var tlsVersionMap = map[string]uint16{
	"1.1":    tls.VersionTLS11,
	"tls1.1": tls.VersionTLS11,
	"1.2":    tls.VersionTLS12,
	"tls1.2": tls.VersionTLS12,
	"1.3":    tls.VersionTLS13,
	"tls1.3": tls.VersionTLS13,
}

// TLSVersionName returns a human-readable name for a TLS version constant.
func TLSVersionName(version uint16) string {
	for name, v := range tlsVersionMap {
		if v == version {
			return name
		}
	}
	return fmt.Sprintf("unknown (%d)", version)
}

// ParseTLSVersion parses a TLS version string (e.g. "1.2", "1.3", "TLS1.2") into a tls version constant.
// Returns 0 if the string is empty (meaning "use default").
func ParseTLSVersion(version string) (uint16, error) {
	if version == "" {
		return 0, nil
	}
	v, ok := tlsVersionMap[strings.ToLower(strings.TrimSpace(version))]
	if !ok {
		return 0, fmt.Errorf("unsupported TLS version: %q (supported: 1.1, 1.2, 1.3)", version)
	}
	return v, nil
}

// ParseTLSCiphers parses a colon-separated list of cipher suite names into cipher suite IDs.
// Only secure cipher suites (from tls.CipherSuites()) are allowed.
// Returns nil if the input is empty.
func ParseTLSCiphers(ciphers string) ([]uint16, error) {
	if ciphers == "" {
		return nil, nil
	}

	// Build lookup map from Go's secure cipher suites only
	cipherMap := make(map[string]uint16)
	for _, cs := range tls.CipherSuites() {
		cipherMap[cs.Name] = cs.ID
	}

	var result []uint16
	for _, name := range strings.Split(ciphers, ":") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		id, ok := cipherMap[name]
		if !ok {
			return nil, fmt.Errorf("unsupported TLS cipher suite: %q", name)
		}
		result = append(result, id)
	}
	return result, nil
}

// ValidateTLSConfig validates the TLS configuration parameters.
// It checks that:
//   - The minimum TLS version is not greater than the maximum TLS version
//   - All configured cipher suites are compatible with the minimum TLS version
func ValidateTLSConfig(minVersion, maxVersion uint16, cipherSuites []uint16) error {
	if minVersion != 0 && maxVersion != 0 && minVersion > maxVersion {
		return fmt.Errorf("minimum TLS version (%s) cannot be higher than maximum TLS version (%s)",
			TLSVersionName(minVersion), TLSVersionName(maxVersion))
	}

	if len(cipherSuites) > 0 && minVersion != 0 {
		availableCiphers := tls.CipherSuites()
		for _, cipherID := range cipherSuites {
			for _, cs := range availableCiphers {
				if cs.ID == cipherID {
					supported := false
					for _, v := range cs.SupportedVersions {
						if v == minVersion {
							supported = true
							break
						}
					}
					if !supported {
						return fmt.Errorf("cipher suite %s is not supported by minimum TLS version %s",
							cs.Name, TLSVersionName(minVersion))
					}
					break
				}
			}
		}
	}

	return nil
}

// buildTLSConfig creates a *tls.Config from the TLSConfig settings.
func (t *TLSConfig) buildTLSConfig() (*tls.Config, error) {
	tlsCfg := &tls.Config{} //nolint:gosec // min version is set below from user config

	minVer, err := ParseTLSVersion(t.MinVersion)
	if err != nil {
		return nil, fmt.Errorf("invalid --tlsminversion: %w", err)
	}
	tlsCfg.MinVersion = minVer

	maxVer, err := ParseTLSVersion(t.MaxVersion)
	if err != nil {
		return nil, fmt.Errorf("invalid --tlsmaxversion: %w", err)
	}
	tlsCfg.MaxVersion = maxVer

	ciphers, err := ParseTLSCiphers(t.Ciphers)
	if err != nil {
		return nil, fmt.Errorf("invalid --tlsciphers: %w", err)
	}

	// Go's tls.Config.CipherSuites only applies to TLS 1.0–1.2.
	// TLS 1.3 cipher suites are not configurable and are always enabled.
	if len(ciphers) > 0 && minVer >= tls.VersionTLS13 {
		log.Log().Warnf("--tlsciphers has no effect when --tlsminversion is 1.3 or higher (TLS 1.3 cipher suites are not configurable), ignoring")
		ciphers = nil
	}
	tlsCfg.CipherSuites = ciphers

	// Validate TLS version range and cipher/version compatibility
	if err := ValidateTLSConfig(minVer, maxVer, ciphers); err != nil {
		return nil, err
	}

	return tlsCfg, nil
}

// generateSelfSignedCert generates a self-signed TLS certificate in memory.
func generateSelfSignedCert() (tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to generate private key: %w", err)
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to generate serial number: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Argo CD Image Updater"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to create certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to marshal private key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return tls.X509KeyPair(certPEM, keyPEM)
}

// validateCertValidity parses the leaf certificates from a tls.Certificate and
// checks that none are expired or not yet valid.
func validateCertValidity(cert tls.Certificate, certPath string) error {
	for _, c := range cert.Certificate {
		parsed, err := x509.ParseCertificate(c)
		if err != nil {
			return fmt.Errorf("could not parse certificate from %s: %w", certPath, err)
		}
		now := time.Now()
		if now.After(parsed.NotAfter) {
			return fmt.Errorf("TLS certificate from %s has expired on %s", certPath, parsed.NotAfter.Format(time.RFC1123Z))
		}
		if now.Before(parsed.NotBefore) {
			return fmt.Errorf("TLS certificate from %s is not yet valid, valid from %s", certPath, parsed.NotBefore.Format(time.RFC1123Z))
		}
	}
	return nil
}

// certFilesExist checks whether both the certificate and key files exist on
// disk and are non-empty. Zero-byte files (e.g. from an uninitialized
// Kubernetes TLS secret) are treated as absent so the server can fall back
// to self-signed certificate generation.
func certFilesExist(certFile, keyFile string) bool {
	for _, f := range []string{certFile, keyFile} {
		info, err := os.Stat(f)
		if err != nil || info.Size() == 0 {
			return false
		}
	}
	return true
}

// Start starts the webhook server
func (s *WebhookServer) Start(ctx context.Context) error {
	log := log.LoggerFromContext(ctx)
	// Create server and register routes
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", s.handleWebhook)
	mux.HandleFunc("/healthz", s.handleHealth)

	addr := fmt.Sprintf(":%d", s.Port)
	s.Server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// errCh captures startup errors from the server goroutine so we can
	// fail fast instead of blocking on ctx.Done() with a dead listener.
	errCh := make(chan error, 1)

	if s.DisableTLS {
		log.Warnf("Starting webhook server in insecure mode (plain HTTP) on port %d", s.Port)

		go func() {
			if err := s.Server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- err
			}
		}()
	} else {
		// Default TLS settings if not explicitly configured
		if s.TLS == nil {
			s.TLS = &TLSConfig{
				CertFile:   DefaultTLSCertPath,
				KeyFile:    DefaultTLSKeyPath,
				MinVersion: DefaultTLSMinVersion,
				MaxVersion: DefaultTLSMaxVersion,
			}
		}
		// Build TLS config from settings
		tlsCfg, err := s.TLS.buildTLSConfig()
		if err != nil {
			return fmt.Errorf("failed to configure TLS: %w", err)
		}

		// Determine whether to load certs from files or generate self-signed
		certFile := s.TLS.CertFile
		keyFile := s.TLS.KeyFile
		if certFilesExist(certFile, keyFile) {
			// Validate cert/key files eagerly so we fail fast on bad certs,
			// and check certificate validity period
			cert, err := tls.LoadX509KeyPair(certFile, keyFile)
			if err != nil {
				return fmt.Errorf("failed to load TLS certificate from %s and %s: %w", certFile, keyFile, err)
			}
			if err := validateCertValidity(cert, certFile); err != nil {
				return err
			}
			log.Infof("Starting webhook server with TLS on port %d (cert: %s, key: %s)", s.Port, certFile, keyFile)
			s.Server.TLSConfig = tlsCfg
			go func() {
				if err := s.Server.ListenAndServeTLS(certFile, keyFile); err != nil && !errors.Is(err, http.ErrServerClosed) {
					errCh <- err
				}
			}()
		} else {
			log.Infof("TLS certificate not found at %s and %s, generating self-signed certificate for this session", certFile, keyFile)
			cert, err := generateSelfSignedCert()
			if err != nil {
				return fmt.Errorf("failed to generate self-signed certificate: %w", err)
			}
			tlsCfg.Certificates = []tls.Certificate{cert}
			s.Server.TLSConfig = tlsCfg
			log.Infof("Starting webhook server with TLS on port %d (using generated self-signed certificate)", s.Port)
			// Pass empty strings since certs are already in TLSConfig.Certificates
			go func() {
				if err := s.Server.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
					errCh <- err
				}
			}()
		}
	}

	// Wait for context cancellation or a startup error
	select {
	case err := <-errCh:
		log.Errorf("Webhook server failed to start: %v", err)
		return fmt.Errorf("webhook server failed to start: %w", err)
	case <-ctx.Done():
	}

	// Graceful shutdown — use context.Background() because ctx is already
	// cancelled (we reached here via <-ctx.Done()).
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	log.Infof("Shutting down webhook server")
	return s.Server.Shutdown(shutdownCtx)
}

// Stop stops the webhook server
func (s *WebhookServer) Stop(ctx context.Context) error {
	log := log.LoggerFromContext(ctx)
	log.Infof("Stopping webhook server")
	return s.Server.Close()
}

// handleHealth handles health check requests
func (s *WebhookServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	webhookLogger := log.Log().WithFields(logrus.Fields{
		"logger": "webhook",
	})
	ctx := log.ContextWithLogger(r.Context(), webhookLogger)
	baseLogger := log.LoggerFromContext(ctx).
		WithField("webhook_remote", r.RemoteAddr)

	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("OK")); err != nil {
		baseLogger.Errorf("Failed to write health check response: %v", err)
	}
}

// handleWebhook handles webhook requests
func (s *WebhookServer) handleWebhook(w http.ResponseWriter, r *http.Request) {
	webhookLogger := log.Log().WithFields(logrus.Fields{
		"logger": "webhook",
	})
	ctx := log.ContextWithLogger(r.Context(), webhookLogger)
	baseLogger := log.LoggerFromContext(ctx).
		WithField("webhook_remote", r.RemoteAddr)
	baseLogger.Debugf("Received webhook request from %s", r.RemoteAddr)
	r.Body = http.MaxBytesReader(w, r.Body, maxWebhookBodySize)

	event, err := s.Handler.ProcessWebhook(r)
	if err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			baseLogger.Warnf("Webhook request body too large (limit: %d bytes)", maxWebhookBodySize)
			http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
			return
		}
		baseLogger.Errorf("Failed to process webhook: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	fields := logrus.Fields{
		"webhook_registry":   event.RegistryURL,
		"webhook_repository": event.Repository,
		"webhook_tag":        event.Tag,
	}
	eventCtx := baseLogger.WithFields(fields)
	eventOpCtx := log.ContextWithLogger(ctx, eventCtx)

	eventCtx.Infof("Received valid webhook event")

	// Process webhook asynchronously
	go func() {
		if s.RateLimiter != nil {
			s.RateLimiter.Take()
		}

		err := s.processWebhookEvent(eventOpCtx, event)
		if err != nil {
			eventCtx.Errorf("Failed to process webhook event: %v", err)
		}
	}()

	// Return success immediately
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("Webhook received and processing")); err != nil {
		eventCtx.Errorf("Failed to write webhook response: %v", err)
	}
}

// processWebhookEvent processes a webhook event and triggers image update checks
func (s *WebhookServer) processWebhookEvent(ctx context.Context, event *argocd.WebhookEvent) error {
	logCtx := log.LoggerFromContext(ctx)
	// The request's context is canceled as soon as the HTTP handler returns.
	// We create a new background context for our asynchronous processing to
	// prevent it from being prematurely terminated.
	processingCtx := log.ContextWithLogger(context.Background(), logCtx)
	logCtx.Infof("Processing webhook event for %s/%s:%s", event.RegistryURL, event.Repository, event.Tag)

	imageList := &api.ImageUpdaterList{}

	logCtx.Debugf("Listing all ImageUpdater CRs...")
	if err := s.Reconciler.List(processingCtx, imageList); err != nil {
		logCtx.Errorf("Failed to list ImageUpdater CRs: %v", err)
		return err
	}

	logCtx.Debugf("Found %d ImageUpdater CRs to process.", len(imageList.Items))

	if err := s.Reconciler.ProcessImageUpdaterCRs(processingCtx, imageList.Items, false, event); err != nil {
		logCtx.Errorf("Failed to process ImageUpdater CRs for webhook: %v", err)
		return err
	}

	return nil
}
