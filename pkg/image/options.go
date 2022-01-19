package image

import (
	"fmt"
	"regexp"
	"runtime"
	"strings"

	"github.com/argoproj-labs/argocd-image-updater/pkg/common"
	"github.com/argoproj-labs/argocd-image-updater/pkg/options"
)

// GetParameterHelmImageName gets the value for image-name option for the image
// from a set of annotations
func (img *ContainerImage) GetParameterHelmImageName(annotations map[string]string) string {
	key := fmt.Sprintf(common.HelmParamImageNameAnnotation, img.normalizedSymbolicName())
	val, ok := annotations[key]
	if !ok {
		return ""
	}
	return val
}

// GetParameterHelmImageTag gets the value for image-tag option for the image
// from a set of annotations
func (img *ContainerImage) GetParameterHelmImageTag(annotations map[string]string) string {
	key := fmt.Sprintf(common.HelmParamImageTagAnnotation, img.normalizedSymbolicName())
	val, ok := annotations[key]
	if !ok {
		return ""
	}
	return val
}

// GetParameterHelmImageSpec gets the value for image-spec option for the image
// from a set of annotations
func (img *ContainerImage) GetParameterHelmImageSpec(annotations map[string]string) string {
	key := fmt.Sprintf(common.HelmParamImageSpecAnnotation, img.normalizedSymbolicName())
	val, ok := annotations[key]
	if !ok {
		return ""
	}
	return val
}

// GetParameterKustomizeImageName gets the value for image-spec option for the
// image from a set of annotations
func (img *ContainerImage) GetParameterKustomizeImageName(annotations map[string]string) string {
	key := fmt.Sprintf(common.KustomizeApplicationNameAnnotation, img.normalizedSymbolicName())
	val, ok := annotations[key]
	if !ok {
		return ""
	}
	return val
}

// HasForceUpdateOptionAnnotation gets the value for force-update option for the
// image from a set of annotations
func (img *ContainerImage) HasForceUpdateOptionAnnotation(annotations map[string]string) bool {
	key := fmt.Sprintf(common.ForceUpdateOptionAnnotation, img.normalizedSymbolicName())
	val, ok := annotations[key]
	return ok && val == "true"
}

// GetParameterSort gets and validates the value for the sort option for the
// image from a set of annotations
func (img *ContainerImage) GetParameterUpdateStrategy(annotations map[string]string) UpdateStrategy {
	logCtx := img.LogContext()
	key := fmt.Sprintf(common.UpdateStrategyAnnotation, img.normalizedSymbolicName())
	val, ok := annotations[key]
	if !ok {
		// Default is sort by version
		logCtx.Tracef("No sort option %s found", key)
		return StrategySemVer
	}
	logCtx.Tracef("found update strategy %s in %s", val, key)
	return img.ParseUpdateStrategy(val)
}

func (img *ContainerImage) ParseUpdateStrategy(val string) UpdateStrategy {
	logCtx := img.LogContext()
	switch strings.ToLower(val) {
	case "semver":
		return StrategySemVer
	case "latest":
		return StrategyLatest
	case "name":
		return StrategyName
	case "digest":
		return StrategyDigest
	default:
		logCtx.Warnf("Unknown sort option %s -- using semver", val)
		return StrategySemVer
	}

}

// GetParameterMatch returns the match function and pattern to use for matching
// tag names. If an invalid option is found, it returns MatchFuncNone as the
// default, to prevent accidental matches.
func (img *ContainerImage) GetParameterMatch(annotations map[string]string) (MatchFuncFn, interface{}) {
	logCtx := img.LogContext()
	key := fmt.Sprintf(common.AllowTagsOptionAnnotation, img.normalizedSymbolicName())
	val, ok := annotations[key]
	if !ok {
		// The old match-tag annotation is deprecated and will be subject to removal
		// in a future version.
		key = fmt.Sprintf(common.OldMatchOptionAnnotation, img.normalizedSymbolicName())
		val, ok = annotations[key]
		if !ok {
			logCtx.Tracef("No match annotation %s found", key)
			return MatchFuncAny, ""
		} else {
			logCtx.Warnf("The 'tag-match' annotation is deprecated and subject to removal. Please use 'allow-tags' annotation instead.")
		}
	}

	return img.ParseMatchfunc(val)
}

