package main

import (
	"fmt"
	"runtime"
	"testing"

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
