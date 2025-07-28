package image

import (
	"context"
	"fmt"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
	"regexp"
	"runtime"
	"strings"

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/common"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/options"
)

// GetParameterHelmImageName gets the value for image-name option for the image
// from a set of annotations
func (img *ContainerImage) GetParameterHelmImageName(annotations map[string]string, annotationPrefix string) string {
	key := fmt.Sprintf(common.Prefixed(annotationPrefix, common.HelmParamImageNameAnnotationSuffix), img.normalizedSymbolicName())
	val, ok := annotations[key]
	if !ok {
		return ""
	}
	return val
}

// GetParameterHelmImageTag gets the value for image-tag option for the image
// from a set of annotations
func (img *ContainerImage) GetParameterHelmImageTag(annotations map[string]string, annotationPrefix string) string {
	key := fmt.Sprintf(common.Prefixed(annotationPrefix, common.HelmParamImageTagAnnotationSuffix), img.normalizedSymbolicName())
	val, ok := annotations[key]
	if !ok {
		return ""
	}
	return val
}

// GetParameterHelmImageSpec gets the value for image-spec option for the image
// from a set of annotations
func (img *ContainerImage) GetParameterHelmImageSpec(annotations map[string]string, annotationPrefix string) string {
	key := fmt.Sprintf(common.Prefixed(annotationPrefix, common.HelmParamImageSpecAnnotationSuffix), img.normalizedSymbolicName())
	val, ok := annotations[key]
	if !ok {
		return ""
	}
	return val
}

// GetParameterKustomizeImageName gets the value for image-spec option for the
// image from a set of annotations
func (img *ContainerImage) GetParameterKustomizeImageName(annotations map[string]string, annotationPrefix string) string {
	key := fmt.Sprintf(common.Prefixed(annotationPrefix, common.KustomizeApplicationNameAnnotationSuffix), img.normalizedSymbolicName())
	val, ok := annotations[key]
	if !ok {
		return ""
	}
	return val
}

// HasForceUpdateOptionAnnotation gets the value for force-update option for the
// image from a set of annotations
func (img *ContainerImage) HasForceUpdateOptionAnnotation(annotations map[string]string, annotationPrefix string) bool {
	forceUpdateAnnotations := []string{
		fmt.Sprintf(common.Prefixed(annotationPrefix, common.ForceUpdateOptionAnnotationSuffix), img.normalizedSymbolicName()),
		common.Prefixed(annotationPrefix, common.ApplicationWideForceUpdateOptionAnnotationSuffix),
	}
	var forceUpdateVal = ""
	for _, key := range forceUpdateAnnotations {
		if val, ok := annotations[key]; ok {
			forceUpdateVal = val
			break
		}
	}
	return forceUpdateVal == "true"
}

// GetParameterSort gets and validates the value for the sort option for the
// image from a set of annotations
func (img *ContainerImage) GetParameterUpdateStrategy(annotations map[string]string, annotationPrefix string) UpdateStrategy {
	updateStrategyAnnotations := []string{
		fmt.Sprintf(common.Prefixed(annotationPrefix, common.UpdateStrategyAnnotationSuffix), img.normalizedSymbolicName()),
		common.Prefixed(annotationPrefix, common.ApplicationWideUpdateStrategyAnnotationSuffix),
	}
	var updateStrategyVal = ""
	for _, key := range updateStrategyAnnotations {
		if val, ok := annotations[key]; ok {
			updateStrategyVal = val
			break
		}
	}
	logCtx := img.LogContext()
	if updateStrategyVal == "" {
		logCtx.Tracef("No sort option found")
		// Default is sort by version
		return StrategySemVer
	}
	logCtx.Tracef("Found update strategy %s", updateStrategyVal)
	return img.ParseUpdateStrategy(context.Background(), updateStrategyVal)
}

