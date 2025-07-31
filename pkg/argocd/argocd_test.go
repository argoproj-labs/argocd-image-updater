package argocd

import (
	"context"
	"fmt"
	"testing"

	"github.com/argoproj/argo-cd/v2/pkg/apiclient/application"
	"github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	ctrlFake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	api "github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"
	"github.com/argoproj-labs/argocd-image-updater/pkg/common"
	"github.com/argoproj-labs/argocd-image-updater/pkg/kube"
	registryCommon "github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/common"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/image"
	registryKube "github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/kube"
	"github.com/argoproj-labs/argocd-image-updater/test/fake"
)

func Test_GetImagesFromApplication(t *testing.T) {
	t.Run("Get list of images from application", func(t *testing.T) {
		application := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
			},
			Spec: v1alpha1.ApplicationSpec{},
			Status: v1alpha1.ApplicationStatus{
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{"nginx:1.12.2", "that/image", "quay.io/dexidp/dex:v1.23.0"},
				},
			},
		}
		applicationImages := &ApplicationImages{
			Application: *application,
			Images:      ImageList{},
		}
		imageList := GetImagesFromApplication(applicationImages)
		require.Len(t, imageList, 3)
		assert.Equal(t, "nginx", imageList[0].ImageName)
		assert.Equal(t, "that/image", imageList[1].ImageName)
		assert.Equal(t, "dexidp/dex", imageList[2].ImageName)
	})

	t.Run("Get list of images from application that has no images", func(t *testing.T) {
		application := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
			},
			Spec: v1alpha1.ApplicationSpec{},
			Status: v1alpha1.ApplicationStatus{
				Summary: v1alpha1.ApplicationSummary{},
			},
		}
		applicationImages := &ApplicationImages{
			Application: *application,
			Images:      ImageList{},
		}
		imageList := GetImagesFromApplication(applicationImages)
		assert.Empty(t, imageList)
	})

	t.Run("Get list of images from application that has force-update", func(t *testing.T) {
		application := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					fmt.Sprintf(registryCommon.Prefixed(common.ImageUpdaterAnnotationPrefix, registryCommon.ForceUpdateOptionAnnotationSuffix), "nginx"): "true",
					common.ImageUpdaterAnnotation: "nginx=nginx",
				},
			},
			Spec: v1alpha1.ApplicationSpec{},
			Status: v1alpha1.ApplicationStatus{
				Summary: v1alpha1.ApplicationSummary{},
			},
		}
		imgToUpdate := image.NewFromIdentifier("nginx")
		image := NewImage(imgToUpdate)
		image.ForceUpdate = true

		applicationImages := &ApplicationImages{
			Application: *application,
			Images:      ImageList{image},
		}

		imageList := GetImagesFromApplication(applicationImages)
		require.Len(t, imageList, 1)
		assert.Equal(t, "nginx", imageList[0].ImageName)
		assert.Nil(t, imageList[0].ImageTag)
	})
}

func Test_GetImagesAndAliasesFromApplication(t *testing.T) {
	t.Run("Get list of images from application", func(t *testing.T) {
		application := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
			},
			Spec: v1alpha1.ApplicationSpec{},
			Status: v1alpha1.ApplicationStatus{
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{"nginx:1.12.2", "that/image", "quay.io/dexidp/dex:v1.23.0"},
				},
			},
		}
		applicationImages := &ApplicationImages{
			Application: *application,
			Images:      ImageList{},
		}

		imageList := GetImagesAndAliasesFromApplication(applicationImages)

		require.Len(t, imageList, 3)
		assert.Equal(t, "nginx", imageList[0].ImageName)
		assert.Equal(t, "", imageList[0].ImageAlias, "No alias should be set")
		assert.Equal(t, "that/image", imageList[1].ImageName)
		assert.Equal(t, "dexidp/dex", imageList[2].ImageName)
	})

	t.Run("Get list of images and image aliases from application that has no images", func(t *testing.T) {
		application := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
			},
			Spec: v1alpha1.ApplicationSpec{},
			Status: v1alpha1.ApplicationStatus{
				Summary: v1alpha1.ApplicationSummary{},
			},
		}
		applicationImages := &ApplicationImages{
			Application: *application,
			Images:      ImageList{},
		}

		imageList := GetImagesAndAliasesFromApplication(applicationImages)
		assert.Empty(t, imageList)
	})
}

func Test_GetApplicationType(t *testing.T) {
	t.Run("Get application of type Helm", func(t *testing.T) {
		application := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
			},
			Spec: v1alpha1.ApplicationSpec{},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeHelm,
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{"nginx:1.12.2", "that/image", "quay.io/dexidp/dex:v1.23.0"},
				},
			},
		}
		appType := GetApplicationType(application, nil)
		assert.Equal(t, ApplicationTypeHelm, appType)
		assert.Equal(t, "Helm", appType.String())
	})

	t.Run("Get application of type Kustomize", func(t *testing.T) {
		application := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
			},
			Spec: v1alpha1.ApplicationSpec{},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeKustomize,
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{"nginx:1.12.2", "that/image", "quay.io/dexidp/dex:v1.23.0"},
				},
			},
		}
		appType := GetApplicationType(application, nil)
		assert.Equal(t, ApplicationTypeKustomize, appType)
		assert.Equal(t, "Kustomize", appType.String())
	})

	t.Run("Get application of unknown Type", func(t *testing.T) {
		application := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
			},
			Spec: v1alpha1.ApplicationSpec{},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypePlugin,
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{"nginx:1.12.2", "that/image", "quay.io/dexidp/dex:v1.23.0"},
				},
			},
		}
		appType := GetApplicationType(application, nil)
		assert.Equal(t, ApplicationTypeUnsupported, appType)
		assert.Equal(t, "Unsupported", appType.String())
	})

	t.Run("Get application with kustomize target", func(t *testing.T) {
		application := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					common.WriteBackTargetAnnotation: "kustomization:.",
				},
			},
			Spec: v1alpha1.ApplicationSpec{},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypePlugin,
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{"nginx:1.12.2", "that/image", "quay.io/dexidp/dex:v1.23.0"},
				},
			},
		}
		// Create a WriteBackConfig with kustomization target to test the logic
		wbc := &WriteBackConfig{
			Target: "kustomization:.",
		}
		appType := GetApplicationType(application, wbc)
		assert.Equal(t, ApplicationTypeKustomize, appType)
	})

}

func Test_GetApplicationSourceType(t *testing.T) {
	t.Run("Get application Source Type for Helm", func(t *testing.T) {
		application := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
			},
			Spec: v1alpha1.ApplicationSpec{},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeHelm,
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{"nginx:1.12.2", "that/image", "quay.io/dexidp/dex:v1.23.0"},
				},
			},
		}
		appType := GetApplicationSourceType(application, nil)
		assert.Equal(t, v1alpha1.ApplicationSourceTypeHelm, appType)
	})

	t.Run("Get application Source type for Kustomize", func(t *testing.T) {
		application := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
			},
			Spec: v1alpha1.ApplicationSpec{},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeKustomize,
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{"nginx:1.12.2", "that/image", "quay.io/dexidp/dex:v1.23.0"},
				},
			},
		}
		appType := GetApplicationSourceType(application, nil)
		assert.Equal(t, v1alpha1.ApplicationSourceTypeKustomize, appType)
	})

	t.Run("Get application of unknown Type", func(t *testing.T) {
		application := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
			},
			Spec: v1alpha1.ApplicationSpec{},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypePlugin,
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{"nginx:1.12.2", "that/image", "quay.io/dexidp/dex:v1.23.0"},
				},
			},
		}
		appType := GetApplicationType(application, nil)
		assert.NotEqual(t, v1alpha1.ApplicationSourceTypeHelm, appType)
		assert.NotEqual(t, v1alpha1.ApplicationSourceTypeKustomize, appType)
	})

	t.Run("Get application Source type with kustomize target", func(t *testing.T) {
		application := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					common.WriteBackTargetAnnotation: "kustomization:.",
				},
			},
			Spec: v1alpha1.ApplicationSpec{},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypePlugin,
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{"nginx:1.12.2", "that/image", "quay.io/dexidp/dex:v1.23.0"},
				},
			},
		}

		// Create a WriteBackConfig with kustomization target to test the logic
		wbc := &WriteBackConfig{
			Target: "kustomization:.",
		}

		appType := GetApplicationSourceType(application, wbc)
		assert.Equal(t, v1alpha1.ApplicationSourceTypeKustomize, appType)
	})
}

