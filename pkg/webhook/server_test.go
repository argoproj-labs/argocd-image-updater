package webhook

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	imageupdaterapi "github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"
	"github.com/argoproj-labs/argocd-image-updater/internal/controller"
	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd"
)

// mockRateLimiter records whether Take was called. handleWebhook calls Take
// from a separate goroutine, so the flag must be safe for concurrent access.
type mockRateLimiter struct {
	called atomic.Bool
}

func (m *mockRateLimiter) Take() time.Time {
	m.called.Store(true)
	return time.Now()
}

// Helper function to create a mock reconciler for testing
func createMockReconciler(t *testing.T) *controller.ImageUpdaterReconciler {
	s := runtime.NewScheme()
	err := imageupdaterapi.AddToScheme(s)
	assert.NoError(t, err)
	err = v1alpha1.AddToScheme(s)
	assert.NoError(t, err)

	cl := fake.NewClientBuilder().WithScheme(s).Build()

	return &controller.ImageUpdaterReconciler{
		Client: cl,
		Scheme: s,
		Config: &controller.ImageUpdaterConfig{},
	}
}

// Helper function to create a mock server
func createMockServer(t *testing.T, port int) *WebhookServer {
	handler := NewWebhookHandler()
	reconciler := createMockReconciler(t)
	server := NewWebhookServer(port, handler, reconciler)
	assert.NotNil(t, server, "Mock server created is nil")
	return server
}

