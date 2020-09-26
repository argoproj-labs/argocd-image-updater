package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd"
	"github.com/argoproj-labs/argocd-image-updater/pkg/client"
	"github.com/argoproj-labs/argocd-image-updater/pkg/env"
	"github.com/argoproj-labs/argocd-image-updater/pkg/health"
	"github.com/argoproj-labs/argocd-image-updater/pkg/log"
	"github.com/argoproj-labs/argocd-image-updater/pkg/registry"
	"github.com/argoproj-labs/argocd-image-updater/pkg/version"

	"github.com/spf13/cobra"
	"golang.org/x/sync/semaphore"
)

var lastRun time.Time

// Default ArgoCD server address when running in same cluster as ArgoCD
const defaultArgoCDServerAddr = "argocd-server.argocd"

// Default path to registry configuration
const defaultRegistriesConfPath = "/app/config/registries.conf"

// ImageUpdaterConfig contains global configuration and required runtime data
type ImageUpdaterConfig struct {
	ClientOpts      argocd.ClientOptions
	ArgocdNamespace string
	DryRun          bool
	CheckInterval   time.Duration
	ArgoClient      argocd.ArgoCD
	LogLevel        string
	KubeClient      *client.KubernetesClient
	MaxConcurrency  int
	HealthPort      int
	RegistriesConf  string
	AppNamePatterns []string
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

// Main loop for argocd-image-controller
func runImageUpdater(cfg *ImageUpdaterConfig, warmUp bool) (argocd.ImageUpdaterResult, error) {
	result := argocd.ImageUpdaterResult{}
	argoClient, err := argocd.NewClient(&cfg.ClientOpts)
	if err != nil {
		return result, err
	}
	cfg.ArgoClient = argoClient

	apps, err := cfg.ArgoClient.ListApplications()
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

	if !warmUp {
		log.Infof("Starting image update cycle, considering %d annotated application(s) for update", len(appList))
	}

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
		lockErr := sem.Acquire(context.TODO(), 1)
		if lockErr != nil {
			log.Errorf("Could not acquire semaphore for application %s: %v", app, lockErr)
			// Release entry in wait group on error, too - we're never gonna execute
			wg.Done()
			continue
		}

		go func(app string, curApplication argocd.ApplicationImages) {
			defer sem.Release(1)
			log.Debugf("Processing application %s", app)
			res := argocd.UpdateApplication(registry.NewClient, cfg.ArgoClient, cfg.KubeClient, &curApplication, dryRun)
			result.NumApplicationsProcessed += 1
			result.NumErrors += res.NumErrors
			result.NumImagesConsidered += res.NumImagesConsidered
			result.NumImagesUpdated += res.NumImagesUpdated
			result.NumSkipped += res.NumSkipped
			wg.Done()
		}(app, curApplication)
	}

	// Wait for all goroutines to finish
	wg.Wait()

	return result, nil
}

func getPrintableInterval(interval time.Duration) string {
	if interval == 0 {
		return "once"
	} else {
		return interval.String()
	}
}

func getPrintableHealthPort(port int) string {
	if port == 0 {
		return "off"
	} else {
		return fmt.Sprintf("%d", port)
	}
}

// newRootCommand implements the root command of argocd-image-updater
func newRootCommand() error {
	var rootCmd = &cobra.Command{
		Use:   "argocd-image-updater",
		Short: "Automatically update container images with ArgoCD",
	}
	rootCmd.AddCommand(newRunCommand())
	rootCmd.AddCommand(newVersionCommand())
	err := rootCmd.Execute()
	return err
}

// newVersionCommand implements "version" command
func newVersionCommand() *cobra.Command {
	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Display version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Printf("%s\n", version.Useragent())
			return nil
		},
	}

	return versionCmd
}

