package image

import (
	"path/filepath"

	"github.com/argoproj-labs/argocd-image-updater/pkg/log"
	"github.com/argoproj-labs/argocd-image-updater/pkg/tag"

	"github.com/Masterminds/semver"
)

// VersionSortMode defines the method to sort a list of tags
type VersionSortMode int

const (
	// VersionSortSemVer sorts tags using semver sorting (the default)
	VersionSortSemVer VersionSortMode = 0
	// VersionSortLatest sorts tags after their creation date
	VersionSortLatest VersionSortMode = 1
	// VersionSortName sorts tags alphabetically by name
	VersionSortName VersionSortMode = 2
)

// ConstraintMatchMode defines how the constraint should be matched
type ConstraintMatchMode int

const (
	// ConstraintMatchSemVer uses semver to match a constraint
	ConstraintMatchSemver ConstraintMatchMode = 0
	// ConstraintMatchRegExp uses regexp to match a constraint
	ConstraintMatchRegExp ConstraintMatchMode = 1
	// ConstraintMatchNone does not enforce a constraint
	ConstraintMatchNone ConstraintMatchMode = 2
)

// VersionConstraint defines a constraint for comparing versions
type VersionConstraint struct {
	Constraint string
	MatchFunc  MatchFuncFn
	MatchArgs  interface{}
	IgnoreList []string
	SortMode   VersionSortMode
}

type MatchFuncFn func(tagName string, pattern interface{}) bool

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

	// The given constraint MUST match a semver constraint
	var semverConstraint *semver.Constraints
	var err error
	if vc.SortMode == VersionSortSemVer {
		if img.ImageTag != nil {
			_, err := semver.NewVersion(img.ImageTag.TagName)
			if err != nil {
				return nil, err
			}
		}

		if vc.Constraint != "" {
			semverConstraint, err = semver.NewConstraint(vc.Constraint)
			if err != nil {
				logCtx.Errorf("invalid constraint '%s' given: '%v'", vc, err)
				return nil, err
			}
		}
	}

	// Loop through all tags to check whether it's an update candidate.
	for _, tag := range availableTags {
		logCtx.Tracef("Finding out whether to consider %s for being updateable", tag.TagName)

		if vc.SortMode == VersionSortSemVer {
			// Non-parseable tag does not mean error - just skip it
			ver, err := semver.NewVersion(tag.TagName)
			if err != nil {
				logCtx.Tracef("Not a valid version: %s", tag.TagName)
				continue
			}

			// If we have a version constraint, check image tag against it. If the
			// constraint is not satisfied, skip tag.
			if semverConstraint != nil {
				if !semverConstraint.Check(ver) {
					logCtx.Tracef("%s did not match constraint %s", ver.Original(), vc.Constraint)
					continue
				}
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

// IsTagIgnored matches tag against the patterns in IgnoreList and returns true if one of them matches
func (vc *VersionConstraint) IsTagIgnored(tag string) bool {
	for _, t := range vc.IgnoreList {
		if match, err := filepath.Match(t, tag); err == nil && match {
			log.Tracef("tag %s is ignored by pattern %s", tag, t)
			return true
		}
	}
	return false
}