// Helper function to wait till server is started
func waitForServerToStart(url string, timeout time.Duration) error {
	client := &http.Client{Timeout: 1 * time.Second}
	duration := time.Now().Add(timeout)

	for time.Now().Before(duration) {
		resp, err := client.Get(url)
		if err == nil {
			resp.Body.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("Server did not start in time.")
}

// Helper function to test connectivity of an endpoint
func testEndpointConnectivity(t *testing.T, url string, expectedStatus int) {
	client := http.Client{Timeout: 5 * time.Second}

	res, err := client.Get(url)
	if res != nil {
		assert.Equal(t, expectedStatus, res.StatusCode, "Did not receive the expected status of %d got: %d", expectedStatus, res.StatusCode)
		defer res.Body.Close()
	}
	assert.NotNil(t, res, "No body received so server is not alive")
	assert.NoError(t, err)
}

// TestNewWebhookServer ensures that WebhookServer struct is inited properly
func TestNewWebhookServer(t *testing.T) {
	handler := NewWebhookHandler()
	reconciler := createMockReconciler(t)
	server := NewWebhookServer(8080, handler, reconciler)

	assert.NotNil(t, server, "Server was nil")
	assert.Equal(t, 8080, server.Port, "Port is not 8080 got %d", server.Port)
	assert.Equal(t, handler, server.Handler, "Handler is not equal")
	assert.NotNil(t, server.Reconciler, "Reconciler was nil")

}

// TestWebhookServerStart ensures that the server is created with the correct endpoints
func TestWebhookServerStart(t *testing.T) {
	server := createMockServer(t, 8080)
	server.DisableTLS = true
	go func() {
		err := server.Start(context.Background())
		if err != http.ErrServerClosed {
			assert.NoError(t, err, "Start returned error: %s", err.Error())
		}
	}()

	address := fmt.Sprintf("http://localhost:%d/", server.Port)
	err := waitForServerToStart(address+"webhook", 5*time.Second)
	assert.NoError(t, err, "Server failed to start")
	defer server.Server.Close()

	testEndpointConnectivity(t, address+"webhook", http.StatusBadRequest)
	testEndpointConnectivity(t, address+"healthz", http.StatusOK)
}

// TestWebhookServerStop ensures that the server is stopped properly
func TestWebhookServerStop(t *testing.T) {
	// Use a unique port to avoid conflicts with other tests running in parallel
	server := createMockServer(t, 8081)
	server.DisableTLS = true
	errorChannel := make(chan error)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		err := server.Start(ctx)
		errorChannel <- err
	}()

	address := fmt.Sprintf("http://localhost:%d/", server.Port)
	err := waitForServerToStart(address+"webhook", 5*time.Second)
	assert.NoError(t, err, "Server failed to start")

	testEndpointConnectivity(t, address+"webhook", http.StatusBadRequest)

	cancel()

	select {
	case err := <-errorChannel:
		assert.NoError(t, err, "Server shutdown with error: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("Server did not shut down properly")
	}

	// Give the server a moment to fully close the connection
	time.Sleep(200 * time.Millisecond)

	// Try to connect - should fail since server is down
	client := http.Client{Timeout: 500 * time.Millisecond}
	_, err = client.Get(address + "webhook")
	assert.NotNil(t, err, "Connecting to endpoint did not return error, server did not shut down properly")
}

// TestWebhookServerHandleHealth tests the health handler
func TestWebhookServerHandleHealth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	server := createMockServer(t, 8080)
	server.handleHealth(rec, req)

	res := rec.Result()
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	assert.NoError(t, err, "Error while parsing body")

	assert.Equal(t, http.StatusOK, res.StatusCode, "Did not receive the correct status code got: %d", res.StatusCode)
	assert.Equal(t, "OK", string(body), "Did not receive the correct health message")
}

// TestWebhookServerHealthEndpoint ensures that the health endpoint of the server is working properly
func TestWebhookServerHealthEndpoint(t *testing.T) {
	server := createMockServer(t, 8080)
	server.DisableTLS = true
	ctx := context.Background()
	go func() {
		err := server.Start(ctx)
		if err != http.ErrServerClosed {
			assert.NoError(t, err, "Start returned error: %s", err.Error())
		}
	}()

	address := fmt.Sprintf("http://localhost:%d/", server.Port)
	err := waitForServerToStart(address, 5*time.Second)
	assert.NoError(t, err, "Server failed to start")
	defer server.Server.Close()

	client := http.Client{Timeout: 3 * time.Second}
	res, err := client.Get(address + "healthz")
	assert.NoError(t, err)
	assert.NotNil(t, res, "Response received was nil")
	if res != nil {
		defer res.Body.Close()

		body, err := io.ReadAll(res.Body)
		assert.NoError(t, err)
		assert.Equal(t, "OK", string(body), "Did not receive 'OK' got: %s", string(body))
		assert.Equal(t, http.StatusOK, res.StatusCode, "Did not receive status 200 got: %d", res.StatusCode)
	}
}

// TestWebhookServerHandleWebhook tests the webhook handler
func TestWebhookServerHandleWebhook(t *testing.T) {
	server := createMockServer(t, 8080)

	handler := NewDockerHubWebhook("")
	assert.NotNil(t, handler, "Docker handler was nil")

	server.Handler.RegisterHandler(handler)

	tests := []struct {
		name           string
		handler        string
		body           []byte
		expectedStatus int
	}{
		{
			name:    "Valid webhook payload",
			handler: "docker.io",
			body: []byte(`{
				"repository": {
					"repo_name": "somepersononthisfakeregistry/myimagethatdoescoolstuff",
					"name": "myimagethatdoescoolstuff",
					"namespace": "randomplaceincluster"
				},
				"push_data": {
					"tag": "v12.0.9"
				}
			}`),
			expectedStatus: http.StatusOK,
		},
		{
			name:           "Invalid webhook payload",
			handler:        "notarealregistry",
			body:           []byte(`{}`),
			expectedStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/webhook?type=%s", tt.handler), bytes.NewReader(tt.body))
			rec := httptest.NewRecorder()

			server.handleWebhook(rec, req)

			res := rec.Result()
			defer res.Body.Close()

			assert.Equal(t, tt.expectedStatus, res.StatusCode, "Did not receive ok status")
		})
	}

}

