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

func Test_GetKustomizeOptions(t *testing.T) {
	t.Run("Get Helm parameter for configured application", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.KustomizeApplicationNameAnnotation, "dummy"): "argoproj/argo-cd",
		}

		img := NewFromIdentifier("dummy=foo/bar:1.12")
		paramName := img.GetParameterKustomizeImageName(annotations)
		assert.Equal(t, "argoproj/argo-cd", paramName)
	})
}

func Test_GetSortOption(t *testing.T) {

	t.Run("Get update strategy semver for configured application", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.UpdateStrategyAnnotation, "dummy"): "semver",
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		sortMode := img.GetParameterUpdateStrategy(annotations)
		assert.Equal(t, VersionSortSemVer, sortMode)
	})

	t.Run("Get update strategy date for configured application", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.UpdateStrategyAnnotation, "dummy"): "latest",
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		sortMode := img.GetParameterUpdateStrategy(annotations)
		assert.Equal(t, VersionSortLatest, sortMode)
	})

	t.Run("Get update strategy name for configured application", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.UpdateStrategyAnnotation, "dummy"): "name",
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		sortMode := img.GetParameterUpdateStrategy(annotations)
		assert.Equal(t, VersionSortName, sortMode)
	})

	t.Run("Get update strategy option configured application because of invalid option", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.UpdateStrategyAnnotation, "dummy"): "invalid",
		}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		sortMode := img.GetParameterUpdateStrategy(annotations)
		assert.Equal(t, VersionSortSemVer, sortMode)
	})

	t.Run("Get update strategy option configured application because of option not set", func(t *testing.T) {
		annotations := map[string]string{}
		img := NewFromIdentifier("dummy=foo/bar:1.12")
		sortMode := img.GetParameterUpdateStrategy(annotations)
		assert.Equal(t, VersionSortSemVer, sortMode)
	})
}
