package webhook

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd"
	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd/mocks"
	"github.com/argoproj-labs/argocd-image-updater/pkg/kube"
	registryKube "github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/kube"
	"github.com/argoproj-labs/argocd-image-updater/test/fake"
)

var (
	mockApps = []v1alpha1.Application{
		{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-appA",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotation: "quay.io/argoprojlabs/argocd-image-updater:1.X.X",
				},
			},
			Spec: v1alpha1.ApplicationSpec{},
			Status: v1alpha1.ApplicationStatus{
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{"quay.io/argoprojlabs/argocd-image-updater:1.16.0"},
				},
			},
		},
		{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-appB",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotation: "localhost/testimage:12.0.X",
				},
			},
			Spec: v1alpha1.ApplicationSpec{},
			Status: v1alpha1.ApplicationStatus{
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{"localhost/testimage:12.0.9"},
				},
			},
		},
		{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-appC",
				Namespace: "argocd",
			},
			Spec: v1alpha1.ApplicationSpec{},
			Status: v1alpha1.ApplicationStatus{
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{"quay.io/centos-bootc/centos-bootc:stream10"},
				},
			},
		},
	}
)

type mockRateLimiter struct {
	Called bool
}

func (m *mockRateLimiter) Take() time.Time {
	m.Called = true
	return time.Now()
}

// Helper function to create a mock server
func createMockServer(t *testing.T, port int) *WebhookServer {
	handler := NewWebhookHandler()
	kubeClient := &kube.ImageUpdaterKubernetesClient{
		KubeClient: &registryKube.KubernetesClient{
			Clientset: fake.NewFakeKubeClient(),
			Namespace: "test",
		},
	}
	argoClient := mocks.NewArgoCD(t)

	server := NewWebhookServer(port, handler, kubeClient, argoClient)
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

	kubeClient := &kube.ImageUpdaterKubernetesClient{
		KubeClient: &registryKube.KubernetesClient{
			Clientset: fake.NewFakeKubeClient(),
			Namespace: "test",
		},
	}

	argoClient := mocks.NewArgoCD(t)

	server := NewWebhookServer(8080, handler, kubeClient, argoClient)

	assert.NotNil(t, server, "Server was nil")
	assert.Equal(t, server.Port, 8080, "Port is not 8080 got %d", server.Port)
	assert.Equal(t, server.Handler, handler, "Handler is not equal")
	assert.Equal(t, server.KubeClient, kubeClient, "KubeClient is not equal")
	assert.Equal(t, server.ArgoClient, argoClient, "ArgoClient is not equal")
}