func TestWebhookServerHandleWebhookOversizedBody(t *testing.T) {
	server := createMockServer(t, 8080)

	handler := NewDockerHubWebhook("")
	assert.NotNil(t, handler, "Docker handler was nil")

	server.Handler.RegisterHandler(handler)

	t.Run("Oversized webhook payload", func(t *testing.T) {
		padding := strings.Repeat("x", maxWebhookBodySize+1)
		body := fmt.Sprintf("{\"padding\":\"%s\"}", padding)

		req := httptest.NewRequest(http.MethodPost, "/webhook?type=docker.io", strings.NewReader(body))
		rec := httptest.NewRecorder()

		server.handleWebhook(rec, req)

		res := rec.Result()
		defer res.Body.Close()

		responseBody, err := io.ReadAll(res.Body)
		assert.NoError(t, err, "Error while parsing body")
		assert.Equal(t, http.StatusRequestEntityTooLarge, res.StatusCode, "Did not receive expected status")
		assert.Contains(t, string(responseBody), "request body too large", "Did not receive expected error message")
	})

	t.Run("Normal webhook payload still accepted", func(t *testing.T) {
		body := []byte(`{
			"repository": {
				"repo_name": "somepersononthisfakeregistry/myimagethatdoescoolstuff",
				"name": "myimagethatdoescoolstuff",
				"namespace": "randomplaceincluster"
			},
			"push_data": {
				"tag": "v12.0.9"
			}
		}`)

		req := httptest.NewRequest(http.MethodPost, "/webhook?type=docker.io", bytes.NewReader(body))
		rec := httptest.NewRecorder()

		server.handleWebhook(rec, req)

		res := rec.Result()
		defer res.Body.Close()

		assert.Equal(t, http.StatusOK, res.StatusCode, "Did not receive ok status")
	})
}

// TestProcessWebhookEvent tests the processWebhookEvent helper function
func TestProcessWebhookEvent(t *testing.T) {
	var wg sync.WaitGroup
	wg.Go(func() {
		server := createMockServer(t, 8080)

		event := &argocd.WebhookEvent{
			RegistryURL: "",
			Repository:  "nginx",
			Tag:         "1.21.0",
			Digest:      "sha256:thisisatestingsha256value",
		}

		err := server.processWebhookEvent(context.Background(), event)
		assert.NoError(t, err)

	})
	wg.Wait()
}

// TestWebhookServerWebhookEndpoint ensures that the webhook endpoint of the server is working properly
func TestWebhookServerWebhookEndpoint(t *testing.T) {
	server := createMockServer(t, 8080)
	server.DisableTLS = true
	ctx := context.Background()

	handler := NewDockerHubWebhook("")
	assert.NotNil(t, handler, "Docker handler was nil")

	server.Handler.RegisterHandler(handler)

	go func() {
		err := server.Start(ctx)
		if err != http.ErrServerClosed {
			assert.NoError(t, err, "Start returned error: %s", err.Error())
		}
	}()

	address := fmt.Sprintf("http://localhost:%d/", server.Port)
	err := waitForServerToStart(address, 5*time.Second)
	assert.NoError(t, err, "Server failed to start")
	defer server.Server.Close()

	body := `{
				"repository": {
					"repo_name": "somepersononthisfakeregistry/myimagethatdoescoolstuff",
					"name": "myimagethatdoescoolstuff",
					"namespace": "randomplaceincluster"
				},
				"push_data": {
					"tag": "v12.0.9"
				}
			}`

	client := http.Client{Timeout: 3 * time.Second}
	res, err := client.Post(address+"webhook?type=docker.io", "application/json", bytes.NewReader([]byte(body)))
	assert.NoError(t, err)
	assert.NotNil(t, res, "Response received was nil")
	if res != nil {
		defer res.Body.Close()

		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, res.StatusCode, "Did not receive status 200 got: %d", res.StatusCode)
	}

	body2 := `{}`

	res2, err := client.Post(address+"webhook?type=notarealregistry", "application/json", bytes.NewReader([]byte(body2)))
	assert.NoError(t, err)
	assert.NotNil(t, res2, "Response received was nil")
	if res2 != nil {
		defer res2.Body.Close()

		assert.NoError(t, err)
		assert.Equal(t, http.StatusBadRequest, res2.StatusCode, "Did not receive status 400 got: %d", res2.StatusCode)
	}
}

