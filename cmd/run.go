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
    "sort"
    "runtime"

	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd"
	"github.com/argoproj-labs/argocd-image-updater/pkg/common"
	"github.com/argoproj-labs/argocd-image-updater/pkg/health"
	"github.com/argoproj-labs/argocd-image-updater/pkg/metrics"
	"github.com/argoproj-labs/argocd-image-updater/pkg/version"
	"github.com/argoproj-labs/argocd-image-updater/pkg/webhook"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/env"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/registry"

	"github.com/argoproj/argo-cd/v2/util/askpass"

	"github.com/spf13/cobra"

	"golang.org/x/sync/semaphore"

	"go.uber.org/ratelimit"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// allow tests to stub the application update function
var updateAppFn = argocd.UpdateApplication
var contState *argocd.SyncIterationState
var contMu sync.Mutex
var contInFlight = map[string]bool{}

// orderApplications reorders apps by schedule policy: lru (least-recent success first),
// fail-first (recent failures first), with optional cooldown to deprioritize recently
// successful apps.
func orderApplications(names []string, appList map[string]argocd.ApplicationImages, state *argocd.SyncIterationState, cfg *ImageUpdaterConfig) []string {
    stats := state.GetStats()
    type item struct{ name string; score int64 }
    items := make([]item, 0, len(names))
    now := time.Now()
    for _, n := range names {
        s := stats[n]
        score := int64(0)
        switch cfg.Schedule {
        case "lru":
            // Older success => higher priority (lower score)
            if !s.LastSuccess.IsZero() { score -= int64(now.Sub(s.LastSuccess).Milliseconds()) }
        case "fail-first":
            score += int64(s.FailCount) * 1_000_000 // dominate by failures
            if !s.LastAttempt.IsZero() { score -= int64(now.Sub(s.LastAttempt).Milliseconds()) }
        }
        if cfg.Cooldown > 0 && !s.LastSuccess.IsZero() && now.Sub(s.LastSuccess) < cfg.Cooldown {
            score -= 1 // slight deprioritization
        }
        items = append(items, item{name: n, score: score})
    }
    sort.Slice(items, func(i, j int) bool { return items[i].score > items[j].score })
    out := make([]string, len(items))
    for i := range items { out[i] = items[i].name }
    return out
}

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
			if err := log.SetLogLevel(cfg.LogLevel); err != nil {
				return err
			}

            if once {
				cfg.CheckInterval = 0
				cfg.HealthPort = 0
			}

            // Enforce sane --max-concurrency values (0=auto)
            if cfg.MaxConcurrency < 0 {
                return fmt.Errorf("--max-concurrency cannot be negative (0=auto)")
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

			if cfg.CheckInterval > 0 && cfg.CheckInterval < 60*time.Second && cfg.Mode != "continuous" {
				log.Warnf("Check interval is very low - it is not recommended to run below 1m0s")
			}

			var err error
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

            // Log effective runtime settings
            log.Infof("Runtime settings: mode=%s interval=%s max_concurrency=%d schedule=%s cooldown=%s per_repo_cap=%d health_port=%d metrics_port=%d registries_conf=%s",
                cfg.Mode,
                getPrintableInterval(cfg.CheckInterval),
                cfg.MaxConcurrency,
                cfg.Schedule,
                cfg.Cooldown.String(),
                cfg.PerRepoCap,
                cfg.HealthPort,
                cfg.MetricsPort,
                cfg.RegistriesConf,
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

                // GitLab Container Registry webhooks are not supported upstream

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
                    if cfg.Mode == "continuous" {
                        runContinuousOnce(cfg)
                        // continuous scheduler loops internally; tick at ~1s
                        time.Sleep(1 * time.Second)
                    } else {
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
				}
				if cfg.CheckInterval == 0 {
					break
				}
                time.Sleep(1 * time.Second)
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
	runCmd.Flags().StringVar(&kubeConfig, "kubeconfig", "", "full path to kubernetes client configuration, i.e. ~/.kube/config")
	runCmd.Flags().IntVar(&cfg.HealthPort, "health-port", 8080, "port to start the health server on, 0 to disable")
	runCmd.Flags().IntVar(&cfg.MetricsPort, "metrics-port", 8081, "port to start the metrics server on, 0 to disable")
	runCmd.Flags().BoolVar(&once, "once", false, "run only once, same as specifying --interval=0 and --health-port=0")
	runCmd.Flags().StringVar(&cfg.RegistriesConf, "registries-conf-path", defaultRegistriesConfPath, "path to registries configuration file")
	runCmd.Flags().IntVar(&cfg.MaxConcurrency, "max-concurrency", 10, "maximum number of update threads to run concurrently (0=auto)")
	runCmd.Flags().StringVar(&cfg.ArgocdNamespace, "argocd-namespace", "", "namespace where ArgoCD runs in (current namespace by default)")
	runCmd.Flags().StringVar(&cfg.AppNamespace, "application-namespace", v1.NamespaceAll, "namespace where Argo Image Updater will manage applications (all namespaces by default)")

	// DEPRECATED: These flags have been removed in the CRD branch and will be deprecated and removed in a future release.
	// The CRD branch introduces a new architecture that eliminates the need for these application matching flags.
	runCmd.Flags().StringSliceVar(&cfg.AppNamePatterns, "match-application-name", nil, "patterns to match application name against. DEPRECATED: this flag will be removed in a future version.")
	runCmd.Flags().StringVar(&cfg.AppLabel, "match-application-label", "", "label selector to match application labels against. DEPRECATED: this flag will be removed in a future version.")

	runCmd.Flags().BoolVar(&warmUpCache, "warmup-cache", true, "whether to perform a cache warm-up on startup")
	runCmd.Flags().StringVar(&cfg.Schedule, "schedule", env.GetStringVal("IMAGE_UPDATER_SCHEDULE", "default"), "scheduling policy: default|lru|fail-first")
	runCmd.Flags().DurationVar(&cfg.Cooldown, "cooldown", env.GetDurationVal("IMAGE_UPDATER_COOLDOWN", 0), "deprioritize apps updated within this duration")
	runCmd.Flags().IntVar(&cfg.PerRepoCap, "per-repo-cap", env.ParseNumFromEnv("IMAGE_UPDATER_PER_REPO_CAP", 0, 0, 100000), "max updates per repo per cycle (0 = unlimited)")
	runCmd.Flags().StringVar(&cfg.Mode, "mode", env.GetStringVal("IMAGE_UPDATER_MODE", "cycle"), "execution mode: cycle|continuous")
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
    // GitLab Container Registry webhooks are not supported upstream

	return runCmd
}

// Main loop for argocd-image-controller
func runImageUpdater(cfg *ImageUpdaterConfig, warmUp bool) (argocd.ImageUpdaterResult, error) {
	result := argocd.ImageUpdaterResult{}
	var err error
    cycleStart := time.Now()
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
    if concurrency == 0 { // auto
        // simple heuristic: 8x CPUs, capped to number of apps
        cpu := runtime.NumCPU()
        if cpu < 1 { cpu = 1 }
        concurrency = cpu * 8
        if concurrency > len(appList) { concurrency = len(appList) }
        if concurrency < 1 { concurrency = 1 }
        log.Infof("Auto concurrency selected: %d workers (cpus=%d apps=%d)", concurrency, cpu, len(appList))
    }
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

            // Optionally reorder apps by scheduling policy
            ordered := make([]string, 0, len(appList))
            for app := range appList { ordered = append(ordered, app) }
            if cfg.Schedule != "default" || cfg.Cooldown > 0 || cfg.PerRepoCap > 0 {
                ordered = orderApplications(ordered, appList, syncState, cfg)
            }

            perRepoCounter := map[string]int{}

            for _, app := range ordered {
                curApplication := appList[app]
                // Per-repo cap if configured
                if cfg.PerRepoCap > 0 {
                    repo := argocd.GetApplicationSource(&curApplication.Application).RepoURL
                    if perRepoCounter[repo] >= cfg.PerRepoCap {
                        continue
                    }
                }
                syncState.RecordAttempt(app)
                metrics.Applications().SetLastAttempt(app, time.Now())
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
                    appStart := time.Now()
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
            res := updateAppFn(upconf, syncState)
            metrics.Applications().ObserveAppUpdateDuration(app, time.Since(appStart))
            syncState.RecordResult(app, res.NumErrors > 0)
            if cfg.PerRepoCap > 0 {
                repo := argocd.GetApplicationSource(&curApplication.Application).RepoURL
                perRepoCounter[repo] = perRepoCounter[repo] + 1
            }
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
            if res.NumErrors == 0 {
                metrics.Applications().SetLastSuccess(app, time.Now())
            }
			wg.Done()
		}(app, curApplication)
	}

    // Wait for all goroutines to finish
	wg.Wait()
    metrics.Applications().ObserveCycleDuration(time.Since(cycleStart))
    metrics.Applications().SetCycleLastEnd(time.Now())

	return result, nil
}

