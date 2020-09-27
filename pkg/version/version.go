package version

import "fmt"

var (
	version    = "9.9.99"
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
	return fmt.Sprintf("%s %s", BinaryName(), Version())
}
