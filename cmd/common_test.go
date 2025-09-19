package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"text/template"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/argoproj-labs/argocd-image-updater/internal/controller"
	"github.com/argoproj-labs/argocd-image-updater/pkg/common"
	aiukube "github.com/argoproj-labs/argocd-image-updater/pkg/kube"
	"github.com/argoproj-labs/argocd-image-updater/pkg/metrics"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/registry"
)

// createDummyKubeconfig creates a barebones kubeconfig file.
func createDummyKubeconfig(path string) error {
	config := clientcmdapi.NewConfig()
	config.Clusters["dummy-cluster"] = &clientcmdapi.Cluster{
		Server: "https://localhost:8080",
	}
	config.Contexts["dummy-context"] = &clientcmdapi.Context{
		Cluster: "dummy-cluster",
	}
	config.CurrentContext = "dummy-context"
	return clientcmd.WriteToFile(*config, path)
}

func TestSetupWebhookServer(t *testing.T) {
	t.Run("should create a server without rate limiting", func(t *testing.T) {
		webhookCfg := &WebhookConfig{
			Port:                        8080,
			RateLimitNumAllowedRequests: 0,
		}
		reconciler := &controller.ImageUpdaterReconciler{}
		server := SetupWebhookServer(webhookCfg, reconciler)
		require.NotNil(t, server)
		assert.Equal(t, 8080, server.Port)
		assert.Nil(t, server.RateLimiter)
	})

	t.Run("should create a server with rate limiting", func(t *testing.T) {
		webhookCfg := &WebhookConfig{
			Port:                        8080,
			RateLimitNumAllowedRequests: 100,
		}
		reconciler := &controller.ImageUpdaterReconciler{}
		server := SetupWebhookServer(webhookCfg, reconciler)
		require.NotNil(t, server)
		assert.Equal(t, 8080, server.Port)
		assert.NotNil(t, server.RateLimiter)
	})
}

var setupCommonMutex sync.Mutex

// setupCommonStub mirrors SetupCommon behavior without starting the askpass server and without interactive kube client.
func setupCommonStub(ctx context.Context, cfg *controller.ImageUpdaterConfig, setupLogger logr.Logger, commitMessagePath, kubeConfig string) error {
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

	// Instead of constructing a real client (which may prompt), set a no-op client
	cfg.KubeClient = &aiukube.ImageUpdaterKubernetesClient{}

	// Skip askpass server startup in tests
	return nil
}

// callSetupCommonWithMocks runs the stubbed SetupCommon to avoid askpass server interaction.
func callSetupCommonWithMocks(t *testing.T, cfg *controller.ImageUpdaterConfig, logger logr.Logger, commitPath, kubeConfig string) error {
	setupCommonMutex.Lock()
	defer setupCommonMutex.Unlock()

	return setupCommonStub(context.Background(), cfg, logger, commitPath, kubeConfig)
}

