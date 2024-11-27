package health

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// Unit test function
func TestStartHealthServer_InvalidPort(t *testing.T) {
	// Use an invalid port number
	port := -1
	errCh := StartHealthServer(port)
	defer close(errCh) // Close the error channel after the test completes
	select {
	case err := <-errCh:
		if err == nil {
			t.Error("Expected error, got nil")
		} else if err.Error() != fmt.Sprintf("listen tcp: address %d: invalid port", port) {
			t.Errorf("Expected error message about invalid port, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Error("Timed out waiting for error")
	}
}

func TestHealthProbe(t *testing.T) {
	// Create a mock HTTP request
	req, err := http.NewRequest("GET", "/healthz", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Create a mock HTTP response recorder
	w := httptest.NewRecorder()

	// Call the HealthProbe function directly
	HealthProbe(w, req)

	// Check the response status code
	if w.Code != http.StatusOK {
		t.Errorf("Expected status OK; got %d", w.Code)
	}

	// Check the response body
	expectedBody := "OK\n"
	if body := w.Body.String(); body != expectedBody {
		t.Errorf("Expected body %q; got %q", expectedBody, body)
	}
}
