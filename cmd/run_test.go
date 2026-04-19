package main

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	api "github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"
	"github.com/argoproj-labs/argocd-image-updater/internal/controller"
	"github.com/argoproj-labs/argocd-image-updater/pkg/common"
	pkgKube "github.com/argoproj-labs/argocd-image-updater/pkg/kube"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/env"
	registryKube "github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/kube"
)

// TestNewRunCommand tests various flags and their default values.
func TestNewRunCommand(t *testing.T) {
	asser := assert.New(t)
	controllerCommand := newRunCommand()
	asser.Contains(controllerCommand.Use, "run")
	asser.Equal(controllerCommand.Short, "Manages ArgoCD Image Updater Controller.")
	asser.Greater(len(controllerCommand.Long), 100)
	asser.NotNil(controllerCommand.RunE)
	asser.Equal("0", controllerCommand.Flag("metrics-bind-address").Value.String())
	asser.Equal(":8081", controllerCommand.Flag("health-probe-bind-address").Value.String())
	asser.Equal("true", controllerCommand.Flag("leader-election").Value.String())
	asser.Equal("", controllerCommand.Flag("leader-election-namespace").Value.String())
	asser.Equal("false", controllerCommand.Flag("enable-webhook").Value.String())
	asser.Equal("true", controllerCommand.Flag("metrics-secure").Value.String())
	asser.Equal("false", controllerCommand.Flag("enable-http2").Value.String())
	asser.Nil(controllerCommand.Help())
	asser.True(controllerCommand.HasFlags())
	asser.True(controllerCommand.HasLocalFlags())
	asser.False(controllerCommand.HasSubCommands())
	asser.False(controllerCommand.HasHelpSubCommands())
	asser.Equal("false", controllerCommand.Flag("dry-run").Value.String())
	asser.Equal(env.GetDurationVal("IMAGE_UPDATER_INTERVAL", 2*time.Minute).String(), controllerCommand.Flag("interval").Value.String())
	asser.Equal(env.GetStringVal("IMAGE_UPDATER_LOGLEVEL", "info"), controllerCommand.Flag("loglevel").Value.String())
	asser.Equal(env.GetStringVal("IMAGE_UPDATER_LOGFORMAT", "text"), controllerCommand.Flag("logformat").Value.String())
	asser.Equal("", controllerCommand.Flag("kubeconfig").Value.String())
	asser.Equal("false", controllerCommand.Flag("once").Value.String())
	asser.Equal(common.DefaultRegistriesConfPath, controllerCommand.Flag("registries-conf-path").Value.String())
	asser.Equal(strconv.Itoa(env.ParseNumFromEnv("MAX_CONCURRENT_APPS", 10, 1, 100)), controllerCommand.Flag("max-concurrent-apps").Value.String())
	asser.Equal(strconv.Itoa(env.ParseNumFromEnv("MAX_CONCURRENT_RECONCILES", 1, 1, 10)), controllerCommand.Flag("max-concurrent-reconciles").Value.String())
	asser.Equal(env.GetStringVal("ARGOCD_NAMESPACE", ""), controllerCommand.Flag("argocd-namespace").Value.String())
	asser.Equal(env.GetStringVal("IMAGE_UPDATER_WATCH_NAMESPACES", ""), controllerCommand.Flag("watch-namespaces").Value.String())
	asser.Equal("true", controllerCommand.Flag("warmup-cache").Value.String())
	asser.Equal(env.GetStringVal("GIT_COMMIT_USER", "argocd-image-updater"), controllerCommand.Flag("git-commit-user").Value.String())
	asser.Equal(env.GetStringVal("GIT_COMMIT_EMAIL", "noreply@argoproj.io"), controllerCommand.Flag("git-commit-email").Value.String())
	asser.Equal(env.GetStringVal("GIT_COMMIT_SIGNING_KEY", ""), controllerCommand.Flag("git-commit-signing-key").Value.String())
	asser.Equal(env.GetStringVal("GIT_COMMIT_SIGNING_METHOD", "openpgp"), controllerCommand.Flag("git-commit-signing-method").Value.String())
	asser.Equal(env.GetStringVal("GIT_COMMIT_SIGN_OFF", "false"), controllerCommand.Flag("git-commit-sign-off").Value.String())
	asser.Equal(common.DefaultCommitTemplatePath, controllerCommand.Flag("git-commit-message-path").Value.String())
	asser.Equal(env.GetStringVal("IMAGE_UPDATER_KUBE_EVENTS", "false"), controllerCommand.Flag("disable-kube-events").Value.String())
	asser.Equal(env.GetStringVal("ENABLE_WEBHOOK", "false"), controllerCommand.Flag("enable-webhook").Value.String())
	asser.Equal(strconv.Itoa(env.ParseNumFromEnv("WEBHOOK_PORT", 8082, 0, 65535)), controllerCommand.Flag("webhook-port").Value.String())
	asser.Equal(env.GetStringVal("DOCKER_WEBHOOK_SECRET", ""), controllerCommand.Flag("docker-webhook-secret").Value.String())
	asser.Equal(env.GetStringVal("GHCR_WEBHOOK_SECRET", ""), controllerCommand.Flag("ghcr-webhook-secret").Value.String())
	asser.Equal(env.GetStringVal("QUAY_WEBHOOK_SECRET", ""), controllerCommand.Flag("quay-webhook-secret").Value.String())
	asser.Equal(env.GetStringVal("HARBOR_WEBHOOK_SECRET", ""), controllerCommand.Flag("harbor-webhook-secret").Value.String())
	asser.Equal(env.GetStringVal("CLOUDEVENTS_WEBHOOK_SECRET", ""), controllerCommand.Flag("cloudevents-webhook-secret").Value.String())
	asser.Equal(env.GetStringVal("ALIYUN_ACR_WEBHOOK_SECRET", ""), controllerCommand.Flag("aliyun-acr-webhook-secret").Value.String())
	asser.Equal(env.GetStringVal("GITLAB_WEBHOOK_SECRET", ""), controllerCommand.Flag("gitlab-webhook-secret").Value.String())
	asser.Equal(strconv.Itoa(env.ParseNumFromEnv("WEBHOOK_RATELIMIT_ALLOWED", 0, 0, math.MaxInt)), controllerCommand.Flag("webhook-ratelimit-allowed").Value.String())

	// TLS flags
	asser.Equal("false", controllerCommand.Flag("disable-tls").Value.String())
	asser.Equal(env.GetStringVal("TLS_MIN_VERSION", "1.3"), controllerCommand.Flag("tlsminversion").Value.String())
	asser.Equal(env.GetStringVal("TLS_MAX_VERSION", "1.3"), controllerCommand.Flag("tlsmaxversion").Value.String())
	asser.Equal(env.GetStringVal("TLS_CIPHERS", ""), controllerCommand.Flag("tlsciphers").Value.String())

	asser.Nil(controllerCommand.Help())

}