// TestWebhookServerRateLimit tests to see if the webhook endpoint's rate limiting functionality works
func TestWebhookServerRateLimit(t *testing.T) {
	server := createMockServer(t, 8080)

	handler := NewDockerHubWebhook("")
	assert.NotNil(t, handler, "Docker handler was nil")

	server.Handler.RegisterHandler(handler)

	mock := &mockRateLimiter{}
	server.RateLimiter = mock

	body := []byte(`{
		"repository": {
			"repo_name": "somepersononthisfakeregistry/myimagethatdoescoolstuff",
			"name": "myimagethatdoescoolstuff",
			"namespace": "randomplaceincluster"
		},
		"push_data": {
			"tag": "v12.0.9"
		}
	}`)

	req := httptest.NewRequest(http.MethodPost, "/webhook?type=docker.io", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	server.handleWebhook(rec, req)

	// Take is called from the goroutine handleWebhook starts.
	assert.Eventually(t, mock.called.Load, 5*time.Second, 10*time.Millisecond, "Take was not called")
}

func TestParseTLSVersion(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  uint16
		expectErr bool
	}{
		{"empty string returns zero", "", 0, false},
		{"1.0 is not supported", "1.0", 0, true},
		{"1.1", "1.1", tls.VersionTLS11, false},
		{"1.2", "1.2", tls.VersionTLS12, false},
		{"1.3", "1.3", tls.VersionTLS13, false},
		{"TLS1.2 uppercase", "TLS1.2", tls.VersionTLS12, false},
		{"tls1.3 lowercase", "tls1.3", tls.VersionTLS13, false},
		{"with whitespace", " 1.2 ", tls.VersionTLS12, false},
		{"invalid version", "1.4", 0, true},
		{"garbage", "foo", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseTLSVersion(tt.input)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestParseTLSCiphers(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expectNil bool
		expectLen int
		expectErr bool
	}{
		{"empty string returns nil", "", true, 0, false},
		{"single valid cipher", "TLS_AES_128_GCM_SHA256", false, 1, false},
		{"multiple valid ciphers colon-separated", "TLS_AES_128_GCM_SHA256:TLS_AES_256_GCM_SHA384", false, 2, false},
		{"with whitespace", " TLS_AES_128_GCM_SHA256 : TLS_AES_256_GCM_SHA384 ", false, 2, false},
		{"invalid cipher", "NOT_A_REAL_CIPHER", false, 0, true},
		{"trailing colon ignored", "TLS_AES_128_GCM_SHA256:", false, 1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseTLSCiphers(tt.input)
			if tt.expectErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				if tt.expectNil {
					assert.Nil(t, result)
				} else {
					assert.Len(t, result, tt.expectLen)
				}
			}
		})
	}
}

