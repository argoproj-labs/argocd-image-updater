package version

import (
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_BinaryName(t *testing.T) {
	require.Equal(t, binaryName, BinaryName())
}

func Test_Version(t *testing.T) {
	assert.Regexp(t, `^v[0-9]+\.[0-9]+\.[0-9]+(\-[a-z]+)*(\+[a-z0-9]+)*$`, Version())
}

func Test_Useragent(t *testing.T) {
	assert.Regexp(t, `^[a-z\-]+:\sv[0-9]+\.[0-9]+\.[0-9]+(-[a-z]+)*(\+[a-z0-9]+)*$`, Useragent())
}

func TestGitCommit(t *testing.T) {
	expected := "unknown"
	actual := GitCommit()
	assert.Equal(t, expected, actual, "GitCommit should return the correct git commit hash")
}

func TestBuildDate(t *testing.T) {
	expected := "1970-01-01T00:00:00Z"
	actual := BuildDate()
	assert.Equal(t, expected, actual, "BuildDate should return the correct build date")
}

func TestGoVersion(t *testing.T) {
	expected := runtime.Version()
	actual := GoVersion()
	assert.Equal(t, expected, actual, "GoVersion should return the correct Go version")
}

func TestGoPlatform(t *testing.T) {
	expected := runtime.GOOS + "/" + runtime.GOARCH
	actual := GoPlatform()
	assert.Equal(t, expected, actual, "GoPlatform should return the correct Go platform")
}

func TestGoCompiler(t *testing.T) {
	expected := runtime.Compiler
	actual := GoCompiler()
	assert.Equal(t, expected, actual, "GoCompiler should return the correct Go compiler")
}
