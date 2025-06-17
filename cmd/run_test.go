package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/argoproj-labs/argocd-image-updater/pkg/common"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/env"
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
	asser.Equal("true", controllerCommand.Flag("leader-elect").Value.String())
	asser.Equal("true", controllerCommand.Flag("metrics-secure").Value.String())
	asser.Equal("false", controllerCommand.Flag("enable-http2").Value.String())
	asser.Nil(controllerCommand.Help())
	asser.True(controllerCommand.HasFlags())
	asser.True(controllerCommand.HasLocalFlags())
	asser.False(controllerCommand.HasSubCommands())
	asser.False(controllerCommand.HasHelpSubCommands())
	asser.Equal(env.GetStringVal("APPLICATIONS_API", common.ApplicationsAPIKindK8S), controllerCommand.Flag("applications-api").Value.String())
	asser.Equal(env.GetStringVal("ARGOCD_SERVER", ""), controllerCommand.Flag("argocd-server-addr").Value.String())
	asser.Equal(env.GetStringVal("ARGOCD_GRPC_WEB", "false"), controllerCommand.Flag("argocd-grpc-web").Value.String())
	asser.Equal(env.GetStringVal("ARGOCD_INSECURE", "false"), controllerCommand.Flag("argocd-insecure").Value.String())
	asser.Equal(env.GetStringVal("ARGOCD_PLAINTEXT", "false"), controllerCommand.Flag("argocd-plaintext").Value.String())
	asser.Equal("", controllerCommand.Flag("argocd-auth-token").Value.String())
	asser.Equal("false", controllerCommand.Flag("dry-run").Value.String())
	asser.Equal(env.GetDurationVal("IMAGE_UPDATER_INTERVAL", 2*time.Minute).String(), controllerCommand.Flag("interval").Value.String())
	asser.Equal(env.GetStringVal("IMAGE_UPDATER_LOGLEVEL", "info"), controllerCommand.Flag("loglevel").Value.String())
	asser.Equal("", controllerCommand.Flag("kubeconfig").Value.String())
	asser.Equal("8080", controllerCommand.Flag("health-port").Value.String())
	asser.Equal("8081", controllerCommand.Flag("metrics-port").Value.String())
	asser.Equal("false", controllerCommand.Flag("once").Value.String())
	asser.Equal(common.DefaultRegistriesConfPath, controllerCommand.Flag("registries-conf-path").Value.String())
	asser.Equal("false", controllerCommand.Flag("disable-kubernetes").Value.String())
	asser.Equal("10", controllerCommand.Flag("max-concurrency").Value.String())
	asser.Equal("", controllerCommand.Flag("argocd-namespace").Value.String())
	asser.Equal("[]", controllerCommand.Flag("match-application-name").Value.String())
	asser.Equal("", controllerCommand.Flag("match-application-label").Value.String())
	asser.Equal("true", controllerCommand.Flag("warmup-cache").Value.String())
	asser.Equal(env.GetStringVal("GIT_COMMIT_USER", "argocd-image-updater"), controllerCommand.Flag("git-commit-user").Value.String())
	asser.Equal(env.GetStringVal("GIT_COMMIT_EMAIL", "noreply@argoproj.io"), controllerCommand.Flag("git-commit-email").Value.String())
	asser.Equal(env.GetStringVal("GIT_COMMIT_SIGNING_KEY", ""), controllerCommand.Flag("git-commit-signing-key").Value.String())
	asser.Equal(env.GetStringVal("GIT_COMMIT_SIGNING_METHOD", "openpgp"), controllerCommand.Flag("git-commit-signing-method").Value.String())
	asser.Equal(env.GetStringVal("GIT_COMMIT_SIGN_OFF", "false"), controllerCommand.Flag("git-commit-sign-off").Value.String())
	asser.Equal(common.DefaultCommitTemplatePath, controllerCommand.Flag("git-commit-message-path").Value.String())
	asser.Equal(env.GetStringVal("IMAGE_UPDATER_KUBE_EVENTS", "false"), controllerCommand.Flag("disable-kube-events").Value.String())

	asser.Nil(controllerCommand.Help())

}
