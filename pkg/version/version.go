package version

import "fmt"

const (
	majorVersion     = "0"
	minorVersion     = "5"
	patchVersion     = "1"
	preReleaseString = ""
)

const binaryName = "argocd-image-updater"

func Version() string {
	version := fmt.Sprintf("v%s.%s.%s", majorVersion, minorVersion, patchVersion)
	if preReleaseString != "" {
		version += fmt.Sprintf("-%s", preReleaseString)
	}
	return version
}

func BinaryName() string {
	return binaryName
}

func Useragent() string {
	return fmt.Sprintf("%s %s", BinaryName(), Version())
}
