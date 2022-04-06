package image

import (
	"fmt"
	"regexp"

	"github.com/Masterminds/semver"
)

// SemVerTransformFuncNone doesn't perform any transformation, i.e. always returns the tagName
func SemVerTransformFuncNone(tagName string) (*semver.Version, error) {
	return semver.NewVersion(tagName)
}

// SemVerTransformerFuncRegexpFactory builds a transformer that uses a regular expression to
// parse before transforming.
func SemVerTransformerFuncRegexpFactory(pattern *regexp.Regexp) SemVerTransformFuncFn {
	return func(tagName string) (*semver.Version, error) {
		tagName = pattern.FindString(tagName)
		if len(tagName) == 0 {
			return nil, fmt.Errorf("failed to match %q", tagName)
		}

		return semver.NewVersion(tagName)
	}
}
