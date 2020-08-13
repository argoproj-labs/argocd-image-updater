package argocd

import (
	"errors"
	"fmt"
	"testing"

	argomock "github.com/argoproj-labs/argocd-image-updater/pkg/argocd/mocks"
	"github.com/argoproj-labs/argocd-image-updater/pkg/client"
	"github.com/argoproj-labs/argocd-image-updater/pkg/common"
	"github.com/argoproj-labs/argocd-image-updater/pkg/image"
	"github.com/argoproj-labs/argocd-image-updater/pkg/registry"
	regmock "github.com/argoproj-labs/argocd-image-updater/pkg/registry/mocks"
	"github.com/argoproj-labs/argocd-image-updater/test/fake"
	"github.com/argoproj-labs/argocd-image-updater/test/fixture"

	"github.com/argoproj/argo-cd/pkg/apis/application/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_UpdateApplication(t *testing.T) {
	t.Run("Test successful update", func(t *testing.T) {
		mockClientFn := func(endpoint *registry.RegistryEndpoint, username, password string) (registry.RegistryClient, error) {
			regMock := regmock.RegistryClient{}
			regMock.On("Tags", mock.Anything).Return([]string{"1.0.1"}, nil)
			return &regMock, nil
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)

		kubeClient := client.KubernetesClient{
			Clientset: fake.NewFakeKubeClient(),
		}
		appImages := &ApplicationImages{
			Application: v1alpha1.Application{
				ObjectMeta: v1.ObjectMeta{
					Name:      "guestbook",
					Namespace: "guestbook",
				},
				Spec: v1alpha1.ApplicationSpec{
					Source: v1alpha1.ApplicationSource{
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
			},
			Images: image.ContainerImageList{
				image.NewFromIdentifier("jannfis/foobar:1.0.1"),
			},
		}
		res := UpdateApplication(mockClientFn, &argoClient, &kubeClient, appImages, false)
		assert.Equal(t, 0, res.NumErrors)
		assert.Equal(t, 0, res.NumSkipped)
		assert.Equal(t, 1, res.NumApplicationsProcessed)
		assert.Equal(t, 1, res.NumImagesConsidered)
		assert.Equal(t, 1, res.NumImagesUpdated)
	})

	t.Run("Test successful update with credentials", func(t *testing.T) {
		mockClientFn := func(endpoint *registry.RegistryEndpoint, username, password string) (registry.RegistryClient, error) {
			regMock := regmock.RegistryClient{}
			assert.Equal(t, "myuser", username)
			assert.Equal(t, "mypass", password)
			regMock.On("Tags", mock.Anything).Return([]string{"1.0.1"}, nil)
			return &regMock, nil
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)

		kubeClient := client.KubernetesClient{
			Clientset: fake.NewFakeClientsetWithResources(fixture.NewSecret("foo", "bar", map[string][]byte{"creds": []byte("myuser:mypass")})),
		}
		appImages := &ApplicationImages{
			Application: v1alpha1.Application{
				ObjectMeta: v1.ObjectMeta{
					Name:      "guestbook",
					Namespace: "guestbook",
					Annotations: map[string]string{
						fmt.Sprintf(common.SecretListAnnotation, "dummy"): "secret:foo/bar#creds",
					},
				},
				Spec: v1alpha1.ApplicationSpec{
					Source: v1alpha1.ApplicationSource{
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
			},
			Images: image.ContainerImageList{
				image.NewFromIdentifier("dummy=jannfis/foobar:1.0.1"),
			},
		}
		res := UpdateApplication(mockClientFn, &argoClient, &kubeClient, appImages, false)
		assert.Equal(t, 0, res.NumErrors)
		assert.Equal(t, 0, res.NumSkipped)
		assert.Equal(t, 1, res.NumApplicationsProcessed)
		assert.Equal(t, 1, res.NumImagesConsidered)
		assert.Equal(t, 1, res.NumImagesUpdated)
	})

	t.Run("Test skip because of image not in list", func(t *testing.T) {
		mockClientFn := func(endpoint *registry.RegistryEndpoint, username, password string) (registry.RegistryClient, error) {
			regMock := regmock.RegistryClient{}
			regMock.On("Tags", mock.Anything).Return([]string{"1.0.1"}, nil)
			return &regMock, nil
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)

		kubeClient := client.KubernetesClient{
			Clientset: fake.NewFakeKubeClient(),
		}
		appImages := &ApplicationImages{
			Application: v1alpha1.Application{
				ObjectMeta: v1.ObjectMeta{
					Name:      "guestbook",
					Namespace: "guestbook",
				},
				Spec: v1alpha1.ApplicationSpec{
					Source: v1alpha1.ApplicationSource{
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
			},
			Images: image.ContainerImageList{
				image.NewFromIdentifier("jannfis/barbar:1.0.1"),
			},
		}
		res := UpdateApplication(mockClientFn, &argoClient, &kubeClient, appImages, false)
		assert.Equal(t, 0, res.NumErrors)
		assert.Equal(t, 1, res.NumSkipped)
		assert.Equal(t, 1, res.NumApplicationsProcessed)
		assert.Equal(t, 0, res.NumImagesConsidered)
		assert.Equal(t, 0, res.NumImagesUpdated)
	})

	t.Run("Test skip because of image up-to-date", func(t *testing.T) {
		mockClientFn := func(endpoint *registry.RegistryEndpoint, username, password string) (registry.RegistryClient, error) {
			regMock := regmock.RegistryClient{}
			regMock.On("Tags", mock.Anything).Return([]string{"1.0.1"}, nil)
			return &regMock, nil
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)

		kubeClient := client.KubernetesClient{
			Clientset: fake.NewFakeKubeClient(),
		}
		appImages := &ApplicationImages{
			Application: v1alpha1.Application{
				ObjectMeta: v1.ObjectMeta{
					Name:      "guestbook",
					Namespace: "guestbook",
				},
				Spec: v1alpha1.ApplicationSpec{
					Source: v1alpha1.ApplicationSource{
						Kustomize: &v1alpha1.ApplicationSourceKustomize{
							Images: v1alpha1.KustomizeImages{
								"jannfis/foobar:1.0.1",
							},
						},
					},
				},
				Status: v1alpha1.ApplicationStatus{
					SourceType: v1alpha1.ApplicationSourceTypeKustomize,
					Summary: v1alpha1.ApplicationSummary{
						Images: []string{
							"jannfis/foobar:1.0.1",
						},
					},
				},
			},
			Images: image.ContainerImageList{
				image.NewFromIdentifier("jannfis/foobar:1.0.1"),
			},
		}
		res := UpdateApplication(mockClientFn, &argoClient, &kubeClient, appImages, false)
		assert.Equal(t, 0, res.NumErrors)
		assert.Equal(t, 0, res.NumSkipped)
		assert.Equal(t, 1, res.NumApplicationsProcessed)
		assert.Equal(t, 1, res.NumImagesConsidered)
		assert.Equal(t, 0, res.NumImagesUpdated)
	})

	t.Run("Error - unknown registry", func(t *testing.T) {
		mockClientFn := func(endpoint *registry.RegistryEndpoint, username, password string) (registry.RegistryClient, error) {
			regMock := regmock.RegistryClient{}
			regMock.On("Tags", mock.Anything).Return([]string{"1.0.1"}, nil)
			return &regMock, nil
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)

		kubeClient := client.KubernetesClient{
			Clientset: fake.NewFakeKubeClient(),
		}
		appImages := &ApplicationImages{
			Application: v1alpha1.Application{
				ObjectMeta: v1.ObjectMeta{
					Name:      "guestbook",
					Namespace: "guestbook",
				},
				Spec: v1alpha1.ApplicationSpec{
					Source: v1alpha1.ApplicationSource{
						Kustomize: &v1alpha1.ApplicationSourceKustomize{
							Images: v1alpha1.KustomizeImages{
								"example.io/jannfis/foobar:1.0.1",
							},
						},
					},
				},
				Status: v1alpha1.ApplicationStatus{
					SourceType: v1alpha1.ApplicationSourceTypeKustomize,
					Summary: v1alpha1.ApplicationSummary{
						Images: []string{
							"example.io/jannfis/foobar:1.0.1",
						},
					},
				},
			},
			Images: image.ContainerImageList{
				image.NewFromIdentifier("example.io/jannfis/foobar:1.0.1"),
			},
		}
		res := UpdateApplication(mockClientFn, &argoClient, &kubeClient, appImages, false)
		assert.Equal(t, 1, res.NumErrors)
		assert.Equal(t, 0, res.NumSkipped)
		assert.Equal(t, 1, res.NumApplicationsProcessed)
		assert.Equal(t, 1, res.NumImagesConsidered)
		assert.Equal(t, 0, res.NumImagesUpdated)
	})

	t.Run("Test error on generic registry client failure", func(t *testing.T) {
		mockClientFn := func(endpoint *registry.RegistryEndpoint, username, password string) (registry.RegistryClient, error) {
			return nil, errors.New("some error")
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)

		kubeClient := client.KubernetesClient{
			Clientset: fake.NewFakeKubeClient(),
		}
		appImages := &ApplicationImages{
			Application: v1alpha1.Application{
				ObjectMeta: v1.ObjectMeta{
					Name:      "guestbook",
					Namespace: "guestbook",
				},
				Spec: v1alpha1.ApplicationSpec{
					Source: v1alpha1.ApplicationSource{
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
			},
			Images: image.ContainerImageList{
				image.NewFromIdentifier("jannfis/foobar:1.0.1"),
			},
		}
		res := UpdateApplication(mockClientFn, &argoClient, &kubeClient, appImages, false)
		assert.Equal(t, 1, res.NumErrors)
		assert.Equal(t, 0, res.NumSkipped)
		assert.Equal(t, 1, res.NumApplicationsProcessed)
		assert.Equal(t, 1, res.NumImagesConsidered)
		assert.Equal(t, 0, res.NumImagesUpdated)
	})

	t.Run("Test error on failure to list tags", func(t *testing.T) {
		mockClientFn := func(endpoint *registry.RegistryEndpoint, username, password string) (registry.RegistryClient, error) {
			regMock := regmock.RegistryClient{}
			regMock.On("Tags", mock.Anything).Return(nil, errors.New("some error"))
			return &regMock, nil
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)

		kubeClient := client.KubernetesClient{
			Clientset: fake.NewFakeKubeClient(),
		}
		appImages := &ApplicationImages{
			Application: v1alpha1.Application{
				ObjectMeta: v1.ObjectMeta{
					Name:      "guestbook",
					Namespace: "guestbook",
				},
				Spec: v1alpha1.ApplicationSpec{
					Source: v1alpha1.ApplicationSource{
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
			},
			Images: image.ContainerImageList{
				image.NewFromIdentifier("jannfis/foobar:1.0.1"),
			},
		}
		res := UpdateApplication(mockClientFn, &argoClient, &kubeClient, appImages, false)
		assert.Equal(t, 1, res.NumErrors)
		assert.Equal(t, 0, res.NumSkipped)
		assert.Equal(t, 1, res.NumApplicationsProcessed)
		assert.Equal(t, 1, res.NumImagesConsidered)
		assert.Equal(t, 0, res.NumImagesUpdated)
	})

	t.Run("Test error on improper semver in tag", func(t *testing.T) {
		mockClientFn := func(endpoint *registry.RegistryEndpoint, username, password string) (registry.RegistryClient, error) {
			regMock := regmock.RegistryClient{}
			regMock.On("Tags", mock.Anything).Return([]string{"1.0.0", "1.0.1"}, nil)
			return &regMock, nil
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)

		kubeClient := client.KubernetesClient{
			Clientset: fake.NewFakeKubeClient(),
		}
		appImages := &ApplicationImages{
			Application: v1alpha1.Application{
				ObjectMeta: v1.ObjectMeta{
					Name:      "guestbook",
					Namespace: "guestbook",
				},
				Spec: v1alpha1.ApplicationSpec{
					Source: v1alpha1.ApplicationSource{
						Kustomize: &v1alpha1.ApplicationSourceKustomize{
							Images: v1alpha1.KustomizeImages{
								"jannfis/foobar:stable",
							},
						},
					},
				},
				Status: v1alpha1.ApplicationStatus{
					SourceType: v1alpha1.ApplicationSourceTypeKustomize,
					Summary: v1alpha1.ApplicationSummary{
						Images: []string{
							"jannfis/foobar:stable",
						},
					},
				},
			},
			Images: image.ContainerImageList{
				image.NewFromIdentifier("jannfis/foobar:stable"),
			},
		}
		res := UpdateApplication(mockClientFn, &argoClient, &kubeClient, appImages, false)
		assert.Equal(t, 1, res.NumErrors)
		assert.Equal(t, 0, res.NumSkipped)
		assert.Equal(t, 1, res.NumApplicationsProcessed)
		assert.Equal(t, 1, res.NumImagesConsidered)
		assert.Equal(t, 0, res.NumImagesUpdated)
	})

}
