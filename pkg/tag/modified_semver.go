package tag

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// semverCollection is a replacement for semver.Collection that breaks version
// comparison ties through a lexical comparison of the original version strings.
// Using this, instead of semver.Collection, when sorting will yield
// deterministic results that semver.Collection will not yield.
type modifiedSemverCollection []*semver.Version

// Len returns the length of a collection. The number of Version instances
// on the slice.
func (s modifiedSemverCollection) Len() int {
	return len(s)
}

// Less is needed for the sort interface to compare two Version objects on the
// slice. If checks if one is less than the other.
func (s modifiedSemverCollection) Less(i, j int) bool {
	const (
		hf = "hotfix"
		rc = "release-candidate"
		pr = "prod"
	)
	baseI, envI, suffixNumI := extractBaseAndSuffix(s[i].Original())
	baseJ, envJ, suffixNumJ := extractBaseAndSuffix(s[j].Original())

	// Compare base versions without suffixes
	baseVersionI, _ := semver.NewVersion(baseI)
	baseVersionJ, _ := semver.NewVersion(baseJ)
	comp := baseVersionI.Compare(baseVersionJ)

	// if the semvars aren't equal, return true if I is smaller or false if J
	if comp != 0 {
		return comp < 0
	}

	// the semvars are equal. Now things get complicated
	switch {
	case envI == hf && envJ == hf:
		// both are hotfixes, so higher suffix wins
		return suffixNumI < suffixNumJ
	case envI == rc && envJ == rc:
		// both are rc's, so higher suffix wins
		return suffixNumI < suffixNumJ
	case envI == hf && envJ == rc:
		// hotfix beats RC
		return false
	case envI == rc && envJ == hf:
		// hotfix's beats RC
		return true
	case envI == pr && envJ == rc:
		// rc beats prod
		return true
	case envI == rc && envJ == pr:
		// rc beats prod
		return false
	case envI == pr && envJ == hf:
		// hotfix beats prod
		return true
	case envI == hf && envJ == pr:
		// hotfix beats prod
		return false
	default:
		// this should never happen
		return suffixNumI < suffixNumJ
	}
}

// Swap is needed for the sort interface to replace the Version objects
// at two different positions in the slice.
func (s modifiedSemverCollection) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}

func extractBaseAndSuffix(version string) (ver, environment string, suffix int) {
	// Regex to match a base version and an optional suffix with .n
	re := regexp.MustCompile(`^([^-]+)(?:-(.*))?`)
	matches := re.FindStringSubmatch(version)
	ver = matches[1]

	if len(matches) > 2 && matches[2] != "" {
		// Check for a trailing .n in the suffix, and extract the number if present
		suffixParts := strings.Split(matches[2], ".")
		environment = suffixParts[0]
		if len(suffixParts) > 1 {
			num, err := strconv.Atoi(suffixParts[len(suffixParts)-1])
			if err == nil {
				suffix = num
			}
		}
	}

	return ver, environment, suffix
}
