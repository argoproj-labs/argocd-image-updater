package main

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd"
	"github.com/argoproj-labs/argocd-image-updater/pkg/common"
	"github.com/argoproj-labs/argocd-image-updater/pkg/env"
	"github.com/argoproj-labs/argocd-image-updater/pkg/health"
	"github.com/argoproj-labs/argocd-image-updater/pkg/image"
	"github.com/argoproj-labs/argocd-image-updater/pkg/kube"
	"github.com/argoproj-labs/argocd-image-updater/pkg/log"
	"github.com/argoproj-labs/argocd-image-updater/pkg/metrics"
	"github.com/argoproj-labs/argocd-image-updater/pkg/registry"
	"github.com/argoproj-labs/argocd-image-updater/pkg/tag"
	"github.com/argoproj-labs/argocd-image-updater/pkg/version"

	"github.com/spf13/cobra"
	"golang.org/x/sync/semaphore"
)

var lastRun time.Time

// Default ArgoCD server address when running in same cluster as ArgoCD
const defaultArgoCDServerAddr = "argocd-server.argocd"

// Default path to registry configuration
const defaultRegistriesConfPath = "/app/config/registries.conf"

// Default path to Git commit message template
const defaultCommitTemplatePath = "/app/config/commit.template"

const applicationsAPIKindK8S = "kubernetes"
const applicationsAPIKindArgoCD = "argocd"

// ImageUpdaterConfig contains global configuration and required runtime data
type ImageUpdaterConfig struct {
	ApplicationsAPIKind string
	ClientOpts          argocd.ClientOptions
	ArgocdNamespace     string
	DryRun              bool
	CheckInterval       time.Duration
	ArgoClient          argocd.ArgoCD
	LogLevel            string
	KubeClient          *kube.KubernetesClient
	MaxConcurrency      int
	HealthPort          int
	MetricsPort         int
	RegistriesConf      string
	AppNamePatterns     []string
	GitCommitUser       string
	GitCommitMail       string
	GitCommitMessage    *template.Template
	DisableKubeEvents   bool
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
	var err error
	var argoClient argocd.ArgoCD
	switch cfg.ApplicationsAPIKind {
	case applicationsAPIKindK8S:
		argoClient, err = argocd.NewK8SClient(cfg.KubeClient)
	case applicationsAPIKindArgoCD:
		argoClient, err = argocd.NewAPIClient(&cfg.ClientOpts)
	default:
		return argocd.ImageUpdaterResult{}, fmt.Errorf("application api '%s' is not supported", cfg.ApplicationsAPIKind)
	}
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
			upconf := &argocd.UpdateConfiguration{
				NewRegFN:          registry.NewClient,
				ArgoClient:        cfg.ArgoClient,
				KubeClient:        cfg.KubeClient,
				UpdateApp:         &curApplication,
				DryRun:            dryRun,
				GitCommitUser:     cfg.GitCommitUser,
				GitCommitEmail:    cfg.GitCommitMail,
				GitCommitMessage:  cfg.GitCommitMessage,
				DisableKubeEvents: cfg.DisableKubeEvents,
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
	rootCmd.AddCommand(newTestCommand())
	rootCmd.AddCommand(newTemplateCommand())
	err := rootCmd.Execute()
	return err
}

// newVersionCommand implements "version" command
func newVersionCommand() *cobra.Command {
	var short bool
	var versionCmd = &cobra.Command{
		Use:   "version",
		Short: "Display version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			if !short {
				fmt.Printf("%s\n", version.Useragent())
				fmt.Printf("  BuildDate: %s\n", version.BuildDate())
				fmt.Printf("  GitCommit: %s\n", version.GitCommit())
				fmt.Printf("  GoVersion: %s\n", version.GoVersion())
				fmt.Printf("  GoCompiler: %s\n", version.GoCompiler())
				fmt.Printf("  Platform: %s\n", version.GoPlatform())
			} else {
				fmt.Printf("%s\n", version.Version())
			}
			return nil
		},
	}
	versionCmd.Flags().BoolVar(&short, "short", false, "show only the version number")
	return versionCmd
}

