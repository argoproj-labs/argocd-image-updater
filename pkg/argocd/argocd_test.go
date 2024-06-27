package argocd

import (
	"context"
	"fmt"
	"testing"

	"github.com/argoproj-labs/argocd-image-updater/pkg/common"
	"github.com/argoproj-labs/argocd-image-updater/pkg/image"
	"github.com/argoproj-labs/argocd-image-updater/pkg/kube"

	"github.com/argoproj/argo-cd/v2/pkg/apiclient/application"
	"github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"github.com/argoproj/argo-cd/v2/pkg/client/clientset/versioned/fake"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	k8stesting "k8s.io/client-go/testing"
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
		imageList := GetImagesFromApplication(application)
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
		imageList := GetImagesFromApplication(application)
		assert.Empty(t, imageList)
	})

	t.Run("Get list of images from application that has force-update", func(t *testing.T) {
		application := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					fmt.Sprintf(common.ForceUpdateOptionAnnotation, "nginx"): "true",
					common.ImageUpdaterAnnotation:                            "nginx=nginx",
				},
			},
			Spec: v1alpha1.ApplicationSpec{},
			Status: v1alpha1.ApplicationStatus{
				Summary: v1alpha1.ApplicationSummary{},
			},
		}
		imageList := GetImagesFromApplication(application)
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
		imageList := GetImagesAndAliasesFromApplication(application)
		require.Len(t, imageList, 3)
		assert.Equal(t, "nginx", imageList[0].ImageName)
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
		imageList := GetImagesAndAliasesFromApplication(application)
		assert.Empty(t, imageList)
	})

	t.Run("Get list of images and aliases from application annotations", func(t *testing.T) {
		application := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					common.ImageUpdaterAnnotation: "webserver=nginx",
				},
			},
			Spec: v1alpha1.ApplicationSpec{},
			Status: v1alpha1.ApplicationStatus{
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{"nginx:1.12.2"},
				},
			},
		}
		imageList := GetImagesAndAliasesFromApplication(application)
		require.Len(t, imageList, 1)
		assert.Equal(t, "nginx", imageList[0].ImageName)
		assert.Equal(t, "webserver", imageList[0].ImageAlias)
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
		appType := GetApplicationType(application)
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
		appType := GetApplicationType(application)
		assert.Equal(t, ApplicationTypeKustomize, appType)
		assert.Equal(t, "Kustomize", appType.String())
	})

	t.Run("Get application of plugin Type", func(t *testing.T) {
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
		appType := GetApplicationType(application)
		assert.Equal(t, ApplicationTypePlugin, appType)
		assert.Equal(t, "Plugin", appType.String())
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
		appType := GetApplicationType(application)
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
		appType := GetApplicationSourceType(application)
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
		appType := GetApplicationSourceType(application)
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
		appType := GetApplicationType(application)
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
		appType := GetApplicationSourceType(application)
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

func Test_FilterApplicationsForUpdate(t *testing.T) {
	t.Run("Filter for applications without patterns", func(t *testing.T) {
		applicationList := []v1alpha1.Application{
			// Annotated and correct type
			{
				ObjectMeta: v1.ObjectMeta{
					Name:      "app1",
					Namespace: "argocd",
					Annotations: map[string]string{
						common.ImageUpdaterAnnotation: "nginx, quay.io/dexidp/dex:v1.23.0",
					},
				},
				Spec: v1alpha1.ApplicationSpec{},
				Status: v1alpha1.ApplicationStatus{
					SourceType: v1alpha1.ApplicationSourceTypeKustomize,
				},
			},
			// Annotated and correct type
			{
				ObjectMeta: v1.ObjectMeta{
					Name:      "app2",
					Namespace: "argocd",
					Annotations: map[string]string{
						common.ImageUpdaterAnnotation: "nginx, quay.io/dexidp/dex:v1.23.0",
					},
				},
				Spec: v1alpha1.ApplicationSpec{},
				Status: v1alpha1.ApplicationStatus{
					SourceType: v1alpha1.ApplicationSourceTypePlugin,
				},
			},
			// Valid type, but not annotated
			{
				ObjectMeta: v1.ObjectMeta{
					Name:      "app3",
					Namespace: "argocd",
				},
				Spec: v1alpha1.ApplicationSpec{},
				Status: v1alpha1.ApplicationStatus{
					SourceType: v1alpha1.ApplicationSourceTypeHelm,
				},
			},
		}
		filtered, err := FilterApplicationsForUpdate(applicationList, []string{}, "")
		require.NoError(t, err)
		require.Len(t, filtered, 2)
		require.Contains(t, filtered, "app1")
		require.Contains(t, filtered, "app2")
		assert.Len(t, filtered["app1"].Images, 2)
		assert.Len(t, filtered["app2"].Images, 2)
	})

	t.Run("Filter for applications with patterns", func(t *testing.T) {
		applicationList := []v1alpha1.Application{
			// Annotated and correct type
			{
				ObjectMeta: v1.ObjectMeta{
					Name:      "app1",
					Namespace: "argocd",
					Annotations: map[string]string{
						common.ImageUpdaterAnnotation: "nginx, quay.io/dexidp/dex:v1.23.0",
					},
				},
				Spec: v1alpha1.ApplicationSpec{},
				Status: v1alpha1.ApplicationStatus{
					SourceType: v1alpha1.ApplicationSourceTypeKustomize,
				},
			},
			// Annotated, but invalid type
			{
				ObjectMeta: v1.ObjectMeta{
					Name:      "app2",
					Namespace: "argocd",
					Annotations: map[string]string{
						common.ImageUpdaterAnnotation: "nginx, quay.io/dexidp/dex:v1.23.0",
					},
				},
				Spec: v1alpha1.ApplicationSpec{},
				Status: v1alpha1.ApplicationStatus{
					SourceType: v1alpha1.ApplicationSourceTypeKustomize,
				},
			},
			// Valid type, but not annotated
			{
				ObjectMeta: v1.ObjectMeta{
					Name:      "otherapp3",
					Namespace: "argocd",
					Annotations: map[string]string{
						common.ImageUpdaterAnnotation: "nginx, quay.io/dexidp/dex:v1.23.0",
					},
				},
				Spec: v1alpha1.ApplicationSpec{},
				Status: v1alpha1.ApplicationStatus{
					SourceType: v1alpha1.ApplicationSourceTypeHelm,
				},
			},
		}
		filtered, err := FilterApplicationsForUpdate(applicationList, []string{"app*"}, "")
		require.NoError(t, err)
		require.Len(t, filtered, 2)
		require.Contains(t, filtered, "app1")
		require.Contains(t, filtered, "app2")
		assert.Len(t, filtered["app1"].Images, 2)
	})

	t.Run("Filter for applications with label", func(t *testing.T) {
		applicationList := []v1alpha1.Application{
			// Annotated and carries required label
			{
				ObjectMeta: v1.ObjectMeta{
					Name:      "app1",
					Namespace: "argocd",
					Annotations: map[string]string{
						common.ImageUpdaterAnnotation: "nginx, quay.io/dexidp/dex:v1.23.0",
					},
					Labels: map[string]string{
						"custom.label/name": "xyz",
					},
				},
				Spec: v1alpha1.ApplicationSpec{},
				Status: v1alpha1.ApplicationStatus{
					SourceType: v1alpha1.ApplicationSourceTypeKustomize,
				},
			},
			// Annotated but does not carry required label
			{
				ObjectMeta: v1.ObjectMeta{
					Name:      "app2",
					Namespace: "argocd",
					Annotations: map[string]string{
						common.ImageUpdaterAnnotation: "nginx, quay.io/dexidp/dex:v1.23.0",
					},
				},
				Spec: v1alpha1.ApplicationSpec{},
				Status: v1alpha1.ApplicationStatus{
					SourceType: v1alpha1.ApplicationSourceTypeHelm,
				},
			},
		}
		filtered, err := FilterApplicationsForUpdate(applicationList, []string{}, "custom.label/name=xyz")
		require.NoError(t, err)
		require.Len(t, filtered, 1)
		require.Contains(t, filtered, "app1")
		assert.Len(t, filtered["app1"].Images, 2)
	})

}

func Test_GetHelmParamAnnotations(t *testing.T) {
	t.Run("Get parameter names without symbolic names", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.HelmParamImageSpecAnnotation, "myimg"): "image.blub",
			fmt.Sprintf(common.HelmParamImageTagAnnotation, "myimg"):  "image.blab",
		}
		name, tag := getHelmParamNamesFromAnnotation(annotations, &image.ContainerImage{
			ImageAlias: "",
		})
		assert.Equal(t, "image.name", name)
		assert.Equal(t, "image.tag", tag)
	})

	t.Run("Find existing image spec annotation", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.HelmParamImageSpecAnnotation, "myimg"): "image.path",
			fmt.Sprintf(common.HelmParamImageTagAnnotation, "myimg"):  "image.tag",
		}
		name, tag := getHelmParamNamesFromAnnotation(annotations, &image.ContainerImage{
			ImageAlias: "myimg",
		})
		assert.Equal(t, "image.path", name)
		assert.Empty(t, tag)
	})

	t.Run("Find existing image name and image tag annotations", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.HelmParamImageNameAnnotation, "myimg"): "image.name",
			fmt.Sprintf(common.HelmParamImageTagAnnotation, "myimg"):  "image.tag",
		}
		name, tag := getHelmParamNamesFromAnnotation(annotations, &image.ContainerImage{
			ImageAlias: "myimg",
		})
		assert.Equal(t, "image.name", name)
		assert.Equal(t, "image.tag", tag)
	})

	t.Run("Find non-existing image name and image tag annotations", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.HelmParamImageNameAnnotation, "otherimg"): "image.name",
			fmt.Sprintf(common.HelmParamImageTagAnnotation, "otherimg"):  "image.tag",
		}
		name, tag := getHelmParamNamesFromAnnotation(annotations, &image.ContainerImage{
			ImageAlias: "myimg",
		})
		assert.Empty(t, name)
		assert.Empty(t, tag)
	})

	t.Run("Find existing image tag annotations", func(t *testing.T) {
		annotations := map[string]string{
			fmt.Sprintf(common.HelmParamImageTagAnnotation, "myimg"): "image.tag",
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
		err := SetKustomizeImage(app, img)
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
		err := SetKustomizeImage(app, img)
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
		err := SetKustomizeImage(app, img)
		require.Error(t, err)
	})

	t.Run("Test set Kustomize image parameters with alias name on Kustomize app with param already set", func(t *testing.T) {
		app := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "testns",
				Annotations: map[string]string{
					fmt.Sprintf(common.KustomizeApplicationNameAnnotation, "foobar"): "foobar",
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
		err := SetKustomizeImage(app, img)
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
					fmt.Sprintf(common.HelmParamImageNameAnnotation, "foobar"): "image.name",
					fmt.Sprintf(common.HelmParamImageTagAnnotation, "foobar"):  "image.tag",
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

		err := SetHelmImage(app, img)
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
					fmt.Sprintf(common.HelmParamImageNameAnnotation, "foobar"): "image.name",
					fmt.Sprintf(common.HelmParamImageTagAnnotation, "foobar"):  "image.tag",
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

		err := SetHelmImage(app, img)
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
					fmt.Sprintf(common.HelmParamImageNameAnnotation, "foobar"): "foobar.image.name",
					fmt.Sprintf(common.HelmParamImageTagAnnotation, "foobar"):  "foobar.image.tag",
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

		err := SetHelmImage(app, img)
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
					fmt.Sprintf(common.HelmParamImageNameAnnotation, "foobar"): "foobar.image.name",
					fmt.Sprintf(common.HelmParamImageTagAnnotation, "foobar"):  "foobar.image.tag",
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

		err := SetHelmImage(app, img)
		require.Error(t, err)
	})

}

func Test_SetPluginImage(t *testing.T) {
	t.Run("Test set Plugin image parameters on Plugin app with existing HELM_ARGS", func(t *testing.T) {
		app := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "testns",
				Annotations: map[string]string{
					fmt.Sprintf(common.HelmParamImageNameAnnotation, "foobar"): "image.name",
					fmt.Sprintf(common.HelmParamImageTagAnnotation, "foobar"):  "image.tag",
				},
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					Plugin: &v1alpha1.ApplicationSourcePlugin{
						Env: []*v1alpha1.EnvEntry{
							{
								Name:  "HELM_ARGS",
								Value: "--set image.tag=1.0.0 --set image.name=jannfis/foobar",
							},
						},
					},
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypePlugin,
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{
						"jannfis/foobar:1.0.0",
					},
				},
			},
		}

		img := image.NewFromIdentifier("foobar=jannfis/foobar:1.0.1")

		err := SetPluginImage(app, img)
		require.NoError(t, err)
		require.NotNil(t, app.Spec.Source.Plugin)
		assert.Len(t, app.Spec.Source.Plugin.Env, 1)

		// Find correct HELM_ARGS
		var helmArgs string
		for _, env := range app.Spec.Source.Plugin.Env {
			if env.Name == "HELM_ARGS" {
				helmArgs = env.Value
				break
			}
		}
		assert.Contains(t, helmArgs, "--set image.tag=1.0.1")
	})

	t.Run("Test set Plugin image parameters on Plugin app without existing HELM_ARGS", func(t *testing.T) {
		app := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "testns",
				Annotations: map[string]string{
					fmt.Sprintf(common.HelmParamImageNameAnnotation, "foobar"): "image.name",
					fmt.Sprintf(common.HelmParamImageTagAnnotation, "foobar"):  "image.tag",
				},
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					Plugin: &v1alpha1.ApplicationSourcePlugin{},
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypePlugin,
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{
						"jannfis/foobar:1.0.0",
					},
				},
			},
		}

		img := image.NewFromIdentifier("foobar=jannfis/foobar:1.0.1")

		err := SetPluginImage(app, img)
		require.NoError(t, err)
		require.NotNil(t, app.Spec.Source.Plugin)
		assert.Len(t, app.Spec.Source.Plugin.Env, 1)

		// Find correct HELM_ARGS
		var helmArgs string
		for _, env := range app.Spec.Source.Plugin.Env {
			if env.Name == "HELM_ARGS" {
				helmArgs = env.Value
				break
			}
		}
		assert.Contains(t, helmArgs, "--set image.tag=1.0.1")
	})

	t.Run("Test set Plugin image parameters on Plugin app with different parameters in HELM_ARGS", func(t *testing.T) {
		app := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "testns",
				Annotations: map[string]string{
					fmt.Sprintf(common.HelmParamImageNameAnnotation, "foobar"): "foobar.image.name",
					fmt.Sprintf(common.HelmParamImageTagAnnotation, "foobar"):  "foobar.image.tag",
				},
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					Plugin: &v1alpha1.ApplicationSourcePlugin{
						Env: []*v1alpha1.EnvEntry{
							{
								Name:  "HELM_ARGS",
								Value: "--set image.tag=1.0.0 --set image.name=jannfis/dummy",
							},
						},
					},
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypePlugin,
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{
						"jannfis/foobar:1.0.0",
					},
				},
			},
		}

		img := image.NewFromIdentifier("foobar=jannfis/foobar:1.0.1")

		err := SetPluginImage(app, img)
		require.NoError(t, err)
		require.NotNil(t, app.Spec.Source.Plugin)
		assert.Len(t, app.Spec.Source.Plugin.Env, 1)

		// Find correct HELM_ARGS
		var helmArgs string
		for _, env := range app.Spec.Source.Plugin.Env {
			if env.Name == "HELM_ARGS" {
				helmArgs = env.Value
				break
			}
		}
		assert.Contains(t, helmArgs, "--set foobar.image.tag=1.0.1")
	})

	t.Run("Test set Plugin image parameters on non Plugin app", func(t *testing.T) {
		app := &v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "testns",
				Annotations: map[string]string{
					fmt.Sprintf(common.HelmParamImageNameAnnotation, "foobar"): "foobar.image.name",
					fmt.Sprintf(common.HelmParamImageTagAnnotation, "foobar"):  "foobar.image.tag",
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

		err := SetPluginImage(app, img)
		require.Error(t, err)
	})
}

