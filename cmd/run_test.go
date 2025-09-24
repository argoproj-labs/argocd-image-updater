package main

import (
	"os"
	"testing"
	"time"
	"context"
	"net"
	"net/http"
	"fmt"

	"github.com/stretchr/testify/assert"

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/env"
	"github.com/argoproj-labs/argocd-image-updater/pkg/argocd"
	"github.com/argoproj-labs/argocd-image-updater/pkg/common"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/argoproj/argo-cd/v2/pkg/apiclient/application"
)

// TestNewRunCommand tests various flags and their default values.
func TestNewRunCommand(t *testing.T) {
	asser := assert.New(t)
	runCmd := newRunCommand()
	asser.Contains(runCmd.Use, "run")
	asser.Greater(len(runCmd.Short), 25)
	asser.NotNil(runCmd.RunE)
	asser.Equal(env.GetStringVal("APPLICATIONS_API", applicationsAPIKindK8S), runCmd.Flag("applications-api").Value.String())
	asser.Equal(env.GetStringVal("ARGOCD_SERVER", ""), runCmd.Flag("argocd-server-addr").Value.String())
	asser.Equal(env.GetStringVal("ARGOCD_GRPC_WEB", "false"), runCmd.Flag("argocd-grpc-web").Value.String())
	asser.Equal(env.GetStringVal("ARGOCD_INSECURE", "false"), runCmd.Flag("argocd-insecure").Value.String())
	asser.Equal(env.GetStringVal("ARGOCD_PLAINTEXT", "false"), runCmd.Flag("argocd-plaintext").Value.String())
	asser.Equal("", runCmd.Flag("argocd-auth-token").Value.String())
	asser.Equal("false", runCmd.Flag("dry-run").Value.String())
	asser.Equal(env.GetDurationVal("IMAGE_UPDATER_INTERVAL", 2*time.Minute).String(), runCmd.Flag("interval").Value.String())
	asser.Equal(env.GetStringVal("IMAGE_UPDATER_LOGLEVEL", "info"), runCmd.Flag("loglevel").Value.String())
	asser.Equal("", runCmd.Flag("kubeconfig").Value.String())
	asser.Equal("8080", runCmd.Flag("health-port").Value.String())
	asser.Equal("8081", runCmd.Flag("metrics-port").Value.String())
	asser.Equal("false", runCmd.Flag("once").Value.String())
	asser.Equal(defaultRegistriesConfPath, runCmd.Flag("registries-conf-path").Value.String())
	asser.Equal("false", runCmd.Flag("disable-kubernetes").Value.String())
	asser.Equal("10", runCmd.Flag("max-concurrency").Value.String())
	asser.Equal("", runCmd.Flag("argocd-namespace").Value.String())
	asser.Equal("[]", runCmd.Flag("match-application-name").Value.String())
	asser.Equal("", runCmd.Flag("match-application-label").Value.String())
	asser.Equal("true", runCmd.Flag("warmup-cache").Value.String())
	asser.Equal(env.GetStringVal("GIT_COMMIT_USER", "argocd-image-updater"), runCmd.Flag("git-commit-user").Value.String())
	asser.Equal(env.GetStringVal("GIT_COMMIT_EMAIL", "noreply@argoproj.io"), runCmd.Flag("git-commit-email").Value.String())
	asser.Equal(env.GetStringVal("GIT_COMMIT_SIGNING_KEY", ""), runCmd.Flag("git-commit-signing-key").Value.String())
	asser.Equal(env.GetStringVal("GIT_COMMIT_SIGNING_METHOD", "openpgp"), runCmd.Flag("git-commit-signing-method").Value.String())
	asser.Equal(env.GetStringVal("GIT_COMMIT_SIGN_OFF", "false"), runCmd.Flag("git-commit-sign-off").Value.String())
	asser.Equal(defaultCommitTemplatePath, runCmd.Flag("git-commit-message-path").Value.String())
	asser.Equal(env.GetStringVal("IMAGE_UPDATER_KUBE_EVENTS", "false"), runCmd.Flag("disable-kube-events").Value.String())

	asser.Nil(runCmd.Help())
}

// TestRootCmd tests main.go#newRootCommand.
func TestRootCmd(t *testing.T) {
	//remove the last element from os.Args so that it will not be taken as the arg to the image-updater command
	os.Args = os.Args[:len(os.Args)-1]
	err := newRootCommand()
	assert.Nil(t, err)
}