func Test_GetApplicationSource(t *testing.T) {
	t.Run("Get application Source for Helm from monosource application", func(t *testing.T) {
		application := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "testns",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					Helm: &v1alpha1.ApplicationSourceHelm{
						Parameters: []v1alpha1.HelmParameter{
							{
								Name:  "image.tag",
								Value: "1.0.0",
							},
						},
					},
				},
			},
			Status: v1alpha1.ApplicationStatus{},
		}

		appSource := GetApplicationSource(application)
		assert.NotNil(t, appSource.Helm)
	})

	t.Run("Get application Source for Kustomize from monosource application", func(t *testing.T) {
		application := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "testns",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					Kustomize: &v1alpha1.ApplicationSourceKustomize{
						Images: v1alpha1.KustomizeImages{
							"jannfis/foobar:1.0.0",
						},
					},
				},
			},
			Status: v1alpha1.ApplicationStatus{},
		}

		appSource := GetApplicationSource(application)
		assert.NotNil(t, appSource.Kustomize)
	})

	t.Run("Get application of unknown Type", func(t *testing.T) {
		application := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "testns",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL: "https://example.argocd",
				},
			},
			Status: v1alpha1.ApplicationStatus{},
		}

		appSource := GetApplicationSource(application)
		assert.NotEmpty(t, appSource)
	})

	t.Run("Get application Source for Helm from multisource application", func(t *testing.T) {
		application := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "testns",
			},
			Spec: v1alpha1.ApplicationSpec{
				Sources: v1alpha1.ApplicationSources{
					v1alpha1.ApplicationSource{
						Path: "sources/source1",
						Helm: &v1alpha1.ApplicationSourceHelm{
							Parameters: []v1alpha1.HelmParameter{
								{
									Name:  "image.tag",
									Value: "1.0.0",
								},
							},
						},
					},
					v1alpha1.ApplicationSource{
						Path: "sources/source2",
					},
				},
			},
			Status: v1alpha1.ApplicationStatus{},
		}

		appSource := GetApplicationSource(application)
		assert.NotNil(t, appSource.Helm)
	})

	t.Run("Get application Source for Kustomize from multisource application", func(t *testing.T) {
		application := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "testns",
			},
			Spec: v1alpha1.ApplicationSpec{
				Sources: v1alpha1.ApplicationSources{
					v1alpha1.ApplicationSource{
						Path: "sources/source1",
						Kustomize: &v1alpha1.ApplicationSourceKustomize{
							Images: v1alpha1.KustomizeImages{
								"jannfis/foobar:1.0.0",
							},
						},
					},
					v1alpha1.ApplicationSource{
						Path: "sources/source2",
					},
				},
			},
			Status: v1alpha1.ApplicationStatus{},
		}

		appSource := GetApplicationSource(application)
		assert.NotNil(t, appSource.Kustomize)
	})

	t.Run("Return first Source for not Kustomize neither Helm from multisource application", func(t *testing.T) {
		application := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "testns",
			},
			Spec: v1alpha1.ApplicationSpec{
				Sources: v1alpha1.ApplicationSources{
					v1alpha1.ApplicationSource{
						Path: "sources/source1",
					},
					v1alpha1.ApplicationSource{
						Path: "sources/source2",
					},
				},
			},
			Status: v1alpha1.ApplicationStatus{},
		}

		appSource := GetApplicationSource(application)
		assert.NotEmpty(t, appSource)
		assert.Equal(t, appSource.Path, "sources/source1")
	})

}

func Test_GetHelmParamAnnotations(t *testing.T) {
	t.Run("Get parameter names without symbolic names", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(registryCommon.Prefixed(common.ImageUpdaterAnnotationPrefix, registryCommon.HelmParamImageSpecAnnotationSuffix), "myimg"): "image.blub",
			fmt.Sprintf(registryCommon.Prefixed(common.ImageUpdaterAnnotationPrefix, registryCommon.HelmParamImageTagAnnotationSuffix), "myimg"):  "image.blab",
		}
		name, tag := getHelmParamNamesFromAnnotation(annotations, &image.ContainerImage{
			ImageAlias: "",
		})
		assert.Equal(t, "image.name", name)
		assert.Equal(t, "image.tag", tag)
	})

	t.Run("Find existing image spec annotation", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(registryCommon.Prefixed(common.ImageUpdaterAnnotationPrefix, registryCommon.HelmParamImageSpecAnnotationSuffix), "myimg"): "image.path",
			fmt.Sprintf(registryCommon.Prefixed(common.ImageUpdaterAnnotationPrefix, registryCommon.HelmParamImageTagAnnotationSuffix), "myimg"):  "image.tag",
		}
		name, tag := getHelmParamNamesFromAnnotation(annotations, &image.ContainerImage{
			ImageAlias: "myimg",
		})
		assert.Equal(t, "image.path", name)
		assert.Empty(t, tag)
	})

	t.Run("Find existing image name and image tag annotations", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(registryCommon.Prefixed(common.ImageUpdaterAnnotationPrefix, registryCommon.HelmParamImageNameAnnotationSuffix), "myimg"): "image.name",
			fmt.Sprintf(registryCommon.Prefixed(common.ImageUpdaterAnnotationPrefix, registryCommon.HelmParamImageTagAnnotationSuffix), "myimg"):  "image.tag",
		}
		name, tag := getHelmParamNamesFromAnnotation(annotations, &image.ContainerImage{
			ImageAlias: "myimg",
		})
		assert.Equal(t, "image.name", name)
		assert.Equal(t, "image.tag", tag)
	})

	t.Run("Find non-existing image name and image tag annotations", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(registryCommon.Prefixed(common.ImageUpdaterAnnotationPrefix, registryCommon.HelmParamImageNameAnnotationSuffix), "otherimg"): "image.name",
			fmt.Sprintf(registryCommon.Prefixed(common.ImageUpdaterAnnotationPrefix, registryCommon.HelmParamImageTagAnnotationSuffix), "otherimg"):  "image.tag",
		}
		name, tag := getHelmParamNamesFromAnnotation(annotations, &image.ContainerImage{
			ImageAlias: "myimg",
		})
		assert.Empty(t, name)
		assert.Empty(t, tag)
	})

	t.Run("Find existing image tag annotations", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(registryCommon.Prefixed(common.ImageUpdaterAnnotationPrefix, registryCommon.HelmParamImageTagAnnotationSuffix), "myimg"): "image.tag",
		}
		name, tag := getHelmParamNamesFromAnnotation(annotations, &image.ContainerImage{
			ImageAlias: "myimg",
		})
		assert.Empty(t, name)
		assert.Equal(t, "image.tag", tag)
	})

	t.Run("No suitable annotations found", func(t *testing.T) {
		annotations := map[string]string{}
		name, tag := getHelmParamNamesFromAnnotation(annotations, &image.ContainerImage{
			ImageAlias: "myimg",
		})
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

func Test_SetKustomizeImage(t *testing.T) {
	t.Run("Test set Kustomize image parameters on Kustomize app with param already set", func(t *testing.T) {
		app := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "testns",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					Kustomize: &v1alpha1.ApplicationSourceKustomize{
						Images: v1alpha1.KustomizeImages{
							"jannfis/foobar:1.0.0",
						},
					},
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeKustomize,
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{
						"jannfis/foobar:1.0.0",
					},
				},
			},
		}
		img := image.NewFromIdentifier("jannfis/foobar:1.0.1")
		wbc := &WriteBackConfig{
			Target: "kustomization:.",
		}
		err := SetKustomizeImage(app, img, wbc)
		require.NoError(t, err)
		require.NotNil(t, app.Spec.Source.Kustomize)
		assert.Len(t, app.Spec.Source.Kustomize.Images, 1)
		assert.Equal(t, v1alpha1.KustomizeImage("jannfis/foobar:1.0.1"), app.Spec.Source.Kustomize.Images[0])
	})

	t.Run("Test set Kustomize image parameters on Kustomize app with no params set", func(t *testing.T) {
		app := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "testns",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeKustomize,
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{
						"jannfis/foobar:1.0.0",
					},
				},
			},
		}
		img := image.NewFromIdentifier("jannfis/foobar:1.0.1")
		err := SetKustomizeImage(app, img, nil)
		require.NoError(t, err)
		require.NotNil(t, app.Spec.Source.Kustomize)
		assert.Len(t, app.Spec.Source.Kustomize.Images, 1)
		assert.Equal(t, v1alpha1.KustomizeImage("jannfis/foobar:1.0.1"), app.Spec.Source.Kustomize.Images[0])
	})

	t.Run("Test set Kustomize image parameters on non-Kustomize app", func(t *testing.T) {
		app := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "testns",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					Kustomize: &v1alpha1.ApplicationSourceKustomize{
						Images: v1alpha1.KustomizeImages{
							"jannfis/foobar:1.0.0",
						},
					},
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeDirectory,
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{
						"jannfis/foobar:1.0.0",
					},
				},
			},
		}
		img := image.NewFromIdentifier("jannfis/foobar:1.0.1")
		err := SetKustomizeImage(app, img, nil)
		require.Error(t, err)
	})

	t.Run("Test set Kustomize image parameters with alias name on Kustomize app with param already set", func(t *testing.T) {
		app := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "testns",
				Annotations: map[string]string{
					fmt.Sprintf(registryCommon.Prefixed(common.ImageUpdaterAnnotationPrefix, registryCommon.KustomizeApplicationNameAnnotationSuffix), "foobar"): "foobar",
				},
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					Kustomize: &v1alpha1.ApplicationSourceKustomize{
						Images: v1alpha1.KustomizeImages{
							"jannfis/foobar:1.0.0",
						},
					},
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeKustomize,
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{
						"jannfis/foobar:1.0.0",
					},
				},
			},
		}
		img := image.NewFromIdentifier("foobar=jannfis/foobar:1.0.1")
		wbc := &WriteBackConfig{
			Target: "kustomization:.",
		}
		err := SetKustomizeImage(app, img, wbc)
		require.NoError(t, err)
		require.NotNil(t, app.Spec.Source.Kustomize)
		assert.Len(t, app.Spec.Source.Kustomize.Images, 1)
		assert.Equal(t, v1alpha1.KustomizeImage("foobar=jannfis/foobar:1.0.1"), app.Spec.Source.Kustomize.Images[0])
	})

}