func newTemplateCommand() *cobra.Command {
	var (
		commitMessageTemplatePath string
		tplStr                    string
	)
	var runCmd = &cobra.Command{
		Use:   "template [<PATH>]",
		Short: "Test & render a commit message template",
		Long: `
The template command lets you validate your commit message template. It will
parse the template at given PATH and execute it with a defined set of changes
so that you can see how it looks like when being templated by Image Updater.

If PATH is not given, will show you the default message that is used.
`,
		Run: func(cmd *cobra.Command, args []string) {
			var tpl *template.Template
			var err error
			if len(args) != 1 {
				tplStr = common.DefaultGitCommitMessage
			} else {
				commitMessageTemplatePath = args[0]
				tplData, err := ioutil.ReadFile(commitMessageTemplatePath)
				if err != nil {
					log.Fatalf("%v", err)
				}
				tplStr = string(tplData)
			}
			if tpl, err = template.New("commitMessage").Parse(tplStr); err != nil {
				log.Fatalf("could not parse commit message template: %v", err)
			}
			chL := []argocd.ChangeEntry{
				{
					Image:  image.NewFromIdentifier("gcr.io/example/example:1.0.0"),
					OldTag: tag.NewImageTag("1.0.0", time.Now(), ""),
					NewTag: tag.NewImageTag("1.0.1", time.Now(), ""),
				},
				{
					Image:  image.NewFromIdentifier("gcr.io/example/updater@sha256:f2ca1bb6c7e907d06dafe4687e579fce76b37e4e93b7605022da52e6ccc26fd2"),
					OldTag: tag.NewImageTag("", time.Now(), "sha256:01d09d19c2139a46aebfb577780d123d7396e97201bc7ead210a2ebff8239dee"),
					NewTag: tag.NewImageTag("", time.Now(), "sha256:7aa7a5359173d05b63cfd682e3c38487f3cb4f7f1d60659fe59fab1505977d4c"),
				},
			}
			fmt.Printf("%s\n", argocd.TemplateCommitMessage(tpl, "example-app", chL))
		},
	}
	return runCmd
}

