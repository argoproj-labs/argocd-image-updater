package webhook

import (
	"fmt"
	"net/http"
	"strings"
	"sync"

	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd"
	"github.com/argoproj-labs/argocd-image-updater/pkg/common"
	"github.com/argoproj-labs/argocd-image-updater/pkg/kube"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/image"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"

	"github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
)

// WebhookServer manages webhook endpoints and triggers update checks
type WebhookServer struct {
	// Port is the port number to listen on
	Port int
	// Handler is the webhook handler
	Handler *WebhookHandler
	// UpdaterConfig holds configuration for image updating
	UpdaterConfig *argocd.UpdateConfiguration
	// KubeClient is the Kubernetes client
	KubeClient *kube.ImageUpdaterKubernetesClient
	// ArgoClient is the ArgoCD client
	ArgoClient argocd.ArgoCD
	// Server is the HTTP server
	Server *http.Server
	// mutex for concurrent update operations
	mutex sync.Mutex
	// mutex for concurrent repo access
	syncState *argocd.SyncIterationState
}

// NewWebhookServer creates a new webhook server
func NewWebhookServer(port int, handler *WebhookHandler, kubeClient *kube.ImageUpdaterKubernetesClient, argoClient argocd.ArgoCD) *WebhookServer {
	return &WebhookServer{
		Port:       port,
		Handler:    handler,
		KubeClient: kubeClient,
		ArgoClient: argoClient,
		syncState:  argocd.NewSyncIterationState(),
	}
}

// Start starts the webhook server
func (s *WebhookServer) Start() error {
	// Create server and register routes
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", s.handleWebhook)
	mux.HandleFunc("/healthz", s.handleHealth)

	addr := fmt.Sprintf(":%d", s.Port)
	s.Server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	log.Infof("Starting webhook server on port %d", s.Port)
	return s.Server.ListenAndServe()
}

// Stop stops the webhook server
func (s *WebhookServer) Stop() error {
	log.Infof("Stopping webhook server")
	return s.Server.Close()
}

// handleHealth handles health check requests
func (s *WebhookServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("OK")); err != nil {
		log.Errorf("Failed to write health check response: %v", err)
	}
}

// handleWebhook handles webhook requests
func (s *WebhookServer) handleWebhook(w http.ResponseWriter, r *http.Request) {
	logCtx := log.WithContext().AddField("remote", r.RemoteAddr)
	logCtx.Debugf("Received webhook request from %s", r.RemoteAddr)

	event, err := s.Handler.ProcessWebhook(r)
	if err != nil {
		logCtx.Errorf("Failed to process webhook: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	logCtx.AddField("registry", event.RegistryURL).
		AddField("repository", event.Repository).
		AddField("tag", event.Tag).
		Infof("Received valid webhook event")

	// Process webhook asynchronously
	go func() {
		err := s.processWebhookEvent(event)
		if err != nil {
			logCtx.Errorf("Failed to process webhook event: %v", err)
		}
	}()

	// Return success immediately
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("Webhook received and processing")); err != nil {
		logCtx.Errorf("Failed to write webhook response: %v", err)
	}
}

// processWebhookEvent processes a webhook event and triggers image update checks
func (s *WebhookServer) processWebhookEvent(event *WebhookEvent) error {
	logCtx := log.WithContext().
		AddField("registry", event.RegistryURL).
		AddField("repository", event.Repository).
		AddField("tag", event.Tag)

	logCtx.Infof("Processing webhook event for %s/%s:%s", event.RegistryURL, event.Repository, event.Tag)

	// Lock for concurrent webhook processing
	s.mutex.Lock()
	defer s.mutex.Unlock()

	// List applications
	apps, err := s.ArgoClient.ListApplications("")
	if err != nil {
		return fmt.Errorf("failed to list applications: %w", err)
	}

	logCtx.Infof("Found %d applications, checking for matches", len(apps))

	// Find applications that are watching this image
	matchedApps := s.findMatchingApplications(apps, event)
	if len(matchedApps) == 0 {
		logCtx.Infof("No applications found watching image %s/%s", event.RegistryURL, event.Repository)
		return nil
	}

	logCtx.Infof("Found %d applications watching image %s/%s", len(matchedApps), event.RegistryURL, event.Repository)

	// Update each matching application
	for appName, appImages := range matchedApps {
		appLogCtx := logCtx.AddField("application", appName)
		appLogCtx.Infof("Triggering image update check for application")

		// Create update configuration for this application
		s.UpdaterConfig.UpdateApp = &appImages

		// Run the update process
		result := argocd.UpdateApplication(s.UpdaterConfig, s.syncState)

		appLogCtx.Infof("Update result: processed=%d, updated=%d, errors=%d, skipped=%d",
			result.NumApplicationsProcessed, result.NumImagesUpdated, result.NumErrors, result.NumSkipped)
	}

	return nil
}

// findMatchingApplications finds applications that are watching the image from the webhook event
func (s *WebhookServer) findMatchingApplications(apps []v1alpha1.Application, event *WebhookEvent) map[string]argocd.ApplicationImages {
	matchedApps := make(map[string]argocd.ApplicationImages)

	for _, app := range apps {
		// Skip applications without image-list annotation
		annotations := app.GetAnnotations()
		if _, exists := annotations[common.ImageUpdaterAnnotation]; !exists {
			continue
		}

		// Parse the image list annotation
		imageList := parseImageList(annotations)
		if imageList == nil {
			continue
		}

		// Check if any of the images match the event
		for _, img := range *imageList {
			// Skip if registry doesn't match
			if img.RegistryURL != "" && img.RegistryURL != event.RegistryURL {
				continue
			}

			// Check if repository matches
			if img.ImageName != event.Repository {
				continue
			}

			// Found a match, add to the list
			appName := fmt.Sprintf("%s/%s", app.Namespace, app.Name)
			appImages := argocd.ApplicationImages{
				Application: app,
				Images:      *imageList,
			}
			matchedApps[appName] = appImages
			break
		}
	}

	return matchedApps
}

// parseImageList is a local helper function that replicates the logic from argocd package
func parseImageList(annotations map[string]string) *image.ContainerImageList {
	results := make(image.ContainerImageList, 0)
	if updateImage, ok := annotations[common.ImageUpdaterAnnotation]; ok {
		splits := strings.Split(updateImage, ",")
		for _, s := range splits {
			img := image.NewFromIdentifier(strings.TrimSpace(s))
			if kustomizeImage := img.GetParameterKustomizeImageName(annotations, common.ImageUpdaterAnnotationPrefix); kustomizeImage != "" {
				img.KustomizeImage = image.NewFromIdentifier(kustomizeImage)
			}
			results = append(results, img)
		}
	}
	return &results
}