func Test_SetHelmImage(t *testing.T) {
	t.Run("Test set Helm image parameters on Helm app with existing parameters", func(t *testing.T) {
		app := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "testns",
				Annotations: map[string]string{
					fmt.Sprintf(registryCommon.Prefixed(common.ImageUpdaterAnnotationPrefix, registryCommon.HelmParamImageNameAnnotationSuffix), "foobar"): "image.name",
					fmt.Sprintf(registryCommon.Prefixed(common.ImageUpdaterAnnotationPrefix, registryCommon.HelmParamImageTagAnnotationSuffix), "foobar"):  "image.tag",
				},
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					Helm: &v1alpha1.ApplicationSourceHelm{
						Parameters: []v1alpha1.HelmParameter{
							{
								Name:  "image.tag",
								Value: "1.0.0",
							},
							{
								Name:  "image.name",
								Value: "jannfis/foobar",
							},
						},
					},
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeHelm,
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{
						"jannfis/foobar:1.0.0",
					},
				},
			},
		}

		img := image.NewFromIdentifier("foobar=jannfis/foobar:1.0.1")
		wbc := &WriteBackConfig{
			Target: "helmvalues:.",
		}
		err := SetHelmImage(app, img, wbc)
		require.NoError(t, err)
		require.NotNil(t, app.Spec.Source.Helm)
		assert.Len(t, app.Spec.Source.Helm.Parameters, 2)

		// Find correct parameter
		var tagParam v1alpha1.HelmParameter
		for _, p := range app.Spec.Source.Helm.Parameters {
			if p.Name == "image.tag" {
				tagParam = p
				break
			}
		}
		assert.Equal(t, "1.0.1", tagParam.Value)
	})

	t.Run("Test set Helm image parameters on Helm app without existing parameters", func(t *testing.T) {
		app := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "testns",
				Annotations: map[string]string{
					fmt.Sprintf(registryCommon.Prefixed(common.ImageUpdaterAnnotationPrefix, registryCommon.HelmParamImageNameAnnotationSuffix), "foobar"): "image.name",
					fmt.Sprintf(registryCommon.Prefixed(common.ImageUpdaterAnnotationPrefix, registryCommon.HelmParamImageTagAnnotationSuffix), "foobar"):  "image.tag",
				},
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					Helm: &v1alpha1.ApplicationSourceHelm{},
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeHelm,
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{
						"jannfis/foobar:1.0.0",
					},
				},
			},
		}

		img := image.NewFromIdentifier("foobar=jannfis/foobar:1.0.1")
		wbc := &WriteBackConfig{
			Target: "helmvalues:.",
		}
		err := SetHelmImage(app, img, wbc)
		require.NoError(t, err)
		require.NotNil(t, app.Spec.Source.Helm)
		assert.Len(t, app.Spec.Source.Helm.Parameters, 2)

		// Find correct parameter
		var tagParam v1alpha1.HelmParameter
		for _, p := range app.Spec.Source.Helm.Parameters {
			if p.Name == "image.tag" {
				tagParam = p
				break
			}
		}
		assert.Equal(t, "1.0.1", tagParam.Value)
	})

	t.Run("Test set Helm image parameters on Helm app with different parameters", func(t *testing.T) {
		app := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "testns",
				Annotations: map[string]string{
					fmt.Sprintf(registryCommon.Prefixed(common.ImageUpdaterAnnotationPrefix, registryCommon.HelmParamImageNameAnnotationSuffix), "foobar"): "foobar.image.name",
					fmt.Sprintf(registryCommon.Prefixed(common.ImageUpdaterAnnotationPrefix, registryCommon.HelmParamImageTagAnnotationSuffix), "foobar"):  "foobar.image.tag",
				},
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					Helm: &v1alpha1.ApplicationSourceHelm{
						Parameters: []v1alpha1.HelmParameter{
							{
								Name:  "image.tag",
								Value: "1.0.0",
							},
							{
								Name:  "image.name",
								Value: "jannfis/dummy",
							},
						},
					},
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeHelm,
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{
						"jannfis/foobar:1.0.0",
					},
				},
			},
		}

		img := image.NewFromIdentifier("foobar=jannfis/foobar:1.0.1")
		wbc := &WriteBackConfig{
			Target: "helmvalues:.",
		}
		err := SetHelmImage(app, img, wbc)
		require.NoError(t, err)
		require.NotNil(t, app.Spec.Source.Helm)
		assert.Len(t, app.Spec.Source.Helm.Parameters, 4)

		// Find correct parameter
		var tagParam v1alpha1.HelmParameter
		for _, p := range app.Spec.Source.Helm.Parameters {
			if p.Name == "foobar.image.tag" {
				tagParam = p
				break
			}
		}
		assert.Equal(t, "1.0.1", tagParam.Value)
	})

	t.Run("Test set Helm image parameters on non Helm app", func(t *testing.T) {
		app := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "testns",
				Annotations: map[string]string{
					fmt.Sprintf(registryCommon.Prefixed(common.ImageUpdaterAnnotationPrefix, registryCommon.HelmParamImageNameAnnotationSuffix), "foobar"): "foobar.image.name",
					fmt.Sprintf(registryCommon.Prefixed(common.ImageUpdaterAnnotationPrefix, registryCommon.HelmParamImageTagAnnotationSuffix), "foobar"):  "foobar.image.tag",
				},
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeKustomize,
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{
						"jannfis/foobar:1.0.0",
					},
				},
			},
		}

		img := image.NewFromIdentifier("foobar=jannfis/foobar:1.0.1")
		wbc := &WriteBackConfig{
			Target: "kustomization:.",
		}
		err := SetHelmImage(app, img, wbc)
		require.Error(t, err)
	})

}

func TestKubernetesClient(t *testing.T) {
	app1 := &v1alpha1.Application{
		ObjectMeta: v1.ObjectMeta{Name: "test-app1", Namespace: "testns"},
	}
	app2 := &v1alpha1.Application{
		ObjectMeta: v1.ObjectMeta{Name: "test-app2", Namespace: "testns"},
	}

	// Create the fake client and pre-load it with test applications
	k8sClient, err := newTestK8sClient(app1, app2)
	require.NoError(t, err)

	t.Run("List applications", func(t *testing.T) {
		cr := &api.ImageUpdater{
			Spec: api.ImageUpdaterSpec{
				Namespace: "testns", // Target namespace
				ApplicationRefs: []api.ApplicationRef{
					{NamePattern: "test-app1"},
					{NamePattern: "test-app2"},
					{NamePattern: "non-existent-app"},
				},
			},
		}
		apps, err := k8sClient.ListApplications(context.TODO(), cr)
		require.NoError(t, err)
		require.Len(t, apps, 2)
		assert.ElementsMatch(t, []string{"test-app1", "test-app2"}, []string{apps[0].Name, apps[1].Name})
	})

	t.Run("Get application successful", func(t *testing.T) {
		app, err := k8sClient.GetApplication(context.TODO(), "testns", "test-app1")
		require.NoError(t, err)
		assert.Equal(t, "test-app1", app.GetName())
	})

	t.Run("Get application not found", func(t *testing.T) {
		_, err := k8sClient.GetApplication(context.TODO(), "test-ns-non-existent", "test-app-non-existent")
		require.Error(t, err)
		assert.True(t, errors.IsNotFound(err), "error should be a 'Not Found' error")
	})

	t.Run("List applications when none are found", func(t *testing.T) {
		// Create the fake client without test applications
		k8sClient, err := newTestK8sClient()
		require.NoError(t, err)

		// Define an ImageUpdater CR that targets applications that don't exist
		cr := &api.ImageUpdater{
			ObjectMeta: v1.ObjectMeta{Name: "test-cr"},
			Spec: api.ImageUpdaterSpec{
				Namespace: "testns",
				ApplicationRefs: []api.ApplicationRef{
					{NamePattern: "test-app1"},
					{NamePattern: "non-existent-app"},
				},
			},
		}
		apps, err := k8sClient.ListApplications(context.TODO(), cr)
		require.NoError(t, err)
		require.NotNil(t, apps)
		assert.Len(t, apps, 0)
	})
}

