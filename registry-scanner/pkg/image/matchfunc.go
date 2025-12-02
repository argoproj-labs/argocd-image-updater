package image

import (
	"regexp"

	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"
	"github.com/k1LoW/calver"
)

// MatchFuncAny matches any pattern, i.e. always returns true
func MatchFuncAny(tagName string, args interface{}) bool {
	return true
}

// MatchFuncNone matches no pattern, i.e. always returns false
func MatchFuncNone(tagName string, args interface{}) bool {
	return false
}

// MatchFuncRegexp matches the tagName against regexp pattern and returns the result
func MatchFuncRegexp(tagName string, args interface{}) bool {
	pattern, ok := args.(*regexp.Regexp)
	if !ok {
		log.Errorf("args is not a RegExp")
		return false
	}
	return pattern.Match([]byte(tagName))
}

// MatchFuncCalVer checks if a tag matches the specified CalVer layout
func MatchFuncCalVer(tagName string, args interface{}) bool {
	layoutStr, ok := args.(string)
	if !ok {
		return false
	}
	_, err := calver.Parse(layoutStr, tagName)
	return err == nil
}