func newTestCommand() *cobra.Command {
	var (
		semverConstraint  string
		strategy          string
		registriesConf    string
		logLevel          string
		allowTags         string
		credentials       string
		kubeConfig        string
		disableKubernetes bool
		ignoreTags        []string
		disableKubeEvents bool
	)
	var runCmd = &cobra.Command{
		Use:   "test IMAGE",
		Short: "Test the behaviour of argocd-image-updater",
		Long: `
The test command lets you test the behaviour of argocd-image-updater before
configuring annotations on your Argo CD Applications.

Its main use case is to tell you to which tag a given image would be updated
to using the given parametrization. Command line switches can be used as a
way to supply the required parameters.
`,
		Example: `
# In the most simple form, check for the latest available (semver) version of
# an image in the registry
argocd-image-updater test nginx

# Check to which version the nginx image within the 1.17 branch would be
# updated to, using the default semver strategy
argocd-image-updater test nginx --semver-constraint v1.17.x

# Check for the latest built image for a tag that matches a pattern
argocd-image-updater test nginx --allow-tags '^1.19.\d+(\-.*)*$' --update-strategy latest
`,
		Run: func(cmd *cobra.Command, args []string) {
			if len(args) != 1 {
				cmd.HelpFunc()(cmd, args)
				log.Fatalf("image needs to be specified")
			}

			if err := log.SetLogLevel(logLevel); err != nil {
				log.Fatalf("could not set log level to %s: %v", logLevel, err)
			}

			ctx := context.Background()

			var kubeClient *kube.KubernetesClient
			var err error
			if kubeConfig != "" {
				kubeClient, err = getKubeConfig(ctx, "", kubeConfig)
				if err != nil {
					log.Fatalf("could not create K8s client: %v", err)
				}
			}

			vc := &image.VersionConstraint{
				Constraint: semverConstraint,
				SortMode:   image.VersionSortSemVer,
			}

			vc.SortMode = image.ParseUpdateStrategy(strategy)

			if allowTags != "" {
				vc.MatchFunc, vc.MatchArgs = image.ParseMatchfunc(allowTags)
			}

			vc.IgnoreList = ignoreTags

			img := image.NewFromIdentifier(args[0])
			log.WithContext().
				AddField("registry", img.RegistryURL).
				AddField("image_name", img.ImageName).
				Infof("getting image")

			if registriesConf != "" {
				if err := registry.LoadRegistryConfiguration(registriesConf, false); err != nil {
					log.Fatalf("could not load registries configuration: %v", err)
				}
			}

			ep, err := registry.GetRegistryEndpoint(img.RegistryURL)
			if err != nil {
				log.Fatalf("could not get registry endpoint: %v", err)
			}

			if err := ep.SetEndpointCredentials(kubeClient); err != nil {
				log.Fatalf("could not set registry credentials: %v", err)
			}

			var creds *image.Credential
			var username, password string
			if credentials != "" {
				credSrc, err := image.ParseCredentialSource(credentials, false)
				if err != nil {
					log.Fatalf("could not parse credential definition '%s': %v", credentials, err)
				}
				creds, err = credSrc.FetchCredentials(img.RegistryURL, kubeClient)
				if err != nil {
					log.Fatalf("could not fetch credentials: %v", err)
				}
				username = creds.Username
				password = creds.Password
			}

			regClient, err := registry.NewClient(ep, username, password)
			if err != nil {
				log.Fatalf("could not create registry client: %v", err)
			}

			log.WithContext().
				AddField("image_name", img.ImageName).
				Infof("Fetching available tags and metadata from registry")

			tags, err := ep.GetTags(img, regClient, vc)
			if err != nil {
				log.Fatalf("could not get tags: %v", err)
			}

			log.WithContext().
				AddField("image_name", img.ImageName).
				Infof("Found %d tags in registry", len(tags.Tags()))

			upImg, err := img.GetNewestVersionFromTags(vc, tags)
			if err != nil {
				log.Fatalf("could not get updateable image from tags: %v", err)
			}
			if upImg == nil {
				log.Infof("no newer version of image found")
				return
			}

			log.Infof("latest image according to constraint is %s", img.WithTag(upImg))
		},
	}

	runCmd.Flags().StringVar(&semverConstraint, "semver-constraint", "", "only consider tags matching semantic version constraint")
	runCmd.Flags().StringVar(&allowTags, "allow-tags", "", "only consider tags in registry that satisfy the match function")
	runCmd.Flags().StringArrayVar(&ignoreTags, "ignore-tags", nil, "ignore tags in registry that match given glob pattern")
	runCmd.Flags().StringVar(&strategy, "update-strategy", "semver", "update strategy to use, one of: semver, latest)")
	runCmd.Flags().StringVar(&registriesConf, "registries-conf", "", "path to registries configuration")
	runCmd.Flags().StringVar(&logLevel, "loglevel", "debug", "log level to use (one of trace, debug, info, warn, error)")
	runCmd.Flags().BoolVar(&disableKubernetes, "disable-kubernetes", false, "whether to disable the Kubernetes client")
	runCmd.Flags().StringVar(&kubeConfig, "kubeconfig", "", "path to your Kubernetes client configuration")
	runCmd.Flags().StringVar(&credentials, "credentials", "", "the credentials definition for the test (overrides registry config)")
	runCmd.Flags().BoolVar(&disableKubeEvents, "disable-kubernetes-events", false, "Disable kubernetes events")
	return runCmd
}