func TestKubernetesClientUpdateSpec(t *testing.T) {
	// TODO: I don't want to remove "conflict" tests now as they can be a useful for inspiration in future.

	//app := &v1alpha1.Application{
	//	ObjectMeta: v1.ObjectMeta{Name: "test-app", Namespace: "testns"},
	//}
	// clientset := fake.NewSimpleClientset(app)

	//t.Run("Successful update after conflict retry", func(t *testing.T) {
	//	attempts := 0
	//	clientset.PrependReactor("update", "*", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
	//		if attempts == 0 {
	//			attempts++
	//			return true, nil, errors.NewConflict(
	//				schema.GroupResource{Group: "argoproj.io", Resource: "Application"}, app.Name, fmt.Errorf("conflict updating %s", app.Name))
	//		} else {
	//			return false, nil, nil
	//		}
	//	})
	//
	//	client, err := NewK8SClient(&kube.ImageUpdaterKubernetesClient{
	//		ApplicationsClientset: clientset,
	//	}, nil)
	//	require.NoError(t, err)
	//
	//	appName := "test-app"
	//	spec, err := client.UpdateSpec(context.TODO(), &application.ApplicationUpdateSpecRequest{
	//		Name: &appName,
	//		Spec: &v1alpha1.ApplicationSpec{Source: &v1alpha1.ApplicationSource{
	//			RepoURL: "https://github.com/argoproj/argocd-example-apps",
	//		}},
	//	})
	//
	//	require.NoError(t, err)
	//	assert.Equal(t, "https://github.com/argoproj/argocd-example-apps", spec.Source.RepoURL)
	//})

	t.Run("Successful update of an application spec", func(t *testing.T) {
		// Initial state of the application
		initialApp := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{Name: "test-app", Namespace: "testns"},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL: "https://github.com/original/repo", // Original value
				},
			},
		}

		// Create a client pre-loaded with this application
		fakeClient, err := newTestK8sClient(initialApp)
		require.NoError(t, err)

		// Define the update request
		appName := "test-app"
		appNamespace := "testns"
		updateRequest := &application.ApplicationUpdateSpecRequest{
			Name:         &appName,
			AppNamespace: &appNamespace,
			Spec: &v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL: "https://github.com/updated/repo", // New value
				},
			},
		}

		// Call the UpdateSpec method
		updatedSpec, err := fakeClient.UpdateSpec(context.TODO(), updateRequest)
		require.NoError(t, err)
		// Assert that the returned spec has the new value
		assert.Equal(t, "https://github.com/updated/repo", updatedSpec.Source.RepoURL)

		// Also, verify the object in the fake cluster was actually updated
		updatedApp, err := fakeClient.GetApplication(context.TODO(), "testns", "test-app")
		require.NoError(t, err)
		assert.Equal(t, "https://github.com/updated/repo", updatedApp.Spec.Source.RepoURL)
	})

	t.Run("UpdateSpec errors - application not found", func(t *testing.T) {
		// Create a client with NO initial applications
		fakeClient, err := newTestK8sClient()
		require.NoError(t, err)

		appName := "non-existent-app"
		appNamespace := "testns"
		updateRequest := &application.ApplicationUpdateSpecRequest{
			Name:         &appName,
			AppNamespace: &appNamespace,
			Spec:         &v1alpha1.ApplicationSpec{},
		}

		_, err = fakeClient.UpdateSpec(context.TODO(), updateRequest)
		require.Error(t, err)
		assert.True(t, errors.IsNotFound(err), "error should be a 'Not Found' error because Get fails")
	})

	//t.Run("UpdateSpec errors - conflict failing retries", func(t *testing.T) {
	//	clientset := fake.NewSimpleClientset(&v1alpha1.Application{
	//		ObjectMeta: v1.ObjectMeta{Name: "test-app", Namespace: "testns"},
	//		Spec:       v1alpha1.ApplicationSpec{},
	//	})
	//
	//	initialApp := &v1alpha1.Application{
	//		ObjectMeta: v1.ObjectMeta{Name: "test-app", Namespace: "testns"},
	//		Spec: v1alpha1.ApplicationSpec{
	//			Source: &v1alpha1.ApplicationSource{
	//				RepoURL: "https://github.com/original/repo", // Original value
	//			},
	//		},
	//	}
	//
	//	// Create a client pre-loaded with this application
	//	fakeClient, err := newTestK8sClient(initialApp)
	//	require.NoError(t, err)
	//
	//	clientset.PrependReactor("update", "applications", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
	//		return true, nil, errors.NewConflict(v1alpha1.Resource("applications"), "test-app", fmt.Errorf("conflict error"))
	//	})
	//
	//	os.Setenv("OVERRIDE_MAX_RETRIES", "0")
	//	defer os.Unsetenv("OVERRIDE_MAX_RETRIES")
	//
	//	appName := "test-app"
	//	spec := &application.ApplicationUpdateSpecRequest{
	//		Name: &appName,
	//		Spec: &v1alpha1.ApplicationSpec{},
	//	}
	//
	//	_, err = fakeClient.UpdateSpec(context.TODO(), spec)
	//	assert.Error(t, err)
	//	assert.Contains(t, err.Error(), "max retries(0) reached while updating application: test-app")
	//})

	//t.Run("UpdateSpec errors - non-conflict update error", func(t *testing.T) {
	//	clientset := fake.NewSimpleClientset(&v1alpha1.Application{
	//		ObjectMeta: v1.ObjectMeta{Name: "test-app", Namespace: "testns"},
	//		Spec:       v1alpha1.ApplicationSpec{},
	//	})
	//
	//	initialApp := &v1alpha1.Application{
	//		ObjectMeta: v1.ObjectMeta{Name: "test-app", Namespace: "testns"},
	//		Spec: v1alpha1.ApplicationSpec{
	//			Source: &v1alpha1.ApplicationSource{
	//				RepoURL: "https://github.com/original/repo", // Original value
	//			},
	//		},
	//	}
	//
	//	// Create a client pre-loaded with this application
	//	fakeClient, err := newTestK8sClient(initialApp)
	//
	//	require.NoError(t, err)
	//
	//	clientset.PrependReactor("update", "applications", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
	//		return true, nil, fmt.Errorf("non-conflict error")
	//	})
	//
	//	appName := "test-app"
	//	appNamespace := "testns"
	//	spec := &application.ApplicationUpdateSpecRequest{
	//		Name:         &appName,
	//		AppNamespace: &appNamespace,
	//		Spec:         &v1alpha1.ApplicationSpec{},
	//	}
	//
	//	_, err = fakeClient.UpdateSpec(context.TODO(), spec)
	//	assert.Error(t, err)
	//	assert.Contains(t, err.Error(), "error updating application: non-conflict error")
	//})
}

func Test_parseImageList(t *testing.T) {
	t.Run("Test basic parsing", func(t *testing.T) {
		assert.Equal(t, []string{"foo", "bar"}, parseImageList(map[string]string{common.ImageUpdaterAnnotation: " foo, bar "}).Originals())
		// should whitespace inside the spec be preserved?
		assert.Equal(t, []string{"foo", "bar", "baz = qux"}, parseImageList(map[string]string{common.ImageUpdaterAnnotation: " foo, bar,baz = qux "}).Originals())
		assert.Equal(t, []string{"foo", "bar", "baz=qux"}, parseImageList(map[string]string{common.ImageUpdaterAnnotation: "foo,bar,baz=qux"}).Originals())
	})
	t.Run("Test kustomize override", func(t *testing.T) {
		imgs := *parseImageList(map[string]string{
			common.ImageUpdaterAnnotation: "foo=bar",
			fmt.Sprintf(registryCommon.Prefixed(common.ImageUpdaterAnnotationPrefix, registryCommon.KustomizeApplicationNameAnnotationSuffix), "foo"): "baz",
		})
		assert.Equal(t, "bar", imgs[0].ImageName)
		assert.Equal(t, "baz", imgs[0].KustomizeImage.ImageName)
	})
}