// ParseMatchfunc returns a matcher function and its argument from given value
func (img *ContainerImage) ParseMatchfunc(val string) (MatchFuncFn, interface{}) {
	logCtx := img.LogContext()

	// The special value "any" doesn't take any parameter
	if strings.ToLower(val) == "any" {
		return MatchFuncAny, nil
	}

	opt := strings.SplitN(val, ":", 2)
	if len(opt) != 2 {
		logCtx.Warnf("Invalid match option syntax '%s', ignoring", val)
		return MatchFuncNone, nil
	}
	switch strings.ToLower(opt[0]) {
	case "regexp":
		re, err := regexp.Compile(opt[1])
		if err != nil {
			logCtx.Warnf("Could not compile regexp '%s'", opt[1])
			return MatchFuncNone, nil
		}
		return MatchFuncRegexp, re
	default:
		logCtx.Warnf("Unknown match function: %s", opt[0])
		return MatchFuncNone, nil
	}
}

// GetParameterPullSecret retrieves an image's pull secret credentials
func (img *ContainerImage) GetParameterPullSecret(annotations map[string]string) *CredentialSource {
	logCtx := img.LogContext()
	key := fmt.Sprintf(common.SecretListAnnotation, img.normalizedSymbolicName())
	val, ok := annotations[key]
	if !ok {
		logCtx.Tracef("No secret annotation %s found", key)
		return nil
	}
	credSrc, err := ParseCredentialSource(val, false)
	if err != nil {
		logCtx.Warnf("Invalid credential reference specified: %s", val)
		return nil
	}
	return credSrc
}

// GetParameterIgnoreTags retrieves a list of tags to ignore from a comma-separated string
func (img *ContainerImage) GetParameterIgnoreTags(annotations map[string]string) []string {
	logCtx := img.LogContext()
	key := fmt.Sprintf(common.IgnoreTagsOptionAnnotation, img.normalizedSymbolicName())
	val, ok := annotations[key]
	if !ok {
		logCtx.Tracef("No ignore-tags annotation %s found", key)
		return nil
	}
	ignoreList := make([]string, 0)
	tags := strings.Split(strings.TrimSpace(val), ",")
	for _, tag := range tags {
		// We ignore empty tags
		trimmed := strings.TrimSpace(tag)
		if trimmed != "" {
			ignoreList = append(ignoreList, trimmed)
		}
	}
	return ignoreList
}

// GetPlatformOptions sets up platform constraints for an image. If no platform
// is specified in the annotations, we restrict the platform for images to the
// platform we're executed on unless unrestricted is set to true, in which case
// we do not setup a platform restriction if no platform annotation is found.
func (img *ContainerImage) GetPlatformOptions(annotations map[string]string, unrestricted bool) *options.ManifestOptions {
	logCtx := img.LogContext()
	var opts *options.ManifestOptions = options.NewManifestOptions()
	key := fmt.Sprintf(common.PlatformsAnnotation, img.normalizedSymbolicName())
	val, ok := annotations[key]
	if !ok {
		if !unrestricted {
			os := runtime.GOOS
			arch := runtime.GOARCH
			variant := ""
			if strings.Contains(runtime.GOARCH, "/") {
				a := strings.SplitN(runtime.GOARCH, "/", 2)
				arch = a[0]
				variant = a[1]
			}
			logCtx.Tracef("Using runtime platform constraint %s", options.PlatformKey(os, arch, variant))
			opts = opts.WithPlatform(os, arch, variant)
		}
	} else {
		platforms := strings.Split(val, ",")
		for _, ps := range platforms {
			pt := strings.TrimSpace(ps)
			os, arch, variant, err := ParsePlatform(pt)
			if err != nil {
				// If the platform identifier could not be parsed, we set the
				// constraint intentionally to the invalid value so we don't
				// end up updating to the wrong architecture possibly.
				os = ps
				logCtx.Warnf("could not parse platform identifier '%v': invalid format", pt)
			}
			logCtx.Tracef("Adding platform constraint %s", options.PlatformKey(os, arch, variant))
			opts = opts.WithPlatform(os, arch, variant)
		}
	}

	return opts
}

func ParsePlatform(platformID string) (string, string, string, error) {
	p := strings.SplitN(platformID, "/", 3)
	if len(p) < 2 {
		return "", "", "", fmt.Errorf("could not parse platform constraint '%s'", platformID)
	}
	os := p[0]
	arch := p[1]
	variant := ""
	if len(p) == 3 {
		variant = p[2]
	}
	return os, arch, variant, nil
}

func (img *ContainerImage) normalizedSymbolicName() string {
	return strings.ReplaceAll(img.ImageAlias, "/", "_")
}
