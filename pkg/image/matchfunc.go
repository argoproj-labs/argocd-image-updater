package image

import (
	"regexp"
)

// MatchFuncAny matches any pattern, i.e. always returns true
func MatchFuncAny(tagName string) bool {
	return true
}

// MatchFuncNone matches no pattern, i.e. always returns false
func MatchFuncNone(tagName string) bool {
	return false
}

// MatchFuncRegexp matches the tagName against regexp pattern and returns the result
func MatchFuncRegexpFactory(pattern *regexp.Regexp) MatchFuncFn {
	return func(tagName string) bool {
		return pattern.Match([]byte(tagName))
	}
}