// Assisted-by: Gemini AI
func Test_parseImageListIuCR(t *testing.T) {
	// newExpectedImageForIuCR is a helper to construct an expected image object.
	newExpectedImageForIuCR := func(identifier string, kustomizeName string) *Image {
		// First, create the neutral image identity. This call correctly
		// sets the `original` field on the returned object.
		imgIdentity := image.NewFromIdentifier(identifier)

		// Then, create the application-specific image, embedding the identity.
		// By assigning the whole identity struct, we ensure the `original`
		// field is preserved in our expected object.
		img := &Image{
			ContainerImage: imgIdentity, // This is the crucial fix
			UpdateStrategy: image.StrategySemVer,
			ForceUpdate:    false,
			AllowTags:      "",
			PullSecret:     "",
			IgnoreTags:     []string{},
			Platforms:      []string{},
		}

		if kustomizeName != "" {
			img.KustomizeImage = image.NewFromIdentifier(kustomizeName)
		}

		return img
	}

	testCases := []struct {
		name           string
		inputImages    []api.ImageConfig
		expectedImages ImageList
	}{
		{
			name: "Basic parsing with alias",
			inputImages: []api.ImageConfig{
				{Alias: "web", ImageName: "nginx:1.21.0"},
				{Alias: "db", ImageName: "postgres:14"},
			},
			expectedImages: ImageList{
				newExpectedImageForIuCR("web=nginx:1.21.0", ""),
				newExpectedImageForIuCR("db=postgres:14", ""),
			},
		},
		{
			name: "Parsing with Kustomize override",
			inputImages: []api.ImageConfig{
				{
					Alias:     "web",
					ImageName: "nginx:1.21.0",
					ManifestTarget: &api.ManifestTarget{
						Kustomize: &api.KustomizeTarget{
							Name: "my-custom-nginx-name",
						},
					},
				},
			},
			expectedImages: ImageList{
				newExpectedImageForIuCR("web=nginx:1.21.0", "my-custom-nginx-name"),
			},
		},
		{
			name: "Mixed list with and without Kustomize override",
			inputImages: []api.ImageConfig{
				{
					Alias:     "web",
					ImageName: "nginx:1.21.0",
					ManifestTarget: &api.ManifestTarget{
						Kustomize: &api.KustomizeTarget{
							Name: "my-custom-nginx-name",
						},
					},
				},
				{Alias: "db", ImageName: "postgres:14"},
			},
			expectedImages: ImageList{
				newExpectedImageForIuCR("web=nginx:1.21.0", "my-custom-nginx-name"),
				newExpectedImageForIuCR("db=postgres:14", ""),
			},
		},
		{
			name:           "Empty input slice",
			inputImages:    []api.ImageConfig{},
			expectedImages: ImageList{},
		},
		{
			name:           "Nil input slice",
			inputImages:    nil,
			expectedImages: ImageList{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseImageListIuCR(context.Background(), tc.inputImages, nil)
			require.NotNil(t, got)
			assert.ElementsMatch(t, tc.expectedImages, *got, "The parsed image list should match the expected list")
		})
	}
}

// Assisted-by: Gemini AI
// Helper functions to create pointers for test data, making the test setup cleaner.
func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }

func Test_newContainerImageFromCommonSettings(t *testing.T) {
	t.Run("should return default settings when parent and settings are nil", func(t *testing.T) {
		// Test case: No parent and no specific settings are provided.
		// Expected: A new image with the hardcoded default values.
		img := newImageFromCommonUpdateSettings(context.Background(), nil, nil)

		assert.NotNil(t, img)
		assert.Equal(t, image.StrategySemVer, img.UpdateStrategy)
		assert.False(t, img.ForceUpdate)
		assert.Equal(t, "", img.AllowTags)
		assert.Equal(t, "", img.PullSecret)
		assert.Equal(t, []string{}, img.IgnoreTags, "IgnoreTags should be an empty, non-nil slice")
	})

	t.Run("should apply settings on top of defaults when no parent is given", func(t *testing.T) {
		// Test case: No parent is provided, but a full set of specific settings are.
		// Expected: The defaults are overridden by the provided settings.
		settings := &api.CommonUpdateSettings{
			UpdateStrategy: strPtr(image.StrategyNewestBuild.String()),
			ForceUpdate:    boolPtr(true),
			AllowTags:      strPtr("v1.*"),
			PullSecret:     strPtr("my-secret"),
			IgnoreTags:     []string{"v1.0.0"},
		}

		img := newImageFromCommonUpdateSettings(context.Background(), settings, nil)

		assert.NotNil(t, img)
		assert.Equal(t, image.StrategyNewestBuild, img.UpdateStrategy)
		assert.True(t, img.ForceUpdate)
		assert.Equal(t, "v1.*", img.AllowTags)
		assert.Equal(t, "my-secret", img.PullSecret)
		assert.Equal(t, []string{"v1.0.0"}, img.IgnoreTags)
	})

	t.Run("should return a clone of the parent when settings are nil", func(t *testing.T) {
		// Test case: A parent is provided, but no new settings.
		// Expected: A new object that is an exact copy of the parent.
		parent := &Image{
			UpdateStrategy: image.StrategyDigest,
			ForceUpdate:    true,
			AllowTags:      "rc-*",
			PullSecret:     "parent-secret",
			IgnoreTags:     []string{"rc-1"},
		}

		img := newImageFromCommonUpdateSettings(context.Background(), nil, parent)

		assert.NotNil(t, img)
		assert.Equal(t, parent, img)
		// Ensure it's a clone, not the same object in memory.
		assert.NotSame(t, parent, img)
	})

	t.Run("should layer settings on top of a parent image", func(t *testing.T) {
		// Test case: A parent and new settings are provided.
		// Expected: The new settings should override the parent's values.
		parent := &Image{
			UpdateStrategy: image.StrategyNewestBuild,
			ForceUpdate:    false,
			AllowTags:      "v1.*",
			PullSecret:     "parent-secret",
			IgnoreTags:     []string{"v1.0.0"},
		}

		settings := &api.CommonUpdateSettings{
			UpdateStrategy: strPtr(image.StrategySemVer.String()), // Override
			ForceUpdate:    boolPtr(true),                         // Override
			// AllowTags is nil, so the parent's value should be kept.
			PullSecret: strPtr("child-secret"), // Override
			IgnoreTags: []string{"v1.1.0"},     // Override
		}

		img := newImageFromCommonUpdateSettings(context.Background(), settings, parent)

		assert.NotNil(t, img)
		assert.Equal(t, image.StrategySemVer, img.UpdateStrategy)
		assert.True(t, img.ForceUpdate)
		assert.Equal(t, "v1.*", img.AllowTags, "AllowTags should be kept from parent")
		assert.Equal(t, "child-secret", img.PullSecret)
		assert.Equal(t, []string{"v1.1.0"}, img.IgnoreTags)
		assert.NotSame(t, parent, img)
	})

	t.Run("should not override parent fields if settings fields are nil", func(t *testing.T) {
		// Test case: A parent is provided, but the new settings struct has nil fields.
		// Expected: Only non-nil fields in the new settings should override the parent.
		parent := &Image{
			UpdateStrategy: image.StrategyNewestBuild,
			ForceUpdate:    true,
			AllowTags:      "v1.*",
			PullSecret:     "parent-secret",
			IgnoreTags:     []string{"v1.0.0"},
		}

		settings := &api.CommonUpdateSettings{
			UpdateStrategy: strPtr(image.StrategyAlphabetical.String()), // Override
			ForceUpdate:    nil,                                         // Should not override
			AllowTags:      nil,                                         // Should not override
			PullSecret:     strPtr("child-secret"),                      // Override
			IgnoreTags:     nil,                                         // Should not override
		}

		img := newImageFromCommonUpdateSettings(context.Background(), settings, parent)

		assert.NotNil(t, img)
		assert.Equal(t, image.StrategyAlphabetical, img.UpdateStrategy)
		assert.True(t, img.ForceUpdate, "ForceUpdate should be kept from parent")
		assert.Equal(t, "v1.*", img.AllowTags, "AllowTags should be kept from parent")
		assert.Equal(t, "child-secret", img.PullSecret)
		assert.Equal(t, []string{"v1.0.0"}, img.IgnoreTags, "IgnoreTags should be kept from parent")
	})

	t.Run("should handle empty but non-nil settings struct", func(t *testing.T) {
		// Test case: An empty (but not nil) settings struct is provided.
		// Expected: Since all fields in the settings struct are nil, it should behave
		// as if no settings were provided, returning a clone of the parent.
		parent := &Image{
			UpdateStrategy: image.StrategyNewestBuild,
			ForceUpdate:    true,
		}
		settings := &api.CommonUpdateSettings{} // Empty struct, all fields are nil

		img := newImageFromCommonUpdateSettings(context.Background(), settings, parent)

		assert.NotNil(t, img)
		// Should be an exact clone of the parent.
		assert.Equal(t, parent, img)
		assert.NotSame(t, parent, img)
	})
}

// Assisted-by: Gemini AI
func Test_nameMatchesPattern(t *testing.T) {
	testCases := []struct {
		name    string
		appName string
		pattern string
		want    bool
	}{
		{
			name:    "Exact match",
			appName: "my-app",
			pattern: "my-app",
			want:    true,
		},
		{
			name:    "No match",
			appName: "other-app",
			pattern: "my-app",
			want:    false,
		},
		{
			name:    "Star wildcard match",
			appName: "my-app-production",
			pattern: "my-app-*",
			want:    true,
		},
		{
			name:    "Star wildcard no match",
			appName: "other-app",
			pattern: "my-app-*",
			want:    false,
		},
		{
			name:    "Question mark wildcard match",
			appName: "app-v1",
			pattern: "app-v?",
			want:    true,
		},
		{
			name:    "Question mark wildcard no match",
			appName: "app-v12",
			pattern: "app-v?",
			want:    false,
		},
		{
			name:    "Character set match",
			appName: "color-red",
			pattern: "color-[rbg]ed",
			want:    true,
		},
		{
			name:    "Character set no match",
			appName: "color-yellow",
			pattern: "color-[rbg]ed",
			want:    false,
		},
		{
			name:    "Character range match",
			appName: "pod-3",
			pattern: "pod-[0-9]",
			want:    true,
		},
		{
			name:    "Character range no match",
			appName: "pod-a",
			pattern: "pod-[0-9]",
			want:    false,
		},
		{
			name:    "Invalid pattern should not match and not panic",
			appName: "any-app",
			pattern: "my-app-[", // This is an invalid glob pattern
			want:    false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := nameMatchesPattern(context.Background(), tc.appName, tc.pattern)
			if got != tc.want {
				t.Errorf("nameMatchesPattern(%q, %q) = %v; want %v", tc.appName, tc.pattern, got, tc.want)
			}
		})
	}
}

