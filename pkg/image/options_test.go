package image

import (
	"fmt"
	"testing"

	"github.com/argoproj-labs/argocd-image-updater/pkg/common"

	"github.com/stretchr/testify/assert"
)

func Test_GetHelmOptions(t *testing.T) {
	t.Run("Get Helm parameter for configured application", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.HelmParamImageNameAnnotation, "dummy"): "release.name",
			fmt.Sprintf(common.HelmParamImageTagAnnotation, "dummy"):  "release.tag",
			fmt.Sprintf(common.HelmParamImageSpecAnnotation, "dummy"): "release.image",
		}

		img := NewFromIdentifier("dummy=foo/bar:1.12")
		paramName := img.GetParameterHelmImageName(annotations)
		paramTag := img.GetParameterHelmImageTag(annotations)
		paramSpec := img.GetParameterHelmImageSpec(annotations)
		assert.Equal(t, "release.name", paramName)
		assert.Equal(t, "release.tag", paramTag)
		assert.Equal(t, "release.image", paramSpec)
	})

	t.Run("Get Helm parameter for non-configured application", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.HelmParamImageNameAnnotation, "dummy"): "release.name",
			fmt.Sprintf(common.HelmParamImageTagAnnotation, "dummy"):  "release.tag",
			fmt.Sprintf(common.HelmParamImageSpecAnnotation, "dummy"): "release.image",
		}

		img := NewFromIdentifier("foo=foo/bar:1.12")
		paramName := img.GetParameterHelmImageName(annotations)
		paramTag := img.GetParameterHelmImageTag(annotations)
		paramSpec := img.GetParameterHelmImageSpec(annotations)
		assert.Equal(t, "", paramName)
		assert.Equal(t, "", paramTag)
		assert.Equal(t, "", paramSpec)
	})

	t.Run("Get Helm parameter for configured application with normalized name", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.HelmParamImageNameAnnotation, "foo_dummy"): "release.name",
			fmt.Sprintf(common.HelmParamImageTagAnnotation, "foo_dummy"):  "release.tag",
			fmt.Sprintf(common.HelmParamImageSpecAnnotation, "foo_dummy"): "release.image",
		}

		img := NewFromIdentifier("foo/dummy=foo/bar:1.12")
		paramName := img.GetParameterHelmImageName(annotations)
		paramTag := img.GetParameterHelmImageTag(annotations)
		paramSpec := img.GetParameterHelmImageSpec(annotations)
		assert.Equal(t, "release.name", paramName)
		assert.Equal(t, "release.tag", paramTag)
		assert.Equal(t, "release.image", paramSpec)
	})
}

func Test_GetSortOption(t *testing.T) {

	t.Run("Get sort option semver for configured application", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.SortOptionAnnotation, "dummy"): "semver",
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		sortMode := img.GetParameterSort(annotations)
		assert.Equal(t, VersionSortSemVer, sortMode)
	})

	t.Run("Get sort option date for configured application", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.SortOptionAnnotation, "dummy"): "date",
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		sortMode := img.GetParameterSort(annotations)
		assert.Equal(t, VersionSortLatest, sortMode)
	})

	t.Run("Get sort option name for configured application", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.SortOptionAnnotation, "dummy"): "name",
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		sortMode := img.GetParameterSort(annotations)
		assert.Equal(t, VersionSortName, sortMode)
	})

	t.Run("Get default sort option configured application because of invalid option", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.SortOptionAnnotation, "dummy"): "invalid",
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		sortMode := img.GetParameterSort(annotations)
		assert.Equal(t, VersionSortSemVer, sortMode)
	})

	t.Run("Get default sort option configured application because of option not set", func(t *testing.T) {
		annotations := map[string]string{}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		sortMode := img.GetParameterSort(annotations)
		assert.Equal(t, VersionSortSemVer, sortMode)
	})
}
