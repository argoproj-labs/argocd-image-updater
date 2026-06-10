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

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"

	"github.com/argoproj-labs/argocd-image-updater/internal/controller"
	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd"
	"github.com/argoproj-labs/argocd-image-updater/pkg/common"
	"github.com/argoproj-labs/argocd-image-updater/pkg/metrics"
	"github.com/argoproj-labs/argocd-image-updater/pkg/webhook"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/registry"
)

// WebhookConfig holds the options for the webhook server
type WebhookConfig struct {
	// RequireSecret requires webhook secrets by default
	RequireSecret bool
	// Port is the port number for the webhook server to listen on
	Port int
	// EnableHTTP2 allows the webhook TLS server to negotiate HTTP/2
	EnableHTTP2 bool
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
	// DisableTLS disables TLS and runs the webhook server with plain HTTP
	DisableTLS bool
	// TLSMinVersion is the minimum TLS version (e.g. "1.2", "1.3")
	TLSMinVersion string
	// TLSMaxVersion is the maximum TLS version (e.g. "1.2", "1.3")
	TLSMaxVersion string
	// TLSCiphers is a colon-separated list of TLS cipher suite names
	TLSCiphers string
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
func SetupWebhookServer(ctx context.Context, webhookCfg *WebhookConfig, reconciler *controller.ImageUpdaterReconciler) *webhook.WebhookServer {
	log := log.LoggerFromContext(ctx)

	// Create webhook handler
	handler := webhook.NewWebhookHandler()

	if !webhookCfg.RequireSecret {
		// Insecure mode: all handlers are registered regardless of whether a secret is
		// configured. Handlers without a secret will accept unauthenticated requests.
		log.Warnf("Webhook secrets are not required (--webhook-require-secret=false). " +
			"All registry handlers will be registered without secret validation. " +
			"This is insecure and should not be used in production.")
		handler.RegisterHandler(webhook.NewDockerHubWebhook(webhookCfg.DockerSecret))
		handler.RegisterHandler(webhook.NewGHCRWebhook(webhookCfg.GHCRSecret))
		handler.RegisterHandler(webhook.NewHarborWebhook(webhookCfg.HarborSecret))
		handler.RegisterHandler(webhook.NewQuayWebhook(webhookCfg.QuaySecret))
		handler.RegisterHandler(webhook.NewAliyunACRWebhook(webhookCfg.AliyunACRSecret))
		handler.RegisterHandler(webhook.NewCloudEventsWebhook(webhookCfg.CloudEventsSecret))
	} else {
		// Secure mode (default): only register handlers for which a secret has been
		// configured. Handlers without a secret are skipped entirely so unauthenticated
		// requests from that registry are rejected before reaching the server.
		registered := 0
		if webhookCfg.DockerSecret != "" {
			handler.RegisterHandler(webhook.NewDockerHubWebhook(webhookCfg.DockerSecret))
			log.Infof("Registered Docker Hub webhook handler")
			registered++
		}
		if webhookCfg.GHCRSecret != "" {
			handler.RegisterHandler(webhook.NewGHCRWebhook(webhookCfg.GHCRSecret))
			log.Infof("Registered GHCR webhook handler")
			registered++
		}
		if webhookCfg.HarborSecret != "" {
			handler.RegisterHandler(webhook.NewHarborWebhook(webhookCfg.HarborSecret))
			log.Infof("Registered Harbor webhook handler")
			registered++
		}
		if webhookCfg.QuaySecret != "" {
			handler.RegisterHandler(webhook.NewQuayWebhook(webhookCfg.QuaySecret))
			log.Infof("Registered Quay webhook handler")
			registered++
		}
		if webhookCfg.AliyunACRSecret != "" {
			handler.RegisterHandler(webhook.NewAliyunACRWebhook(webhookCfg.AliyunACRSecret))
			log.Infof("Registered Aliyun ACR webhook handler")
			registered++
		}
		if webhookCfg.CloudEventsSecret != "" {
			handler.RegisterHandler(webhook.NewCloudEventsWebhook(webhookCfg.CloudEventsSecret))
			log.Infof("Registered CloudEvents webhook handler")
			registered++
		}
		if registered == 0 {
			log.Warnf("Webhook server is enabled with --webhook-require-secret=true but no secrets " +
				"are configured. No handlers will be registered and all webhook requests will be rejected. " +
				"Configure at least one *-webhook-secret flag or set --webhook-require-secret=false.")
		}
	}

	// Create webhook server
	server := webhook.NewWebhookServer(webhookCfg.Port, handler, reconciler)

	// Configure TLS
	server.DisableTLS = webhookCfg.DisableTLS
	server.TLS = &webhook.TLSConfig{
		CertFile:    webhook.DefaultTLSCertPath,
		KeyFile:     webhook.DefaultTLSKeyPath,
		MinVersion:  webhookCfg.TLSMinVersion,
		MaxVersion:  webhookCfg.TLSMaxVersion,
		Ciphers:     webhookCfg.TLSCiphers,
		EnableHTTP2: webhookCfg.EnableHTTP2,
	}

	if webhookCfg.RateLimitNumAllowedRequests > 0 {
		server.RateLimiter = ratelimit.New(webhookCfg.RateLimitNumAllowedRequests, ratelimit.Per(time.Hour))
	}
	return server
}
