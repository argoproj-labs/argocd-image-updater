package main

import (
	"math"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/argoproj-labs/argocd-image-updater/pkg/common"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/env"
)

// TestNewWebhookCommand tests various flags and their default values.
func TestNewWebhookCommand(t *testing.T) {
	asser := assert.New(t)
	controllerCommand := NewWebhookCommand()
	asser.Contains(controllerCommand.Use, "webhook")
	asser.Equal(controllerCommand.Short, "Start webhook server to receive registry events")
	asser.Greater(len(controllerCommand.Long), 100)
	asser.NotNil(controllerCommand.RunE)
	asser.Nil(controllerCommand.Help())
	asser.True(controllerCommand.HasFlags())
	asser.True(controllerCommand.HasLocalFlags())
	asser.False(controllerCommand.HasSubCommands())
	asser.False(controllerCommand.HasHelpSubCommands())
	asser.Equal(env.GetStringVal("IMAGE_UPDATER_LOGLEVEL", "info"), controllerCommand.Flag("loglevel").Value.String())
	asser.Equal("", controllerCommand.Flag("kubeconfig").Value.String())
	asser.Equal(common.DefaultRegistriesConfPath, controllerCommand.Flag("registries-conf-path").Value.String())
	asser.Equal(strconv.Itoa(env.ParseNumFromEnv("MAX_CONCURRENT_APPS", 10, 1, 100)), controllerCommand.Flag("max-concurrent-apps").Value.String())
	asser.Equal(strconv.Itoa(env.ParseNumFromEnv("MAX_CONCURRENT_UPDATERS", 1, 1, 10)), controllerCommand.Flag("max-concurrent-updaters").Value.String())
	asser.Equal(env.GetStringVal("ARGOCD_NAMESPACE", ""), controllerCommand.Flag("argocd-namespace").Value.String())
	asser.Equal(env.GetStringVal("GIT_COMMIT_USER", "argocd-image-updater"), controllerCommand.Flag("git-commit-user").Value.String())
	asser.Equal(env.GetStringVal("GIT_COMMIT_EMAIL", "noreply@argoproj.io"), controllerCommand.Flag("git-commit-email").Value.String())
	asser.Equal(env.GetStringVal("GIT_COMMIT_SIGNING_KEY", ""), controllerCommand.Flag("git-commit-signing-key").Value.String())
	asser.Equal(env.GetStringVal("GIT_COMMIT_SIGNING_METHOD", "openpgp"), controllerCommand.Flag("git-commit-signing-method").Value.String())
	asser.Equal(env.GetStringVal("GIT_COMMIT_SIGN_OFF", "false"), controllerCommand.Flag("git-commit-sign-off").Value.String())
	asser.Equal(common.DefaultCommitTemplatePath, controllerCommand.Flag("git-commit-message-path").Value.String())
	asser.Equal(env.GetStringVal("IMAGE_UPDATER_KUBE_EVENTS", "false"), controllerCommand.Flag("disable-kube-events").Value.String())
	asser.Equal(strconv.Itoa(env.ParseNumFromEnv("WEBHOOK_PORT", 8080, 0, 65535)), controllerCommand.Flag("webhook-port").Value.String())
	asser.Equal(env.GetStringVal("DOCKER_WEBHOOK_SECRET", ""), controllerCommand.Flag("docker-webhook-secret").Value.String())
	asser.Equal(env.GetStringVal("GHCR_WEBHOOK_SECRET", ""), controllerCommand.Flag("ghcr-webhook-secret").Value.String())
	asser.Equal(env.GetStringVal("QUAY_WEBHOOK_SECRET", ""), controllerCommand.Flag("quay-webhook-secret").Value.String())
	asser.Equal(env.GetStringVal("HARBOR_WEBHOOK_SECRET", ""), controllerCommand.Flag("harbor-webhook-secret").Value.String())
	asser.Equal(strconv.Itoa(env.ParseNumFromEnv("WEBHOOK_RATELIMIT_ALLOWED", 0, 0, math.MaxInt)), controllerCommand.Flag("webhook-ratelimit-allowed").Value.String())

	asser.Nil(controllerCommand.Help())
}
