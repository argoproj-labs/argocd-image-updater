package image

import (
	"github.com/argoproj-labs/argocd-image-updater/pkg/log"
	"github.com/argoproj-labs/argocd-image-updater/pkg/tag"

	"github.com/Masterminds/semver"
)

// VersionSort defines the method to sort a list of tags
type VersionSort int

const (
	// VersionSortSemVer sorts tags using semver sorting (the default)
	VersionSortSemVer = 0
	// VersionSortLatest sorts tags after their creation date
	VersionSortLatest = 1
	// VersionSortName sorts tags alphabetically by name
	VersionSortName = 2
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

	var availableTags tag.SortableImageTagList
	switch vc.SortMode {
	case VersionSortSemVer:
		availableTags = tagList.SortBySemVer()
	case VersionSortName:
		availableTags = tagList.SortByName()
	case VersionSortLatest:
		availableTags = tagList.SortByDate()
	}

	considerTags := tag.SortableImageTagList{}

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

	// Loop through all tags to check whether it's an update candidate.
	for _, tag := range availableTags {

		// Non-parseable tag does not mean error - just skip it
		ver, err := semver.NewVersion(tag.TagName)
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
		considerTags = append(considerTags, tag)
	}

	logCtx.Debugf("found %d from %d tags eligible for consideration", len(considerTags), len(availableTags))

	// Sort update candidates and return the most recent version in its original
	// form, so we can later fetch it from the registry.
	if len(considerTags) > 0 {
		return considerTags[len(considerTags)-1], nil
	} else {
		return img.ImageTag, nil
	}
}