// TestWebhookServerStart ensures that the server is created with the correct endpoints
func TestWebhookServerStart(t *testing.T) {
	server := createMockServer(t, 8080)
	go func() {
		err := server.Start()
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
	server := createMockServer(t, 8080)
	errorChannel := make(chan error)
	go func() {
		err := server.Start()
		errorChannel <- err
	}()

	address := fmt.Sprintf("http://localhost:%d/", server.Port)
	err := waitForServerToStart(address, 5*time.Second)
	assert.NoError(t, err, "Server failed to start")

	testEndpointConnectivity(t, address+"webhook", http.StatusBadRequest)

	err = server.Stop()
	select {
	case err := <-errorChannel:
		assert.Equal(t, http.ErrServerClosed.Error(), err.Error())
	case <-time.After(3 * time.Second):
		t.Fatal("Server did not shut down properly")
	}
	assert.NoError(t, err)

	client := http.Client{Timeout: 5 * time.Second}
	_, err = client.Get("http://localhost:8080/webhook")
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
	go func() {
		err := server.Start()
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
	t.Skip("skip this test for CRD branch until we implement GITOPS-7336")
	server := createMockServer(t, 8080)
	mockArgoClient := server.ArgoClient.(*mocks.ArgoCD)
	mockArgoClient.On("ListApplications", mock.Anything).Return([]v1alpha1.Application{}, nil).Maybe()

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
			handler: "docker",
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

// TestProcessWebhookEvent tests the processWebhookEvent helper function
func TestProcessWebhookEvent(t *testing.T) {
	t.Skip("skip this test for CRD branch until we implement GITOPS-7336")
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		server := createMockServer(t, 8080)
		mockArgoClient := server.ArgoClient.(*mocks.ArgoCD)
		mockArgoClient.On("ListApplications", mock.Anything).Return(mockApps, nil).Once()

		event := &WebhookEvent{
			RegistryURL: "",
			Repository:  "nginx",
			Tag:         "1.21.0",
			Digest:      "sha256:thisisatestingsha256value",
		}

		err := server.processWebhookEvent(event)
		assert.NoError(t, err)

		mockArgoClient.AssertExpectations(t)
	}()
	wg.Wait()
}

// TestWebhookServerWebhookEndpoint ensures that the webhook endpoint of the server is working properly
func TestWebhookServerWebhookEndpoint(t *testing.T) {
	t.Skip("skip this test for CRD branch until we implement GITOPS-7336")
	server := createMockServer(t, 8080)
	mockArgoClient := server.ArgoClient.(*mocks.ArgoCD)
	mockArgoClient.On("ListApplications", mock.Anything).Return(mockApps, nil).Once()

	handler := NewDockerHubWebhook("")
	assert.NotNil(t, handler, "Docker handler was nil")

	server.Handler.RegisterHandler(handler)

	go func() {
		err := server.Start()
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
	res, err := client.Post(address+"webhook?type=docker", "application/json", bytes.NewReader([]byte(body)))
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

// TestFindMatchingApplications tests the helper function used in processWebhookEvent
func TestFindMatchingApplications(t *testing.T) {
	t.Skip("skip this test for CRD branch until we implement GITOPS-7336")

	server := createMockServer(t, 8080)

	tests := []struct {
		name            string
		apps            []v1alpha1.Application
		event           *WebhookEvent
		expectedMatches map[string]argocd.ApplicationImages
	}{
		{
			name: "find single in many",
			apps: mockApps,
			event: &WebhookEvent{
				RegistryURL: "quay.io",
				Repository:  "argoprojlabs/argocd-image-updater",
				Tag:         "1.17.0",
				Digest:      "sha256:thisisatestingsha256value",
			},
			expectedMatches: map[string]argocd.ApplicationImages{
				"argocd/test-appA": {
					Application: v1alpha1.Application{
						ObjectMeta: v1.ObjectMeta{
							Name:      "test-appA",
							Namespace: "argocd",
							Annotations: map[string]string{
								ImageUpdaterAnnotation: "quay.io/argoprojlabs/argocd-image-updater:1.X.X",
							},
						},
						Spec: v1alpha1.ApplicationSpec{},
						Status: v1alpha1.ApplicationStatus{
							Summary: v1alpha1.ApplicationSummary{
								Images: []string{"quay.io/argoprojlabs/argocd-image-updater:1.16.0"},
							},
						},
					},
					// TODO: webhook for CRD will be refactored in GITOPS-7336
					//Images: image.ContainerImageList{
					//	image.NewFromIdentifier("quay.io/argoprojlabs/argocd-image-updater:1.X.X"),
					//},
				},
			},
		},
		{
			name: "find none",
			apps: mockApps,
			event: &WebhookEvent{
				RegistryURL: "",
				Repository:  "nginx",
				Tag:         "1.21.0",
				Digest:      "sha256:thisisatestingsha256value",
			},
			expectedMatches: map[string]argocd.ApplicationImages{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matchedApps := server.findMatchingApplications(tt.apps, tt.event)
			assert.Equal(t, matchedApps, tt.expectedMatches, "Matched apps were not equal")
		})
	}
}

// TestParseImageList tests the helper function
// The tests are an exact duplicate of the ones in the Argocd package
func TestParseImageList(t *testing.T) {

	t.Run("Test basic parsing", func(t *testing.T) {
		assert.Equal(t, []string{"foo", "bar"}, parseImageList(map[string]string{ImageUpdaterAnnotation: " foo, bar "}).Originals())
		// should whitespace inside the spec be preserved?
		assert.Equal(t, []string{"foo", "bar", "baz = qux"}, parseImageList(map[string]string{ImageUpdaterAnnotation: " foo, bar,baz = qux "}).Originals())
		assert.Equal(t, []string{"foo", "bar", "baz=qux"}, parseImageList(map[string]string{ImageUpdaterAnnotation: "foo,bar,baz=qux"}).Originals())
	})
	t.Run("Test kustomize override", func(t *testing.T) {
		imgs := *parseImageList(map[string]string{
			ImageUpdaterAnnotation: "foo=bar",
			fmt.Sprintf(Prefixed(ImageUpdaterAnnotationPrefix, KustomizeApplicationNameAnnotationSuffix), "foo"): "baz",
		})
		assert.Equal(t, "bar", imgs[0].ImageName)
		assert.Equal(t, "baz", imgs[0].KustomizeImage.ImageName)
	})
}

// TestWebhookServerRateLimit tests to see if the webhook endpoint's rate limiting functionality works
func TestWebhookServerRateLimit(t *testing.T) {
	server := createMockServer(t, 8080)
	mockArgoClient := server.ArgoClient.(*mocks.ArgoCD)
	mockArgoClient.On("ListApplications", mock.Anything).Return([]v1alpha1.Application{}, nil).Maybe()

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

	req := httptest.NewRequest(http.MethodPost, "/webhook?type=docker", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	server.handleWebhook(rec, req)

	// Wait for thread to call it.
	time.Sleep(time.Second)

	assert.True(t, mock.Called, "Take was not called")
}
