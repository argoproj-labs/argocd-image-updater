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
	"github.com/argoproj-labs/argocd-image-updater/pkg/image"
	"github.com/argoproj-labs/argocd-image-updater/pkg/log"
	"github.com/argoproj-labs/argocd-image-updater/pkg/registry"
	"github.com/argoproj-labs/argocd-image-updater/pkg/version"

	"github.com/spf13/cobra"
	"golang.org/x/sync/semaphore"
)

var lastRun time.Time

// Default ArgoCD server address when running in same cluster as ArgoCD
const defaultArgoCDServerAddr = "argocd-server.argocd"

// ImageUpdaterConfig contains global configuration and required runtime data
type ImageUpdaterConfig struct {
	ClientOpts      argocd.ClientOptions
	ArgocdNamespace string
	DryRun          bool
	CheckInterval   time.Duration
	ArgoClient      *argocd.ArgoCD
	LogLevel        string
	KubeClient      *client.KubernetesClient
	MaxConcurrency  int
	HealthPort      int
	RegistriesConf  string
}

// Stores some statistics about the results of a run
type ImageUpdaterResult struct {
	NumApplicationsProcessed int
	NumImagesUpdated         int
	NumImagesConsidered      int
	NumSkipped               int
	NumErrors                int
}

// Update all images of a single application. Will run in a goroutine.
func updateApplication(argoClient *argocd.ArgoCD, kubeClient *client.KubernetesClient, curApplication *argocd.ApplicationImages, dryRun bool) ImageUpdaterResult {
	result := ImageUpdaterResult{}
	app := curApplication.Application.GetName()

	// Get all images that are deployed with the current application
	applicationImages := argocd.GetImagesFromApplication(&curApplication.Application)

	result.NumApplicationsProcessed += 1

	// Loop through all images of current application, and check whether one of
	// its images is eligible for updating.
	//
	// Whether an image qualifies for update is dependent on semantic version
	// constraints which are part of the application's annotation values.
	//
	for _, applicationImage := range applicationImages {
		updateableImage := curApplication.Images.ContainsImage(applicationImage, false)
		if updateableImage == nil {
			log.WithContext().AddField("application", app).Debugf("Image %s not in list of allowed images, skipping", applicationImage.ImageName)
			result.NumSkipped += 1
			continue
		}

		result.NumImagesConsidered += 1

		imgCtx := log.WithContext().
			AddField("application", app).
			AddField("registry", applicationImage.RegistryURL).
			AddField("image_name", applicationImage.ImageName).
			AddField("image_tag", applicationImage.ImageTag)

		imgCtx.Debugf("Considering this image for update")

		rep, err := registry.GetRegistryEndpoint(applicationImage.RegistryURL)
		if err != nil {
			imgCtx.Errorf("Could not get registry endpoint from configuration: %v", err)
			result.NumErrors += 1
			continue
		}

		var vc image.VersionConstraint
		if updateableImage.ImageTag != nil {
			vc.Constraint = updateableImage.ImageTag.TagName
			imgCtx.Debugf("Using version constraint '%s' when looking for a new tag", vc.Constraint)
		} else {
			imgCtx.Debugf("Using no version constraint when looking for a new tag")
		}

		vc.SortMode = updateableImage.GetParameterUpdateStrategy(curApplication.Application.Annotations)

		ep, err := registry.GetRegistryEndpoint(updateableImage.RegistryURL)
		if err != nil {
			imgCtx.Errorf("Could not get registry endpoint: %v", err)
			continue
		}

		err = ep.SetEndpointCredentials(kubeClient)
		if err != nil {
			imgCtx.Errorf("Could not set registry endpoint credentiasl: %v", err)
			continue
		}

		regClient, err := registry.NewClient(ep)
		if err != nil {
			imgCtx.Errorf("Could not create registry client: %v", err)
			continue
		}

		// Get list of available image tags from the repository
		tags, err := rep.GetTags(applicationImage, regClient, &vc)
		if err != nil {
			imgCtx.Errorf("Could not get tags from registry: %v", err)
			result.NumErrors += 1
			continue
		}

		imgCtx.Tracef("List of available tags found: %v", tags.Tags())

		// Get the latest available tag matching any constraint that might be set
		// for allowed updates.
		latest, err := applicationImage.GetNewestVersionFromTags(&vc, tags)
		if err != nil {
			imgCtx.Errorf("Unable to find newest version from available tags: %v", err)
			result.NumErrors += 1
			continue
		}

		// If we have no latest tag information, it means there was no tag which
		// has met our version constraint (or there was no semantic versioned tag
		// at all in the repository)
		if latest == nil {
			imgCtx.Debugf("No suitable image tag for upgrade found in list of available tags.")
			result.NumSkipped += 1
			continue
		}

		// If the latest tag does not match image's current tag, it means we have
		// an update candidate.
		if applicationImage.ImageTag.TagName != latest.TagName {
			if dryRun {
				imgCtx.Infof("Would upgrade image to %s, but this is a dry run. Skipping.", applicationImage.WithTag(latest).String())
				continue
			}

			imgCtx.Infof("Upgrading image to %s", applicationImage.WithTag(latest).String())

			if appType := argocd.GetApplicationType(&curApplication.Application); appType == argocd.ApplicationTypeKustomize {
				err = argoClient.SetKustomizeImage(&curApplication.Application, updateableImage.WithTag(latest))
			} else if appType == argocd.ApplicationTypeHelm {
				err = argoClient.SetHelmImage(&curApplication.Application, updateableImage.WithTag(latest))
			} else {
				result.NumErrors += 1
				err = fmt.Errorf("Could not update application %s - neither Helm nor Kustomize application", app)
			}

			if err != nil {
				imgCtx.Errorf("Error while trying to update image: %v", err)
				result.NumErrors += 1
				continue
			} else {
				imgCtx.Infof("Successfully updated image '%s' to '%s'", applicationImage.GetFullNameWithTag(), applicationImage.WithTag(latest).GetFullNameWithTag())
				result.NumImagesUpdated += 1
			}
		} else {
			imgCtx.Debugf("Image '%s' already on latest allowed version", applicationImage.GetFullNameWithTag())
		}
	}

	return result
}

