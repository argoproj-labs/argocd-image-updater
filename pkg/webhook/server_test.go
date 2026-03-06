package webhook

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	imageupdaterapi "github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"
	"github.com/argoproj-labs/argocd-image-updater/internal/controller"
	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd"
)

type mockRateLimiter struct {
	Called bool
}

func (m *mockRateLimiter) Take() time.Time {
	m.Called = true
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
		assert.Equal(t, res.StatusCode, expectedStatus, "Did not receive the expected status of %d got: %d", expectedStatus, res.StatusCode)
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
	assert.Equal(t, server.Port, 8080, "Port is not 8080 got %d", server.Port)
	assert.Equal(t, server.Handler, handler, "Handler is not equal")
	assert.NotNil(t, server.Reconciler, "Reconciler was nil")

}

// TestWebhookServerStart ensures that the server is created with the correct endpoints
func TestWebhookServerStart(t *testing.T) {
	server := createMockServer(t, 8080)
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

	assert.Equal(t, res.StatusCode, http.StatusOK, "Did not receive the correct status code got: %d", res.StatusCode)
	assert.Equal(t, string(body), "OK", "Did not receive the correct health message")
}

// TestWebhookServerHealthEndpoint ensures that the health endpoint of the server is working properly
func TestWebhookServerHealthEndpoint(t *testing.T) {
	server := createMockServer(t, 8080)
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
		assert.Equal(t, string(body), "OK", "Did not receive 'OK' got: %s", string(body))
		assert.Equal(t, res.StatusCode, http.StatusOK, "Did not receive status 200 got: %d", res.StatusCode)
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

			assert.Equal(t, res.StatusCode, tt.expectedStatus, "Did not receive ok status")
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
	wg.Add(1)
	go func() {
		defer wg.Done()
		server := createMockServer(t, 8080)

		event := &argocd.WebhookEvent{
			RegistryURL: "",
			Repository:  "nginx",
			Tag:         "1.21.0",
			Digest:      "sha256:thisisatestingsha256value",
		}

		err := server.processWebhookEvent(context.Background(), event)
		assert.NoError(t, err)

	}()
	wg.Wait()
}

// TestWebhookServerWebhookEndpoint ensures that the webhook endpoint of the server is working properly
func TestWebhookServerWebhookEndpoint(t *testing.T) {
	server := createMockServer(t, 8080)
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
		assert.Equal(t, res.StatusCode, http.StatusOK, "Did not receive status 200 got: %d", res.StatusCode)
	}

	body2 := `{}`

	res2, err := client.Post(address+"webhook?type=notarealregistry", "application/json", bytes.NewReader([]byte(body2)))
	assert.NoError(t, err)
	assert.NotNil(t, res2, "Response received was nil")
	if res2 != nil {
		defer res2.Body.Close()

		assert.NoError(t, err)
		assert.Equal(t, res2.StatusCode, http.StatusBadRequest, "Did not receive status 400 got: %d", res.StatusCode)
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

	// Wait for thread to call it.
	time.Sleep(time.Second)

	assert.True(t, mock.Called, "Take was not called")
}