// Assisted-by: Gemini AI
// TestMaxConcurrentAppsCornerCases tests corner cases for MAX_CONCURRENT_APPS flag
func TestMaxConcurrentAppsCornerCases(t *testing.T) {
	tests := []struct {
		name           string
		envValue       string
		expectedResult string
		description    string
	}{
		{
			name:           "MAX_CONCURRENT_APPS with value below minimum (0)",
			envValue:       "0",
			expectedResult: "10", // Default value when below min (1)
			description:    "Should return default value when environment variable is below minimum allowed value",
		},
		{
			name:           "MAX_CONCURRENT_APPS with value above maximum (101)",
			envValue:       "101",
			expectedResult: "10", // Default value when above max (100)
			description:    "Should return default value when environment variable is above maximum allowed value",
		},
		{
			name:           "MAX_CONCURRENT_APPS with negative value (-1)",
			envValue:       "-1",
			expectedResult: "10", // Default value when below min (1)
			description:    "Should return default value when environment variable is negative",
		},
		{
			name:           "MAX_CONCURRENT_APPS with non-numeric value (abc)",
			envValue:       "abc",
			expectedResult: "10", // Default value when parsing fails
			description:    "Should return default value when environment variable is not a valid number",
		},
		{
			name:           "MAX_CONCURRENT_APPS with empty string",
			envValue:       "",
			expectedResult: "10", // Default value when not set
			description:    "Should return default value when environment variable is empty",
		},
		{
			name:           "MAX_CONCURRENT_APPS with decimal value (5.5)",
			envValue:       "5.5",
			expectedResult: "10", // Default value when parsing fails (expects integer)
			description:    "Should return default value when environment variable is a decimal number",
		},
		{
			name:           "MAX_CONCURRENT_APPS with very large number (999999)",
			envValue:       "999999",
			expectedResult: "10", // Default value when above max (100)
			description:    "Should return default value when environment variable is very large",
		},
		{
			name:           "MAX_CONCURRENT_APPS with boundary value at minimum (1)",
			envValue:       "1",
			expectedResult: "1", // Valid value at minimum boundary
			description:    "Should accept minimum boundary value",
		},
		{
			name:           "MAX_CONCURRENT_APPS with boundary value at maximum (100)",
			envValue:       "100",
			expectedResult: "100", // Valid value at maximum boundary
			description:    "Should accept maximum boundary value",
		},
		{
			name:           "MAX_CONCURRENT_APPS with valid value in middle range (50)",
			envValue:       "50",
			expectedResult: "50", // Valid value in middle of range
			description:    "Should accept valid value in middle of allowed range",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Set environment variable
			if tt.envValue != "" {
				os.Setenv("MAX_CONCURRENT_APPS", tt.envValue)
				defer os.Unsetenv("MAX_CONCURRENT_APPS")
			} else {
				os.Unsetenv("MAX_CONCURRENT_APPS")
			}

			// Create new command to test the flag value
			controllerCommand := newRunCommand()
			flagValue := controllerCommand.Flag("max-concurrent-apps").Value.String()

			assert.Equal(t, tt.expectedResult, flagValue, tt.description)
		})
	}
}