func TestBuildTLSConfig(t *testing.T) {
	t.Run("default config", func(t *testing.T) {
		cfg := &TLSConfig{}
		tlsCfg, err := cfg.buildTLSConfig(context.Background())
		require.NoError(t, err)
		assert.Equal(t, uint16(0), tlsCfg.MinVersion)
		assert.Equal(t, uint16(0), tlsCfg.MaxVersion)
		assert.Nil(t, tlsCfg.CipherSuites)
		assert.Equal(t, []string{"http/1.1"}, tlsCfg.NextProtos)
	})

	t.Run("with min and max version", func(t *testing.T) {
		cfg := &TLSConfig{
			MinVersion: "1.2",
			MaxVersion: "1.3",
		}
		tlsCfg, err := cfg.buildTLSConfig(context.Background())
		require.NoError(t, err)
		assert.Equal(t, uint16(tls.VersionTLS12), tlsCfg.MinVersion)
		assert.Equal(t, uint16(tls.VersionTLS13), tlsCfg.MaxVersion)
	})

	t.Run("min equals max is valid", func(t *testing.T) {
		cfg := &TLSConfig{
			MinVersion: "1.3",
			MaxVersion: "1.3",
		}
		tlsCfg, err := cfg.buildTLSConfig(context.Background())
		require.NoError(t, err)
		assert.Equal(t, uint16(tls.VersionTLS13), tlsCfg.MinVersion)
		assert.Equal(t, uint16(tls.VersionTLS13), tlsCfg.MaxVersion)
	})

	t.Run("min greater than max is invalid", func(t *testing.T) {
		cfg := &TLSConfig{
			MinVersion: "1.3",
			MaxVersion: "1.2",
		}
		_, err := cfg.buildTLSConfig(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "minimum TLS 1.3 cannot be higher than maximum TLS 1.2")
	})

	t.Run("unset minimum leaves the maximum unbounded above", func(t *testing.T) {
		_, err := (&TLSConfig{MinVersion: "1.3"}).buildTLSConfig(context.Background())
		assert.NoError(t, err)
	})

	t.Run("maximum below the crypto/tls default minimum is invalid", func(t *testing.T) {
		// crypto/tls floors a server at TLS 1.2 when MinVersion is unset, so
		// this range is empty and every handshake would fail.
		_, err := (&TLSConfig{MaxVersion: "1.1"}).buildTLSConfig(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "maximum TLS 1.1 is below TLS 1.2")
		assert.Contains(t, err.Error(), "set --tlsminversion explicitly")
	})

	t.Run("explicit minimum opens up a low maximum", func(t *testing.T) {
		// The escape hatch the error above points at: naming the minimum
		// explicitly makes crypto/tls offer TLS 1.1.
		tlsCfg, err := (&TLSConfig{MinVersion: "1.1", MaxVersion: "1.1"}).buildTLSConfig(context.Background())
		require.NoError(t, err)
		assert.Equal(t, uint16(tls.VersionTLS11), tlsCfg.MinVersion)
		assert.Equal(t, uint16(tls.VersionTLS11), tlsCfg.MaxVersion)
	})

	t.Run("invalid min version", func(t *testing.T) {
		cfg := &TLSConfig{
			MinVersion: "invalid",
		}
		_, err := cfg.buildTLSConfig(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "--tlsminversion")
	})

	t.Run("min version 1.0 is clamped instead of fatal", func(t *testing.T) {
		// #1850: a cluster TLS policy naming a 1.0 floor must not stop the
		// server from starting. We still never negotiate TLS 1.0.
		cfg := &TLSConfig{
			MinVersion: "1.0",
			MaxVersion: "1.3",
		}
		tlsCfg, err := cfg.buildTLSConfig(context.Background())
		require.NoError(t, err)
		assert.Equal(t, uint16(tls.VersionTLS12), tlsCfg.MinVersion)
	})

	t.Run("max version 1.0 is still rejected", func(t *testing.T) {
		// Unlike a minimum, a maximum of 1.0 cannot be satisfied by clamping:
		// it asks us to cap at a version we never speak.
		cfg := &TLSConfig{
			MaxVersion: "1.0",
		}
		_, err := cfg.buildTLSConfig(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "--tlsmaxversion")
	})

	t.Run("invalid max version", func(t *testing.T) {
		cfg := &TLSConfig{
			MaxVersion: "invalid",
		}
		_, err := cfg.buildTLSConfig(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "--tlsmaxversion")
	})

	t.Run("invalid cipher", func(t *testing.T) {
		cfg := &TLSConfig{
			Ciphers: "NOT_REAL",
		}
		_, err := cfg.buildTLSConfig(context.Background())
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "--tlsciphers")
	})

	t.Run("with valid ciphers colon-separated", func(t *testing.T) {
		cfg := &TLSConfig{
			MinVersion: "1.2",
			Ciphers:    "TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384:TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
		}
		tlsCfg, err := cfg.buildTLSConfig(context.Background())
		require.NoError(t, err)
		assert.Len(t, tlsCfg.CipherSuites, 2)
	})

	t.Run("TLS 1.3 cipher names are dropped when min version is below 1.3", func(t *testing.T) {
		cfg := &TLSConfig{
			MinVersion: "1.2",
			Ciphers:    "TLS_AES_128_GCM_SHA256:TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
		}
		tlsCfg, err := cfg.buildTLSConfig(context.Background())
		require.NoError(t, err)
		// Go ignores TLS 1.3 suites in CipherSuites, so they never reach validation.
		assert.Equal(t, []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256}, tlsCfg.CipherSuites)
	})

	t.Run("only TLS 1.3 cipher names with min version below 1.3 falls back to defaults", func(t *testing.T) {
		cfg := &TLSConfig{
			MinVersion: "1.2",
			Ciphers:    "TLS_AES_128_GCM_SHA256:TLS_AES_256_GCM_SHA384",
		}
		tlsCfg, err := cfg.buildTLSConfig(context.Background())
		require.NoError(t, err)
		assert.Empty(t, tlsCfg.CipherSuites)
	})

	t.Run("low min version with TLS 1.2 cipher is accepted", func(t *testing.T) {
		// Regression test for #1850: --tlsminversion 1.1 with a TLS 1.2-only
		// cipher is a valid configuration — the suite is simply offered when
		// TLS 1.2 is negotiated.
		cfg := &TLSConfig{
			MinVersion: "1.1",
			MaxVersion: "1.3",
			Ciphers:    "TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256",
		}
		tlsCfg, err := cfg.buildTLSConfig(context.Background())
		require.NoError(t, err)
		assert.Equal(t, []uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256}, tlsCfg.CipherSuites)
	})

	t.Run("http2 can be enabled explicitly", func(t *testing.T) {
		cfg := &TLSConfig{
			EnableHTTP2: true,
		}
		tlsCfg, err := cfg.buildTLSConfig(context.Background())
		require.NoError(t, err)
		assert.Empty(t, tlsCfg.NextProtos)
	})

	t.Run("http2 disabled explicitly sets NextProtos to http/1.1 only", func(t *testing.T) {
		cfg := &TLSConfig{EnableHTTP2: false}
		tlsCfg, err := cfg.buildTLSConfig(context.Background())
		require.NoError(t, err)
		assert.Equal(t, []string{"http/1.1"}, tlsCfg.NextProtos,
			"NextProtos must be exactly [http/1.1] so the server never advertises h2")
	})

	t.Run("http2 disabled does not advertise h2 via ALPN", func(t *testing.T) {
		cfg := &TLSConfig{EnableHTTP2: false}
		tlsCfg, err := cfg.buildTLSConfig(context.Background())
		require.NoError(t, err)
		assert.NotContains(t, tlsCfg.NextProtos, "h2",
			"h2 must not appear in NextProtos when HTTP/2 is disabled")
	})

	t.Run("http2 enabled leaves NextProtos nil", func(t *testing.T) {
		cfg := &TLSConfig{EnableHTTP2: true}
		tlsCfg, err := cfg.buildTLSConfig(context.Background())
		require.NoError(t, err)
		assert.Nil(t, tlsCfg.NextProtos,
			"NextProtos must be nil when EnableHTTP2 is true so net/http can manage ALPN freely")
	})
}