// newRunCommand implements "run" command
func newRunCommand() *cobra.Command {
	var cfg *ImageUpdaterConfig = &ImageUpdaterConfig{}
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
				tpl, err := ioutil.ReadFile(commitMessagePath)
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

			var err error
			if !disableKubernetes {
				ctx := context.Background()
				cfg.KubeClient, err = getKubeConfig(ctx, cfg.ArgocdNamespace, kubeConfig)
				if err != nil {
					log.Fatalf("could not create K8s client: %v", err)
				}
				if cfg.ClientOpts.ServerAddr == "" {
					cfg.ClientOpts.ServerAddr = fmt.Sprintf("argocd-server.%s", cfg.KubeClient.Namespace)
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

			// Health server will start in a go routine and run asynchronously
			var hsErrCh chan error
			var msErrCh chan error
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
					return nil
				case err := <-msErrCh:
					if err != nil {
						log.Errorf("Metrics server exited with error: %v", err)
					} else {
						log.Infof("Metrics server exited gracefully")
					}
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

	runCmd.Flags().StringVar(&cfg.ApplicationsAPIKind, "applications-api", env.GetStringVal("APPLICATIONS_API", applicationsAPIKindK8S), "API kind that is used to manage Argo CD applications ('kubernetes' or 'argocd')")
	runCmd.Flags().StringVar(&cfg.ClientOpts.ServerAddr, "argocd-server-addr", env.GetStringVal("ARGOCD_SERVER", ""), "address of ArgoCD API server")
	runCmd.Flags().BoolVar(&cfg.ClientOpts.GRPCWeb, "argocd-grpc-web", env.GetBoolVal("ARGOCD_GRPC_WEB", false), "use grpc-web for connection to ArgoCD")
	runCmd.Flags().BoolVar(&cfg.ClientOpts.Insecure, "argocd-insecure", env.GetBoolVal("ARGOCD_INSECURE", false), "(INSECURE) ignore invalid TLS certs for ArgoCD server")
	runCmd.Flags().BoolVar(&cfg.ClientOpts.Plaintext, "argocd-plaintext", env.GetBoolVal("ARGOCD_PLAINTEXT", false), "(INSECURE) connect without TLS to ArgoCD server")
	runCmd.Flags().StringVar(&cfg.ClientOpts.AuthToken, "argocd-auth-token", "", "use token for authenticating to ArgoCD (unsafe - consider setting ARGOCD_TOKEN env var instead)")
	runCmd.Flags().BoolVar(&cfg.DryRun, "dry-run", false, "run in dry-run mode. If set to true, do not perform any changes")
	runCmd.Flags().DurationVar(&cfg.CheckInterval, "interval", 2*time.Minute, "interval for how often to check for updates")
	runCmd.Flags().StringVar(&cfg.LogLevel, "loglevel", env.GetStringVal("IMAGE_UPDATER_LOGLEVEL", "info"), "set the loglevel to one of trace|debug|info|warn|error")
	runCmd.Flags().StringVar(&kubeConfig, "kubeconfig", "", "full path to kubernetes client configuration, i.e. ~/.kube/config")
	runCmd.Flags().IntVar(&cfg.HealthPort, "health-port", 8080, "port to start the health server on, 0 to disable")
	runCmd.Flags().IntVar(&cfg.MetricsPort, "metrics-port", 8081, "port to start the metrics server on, 0 to disable")
	runCmd.Flags().BoolVar(&once, "once", false, "run only once, same as specifying --interval=0 and --health-port=0")
	runCmd.Flags().StringVar(&cfg.RegistriesConf, "registries-conf-path", defaultRegistriesConfPath, "path to registries configuration file")
	runCmd.Flags().BoolVar(&disableKubernetes, "disable-kubernetes", false, "do not create and use a Kubernetes client")
	runCmd.Flags().IntVar(&cfg.MaxConcurrency, "max-concurrency", 10, "maximum number of update threads to run concurrently")
	runCmd.Flags().StringVar(&cfg.ArgocdNamespace, "argocd-namespace", "", "namespace where ArgoCD runs in (current namespace by default)")
	runCmd.Flags().StringSliceVar(&cfg.AppNamePatterns, "match-application-name", nil, "patterns to match application name against")
	runCmd.Flags().BoolVar(&warmUpCache, "warmup-cache", true, "whether to perform a cache warm-up on startup")
	runCmd.Flags().StringVar(&cfg.GitCommitUser, "git-commit-user", env.GetStringVal("GIT_COMMIT_USER", "argocd-image-updater"), "Username to use for Git commits")
	runCmd.Flags().StringVar(&cfg.GitCommitMail, "git-commit-email", env.GetStringVal("GIT_COMMIT_EMAIL", "noreply@argoproj.io"), "E-Mail address to use for Git commits")
	runCmd.Flags().StringVar(&commitMessagePath, "git-commit-message-path", defaultCommitTemplatePath, "Path to a template to use for Git commit messages")
	runCmd.Flags().BoolVar(&cfg.DisableKubeEvents, "disable-kube-events", env.GetBoolVal("IMAGE_UPDATER_KUBE_EVENTS", false), "Disable kubernetes events")

	return runCmd
}

func getKubeConfig(ctx context.Context, namespace string, kubeConfig string) (*kube.KubernetesClient, error) {
	var fullKubeConfigPath string
	var kubeClient *kube.KubernetesClient
	var err error

	if kubeConfig != "" {
		fullKubeConfigPath, err = filepath.Abs(kubeConfig)
		if err != nil {
			return nil, fmt.Errorf("cannot expand path %s: %v", kubeConfig, err)
		}
	}

	if fullKubeConfigPath != "" {
		log.Debugf("Creating Kubernetes client from %s", fullKubeConfigPath)
	} else {
		log.Debugf("Creating in-cluster Kubernetes client")
	}

	kubeClient, err = kube.NewKubernetesClientFromConfig(ctx, namespace, fullKubeConfigPath)
	if err != nil {
		return nil, err
	}

	return kubeClient, nil
}

func main() {
	err := newRootCommand()
	if err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}