// Main loop for argocd-image-controller
func runImageUpdater(cfg *ImageUpdaterConfig) (ImageUpdaterResult, error) {
	result := ImageUpdaterResult{}
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
	appList, err := argocd.FilterApplicationsForUpdate(apps)
	if err != nil {
		return result, err
	}

	log.Debugf("Considering %d applications with annotations for update", len(appList))

	// Allow a maximum of MaxConcurrency number of goroutines to exist at the
	// same time.
	sem := semaphore.NewWeighted(int64(cfg.MaxConcurrency))

	var wg sync.WaitGroup
	wg.Add(len(appList))

	for app, curApplication := range appList {
		lockErr := sem.Acquire(context.TODO(), 1)
		if lockErr != nil {
			log.Errorf("could not acquire semaphore for application %s: %v", app, lockErr)
			// Release entry in wait group on error, too - we're never gonna execute
			wg.Done()
			continue
		}

		go func(app string, curApplication argocd.ApplicationImages) {
			defer sem.Release(1)
			log.Debugf("Processing application %s", app)
			res := updateApplication(cfg.ArgoClient, cfg.KubeClient, &curApplication, cfg.DryRun)
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
					log.Warnf("Registry configuration at %s could not be read: %v -- using a default configuration", cfg.RegistriesConf, err)
				} else {
					err = registry.LoadRegistryConfiguration(cfg.RegistriesConf)
					if err != nil {
						log.Errorf("Could not load registry configuration from %s: %v", cfg.RegistriesConf, err)
						return nil
					}
				}
			}

			if cfg.CheckInterval > 0 && cfg.CheckInterval < 60*time.Second {
				log.Warnf("check interval is very low - it is not recommended to run below 1m0s")
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
						log.Debugf("Starting image update process")
						result, err := runImageUpdater(cfg)
						if err != nil {
							log.Errorf("Error: %v", err)
						} else if result.NumImagesUpdated > 0 || result.NumErrors > 0 {
							log.Infof("Processing results: applications=%d images_considered=%d images_updated=%d errors=%d",
								result.NumApplicationsProcessed,
								result.NumImagesConsidered,
								result.NumImagesUpdated,
								result.NumErrors)
						} else {
							log.Debugf("Processing results: applications=%d images_considered=%d images_skipped=%d images_updated=%d errors=%d",
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
	runCmd.Flags().StringVar(&cfg.LogLevel, "loglevel", "info", "set the loglevel to one of trace|debug|info|warn|error")
	runCmd.Flags().StringVar(&kubeConfig, "kubeconfig", "", "full path to kubernetes client configuration, i.e. ~/.kube/config")
	runCmd.Flags().IntVar(&cfg.HealthPort, "health-port", 8080, "port to start the health server on, 0 to disable")
	runCmd.Flags().BoolVar(&once, "once", false, "run only once, same as specifying --interval=0 and --health-port=0")
	runCmd.Flags().StringVar(&cfg.RegistriesConf, "registries-conf-path", "", "path to registries configuration file")
	runCmd.Flags().BoolVar(&disableKubernetes, "disable-kubernetes", false, "do not create and use a Kubernetes client")
	runCmd.Flags().IntVar(&cfg.MaxConcurrency, "max-concurrency", 10, "maximum number of update threads to run concurrently")
	runCmd.Flags().StringVar(&cfg.ArgocdNamespace, "argocd-namespace", "argocd", "namespace where ArgoCD runs in")

	return runCmd
}

func main() {
	err := newRootCommand()
	if err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}
