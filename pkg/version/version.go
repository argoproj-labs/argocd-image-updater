package version

import (
	"fmt"
	"runtime"
	"time"
)

var (
	version    = "9.9.99"
	buildDate  = time.Now().UTC().Format(time.RFC3339)
	gitCommit  = "unknown"
	binaryName = "argocd-image-updater"
)

func Version() string {
	version := fmt.Sprintf("v%s+%s", version, gitCommit[0:7])
	return version
}

func BinaryName() string {
	return binaryName
}

func Useragent() string {
	return fmt.Sprintf("%s: %s", BinaryName(), Version())
}

func GitCommit() string {
	return gitCommit
}

func BuildDate() string {
	return buildDate
}

func GoVersion() string {
	return runtime.Version()
}

func GoPlatform() string {
	return runtime.GOOS + "/" + runtime.GOARCH
}

func GoCompiler() string {
	return runtime.Compiler
}