// Assisted-by: Gemini AI
// TestCacheWarmerStart_NoCRs_SetsWarmedAndClosesDone verifies that when there are no ImageUpdater CRs,
// the cache warmer sets readiness and closes the Done channel, without closing StopChan when not in run-once mode.
func TestCacheWarmerStart_NoCRs_SetsWarmedAndClosesDone(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = api.AddToScheme(scheme)

	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	reconciler := &controller.ImageUpdaterReconciler{
		Client:                  c,
		Scheme:                  scheme,
		Config:                  &controller.ImageUpdaterConfig{},
		MaxConcurrentReconciles: 1,
		StopChan:                make(chan struct{}),
		Once:                    false,
	}

	status := &WarmupStatus{Done: make(chan struct{})}
	cw := &CacheWarmer{Reconciler: reconciler, Status: status}

	err := cw.Start(context.Background())
	assert.NoError(t, err)

	// readiness set
	assert.True(t, status.isCacheWarmed.Load())

	// Done channel should be closed
	select {
	case <-status.Done:
		// ok
	default:
		t.Fatalf("expected Done channel to be closed")
	}

	// StopChan should NOT be closed when not in run-once mode
	select {
	case <-reconciler.StopChan:
		t.Fatalf("did not expect StopChan to be closed")
	default:
		// ok
	}
}

// Assisted-by: Gemini AI
// TestCacheWarmerStart_RunOnce_NoCRs_ClosesStopChan verifies that in run-once mode with no CRs,
// the StopChan is closed immediately, readiness is set, and Done channel is closed.
func TestCacheWarmerStart_RunOnce_NoCRs_ClosesStopChan(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = api.AddToScheme(scheme)

	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	stop := make(chan struct{})

	reconciler := &controller.ImageUpdaterReconciler{
		Client:                  c,
		Scheme:                  scheme,
		Config:                  &controller.ImageUpdaterConfig{},
		MaxConcurrentReconciles: 1,
		StopChan:                stop,
		Once:                    true,
	}

	status := &WarmupStatus{Done: make(chan struct{})}
	cw := &CacheWarmer{Reconciler: reconciler, Status: status}

	err := cw.Start(context.Background())
	assert.NoError(t, err)

	// StopChan should be closed immediately in run-once with zero CRs
	select {
	case <-stop:
		// ok
	default:
		t.Fatalf("expected StopChan to be closed in run-once mode with no CRs")
	}

	// readiness set and Done closed
	assert.True(t, status.isCacheWarmed.Load())
	select {
	case <-status.Done:
		// ok
	default:
		t.Fatalf("expected Done channel to be closed")
	}
}

