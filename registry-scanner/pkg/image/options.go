package image

import (
	"context"
	"fmt"
	"regexp"
	"runtime"
	"strings"

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/options"
)

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
	if len(platforms) == 0 {
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
