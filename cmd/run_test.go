package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/env"
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
	asser.Equal("2m0s", runCmd.Flag("interval").Value.String())
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
