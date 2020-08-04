package argocd

import (
	"fmt"
	"testing"

	"github.com/argoproj-labs/argocd-image-updater/pkg/common"

	"github.com/argoproj/argo-cd/pkg/apis/application/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func Test_GetHelmParamAnnotations(t *testing.T) {
	t.Run("Get parameter names without symbolic names", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.HelmParamImageSpecAnnotation, "myimg"): "image.blub",
			fmt.Sprintf(common.HelmParamImageTagAnnotation, "myimg"):  "image.blab",
		}
		name, tag := getHelmParamNamesFromAnnotation(annotations, "")
		assert.Equal(t, "image.name", name)
		assert.Equal(t, "image.tag", tag)
	})

	t.Run("Find existing image spec annotation", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.HelmParamImageSpecAnnotation, "myimg"): "image.path",
			fmt.Sprintf(common.HelmParamImageTagAnnotation, "myimg"):  "image.tag",
		}
		name, tag := getHelmParamNamesFromAnnotation(annotations, "myimg")
		assert.Equal(t, "image.path", name)
		assert.Empty(t, tag)
	})

	t.Run("Find existing image name and image tag annotations", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.HelmParamImageNameAnnotation, "myimg"): "image.name",
			fmt.Sprintf(common.HelmParamImageTagAnnotation, "myimg"):  "image.tag",
		}
		name, tag := getHelmParamNamesFromAnnotation(annotations, "myimg")
		assert.Equal(t, "image.name", name)
		assert.Equal(t, "image.tag", tag)
	})

	t.Run("Find non-existing image name and image tag annotations", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.HelmParamImageNameAnnotation, "otherimg"): "image.name",
			fmt.Sprintf(common.HelmParamImageTagAnnotation, "otherimg"):  "image.tag",
		}
		name, tag := getHelmParamNamesFromAnnotation(annotations, "myimg")
		assert.Empty(t, name)
		assert.Empty(t, tag)
	})

	t.Run("Find existing image tag annotations", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.HelmParamImageTagAnnotation, "myimg"): "image.tag",
		}
		name, tag := getHelmParamNamesFromAnnotation(annotations, "myimg")
		assert.Empty(t, name)
		assert.Equal(t, "image.tag", tag)
	})

	t.Run("No suitable annotations found", func(t *testing.T) {
		annotations := map[string]string{}
		name, tag := getHelmParamNamesFromAnnotation(annotations, "myimg")
		assert.Empty(t, name)
		assert.Empty(t, tag)
	})

}

func Test_MergeHelmParams(t *testing.T) {
	t.Run("Merge set with existing parameters", func(t *testing.T) {
		srcParams := []v1alpha1.HelmParameter{
			{
				Name:  "someparam",
				Value: "somevalue",
			},
			{
				Name:  "image.name",
				Value: "foobar",
			},
			{
				Name:  "otherparam",
				Value: "othervalue",
			},
			{
				Name:  "image.tag",
				Value: "1.2.3",
			},
		}
		mergeParams := []v1alpha1.HelmParameter{
			{
				Name:  "image.name",
				Value: "foobar",
			},
			{
				Name:  "image.tag",
				Value: "1.2.4",
			},
		}

		dstParams := mergeHelmParams(srcParams, mergeParams)

		param := getHelmParam(dstParams, "someparam")
		require.NotNil(t, param)
		assert.Equal(t, "somevalue", param.Value)

		param = getHelmParam(dstParams, "otherparam")
		require.NotNil(t, param)
		assert.Equal(t, "othervalue", param.Value)

		param = getHelmParam(dstParams, "image.name")
		require.NotNil(t, param)
		assert.Equal(t, "foobar", param.Value)

		param = getHelmParam(dstParams, "image.tag")
		require.NotNil(t, param)
		assert.Equal(t, "1.2.4", param.Value)
	})

	t.Run("Merge set with empty src parameters", func(t *testing.T) {
		srcParams := []v1alpha1.HelmParameter{}
		mergeParams := []v1alpha1.HelmParameter{
			{
				Name:  "image.name",
				Value: "foobar",
			},
			{
				Name:  "image.tag",
				Value: "1.2.4",
			},
		}

		dstParams := mergeHelmParams(srcParams, mergeParams)

		param := getHelmParam(dstParams, "image.name")
		require.NotNil(t, param)
		assert.Equal(t, "foobar", param.Value)

		param = getHelmParam(dstParams, "image.tag")
		require.NotNil(t, param)
		assert.Equal(t, "1.2.4", param.Value)
	})
}