// Assisted-by: Gemini AI
func Test_nameMatchesPatterns(t *testing.T) {
	testCases := []struct {
		name     string
		appName  string
		patterns []string
		want     bool
	}{
		{
			name:     "Empty patterns list should return true",
			appName:  "any-app",
			patterns: []string{},
			want:     true,
		},
		{
			name:     "Nil patterns list should return true",
			appName:  "any-app",
			patterns: nil,
			want:     true,
		},
		{
			name:     "Match on first pattern",
			appName:  "app-prod",
			patterns: []string{"app-prod", "app-staging", "app-dev"},
			want:     true,
		},
		{
			name:     "Match on last pattern with wildcard",
			appName:  "app-dev-feature-branch",
			patterns: []string{"app-prod", "app-staging", "app-dev-*"},
			want:     true,
		},
		{
			name:     "No match in list",
			appName:  "infra-service",
			patterns: []string{"app-prod", "app-staging", "app-dev"},
			want:     false,
		},
		{
			name:     "List contains an invalid pattern but a valid one matches first",
			appName:  "app-prod",
			patterns: []string{"app-prod", "app-["},
			want:     true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := nameMatchesPatterns(context.Background(), tc.appName, tc.patterns)
			if got != tc.want {
				t.Errorf("nameMatchesPatterns(%q, %v) = %v; want %v", tc.appName, tc.patterns, got, tc.want)
			}
		})
	}
}

// Assisted-by: Gemini AI
func Test_nameMatchesLabels(t *testing.T) {
	testCases := []struct {
		name      string
		appLabels map[string]string
		selector  *v1.LabelSelector
		want      bool
	}{
		{
			name:      "Nil selector should match",
			appLabels: map[string]string{"env": "prod"},
			selector:  nil,
			want:      true,
		},
		{
			name:      "Empty selector should match",
			appLabels: map[string]string{"env": "prod"},
			selector:  &v1.LabelSelector{},
			want:      true,
		},
		{
			name:      "MatchLabels: simple match",
			appLabels: map[string]string{"env": "prod", "tier": "frontend"},
			selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"env": "prod"},
			},
			want: true,
		},
		{
			name:      "MatchLabels: exact match on multiple labels",
			appLabels: map[string]string{"env": "prod", "tier": "frontend"},
			selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"env": "prod", "tier": "frontend"},
			},
			want: true,
		},
		{
			name:      "MatchLabels: mismatch on value",
			appLabels: map[string]string{"env": "staging"},
			selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"env": "prod"},
			},
			want: false,
		},
		{
			name:      "MatchLabels: mismatch on missing key",
			appLabels: map[string]string{"tier": "frontend"},
			selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"env": "prod"},
			},
			want: false,
		},
		{
			name:      "MatchExpressions: 'In' operator match",
			appLabels: map[string]string{"env": "staging"},
			selector: &v1.LabelSelector{
				MatchExpressions: []v1.LabelSelectorRequirement{
					{Key: "env", Operator: v1.LabelSelectorOpIn, Values: []string{"prod", "staging"}},
				},
			},
			want: true,
		},
		{
			name:      "MatchExpressions: 'In' operator mismatch",
			appLabels: map[string]string{"env": "dev"},
			selector: &v1.LabelSelector{
				MatchExpressions: []v1.LabelSelectorRequirement{
					{Key: "env", Operator: v1.LabelSelectorOpIn, Values: []string{"prod", "staging"}},
				},
			},
			want: false,
		},
		{
			name:      "MatchExpressions: 'NotIn' operator match",
			appLabels: map[string]string{"env": "dev"},
			selector: &v1.LabelSelector{
				MatchExpressions: []v1.LabelSelectorRequirement{
					{Key: "env", Operator: v1.LabelSelectorOpNotIn, Values: []string{"prod", "staging"}},
				},
			},
			want: true,
		},
		{
			name:      "MatchExpressions: 'NotIn' operator mismatch",
			appLabels: map[string]string{"env": "prod"},
			selector: &v1.LabelSelector{
				MatchExpressions: []v1.LabelSelectorRequirement{
					{Key: "env", Operator: v1.LabelSelectorOpNotIn, Values: []string{"prod", "staging"}},
				},
			},
			want: false,
		},
		{
			name:      "MatchExpressions: 'Exists' operator match",
			appLabels: map[string]string{"env": "prod"},
			selector: &v1.LabelSelector{
				MatchExpressions: []v1.LabelSelectorRequirement{
					{Key: "env", Operator: v1.LabelSelectorOpExists},
				},
			},
			want: true,
		},
		{
			name:      "MatchExpressions: 'Exists' operator mismatch",
			appLabels: map[string]string{"tier": "frontend"},
			selector: &v1.LabelSelector{
				MatchExpressions: []v1.LabelSelectorRequirement{
					{Key: "env", Operator: v1.LabelSelectorOpExists},
				},
			},
			want: false,
		},
		{
			name:      "MatchExpressions: 'DoesNotExist' operator match",
			appLabels: map[string]string{"tier": "frontend"},
			selector: &v1.LabelSelector{
				MatchExpressions: []v1.LabelSelectorRequirement{
					{Key: "env", Operator: v1.LabelSelectorOpDoesNotExist},
				},
			},
			want: true,
		},
		{
			name:      "MatchExpressions: 'DoesNotExist' operator mismatch",
			appLabels: map[string]string{"env": "prod"},
			selector: &v1.LabelSelector{
				MatchExpressions: []v1.LabelSelectorRequirement{
					{Key: "env", Operator: v1.LabelSelectorOpDoesNotExist},
				},
			},
			want: false,
		},
		{
			name:      "Combined MatchLabels and MatchExpressions: both match",
			appLabels: map[string]string{"env": "prod", "tier": "frontend"},
			selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"env": "prod"},
				MatchExpressions: []v1.LabelSelectorRequirement{
					{Key: "tier", Operator: v1.LabelSelectorOpIn, Values: []string{"frontend", "backend"}},
				},
			},
			want: true,
		},
		{
			name:      "Combined MatchLabels and MatchExpressions: one mismatch",
			appLabels: map[string]string{"env": "prod", "tier": "database"},
			selector: &v1.LabelSelector{
				MatchLabels: map[string]string{"env": "prod"},
				MatchExpressions: []v1.LabelSelectorRequirement{
					{Key: "tier", Operator: v1.LabelSelectorOpIn, Values: []string{"frontend", "backend"}},
				},
			},
			want: false,
		},
		{
			name:      "Invalid selector should not match",
			appLabels: map[string]string{"env": "prod"},
			selector: &v1.LabelSelector{
				MatchExpressions: []v1.LabelSelectorRequirement{
					{Key: "env", Operator: "InvalidOperator", Values: []string{"prod"}},
				},
			},
			want: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := nameMatchesLabels(tc.appLabels, tc.selector)
			if got != tc.want {
				t.Errorf("nameMatchesLabels() = %v, want %v", got, tc.want)
			}
		})
	}
}

