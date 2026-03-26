package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd"
	"github.com/argoproj-labs/argocd-image-updater/pkg/common"
	"github.com/argoproj-labs/argocd-image-updater/pkg/health"
	"github.com/argoproj-labs/argocd-image-updater/pkg/metrics"
	"github.com/argoproj-labs/argocd-image-updater/pkg/version"
	"github.com/argoproj-labs/argocd-image-updater/pkg/webhook"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/env"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/image"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/registry"

	"github.com/argoproj/argo-cd/v3/util/askpass"
	argocdlog "github.com/argoproj/argo-cd/v3/util/log"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"

	"golang.org/x/sync/semaphore"

	"go.uber.org/ratelimit"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// newRunCommand implements "run" command
func newRunCommand() *cobra.Command {
	var cfg *ImageUpdaterConfig = &ImageUpdaterConfig{}
	var webhookCfg *WebhookConfig = &WebhookConfig{}
	var once bool
	var kubeConfig string
	var disableKubernetes bool
	var warmUpCache bool = true
	var commitMessagePath string
	var commitMessageTpl string
	var runCmd = &cobra.Command{
		Use:   "run",
		Short: "Runs the argocd-image-updater with a set of options",
		RunE: func(cmd *cobra.Command, args []string) error {
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
				return fmt.Errorf("Invalid log format '%s'", cfg.LogFormat)
			}
			log.SetLogFormat(logFormat)

			if once {
				cfg.CheckInterval = 0
				cfg.HealthPort = 0
			}

			// Enforce sane --max-concurrency values
			if cfg.MaxConcurrency < 1 {
				return fmt.Errorf("--max-concurrency must be greater than 1")
			}

			log.Infof("%s %s starting [loglevel:%s, interval:%s, healthport:%s]",
				version.BinaryName(),
				version.Version(),
				strings.ToUpper(cfg.LogLevel),
				getPrintableInterval(cfg.CheckInterval),
				getPrintableHealthPort(cfg.HealthPort),
			)

			// User can specify a path to a template used for Git commit messages
			if commitMessagePath != "" {
				tpl, err := os.ReadFile(commitMessagePath)
				if err != nil {
					if errors.Is(err, os.ErrNotExist) {
						log.Warnf("commit message template at %s does not exist, using default", commitMessagePath)
						commitMessageTpl = common.DefaultGitCommitMessage
					} else {
						log.Fatalf("could not read commit message template: %v", err)
					}
				} else {
					commitMessageTpl = string(tpl)
				}
			}

			if commitMessageTpl == "" {
				log.Infof("Using default Git commit messages")
				commitMessageTpl = common.DefaultGitCommitMessage
			}

			if tpl, err := template.New("commitMessage").Parse(commitMessageTpl); err != nil {
				log.Fatalf("could not parse commit message template: %v", err)
			} else {
				log.Debugf("Successfully parsed commit message template")
				cfg.GitCommitMessage = tpl
			}

			// Load registries configuration early on. We do not consider it a fatal
			// error when the file does not exist, but we emit a warning.
			if cfg.RegistriesConf != "" {
				st, err := os.Stat(cfg.RegistriesConf)
				if err != nil || st.IsDir() {
					log.Warnf("Registry configuration at %s could not be read: %v -- using default configuration", cfg.RegistriesConf, err)
				} else {
					err = registry.LoadRegistryConfiguration(cfg.RegistriesConf, false)
					if err != nil {
						log.Errorf("Could not load registry configuration from %s: %v", cfg.RegistriesConf, err)
						return nil
					}
				}
			}

			if cfg.CheckInterval > 0 && cfg.CheckInterval < 60*time.Second {
				log.Warnf("Check interval is very low - it is not recommended to run below 1m0s")
			}

			if !disableKubernetes {
				ctx := context.Background()
				cfg.KubeClient, err = getKubeConfig(ctx, cfg.ArgocdNamespace, kubeConfig)
				if err != nil {
					log.Fatalf("could not create K8s client: %v", err)
				}
				if cfg.ClientOpts.ServerAddr == "" {
					cfg.ClientOpts.ServerAddr = fmt.Sprintf("argocd-server.%s", cfg.KubeClient.KubeClient.Namespace)
				}
			}
			if cfg.ClientOpts.ServerAddr == "" {
				cfg.ClientOpts.ServerAddr = defaultArgoCDServerAddr
			}

			if token := os.Getenv("ARGOCD_TOKEN"); token != "" && cfg.ClientOpts.AuthToken == "" {
				log.Debugf("Using ArgoCD API credentials from environment ARGOCD_TOKEN")
				cfg.ClientOpts.AuthToken = token
			}

			log.Infof("ArgoCD configuration: [apiKind=%s, server=%s, auth_token=%v, insecure=%v, grpc_web=%v, plaintext=%v]",
				cfg.ApplicationsAPIKind,
				cfg.ClientOpts.ServerAddr,
				cfg.ClientOpts.AuthToken != "",
				cfg.ClientOpts.Insecure,
				cfg.ClientOpts.GRPCWeb,
				cfg.ClientOpts.Plaintext,
			)

			// Initialize metrics before starting the metrics server or using any counters
			metrics.InitMetrics()

			// Health server will start in a go routine and run asynchronously
			var hsErrCh chan error
			var msErrCh chan error
			var whErrCh chan error
			if cfg.HealthPort > 0 {
				log.Infof("Starting health probe server TCP port=%d", cfg.HealthPort)
				hsErrCh = health.StartHealthServer(cfg.HealthPort)
			}

			if cfg.MetricsPort > 0 {
				log.Infof("Starting metrics server on TCP port=%d", cfg.MetricsPort)
				msErrCh = metrics.StartMetricsServer(cfg.MetricsPort)
			}

			if warmUpCache {
				err := warmupImageCache(cfg)
				if err != nil {
					log.Errorf("Error warming up cache: %v", err)
					return err
				}
			}

			// Start up the credentials store server
			cs := askpass.NewServer(askpass.SocketPath)
			csErrCh := make(chan error)
			go func() {
				log.Debugf("Starting askpass server")
				csErrCh <- cs.Run()
			}()

			// Wait for cred server to be started, just in case
			err = <-csErrCh
			if err != nil {
				log.Errorf("Error running askpass server: %v", err)
				return err
			}

			cfg.GitCreds = cs

			// Start the webhook server if enabled
			var webhookServer *webhook.WebhookServer
			if cfg.EnableWebhook && webhookCfg.Port > 0 {
				// Initialize the ArgoCD client for webhook server
				var argoClient argocd.ArgoCD
				switch cfg.ApplicationsAPIKind {
				case applicationsAPIKindK8S:
					argoClient, err = argocd.NewK8SClient(cfg.KubeClient, &argocd.K8SClientOptions{AppNamespace: cfg.AppNamespace})
				case applicationsAPIKindArgoCD:
					argoClient, err = argocd.NewAPIClient(&cfg.ClientOpts)
				}
				if err != nil {
					log.Fatalf("Could not create ArgoCD client for webhook server: %v", err)
				}

				// Create webhook handler
				handler := webhook.NewWebhookHandler()

				// Register supported webhook handlers with default empty secrets
				dockerHandler := webhook.NewDockerHubWebhook(webhookCfg.DockerSecret)
				handler.RegisterHandler(dockerHandler)

				ghcrHandler := webhook.NewGHCRWebhook(webhookCfg.GHCRSecret)
				handler.RegisterHandler(ghcrHandler)

				harborHandler := webhook.NewHarborWebhook(webhookCfg.HarborSecret)
				handler.RegisterHandler(harborHandler)

				quayHandler := webhook.NewQuayWebhook(webhookCfg.QuaySecret)
				handler.RegisterHandler(quayHandler)

				log.Infof("Starting webhook server on port %d", webhookCfg.Port)
				webhookServer = webhook.NewWebhookServer(webhookCfg.Port, handler, cfg.KubeClient, argoClient)

				if webhookCfg.RateLimitNumAllowedRequests > 0 {
					webhookServer.RateLimiter = ratelimit.New(webhookCfg.RateLimitNumAllowedRequests, ratelimit.Per(time.Hour))
				}

				// Set updater config
				webhookServer.UpdaterConfig = &argocd.UpdateConfiguration{
					NewRegFN:               registry.NewClient,
					ArgoClient:             cfg.ArgoClient,
					KubeClient:             cfg.KubeClient,
					DryRun:                 cfg.DryRun,
					GitCommitUser:          cfg.GitCommitUser,
					GitCommitEmail:         cfg.GitCommitMail,
					GitCommitMessage:       cfg.GitCommitMessage,
					GitCommitSigningKey:    cfg.GitCommitSigningKey,
					GitCommitSigningMethod: cfg.GitCommitSigningMethod,
					GitCommitSignOff:       cfg.GitCommitSignOff,
					DisableKubeEvents:      cfg.DisableKubeEvents,
					GitCreds:               cfg.GitCreds,
				}

				whErrCh = make(chan error, 1)
				go func() {
					if err := webhookServer.Start(); err != nil {
						log.Errorf("Webhook server error: %v", err)
						whErrCh <- err
					}
				}()

				log.Infof("Webhook server started and listening on port %d", webhookCfg.Port)
			}

			// This is our main loop. We leave it only when our health probe server
			// returns an error.
			for {
				select {
				case err := <-hsErrCh:
					if err != nil {
						log.Errorf("Health probe server exited with error: %v", err)
					} else {
						log.Infof("Health probe server exited gracefully")
					}
					// Clean shutdown of webhook server if running
					if webhookServer != nil {
						if err := webhookServer.Stop(); err != nil {
							log.Errorf("Error stopping webhook server: %v", err)
						}
					}
					return nil
				case err := <-msErrCh:
					if err != nil {
						log.Errorf("Metrics server exited with error: %v", err)
					} else {
						log.Infof("Metrics server exited gracefully")
					}
					// Clean shutdown of webhook server if running
					if webhookServer != nil {
						if err := webhookServer.Stop(); err != nil {
							log.Errorf("Error stopping webhook server: %v", err)
						}
					}
					return nil
				case err := <-whErrCh:
					log.Errorf("Webhook server exited with error: %v", err)
					return nil
				default:
					if lastRun.IsZero() || time.Since(lastRun) > cfg.CheckInterval {
						result, err := runImageUpdater(cfg, false)
						if err != nil {
							log.Errorf("Error: %v", err)
						} else {
							log.Infof("Processing results: applications=%d images_considered=%d images_skipped=%d images_updated=%d errors=%d",
								result.NumApplicationsProcessed,
								result.NumImagesConsidered,
								result.NumSkipped,
								result.NumImagesUpdated,
								result.NumErrors)
						}
						lastRun = time.Now()
					}
				}
				if cfg.CheckInterval == 0 {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
			log.Infof("Finished.")
			return nil
		},
	}

	// DEPRECATED: These flags have been removed in the CRD branch and will be deprecated and removed in a future release.
	// The CRD branch introduces a new architecture that eliminates the need for these native ArgoCD client configuration flags.
	runCmd.Flags().StringVar(&cfg.ApplicationsAPIKind, "applications-api", env.GetStringVal("APPLICATIONS_API", applicationsAPIKindK8S), "API kind that is used to manage Argo CD applications ('kubernetes' or 'argocd'). DEPRECATED: this flag will be removed in a future version.")
	runCmd.Flags().StringVar(&cfg.ClientOpts.ServerAddr, "argocd-server-addr", env.GetStringVal("ARGOCD_SERVER", ""), "address of ArgoCD API server. DEPRECATED: this flag will be removed in a future version.")
	runCmd.Flags().BoolVar(&cfg.ClientOpts.GRPCWeb, "argocd-grpc-web", env.GetBoolVal("ARGOCD_GRPC_WEB", false), "use grpc-web for connection to ArgoCD. DEPRECATED: this flag will be removed in a future version.")
	runCmd.Flags().BoolVar(&cfg.ClientOpts.Insecure, "argocd-insecure", env.GetBoolVal("ARGOCD_INSECURE", false), "(INSECURE) ignore invalid TLS certs for ArgoCD server. DEPRECATED: this flag will be removed in a future version.")
	runCmd.Flags().BoolVar(&cfg.ClientOpts.Plaintext, "argocd-plaintext", env.GetBoolVal("ARGOCD_PLAINTEXT", false), "(INSECURE) connect without TLS to ArgoCD server. DEPRECATED: this flag will be removed in a future version.")
	runCmd.Flags().StringVar(&cfg.ClientOpts.AuthToken, "argocd-auth-token", "", "use token for authenticating to ArgoCD (unsafe - consider setting ARGOCD_TOKEN env var instead). DEPRECATED: this flag will be removed in a future version.")
	runCmd.Flags().BoolVar(&disableKubernetes, "disable-kubernetes", false, "do not create and use a Kubernetes client. DEPRECATED: this flag will be removed in a future version.")

	runCmd.Flags().BoolVar(&cfg.DryRun, "dry-run", false, "run in dry-run mode. If set to true, do not perform any changes")
	runCmd.Flags().DurationVar(&cfg.CheckInterval, "interval", env.GetDurationVal("IMAGE_UPDATER_INTERVAL", 2*time.Minute), "interval for how often to check for updates")
	runCmd.Flags().StringVar(&cfg.LogLevel, "loglevel", env.GetStringVal("IMAGE_UPDATER_LOGLEVEL", "info"), "set the loglevel to one of trace|debug|info|warn|error")
	runCmd.Flags().StringVar(&cfg.LogFormat, "logformat", env.GetStringVal("IMAGE_UPDATER_LOGFORMAT", "text"), "set the log format to one of text|json")
	runCmd.Flags().StringVar(&kubeConfig, "kubeconfig", "", "full path to kubernetes client configuration, i.e. ~/.kube/config")
	runCmd.Flags().IntVar(&cfg.HealthPort, "health-port", 8080, "port to start the health server on, 0 to disable")
	runCmd.Flags().IntVar(&cfg.MetricsPort, "metrics-port", 8081, "port to start the metrics server on, 0 to disable")
	runCmd.Flags().BoolVar(&once, "once", false, "run only once, same as specifying --interval=0 and --health-port=0")
	runCmd.Flags().StringVar(&cfg.RegistriesConf, "registries-conf-path", defaultRegistriesConfPath, "path to registries configuration file")
	runCmd.Flags().IntVar(&cfg.MaxConcurrency, "max-concurrency", 10, "maximum number of update threads to run concurrently")
	runCmd.Flags().StringVar(&cfg.ArgocdNamespace, "argocd-namespace", "", "namespace where ArgoCD runs in (current namespace by default)")
	runCmd.Flags().StringVar(&cfg.AppNamespace, "application-namespace", v1.NamespaceAll, "namespace where Argo Image Updater will manage applications (all namespaces by default)")

	// DEPRECATED: These flags have been removed in the CRD branch and will be deprecated and removed in a future release.
	// The CRD branch introduces a new architecture that eliminates the need for these application matching flags.
	runCmd.Flags().StringSliceVar(&cfg.AppNamePatterns, "match-application-name", nil, "patterns to match application name against. DEPRECATED: this flag will be removed in a future version.")
	runCmd.Flags().StringVar(&cfg.AppLabel, "match-application-label", "", "label selector to match application labels against. DEPRECATED: this flag will be removed in a future version.")

	runCmd.Flags().BoolVar(&warmUpCache, "warmup-cache", true, "whether to perform a cache warm-up on startup")
	runCmd.Flags().StringVar(&cfg.GitCommitUser, "git-commit-user", env.GetStringVal("GIT_COMMIT_USER", "argocd-image-updater"), "Username to use for Git commits")
	runCmd.Flags().StringVar(&cfg.GitCommitMail, "git-commit-email", env.GetStringVal("GIT_COMMIT_EMAIL", "noreply@argoproj.io"), "E-Mail address to use for Git commits")
	runCmd.Flags().StringVar(&cfg.GitCommitSigningKey, "git-commit-signing-key", env.GetStringVal("GIT_COMMIT_SIGNING_KEY", ""), "GnuPG key ID or path to Private SSH Key used to sign the commits")
	runCmd.Flags().StringVar(&cfg.GitCommitSigningMethod, "git-commit-signing-method", env.GetStringVal("GIT_COMMIT_SIGNING_METHOD", "openpgp"), "Method used to sign Git commits ('openpgp' or 'ssh')")
	runCmd.Flags().BoolVar(&cfg.GitCommitSignOff, "git-commit-sign-off", env.GetBoolVal("GIT_COMMIT_SIGN_OFF", false), "Whether to sign-off git commits")
	runCmd.Flags().StringVar(&commitMessagePath, "git-commit-message-path", defaultCommitTemplatePath, "Path to a template to use for Git commit messages")
	runCmd.Flags().BoolVar(&cfg.DisableKubeEvents, "disable-kube-events", env.GetBoolVal("IMAGE_UPDATER_KUBE_EVENTS", false), "Disable kubernetes events")
	runCmd.Flags().BoolVar(&cfg.EnableWebhook, "enable-webhook", env.GetBoolVal("ENABLE_WEBHOOK", false), "Enable webhook server for receiving registry events")

	runCmd.Flags().IntVar(&webhookCfg.Port, "webhook-port", env.ParseNumFromEnv("WEBHOOK_PORT", 8082, 0, 65535), "Port to listen on for webhook events")
	runCmd.Flags().StringVar(&webhookCfg.DockerSecret, "docker-webhook-secret", env.GetStringVal("DOCKER_WEBHOOK_SECRET", ""), "Secret for validating Docker Hub webhooks")
	runCmd.Flags().StringVar(&webhookCfg.GHCRSecret, "ghcr-webhook-secret", env.GetStringVal("GHCR_WEBHOOK_SECRET", ""), "Secret for validating GitHub Container Registry webhooks")
	runCmd.Flags().StringVar(&webhookCfg.QuaySecret, "quay-webhook-secret", env.GetStringVal("QUAY_WEBHOOK_SECRET", ""), "Secret for validating Quay webhooks")
	runCmd.Flags().StringVar(&webhookCfg.HarborSecret, "harbor-webhook-secret", env.GetStringVal("HARBOR_WEBHOOK_SECRET", ""), "Secret for validating Harbor webhooks")
	runCmd.Flags().IntVar(&webhookCfg.RateLimitNumAllowedRequests, "webhook-ratelimit-allowed", env.ParseNumFromEnv("WEBHOOK_RATELIMIT_ALLOWED", 0, 0, math.MaxInt), "The number of allowed requests in an hour for webhook rate limiting, setting to 0 disables ratelimiting")

	return runCmd
}

// Main loop for argocd-image-controller
func runImageUpdater(cfg *ImageUpdaterConfig, warmUp bool) (argocd.ImageUpdaterResult, error) {
	result := argocd.ImageUpdaterResult{}
	var err error
	var argoClient argocd.ArgoCD
	switch cfg.ApplicationsAPIKind {
	case applicationsAPIKindK8S:
		argoClient, err = argocd.NewK8SClient(cfg.KubeClient, &argocd.K8SClientOptions{AppNamespace: cfg.AppNamespace})
	case applicationsAPIKindArgoCD:
		argoClient, err = argocd.NewAPIClient(&cfg.ClientOpts)
	default:
		return argocd.ImageUpdaterResult{}, fmt.Errorf("application api '%s' is not supported", cfg.ApplicationsAPIKind)
	}
	if err != nil {
		return result, err
	}
	cfg.ArgoClient = argoClient

	apps, err := cfg.ArgoClient.ListApplications(cfg.AppLabel)
	if err != nil {
		log.WithContext().
			AddField("argocd_server", cfg.ClientOpts.ServerAddr).
			AddField("grpc_web", cfg.ClientOpts.GRPCWeb).
			AddField("grpc_webroot", cfg.ClientOpts.GRPCWebRootPath).
			AddField("plaintext", cfg.ClientOpts.Plaintext).
			AddField("insecure", cfg.ClientOpts.Insecure).
			Errorf("error while communicating with ArgoCD")
		return result, err
	}

	// Get the list of applications that are allowed for updates, that is, those
	// applications which have correct annotation.
	appList, err := argocd.FilterApplicationsForUpdate(apps, cfg.AppNamePatterns)
	if err != nil {
		return result, err
	}

	metrics.Applications().SetNumberOfApplications(len(appList))

	if !warmUp {
		log.Infof("Starting image update cycle, considering %d annotated application(s) for update", len(appList))
	}

	syncState := argocd.NewSyncIterationState()

	// Allow a maximum of MaxConcurrency number of goroutines to exist at the
	// same time. If in warm-up mode, set to 1 explicitly.
	var concurrency int = cfg.MaxConcurrency
	if warmUp {
		concurrency = 1
	}
	var dryRun bool = cfg.DryRun
	if warmUp {
		dryRun = true
	}
	sem := semaphore.NewWeighted(int64(concurrency))

	var wg sync.WaitGroup
	wg.Add(len(appList))

	for app, curApplication := range appList {
		lockErr := sem.Acquire(context.Background(), 1)
		if lockErr != nil {
			log.Errorf("Could not acquire semaphore for application %s: %v", app, lockErr)
			// Release entry in wait group on error, too - we're never gonna execute
			wg.Done()
			continue
		}

		go func(app string, curApplication argocd.ApplicationImages) {
			defer sem.Release(1)
			log.Debugf("Processing application %s", app)
			upconf := &argocd.UpdateConfiguration{
				NewRegFN:               registry.NewClient,
				ArgoClient:             cfg.ArgoClient,
				KubeClient:             cfg.KubeClient,
				UpdateApp:              &curApplication,
				DryRun:                 dryRun,
				GitCommitUser:          cfg.GitCommitUser,
				GitCommitEmail:         cfg.GitCommitMail,
				GitCommitMessage:       cfg.GitCommitMessage,
				GitCommitSigningKey:    cfg.GitCommitSigningKey,
				GitCommitSigningMethod: cfg.GitCommitSigningMethod,
				GitCommitSignOff:       cfg.GitCommitSignOff,
				DisableKubeEvents:      cfg.DisableKubeEvents,
				GitCreds:               cfg.GitCreds,
			}
			res := argocd.UpdateApplication(upconf, syncState)
			result.NumApplicationsProcessed += 1
			result.NumErrors += res.NumErrors
			result.NumImagesConsidered += res.NumImagesConsidered
			result.NumImagesUpdated += res.NumImagesUpdated
			result.NumSkipped += res.NumSkipped
			if !warmUp && !cfg.DryRun {
				metrics.Applications().IncreaseImageUpdate(app, res.NumImagesUpdated)
			}
			metrics.Applications().IncreaseUpdateErrors(app, res.NumErrors)
			metrics.Applications().SetNumberOfImagesWatched(app, res.NumImagesConsidered)
			wg.Done()
		}(app, curApplication)
	}

	// Wait for all goroutines to finish
	wg.Wait()

	return result, nil
}

// warmupImageCache performs a cache warm-up, which is basically one cycle of
// the image update process with dryRun set to true and a maximum concurrency
// of 1, i.e. sequential processing.
func warmupImageCache(cfg *ImageUpdaterConfig) error {
	log.Infof("Warming up image cache")
	_, err := runImageUpdater(cfg, true)
	if err != nil {
		return nil
	}
	entries := 0
	eps := registry.ConfiguredEndpoints()
	for _, ep := range eps {
		r, err := registry.GetRegistryEndpoint(&image.ContainerImage{RegistryURL: ep})
		if err == nil {
			entries += r.Cache.NumEntries()
		}
	}
	log.Infof("Finished cache warm-up, pre-loaded %d meta data entries from %d registries", entries, len(eps))
	return nil
}
