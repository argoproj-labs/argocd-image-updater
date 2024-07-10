package health

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Unit test function
func TestStartHealthServer(t *testing.T) {
	port := 8080
	errCh := StartHealthServer(port)
	defer close(errCh) // Close the error channel after the test completes

	// Make a request to the health endpoint
	resp, err := http.Get("http://localhost:8080/healthz")
	if err != nil {
		t.Fatalf("Failed to send GET request: %v", err)
	}
	defer resp.Body.Close()

	// Check if the response status code is 200 OK
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
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