// Assisted-by: Gemini AI
func Test_processApplicationForUpdate(t *testing.T) {
	// Define common applications and references to be used across test cases
	kustomizeApp := &v1alpha1.Application{
		ObjectMeta: v1.ObjectMeta{Name: "kustomize-app", Namespace: "testns"},
		Status:     v1alpha1.ApplicationStatus{SourceType: v1alpha1.ApplicationSourceTypeKustomize},
	}
	helmApp := &v1alpha1.Application{
		ObjectMeta: v1.ObjectMeta{Name: "helm-app", Namespace: "testns"},
		Status:     v1alpha1.ApplicationStatus{SourceType: v1alpha1.ApplicationSourceTypeHelm},
	}
	unsupportedApp := &v1alpha1.Application{
		ObjectMeta: v1.ObjectMeta{Name: "unsupported-app", Namespace: "testns"},
		Status:     v1alpha1.ApplicationStatus{SourceType: v1alpha1.ApplicationSourceTypePlugin},
	}

	appRefWithImages := api.ApplicationRef{
		NamePattern: "some-app",
		Images: []api.ImageConfig{
			{Alias: "web", ImageName: "nginx:1.21.0"},
			{Alias: "db", ImageName: "postgres:14"},
		},
	}

	appRefWithoutImages := api.ApplicationRef{
		NamePattern: "some-app-no-images",
		Images:      nil,
	}

	// Define the test cases
	testCases := []struct {
		name              string
		app               *v1alpha1.Application
		appRef            api.ApplicationRef
		appNSName         string
		initialApps       map[string]ApplicationImages
		expectedAppsCount int
		expectKey         bool
		expectedImagesLen int
	}{
		{
			name:              "Supported Kustomize app should be added",
			app:               kustomizeApp,
			appRef:            appRefWithImages,
			appNSName:         "testns/kustomize-app",
			initialApps:       make(map[string]ApplicationImages),
			expectedAppsCount: 1,
			expectKey:         true,
			expectedImagesLen: 2,
		},
		{
			name:              "Supported Helm app should be added",
			app:               helmApp,
			appRef:            appRefWithImages,
			appNSName:         "testns/helm-app",
			initialApps:       make(map[string]ApplicationImages),
			expectedAppsCount: 1,
			expectKey:         true,
			expectedImagesLen: 2,
		},
		{
			name:              "Unsupported app type should be skipped",
			app:               unsupportedApp,
			appRef:            appRefWithImages,
			appNSName:         "testns/unsupported-app",
			initialApps:       make(map[string]ApplicationImages),
			expectedAppsCount: 0,
			expectKey:         false,
		},
		{
			name:              "Supported app with no images in ref should be added with empty image list",
			app:               kustomizeApp,
			appRef:            appRefWithoutImages,
			appNSName:         "testns/kustomize-app-no-images",
			initialApps:       make(map[string]ApplicationImages),
			expectedAppsCount: 1,
			expectKey:         true,
			expectedImagesLen: 0,
		},
		{
			name:      "Should add to a pre-populated map without affecting existing entries",
			app:       kustomizeApp,
			appRef:    appRefWithImages,
			appNSName: "testns/kustomize-app",
			initialApps: map[string]ApplicationImages{
				"testns/existing-app": {Application: *helmApp},
			},
			expectedAppsCount: 2,
			expectKey:         true,
			expectedImagesLen: 2,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			appsForUpdate := tc.initialApps

			processApplicationForUpdate(ctx, tc.app, tc.appRef, nil, nil, tc.appNSName, appsForUpdate)

			assert.Len(t, appsForUpdate, tc.expectedAppsCount, "The final map should have the expected number of applications")

			if tc.expectKey {
				require.Contains(t, appsForUpdate, tc.appNSName, "The application should be present in the map")
				processedApp := appsForUpdate[tc.appNSName]
				assert.Equal(t, *tc.app, processedApp.Application, "The application data in the map should match the input")
				assert.Len(t, processedApp.Images, tc.expectedImagesLen, "The application should have the correct number of images")

				// Verify one of the images to ensure parsing was correct
				if tc.expectedImagesLen > 0 {
					assert.Equal(t, "nginx", processedApp.Images[0].ImageName)
					assert.Equal(t, "web", processedApp.Images[0].ImageAlias)
				}
			} else {
				assert.NotContains(t, appsForUpdate, tc.appNSName, "The unsupported application should not be in the map")
			}
		})
	}
}

// Assisted-by: Gemini AI
func Test_calculateSpecificity(t *testing.T) {
	testCases := []struct {
		name      string
		appRef    api.ApplicationRef
		wantScore int
	}{
		{
			name:      "Exact name, no labels",
			appRef:    api.ApplicationRef{NamePattern: "app-one"},
			wantScore: 1_000_000 + 7, // 1M for exact match, 7 for "app-one"
		},
		{
			name:      "Simple wildcard, no labels",
			appRef:    api.ApplicationRef{NamePattern: "app-*"},
			wantScore: 4, // 4 for "app-"
		},
		{
			name:      "More specific wildcard, no labels",
			appRef:    api.ApplicationRef{NamePattern: "app-prod-*"},
			wantScore: 9, // 9 for "app-prod-"
		},
		{
			name:      "Question mark wildcard",
			appRef:    api.ApplicationRef{NamePattern: "app-v?"},
			wantScore: 5, // 5 for "app-v"
		},
		{
			name:      "Character set wildcard",
			appRef:    api.ApplicationRef{NamePattern: "app-[abc]"},
			wantScore: 4, // 4 for "app-"
		},
		{
			name:      "Wildcard only",
			appRef:    api.ApplicationRef{NamePattern: "*"},
			wantScore: 0,
		},
		{
			name:      "Exact name with empty label selector",
			appRef:    api.ApplicationRef{NamePattern: "app-one", LabelSelectors: &v1.LabelSelector{}},
			wantScore: 1_000_000 + 7 + 10_000, // +10k for selector presence
		},
		{
			name: "Exact name with one MatchLabel",
			appRef: api.ApplicationRef{
				NamePattern:    "app-one",
				LabelSelectors: &v1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
			},
			wantScore: 1_000_000 + 7 + 10_000 + 100, // +100 for the label
		},
		{
			name: "Wildcard name with one MatchLabel",
			appRef: api.ApplicationRef{
				NamePattern:    "app-*",
				LabelSelectors: &v1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}},
			},
			wantScore: 4 + 10_000 + 100,
		},
		{
			name: "Wildcard name with one MatchExpression",
			appRef: api.ApplicationRef{
				NamePattern: "app-*",
				LabelSelectors: &v1.LabelSelector{
					MatchExpressions: []v1.LabelSelectorRequirement{{Key: "env", Operator: "In", Values: []string{"prod"}}},
				},
			},
			wantScore: 4 + 10_000 + 100, // +100 for the expression
		},
		{
			name: "Wildcard name with complex selector",
			appRef: api.ApplicationRef{
				NamePattern: "app-*",
				LabelSelectors: &v1.LabelSelector{
					MatchLabels:      map[string]string{"env": "prod", "tier": "backend"},
					MatchExpressions: []v1.LabelSelectorRequirement{{Key: "region", Operator: "Exists"}},
				},
			},
			wantScore: 4 + 10_000 + (2 * 100) + (1 * 100), // 10_000 + 200 + 100 + 4 = 10_304
		},
		{
			name:      "Empty pattern is an exact match",
			appRef:    api.ApplicationRef{NamePattern: ""},
			wantScore: 1_000_000,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := calculateSpecificity(tc.appRef)
			assert.Equal(t, tc.wantScore, got)
		})
	}
}

// Assisted-by: Gemini AI
func Test_sortApplicationRefs(t *testing.T) {
	// Define a set of ApplicationRefs with varying specificity for use in test cases.
	// The scores are calculated based on the logic in calculateSpecificity.
	refExactWithLabels := api.ApplicationRef{
		NamePattern: "app-one",
		LabelSelectors: &v1.LabelSelector{
			MatchLabels: map[string]string{"env": "prod"},
		},
	} // Highest score: 1_010_107

	refExact := api.ApplicationRef{
		NamePattern: "app-one",
	} // High score: 1_000_007

	refWildcardWithLabels := api.ApplicationRef{
		NamePattern: "app-*",
		LabelSelectors: &v1.LabelSelector{
			MatchLabels: map[string]string{"env": "prod"},
		},
	} // Medium-high score: 10_104

	refWildcardSpecific := api.ApplicationRef{
		NamePattern: "app-prod-*",
	} // Medium-low score: 9

	refWildcardGeneral := api.ApplicationRef{
		NamePattern: "app-*",
	} // Low score: 4

	refWildcardSingleChar := api.ApplicationRef{
		NamePattern: "app-?",
	} // Low score: 4 (same as app-*)

	refWildcardBroadest := api.ApplicationRef{
		NamePattern: "*",
	} // Lowest score: 0

	testCases := []struct {
		name      string
		inputRefs []api.ApplicationRef
		wantRefs  []api.ApplicationRef
	}{
		{
			name:      "Sorts from most to least specific",
			inputRefs: []api.ApplicationRef{refWildcardGeneral, refExact, refWildcardBroadest, refExactWithLabels, refWildcardSpecific, refWildcardWithLabels},
			wantRefs:  []api.ApplicationRef{refExactWithLabels, refExact, refWildcardWithLabels, refWildcardSpecific, refWildcardGeneral, refWildcardBroadest},
		},
		{
			name:      "Maintains stable order for equal scores (case 1)",
			inputRefs: []api.ApplicationRef{refWildcardGeneral, refWildcardSingleChar}, // app-*, then app-?
			wantRefs:  []api.ApplicationRef{refWildcardGeneral, refWildcardSingleChar}, // Should remain in the same order
		},
		{
			name:      "Maintains stable order for equal scores (case 2)",
			inputRefs: []api.ApplicationRef{refWildcardSingleChar, refWildcardGeneral}, // app-?, then app-*
			wantRefs:  []api.ApplicationRef{refWildcardSingleChar, refWildcardGeneral}, // Should remain in the same order
		},
		{
			name:      "Handles empty slice",
			inputRefs: []api.ApplicationRef{},
			wantRefs:  []api.ApplicationRef{},
		},
		{
			name:      "Handles nil slice",
			inputRefs: nil,
			wantRefs:  []api.ApplicationRef{},
		},
		{
			name:      "Handles slice with one element",
			inputRefs: []api.ApplicationRef{refWildcardGeneral},
			wantRefs:  []api.ApplicationRef{refWildcardGeneral},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := sortApplicationRefs(tc.inputRefs)
			assert.Equal(t, tc.wantRefs, got, "The sorted slice should match the expected order")
		})
	}
}