// newRunCommand implements "run" command
func newRunCommand() *cobra.Command {
	var cfg *ImageUpdaterConfig = &ImageUpdaterConfig{}
	var once bool
	var kubeConfig string
	var disableKubernetes bool
	var warmUpCache bool = true
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

			var fullKubeConfigPath string
			var err error

			if !disableKubernetes {
				if kubeConfig != "" {
					fullKubeConfigPath, err = filepath.Abs(kubeConfig)
					if err != nil {
						log.Fatalf("Cannot expand path %s: %v", kubeConfig, err)
					}
				}

				if fullKubeConfigPath != "" {
					log.Debugf("Creating Kubernetes client from %s", fullKubeConfigPath)
				} else {
					log.Debugf("Creating in-cluster Kubernetes client")
				}

				cfg.KubeClient, err = client.NewKubernetesClient(fullKubeConfigPath)
				if err != nil {
					log.Fatalf("Cannot create kubernetes client: %v", err)
				}
			} else if kubeConfig != "" {
				return fmt.Errorf("--kubeconfig and --disable-kubernetes cannot be specified together")
			}

			if token := os.Getenv("ARGOCD_TOKEN"); token != "" && cfg.ClientOpts.AuthToken == "" {
				log.Debugf("Using ArgoCD API credentials from environment ARGOCD_TOKEN")
				cfg.ClientOpts.AuthToken = token
			}

			log.Infof("ArgoCD configuration: [server=%s, auth_token=%v, insecure=%v, grpc_web=%v, plaintext=%v]",
				cfg.ClientOpts.ServerAddr,
				cfg.ClientOpts.AuthToken != "",
				cfg.ClientOpts.Insecure,
				cfg.ClientOpts.GRPCWeb,
				cfg.ClientOpts.Plaintext,
			)

			// Health server will start in a go routine and run asynchronously
			var hsErrCh chan error
			if cfg.HealthPort > 0 {
				log.Infof("Starting health probe server TCP port=%d", cfg.HealthPort)
				hsErrCh = health.StartHealthServer(cfg.HealthPort)
			}

			if warmUpCache {
				err := warmupImageCache(cfg)
				if err != nil {
					log.Errorf("Error warming up cache: %v", err)
					return err
				}
			}

			// This is our main loop. We leave it only when our health probe server
			// returns an error.
			for {
				select {
				case err := <-hsErrCh:
					if err != nil {
						log.Errorf("Health probe server exited with error: %v", err)
						return nil
					} else {
						log.Infof("Health probe server exited gracefully")
					}
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

	runCmd.Flags().StringVar(&cfg.ClientOpts.ServerAddr, "argocd-server-addr", env.GetStringVal("ARGOCD_SERVER", defaultArgoCDServerAddr), "address of ArgoCD API server")
	runCmd.Flags().BoolVar(&cfg.ClientOpts.GRPCWeb, "argocd-grpc-web", env.GetBoolVal("ARGOCD_GRPC_WEB", false), "use grpc-web for connection to ArgoCD")
	runCmd.Flags().BoolVar(&cfg.ClientOpts.Insecure, "argocd-insecure", env.GetBoolVal("ARGOCD_INSECURE", false), "(INSECURE) ignore invalid TLS certs for ArgoCD server")
	runCmd.Flags().BoolVar(&cfg.ClientOpts.Plaintext, "argocd-plaintext", env.GetBoolVal("ARGOCD_PLAINTEXT", false), "(INSECURE) connect without TLS to ArgoCD server")
	runCmd.Flags().StringVar(&cfg.ClientOpts.AuthToken, "argocd-auth-token", "", "use token for authenticating to ArgoCD (unsafe - consider setting ARGOCD_TOKEN env var instead)")
	runCmd.Flags().BoolVar(&cfg.DryRun, "dry-run", false, "run in dry-run mode. If set to true, do not perform any changes")
	runCmd.Flags().DurationVar(&cfg.CheckInterval, "interval", 2*time.Minute, "interval for how often to check for updates")
	runCmd.Flags().StringVar(&cfg.LogLevel, "loglevel", env.GetStringVal("IMAGE_UPDATER_LOGLEVEL", "info"), "set the loglevel to one of trace|debug|info|warn|error")
	runCmd.Flags().StringVar(&kubeConfig, "kubeconfig", "", "full path to kubernetes client configuration, i.e. ~/.kube/config")
	runCmd.Flags().IntVar(&cfg.HealthPort, "health-port", 8080, "port to start the health server on, 0 to disable")
	runCmd.Flags().BoolVar(&once, "once", false, "run only once, same as specifying --interval=0 and --health-port=0")
	runCmd.Flags().StringVar(&cfg.RegistriesConf, "registries-conf-path", defaultRegistriesConfPath, "path to registries configuration file")
	runCmd.Flags().BoolVar(&disableKubernetes, "disable-kubernetes", false, "do not create and use a Kubernetes client")
	runCmd.Flags().IntVar(&cfg.MaxConcurrency, "max-concurrency", 10, "maximum number of update threads to run concurrently")
	runCmd.Flags().StringVar(&cfg.ArgocdNamespace, "argocd-namespace", "argocd", "namespace where ArgoCD runs in")
	runCmd.Flags().StringSliceVar(&cfg.AppNamePatterns, "match-application-name", nil, "patterns to match application name against")
	runCmd.Flags().BoolVar(&warmUpCache, "warmup-cache", true, "whether to perform a cache warm-up on startup")

	return runCmd
}

func main() {
	err := newRootCommand()
	if err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}
