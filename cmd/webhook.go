package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"text/template"
	"time"

	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd"
	"github.com/argoproj-labs/argocd-image-updater/pkg/common"
	"github.com/argoproj-labs/argocd-image-updater/pkg/version"
	"github.com/argoproj-labs/argocd-image-updater/pkg/webhook"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/env"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/registry"

	"github.com/argoproj/argo-cd/v2/util/askpass"
	"github.com/spf13/cobra"
	"go.uber.org/ratelimit"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// WebhookConfig holds the options for the webhook server
type WebhookConfig struct {
	Port                        int
	DockerSecret                string
	GHCRSecret                  string
	QuaySecret                  string
	HarborSecret                string
	RateLimitNumAllowedRequests int
	GitLabSecret                string
}

// NewWebhookCommand creates a new webhook command
func NewWebhookCommand() *cobra.Command {
	var cfg *ImageUpdaterConfig = &ImageUpdaterConfig{}
	var webhookCfg *WebhookConfig = &WebhookConfig{}
	var kubeConfig string
	var disableKubernetes bool
	var commitMessagePath string
	var commitMessageTpl string
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
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := log.SetLogLevel(cfg.LogLevel); err != nil {
				return err
			}

			if cfg.MaxConcurrency < 1 {
				return fmt.Errorf("--max-concurrency must be greater than 1")
			}

			log.Infof("%s %s starting [loglevel:%s, webhookport:%s]",
				version.BinaryName(),
				version.Version(),
				strings.ToUpper(cfg.LogLevel),
				strconv.Itoa(webhookCfg.Port),
			)

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
				log.Debugf("Successfully parsed commit messege template")
				cfg.GitCommitMessage = tpl
			}

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

			err = runWebhook(cfg, webhookCfg)
			return err
		},
	}

	// DEPRECATED: These flags have been removed in the CRD branch and will be deprecated and removed in a future release.
	// The CRD branch introduces a new architecture that eliminates the need for these native ArgoCD client configuration flags.
	webhookCmd.Flags().StringVar(&cfg.ApplicationsAPIKind, "applications-api", env.GetStringVal("APPLICATIONS_API", applicationsAPIKindK8S), "API kind that is used to manage Argo CD applications ('kubernetes' or 'argocd'). DEPRECATED: this flag will be removed in a future version.")
	webhookCmd.Flags().StringVar(&cfg.ClientOpts.ServerAddr, "argocd-server-addr", env.GetStringVal("ARGOCD_SERVER", ""), "address of ArgoCD API server. DEPRECATED: this flag will be removed in a future version.")
	webhookCmd.Flags().BoolVar(&cfg.ClientOpts.GRPCWeb, "argocd-grpc-web", env.GetBoolVal("ARGOCD_GRPC_WEB", false), "use grpc-web for connection to ArgoCD. DEPRECATED: this flag will be removed in a future version.")
	webhookCmd.Flags().BoolVar(&cfg.ClientOpts.Insecure, "argocd-insecure", env.GetBoolVal("ARGOCD_INSECURE", false), "(INSECURE) ignore invalid TLS certs for ArgoCD server. DEPRECATED: this flag will be removed in a future version.")
	webhookCmd.Flags().BoolVar(&cfg.ClientOpts.Plaintext, "argocd-plaintext", env.GetBoolVal("ARGOCD_PLAINTEXT", false), "(INSECURE) connect without TLS to ArgoCD server. DEPRECATED: this flag will be removed in a future version.")
	webhookCmd.Flags().StringVar(&cfg.ClientOpts.AuthToken, "argocd-auth-token", "", "use token for authenticating to ArgoCD (unsafe - consider setting ARGOCD_TOKEN env var instead). DEPRECATED: this flag will be removed in a future version.")
	webhookCmd.Flags().BoolVar(&disableKubernetes, "disable-kubernetes", false, "do not create and use a Kubernetes client. DEPRECATED: this flag will be removed in a future version.")

	// Set Image Updater flags
	webhookCmd.Flags().StringVar(&cfg.LogLevel, "loglevel", env.GetStringVal("IMAGE_UPDATER_LOGLEVEL", "info"), "set the loglevel to one of trace|debug|info|warn|error")
	webhookCmd.Flags().StringVar(&kubeConfig, "kubeconfig", "", "full path to kubernetes client configuration, i.e. ~/.kube/config")
	webhookCmd.Flags().StringVar(&cfg.RegistriesConf, "registries-conf-path", defaultRegistriesConfPath, "path to registries configuration file")
	webhookCmd.Flags().IntVar(&cfg.MaxConcurrency, "max-concurrency", 10, "maximum number of update threads to run concurrently")
	webhookCmd.Flags().StringVar(&cfg.ArgocdNamespace, "argocd-namespace", "", "namespace where ArgoCD runs in (current namespace by default)")
	webhookCmd.Flags().StringVar(&cfg.AppNamespace, "application-namespace", v1.NamespaceAll, "namespace where Argo Image Updater will manage applications (all namespaces by default)")

	// DEPRECATED: These flags have been removed in the CRD branch and will be deprecated and removed in a future release.
	// The CRD branch introduces a new architecture that eliminates the need for these application matching flags.
	webhookCmd.Flags().StringSliceVar(&cfg.AppNamePatterns, "match-application-name", nil, "patterns to match application name against. DEPRECATED: this flag will be removed in a future version.")
	webhookCmd.Flags().StringVar(&cfg.AppLabel, "match-application-label", "", "label selector to match application labels against. DEPRECATED: this flag will be removed in a future version.")

	webhookCmd.Flags().StringVar(&cfg.GitCommitUser, "git-commit-user", env.GetStringVal("GIT_COMMIT_USER", "argocd-image-updater"), "Username to use for Git commits")
	webhookCmd.Flags().StringVar(&cfg.GitCommitMail, "git-commit-email", env.GetStringVal("GIT_COMMIT_EMAIL", "noreply@argoproj.io"), "E-Mail address to use for Git commits")
	webhookCmd.Flags().StringVar(&cfg.GitCommitSigningKey, "git-commit-signing-key", env.GetStringVal("GIT_COMMIT_SIGNING_KEY", ""), "GnuPG key ID or path to Private SSH Key used to sign the commits")
	webhookCmd.Flags().StringVar(&cfg.GitCommitSigningMethod, "git-commit-signing-method", env.GetStringVal("GIT_COMMIT_SIGNING_METHOD", "openpgp"), "Method used to sign Git commits ('openpgp' or 'ssh')")
	webhookCmd.Flags().BoolVar(&cfg.GitCommitSignOff, "git-commit-sign-off", env.GetBoolVal("GIT_COMMIT_SIGN_OFF", false), "Whether to sign-off git commits")
	webhookCmd.Flags().StringVar(&commitMessagePath, "git-commit-message-path", defaultCommitTemplatePath, "Path to a template to use for Git commit messages")
	webhookCmd.Flags().BoolVar(&cfg.DisableKubeEvents, "disable-kube-events", env.GetBoolVal("IMAGE_UPDATER_KUBE_EVENTS", false), "Disable kubernetes events")

	webhookCmd.Flags().IntVar(&webhookCfg.Port, "webhook-port", env.ParseNumFromEnv("WEBHOOK_PORT", 8080, 0, 65535), "Port to listen on for webhook events")
	webhookCmd.Flags().StringVar(&webhookCfg.DockerSecret, "docker-webhook-secret", env.GetStringVal("DOCKER_WEBHOOK_SECRET", ""), "Secret for validating Docker Hub webhooks")
	webhookCmd.Flags().StringVar(&webhookCfg.GHCRSecret, "ghcr-webhook-secret", env.GetStringVal("GHCR_WEBHOOK_SECRET", ""), "Secret for validating GitHub Container Registry webhooks")
	webhookCmd.Flags().StringVar(&webhookCfg.QuaySecret, "quay-webhook-secret", env.GetStringVal("QUAY_WEBHOOK_SECRET", ""), "Secret for validating Quay webhooks")
	webhookCmd.Flags().StringVar(&webhookCfg.HarborSecret, "harbor-webhook-secret", env.GetStringVal("HARBOR_WEBHOOK_SECRET", ""), "Secret for validating Harbor webhooks")
	webhookCmd.Flags().IntVar(&webhookCfg.RateLimitNumAllowedRequests, "webhook-ratelimit-allowed", env.ParseNumFromEnv("WEBHOOK_RATELIMIT_ALLOWED", 0, 0, math.MaxInt), "The number of allowed requests in an hour for webhook rate limiting, setting to 0 disables ratelimiting")
	webhookCmd.Flags().StringVar(&webhookCfg.GitLabSecret, "gitlab-webhook-secret", env.GetStringVal("GITLAB_WEBHOOK_SECRET", ""), "Secret for validating GitLab Container Registry webhooks")

	return webhookCmd
}

