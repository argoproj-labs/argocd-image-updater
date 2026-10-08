package webhook

import (
	"cmp"
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
	"maps"
	"math/big"
	"net"
	"net/http"
	"os"
	"slices"
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

	// goDefaultTLSMinVersion mirrors the minimum version crypto/tls enforces
	// for a server when tls.Config.MinVersion is left unset. An unset minimum
	// is therefore a floor of its own, not "no floor": a server configured
	// with only a maximum below this rejects every handshake.
	goDefaultTLSMinVersion = tls.VersionTLS12
)

// TLSConfig holds TLS configuration for the server
type TLSConfig struct {
	// CertFile is the path to the TLS certificate file
	CertFile string
	// KeyFile is the path to the TLS private key file
	KeyFile string
	// EnableHTTP2 allows the TLS server to negotiate HTTP/2
	EnableHTTP2 bool
	// MinVersion is the minimum TLS version (e.g. "1.2", "1.3")
	MinVersion string
	// MaxVersion is the maximum TLS version (e.g. "1.2", "1.3")
	MaxVersion string
	// Ciphers is a colon-separated list of TLS cipher suite names
	Ciphers string
	// CurvePreferences is a colon-separated list of allowed TLS key exchange
	// groups (e.g. "X25519:CurveP256"). It selects which groups are enabled;
	// Go ignores list order and chooses from this set using its internal
	// preference order. See crypto/tls.Config.CurvePreferences.
	CurvePreferences string
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

// parseTLSMinVersion parses the configured minimum TLS version.
//
// Unlike ParseTLSVersion it does not fail on TLS 1.0: the minimum is clamped
// up to TLS 1.2 with a warning instead. Clamping a minimum upward can only
// strengthen the connection — TLS 1.0 stays out of tlsVersionMap, so it can
// never be negotiated either way — and a cluster-wide TLS policy that names a
// 1.0 floor (such as OpenShift's built-in "Old" profile) should not leave the
// server unable to start. We clamp to 1.2 rather than 1.1 because RFC 8996
// deprecates both 1.0 and 1.1.
func parseTLSMinVersion(ctx context.Context, version string) (uint16, error) {
	switch strings.ToLower(strings.TrimSpace(version)) {
	case "1.0", "tls1.0":
		log.LoggerFromContext(ctx).Warnf("--tlsminversion %q is not supported (TLS 1.0 is deprecated by RFC 8996), using 1.2 instead", version)
		return tls.VersionTLS12, nil
	}
	return ParseTLSVersion(version)
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
	for name := range strings.SplitSeq(ciphers, ":") {
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

// dropTLS13Ciphers removes cipher suites that can only be negotiated at
// TLS 1.3 from the given list. TLS 1.3 suites are not configurable in Go, so
// listing them in tls.Config.CipherSuites has no effect other than to make the
// configured list differ from the one crypto/tls actually considers. It
// returns the remaining suites plus the names of the ones that were dropped,
// so the caller can tell the admin which entries were ignored.
func dropTLS13Ciphers(cipherSuites []uint16) (kept []uint16, dropped []string) {
	for _, id := range cipherSuites {
		tls13Only := false
		for _, cs := range tls.CipherSuites() {
			if cs.ID != id {
				continue
			}
			tls13Only = len(cs.SupportedVersions) > 0 &&
				!slices.ContainsFunc(cs.SupportedVersions, func(v uint16) bool { return v < tls.VersionTLS13 })
			if tls13Only {
				dropped = append(dropped, cs.Name)
			}
			break
		}
		if !tls13Only {
			kept = append(kept, id)
		}
	}
	return kept, dropped
}

// buildTLSConfig creates a *tls.Config from the TLSConfig settings.
func (t *TLSConfig) buildTLSConfig(ctx context.Context) (*tls.Config, error) {
	log := log.LoggerFromContext(ctx)
	tlsCfg := &tls.Config{} //nolint:gosec // min version is set below from user config

	minVer, err := parseTLSMinVersion(ctx, t.MinVersion)
	if err != nil {
		return nil, fmt.Errorf("invalid --tlsminversion: %w", err)
	}
	tlsCfg.MinVersion = minVer

	maxVer, err := ParseTLSVersion(t.MaxVersion)
	if err != nil {
		return nil, fmt.Errorf("invalid --tlsmaxversion: %w", err)
	}
	tlsCfg.MaxVersion = maxVer

	// An empty range leaves the server with no version it can negotiate at all,
	// so fail fast on it. An unset minimum is not unbounded below: crypto/tls
	// floors a server at goDefaultTLSMinVersion, so "no minimum, maximum 1.1"
	// rejects every handshake just as surely as an inverted range does.
	if maxVer != 0 {
		if effectiveMin := cmp.Or(minVer, uint16(goDefaultTLSMinVersion)); effectiveMin > maxVer {
			if minVer == 0 {
				return nil, fmt.Errorf("maximum %s is below %s, the minimum crypto/tls applies when no minimum is configured; set --tlsminversion explicitly to negotiate below %s",
					tls.VersionName(maxVer), tls.VersionName(goDefaultTLSMinVersion), tls.VersionName(goDefaultTLSMinVersion))
			}
			return nil, fmt.Errorf("minimum %s cannot be higher than maximum %s",
				tls.VersionName(minVer), tls.VersionName(maxVer))
		}
	}

	ciphers, err := ParseTLSCiphers(t.Ciphers)
	if err != nil {
		return nil, fmt.Errorf("invalid --tlsciphers: %w", err)
	}

	// Cipher suites are deliberately not cross-checked against the version
	// range. The minimum version is a floor on the negotiated handshake, not a
	// requirement that every configured suite be usable at that floor: Go
	// applies tls.Config.CipherSuites per negotiated version, so suites that do
	// not apply to the version actually negotiated are simply not offered. That
	// makes "accept TLS 1.1 and above, and use this TLS 1.2 suite whenever 1.2
	// is negotiated" a perfectly ordinary configuration. Beyond rejecting suite
	// names Go itself does not consider secure (see ParseTLSCiphers), suite
	// selection is left to crypto/tls, which knows best what it can negotiate.
	//
	// Go's tls.Config.CipherSuites only applies to TLS 1.1 and 1.2.
	// TLS 1.3 cipher suites are not configurable and are always enabled.
	if len(ciphers) > 0 {
		if minVer >= tls.VersionTLS13 {
			log.Warnf("--tlsciphers has no effect when --tlsminversion is 1.3 or higher (TLS 1.3 cipher suites are not configurable), ignoring")
			ciphers = nil
		} else if kept, dropped := dropTLS13Ciphers(ciphers); len(dropped) > 0 {
			log.Warnf("Ignoring TLS 1.3 cipher suites in --tlsciphers (TLS 1.3 cipher suites are not configurable): %s", strings.Join(dropped, ", "))
			if len(kept) == 0 {
				log.Warnf("No configurable cipher suites left in --tlsciphers, using the Go standard library defaults")
			}
			ciphers = kept
		}
	}
	tlsCfg.CipherSuites = ciphers

	if t.CurvePreferences != "" {
		var curveNames []string
		for name := range strings.SplitSeq(t.CurvePreferences, ":") {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			curveNames = append(curveNames, name)
		}
		curves, err := parseCurvePreferences(curveNames)
		if err != nil {
			return nil, fmt.Errorf("invalid --tlscurvepreferences: %w", err)
		}
		// Hybrid/PQ groups are TLS 1.3-only. Go silently drops them from any
		// lower version, so a hybrid-only allow-list with --tlsmaxversion
		// below 1.3 leaves the server with no usable group.
		if maxVer != 0 && maxVer < tls.VersionTLS13 && !hasClassicalKeyExchange(curves) {
			return nil, fmt.Errorf("--tlscurvepreferences %q lists only TLS 1.3 key exchange groups, but --tlsmaxversion is %s; include a classical group such as X25519 or CurveP256, or raise --tlsmaxversion to 1.3",
				t.CurvePreferences, tls.VersionName(maxVer))
		}
		tlsCfg.CurvePreferences = curves
	}

	if !t.EnableHTTP2 {
		log.Debugf("Disabling HTTP/2 on webhook TLS server")
		tlsCfg.NextProtos = []string{"http/1.1"}
	}

	return tlsCfg, nil
}

// parseCurvePreferences parses curve names into CurveID values for use as an
// allow-list in tls.Config.CurvePreferences. Go ignores the order of that
// slice when negotiating; it selects from the enabled set using its internal
// preference order.
func parseCurvePreferences(names []string) ([]tls.CurveID, error) {
	if len(names) == 0 {
		return nil, nil
	}
	curves := make([]tls.CurveID, 0, len(names))
	for _, name := range names {
		if id, ok := allowedCurveNames[name]; ok {
			curves = append(curves, id)
		} else {
			return nil, fmt.Errorf("unknown curve: %q (supported: %v)", name, slices.Sorted(maps.Keys(allowedCurveNames)))
		}
	}
	return curves, nil
}

// isTLS13OnlyKeyExchange reports whether curve is a hybrid/post-quantum group
// that crypto/tls only offers for TLS 1.3 (mirrors Go's unexported helper).
func isTLS13OnlyKeyExchange(curve tls.CurveID) bool {
	switch curve {
	case tls.X25519MLKEM768, tls.SecP256r1MLKEM768, tls.SecP384r1MLKEM1024, tls.MLKEM1024:
		return true
	default:
		return false
	}
}

// hasClassicalKeyExchange reports whether curves includes at least one group
// usable below TLS 1.3 (X25519 / NIST P-curves).
func hasClassicalKeyExchange(curves []tls.CurveID) bool {
	return slices.ContainsFunc(curves, func(c tls.CurveID) bool {
		return !isTLS13OnlyKeyExchange(c)
	})
}

var allowedCurveNames = func() map[string]tls.CurveID {
	curves := []tls.CurveID{
		tls.X25519MLKEM768,
		tls.SecP256r1MLKEM768,
		tls.SecP384r1MLKEM1024,
		tls.MLKEM1024,
		tls.X25519,
		tls.CurveP256,
		tls.CurveP384,
		tls.CurveP521,
	}

	allowed := make(map[string]tls.CurveID, len(curves))
	for _, curve := range curves {
		allowed[curve.String()] = curve
	}
	return allowed
}()

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
		tlsCfg, err := s.TLS.buildTLSConfig(ctx)
		if err != nil {
			return fmt.Errorf("failed to configure TLS: %w", err)
		}
		if !s.TLS.EnableHTTP2 {
			// Prevent net/http from automatically enabling HTTP/2 for TLS servers.
			// Setting only tls.Config.NextProtos prevents ALPN advertisement, but
			// net/http still registers an h2 handler unless TLSNextProto is
			// explicitly set to a non-nil map.
			log.Debugf("Disabling HTTP/2: setting TLSNextProto to empty map to prevent net/http from auto-registering h2 handler")
			s.Server.TLSNextProto = map[string]func(*http.Server, *tls.Conn, http.Handler){}
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
