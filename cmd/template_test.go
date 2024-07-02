package main

import (
	"bytes"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewTemplateCommandWithEmptyArgs(t *testing.T) {

	defaultExpectedOutput := `build: automatic update of example-app

updates image example/example tag '1.0.0' to '1.0.1'
updates image example/updater tag 'sha256:01d09d19c2139a46aebfb577780d123d7396e97201bc7ead210a2ebff8239dee' to 'sha256:7aa7a5359173d05b63cfd682e3c38487f3cb4f7f1d60659fe59fab1505977d4c'

`
	cmd := newTemplateCommand()
	buf := new(bytes.Buffer)
	args := []string{}
	cmd.SetOut(buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("could not execute command: %v", err)
	}
	assert.NoError(t, err)
	assert.Equal(t, defaultExpectedOutput, buf.String())

}

func TestNewTemplateCommandWithCustomTemplate(t *testing.T) {

	testTemplate := `Custom Commit Message:
App: {{.AppName}}
{{- range .AppChanges }}
- {{.Image}}: {{.OldTag}} -> {{.NewTag}}
{{- end }}`

	expectedOutput := `Custom Commit Message:
App: example-app
- example/example: 1.0.0 -> 1.0.1
- example/updater: sha256:01d09d19c2139a46aebfb577780d123d7396e97201bc7ead210a2ebff8239dee -> sha256:7aa7a5359173d05b63cfd682e3c38487f3cb4f7f1d60659fe59fab1505977d4c
`

	// Create a temporary file to hold the test template
	tempFile, err := os.CreateTemp("", "test-template.tmpl")
	if err != nil {
		t.Fatalf("could not create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())
	_, err = tempFile.WriteString(testTemplate)
	if err != nil {
		t.Fatalf("could not write to temp file: %v", err)
	}
	tempFile.Close()

	cmd := newTemplateCommand()
	buf := new(bytes.Buffer)
	args := []string{tempFile.Name()}
	cmd.SetOut(buf)
	cmd.SetArgs(args)
	err = cmd.Execute()
	if err != nil {
		t.Fatalf("could not execute command: %v", err)
	}
	assert.Equal(t, expectedOutput, buf.String())
}

func TestNewTemplateCommandWithInvalidTemplate(t *testing.T) {

	testTemplate := `Custom Commit Message:
App: {{.AppName}}
{{- range .AppChanges }}
- {{.Image}}: {{.OldTag}} -> {{.NewTag}}
{{- end`

	// Create a temporary file to hold the test template
	tempFile, err := os.CreateTemp("", "test-template.tmpl")
	if err != nil {
		t.Fatalf("could not create temp file: %v", err)
	}
	defer os.Remove(tempFile.Name())
	_, err = tempFile.WriteString(testTemplate)
	if err != nil {
		t.Fatalf("could not write to temp file: %v", err)
	}
	tempFile.Close()

	cmd := newTemplateCommand()
	buf := new(bytes.Buffer)
	args := []string{tempFile.Name()}
	cmd.SetErr(buf)
	cmd.SetArgs(args)
	err = cmd.Execute()
	if err != nil {
		t.Fatalf("could not execute command: %v", err)
	}
	assert.Contains(t, buf.String(), "could not parse commit message template")
}

func TestNewTemplateCommandWithInvalidPath(t *testing.T) {

	cmd := newTemplateCommand()
	buf := new(bytes.Buffer)
	args := []string{"test-template.tmpl"}
	cmd.SetErr(buf)
	cmd.SetArgs(args)
	err := cmd.Execute()
	if err != nil {
		t.Fatalf("could not execute command: %v", err)
	}
	assert.Contains(t, buf.String(), "no such file or directory")
}
