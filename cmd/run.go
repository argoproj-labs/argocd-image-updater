package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"math"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/bombsimon/logrusr/v2"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	argocdlog "github.com/argoproj/argo-cd/v3/util/log"

	api "github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"
	"github.com/argoproj-labs/argocd-image-updater/internal/controller"
	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd"
	"github.com/argoproj-labs/argocd-image-updater/pkg/common"
	"github.com/argoproj-labs/argocd-image-updater/pkg/version"
	"github.com/argoproj-labs/argocd-image-updater/pkg/webhook"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/env"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
)

// newRunCommand implements "controller" command
func newRunCommand() *cobra.Command {
	var metricsAddr string
	var enableLeaderElection bool
	var leaderElectionNamespace string
	var probeAddr string
	var secureMetrics bool
	var enableHTTP2 bool
	var cfg = &controller.ImageUpdaterConfig{}
	var webhookCfg *WebhookConfig = &WebhookConfig{}
	var once bool
	var kubeConfig string
	var warmUpCache bool
	var commitMessagePath string
	var MaxConcurrentReconciles int

	var controllerCmd = &cobra.Command{
		Use:   "run",
		Short: "Manages ArgoCD Image Updater Controller.",
		Long: `The 'run' command starts the Kubernetes controller responsible for managing
ImageUpdater Custom Resources (CRs).

This controller monitors ImageUpdater CRs and reconciles them by:
  - Checking for new container image versions from specified registries.
  - Applying updates to applications based on CR policies.
  - Updating the status of the ImageUpdater CRs.

It operates as a long-running manager process within the Kubernetes cluster.
Flags can configure its metrics, health probes, and leader election.
This enables a CRD-driven approach to automated image updates with Argo CD.
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Configure the controller for run once mode
			if once {
				cfg.CheckInterval = 0
				probeAddr = "0"
				warmUpCache = true
			}

			// Configure the global logger for vendored argo-cd utilities
			logLvl, err := logrus.ParseLevel(cfg.LogLevel)
			if err != nil {
				return fmt.Errorf("could not parse log level: %w", err)
			}
			logrus.SetLevel(logLvl)
			logrus.SetFormatter(argocdlog.CreateFormatter(cfg.LogFormat))

			if err := log.SetLogLevel(cfg.LogLevel); err != nil {
				return err
			}

			var logFormat log.LogFormat
			switch cfg.LogFormat {
			case "text":
				logFormat = log.LogFormatText
			case "json":
				logFormat = log.LogFormatJSON
			default:
				return fmt.Errorf("invalid log format '%s'", cfg.LogFormat)
			}

			log.SetLogFormat(logFormat)

			ctrl.SetLogger(logrusr.New(log.Log()))
			setupLogger := ctrl.Log.WithName("controller-setup").
				WithValues(logrusFieldsToLogrValues(common.ControllerLogFields)...)

			setupLogger.Info("Controller runtime logger initialized.", "setAppLogLevel", cfg.LogLevel)
			logFields := []interface{}{
				"app", version.BinaryName() + ": " + version.Version(),
				"loglevel", strings.ToUpper(cfg.LogLevel),
				"interval", argocd.GetPrintableInterval(cfg.CheckInterval),
				"healthPort", probeAddr,
			}
			if cfg.ArgocdNamespace != "" {
				logFields = append(logFields, "argocdNamespace", cfg.ArgocdNamespace)
			}
			setupLogger.Info("starting", logFields...)

			// Create context with signal handling
			ctx := ctrl.SetupSignalHandler()
			err = SetupCommon(ctx, cfg, setupLogger, commitMessagePath, kubeConfig)
			if err != nil {
				return err
			}

			if cfg.CheckInterval > 0 && cfg.CheckInterval < 60*time.Second {
				setupLogger.Info("Warning: Check interval is very low. It is not recommended to run below 1m0s", "interval", cfg.CheckInterval)
			}

			// if the enable-http2 flag is false (the default), http/2 should be disabled
			// due to its vulnerabilities. More specifically, disabling http/2 will
			// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
			// Rapid Reset CVEs. For more information see:
			// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
			// - https://github.com/advisories/GHSA-4374-p667-p6c8
			var tlsOpts []func(*tls.Config)
			disableHTTP2 := func(c *tls.Config) {
				setupLogger.Info("Disabling HTTP/2 support")
				c.NextProtos = []string{"http/1.1"}
			}

			if !enableHTTP2 {
				tlsOpts = append(tlsOpts, disableHTTP2)
			}

			// Metrics endpoint is enabled in 'config/default/kustomization.yaml'. The Metrics options configure the server.
			// More info:
			// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/metrics/server
			// - https://book.kubebuilder.io/reference/metrics.html
			metricsServerOptions := metricsserver.Options{
				BindAddress:   metricsAddr,
				SecureServing: secureMetrics,
				// TODO(user): TLSOpts is used to allow configuring the TLS config used for the server. If certificates are
				// not provided, self-signed certificates will be generated by default. This option is not recommended for
				// production environments as self-signed certificates do not offer the same level of trust and security
				// as certificates issued by a trusted Certificate Authority (CA). The primary risk is potentially allowing
				// unauthorized access to sensitive metrics data. Consider replacing with CertDir, CertName, and KeyName
				// to provide certificates, ensuring the server communicates using trusted and secure certificates.
				TLSOpts: tlsOpts,
			}

			if secureMetrics {
				// FilterProvider is used to protect the metrics endpoint with authn/authz.
				// These configurations ensure that only authorized users and service accounts
				// can access the metrics endpoint. The RBAC are configured in 'config/rbac/kustomization.yaml'. More info:
				// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/metrics/filters#WithAuthenticationAndAuthorization
				metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
			}

			mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
				Scheme:                  scheme,
				Metrics:                 metricsServerOptions,
				HealthProbeBindAddress:  probeAddr,
				LeaderElection:          enableLeaderElection,
				LeaderElectionID:        "c21b75f2.argoproj.io",
				LeaderElectionNamespace: leaderElectionNamespace,
			})
			if err != nil {
				setupLogger.Error(err, "unable to start manager")
				return err
			}

			// Add the CacheWarmer as a Runnable to the manager.
			setupLogger.Info("Adding cache warmer to the manager.")
			warmupState := &WarmupStatus{
				Done: make(chan struct{}),
			}

			// Create stop channel for run-once mode
			stopChan := make(chan struct{})

			reconciler := &controller.ImageUpdaterReconciler{
				Client:                  mgr.GetClient(),
				Scheme:                  mgr.GetScheme(),
				Config:                  cfg,
				MaxConcurrentReconciles: MaxConcurrentReconciles,
				CacheWarmed:             warmupState.Done,
				StopChan:                stopChan,
				Once:                    once,
			}

			if warmUpCache {
				if err := mgr.Add(&CacheWarmer{
					Reconciler: reconciler,
					Status:     warmupState,
				}); err != nil {
					setupLogger.Error(err, "unable to add cache warmer to manager")
					return err
				}
			} else {
				setupLogger.Info("Cache warm-up disabled, skipping cache warmer")
				// If warm-up is disabled, we need to signal that cache is warmed
				close(warmupState.Done)
				warmupState.isCacheWarmed.Store(true)
			}

			// Start the webhook server if enabled
			setupLogger.Info("Adding Webhook Server as a Runnable to the manager.")
			if cfg.EnableWebhook && webhookCfg.Port > 0 {
				if err := mgr.Add(&WebhookServerRunnable{
					Reconciler:    reconciler,
					WebhookConfig: webhookCfg,
				}); err != nil {
					setupLogger.Error(err, "unable to add webhook server to manager")
					return err
				}
				setupLogger.Info("Webhook server runnable added to manager")
			} else {
				setupLogger.Info("webhook server is disabled, skip adding webhook server runnable to manager")
			}

			if err = reconciler.SetupWithManager(mgr); err != nil {
				setupLogger.Error(err, "unable to create controller", "controller", "ImageUpdater")
				return err
			}
			// +kubebuilder:scaffold:builder

			if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
				setupLogger.Error(err, "unable to set up health check")
				return err
			}

			if err := mgr.AddReadyzCheck("warmup-check", func(req *http.Request) error {
				if !warmupState.isCacheWarmed.Load() {
					// If the cache is not yet warmed, the check fails.
					return fmt.Errorf("cache is not yet warmed")
				}
				// Once warmed, the check passes.
				return nil
			}); err != nil {
				setupLogger.Error(err, "unable to set up ready check")
				return err
			}

			setupLogger.Info("starting manager")

			// Create a context that can be cancelled by the stop channel
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()

			// Start a goroutine to listen for stop signal from reconciler
			go func() {
				select {
				case <-stopChan:
					setupLogger.Info("received stop signal from reconciler, shutting down manager")
					cancel()
				case <-ctx.Done():
					// Normal shutdown signal (Ctrl+C, etc.) - context already cancelled
					setupLogger.Info("received shutdown signal")
				}
			}()

			if err := mgr.Start(ctx); err != nil {
				setupLogger.Error(err, "problem running manager")
				return err
			}
			return nil
		},
	}

	controllerCmd.Flags().StringVar(&metricsAddr, "metrics-bind-address", "0", "The address the metrics endpoint binds to. Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	controllerCmd.Flags().StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to. Change to 0 to disable the probe service.")
	controllerCmd.Flags().BoolVar(&secureMetrics, "metrics-secure", true, "If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	controllerCmd.Flags().BoolVar(&enableHTTP2, "enable-http2", false, "If set, HTTP/2 will be enabled for the metrics and webhook servers")

	controllerCmd.Flags().BoolVar(&enableLeaderElection, "leader-election", true, "Enable leader election for controller manager. Enabling this will ensure there is only one active controller manager.")
	controllerCmd.Flags().StringVar(&leaderElectionNamespace, "leader-election-namespace", "", "The namespace used for the leader election lease. If empty, the controller will use the namespace of the pod it is running in. When running locally this value must be set.")
	controllerCmd.Flags().BoolVar(&cfg.DryRun, "dry-run", false, "run in dry-run mode. If set to true, do not perform any changes")
	controllerCmd.Flags().DurationVar(&cfg.CheckInterval, "interval", env.GetDurationVal("IMAGE_UPDATER_INTERVAL", 2*time.Minute), "interval for how often to check for updates")
	controllerCmd.Flags().StringVar(&cfg.LogLevel, "loglevel", env.GetStringVal("IMAGE_UPDATER_LOGLEVEL", "info"), "set the loglevel to one of trace|debug|info|warn|error")
	controllerCmd.Flags().StringVar(&cfg.LogFormat, "logformat", env.GetStringVal("IMAGE_UPDATER_LOGFORMAT", "text"), "set the log format to one of text|json")
	controllerCmd.Flags().StringVar(&kubeConfig, "kubeconfig", "", "full path to kubernetes client configuration, i.e. ~/.kube/config")

	controllerCmd.Flags().BoolVar(&once, "once", false, "run only once, same as specifying --warmup-cache=true, --interval=0 and --health-probe-bind-address=0")
	controllerCmd.Flags().StringVar(&cfg.RegistriesConf, "registries-conf-path", common.DefaultRegistriesConfPath, "path to registries configuration file")
	controllerCmd.Flags().IntVar(&cfg.MaxConcurrentApps, "max-concurrent-apps", env.ParseNumFromEnv("MAX_CONCURRENT_APPS", 10, 1, 100), "maximum number of ArgoCD applications that can be updated concurrently (must be >= 1)")
	controllerCmd.Flags().IntVar(&MaxConcurrentReconciles, "max-concurrent-reconciles", env.ParseNumFromEnv("MAX_CONCURRENT_RECONCILES", 1, 1, 10), "maximum number of concurrent Reconciles which can be run (must be >= 1)")
	controllerCmd.Flags().StringVar(&cfg.ArgocdNamespace, "argocd-namespace", env.GetStringVal("ARGOCD_NAMESPACE", ""), "namespace where ArgoCD runs in (controller namespace by default)")
	controllerCmd.Flags().BoolVar(&warmUpCache, "warmup-cache", true, "whether to perform a cache warm-up on startup")
	controllerCmd.Flags().BoolVar(&cfg.DisableKubeEvents, "disable-kube-events", env.GetBoolVal("IMAGE_UPDATER_KUBE_EVENTS", false), "Disable kubernetes events")

	// Git flags
	controllerCmd.Flags().StringVar(&cfg.GitCommitUser, "git-commit-user", env.GetStringVal("GIT_COMMIT_USER", "argocd-image-updater"), "Username to use for Git commits")
	controllerCmd.Flags().StringVar(&cfg.GitCommitMail, "git-commit-email", env.GetStringVal("GIT_COMMIT_EMAIL", "noreply@argoproj.io"), "E-Mail address to use for Git commits")
	controllerCmd.Flags().StringVar(&cfg.GitCommitSigningKey, "git-commit-signing-key", env.GetStringVal("GIT_COMMIT_SIGNING_KEY", ""), "GnuPG key ID or path to Private SSH Key used to sign the commits")
	controllerCmd.Flags().StringVar(&cfg.GitCommitSigningMethod, "git-commit-signing-method", env.GetStringVal("GIT_COMMIT_SIGNING_METHOD", "openpgp"), "Method used to sign Git commits ('openpgp' or 'ssh')")
	controllerCmd.Flags().BoolVar(&cfg.GitCommitSignOff, "git-commit-sign-off", env.GetBoolVal("GIT_COMMIT_SIGN_OFF", false), "Whether to sign-off git commits")
	controllerCmd.Flags().StringVar(&commitMessagePath, "git-commit-message-path", common.DefaultCommitTemplatePath, "Path to a template to use for Git commit messages")

	// Webhook flags
	controllerCmd.Flags().BoolVar(&cfg.EnableWebhook, "enable-webhook", env.GetBoolVal("ENABLE_WEBHOOK", false), "Enable webhook server for receiving registry events")
	controllerCmd.Flags().IntVar(&webhookCfg.Port, "webhook-port", env.ParseNumFromEnv("WEBHOOK_PORT", 8082, 0, 65535), "Port to listen on for webhook events")
	controllerCmd.Flags().StringVar(&webhookCfg.DockerSecret, "docker-webhook-secret", env.GetStringVal("DOCKER_WEBHOOK_SECRET", ""), "Secret for validating Docker Hub webhooks")
	controllerCmd.Flags().StringVar(&webhookCfg.GHCRSecret, "ghcr-webhook-secret", env.GetStringVal("GHCR_WEBHOOK_SECRET", ""), "Secret for validating GitHub Container Registry webhooks")
	controllerCmd.Flags().StringVar(&webhookCfg.QuaySecret, "quay-webhook-secret", env.GetStringVal("QUAY_WEBHOOK_SECRET", ""), "Secret for validating Quay webhooks")
	controllerCmd.Flags().StringVar(&webhookCfg.HarborSecret, "harbor-webhook-secret", env.GetStringVal("HARBOR_WEBHOOK_SECRET", ""), "Secret for validating Harbor webhooks")
	controllerCmd.Flags().IntVar(&webhookCfg.RateLimitNumAllowedRequests, "webhook-ratelimit-allowed", env.ParseNumFromEnv("WEBHOOK_RATELIMIT_ALLOWED", 0, 0, math.MaxInt), "The number of allowed requests in an hour for webhook rate limiting, setting to 0 disables ratelimiting")

	return controllerCmd
}

// logrusFieldsToLogrValues converts a logrus.Fields map (map[string]interface{})
// into a flattened slice of key-value pairs, which is the format expected
// by the logr.WithValues function used by controller-runtime.
//
// For example, a map like:
//
//	logrus.Fields{"controller": "imageupdater", "version": "v1.0"}
//
// will be converted to a slice like:
//
//	[]interface{}{"controller", "imageupdater", "version", "v1.0"}
func logrusFieldsToLogrValues(fields logrus.Fields) []interface{} {
	values := make([]interface{}, 0, len(fields)*2)
	for key, val := range fields {
		values = append(values, key, val)
	}
	return values
}

// WarmupStatus holds the shared state indicating if the cache warm-up is complete.
type WarmupStatus struct {
	isCacheWarmed atomic.Bool
	Done          chan struct{}
}

// CacheWarmer implements manager.Runnable to warm up caches after the manager has started them.
type CacheWarmer struct {
	// We pass the whole Reconciler struct here, since it now holds all dependencies.
	Reconciler *controller.ImageUpdaterReconciler
	Status     *WarmupStatus
}

// Start contains the logic for Warmup Cache that will be executed by the manager.
func (cw *CacheWarmer) Start(ctx context.Context) error {
	defer close(cw.Status.Done)
	warmUpCacheLogger := common.LogFields(logrus.Fields{
		"logger": "warmup-cache",
	})
	ctx = log.ContextWithLogger(ctx, warmUpCacheLogger)

	warmUpCacheLogger.Debugf("Caches are synced. Warming up image cache...")
	imageList := &api.ImageUpdaterList{}

	warmUpCacheLogger.Debugf("Listing all ImageUpdater CRs before starting manager...")
	if err := cw.Reconciler.List(ctx, imageList); err != nil {
		warmUpCacheLogger.Errorf("Failed to list ImageUpdater CRs during warm-up: %v", err)
		return err
	}

	warmUpCacheLogger.Debugf("Found %d ImageUpdater CRs to process for cache warm-up.", len(imageList.Items))

	// If we're in run-once mode, count the total CRs and set up WaitGroup
	if cw.Reconciler.Once {
		cw.Reconciler.Wg.Add(len(imageList.Items))

		warmUpCacheLogger.Infof("Run-once mode: will process %d CRs before stopping", len(imageList.Items))

		// If there are no CRs, signal to stop immediately
		if len(imageList.Items) == 0 {
			warmUpCacheLogger.Infof("No CRs found in run-once mode - will stop immediately")
			if cw.Reconciler.StopChan != nil {
				close(cw.Reconciler.StopChan)
			}
		} else {
			// Start the stop watcher that will wait for all CRs to complete
			if cw.Reconciler.StopChan != nil {
				go func() {
					cw.Reconciler.Wg.Wait()
					close(cw.Reconciler.StopChan)
				}()
			}
		}
	}

	if err := cw.Reconciler.ProcessImageUpdaterCRs(ctx, imageList.Items, true, nil); err != nil {
		warmUpCacheLogger.Errorf("Failed to process ImageUpdater CRs during warm-up: %v", err)
		return err
	}

	cw.Status.isCacheWarmed.Store(true)
	warmUpCacheLogger.Debugf("Readiness state set to 'true'. Controller can now start reconciling.")
	warmUpCacheLogger.Infof("Finished cache warm-up.")
	return nil
}

// NeedLeaderElection tells the manager that this runnable should only be
// run on the leader replica.
func (cw *CacheWarmer) NeedLeaderElection() bool {
	return true
}

// WebhookServerRunnable implements manager.Runnable to update an app after event was triggered.
type WebhookServerRunnable struct {
	Reconciler    *controller.ImageUpdaterReconciler
	WebhookConfig *WebhookConfig
	webhookServer *webhook.WebhookServer
}

// Start contains the logic for Webhook that will be executed by the manager.
func (ws *WebhookServerRunnable) Start(ctx context.Context) error {
	webhookLogger := common.LogFields(logrus.Fields{
		"logger": "webhook-runnable",
	})
	ctx = log.ContextWithLogger(ctx, webhookLogger)

	ws.webhookServer = SetupWebhookServer(ws.WebhookConfig, ws.Reconciler)

	// Start webhook server in goroutine with context handling
	return ws.webhookServer.Start(ctx)
}

func (ws *WebhookServerRunnable) NeedLeaderElection() bool {
	return true
}