// Assisted-by: Gemini AI
func Test_FilterApplicationsForUpdate(t *testing.T) {
	// Define common applications to be used across test cases
	appProd := &v1alpha1.Application{
		ObjectMeta: v1.ObjectMeta{Name: "app-prod", Namespace: "testns", Labels: map[string]string{"env": "prod"}},
		Spec: v1alpha1.ApplicationSpec{
			Source: &v1alpha1.ApplicationSource{
				RepoURL:        "https://github.com/example/repo.git",
				TargetRevision: "main",
				Path:           "kustomize",
			},
		},
		Status: v1alpha1.ApplicationStatus{SourceType: v1alpha1.ApplicationSourceTypeKustomize},
	}
	appStaging := &v1alpha1.Application{
		ObjectMeta: v1.ObjectMeta{Name: "app-staging", Namespace: "testns", Labels: map[string]string{"env": "staging"}},
		Spec: v1alpha1.ApplicationSpec{
			Source: &v1alpha1.ApplicationSource{
				RepoURL:        "https://github.com/example/repo.git",
				TargetRevision: "main",
				Path:           "helm",
			},
		},
		Status: v1alpha1.ApplicationStatus{SourceType: v1alpha1.ApplicationSourceTypeHelm},
	}
	otherProd := &v1alpha1.Application{
		ObjectMeta: v1.ObjectMeta{Name: "other-prod", Namespace: "testns", Labels: map[string]string{"env": "prod"}},
		Spec: v1alpha1.ApplicationSpec{
			Source: &v1alpha1.ApplicationSource{
				RepoURL:        "https://github.com/example/repo.git",
				TargetRevision: "main",
				Path:           "kustomize",
			},
		},
		Status: v1alpha1.ApplicationStatus{SourceType: v1alpha1.ApplicationSourceTypeKustomize},
	}
	unsupportedApp := &v1alpha1.Application{
		ObjectMeta: v1.ObjectMeta{Name: "unsupported-app", Namespace: "testns", Labels: map[string]string{"env": "prod"}},
		Spec: v1alpha1.ApplicationSpec{
			Source: &v1alpha1.ApplicationSource{
				RepoURL:        "https://github.com/example/repo.git",
				TargetRevision: "main",
				Path:           "plugin",
			},
		},
		Status: v1alpha1.ApplicationStatus{SourceType: v1alpha1.ApplicationSourceTypePlugin},
	}

	testCases := []struct {
		name            string
		initialApps     []client.Object
		imageUpdaterCR  *api.ImageUpdater
		expectedKeys    []string
		expectedImages  map[string]int // map[appKey]expectedImageCount, for specificity check
		expectNilResult bool
		expectError     bool
	}{
		{
			name:        "Fast path for exact name matches",
			initialApps: []client.Object{appProd, appStaging},
			imageUpdaterCR: &api.ImageUpdater{
				Spec: api.ImageUpdaterSpec{
					Namespace: "testns",
					ApplicationRefs: []api.ApplicationRef{
						{NamePattern: "app-prod", Images: []api.ImageConfig{{Alias: "nginx", ImageName: "nginx:1.0"}}},
					},
				},
			},
			expectedKeys: []string{"testns/app-prod"},
		},
		{
			name:        "Slow path with wildcard name pattern",
			initialApps: []client.Object{appProd, appStaging, otherProd},
			imageUpdaterCR: &api.ImageUpdater{
				Spec: api.ImageUpdaterSpec{
					Namespace: "testns",
					ApplicationRefs: []api.ApplicationRef{
						{NamePattern: "app-*", Images: []api.ImageConfig{{Alias: "nginx", ImageName: "nginx:1.0"}}},
					},
				},
			},
			expectedKeys: []string{"testns/app-prod", "testns/app-staging"},
		},
		{
			name:        "Slow path with label selector",
			initialApps: []client.Object{appProd, appStaging, otherProd},
			imageUpdaterCR: &api.ImageUpdater{
				Spec: api.ImageUpdaterSpec{
					Namespace: "testns",
					ApplicationRefs: []api.ApplicationRef{
						{NamePattern: "*", LabelSelectors: &v1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}}},
					},
				},
			},
			expectedKeys: []string{"testns/app-prod", "testns/other-prod"},
		},
		{
			name:        "Specificity rule is applied correctly",
			initialApps: []client.Object{appProd},
			imageUpdaterCR: &api.ImageUpdater{
				Spec: api.ImageUpdaterSpec{
					Namespace: "testns",
					ApplicationRefs: []api.ApplicationRef{
						// General rule with 1 image
						{NamePattern: "app-*", Images: []api.ImageConfig{{Alias: "nginx", ImageName: "nginx:1.0"}}},
						// Specific rule with 2 images
						{NamePattern: "app-prod", Images: []api.ImageConfig{{Alias: "nginx", ImageName: "nginx:1.0"}, {Alias: "redis", ImageName: "redis:6"}}},
					},
				},
			},
			expectedKeys:   []string{"testns/app-prod"},
			expectedImages: map[string]int{"testns/app-prod": 2}, // Should match the more specific rule
		},
		{
			name:        "Unsupported application type is skipped",
			initialApps: []client.Object{appProd, unsupportedApp},
			imageUpdaterCR: &api.ImageUpdater{
				Spec: api.ImageUpdaterSpec{
					Namespace: "testns",
					ApplicationRefs: []api.ApplicationRef{
						{NamePattern: "*", LabelSelectors: &v1.LabelSelector{MatchLabels: map[string]string{"env": "prod"}}},
					},
				},
			},
			expectedKeys: []string{"testns/app-prod"},
		},
		{
			name:        "No applications in namespace returns nil result",
			initialApps: nil,
			imageUpdaterCR: &api.ImageUpdater{
				Spec: api.ImageUpdaterSpec{
					Namespace: "testns",
					ApplicationRefs: []api.ApplicationRef{
						{NamePattern: "*"},
					},
				},
			},
			expectNilResult: true,
		},
		{
			name:        "No matching applications found returns empty map",
			initialApps: []client.Object{appStaging},
			imageUpdaterCR: &api.ImageUpdater{
				Spec: api.ImageUpdaterSpec{
					Namespace: "testns",
					ApplicationRefs: []api.ApplicationRef{
						{NamePattern: "app-prod"},
					},
				},
			},
			expectedKeys: []string{},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			ctx := context.Background()
			client, err := newTestK8sClient(tc.initialApps...)
			require.NoError(t, err)
			kubeClient := kube.ImageUpdaterKubernetesClient{
				KubeClient: &registryKube.KubernetesClient{
					Clientset: fake.NewFakeKubeClient(),
				},
			}
			// Execute
			appsForUpdate, err := FilterApplicationsForUpdate(ctx, client, &kubeClient, tc.imageUpdaterCR)

			// Assert
			if tc.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			if tc.expectNilResult {
				assert.Nil(t, appsForUpdate)
				return
			}

			require.NotNil(t, appsForUpdate)
			assert.Len(t, appsForUpdate, len(tc.expectedKeys), "The number of applications to update should match")

			for _, key := range tc.expectedKeys {
				assert.Contains(t, appsForUpdate, key)
				if count, ok := tc.expectedImages[key]; ok {
					assert.Len(t, appsForUpdate[key].Images, count, "The number of images for app %s should match the most specific rule", key)
				}
			}
		})
	}
}

func Test_GetParameterPullSecret(t *testing.T) {
	t.Run("Get cred source from a valid pull secret string", func(t *testing.T) {
		img := NewImage(image.NewFromIdentifier("dummy=foo/bar:1.12"))
		img.PullSecret = "pullsecret:foo/bar"
		credSrc := GetParameterPullSecret(context.Background(), img)
		require.NotNil(t, credSrc)
		assert.Equal(t, image.CredentialSourcePullSecret, credSrc.Type)
		assert.Equal(t, "foo", credSrc.SecretNamespace)
		assert.Equal(t, "bar", credSrc.SecretName)
		assert.Equal(t, ".dockerconfigjson", credSrc.SecretField)
	})

	t.Run("Return nil for an invalid pull secret string", func(t *testing.T) {
		img := NewImage(image.NewFromIdentifier("dummy=foo/bar:1.12"))
		img.PullSecret = "pullsecret:invalid"
		credSrc := GetParameterPullSecret(context.Background(), img)
		require.Nil(t, credSrc)
	})

	t.Run("Return nil for an empty pull secret string", func(t *testing.T) {
		img := NewImage(image.NewFromIdentifier("dummy=foo/bar:1.12"))
		// img.PullSecret is "" by default, so no need to set it
		credSrc := GetParameterPullSecret(context.Background(), img)
		require.Nil(t, credSrc)
	})
}

// Helper function to create a new fake client for tests
func newTestK8sClient(initObjs ...client.Object) (*K8sClient, error) {
	// Register the Argo CD Application scheme so the fake client knows about it
	scheme := runtime.NewScheme()
	err := v1alpha1.AddToScheme(scheme)
	if err != nil {
		return nil, fmt.Errorf("failed to add argocd scheme: %w", err)
	}

	// Create a fake client builder and add any initial objects
	builder := ctrlFake.NewClientBuilder().WithScheme(scheme)
	if len(initObjs) > 0 {
		builder.WithObjects(initObjs...)
	}

	// Build the fake client
	fakeClient := builder.Build()

	// Use constructor to create the k8sClient instance
	return &K8sClient{
		fakeClient,
	}, nil
}
