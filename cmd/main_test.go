package main

import (
	"github.com/stretchr/testify/assert"
	"os"
	"testing"
)

// TestRootCmd tests main.go#newRootCommand.
func TestRootCmd(t *testing.T) {
	//remove the last element from os.Args so that it will not be taken as the arg to the image-updater command
	os.Args = os.Args[:len(os.Args)-1]
	err := newRootCommand()
	assert.Nil(t, err)
}
