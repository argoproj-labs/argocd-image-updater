package image

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/argoproj-labs/argocd-image-updater/pkg/common"
	"github.com/argoproj-labs/argocd-image-updater/pkg/log"
)

// Helper to get value of an annotation, with a possible default key set
func getAnnotationWithDefault(annotations map[string]string, key, defaultKey, defaultValue string) string {
	val, ok := annotations[key]
	if !ok {
		if defaultKey == "" {
			return defaultValue
		}
		val, ok = annotations[defaultKey]
		if !ok {
			return defaultValue
		}
	}
	return val
}

// GetParameterHelmImageName gets the value for image-name option for the image
// from a set of annotations
func (img *ContainerImage) GetParameterHelmImageName(annotations map[string]string) string {
	key := fmt.Sprintf(common.HelmParamImageNameAnnotation, img.normalizedSymbolicName())
	return getAnnotationWithDefault(annotations, key, "", "")
}

// GetParameterHelmImageTag gets the value for image-tag option for the image
// from a set of annotations
func (img *ContainerImage) GetParameterHelmImageTag(annotations map[string]string) string {
	key := fmt.Sprintf(common.HelmParamImageTagAnnotation, img.normalizedSymbolicName())
	return getAnnotationWithDefault(annotations, key, "", "")
}

// GetParameterHelmImageSpec gets the value for image-spec option for the image
// from a set of annotations
func (img *ContainerImage) GetParameterHelmImageSpec(annotations map[string]string) string {
	key := fmt.Sprintf(common.HelmParamImageSpecAnnotation, img.normalizedSymbolicName())
	return getAnnotationWithDefault(annotations, key, "", "")
}

// GetParameterKustomizeImageName gets the value for image-spec option for the
// image from a set of annotations
func (img *ContainerImage) GetParameterKustomizeImageName(annotations map[string]string) string {
	key := fmt.Sprintf(common.KustomizeApplicationNameAnnotation, img.normalizedSymbolicName())
	return getAnnotationWithDefault(annotations, key, "", "")
}

// HasForceUpdateOptionAnnotation gets the value for force-update option for the
// image from a set of annotations
func (img *ContainerImage) HasForceUpdateOptionAnnotation(annotations map[string]string) bool {
	key := fmt.Sprintf(common.ForceUpdateOptionAnnotation, img.normalizedSymbolicName())
	val := getAnnotationWithDefault(annotations, key, common.DefaultForceUpdateOptionAnnotation, "false")
	return val == "true"
}

// GetParameterSort gets and validates the value for the sort option for the
// image from a set of annotations
func (img *ContainerImage) GetParameterUpdateStrategy(annotations map[string]string) VersionSortMode {
	key := fmt.Sprintf(common.UpdateStrategyAnnotation, img.normalizedSymbolicName())
	val := getAnnotationWithDefault(annotations, key, common.DefaultUpdateStrategyAnnotation, common.DefaultUpdateStrategyValue)
	return img.ParseUpdateStrategy(val)
}

// ParseUpdateStrategy returns the update strategy from a string identifier
func (img *ContainerImage) ParseUpdateStrategy(val string) VersionSortMode {
	switch strings.ToLower(val) {
	case "semver":
		return VersionSortSemVer
	case "latest":
		return VersionSortLatest
	case "name":
		return VersionSortName
	case "digest":
		return VersionSortDigest
	default:
		img.LogContext().Warnf("Unknown sort option %s -- using semver", val)
		return VersionSortSemVer
	}

}

// GetParameterAllowTags returns the match function and pattern to use for matching
// tag names. If an invalid option is found, it returns MatchFuncNone as the
// default, to prevent accidental matches.
func (img *ContainerImage) GetParameterAllowTags(annotations map[string]string) (MatchFuncFn, interface{}) {
	key := fmt.Sprintf(common.AllowTagsOptionAnnotation, img.normalizedSymbolicName())
	val := getAnnotationWithDefault(annotations, key, common.DefaultAllowTagsOptionAnnotation, "")
	if val == "" {
		// DEPRECATED: This code will be removed
		key = fmt.Sprintf(common.OldMatchOptionAnnotation, img.normalizedSymbolicName())
		val = getAnnotationWithDefault(annotations, key, "", "")
		if val == "" {
			img.LogContext().Tracef("No match annotation %s found", key)
			return MatchFuncAny, ""
		} else {
			img.LogContext().Warnf("The 'tag-match' annotation is deprecated and subject to removal. Please use 'allow-tags' annotation instead.")
		}
	}

	return img.ParseMatchfunc(val)
}

// ParseMatchfunc returns a matcher function and its argument from given value
func (img *ContainerImage) ParseMatchfunc(val string) (MatchFuncFn, interface{}) {
	// The special value "any" doesn't take any parameter
	if strings.ToLower(val) == "any" {
		return MatchFuncAny, nil
	}

	opt := strings.SplitN(val, ":", 2)
	if len(opt) != 2 {
		img.LogContext().Warnf("Invalid match option syntax '%s', ignoring", val)
		return MatchFuncNone, nil
	}
	switch strings.ToLower(opt[0]) {
	case "regexp":
		re, err := regexp.Compile(opt[1])
		if err != nil {
			log.Warnf("Could not compile regexp '%s'", opt[1])
			return MatchFuncNone, nil
		}
		return MatchFuncRegexp, re
	default:
		img.LogContext().Warnf("Unknown match function: %s", opt[0])
		return MatchFuncNone, nil
	}
}

// GetParameterPullSecret retrieves an image's pull secret credentials
func (img *ContainerImage) GetParameterPullSecret(annotations map[string]string) *CredentialSource {
	key := fmt.Sprintf(common.SecretListAnnotation, img.normalizedSymbolicName())
	val := getAnnotationWithDefault(annotations, key, common.DefaultSecretListAnnotation, "")
	if val == "" {
		img.LogContext().Tracef("No secret annotation %s found", key)
		return nil
	}
	credSrc, err := ParseCredentialSource(val, false)
	if err != nil {
		img.LogContext().Warnf("Invalid credential reference specified: %s", val)
		return nil
	}
	return credSrc
}

// GetParameterIgnoreTags retrieves a list of tags to ignore from a comma-separated string
func (img *ContainerImage) GetParameterIgnoreTags(annotations map[string]string) []string {
	key := fmt.Sprintf(common.IgnoreTagsOptionAnnotation, img.normalizedSymbolicName())
	val := getAnnotationWithDefault(annotations, key, common.DefaultIgnoreTagsOptionAnnotation, "")
	if val == "" {
		img.LogContext().Tracef("No ignore-tags annotation %s found", key)
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

// normalizedSymbolicName returns the image's alias with all slash characters
// replaced by an underscore
func (img *ContainerImage) normalizedSymbolicName() string {
	return strings.ReplaceAll(img.ImageAlias, "/", "_")
}