// TestContinuousScheduling ensures that due apps are launched independently and respect interval.
func TestContinuousScheduling(t *testing.T) {
    t.Cleanup(func(){ updateAppFn = argocd.UpdateApplication })
    // stub updater: sleep based on app name prefix to simulate slow/fast
    var fastRuns, slowRuns int
    updateAppFn = func(conf *argocd.UpdateConfiguration, state *argocd.SyncIterationState) argocd.ImageUpdaterResult {
        app := conf.UpdateApp.Application.GetName()
        if app == "slow" {
            slowRuns++
            time.Sleep(250 * time.Millisecond)
        } else {
            fastRuns++
            time.Sleep(10 * time.Millisecond)
        }
        return argocd.ImageUpdaterResult{}
    }

    cfg := &ImageUpdaterConfig{
        ApplicationsAPIKind: applicationsAPIKindK8S,
        CheckInterval: 100 * time.Millisecond,
        MaxConcurrency: 2,
        Mode: "continuous",
    }
    cfg.ArgoClient = &fakeArgo{apps: []string{"slow", "fast"}}

    // Kick scheduler a few times within ~400ms window
    deadline := time.Now().Add(400 * time.Millisecond)
    for time.Now().Before(deadline) {
        runContinuousOnce(cfg)
        time.Sleep(20 * time.Millisecond)
    }

    // Expect fast to have run more than slow
    if !(fastRuns > slowRuns) {
        t.Fatalf("expected fast runs > slow runs; got fast=%d slow=%d", fastRuns, slowRuns)
    }
}

// pickFreePort returns an available TCP port on localhost.
func pickFreePort(t *testing.T) int {
    l, err := net.Listen("tcp", "127.0.0.1:0")
    if err != nil { t.Fatalf("pickFreePort: %v", err) }
    defer l.Close()
    return l.Addr().(*net.TCPAddr).Port
}

// TestRun_StartsWebhookWhenEnabled verifies run mode starts webhook server when enabled via env.
func TestRun_StartsWebhookWhenEnabled(t *testing.T) {
    // choose a free port
    port := pickFreePort(t)

    // Prepare command with flags that avoid external deps and exit quickly
    cmd := newRunCommand()
    cmd.SetArgs([]string{
        "--disable-kubernetes",
        "--warmup-cache=false",
        "--once",
        "--loglevel", "debug",
    })

    // Set envs to enable webhook
    t.Setenv("ENABLE_WEBHOOK", "true")
    t.Setenv("WEBHOOK_PORT",  fmt.Sprintf("%d", port))

    // Run the command; it should return (once-mode) while webhook goroutine keeps listening
    if err := cmd.Execute(); err != nil {
        t.Fatalf("run command returned error: %v", err)
    }

    // Give the server a moment to bind
    deadline := time.Now().Add(500 * time.Millisecond)
    var lastErr error
    for time.Now().Before(deadline) {
        resp, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/healthz", port))
        if err == nil && resp != nil {
            resp.Body.Close()
            if resp.StatusCode == http.StatusOK { return }
        }
        lastErr = err
        time.Sleep(50 * time.Millisecond)
    }
    t.Fatalf("webhook did not start on port %d: lastErr=%v", port, lastErr)
}

type fakeArgo struct{ apps []string }
func (f *fakeArgo) GetApplication(ctx context.Context, name string) (*v1alpha1.Application, error) { return nil, nil }
func (f *fakeArgo) ListApplications(_ string) ([]v1alpha1.Application, error) {
    out := make([]v1alpha1.Application, 0, len(f.apps))
    for _, n := range f.apps {
        out = append(out, v1alpha1.Application{
            ObjectMeta: v1.ObjectMeta{Name: n, Annotations: map[string]string{common.ImageUpdaterAnnotation: ""}},
            Spec: v1alpha1.ApplicationSpec{Source: &v1alpha1.ApplicationSource{Kustomize: &v1alpha1.ApplicationSourceKustomize{}}},
            Status: v1alpha1.ApplicationStatus{SourceType: v1alpha1.ApplicationSourceTypeKustomize},
        })
    }
    return out, nil
}
func (f *fakeArgo) UpdateSpec(ctx context.Context, _ *application.ApplicationUpdateSpecRequest) (*v1alpha1.ApplicationSpec, error) { return nil, nil }