func Test_parseHelmArgs(t *testing.T) {
	t.Run("Test parse Helm arguments with mixed parameters", func(t *testing.T) {
		helmArgs := "--set image.tag=1.0.0 --set image.name=jannfis/foobar -f values.yaml"
		params, otherArgs := parseHelmArgs(helmArgs)

		assert.Len(t, params, 2)
		assert.Equal(t, "1.0.0", params["image.tag"])
		assert.Equal(t, "jannfis/foobar", params["image.name"])
		assert.Len(t, otherArgs, 2)
		assert.Contains(t, otherArgs, "-f")
		assert.Contains(t, otherArgs, "values.yaml")
	})

	t.Run("Test parse Helm arguments with only --set parameters", func(t *testing.T) {
		helmArgs := "--set image.tag=1.0.0 --set image.name=jannfis/foobar"
		params, otherArgs := parseHelmArgs(helmArgs)

		assert.Len(t, params, 2)
		assert.Equal(t, "1.0.0", params["image.tag"])
		assert.Equal(t, "jannfis/foobar", params["image.name"])
		assert.Len(t, otherArgs, 0)
	})

	t.Run("Test parse Helm arguments with only other parameters", func(t *testing.T) {
		helmArgs := "-f values.yaml --debug"
		params, otherArgs := parseHelmArgs(helmArgs)

		assert.Len(t, params, 0)
		assert.Len(t, otherArgs, 3)
		assert.Contains(t, otherArgs, "-f")
		assert.Contains(t, otherArgs, "values.yaml")
		assert.Contains(t, otherArgs, "--debug")
	})

	t.Run("Test parse empty Helm arguments", func(t *testing.T) {
		helmArgs := ""
		params, otherArgs := parseHelmArgs(helmArgs)

		assert.Len(t, params, 0)
		assert.Len(t, otherArgs, 0)
	})
}

