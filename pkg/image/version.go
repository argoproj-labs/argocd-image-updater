package image

import (
	"sort"
	"time"

	"github.com/argoproj-labs/argocd-image-updater/pkg/log"
	"github.com/argoproj-labs/argocd-image-updater/pkg/tag"

	"github.com/Masterminds/semver"
)

// VersionSort defines the method to sort a list of tags
type VersionSort int

const (
	// VersionSortSemVer sorts tags using semver sorting
	VersionSortSemVer = 1
	// VersionSortLatest sorts tags after their creation date
	VersionSortLatest = 2
	// VersionSortName sorts tags alphabetically by name
	VersionSortName = 3
)

// VersionConstraint defines a constraint for comparing versions
type VersionConstraint struct {
	Constraint    string
	EnforceSemVer bool
	SortMode      VersionSort
}

// String returns the string representation of VersionConstraint
func (vc *VersionConstraint) String() string {
	return vc.Constraint
}

// GetNewestVersionFromTags returns the latest available version from a list of
// tags while optionally taking a semver constraint into account. Returns the
// original version if no new version could be found from the list of tags.
func (img *ContainerImage) GetNewestVersionFromTags(vc *VersionConstraint, tagList *tag.ImageTagList) (*tag.ImageTag, error) {
	logCtx := log.NewContext()
	logCtx.AddField("image", img.String())

	availableTags := tagList.Tags()

	// It makes no sense to proceed if we have no available tags
	if len(availableTags) == 0 {
		return img.ImageTag, nil
	}

	_, err := semver.NewVersion(img.ImageTag.TagName)
	if err != nil {
		return nil, err
	}

	// The given constraint MUST match a semver constraint
	var semverConstraint *semver.Constraints
	if vc.Constraint != "" {
		semverConstraint, err = semver.NewConstraint(vc.Constraint)
		if err != nil {
			logCtx.Errorf("invalid constraint '%s' given: '%v'", vc, err)
			return nil, err
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
		return tag.NewImageTag(tagVersions[len(tagVersions)-1].Original(), time.Unix(0, 0)), nil
	} else {
		return img.ImageTag, nil
	}
}
