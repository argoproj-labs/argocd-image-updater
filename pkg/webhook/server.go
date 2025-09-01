package webhook

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
	"go.uber.org/ratelimit"

	api "github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"
	"github.com/argoproj-labs/argocd-image-updater/internal/controller"
	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
)

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

	log.Infof("Starting webhook server on port %d", s.Port)

	// Start server in goroutine
	go func() {
		if err := s.Server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Errorf("Webhook server error: %v", err)
		}
	}()

	// Wait for context cancellation
	<-ctx.Done()

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
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
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write([]byte("OK")); err != nil {
		log.Errorf("Failed to write health check response: %v", err)
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

	event, err := s.Handler.ProcessWebhook(r)
	if err != nil {
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