func Test_buildHelmArgs(t *testing.T) {
	t.Run("Test build Helm arguments with mixed parameters", func(t *testing.T) {
		params := map[string]string{
			"image.tag":  "1.0.0",
			"image.name": "jannfis/foobar",
		}
		otherArgs := []string{"-f", "values.yaml"}

		helmArgs := buildHelmArgs(params, otherArgs)

		assert.Contains(t, helmArgs, "--set image.tag=1.0.0")
		assert.Contains(t, helmArgs, "--set image.name=jannfis/foobar")
		assert.Contains(t, helmArgs, "-f values.yaml")
	})

	t.Run("Test build Helm arguments with only --set parameters", func(t *testing.T) {
		params := map[string]string{
			"image.tag":  "1.0.0",
			"image.name": "jannfis/foobar",
		}
		otherArgs := []string{}

		helmArgs := buildHelmArgs(params, otherArgs)

		assert.Contains(t, helmArgs, "--set image.tag=1.0.0")
		assert.Contains(t, helmArgs, "--set image.name=jannfis/foobar")
	})

	t.Run("Test build Helm arguments with only other parameters", func(t *testing.T) {
		params := map[string]string{}
		otherArgs := []string{"-f", "values.yaml", "--debug"}

		helmArgs := buildHelmArgs(params, otherArgs)

		assert.Contains(t, helmArgs, "-f values.yaml")
		assert.Contains(t, helmArgs, "--debug")
	})

	t.Run("Test build empty Helm arguments", func(t *testing.T) {
		params := map[string]string{}
		otherArgs := []string{}

		helmArgs := buildHelmArgs(params, otherArgs)

		assert.Equal(t, "", helmArgs)
	})
}