// runWebhook starts the webhook server
func runWebhook(cfg *ImageUpdaterConfig, webhookCfg *WebhookConfig) error {
	log.Infof("Starting webhook server on port %d", webhookCfg.Port)

	// Initialize the ArgoCD client
	var err error

	// Create Kubernetes client
	cfg.KubeClient, err = getKubeConfig(context.TODO(), "", "")
	if err != nil {
		log.Fatalf("Could not create Kubernetes client: %v", err)
		return err
	}

	// Set up based on application API kind
	if cfg.ApplicationsAPIKind == applicationsAPIKindK8S {
		cfg.ArgoClient, err = argocd.NewK8SClient(cfg.KubeClient, &argocd.K8SClientOptions{AppNamespace: cfg.AppNamespace})
	} else {
		cfg.ArgoClient, err = argocd.NewAPIClient(&cfg.ClientOpts)
	}

	if err != nil {
		log.Fatalf("Could not create ArgoCD client: %v", err)
	}

	// Create webhook handler
	handler := webhook.NewWebhookHandler()

	// Register supported webhook handlers
	dockerHandler := webhook.NewDockerHubWebhook(webhookCfg.DockerSecret)
	handler.RegisterHandler(dockerHandler)

	ghcrHandler := webhook.NewGHCRWebhook(webhookCfg.GHCRSecret)
	handler.RegisterHandler(ghcrHandler)

	quayHandler := webhook.NewQuayWebhook(webhookCfg.QuaySecret)
	handler.RegisterHandler(quayHandler)

	harborHandler := webhook.NewHarborWebhook(webhookCfg.HarborSecret)
	handler.RegisterHandler(harborHandler)

	gitlabHandler := webhook.NewGitLabWebhook(webhookCfg.GitLabSecret)
	handler.RegisterHandler(gitlabHandler)

	// Create webhook server
	server := webhook.NewWebhookServer(webhookCfg.Port, handler, cfg.KubeClient, cfg.ArgoClient)

	if webhookCfg.RateLimitNumAllowedRequests > 0 {
		server.RateLimiter = ratelimit.New(webhookCfg.RateLimitNumAllowedRequests, ratelimit.Per(time.Hour))
	}

	// Set updater config
	server.UpdaterConfig = &argocd.UpdateConfiguration{
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

	// Set up graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// Start the server in a separate goroutine
	go func() {
		if err := server.Start(); err != nil {
			log.Fatalf("Failed to start webhook server: %v", err)
		}
	}()

	// Wait for interrupt signal
	<-stop

	// Gracefully shut down the server
	log.Infof("Shutting down webhook server")
	if err := server.Stop(); err != nil {
		log.Errorf("Error stopping webhook server: %v", err)
	}

	return nil
}
