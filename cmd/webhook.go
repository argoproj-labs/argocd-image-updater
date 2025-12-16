package main

import (
	"context"
	"math"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/bombsimon/logrusr/v2"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/argoproj-labs/argocd-image-updater/internal/controller"
	"github.com/argoproj-labs/argocd-image-updater/pkg/common"
	"github.com/argoproj-labs/argocd-image-updater/pkg/version"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/env"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
)

// NewWebhookCommand creates a new webhook command
func NewWebhookCommand() *cobra.Command {
	var cfg *controller.ImageUpdaterConfig = &controller.ImageUpdaterConfig{}
	var webhookCfg *WebhookConfig = &WebhookConfig{}
	var kubeConfig string
	var commitMessagePath string
	var MaxConcurrentUpdaters int
	var webhookCmd = &cobra.Command{
		Use:   "webhook",
		Short: "Start webhook server to receive registry events",
		Long: `
The webhook command starts a server that listens for webhook events from
container registries. When an event is received, it can trigger an image
update check for the affected images.

Supported registries:
- Docker Hub
- GitHub Container Registry (GHCR)
- Quay
- Harbor
- AWS ECR (via EventBridge CloudEvents)
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := log.SetLogLevel(cfg.LogLevel); err != nil {
				return err
			}
			ctrl.SetLogger(logrusr.New(log.Log()))
			setupLogger := ctrl.Log.WithName("webhook-setup")

			setupLogger.Info("Webhook logger initialized.", "setLogLevel", cfg.LogLevel)
			setupLogger.Info("starting",
				"app", version.BinaryName()+": "+version.Version(),
				"loglevel", strings.ToUpper(cfg.LogLevel),
				"webhookPort", strconv.Itoa(webhookCfg.Port),
			)

			ctx := ctrl.SetupSignalHandler()
			err := SetupCommon(ctx, cfg, setupLogger, commitMessagePath, kubeConfig)
			if err != nil {
				return err
			}

			err = runWebhook(ctx, cfg, webhookCfg, MaxConcurrentUpdaters)
			return err
		},
	}

	// Set Image Updater flags
	webhookCmd.Flags().StringVar(&cfg.LogLevel, "loglevel", env.GetStringVal("IMAGE_UPDATER_LOGLEVEL", "info"), "set the loglevel to one of trace|debug|info|warn|error")
	webhookCmd.Flags().StringVar(&kubeConfig, "kubeconfig", "", "full path to kubernetes client configuration, i.e. ~/.kube/config")
	webhookCmd.Flags().StringVar(&cfg.RegistriesConf, "registries-conf-path", common.DefaultRegistriesConfPath, "path to registries configuration file")
	webhookCmd.Flags().IntVar(&cfg.MaxConcurrentApps, "max-concurrent-apps", env.ParseNumFromEnv("MAX_CONCURRENT_APPS", 10, 1, 100), "maximum number of ArgoCD applications that can be updated concurrently (must be >= 1)")
	webhookCmd.Flags().IntVar(&MaxConcurrentUpdaters, "max-concurrent-updaters", env.ParseNumFromEnv("MAX_CONCURRENT_UPDATERS", 1, 1, 10), "maximum number of concurrent ImageUpdater CRs that can be processed (must be >= 1)")
	webhookCmd.Flags().StringVar(&cfg.ArgocdNamespace, "argocd-namespace", env.GetStringVal("ARGOCD_NAMESPACE", ""), "namespace where ArgoCD runs in (controller namespace by default)")

	webhookCmd.Flags().StringVar(&cfg.GitCommitUser, "git-commit-user", env.GetStringVal("GIT_COMMIT_USER", "argocd-image-updater"), "Username to use for Git commits")
	webhookCmd.Flags().StringVar(&cfg.GitCommitMail, "git-commit-email", env.GetStringVal("GIT_COMMIT_EMAIL", "noreply@argoproj.io"), "E-Mail address to use for Git commits")
	webhookCmd.Flags().StringVar(&cfg.GitCommitSigningKey, "git-commit-signing-key", env.GetStringVal("GIT_COMMIT_SIGNING_KEY", ""), "GnuPG key ID or path to Private SSH Key used to sign the commits")
	webhookCmd.Flags().StringVar(&cfg.GitCommitSigningMethod, "git-commit-signing-method", env.GetStringVal("GIT_COMMIT_SIGNING_METHOD", "openpgp"), "Method used to sign Git commits ('openpgp' or 'ssh')")
	webhookCmd.Flags().BoolVar(&cfg.GitCommitSignOff, "git-commit-sign-off", env.GetBoolVal("GIT_COMMIT_SIGN_OFF", false), "Whether to sign-off git commits")
	webhookCmd.Flags().StringVar(&commitMessagePath, "git-commit-message-path", common.DefaultCommitTemplatePath, "Path to a template to use for Git commit messages")
	webhookCmd.Flags().BoolVar(&cfg.DisableKubeEvents, "disable-kube-events", env.GetBoolVal("IMAGE_UPDATER_KUBE_EVENTS", false), "Disable kubernetes events")

	webhookCmd.Flags().IntVar(&webhookCfg.Port, "webhook-port", env.ParseNumFromEnv("WEBHOOK_PORT", 8080, 0, 65535), "Port to listen on for webhook events")
	webhookCmd.Flags().StringVar(&webhookCfg.DockerSecret, "docker-webhook-secret", env.GetStringVal("DOCKER_WEBHOOK_SECRET", ""), "Secret for validating Docker Hub webhooks")
	webhookCmd.Flags().StringVar(&webhookCfg.GHCRSecret, "ghcr-webhook-secret", env.GetStringVal("GHCR_WEBHOOK_SECRET", ""), "Secret for validating GitHub Container Registry webhooks")
	webhookCmd.Flags().StringVar(&webhookCfg.QuaySecret, "quay-webhook-secret", env.GetStringVal("QUAY_WEBHOOK_SECRET", ""), "Secret for validating Quay webhooks")
	webhookCmd.Flags().StringVar(&webhookCfg.HarborSecret, "harbor-webhook-secret", env.GetStringVal("HARBOR_WEBHOOK_SECRET", ""), "Secret for validating Harbor webhooks")
	webhookCmd.Flags().StringVar(&webhookCfg.CloudEventsSecret, "cloudevents-webhook-secret", env.GetStringVal("CLOUDEVENTS_WEBHOOK_SECRET", ""), "Secret for validating CloudEvents webhooks")
	webhookCmd.Flags().IntVar(&webhookCfg.RateLimitNumAllowedRequests, "webhook-ratelimit-allowed", env.ParseNumFromEnv("WEBHOOK_RATELIMIT_ALLOWED", 0, 0, math.MaxInt), "The number of allowed requests in an hour for webhook rate limiting, setting to 0 disables ratelimiting")

	return webhookCmd
}

// runWebhook starts the webhook server
func runWebhook(ctx context.Context, cfg *controller.ImageUpdaterConfig, webhookCfg *WebhookConfig, maxConcurrentUpdaters int) error {
	webhookLogger := log.Log().WithFields(logrus.Fields{
		"logger": "webhook-command",
	})
	ctx = log.ContextWithLogger(ctx, webhookLogger)

	config, err := ctrl.GetConfig()
	if err != nil {
		webhookLogger.Errorf("could not get k8s config: %v", err)
		return err
	}

	k8sClient, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		webhookLogger.Errorf("could not create ks client: %v", err)
		return err
	}

	reconciler := &controller.ImageUpdaterReconciler{
		Client:                  k8sClient,
		Scheme:                  scheme,
		Config:                  cfg,
		MaxConcurrentReconciles: maxConcurrentUpdaters,
	}

	server := SetupWebhookServer(webhookCfg, reconciler)

	// Create a context that is cancelled on an interrupt signal
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	return server.Start(ctx)
}
