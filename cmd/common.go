package main

import (
	"context"
	"errors"
	"os"
	"text/template"
	"time"

	"github.com/argoproj/argo-cd/v3/util/askpass"
	"github.com/go-logr/logr"
	"go.uber.org/ratelimit"

	"github.com/argoproj-labs/argocd-image-updater/internal/controller"
	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd"
	"github.com/argoproj-labs/argocd-image-updater/pkg/common"
	"github.com/argoproj-labs/argocd-image-updater/pkg/metrics"
	"github.com/argoproj-labs/argocd-image-updater/pkg/webhook"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/registry"
)

// WebhookConfig holds the options for the webhook server
type WebhookConfig struct {
	// Port is the port number for the webhook server to listen on
	Port int
	// DockerSecret is the secret for validating Docker Hub webhooks
	DockerSecret string
	// GHCRSecret is the secret for validating GitHub Container Registry webhooks
	GHCRSecret string
	// QuaySecret is the secret for validating Quay webhooks
	QuaySecret string
	// HarborSecret is the secret for validating Harbor webhooks
	HarborSecret string
	// AliyunACRSecret is the secret for validating Aliyun ACR webhooks
	AliyunACRSecret string
	// CloudEventsSecret is the secret for validating CloudEvents webhooks
	CloudEventsSecret string
	// RateLimitNumAllowedRequests is the number of allowed requests per hour for rate limiting (0 disables rate limiting)
	RateLimitNumAllowedRequests int
}

// SetupCommon initializes common components (logging, context, etc.)
func SetupCommon(ctx context.Context, cfg *controller.ImageUpdaterConfig, setupLogger logr.Logger, commitMessagePath, kubeConfig string) error {
	// Initialize metrics before starting the metrics server or using any counters
	metrics.InitMetrics()

	var commitMessageTpl string

	// User can specify a path to a template used for Git commit messages
	if commitMessagePath != "" {
		tpl, err := os.ReadFile(commitMessagePath)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				setupLogger.Info("commit message template not found, using default", "path", commitMessagePath)
				commitMessageTpl = common.DefaultGitCommitMessage
			} else {
				setupLogger.Error(err, "could not read commit message template", "path", commitMessagePath)
				return err
			}
		} else {
			commitMessageTpl = string(tpl)
		}
	}

	if commitMessageTpl == "" {
		setupLogger.Info("Using default Git commit message template")
		commitMessageTpl = common.DefaultGitCommitMessage
	}

	if tpl, err := template.New("commitMessage").Parse(commitMessageTpl); err != nil {
		setupLogger.Error(err, "could not parse commit message template")
		return err
	} else {
		setupLogger.V(1).Info("Successfully parsed commit message template")
		cfg.GitCommitMessage = tpl
	}

	// Load registries configuration early on. We do not consider it a fatal
	// error when the file does not exist, but we emit a warning.
	if cfg.RegistriesConf != "" {
		st, err := os.Stat(cfg.RegistriesConf)
		if err != nil || st.IsDir() {
			setupLogger.Info("Registry configuration not found or is a directory, using default configuration", "path", cfg.RegistriesConf, "error", err)
		} else {
			err = registry.LoadRegistryConfiguration(ctx, cfg.RegistriesConf, false)
			if err != nil {
				setupLogger.Error(err, "could not load registry configuration", "path", cfg.RegistriesConf)
				return err
			}
		}
	}

	// Setup Kubernetes client
	var err error
	cfg.KubeClient, err = argocd.GetKubeConfig(ctx, cfg.ArgocdNamespace, kubeConfig)
	if err != nil {
		setupLogger.Error(err, "could not create K8s client")
		return err
	}

	// Start up the credentials store server
	cs := askpass.NewServer(askpass.SocketPath)
	csErrCh := make(chan error)
	go func() {
		setupLogger.V(1).Info("Starting askpass server")
		csErrCh <- cs.Run()
	}()

	// Wait for cred server to be started, just in case
	if err := <-csErrCh; err != nil {
		setupLogger.Error(err, "Error running askpass server")
		return err
	}

	cfg.GitCreds = cs

	return nil
}

// SetupWebhookServer creates and configures a new webhook server.
func SetupWebhookServer(webhookCfg *WebhookConfig, reconciler *controller.ImageUpdaterReconciler) *webhook.WebhookServer {
	// Create webhook handler
	handler := webhook.NewWebhookHandler()

	// Register supported webhook handlers with default empty secrets
	handler.RegisterHandler(webhook.NewDockerHubWebhook(webhookCfg.DockerSecret))
	handler.RegisterHandler(webhook.NewGHCRWebhook(webhookCfg.GHCRSecret))
	handler.RegisterHandler(webhook.NewHarborWebhook(webhookCfg.HarborSecret))
	handler.RegisterHandler(webhook.NewQuayWebhook(webhookCfg.QuaySecret))
	handler.RegisterHandler(webhook.NewAliyunACRWebhook(webhookCfg.AliyunACRSecret))
	handler.RegisterHandler(webhook.NewCloudEventsWebhook(webhookCfg.CloudEventsSecret))

	// Create webhook server
	server := webhook.NewWebhookServer(webhookCfg.Port, handler, reconciler)

	if webhookCfg.RateLimitNumAllowedRequests > 0 {
		server.RateLimiter = ratelimit.New(webhookCfg.RateLimitNumAllowedRequests, ratelimit.Per(time.Hour))
	}
	return server
}
