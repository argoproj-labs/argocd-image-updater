package image

import (
	"fmt"
	"strings"

	"github.com/argoproj-labs/argocd-image-updater/pkg/common"
	"github.com/argoproj-labs/argocd-image-updater/pkg/log"
)

// GetParameterHelmImageName gets the value for image-name option for the image
// from a set of annotations
func (img *ContainerImage) GetParameterHelmImageName(annotations map[string]string) string {
	key := fmt.Sprintf(common.HelmParamImageNameAnnotation, img.normalizedSymbolicName())
	val, ok := annotations[key]
	if !ok {
		return ""
	}
	return val
}

// GetParameterHelmImageTag gets the value for image-tag option for the image
// from a set of annotations
func (img *ContainerImage) GetParameterHelmImageTag(annotations map[string]string) string {
	key := fmt.Sprintf(common.HelmParamImageTagAnnotation, img.normalizedSymbolicName())
	val, ok := annotations[key]
	if !ok {
		return ""
	}
	return val
}

// GetParameterHelmImageSpec gets the value for image-spec option for the image
// from a set of annotations
func (img *ContainerImage) GetParameterHelmImageSpec(annotations map[string]string) string {
	key := fmt.Sprintf(common.HelmParamImageSpecAnnotation, img.normalizedSymbolicName())
	val, ok := annotations[key]
	if !ok {
		return ""
	}
	return val
}

// GetParameterKustomizeImageName gets the value for image-spec option for the
// image from a set of annotations
func (img *ContainerImage) GetParameterKustomizeImageName(annotations map[string]string) string {
	key := fmt.Sprintf(common.KustomizeApplicationNameAnnotation, img.normalizedSymbolicName())
	val, ok := annotations[key]
	if !ok {
		return ""
	}
	return val
}

// GetParameterSort gets and validates the value for the sort option for the
// image from a set of annotations
func (img *ContainerImage) GetParameterSort(annotations map[string]string) VersionSortMode {
	key := fmt.Sprintf(common.SortOptionAnnotation, img.normalizedSymbolicName())
	val, ok := annotations[key]
	if !ok {
		// Default is sort by version
		log.Tracef("No sort option %s found", key)
		return VersionSortSemVer
	}
	switch strings.ToLower(val) {
	case "semver":
		log.Tracef("Sort option semver in %s", key)
		return VersionSortSemVer
	case "date":
		log.Tracef("Sort option date in %s", key)
		return VersionSortLatest
	case "name":
		log.Tracef("Sort option name in %s", key)
		return VersionSortName
	default:
		log.Warnf("Unknown sort option in %s: %s -- using semver", key, val)
		return VersionSortSemVer
	}
}

func (img *ContainerImage) normalizedSymbolicName() string {
	return strings.ReplaceAll(img.ImageAlias, "/", "_")
}