func TestParseTLSMinVersion(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		expected  uint16
		expectErr bool
	}{
		{"empty means default", "", 0, false},
		{"1.1", "1.1", tls.VersionTLS11, false},
		{"1.2", "1.2", tls.VersionTLS12, false},
		{"1.3", "TLS1.3", tls.VersionTLS13, false},
		// #1850: a configured minimum of 1.0 is clamped up, not fatal.
		{"1.0 is clamped to 1.2", "1.0", tls.VersionTLS12, false},
		{"tls1.0 is clamped to 1.2", "TLS1.0", tls.VersionTLS12, false},
		{"1.0 with whitespace is clamped to 1.2", " 1.0 ", tls.VersionTLS12, false},
		{"still rejects nonsense", "1.4", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := parseTLSMinVersion(context.Background(), tt.input)
			if tt.expectErr {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expected, v)
		})
	}

	t.Run("1.0 is never negotiable", func(t *testing.T) {
		// Clamping the minimum must not make TLS 1.0 reachable anywhere else.
		_, err := ParseTLSVersion("1.0")
		assert.Error(t, err)
		v, err := parseTLSMinVersion(context.Background(), "1.0")
		require.NoError(t, err)
		assert.Greater(t, v, uint16(tls.VersionTLS10))
	})
}

