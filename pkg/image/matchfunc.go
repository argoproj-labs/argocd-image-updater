package image

import (
	"regexp"

	"github.com/argoproj-labs/argocd-image-updater/pkg/log"
)

// MatchFuncAny matches any pattern, i.e. always returns true
func MatchFuncAny(tagName string, args interface{}) (string, bool) {
	return tagName, true
}

// MatchFuncNone matches no pattern, i.e. always returns false
func MatchFuncNone(tagName string, args interface{}) (string, bool) {
	return tagName, false
}

// MatchFuncRegexp matches the tagName against regexp pattern and returns the result
func MatchFuncRegexp(tagName string, args interface{}) (string, bool) {
	pattern, ok := args.(*regexp.Regexp)
	if !ok {
		log.Errorf("args is not a RegExp")
		return tagName, false
	}
	matches := pattern.FindStringSubmatch(tagName)
	switch len(matches) {
	case 0:
		return "", false
	case 1:
		return tagName, true
	default:
		return matches[1], true
	}
}