// runContinuousOnce runs a non-blocking scheduling pass that launches or skips
// per-app workers based on last attempt time and the configured interval. Each
// app re-schedules independently; shared limits still apply downstream.
func runContinuousOnce(cfg *ImageUpdaterConfig) {
    apps, err := cfg.ArgoClient.ListApplications(cfg.AppLabel)
    if err != nil { log.Errorf("continuous: list apps error: %v", err); return }
    appList, err := argocd.FilterApplicationsForUpdate(apps, cfg.AppNamePatterns)
    if err != nil { log.Errorf("continuous: filter apps error: %v", err); return }

    // Build or fetch per-process state
    if contState == nil { contState = argocd.NewSyncIterationState() }
    syncState := contState
    ordered := make([]string, 0, len(appList))
    for a := range appList { ordered = append(ordered, a) }
    if cfg.Schedule != "default" || cfg.Cooldown > 0 || cfg.PerRepoCap > 0 {
        ordered = orderApplications(ordered, appList, syncState, cfg)
    }

    // Use auto-concurrency when set
    concurrency := cfg.MaxConcurrency
    if concurrency == 0 {
        cpu := runtime.NumCPU(); if cpu < 1 { cpu = 1 }
        concurrency = cpu * 8
        if concurrency > len(appList) { concurrency = len(appList) }
        if concurrency < 1 { concurrency = 1 }
    }
    sem := semaphore.NewWeighted(int64(concurrency))

    now := time.Now()
    for _, name := range ordered {
        s := syncState.GetStats()[name]
        if !s.LastAttempt.IsZero() && now.Sub(s.LastAttempt) < cfg.CheckInterval {
            continue // not due yet
        }
        // don't double-dispatch same app
        contMu.Lock()
        if contInFlight[name] { contMu.Unlock(); continue }
        contInFlight[name] = true
        contMu.Unlock()
        if err := sem.Acquire(context.Background(), 1); err != nil { continue }
        cur := appList[name]
        syncState.RecordAttempt(name)
        if m := metrics.Applications(); m != nil {
            m.SetLastAttempt(name, time.Now())
        }
        go func(appName string, ai argocd.ApplicationImages) {
            defer sem.Release(1)
            defer func(){ contMu.Lock(); delete(contInFlight, appName); contMu.Unlock() }()
            start := time.Now()
            log.WithContext().AddField("application", appName).Infof("continuous: start processing")
            upconf := &argocd.UpdateConfiguration{
                NewRegFN:               registry.NewClient,
                ArgoClient:             cfg.ArgoClient,
                KubeClient:             cfg.KubeClient,
                UpdateApp:              &ai,
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
            res := updateAppFn(upconf, syncState)
            if m := metrics.Applications(); m != nil {
                m.ObserveAppUpdateDuration(appName, time.Since(start))
                if res.NumErrors == 0 { m.SetLastSuccess(appName, time.Now()) }
            }
            dur := time.Since(start)
            if res.NumErrors == 0 {
                log.WithContext().AddField("application", appName).Infof("continuous: finished processing: success, duration=%s", dur)
            } else {
                log.WithContext().AddField("application", appName).Infof("continuous: finished processing: failed, duration=%s", dur)
            }
        }(name, cur)
    }
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
		r, err := registry.GetRegistryEndpoint(ep)
		if err == nil {
			entries += r.Cache.NumEntries()
		}
	}
	log.Infof("Finished cache warm-up, pre-loaded %d meta data entries from %d registries", entries, len(eps))
	return nil
}