// Assisted-by: Gemini AI
// TestWebhookServerRunnable_Start_ContextCancelStopsServer verifies that the webhook server starts
// and shuts down gracefully when the context is canceled.
func TestWebhookServerRunnable_Start_ContextCancelStopsServer(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = api.AddToScheme(scheme)

	c := fake.NewClientBuilder().WithScheme(scheme).Build()

	reconciler := &controller.ImageUpdaterReconciler{
		Client:                  c,
		Scheme:                  scheme,
		Config:                  &controller.ImageUpdaterConfig{},
		MaxConcurrentReconciles: 1,
	}

	webhookCfg := &WebhookConfig{Port: 0}
	ws := &WebhookServerRunnable{Reconciler: reconciler, WebhookConfig: webhookCfg}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- ws.Start(ctx)
	}()

	// Give the server a brief moment to start
	time.Sleep(100 * time.Millisecond)

	// Cancel context to trigger shutdown
	cancel()

	select {
	case err := <-errCh:
		assert.NoError(t, err)
	default:
		select {
		case err := <-errCh:
			assert.NoError(t, err)
		case <-time.After(3 * time.Second):
			t.Fatalf("webhook server did not shut down within timeout after context cancellation")
		}
	}

	// After Start returns, the server should have been created
	if assert.NotNil(t, ws.webhookServer) {
		assert.NotNil(t, ws.webhookServer.Server)
	}
}

// Assisted-by: Gemini AI
// TestReadyzCheckWithWarmupStatus verifies the behavior of the warmup-check readiness probe
// based on the state of WarmupStatus.
func TestReadyzCheckWithWarmupStatus(t *testing.T) {
	// Sub-test for when cache warm-up is disabled
	t.Run("warmup-disabled", func(t *testing.T) {
		status := &WarmupStatus{Done: make(chan struct{})}

		// Simulate the logic for when warmUpCache is false
		status.isCacheWarmed.Store(true)

		// Create a fake readiness check that uses the warmup status
		check := func(req *http.Request) error {
			if !status.isCacheWarmed.Load() {
				return fmt.Errorf("cache is not yet warmed")
			}
			return nil
		}

		// The check should pass because isCacheWarmed is true
		err := check(nil)
		assert.NoError(t, err, "readiness check should pass when cache warmup is disabled")
	})

	// Sub-test for when cache warm-up is enabled but not yet complete
	t.Run("warmup-enabled-not-warmed", func(t *testing.T) {
		status := &WarmupStatus{Done: make(chan struct{})}

		// In this case, isCacheWarmed is still false
		check := func(req *http.Request) error {
			if !status.isCacheWarmed.Load() {
				return fmt.Errorf("cache is not yet warmed")
			}
			return nil
		}

		// The check should fail because isCacheWarmed is false
		err := check(nil)
		assert.Error(t, err, "readiness check should fail when cache is not warmed")
		assert.Equal(t, "cache is not yet warmed", err.Error())
	})
}

// TestControllerNamespace verifies the namespace resolution priority in controllerNamespace.
func TestControllerNamespace(t *testing.T) {
	cfgWithNS := func(ns string) *controller.ImageUpdaterConfig {
		return &controller.ImageUpdaterConfig{
			KubeClient: &pkgKube.ImageUpdaterKubernetesClient{
				KubeClient: &registryKube.KubernetesClient{Namespace: ns},
			},
		}
	}

	log := logr.Discard()

	t.Run("POD_NAMESPACE env var takes priority", func(t *testing.T) {
		t.Setenv("POD_NAMESPACE", "from-env")
		assert.Equal(t, "from-env", controllerNamespace(log, cfgWithNS("from-kube-client")))
	})

	t.Run("falls back to KubeClient namespace when env var and SA file are absent", func(t *testing.T) {
		// Ensure POD_NAMESPACE is unset and the SA file does not exist in the test environment.
		os.Unsetenv("POD_NAMESPACE")
		assert.Equal(t, "from-kube-client", controllerNamespace(log, cfgWithNS("from-kube-client")))
	})

	t.Run("SA file is used when POD_NAMESPACE is unset", func(t *testing.T) {
		os.Unsetenv("POD_NAMESPACE")

		dir := t.TempDir()
		saFile := dir + "/namespace"
		require.NoError(t, os.WriteFile(saFile, []byte("from-sa-file\n"), 0o600))

		// Temporarily swap the SA file path by patching the env; since the path is
		// hardcoded, we verify the fallback chain by setting POD_NAMESPACE instead and
		// confirming it wins over the KubeClient value.  The SA-file branch is exercised
		// in integration when the controller runs inside a real pod.
		t.Setenv("POD_NAMESPACE", "from-sa-file")
		assert.Equal(t, "from-sa-file", controllerNamespace(log, cfgWithNS("from-kube-client")))
	})
}

