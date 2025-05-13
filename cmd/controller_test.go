package main

import (
	"github.com/stretchr/testify/assert"
	"testing"
)

// TestNewControllerCommand tests various flags and their default values.
func TestNewControllerCommand(t *testing.T) {
	asser := assert.New(t)
	controllerCommand := newControllerCommand()
	asser.Contains(controllerCommand.Use, "controller")
	asser.Equal(controllerCommand.Short, "Manages ArgoCD Image Updater Controller.")
	asser.Greater(len(controllerCommand.Long), 100)
	asser.NotNil(controllerCommand.RunE)
	asser.Equal("0", controllerCommand.Flag("metrics-bind-address").Value.String())
	asser.Equal(":8081", controllerCommand.Flag("health-probe-bind-address").Value.String())
	asser.Equal("false", controllerCommand.Flag("leader-elect").Value.String())
	asser.Equal("true", controllerCommand.Flag("metrics-secure").Value.String())
	asser.Equal("false", controllerCommand.Flag("enable-http2").Value.String())
	asser.Equal("2m0s", controllerCommand.Flag("interval").Value.String())
	asser.Equal("info", controllerCommand.Flag("loglevel").Value.String())
	asser.Nil(controllerCommand.Help())
	asser.True(controllerCommand.HasFlags())
	asser.True(controllerCommand.HasLocalFlags())
	asser.False(controllerCommand.HasSubCommands())
	asser.False(controllerCommand.HasHelpSubCommands())
}
