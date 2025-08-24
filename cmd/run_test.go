package main

import (
	"os"
	"strconv"
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
	asser.Equal("", controllerCommand.Flag("kubeconfig").Value.String())
	asser.Equal("8080", controllerCommand.Flag("health-port").Value.String())
	asser.Equal("8081", controllerCommand.Flag("metrics-port").Value.String())
	asser.Equal("false", controllerCommand.Flag("once").Value.String())
	asser.Equal(common.DefaultRegistriesConfPath, controllerCommand.Flag("registries-conf-path").Value.String())
	asser.Equal("false", controllerCommand.Flag("disable-kubernetes").Value.String())
	asser.Equal(strconv.Itoa(env.ParseNumFromEnv("MAX_CONCURRENT_APPS", 10, 1, 100)), controllerCommand.Flag("max-concurrent-apps").Value.String())
	asser.Equal(strconv.Itoa(env.ParseNumFromEnv("MAX_CONCURRENT_RECONCILES", 1, 1, 10)), controllerCommand.Flag("max-concurrent-reconciles").Value.String())
	asser.Equal("", controllerCommand.Flag("argocd-namespace").Value.String())
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