func (img *ContainerImage) ParseUpdateStrategy(ctx context.Context, val string) UpdateStrategy {
	logCtx := log.LoggerFromContext(ctx)
	switch strings.ToLower(val) {
	case "semver":
		return StrategySemVer
	case "latest":
		logCtx.Warnf("\"latest\" strategy has been renamed to \"newest-build\". Please switch to the new convention as support for the old naming convention will be removed in future versions.")
		fallthrough
	case "newest-build":
		return StrategyNewestBuild
	case "name":
		logCtx.Warnf("\"name\" strategy has been renamed to \"alphabetical\". Please switch to the new convention as support for the old naming convention will be removed in future versions.")
		fallthrough
	case "alphabetical":
		return StrategyAlphabetical
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
func (img *ContainerImage) GetParameterMatch(annotations map[string]string, annotationPrefix string) (MatchFuncFn, interface{}) {
	allowTagsAnnotations := []string{
		fmt.Sprintf(common.Prefixed(annotationPrefix, common.AllowTagsOptionAnnotationSuffix), img.normalizedSymbolicName()),
		common.Prefixed(annotationPrefix, common.ApplicationWideAllowTagsOptionAnnotationSuffix),
	}
	var allowTagsVal = ""
	for _, key := range allowTagsAnnotations {
		if val, ok := annotations[key]; ok {
			allowTagsVal = val
			break
		}
	}
	logCtx := img.LogContext()
	if allowTagsVal == "" {
		// The old match-tag annotation is deprecated and will be subject to removal
		// in a future version.
		key := fmt.Sprintf(common.Prefixed(annotationPrefix, common.OldMatchOptionAnnotationSuffix), img.normalizedSymbolicName())
		val, ok := annotations[key]
		if ok {
			logCtx.Warnf("The 'tag-match' annotation is deprecated and subject to removal. Please use 'allow-tags' annotation instead.")
			allowTagsVal = val
		}
	}
	if allowTagsVal == "" {
		logCtx.Tracef("No match annotation found")
		return MatchFuncAny, ""
	}
	return img.ParseMatch(context.Background(), allowTagsVal)
}

// ParseMatch returns a matcher function and its argument from given value
func (img *ContainerImage) ParseMatch(ctx context.Context, val string) (MatchFuncFn, interface{}) {
	log := log.LoggerFromContext(ctx)

	if val == "" {
		log.Tracef("No tag match constraint found, allowing all tags")
		return MatchFuncAny, ""
	}

	// The special value "any" doesn't take any parameter
	if strings.ToLower(val) == "any" {
		return MatchFuncAny, nil
	}

	opt := strings.SplitN(val, ":", 2)
	if len(opt) != 2 {
		log.Warnf("Invalid match option syntax '%s', ignoring", val)
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
		log.Warnf("Unknown match function: %s", opt[0])
		return MatchFuncNone, nil
	}
}

// GetParameterIgnoreTags retrieves a list of tags to ignore from a comma-separated string
func (img *ContainerImage) GetParameterIgnoreTags(annotations map[string]string, annotationPrefix string) []string {
	ignoreTagsAnnotations := []string{
		fmt.Sprintf(common.Prefixed(annotationPrefix, common.IgnoreTagsOptionAnnotationSuffix), img.normalizedSymbolicName()),
		common.Prefixed(annotationPrefix, common.ApplicationWideIgnoreTagsOptionAnnotationSuffix),
	}
	var ignoreTagsVal = ""
	for _, key := range ignoreTagsAnnotations {
		if val, ok := annotations[key]; ok {
			ignoreTagsVal = val
			break
		}
	}
	logCtx := img.LogContext()
	if ignoreTagsVal == "" {
		logCtx.Tracef("No ignore-tags annotation found")
		return nil
	}
	ignoreList := make([]string, 0)
	tags := strings.Split(strings.TrimSpace(ignoreTagsVal), ",")
	for _, tag := range tags {
		// We ignore empty tags
		trimmed := strings.TrimSpace(tag)
		if trimmed != "" {
			ignoreList = append(ignoreList, trimmed)
		}
	}
	return ignoreList
}

// GetPlatformOptions creates manifest options with platform constraints for an image.
//
// If a `platforms` slice is provided, its contents are used to set the platform
// constraints for the image lookup.
//
// If the `platforms` slice is empty or nil, the behavior is controlled by the
// `ignoreRuntimePlatform` flag:
//   - If `ignoreRuntimePlatform` is `false` (the default, more secure behavior),
//     the platform of the running image updater process is used as a fallback
//     constraint. This ensures that by default, only images compatible with the
//     current environment are considered.
//   - If `ignoreRuntimePlatform` is `true`, no platform constraints are applied,
//     allowing images for any platform to be considered.
func (img *ContainerImage) GetPlatformOptions(ctx context.Context, ignoreRuntimePlatform bool, platforms []string) *options.ManifestOptions {
	log := log.LoggerFromContext(ctx)
	var opts *options.ManifestOptions = options.NewManifestOptions()
	if platforms == nil || len(platforms) == 0 {
		if !ignoreRuntimePlatform {
			os := runtime.GOOS
			arch := runtime.GOARCH
			variant := ""
			if strings.Contains(runtime.GOARCH, "/") {
				a := strings.SplitN(runtime.GOARCH, "/", 2)
				arch = a[0]
				variant = a[1]
			}
			log.Tracef("Using runtime platform constraint %s", options.PlatformKey(os, arch, variant))
			opts = opts.WithPlatform(os, arch, variant)
		}
	} else {
		for _, ps := range platforms {
			pt := strings.TrimSpace(ps)
			os, arch, variant, err := ParsePlatform(pt)
			if err != nil {
				// If the platform identifier could not be parsed, we set the
				// constraint intentionally to the invalid value so we don't
				// end up updating to the wrong architecture possibly.
				os = ps
				log.Warnf("could not parse platform identifier '%v': invalid format", pt)
			}
			log.Tracef("Adding platform constraint %s", options.PlatformKey(os, arch, variant))
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
