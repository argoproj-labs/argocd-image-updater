package image

import (
	"sort"

	"github.com/argoproj-labs/argocd-image-updater/pkg/log"

	"github.com/Masterminds/semver"
)

// GetNewestVersionFromTags returns the latest available version from a list of
// tags while optionally taking a semver constraint into account. Returns the
// original version if no new version could be found from the list of tags.
func (img *ContainerImage) GetNewestVersionFromTags(constraint string, availableTags []string) (string, error) {
	logCtx := log.NewContext()
	logCtx.AddField("image", img.String())

	// It makes no sense to proceed if we have no available tags
	if len(availableTags) == 0 {
		return img.ImageTag, nil
	}

	_, err := semver.NewVersion(img.ImageTag)
	if err != nil {
		return "", err
	}

	// The given constraint MUST match a semver constraint
	var semverConstraint *semver.Constraints
	if constraint != "" {
		semverConstraint, err = semver.NewConstraint(constraint)
		if err != nil {
			logCtx.Errorf("invalid constraint '%s' given: '%v'", constraint, err)
			return "", err
		}
	}

	tagVersions := make([]*semver.Version, 0)

	// Loop through all tags to check whether it's an update candidate.
	for _, tag := range availableTags {

		// Non-parseable tag does not mean error - just skip it
		ver, err := semver.NewVersion(tag)
		if err != nil {
			continue
		}

		// If we have a version constraint, check image tag against it. If the
		// constraint is not satisfied, skip tag.
		if semverConstraint != nil {
			if !semverConstraint.Check(ver) {
				continue
			}
		}

		// Append tag as update candidate
		tagVersions = append(tagVersions, ver)
	}

	logCtx.Debugf("found %d from %d tags eligible for consideration", len(tagVersions), len(availableTags))

	// Sort update candidates and return the most recent version in its original
	// form, so we can later fetch it from the registry.
	if len(tagVersions) > 0 {
		sort.Sort(semver.Collection(tagVersions))
		return tagVersions[len(tagVersions)-1].Original(), nil
	} else {
		return img.ImageTag, nil
	}
}