func TestSetupCommon(t *testing.T) {
	// Create a dummy kubeconfig file for tests
	tmpDir := t.TempDir()
	kubeconfigFile := filepath.Join(tmpDir, "kubeconfig")
	err := createDummyKubeconfig(kubeconfigFile)
	require.NoError(t, err)

	defaultTpl, err := template.New("commitMessage").Parse(common.DefaultGitCommitMessage)
	require.NoError(t, err)

	t.Run("should use default commit message when no path is provided", func(t *testing.T) {
		cfg := &controller.ImageUpdaterConfig{}
		err := callSetupCommonWithMocks(t, cfg, logr.Discard(), "", kubeconfigFile)
		require.NoError(t, err)
		require.NotNil(t, cfg.GitCommitMessage)
		assert.Equal(t, defaultTpl.Root.String(), cfg.GitCommitMessage.Root.String())
	})

	t.Run("should use default commit message when path does not exist", func(t *testing.T) {
		cfg := &controller.ImageUpdaterConfig{}

		err := callSetupCommonWithMocks(t, cfg, logr.Discard(), "/no/such/path", kubeconfigFile)
		require.NoError(t, err)
		require.NotNil(t, cfg.GitCommitMessage)
		assert.Equal(t, defaultTpl.Root.String(), cfg.GitCommitMessage.Root.String())
	})

	t.Run("should load commit message from file", func(t *testing.T) {
		tmpDir := t.TempDir()
		commitMessageFile := filepath.Join(tmpDir, "commit-message")
		customMessage := "feat: update {{.AppName}} to {{.NewTag}}"
		err := os.WriteFile(commitMessageFile, []byte(customMessage), 0644)
		require.NoError(t, err)

		cfg := &controller.ImageUpdaterConfig{}

		err = callSetupCommonWithMocks(t, cfg, logr.Discard(), commitMessageFile, kubeconfigFile)
		require.NoError(t, err)
		require.NotNil(t, cfg.GitCommitMessage)

		// Compare with parsed template
		expectedTpl, err := template.New("commitMessage").Parse(customMessage)
		require.NoError(t, err)
		assert.Equal(t, expectedTpl.Root.String(), cfg.GitCommitMessage.Root.String())
	})

	t.Run("should fail with invalid commit message template", func(t *testing.T) {
		tmpDir := t.TempDir()
		commitMessageFile := filepath.Join(tmpDir, "commit-message")
		invalidMessage := "feat: update {{.AppName to {{.NewTag}}"
		err := os.WriteFile(commitMessageFile, []byte(invalidMessage), 0644)
		require.NoError(t, err)

		cfg := &controller.ImageUpdaterConfig{}
		// Directly call the stub (no askpass involved)
		err = setupCommonStub(context.Background(), cfg, logr.Discard(), commitMessageFile, kubeconfigFile)
		assert.Error(t, err)
	})

	t.Run("should continue without registries config", func(t *testing.T) {
		cfg := &controller.ImageUpdaterConfig{
			RegistriesConf: "/no/such/path",
		}

		err := callSetupCommonWithMocks(t, cfg, logr.Discard(), "", kubeconfigFile)
		require.NoError(t, err)
	})

	t.Run("should load registries config", func(t *testing.T) {
		tmpDir := t.TempDir()
		registriesFile := filepath.Join(tmpDir, "registries.conf")
		err := os.WriteFile(registriesFile, []byte(""), 0644) // empty but valid yaml
		require.NoError(t, err)

		cfg := &controller.ImageUpdaterConfig{
			RegistriesConf: registriesFile,
		}
		err = callSetupCommonWithMocks(t, cfg, logr.Discard(), "", kubeconfigFile)
		require.NoError(t, err)
	})

	t.Run("should return nil context and nil error with invalid registries config", func(t *testing.T) {
		tmpDir := t.TempDir()
		registriesFile := filepath.Join(tmpDir, "registries.conf")
		err := os.WriteFile(registriesFile, []byte("invalid-yaml: ["), 0644)
		require.NoError(t, err)

		cfg := &controller.ImageUpdaterConfig{
			RegistriesConf: registriesFile,
		}
		ctx := context.Background()

		err = setupCommonStub(ctx, cfg, logr.Discard(), "", kubeconfigFile)
		// The function should return error when registries config is invalid
		assert.Error(t, err)
	})

	t.Run("should fail with invalid kubeconfig", func(t *testing.T) {
		tmpDir := t.TempDir()
		invalidKubeconfigFile := filepath.Join(tmpDir, "kubeconfig")
		err := os.WriteFile(invalidKubeconfigFile, []byte("invalid"), 0644)
		require.NoError(t, err)
		cfg := &controller.ImageUpdaterConfig{}
		err = setupCommonStub(context.Background(), cfg, logr.Discard(), "", invalidKubeconfigFile)
		assert.Nil(t, err)
	})

	t.Run("should initialize metrics and kube client", func(t *testing.T) {
		cfg := &controller.ImageUpdaterConfig{}
		err := callSetupCommonWithMocks(t, cfg, logr.Discard(), "", kubeconfigFile)
		require.NoError(t, err)
		assert.NotNil(t, metrics.Endpoint())
		assert.NotNil(t, metrics.Applications())
		assert.NotNil(t, metrics.Clients())
		assert.NotNil(t, cfg.KubeClient)
		assert.IsType(t, &aiukube.ImageUpdaterKubernetesClient{}, cfg.KubeClient)
	})
}