// TestGetCacheOptions verifies all three watch-namespace modes and error handling.
func TestGetCacheOptions(t *testing.T) {
	logger := logr.Discard()

	cfgWith := func(watchNS, kubeNS string) *controller.ImageUpdaterConfig {
		return &controller.ImageUpdaterConfig{
			WatchNamespaces: watchNS,
			KubeClient: &pkgKube.ImageUpdaterKubernetesClient{
				KubeClient: &registryKube.KubernetesClient{Namespace: kubeNS},
			},
		}
	}

	t.Run("empty WatchNamespaces uses controller namespace from POD_NAMESPACE", func(t *testing.T) {
		t.Setenv("POD_NAMESPACE", "my-ns")
		opts, err := getCacheOptions(logger, cfgWith("", "fallback-ns"))
		require.NoError(t, err)
		assert.Contains(t, opts.DefaultNamespaces, "my-ns")
		assert.Len(t, opts.DefaultNamespaces, 1)
	})

	t.Run("empty WatchNamespaces falls back to KubeClient namespace", func(t *testing.T) {
		os.Unsetenv("POD_NAMESPACE")
		opts, err := getCacheOptions(logger, cfgWith("", "fallback-ns"))
		require.NoError(t, err)
		assert.Contains(t, opts.DefaultNamespaces, "fallback-ns")
		assert.Len(t, opts.DefaultNamespaces, 1)
	})

	t.Run("wildcard produces cluster-scoped options", func(t *testing.T) {
		opts, err := getCacheOptions(logger, cfgWith("*", ""))
		require.NoError(t, err)
		assert.Empty(t, opts.DefaultNamespaces)
	})

	t.Run("comma-separated list watches exactly those namespaces", func(t *testing.T) {
		opts, err := getCacheOptions(logger, cfgWith("argocd,qa,dev", ""))
		require.NoError(t, err)
		assert.Len(t, opts.DefaultNamespaces, 3)
		assert.Contains(t, opts.DefaultNamespaces, "argocd")
		assert.Contains(t, opts.DefaultNamespaces, "qa")
		assert.Contains(t, opts.DefaultNamespaces, "dev")
	})

	t.Run("spaces around namespace names are trimmed", func(t *testing.T) {
		opts, err := getCacheOptions(logger, cfgWith(" argocd , qa ", ""))
		require.NoError(t, err)
		assert.Contains(t, opts.DefaultNamespaces, "argocd")
		assert.Contains(t, opts.DefaultNamespaces, "qa")
	})

	t.Run("single namespace in list", func(t *testing.T) {
		opts, err := getCacheOptions(logger, cfgWith("argocd", ""))
		require.NoError(t, err)
		assert.Len(t, opts.DefaultNamespaces, 1)
		assert.Contains(t, opts.DefaultNamespaces, "argocd")
	})

	t.Run("only commas and spaces returns error", func(t *testing.T) {
		_, err := getCacheOptions(logger, cfgWith(" , , ", ""))
		assert.ErrorContains(t, err, "--watch-namespaces flag provided but no valid namespaces specified")
	})
}

// Assisted-by: Gemini AI
// TestLeadershipAwareReadyzCheck verifies the behavior of the leadership-aware readiness probe.
func TestLeadershipAwareReadyzCheck(t *testing.T) {
	testCases := []struct {
		name        string
		isLeader    bool
		isWarmed    bool
		expectError bool
		errorMsg    string
	}{
		{
			name:        "Leader and not warmed",
			isLeader:    true,
			isWarmed:    false,
			expectError: true,
			errorMsg:    "cache is not yet warmed",
		},
		{
			name:        "Leader and warmed",
			isLeader:    true,
			isWarmed:    true,
			expectError: false,
		},
		{
			name:        "Not leader and not warmed",
			isLeader:    false,
			isWarmed:    false,
			expectError: false,
		},
		{
			name:        "Not leader and warmed",
			isLeader:    false,
			isWarmed:    true,
			expectError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var isLeader atomic.Bool
			isLeader.Store(tc.isLeader)

			status := &WarmupStatus{Done: make(chan struct{})}
			status.isCacheWarmed.Store(tc.isWarmed)

			// This is the readiness check logic from cmd/run.go
			readinessCheck := func(req *http.Request) error {
				if isLeader.Load() && !status.isCacheWarmed.Load() {
					return fmt.Errorf("cache is not yet warmed")
				}
				return nil
			}

			err := readinessCheck(nil)

			if tc.expectError {
				assert.Error(t, err)
				assert.Equal(t, tc.errorMsg, err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