func TestDropTLS13Ciphers(t *testing.T) {
	tests := []struct {
		name            string
		input           []uint16
		expectedKept    []uint16
		expectedDropped []string
	}{
		{"empty input", nil, nil, nil},
		{
			"TLS 1.2 suites are kept",
			[]uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256},
			[]uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256},
			nil,
		},
		{
			"TLS 1.3 suites are dropped",
			[]uint16{tls.TLS_AES_128_GCM_SHA256, tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256},
			[]uint16{tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256},
			[]string{"TLS_AES_128_GCM_SHA256"},
		},
		{
			"all TLS 1.3 suites",
			[]uint16{tls.TLS_AES_128_GCM_SHA256, tls.TLS_AES_256_GCM_SHA384},
			nil,
			[]string{"TLS_AES_128_GCM_SHA256", "TLS_AES_256_GCM_SHA384"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kept, dropped := dropTLS13Ciphers(tt.input)
			assert.Equal(t, tt.expectedKept, kept)
			assert.Equal(t, tt.expectedDropped, dropped)
		})
	}
}

func TestInsecureCipherRejected(t *testing.T) {
	// Pick an insecure cipher name
	insecureCiphers := tls.InsecureCipherSuites()
	if len(insecureCiphers) > 0 {
		_, err := ParseTLSCiphers(insecureCiphers[0].Name)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported TLS cipher suite")
	}
}

func TestBuildTLSConfigKeepsEveryTLS12Cipher(t *testing.T) {
	// Every TLS 1.1/1.2 suite Go considers secure must survive buildTLSConfig
	// unchanged, at any supported minimum version. Cross-checking suites
	// against the version range is crypto/tls's job, not ours (#1850).
	for _, cs := range tls.CipherSuites() {
		if !slices.ContainsFunc(cs.SupportedVersions, func(v uint16) bool { return v < tls.VersionTLS13 }) {
			continue
		}
		for _, minVer := range []string{"", "1.1", "1.2"} {
			t.Run(fmt.Sprintf("%s/min=%q", cs.Name, minVer), func(t *testing.T) {
				cfg := &TLSConfig{
					MinVersion: minVer,
					MaxVersion: "1.3",
					Ciphers:    cs.Name,
				}
				tlsCfg, err := cfg.buildTLSConfig(context.Background())
				require.NoError(t, err)
				assert.Equal(t, []uint16{cs.ID}, tlsCfg.CipherSuites)
			})
		}
	}
}

func TestBuildTLSConfigCiphersIgnoredForTLS13Only(t *testing.T) {
	cfg := &TLSConfig{
		MinVersion: "1.3",
		MaxVersion: "1.3",
		Ciphers:    "TLS_AES_128_GCM_SHA256",
	}
	tlsCfg, err := cfg.buildTLSConfig(context.Background())
	require.NoError(t, err)
	// Ciphers should be silently dropped (warning logged) since TLS 1.3
	// cipher suites are not configurable in Go.
	assert.Nil(t, tlsCfg.CipherSuites)
}

func TestGenerateSelfSignedCert(t *testing.T) {
	cert, err := generateSelfSignedCert()
	require.NoError(t, err)
	assert.NotEmpty(t, cert.Certificate, "certificate should not be empty")
	assert.NotNil(t, cert.PrivateKey, "private key should not be nil")
}

func TestCertFilesExist(t *testing.T) {
	t.Run("nonexistent files", func(t *testing.T) {
		assert.False(t, certFilesExist("/nonexistent/cert.pem", "/nonexistent/key.pem"))
	})

	t.Run("partial files", func(t *testing.T) {
		// Create a temp file for cert only
		tmpFile, err := os.CreateTemp("", "cert-*.pem")
		require.NoError(t, err)
		defer os.Remove(tmpFile.Name())
		_, _ = tmpFile.WriteString("some content")
		tmpFile.Close()

		assert.False(t, certFilesExist(tmpFile.Name(), "/nonexistent/key.pem"))
	})

	t.Run("zero-byte files treated as absent", func(t *testing.T) {
		certFile, err := os.CreateTemp("", "cert-*.pem")
		require.NoError(t, err)
		defer os.Remove(certFile.Name())
		certFile.Close()

		keyFile, err := os.CreateTemp("", "key-*.pem")
		require.NoError(t, err)
		defer os.Remove(keyFile.Name())
		keyFile.Close()

		assert.False(t, certFilesExist(certFile.Name(), keyFile.Name()))
	})
}

