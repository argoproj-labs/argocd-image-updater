package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/argoproj-labs/argocd-image-updater/pkg/version"
)

// Helper function to create the expected full version output
func fullVersionOutput() string {
	return version.Useragent() + "\n" +
		"  BuildDate: " + version.BuildDate() + "\n" +
		"  GitCommit: " + version.GitCommit() + "\n" +
		"  GoVersion: " + version.GoVersion() + "\n" +
		"  GoCompiler: " + version.GoCompiler() + "\n" +
		"  Platform: " + version.GoPlatform() + "\n"
}

func TestNewVersionCommand(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name:     "default output",
			args:     []string{},
			expected: fullVersionOutput(),
		},
		{
			name:     "short flag output",
			args:     []string{"--short"},
			expected: version.Version() + "\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := newVersionCommand()
			buf := new(bytes.Buffer)
			cmd.SetOut(buf)
			cmd.SetArgs(tt.args)
			err := cmd.Execute()
			assert.NoError(t, err)
			assert.Equal(t, tt.expected, buf.String())
		})
	}
}