func TestKubernetesClient(t *testing.T) {
	app1 := &v1alpha1.Application{
		ObjectMeta: v1.ObjectMeta{Name: "test-app1", Namespace: "testns1"},
	}
	app2 := &v1alpha1.Application{
		ObjectMeta: v1.ObjectMeta{Name: "test-app2", Namespace: "testns2"},
	}

	client, err := NewK8SClient(&kube.KubernetesClient{
		Namespace:             "testns1",
		ApplicationsClientset: fake.NewSimpleClientset(app1, app2),
	})

	require.NoError(t, err)

	t.Run("List applications", func(t *testing.T) {
		apps, err := client.ListApplications()
		require.NoError(t, err)
		require.Len(t, apps, 1)

		assert.ElementsMatch(t, []string{"test-app1"}, []string{app1.Name})
	})

	t.Run("Get application successful", func(t *testing.T) {
		app, err := client.GetApplication(context.TODO(), "test-app1")
		require.NoError(t, err)
		assert.Equal(t, "test-app1", app.GetName())
	})

	t.Run("Get application not found", func(t *testing.T) {
		_, err := client.GetApplication(context.TODO(), "test-app2")
		require.Error(t, err)
		assert.True(t, errors.IsNotFound(err))
	})
}

func TestKubernetesClient_UpdateSpec_Conflict(t *testing.T) {
	app := &v1alpha1.Application{
		ObjectMeta: v1.ObjectMeta{Name: "test-app", Namespace: "testns"},
	}
	clientset := fake.NewSimpleClientset(app)

	attempts := 0
	clientset.PrependReactor("update", "*", func(action k8stesting.Action) (handled bool, ret runtime.Object, err error) {
		if attempts == 0 {
			attempts++
			return true, nil, errors.NewConflict(
				schema.GroupResource{Group: "argoproj.io", Resource: "Application"}, app.Name, fmt.Errorf("conflict updating %s", app.Name))
		} else {
			return false, nil, nil
		}
	})

	client, err := NewK8SClient(&kube.KubernetesClient{
		Namespace:             "testns",
		ApplicationsClientset: clientset,
	})
	require.NoError(t, err)

	appName := "test-app"

	spec, err := client.UpdateSpec(context.TODO(), &application.ApplicationUpdateSpecRequest{
		Name: &appName,
		Spec: &v1alpha1.ApplicationSpec{Source: &v1alpha1.ApplicationSource{
			RepoURL: "https://github.com/argoproj/argocd-example-apps",
		}},
	})

	require.NoError(t, err)

	assert.Equal(t, "https://github.com/argoproj/argocd-example-apps", spec.Source.RepoURL)
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
			common.ImageUpdaterAnnotation:                                 "foo=bar",
			fmt.Sprintf(common.KustomizeApplicationNameAnnotation, "foo"): "baz",
		})
		assert.Equal(t, "bar", imgs[0].ImageName)
		assert.Equal(t, "baz", imgs[0].KustomizeImage.ImageName)
	})
}
