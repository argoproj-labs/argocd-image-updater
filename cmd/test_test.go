package main

import (
	"fmt"
	"runtime"
	"testing"

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/image"

	"github.com/stretchr/testify/assert"
)

// TestNewTestCommand tests various flags and their default values.
func TestNewTestCommand(t *testing.T) {
	asser := assert.New(t)
	testCmd := newTestCommand()
	asser.Contains(testCmd.Use, "test")
	asser.Greater(len(testCmd.Short), 25)
	asser.Greater(len(testCmd.Long), 100)
	asser.NotNil(testCmd.Run)
	asser.Equal("", testCmd.Flag("semver-constraint").Value.String())
	asser.Equal("", testCmd.Flag("allow-tags").Value.String())
	asser.Equal("[]", testCmd.Flag("ignore-tags").Value.String())
	asser.Equal("semver", testCmd.Flag("update-strategy").Value.String())
	asser.Equal("", testCmd.Flag("registries-conf-path").Value.String())
	asser.Equal("debug", testCmd.Flag("loglevel").Value.String())
	asser.Equal("", testCmd.Flag("kubeconfig").Value.String())
	asser.Equal("", testCmd.Flag("credentials").Value.String())
	asser.Equal(fmt.Sprintf("[%s/%s]", runtime.GOOS, runtime.GOARCH), testCmd.Flag("platforms").Value.String())
	asser.Equal("20", testCmd.Flag("rate-limit").Value.String())
	asser.Nil(testCmd.Help())
	asser.True(testCmd.HasExample())
	asser.True(testCmd.HasFlags())
	asser.True(testCmd.HasLocalFlags())
	asser.False(testCmd.HasSubCommands())
	asser.False(testCmd.HasParent())
	asser.False(testCmd.HasHelpSubCommands())
}

func Test_resolveCalVerLayout(t *testing.T) {
	// The controller reads the calver layout from the tag position of the
	// image name (see parseImageList). This command must agree with it, or a
	// line copied out of the image list would be tested against a different
	// layout than the one that will actually run.
	tests := []struct {
		name         string
		flag         string
		identifier   string
		wantLayout   string
		wantImageTag string
	}{
		{
			name:         "the layout is taken from the image tag",
			identifier:   "some/image:vYYYY-0M-0D",
			wantLayout:   "vYYYY-0M-0D",
			wantImageTag: "vYYYY-0M-0D",
		},
		{
			name:       "an image with no tag falls back to the default",
			identifier: "some/image",
			wantLayout: "",
		},
		{
			name:       "the flag applies when the image has no tag",
			flag:       "vYYYY-0M-0D",
			identifier: "some/image",
			wantLayout: "vYYYY-0M-0D",
		},
		{
			name:         "the flag overrides the image tag",
			flag:         "YYYY.0M.0D",
			identifier:   "some/image:vYYYY-0M-0D",
			wantLayout:   "YYYY.0M.0D",
			wantImageTag: "vYYYY-0M-0D",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			layout, imageTagLayout := resolveCalVerLayout(tt.flag, image.NewFromIdentifier(tt.identifier))
			assert.Equal(t, tt.wantLayout, layout)
			assert.Equal(t, tt.wantImageTag, imageTagLayout)
		})
	}

	t.Run("a nil image is not a panic", func(t *testing.T) {
		layout, imageTagLayout := resolveCalVerLayout("vYYYY-0M-0D", nil)
		assert.Equal(t, "vYYYY-0M-0D", layout)
		assert.Equal(t, "", imageTagLayout)
	})
}
