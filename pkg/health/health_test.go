package health

import (
	"net/http"
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
