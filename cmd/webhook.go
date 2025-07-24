package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd"
	"github.com/argoproj-labs/argocd-image-updater/pkg/kube"
	"github.com/argoproj-labs/argocd-image-updater/pkg/webhook"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"

	"github.com/argoproj/argo-cd/v2/util/askpass"

	"github.com/spf13/cobra"
)

// WebhookOptions holds the options for the webhook server
type WebhookOptions struct {
	Port                int
	DockerSecret        string
	GHCRSecret          string
	UpdateOnEvent       bool
	ApplicationsAPIKind string
	AppNamespace        string
	ServerAddr          string
	Insecure            bool
	Plaintext           bool
	GRPCWeb             bool
	AuthToken           string
}

var webhookOpts WebhookOptions

// NewWebhookCommand creates a new webhook command
func NewWebhookCommand() *cobra.Command {
	// !! for now just setting to default git credentials
	var cfg *ImageUpdaterConfig = &ImageUpdaterConfig{
		GitCommitUser: "argocd-image-updater",
		GitCommitMail: "noreplay@argoproj.io",
	}
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
`,
		// TODO: as mentioned this needs to be better inline with run
		Run: func(cmd *cobra.Command, args []string) {
			// !! this was just copy and pasted from run for now
			// Start up the credentials store server
			cs := askpass.NewServer(askpass.SocketPath)
			csErrCh := make(chan error)
			go func() {
				log.Debugf("Starting askpass server")
				csErrCh <- cs.Run()
			}()

			// Wait for cred server to be started, just in case
			err := <-csErrCh
			if err != nil {
				log.Errorf("Error running askpass server: %v", err)
				os.Exit(1) // TODO: NEED TO REFACTOR TO HAVE COMMAND TO RETURN ERROR
			}

			runWebhook(cfg)
		},
	}

	// TODO: Need to get the flags consistent with the run command ones
	webhookCmd.Flags().IntVar(&webhookOpts.Port, "port", 8080, "Port to listen on for webhook events")
	webhookCmd.Flags().StringVar(&webhookOpts.DockerSecret, "docker-secret", "", "Secret for validating Docker Hub webhooks")
	webhookCmd.Flags().StringVar(&webhookOpts.GHCRSecret, "ghcr-secret", "", "Secret for validating GitHub Container Registry webhooks")
	webhookCmd.Flags().BoolVar(&webhookOpts.UpdateOnEvent, "update-on-event", true, "Whether to trigger image update checks when webhook events are received")
	webhookCmd.Flags().StringVar(&webhookOpts.ApplicationsAPIKind, "applications-api", applicationsAPIKindK8S, "API kind that is used to manage Argo CD applications ('kubernetes' or 'argocd')")
	webhookCmd.Flags().StringVar(&webhookOpts.AppNamespace, "application-namespace", "", "namespace where Argo Image Updater will manage applications")
	webhookCmd.Flags().StringVar(&webhookOpts.ServerAddr, "argocd-server-addr", "", "address of ArgoCD API server")
	webhookCmd.Flags().BoolVar(&webhookOpts.Insecure, "argocd-insecure", false, "(INSECURE) ignore invalid TLS certs for ArgoCD server")
	webhookCmd.Flags().BoolVar(&webhookOpts.Plaintext, "argocd-plaintext", false, "(INSECURE) connect without TLS to ArgoCD server")
	webhookCmd.Flags().BoolVar(&webhookOpts.GRPCWeb, "argocd-grpc-web", false, "use grpc-web for connection to ArgoCD")
	webhookCmd.Flags().StringVar(&webhookOpts.AuthToken, "argocd-auth-token", "", "use token for authenticating to ArgoCD")

	return webhookCmd
}

// runWebhook starts the webhook server
func runWebhook(cfg *ImageUpdaterConfig) {
	log.Infof("Starting webhook server on port %d", webhookOpts.Port)

	// Initialize the ArgoCD client
	var argoClient argocd.ArgoCD
	var err error

	// Create Kubernetes client
	var kubeClient *kube.ImageUpdaterKubernetesClient
	kubeClient, err = getKubeConfig(context.TODO(), "", "")
	if err != nil {
		log.Fatalf("Could not create Kubernetes client: %v", err)
	}

	// Set up based on application API kind
	if webhookOpts.ApplicationsAPIKind == applicationsAPIKindK8S {
		argoClient, err = argocd.NewK8SClient(kubeClient, &argocd.K8SClientOptions{AppNamespace: webhookOpts.AppNamespace})
	} else {
		// Use defaults if not specified
		serverAddr := webhookOpts.ServerAddr
		if serverAddr == "" {
			serverAddr = defaultArgoCDServerAddr
		}

		// Check for auth token from environment if not provided
		authToken := webhookOpts.AuthToken
		if authToken == "" {
			if token := os.Getenv("ARGOCD_TOKEN"); token != "" {
				authToken = token
			}
		}

		clientOpts := argocd.ClientOptions{
			ServerAddr: serverAddr,
			Insecure:   webhookOpts.Insecure,
			Plaintext:  webhookOpts.Plaintext,
			GRPCWeb:    webhookOpts.GRPCWeb,
			AuthToken:  authToken,
		}
		argoClient, err = argocd.NewAPIClient(&clientOpts)
	}

	if err != nil {
		log.Fatalf("Could not create ArgoCD client: %v", err)
	}

	// Create webhook handler
	handler := webhook.NewWebhookHandler()

	// Register supported webhook handlers
	dockerHandler := webhook.NewDockerHubWebhook(webhookOpts.DockerSecret)
	handler.RegisterHandler(dockerHandler)

	ghcrHandler := webhook.NewGHCRWebhook(webhookOpts.GHCRSecret)
	handler.RegisterHandler(ghcrHandler)

	quayHandler := webhook.NewQuayWebhook("")
	handler.RegisterHandler(quayHandler)

	// Create webhook server
	server := webhook.NewWebhookServer(webhookOpts.Port, handler, kubeClient, argoClient)

	// Set updater config
	server.UpdaterConfig = &argocd.UpdaterConfig{
		GitCommitUser:  cfg.GitCommitUser,
		GitCommitEmail: cfg.GitCommitMail,
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
}