func TestWebhookServerStartWithTLS(t *testing.T) {
	server := createMockServer(t, 8083)
	server.TLS = &TLSConfig{
		CertFile:   "/nonexistent/cert.pem",
		KeyFile:    "/nonexistent/key.pem",
		MinVersion: DefaultTLSMinVersion,
		MaxVersion: DefaultTLSMaxVersion,
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start(ctx)
	}()

	// Server should start with self-signed cert since files don't exist
	// Wait for it to start by polling the HTTPS endpoint
	tlsClient := &http.Client{
		Timeout: 1 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // test only
		},
	}

	address := fmt.Sprintf("https://localhost:%d/healthz", server.Port)
	var lastErr error
	for range 50 {
		resp, err := tlsClient.Get(address)
		if err == nil {
			resp.Body.Close()
			assert.Equal(t, http.StatusOK, resp.StatusCode)
			lastErr = nil
			break
		}
		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}
	assert.NoError(t, lastErr, "Server with self-signed TLS did not start in time")

	cancel()
	<-errCh
}

// TestWebhookServerTLSNextProtoDisablesHTTP2 is an end-to-end regression test
// that verifies HTTP/2 is fully disabled when EnableHTTP2 is false. It starts
// a TLS server, attempts ALPN negotiation offering both h2 and http/1.1, and
// asserts that the server selects http/1.1 — confirming that neither ALPN
// advertisement nor net/http's automatic h2 handler registration is active.
func TestWebhookServerTLSNextProtoDisablesHTTP2(t *testing.T) {
	server := createMockServer(t, 8085)
	server.TLS = &TLSConfig{
		CertFile:    "/nonexistent/cert.pem",
		KeyFile:     "/nonexistent/key.pem",
		MinVersion:  DefaultTLSMinVersion,
		MaxVersion:  DefaultTLSMaxVersion,
		EnableHTTP2: false,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Start(ctx)
	}()

	// Wait for server to be ready before connecting
	tlsClient := &http.Client{
		Timeout: 1 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}, //nolint:gosec // test only
		},
	}
	address := fmt.Sprintf("https://localhost:%d/healthz", server.Port)
	var lastErr error
	for range 50 {
		resp, err := tlsClient.Get(address)
		if err == nil {
			resp.Body.Close()
			lastErr = nil
			break
		}
		lastErr = err
		time.Sleep(100 * time.Millisecond)
	}
	require.NoError(t, lastErr, "TLS server did not start in time")

	// Attempt ALPN negotiation offering h2 first — the server must reject it
	// and fall back to http/1.1 because EnableHTTP2 is false.
	conn, err := tls.Dial("tcp", fmt.Sprintf("localhost:%d", server.Port), &tls.Config{
		InsecureSkipVerify: true,                       //nolint:gosec // test only
		NextProtos:         []string{"h2", "http/1.1"}, // offer h2 first
	})
	require.NoError(t, err, "failed to connect to TLS server")
	defer conn.Close()

	negotiated := conn.ConnectionState().NegotiatedProtocol
	assert.Equal(t, "http/1.1", negotiated,
		"expected http/1.1 to be negotiated when EnableHTTP2 is false, got %q", negotiated)

	cancel()
	<-errCh
}

func TestWebhookServerStartWithCorruptCert(t *testing.T) {
	// Create temp files with invalid cert/key content
	certFile, err := os.CreateTemp("", "bad-cert-*.pem")
	require.NoError(t, err)
	defer os.Remove(certFile.Name())
	_, _ = certFile.WriteString("not a valid certificate")
	certFile.Close()

	keyFile, err := os.CreateTemp("", "bad-key-*.pem")
	require.NoError(t, err)
	defer os.Remove(keyFile.Name())
	_, _ = keyFile.WriteString("not a valid key")
	keyFile.Close()

	server := createMockServer(t, 8084)
	server.TLS = &TLSConfig{
		CertFile:   certFile.Name(),
		KeyFile:    keyFile.Name(),
		MinVersion: DefaultTLSMinVersion,
		MaxVersion: DefaultTLSMaxVersion,
	}

	// Start should return an error immediately for corrupt certs
	err = server.Start(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to load TLS certificate")
}
