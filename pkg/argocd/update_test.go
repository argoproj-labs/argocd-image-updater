package argocd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	yaml "sigs.k8s.io/yaml/goyaml.v3"

	iuapi "github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"
	"github.com/argoproj-labs/argocd-image-updater/ext/git"
	gitmock "github.com/argoproj-labs/argocd-image-updater/ext/git/mocks"
	argomock "github.com/argoproj-labs/argocd-image-updater/pkg/argocd/mocks"
	"github.com/argoproj-labs/argocd-image-updater/pkg/kube"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/image"
	registryKube "github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/kube"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/registry"
	regmock "github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/registry/mocks"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/tag"
	"github.com/argoproj-labs/argocd-image-updater/test/fake"
	"github.com/argoproj-labs/argocd-image-updater/test/fixture"

	"github.com/argoproj/argo-cd/v3/pkg/apiclient/application"
	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/distribution/distribution/v3/manifest/schema1" //nolint:staticcheck
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_UpdateApplication(t *testing.T) {
	t.Run("Test kustomize w/ multiple images w/ different registry w/ different tags", func(t *testing.T) {
		mockClientFn := func(endpoint *registry.RegistryEndpoint, username, password string) (registry.RegistryClient, error) {
			regMock := regmock.RegistryClient{}
			regMock.On("NewRepository", mock.Anything).Return(nil)
			regMock.On("Tags", mock.Anything).Return([]string{"1.0.2", "1.0.3"}, nil)
			return &regMock, nil
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)

		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeKubeClient(),
			},
		}

		imageList := ImageList{
			NewImage(image.NewFromIdentifier("foobar=gcr.io/jannfis/foobar:>=1.0.1")),
			NewImage(image.NewFromIdentifier("foobar=gcr.io/jannfis/barbar:>=1.0.1")),
		}

		appImages := &ApplicationImages{
			Application: v1alpha1.Application{
				ObjectMeta: v1.ObjectMeta{
					Name:      "guestbook",
					Namespace: "guestbook",
				},
				Spec: v1alpha1.ApplicationSpec{
					Source: &v1alpha1.ApplicationSource{
						Kustomize: &v1alpha1.ApplicationSourceKustomize{
							Images: v1alpha1.KustomizeImages{
								"jannfis/foobar:1.0.1",
								"jannfis/barbar:1.0.1",
							},
						},
					},
				},
				Status: v1alpha1.ApplicationStatus{
					SourceType: v1alpha1.ApplicationSourceTypeKustomize,
					Summary: v1alpha1.ApplicationSummary{
						Images: []string{
							"gcr.io/jannfis/foobar:1.0.1",
							"gcr.io/jannfis/barbar:1.0.1",
						},
					},
				},
			},
			WriteBackConfig: &WriteBackConfig{
				Method: WriteBackApplication,
			},
			Images: imageList,
		}
		res := UpdateApplication(context.Background(), &UpdateConfiguration{
			NewRegFN:   mockClientFn,
			ArgoClient: &argoClient,
			KubeClient: &kubeClient,
			UpdateApp:  appImages,
			DryRun:     false,
		}, NewSyncIterationState())
		assert.Equal(t, v1alpha1.KustomizeImage("gcr.io/jannfis/foobar:1.0.3"), appImages.Application.Spec.Source.Kustomize.Images[0])
		assert.Equal(t, v1alpha1.KustomizeImage("gcr.io/jannfis/barbar:1.0.3"), appImages.Application.Spec.Source.Kustomize.Images[1])
		assert.Equal(t, 0, res.NumErrors)
		assert.Equal(t, 0, res.NumSkipped)
		assert.Equal(t, 1, res.NumApplicationsProcessed)
		assert.Equal(t, 2, res.NumImagesConsidered)
		assert.Equal(t, 2, res.NumImagesUpdated)
	})

	t.Run("Update app w/ GitHub App creds", func(t *testing.T) {
		mockClientFn := func(endpoint *registry.RegistryEndpoint, username, password string) (registry.RegistryClient, error) {
			regMock := regmock.RegistryClient{}
			regMock.On("NewRepository", mock.Anything).Return(nil)
			regMock.On("Tags", mock.Anything).Return([]string{"1.0.2", "1.0.3"}, nil)
			return &regMock, nil
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)

		secret := fixture.NewSecret("argocd-image-updater", "git-creds", map[string][]byte{
			"githubAppID":             []byte("12345678"),
			"githubAppInstallationID": []byte("87654321"),
			"githubAppPrivateKey":     []byte("foo"),
		})
		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeClientsetWithResources(secret),
			},
		}
		imageList := ImageList{
			NewImage(image.NewFromIdentifier("foo=gcr.io/jannfis/foobar:>=1.0.1")),
		}
		appImages := &ApplicationImages{
			Application: v1alpha1.Application{
				ObjectMeta: v1.ObjectMeta{
					Name:      "guestbook",
					Namespace: "guestbook",
				},
				Spec: v1alpha1.ApplicationSpec{
					Source: &v1alpha1.ApplicationSource{
						RepoURL:        "https://example.com/example",
						TargetRevision: "main",
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
							"gcr.io/jannfis/foobar:1.0.1",
						},
					},
				},
			},
			Images: imageList,
			WriteBackConfig: &WriteBackConfig{
				Method:  WriteBackGit,
				GitRepo: "https://example.com/example",
				GetCreds: func(app *v1alpha1.Application) (git.Creds, error) {
					return getCredsFromSecret(&WriteBackConfig{}, "argocd-image-updater/git-creds", &kubeClient)
				},
			},
		}
		res := UpdateApplication(context.Background(), &UpdateConfiguration{
			NewRegFN:   mockClientFn,
			ArgoClient: &argoClient,
			KubeClient: &kubeClient,
			UpdateApp:  appImages,
			DryRun:     false,
		}, NewSyncIterationState())
		assert.Equal(t, v1alpha1.KustomizeImage("gcr.io/jannfis/foobar:1.0.3"), appImages.Application.Spec.Source.Kustomize.Images[0])
		assert.Equal(t, 0, res.NumSkipped)
		assert.Equal(t, 1, res.NumApplicationsProcessed)
		assert.Equal(t, 1, res.NumImagesConsidered)
		// configured githubApp creds will take effect and git client will catch the invalid GithubAppPrivateKey "foo":
		// "Could not update application spec: could not parse private key: invalid key: Key must be a PEM encoded PKCS1 or PKCS8 key"
		assert.Equal(t, 1, res.NumErrors)
	})

	t.Run("Test successful update", func(t *testing.T) {
		mockClientFn := func(endpoint *registry.RegistryEndpoint, username, password string) (registry.RegistryClient, error) {
			regMock := regmock.RegistryClient{}
			regMock.On("NewRepository", mock.MatchedBy(func(s string) bool {
				return s == "jannfis/foobar"
			})).Return(nil)
			regMock.On("Tags", mock.Anything).Return([]string{"1.0.1"}, nil)
			return &regMock, nil
		}

		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeKubeClient(),
			},
		}
		appImages := &ApplicationImages{
			Application: v1alpha1.Application{
				ObjectMeta: v1.ObjectMeta{
					Name:      "guestbook",
					Namespace: "guestbook",
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
			},
			Images: ImageList{
				NewImage(image.NewFromIdentifier("jannfis/foobar:~1.0.0")),
			},
			WriteBackConfig: &WriteBackConfig{
				Method: WriteBackApplication,
			},
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, &application.ApplicationUpdateSpecRequest{
			Name:         &appImages.Application.Name,
			AppNamespace: &appImages.Application.Namespace,
			Spec:         &appImages.Application.Spec,
		}).Return(nil, nil)

		res := UpdateApplication(context.Background(), &UpdateConfiguration{
			NewRegFN:   mockClientFn,
			ArgoClient: &argoClient,
			KubeClient: &kubeClient,
			UpdateApp:  appImages,
			DryRun:     false,
		}, NewSyncIterationState())
		assert.Equal(t, 0, res.NumErrors)
		assert.Equal(t, 0, res.NumSkipped)
		assert.Equal(t, 1, res.NumApplicationsProcessed)
		assert.Equal(t, 1, res.NumImagesConsidered)
		assert.Equal(t, 1, res.NumImagesUpdated)
	})

	t.Run("Test successful update two images", func(t *testing.T) {
		mockClientFn := func(endpoint *registry.RegistryEndpoint, username, password string) (registry.RegistryClient, error) {
			regMock := regmock.RegistryClient{}
			regMock.On("NewRepository", mock.MatchedBy(func(s string) bool {
				return s == "jannfis/foobar" || s == "jannfis/barbar"
			})).Return(nil)
			regMock.On("Tags", mock.Anything).Return([]string{"1.0.1"}, nil)
			return &regMock, nil
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)

		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeKubeClient(),
			},
		}
		appImages := &ApplicationImages{
			Application: v1alpha1.Application{
				ObjectMeta: v1.ObjectMeta{
					Name:      "guestbook",
					Namespace: "guestbook",
				},
				Spec: v1alpha1.ApplicationSpec{
					Source: &v1alpha1.ApplicationSource{
						Kustomize: &v1alpha1.ApplicationSourceKustomize{
							Images: v1alpha1.KustomizeImages{
								"jannfis/foobar:1.0.0",
								"jannfis/barbar:1.0.0",
							},
						},
					},
				},
				Status: v1alpha1.ApplicationStatus{
					SourceType: v1alpha1.ApplicationSourceTypeKustomize,
					Summary: v1alpha1.ApplicationSummary{
						Images: []string{
							"jannfis/foobar:1.0.0",
							"jannfis/barbar:1.0.0",
						},
					},
				},
			},
			Images: ImageList{
				NewImage(
					image.NewFromIdentifier("jannfis/foobar:~1.0.0")),
				NewImage(
					image.NewFromIdentifier("jannfis/barbar:~1.0.0")),
			},
			WriteBackConfig: &WriteBackConfig{
				Method: WriteBackApplication,
			},
		}
		res := UpdateApplication(context.Background(), &UpdateConfiguration{
			NewRegFN:   mockClientFn,
			ArgoClient: &argoClient,
			KubeClient: &kubeClient,
			UpdateApp:  appImages,
			DryRun:     false,
		}, NewSyncIterationState())
		assert.Equal(t, 0, res.NumErrors)
		assert.Equal(t, 0, res.NumSkipped)
		assert.Equal(t, 1, res.NumApplicationsProcessed)
		assert.Equal(t, 2, res.NumImagesConsidered)
		assert.Equal(t, 2, res.NumImagesUpdated)
	})

	t.Run("Test kustomize w/ different registry", func(t *testing.T) {
		mockClientFn := func(endpoint *registry.RegistryEndpoint, username, password string) (registry.RegistryClient, error) {
			regMock := regmock.RegistryClient{}
			assert.Equal(t, endpoint.RegistryPrefix, "quay.io")
			regMock.On("NewRepository", mock.MatchedBy(func(s string) bool {
				return s == "jannfis/foobar"
			})).Return(nil)
			regMock.On("Tags", mock.Anything).Return([]string{"1.0.1"}, nil)
			return &regMock, nil
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)

		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeKubeClient(),
			},
		}

		containerImg := image.NewFromIdentifier("foobar=quay.io/jannfis/foobar:~1.0.0")
		iuImg := NewImage(containerImg)
		iuImg.KustomizeImageName = "jannfis/foobar"
		iuImg.ForceUpdate = true

		imageList := ImageList{iuImg}

		appImages := &ApplicationImages{
			Application: v1alpha1.Application{
				ObjectMeta: v1.ObjectMeta{
					Name:      "guestbook",
					Namespace: "guestbook",
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
			},
			Images: imageList,
			WriteBackConfig: &WriteBackConfig{
				Method: WriteBackApplication,
			},
		}
		res := UpdateApplication(context.Background(), &UpdateConfiguration{
			NewRegFN:   mockClientFn,
			ArgoClient: &argoClient,
			KubeClient: &kubeClient,
			UpdateApp:  appImages,
			DryRun:     false,
		}, NewSyncIterationState())
		assert.Equal(t, 0, res.NumErrors)
		assert.Equal(t, 0, res.NumSkipped)
		assert.Equal(t, 1, res.NumApplicationsProcessed)
		assert.Equal(t, 1, res.NumImagesConsidered)
		assert.Equal(t, 1, res.NumImagesUpdated)
	})

	t.Run("Test kustomize w/ different registry and org", func(t *testing.T) {
		mockClientFn := func(endpoint *registry.RegistryEndpoint, username, password string) (registry.RegistryClient, error) {
			regMock := regmock.RegistryClient{}
			assert.Equal(t, endpoint.RegistryPrefix, "quay.io")
			regMock.On("NewRepository", mock.MatchedBy(func(s string) bool {
				return s == "someorg/foobar"
			})).Return(nil)
			regMock.On("Tags", mock.Anything).Return([]string{"1.0.1"}, nil)
			return &regMock, nil
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)

		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeKubeClient(),
			},
		}

		containerImg := image.NewFromIdentifier("foobar=quay.io/someorg/foobar:~1.0.0")
		iuImg := NewImage(containerImg)
		iuImg.KustomizeImageName = "jannfis/foobar"
		iuImg.ForceUpdate = true

		imageList := ImageList{iuImg}
		appImages := &ApplicationImages{
			Application: v1alpha1.Application{
				ObjectMeta: v1.ObjectMeta{
					Name:      "guestbook",
					Namespace: "guestbook",
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
			},
			Images: imageList,
			WriteBackConfig: &WriteBackConfig{
				Method: WriteBackApplication,
			},
		}
		res := UpdateApplication(context.Background(), &UpdateConfiguration{
			NewRegFN:   mockClientFn,
			ArgoClient: &argoClient,
			KubeClient: &kubeClient,
			UpdateApp:  appImages,
			DryRun:     false,
		}, NewSyncIterationState())
		assert.Equal(t, 0, res.NumErrors)
		assert.Equal(t, 0, res.NumSkipped)
		assert.Equal(t, 1, res.NumApplicationsProcessed)
		assert.Equal(t, 1, res.NumImagesConsidered)
		assert.Equal(t, 1, res.NumImagesUpdated)
	})

	t.Run("Test successful update when no tag is set in running workload", func(t *testing.T) {
		mockClientFn := func(endpoint *registry.RegistryEndpoint, username, password string) (registry.RegistryClient, error) {
			regMock := regmock.RegistryClient{}
			regMock.On("NewRepository", mock.Anything).Return(nil)
			regMock.On("Tags", mock.Anything).Return([]string{"1.0.1"}, nil)
			return &regMock, nil
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)

		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeKubeClient(),
			},
		}
		appImages := &ApplicationImages{
			Application: v1alpha1.Application{
				ObjectMeta: v1.ObjectMeta{
					Name:      "guestbook",
					Namespace: "guestbook",
				},
				Spec: v1alpha1.ApplicationSpec{
					Source: &v1alpha1.ApplicationSource{
						Kustomize: &v1alpha1.ApplicationSourceKustomize{
							Images: v1alpha1.KustomizeImages{
								"jannfis/foobar",
							},
						},
					},
				},
				Status: v1alpha1.ApplicationStatus{
					SourceType: v1alpha1.ApplicationSourceTypeKustomize,
					Summary: v1alpha1.ApplicationSummary{
						Images: []string{
							"jannfis/foobar",
						},
					},
				},
			},
			Images: ImageList{
				NewImage(
					image.NewFromIdentifier("jannfis/foobar:1.0.x")),
			},
			WriteBackConfig: &WriteBackConfig{
				Method: WriteBackApplication,
			},
		}
		res := UpdateApplication(context.Background(), &UpdateConfiguration{
			NewRegFN:   mockClientFn,
			ArgoClient: &argoClient,
			KubeClient: &kubeClient,
			UpdateApp:  appImages,
			DryRun:     false,
		}, NewSyncIterationState())
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
			regMock.On("NewRepository", mock.Anything).Return(nil)
			regMock.On("Tags", mock.Anything).Return([]string{"1.0.1"}, nil)
			return &regMock, nil
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)

		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeClientsetWithResources(fixture.NewSecret("foo", "bar", map[string][]byte{"creds": []byte("myuser:mypass")})),
			},
		}

		img := NewImage(image.NewFromIdentifier("dummy=jannfis/foobar:1.0.1"))
		img.PullSecret = "secret:foo/bar#creds"

		appImages := &ApplicationImages{
			Application: v1alpha1.Application{
				ObjectMeta: v1.ObjectMeta{
					Name:      "guestbook",
					Namespace: "guestbook",
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
			},
			Images: ImageList{img},
			WriteBackConfig: &WriteBackConfig{
				Method: WriteBackApplication,
			},
		}
		res := UpdateApplication(context.Background(), &UpdateConfiguration{
			NewRegFN:   mockClientFn,
			ArgoClient: &argoClient,
			KubeClient: &kubeClient,
			UpdateApp:  appImages,
			DryRun:     false,
		}, NewSyncIterationState())
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

		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeKubeClient(),
			},
		}
		appImages := &ApplicationImages{
			Application: v1alpha1.Application{
				ObjectMeta: v1.ObjectMeta{
					Name:      "guestbook",
					Namespace: "guestbook",
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
			},
			Images: ImageList{
				NewImage(
					image.NewFromIdentifier("jannfis/barbar:1.0.1")),
			},
			WriteBackConfig: &WriteBackConfig{
				Method: WriteBackApplication,
			},
		}
		res := UpdateApplication(context.Background(), &UpdateConfiguration{
			NewRegFN:   mockClientFn,
			ArgoClient: &argoClient,
			KubeClient: &kubeClient,
			UpdateApp:  appImages,
			DryRun:     false,
		}, NewSyncIterationState())
		assert.Equal(t, 0, res.NumErrors)
		assert.Equal(t, 1, res.NumSkipped)
		assert.Equal(t, 1, res.NumApplicationsProcessed)
		assert.Equal(t, 0, res.NumImagesConsidered)
		assert.Equal(t, 0, res.NumImagesUpdated)
	})

	t.Run("Test skip because of image up-to-date", func(t *testing.T) {
		mockClientFn := func(endpoint *registry.RegistryEndpoint, username, password string) (registry.RegistryClient, error) {
			regMock := regmock.RegistryClient{}
			regMock.On("NewRepository", mock.Anything).Return(nil)
			regMock.On("Tags", mock.Anything).Return([]string{"1.0.1"}, nil)
			return &regMock, nil
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)

		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeKubeClient(),
			},
		}
		appImages := &ApplicationImages{
			Application: v1alpha1.Application{
				ObjectMeta: v1.ObjectMeta{
					Name:      "guestbook",
					Namespace: "guestbook",
				},
				Spec: v1alpha1.ApplicationSpec{
					Source: &v1alpha1.ApplicationSource{
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
			Images: ImageList{
				NewImage(
					image.NewFromIdentifier("jannfis/foobar:1.0.1")),
			},
			WriteBackConfig: &WriteBackConfig{
				Method: WriteBackApplication,
			},
		}
		res := UpdateApplication(context.Background(), &UpdateConfiguration{
			NewRegFN:   mockClientFn,
			ArgoClient: &argoClient,
			KubeClient: &kubeClient,
			UpdateApp:  appImages,
			DryRun:     false,
		}, NewSyncIterationState())
		assert.Equal(t, 0, res.NumErrors)
		assert.Equal(t, 0, res.NumSkipped)
		assert.Equal(t, 1, res.NumApplicationsProcessed)
		assert.Equal(t, 1, res.NumImagesConsidered)
		assert.Equal(t, 0, res.NumImagesUpdated)
	})

	t.Run("Test update because of image registry changed", func(t *testing.T) {
		mockClientFn := func(endpoint *registry.RegistryEndpoint, username, password string) (registry.RegistryClient, error) {
			regMock := regmock.RegistryClient{}
			regMock.On("NewRepository", mock.Anything).Return(nil)
			regMock.On("Tags", mock.Anything).Return([]string{"1.0.1"}, nil)
			return &regMock, nil
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)

		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeKubeClient(),
			},
		}

		containerImg := image.NewFromIdentifier("foobar=gcr.io/jannfis/foobar:>=1.0.1")
		iuImg := NewImage(containerImg)
		iuImg.KustomizeImageName = "jannfis/foobar"
		iuImg.ContainerImage.KustomizeImage = image.NewFromIdentifier("jannfis/foobar")
		iuImg.ForceUpdate = false
		imageList := ImageList{iuImg}
		appImages := &ApplicationImages{
			Application: v1alpha1.Application{
				ObjectMeta: v1.ObjectMeta{
					Name:      "guestbook",
					Namespace: "guestbook",
				},
				Spec: v1alpha1.ApplicationSpec{
					Source: &v1alpha1.ApplicationSource{
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
			Images: imageList,
			WriteBackConfig: &WriteBackConfig{
				Method: WriteBackApplication,
			},
		}
		res := UpdateApplication(context.Background(), &UpdateConfiguration{
			NewRegFN:   mockClientFn,
			ArgoClient: &argoClient,
			KubeClient: &kubeClient,
			UpdateApp:  appImages,
			DryRun:     false,
		}, NewSyncIterationState())
		assert.Equal(t, 0, res.NumErrors)
		assert.Equal(t, 0, res.NumSkipped)
		assert.Equal(t, 1, res.NumApplicationsProcessed)
		assert.Equal(t, 1, res.NumImagesConsidered)
		assert.Equal(t, 1, res.NumImagesUpdated)
	})

	t.Run("Test not updated because kustomize image is the same", func(t *testing.T) {
		mockClientFn := func(endpoint *registry.RegistryEndpoint, username, password string) (registry.RegistryClient, error) {
			regMock := regmock.RegistryClient{}
			regMock.On("NewRepository", mock.Anything).Return(nil)
			regMock.On("Tags", mock.Anything).Return([]string{"1.0.1"}, nil)
			return &regMock, nil
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)

		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeKubeClient(),
			},
		}

		containerImg := image.NewFromIdentifier("foobar=gcr.io/jannfis/foobar:>=1.0.1")
		iuImg := NewImage(containerImg)
		iuImg.KustomizeImageName = "jannfis/foobar"
		iuImg.ForceUpdate = false
		imageList := ImageList{iuImg}
		appImages := &ApplicationImages{
			Application: v1alpha1.Application{
				ObjectMeta: v1.ObjectMeta{
					Name:      "guestbook",
					Namespace: "guestbook",
				},
				Spec: v1alpha1.ApplicationSpec{
					Source: &v1alpha1.ApplicationSource{
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
							"gcr.io/jannfis/foobar:1.0.1",
						},
					},
				},
			},
			Images: imageList,
			WriteBackConfig: &WriteBackConfig{
				Method: WriteBackApplication,
			},
		}
		res := UpdateApplication(context.Background(), &UpdateConfiguration{
			NewRegFN:   mockClientFn,
			ArgoClient: &argoClient,
			KubeClient: &kubeClient,
			UpdateApp:  appImages,
			DryRun:     false,
		}, NewSyncIterationState())
		assert.Equal(t, 0, res.NumErrors)
		assert.Equal(t, 0, res.NumSkipped)
		assert.Equal(t, 1, res.NumApplicationsProcessed)
		assert.Equal(t, 1, res.NumImagesConsidered)
		assert.Equal(t, 0, res.NumImagesUpdated)
	})

	t.Run("Test skip because of match-tag pattern doesn't match", func(t *testing.T) {
		meta := make([]*schema1.SignedManifest, 4) //nolint:staticcheck
		for i := 0; i < 4; i++ {
			ts := fmt.Sprintf("2006-01-02T15:%.02d:05.999999999Z", i)
			meta[i] = &schema1.SignedManifest{ //nolint:staticcheck
				Manifest: schema1.Manifest{ //nolint:staticcheck
					History: []schema1.History{ //nolint:staticcheck
						{
							V1Compatibility: `{"created":"` + ts + `"}`,
						},
					},
				},
			}
		}
		called := 0
		mockClientFn := func(endpoint *registry.RegistryEndpoint, username, password string) (registry.RegistryClient, error) {
			regMock := regmock.RegistryClient{}
			regMock.On("NewRepository", mock.Anything).Return(nil)
			regMock.On("Tags", mock.Anything).Return([]string{"one", "two", "three", "four"}, nil)
			regMock.On("Manifest", mock.Anything).Return(meta[called], nil)
			called += 1
			return &regMock, nil
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)

		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeKubeClient(),
			},
		}
		appImages := &ApplicationImages{
			Application: v1alpha1.Application{
				ObjectMeta: v1.ObjectMeta{
					Name:      "guestbook",
					Namespace: "guestbook",
				},
				Spec: v1alpha1.ApplicationSpec{
					Source: &v1alpha1.ApplicationSource{
						Kustomize: &v1alpha1.ApplicationSourceKustomize{
							Images: v1alpha1.KustomizeImages{
								"jannfis/foobar:one",
							},
						},
					},
				},
				Status: v1alpha1.ApplicationStatus{
					SourceType: v1alpha1.ApplicationSourceTypeKustomize,
					Summary: v1alpha1.ApplicationSummary{
						Images: []string{
							"jannfis/foobar:one",
						},
					},
				},
			},
			Images: ImageList{
				NewImage(
					image.NewFromIdentifier("dummy=jannfis/foobar")),
			},
			WriteBackConfig: &WriteBackConfig{
				Method: WriteBackApplication,
			},
		}
		res := UpdateApplication(context.Background(), &UpdateConfiguration{
			NewRegFN:   mockClientFn,
			ArgoClient: &argoClient,
			KubeClient: &kubeClient,
			UpdateApp:  appImages,
			DryRun:     false,
		}, NewSyncIterationState())
		assert.Equal(t, 0, res.NumErrors)
		assert.Equal(t, 0, res.NumSkipped)
		assert.Equal(t, 1, res.NumApplicationsProcessed)
		assert.Equal(t, 1, res.NumImagesConsidered)
		assert.Equal(t, 0, res.NumImagesUpdated)
	})

	t.Run("Test skip because of ignored", func(t *testing.T) {
		meta := make([]*schema1.SignedManifest, 4) //nolint:staticcheck
		for i := 0; i < 4; i++ {
			ts := fmt.Sprintf("2006-01-02T15:%.02d:05.999999999Z", i)
			meta[i] = &schema1.SignedManifest{ //nolint:staticcheck
				Manifest: schema1.Manifest{ //nolint:staticcheck
					History: []schema1.History{ //nolint:staticcheck
						{
							V1Compatibility: `{"created":"` + ts + `"}`,
						},
					},
				},
			}
		}
		called := 0
		mockClientFn := func(endpoint *registry.RegistryEndpoint, username, password string) (registry.RegistryClient, error) {
			regMock := regmock.RegistryClient{}
			regMock.On("NewRepository", mock.Anything).Return(nil)
			regMock.On("Tags", mock.Anything).Return([]string{"one", "two", "three", "four"}, nil)
			regMock.On("Manifest", mock.Anything).Return(meta[called], nil)
			called += 1
			return &regMock, nil
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)

		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeKubeClient(),
			},
		}
		appImages := &ApplicationImages{
			Application: v1alpha1.Application{
				ObjectMeta: v1.ObjectMeta{
					Name:      "guestbook",
					Namespace: "guestbook",
				},
				Spec: v1alpha1.ApplicationSpec{
					Source: &v1alpha1.ApplicationSource{
						Kustomize: &v1alpha1.ApplicationSourceKustomize{
							Images: v1alpha1.KustomizeImages{
								"jannfis/foobar:one",
							},
						},
					},
				},
				Status: v1alpha1.ApplicationStatus{
					SourceType: v1alpha1.ApplicationSourceTypeKustomize,
					Summary: v1alpha1.ApplicationSummary{
						Images: []string{
							"jannfis/foobar:one",
						},
					},
				},
			},
			Images: ImageList{
				NewImage(
					image.NewFromIdentifier("dummy=jannfis/foobar")),
			},
			WriteBackConfig: &WriteBackConfig{
				Method: WriteBackApplication,
			},
		}
		res := UpdateApplication(context.Background(), &UpdateConfiguration{
			NewRegFN:   mockClientFn,
			ArgoClient: &argoClient,
			KubeClient: &kubeClient,
			UpdateApp:  appImages,
			DryRun:     false,
		}, NewSyncIterationState())
		assert.Equal(t, 0, res.NumErrors)
		assert.Equal(t, 0, res.NumSkipped)
		assert.Equal(t, 1, res.NumApplicationsProcessed)
		assert.Equal(t, 1, res.NumImagesConsidered)
		assert.Equal(t, 0, res.NumImagesUpdated)
	})

	t.Run("Update from inferred registry", func(t *testing.T) {
		mockClientFn := func(endpoint *registry.RegistryEndpoint, username, password string) (registry.RegistryClient, error) {
			regMock := regmock.RegistryClient{}
			regMock.On("NewRepository", mock.Anything).Return(nil)
			regMock.On("Tags", mock.Anything).Return([]string{"1.0.1", "1.0.2"}, nil)
			return &regMock, nil
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)

		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeKubeClient(),
			},
		}
		appImages := &ApplicationImages{
			Application: v1alpha1.Application{
				ObjectMeta: v1.ObjectMeta{
					Name:      "guestbook",
					Namespace: "guestbook",
				},
				Spec: v1alpha1.ApplicationSpec{
					Source: &v1alpha1.ApplicationSource{
						Kustomize: &v1alpha1.ApplicationSourceKustomize{
							Images: v1alpha1.KustomizeImages{
								"example.io/jannfis/example:1.0.1",
							},
						},
					},
				},
				Status: v1alpha1.ApplicationStatus{
					SourceType: v1alpha1.ApplicationSourceTypeKustomize,
					Summary: v1alpha1.ApplicationSummary{
						Images: []string{
							"example.io/jannfis/example:1.0.1",
						},
					},
				},
			},
			Images: ImageList{
				NewImage(
					image.NewFromIdentifier("example.io/jannfis/example:1.0.x")),
			},
			WriteBackConfig: &WriteBackConfig{
				Method: WriteBackApplication,
			},
		}
		res := UpdateApplication(context.Background(), &UpdateConfiguration{
			NewRegFN:   mockClientFn,
			ArgoClient: &argoClient,
			KubeClient: &kubeClient,
			UpdateApp:  appImages,
			DryRun:     false,
		}, NewSyncIterationState())
		assert.Equal(t, 0, res.NumErrors)
		assert.Equal(t, 0, res.NumSkipped)
		assert.Equal(t, 1, res.NumApplicationsProcessed)
		assert.Equal(t, 1, res.NumImagesConsidered)
		assert.Equal(t, 1, res.NumImagesUpdated)
	})

	t.Run("Test error on generic registry client failure", func(t *testing.T) {
		mockClientFn := func(endpoint *registry.RegistryEndpoint, username, password string) (registry.RegistryClient, error) {
			return nil, errors.New("some error")
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)

		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeKubeClient(),
			},
		}
		appImages := &ApplicationImages{
			Application: v1alpha1.Application{
				ObjectMeta: v1.ObjectMeta{
					Name:      "guestbook",
					Namespace: "guestbook",
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
			},
			WriteBackConfig: &WriteBackConfig{
				Method: WriteBackApplication,
			},
			Images: ImageList{
				NewImage(
					image.NewFromIdentifier("jannfis/foobar:1.0.1")),
			},
		}
		res := UpdateApplication(context.Background(), &UpdateConfiguration{
			NewRegFN:   mockClientFn,
			ArgoClient: &argoClient,
			KubeClient: &kubeClient,
			UpdateApp:  appImages,
			DryRun:     false,
		}, NewSyncIterationState())
		assert.Equal(t, 1, res.NumErrors)
		assert.Equal(t, 0, res.NumSkipped)
		assert.Equal(t, 1, res.NumApplicationsProcessed)
		assert.Equal(t, 1, res.NumImagesConsidered)
		assert.Equal(t, 0, res.NumImagesUpdated)
	})

	t.Run("Test error on failure to list tags", func(t *testing.T) {
		mockClientFn := func(endpoint *registry.RegistryEndpoint, username, password string) (registry.RegistryClient, error) {
			regMock := regmock.RegistryClient{}
			regMock.On("NewRepository", mock.Anything).Return(nil)
			regMock.On("Tags", mock.Anything).Return(nil, errors.New("some error"))
			return &regMock, nil
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)

		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeKubeClient(),
			},
		}
		appImages := &ApplicationImages{
			Application: v1alpha1.Application{
				ObjectMeta: v1.ObjectMeta{
					Name:      "guestbook",
					Namespace: "guestbook",
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
			},
			WriteBackConfig: &WriteBackConfig{
				Method: WriteBackApplication,
			},
			Images: ImageList{
				NewImage(
					image.NewFromIdentifier("jannfis/foobar:1.0.1")),
			},
		}
		res := UpdateApplication(context.Background(), &UpdateConfiguration{
			NewRegFN:   mockClientFn,
			ArgoClient: &argoClient,
			KubeClient: &kubeClient,
			UpdateApp:  appImages,
			DryRun:     false,
		}, NewSyncIterationState())
		assert.Equal(t, 1, res.NumErrors)
		assert.Equal(t, 0, res.NumSkipped)
		assert.Equal(t, 1, res.NumApplicationsProcessed)
		assert.Equal(t, 1, res.NumImagesConsidered)
		assert.Equal(t, 0, res.NumImagesUpdated)
	})

	t.Run("Test error on improper semver in tag", func(t *testing.T) {
		mockClientFn := func(endpoint *registry.RegistryEndpoint, username, password string) (registry.RegistryClient, error) {
			regMock := regmock.RegistryClient{}
			regMock.On("NewRepository", mock.Anything).Return(nil)
			regMock.On("Tags", mock.Anything).Return([]string{"1.0.0", "1.0.1"}, nil)
			return &regMock, nil
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)

		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeKubeClient(),
			},
		}
		appImages := &ApplicationImages{
			Application: v1alpha1.Application{
				ObjectMeta: v1.ObjectMeta{
					Name:      "guestbook",
					Namespace: "guestbook",
				},
				Spec: v1alpha1.ApplicationSpec{
					Source: &v1alpha1.ApplicationSource{
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
			WriteBackConfig: &WriteBackConfig{
				Method: WriteBackApplication,
			},
			Images: ImageList{
				NewImage(
					image.NewFromIdentifier("jannfis/foobar:stable")),
			},
		}
		res := UpdateApplication(context.Background(), &UpdateConfiguration{
			NewRegFN:   mockClientFn,
			ArgoClient: &argoClient,
			KubeClient: &kubeClient,
			UpdateApp:  appImages,
			DryRun:     false,
		}, NewSyncIterationState())
		assert.Equal(t, 1, res.NumErrors)
		assert.Equal(t, 0, res.NumSkipped)
		assert.Equal(t, 1, res.NumApplicationsProcessed)
		assert.Equal(t, 1, res.NumImagesConsidered)
		assert.Equal(t, 0, res.NumImagesUpdated)
	})

	t.Run("Test Kubernetes Job with forceUpdate and digest strategy (issue #1344)", func(t *testing.T) {
		// This test reproduces the scenario from issue #1344:
		// - Application uses a Kubernetes Job (not in app.Status.Summary.Images)
		// - forceUpdate: true is set
		// - updateStrategy: "digest" is used
		// - A version constraint (tag) is provided

		mockClientFn := func(endpoint *registry.RegistryEndpoint, username, password string) (registry.RegistryClient, error) {
			regMock := regmock.RegistryClient{}
			regMock.On("NewRepository", mock.MatchedBy(func(s string) bool {
				return s == "org/job-image"
			})).Return(nil)
			// Return the tag that matches our version constraint
			regMock.On("Tags", mock.Anything).Return([]string{"latest"}, nil)

			// For digest strategy, we need to mock ManifestForTag and TagMetadata
			meta1 := &schema1.SignedManifest{} //nolint:staticcheck
			meta1.Name = "org/job-image"
			meta1.Tag = "latest"
			regMock.On("ManifestForTag", mock.Anything, "latest").Return(meta1, nil)
			// Create a digest as [32]byte array
			var digest [32]byte
			copy(digest[:], []byte("abcdef1234567890"))
			regMock.On("TagMetadata", mock.Anything, mock.Anything, mock.Anything).Return(&tag.TagInfo{
				CreatedAt: time.Unix(1234567890, 0),
				Digest:    digest,
			}, nil)

			return &regMock, nil
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)

		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeKubeClient(),
			},
		}

		// Image configuration with forceUpdate and digest strategy
		// The tag "latest" serves as the version constraint for digest strategy
		containerImg := image.NewFromIdentifier("job-image=gcr.io/org/job-image:latest")
		iuImg := NewImage(containerImg)
		iuImg.KustomizeImageName = "org/job-image"
		iuImg.ForceUpdate = true
		iuImg.UpdateStrategy = image.StrategyDigest

		imageList := ImageList{iuImg}

		appImages := &ApplicationImages{
			Application: v1alpha1.Application{
				ObjectMeta: v1.ObjectMeta{
					Name:      "job-app",
					Namespace: "default",
				},
				Spec: v1alpha1.ApplicationSpec{
					Source: &v1alpha1.ApplicationSource{
						Kustomize: &v1alpha1.ApplicationSourceKustomize{
							Images: v1alpha1.KustomizeImages{},
						},
					},
				},
				Status: v1alpha1.ApplicationStatus{
					SourceType: v1alpha1.ApplicationSourceTypeKustomize,
					Summary: v1alpha1.ApplicationSummary{
						// Empty images list - simulating a Kubernetes Job that doesn't
						// appear in the application status
						Images: []string{},
					},
				},
			},
			Images: imageList,
			WriteBackConfig: &WriteBackConfig{
				Method: WriteBackApplication,
			},
		}

		// Before the fix for issue #1344, this would fail with:
		// "cannot use update strategy 'digest' for image... without a version constraint"
		// because the constraint was lost when setting ImageTag to nil
		res := UpdateApplication(context.Background(), &UpdateConfiguration{
			NewRegFN:   mockClientFn,
			ArgoClient: &argoClient,
			KubeClient: &kubeClient,
			UpdateApp:  appImages,
			DryRun:     false,
		}, NewSyncIterationState())

		// Verify the update succeeded
		assert.Equal(t, 0, res.NumErrors, "Should not have errors with forceUpdate + digest strategy")
		assert.Equal(t, 0, res.NumSkipped)
		assert.Equal(t, 1, res.NumApplicationsProcessed)
		assert.Equal(t, 1, res.NumImagesConsidered)
		assert.Equal(t, 1, res.NumImagesUpdated, "Image should be updated with digest")

		// Verify the kustomize image was updated with the digest
		require.Len(t, appImages.Application.Spec.Source.Kustomize.Images, 1)
		updatedImage := string(appImages.Application.Spec.Source.Kustomize.Images[0])
		assert.Contains(t, updatedImage, "gcr.io/org/job-image")
		assert.Contains(t, updatedImage, "latest")
		assert.Contains(t, updatedImage, "sha256:", "Image should include digest")

		// The constraint on the original image must still be present.
		require.NotNil(t, iuImg.ContainerImage.ImageTag)
		assert.Equal(t, "latest", iuImg.ContainerImage.ImageTag.TagName)
	})

}

func Test_MarshalParamsOverride(t *testing.T) {
	t.Run("Valid Kustomize source", func(t *testing.T) {
		expected := `
kustomize:
  images:
  - baz
  - foo
  - bar
`
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "https://example.com/example",
					TargetRevision: "main",
					Kustomize: &v1alpha1.ApplicationSourceKustomize{
						Images: v1alpha1.KustomizeImages{
							"foo",
							"bar",
						},
					},
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeKustomize,
			},
		}
		originalData := []byte(`
kustomize:
  images:
  - baz
`)
		// This test doesn't use helmvalues, but we populate Images for consistency.
		applicationImages := &ApplicationImages{
			Application: app,
			Images: ImageList{
				NewImage(
					image.NewFromIdentifier("nginx")),
			},
		}

		yaml, err := marshalParamsOverride(context.Background(), applicationImages, originalData)
		require.NoError(t, err)
		assert.NotEmpty(t, yaml)
		assert.Equal(t, strings.TrimSpace(expected), strings.TrimSpace(string(yaml)))
	})

	t.Run("Merge images param", func(t *testing.T) {
		expected := `
kustomize:
  images:
  - existing:latest
  - updated:latest
  - new
`
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "https://example.com/example",
					TargetRevision: "main",
					Kustomize: &v1alpha1.ApplicationSourceKustomize{
						Images: v1alpha1.KustomizeImages{
							"new",
							"updated:latest",
						},
					},
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeKustomize,
			},
		}
		originalData := []byte(`
kustomize:
  images:
  - existing:latest
  - updated:old
`)
		applicationImages := &ApplicationImages{
			Application: app,
			Images: ImageList{
				NewImage(
					image.NewFromIdentifier("nginx")),
			},
		}

		yaml, err := marshalParamsOverride(context.Background(), applicationImages, originalData)
		require.NoError(t, err)
		assert.NotEmpty(t, yaml)
		assert.Equal(t, strings.TrimSpace(expected), strings.TrimSpace(string(yaml)))
	})

	t.Run("Empty Kustomize source", func(t *testing.T) {
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "https://example.com/example",
					TargetRevision: "main",
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeKustomize,
			},
		}
		applicationImages := &ApplicationImages{
			Application: app,
			Images: ImageList{
				NewImage(
					image.NewFromIdentifier("nginx")),
			},
		}

		yaml, err := marshalParamsOverride(context.Background(), applicationImages, nil)
		require.NoError(t, err)
		assert.Empty(t, yaml)
		assert.Equal(t, "", strings.TrimSpace(string(yaml)))
	})

	t.Run("Valid Helm source", func(t *testing.T) {
		expected := `
helm:
  parameters:
  - name: baz
    value: baz
    forcestring: false
  - name: foo
    value: bar
    forcestring: true
  - name: bar
    value: foo
    forcestring: true
`
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "https://example.com/example",
					TargetRevision: "main",
					Helm: &v1alpha1.ApplicationSourceHelm{
						Parameters: []v1alpha1.HelmParameter{
							{
								Name:        "foo",
								Value:       "bar",
								ForceString: true,
							},
							{
								Name:        "bar",
								Value:       "foo",
								ForceString: true,
							},
						},
					},
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeHelm,
			},
		}

		originalData := []byte(`
helm:
  parameters:
  - name: baz
    value: baz
    forcestring: false
`)
		applicationImages := &ApplicationImages{
			Application: app,
			Images: ImageList{
				NewImage(
					image.NewFromIdentifier("nginx")),
			},
		}
		yaml, err := marshalParamsOverride(context.Background(), applicationImages, originalData)
		require.NoError(t, err)
		assert.NotEmpty(t, yaml)
		assert.Equal(t, strings.TrimSpace(strings.ReplaceAll(expected, "\t", "  ")), strings.TrimSpace(string(yaml)))
	})

	t.Run("Empty originalData error with valid Helm source", func(t *testing.T) {
		expected := `
helm:
  parameters:
  - name: foo
    value: bar
    forcestring: true
  - name: bar
    value: foo
    forcestring: true
`
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "https://example.com/example",
					TargetRevision: "main",
					Helm: &v1alpha1.ApplicationSourceHelm{
						Parameters: []v1alpha1.HelmParameter{
							{
								Name:        "foo",
								Value:       "bar",
								ForceString: true,
							},
							{
								Name:        "bar",
								Value:       "foo",
								ForceString: true,
							},
						},
					},
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeHelm,
			},
		}

		originalData := []byte(``)
		applicationImages := &ApplicationImages{
			Application: app,
			Images: ImageList{
				NewImage(
					image.NewFromIdentifier("nginx")),
			},
		}
		yaml, err := marshalParamsOverride(context.Background(), applicationImages, originalData)
		require.NoError(t, err)
		assert.NotEmpty(t, yaml)
		assert.Equal(t, strings.TrimSpace(strings.ReplaceAll(expected, "\t", "  ")), strings.TrimSpace(string(yaml)))
	})

	t.Run("Invalid unmarshal originalData error with valid Helm source", func(t *testing.T) {
		expected := `
helm:
  parameters:
  - name: foo
    value: bar
    forcestring: true
  - name: bar
    value: foo
    forcestring: true
`
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "https://example.com/example",
					TargetRevision: "main",
					Helm: &v1alpha1.ApplicationSourceHelm{
						Parameters: []v1alpha1.HelmParameter{
							{
								Name:        "foo",
								Value:       "bar",
								ForceString: true,
							},
							{
								Name:        "bar",
								Value:       "foo",
								ForceString: true,
							},
						},
					},
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeHelm,
			},
		}

		originalData := []byte(`random content`)
		applicationImages := &ApplicationImages{
			Application: app,
			Images: ImageList{
				NewImage(
					image.NewFromIdentifier("nginx")),
			},
		}
		yaml, err := marshalParamsOverride(context.Background(), applicationImages, originalData)
		require.NoError(t, err)
		assert.NotEmpty(t, yaml)
		assert.Equal(t, strings.TrimSpace(strings.ReplaceAll(expected, "\t", "  ")), strings.TrimSpace(string(yaml)))
	})

	t.Run("Empty Helm source", func(t *testing.T) {
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "https://example.com/example",
					TargetRevision: "main",
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeHelm,
			},
		}

		applicationImages := &ApplicationImages{
			Application: app,
			Images: ImageList{
				NewImage(
					image.NewFromIdentifier("nginx")),
			},
		}

		yaml, err := marshalParamsOverride(context.Background(), applicationImages, nil)
		require.NoError(t, err)
		assert.Empty(t, yaml)
	})

	t.Run("Valid Helm source with Helm values file", func(t *testing.T) {
		expected := `
image.name: nginx
image.tag: v1.0.0
replicas: 1
`
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "https://example.com/example",
					TargetRevision: "main",
					Helm: &v1alpha1.ApplicationSourceHelm{
						Parameters: []v1alpha1.HelmParameter{
							{
								Name:        "image.name",
								Value:       "nginx",
								ForceString: true,
							},
							{
								Name:        "image.tag",
								Value:       "v1.0.0",
								ForceString: true,
							},
						},
					},
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeHelm,
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{
						"nginx:v0.0.0",
					},
				},
			},
		}

		originalData := []byte(`
image.name: nginx
image.tag: v0.0.0
replicas: 1
`)
		im := NewImage(
			image.NewFromIdentifier("nginx"))
		im.HelmImageName = "image.name"
		im.HelmImageTag = "image.tag"
		applicationImages := &ApplicationImages{
			Application: app,
			Images:      ImageList{im},
			WriteBackConfig: &WriteBackConfig{
				Target: "./test-values.yaml",
			},
		}

		yaml, err := marshalParamsOverride(context.Background(), applicationImages, originalData)
		require.NoError(t, err)
		assert.NotEmpty(t, yaml)
		assert.Equal(t, strings.TrimSpace(strings.ReplaceAll(expected, "\t", "  ")), strings.TrimSpace(string(yaml)))
	})

	t.Run("Valid Helm source with Helm values file and image-spec", func(t *testing.T) {
		expected := `
image.spec.foo: nginx:v1.0.0
replicas: 1
`
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "https://example.com/example",
					TargetRevision: "main",
					Helm: &v1alpha1.ApplicationSourceHelm{
						Parameters: []v1alpha1.HelmParameter{
							{
								Name:        "image.spec.foo",
								Value:       "nginx:v1.0.0",
								ForceString: true,
							},
						},
					},
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeHelm,
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{
						"nginx:v0.0.0",
					},
				},
			},
		}

		originalData := []byte(`
image.spec.foo: nginx:v0.0.0
replicas: 1
`)
		im := NewImage(
			image.NewFromIdentifier("nginx"))
		im.HelmImageSpec = "image.spec.foo"
		applicationImages := &ApplicationImages{
			Application: app,
			Images:      ImageList{im},
			WriteBackConfig: &WriteBackConfig{
				Target: "./test-values.yaml",
			},
		}

		yaml, err := marshalParamsOverride(context.Background(), applicationImages, originalData)
		require.NoError(t, err)
		assert.NotEmpty(t, yaml)
		assert.Equal(t, strings.TrimSpace(strings.ReplaceAll(expected, "\t", "  ")), strings.TrimSpace(string(yaml)))

		// when image.spec.foo fields are missing in the target helm value file,
		// they should be auto created without corrupting any other pre-existing elements.
		originalData = []byte("test-value1: one")
		expected = `
test-value1: one
image:
  spec:
    foo: nginx:v1.0.0
`

		yaml, err = marshalParamsOverride(context.Background(), applicationImages, originalData)
		require.NoError(t, err)
		assert.NotEmpty(t, yaml)
		assert.Equal(t, strings.TrimSpace(strings.ReplaceAll(expected, "\t", "  ")), strings.TrimSpace(string(yaml)))
	})

	t.Run("Valid Helm source with Helm values file with multiple images", func(t *testing.T) {
		expected := `
nginx.image.name: nginx
nginx.image.tag: v1.0.0
redis.image.name: redis
redis.image.tag: v1.0.0
replicas: 1
`
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Sources: []v1alpha1.ApplicationSource{
					{
						Chart: "my-app",
						Helm: &v1alpha1.ApplicationSourceHelm{
							ReleaseName: "my-app",
							ValueFiles:  []string{"$values/some/dir/values.yaml"},
							Parameters: []v1alpha1.HelmParameter{
								{
									Name:        "nginx.image.name",
									Value:       "nginx",
									ForceString: true,
								},
								{
									Name:        "nginx.image.tag",
									Value:       "v1.0.0",
									ForceString: true,
								},
								{
									Name:        "redis.image.name",
									Value:       "redis",
									ForceString: true,
								},
								{
									Name:        "redis.image.tag",
									Value:       "v1.0.0",
									ForceString: true,
								},
							},
						},
						RepoURL:        "https://example.com/example",
						TargetRevision: "main",
					},
					{
						Ref:            "values",
						RepoURL:        "https://example.com/example2",
						TargetRevision: "main",
					},
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceTypes: []v1alpha1.ApplicationSourceType{
					v1alpha1.ApplicationSourceTypeHelm,
					"",
				},
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{
						"nginx:v0.0.0",
						"redis:v0.0.0",
					},
				},
			},
		}

		originalData := []byte(`
nginx.image.name: nginx
nginx.image.tag: v0.0.0
redis.image.name: redis
redis.image.tag: v0.0.0
replicas: 1
`)
		imNginx := NewImage(
			image.NewFromIdentifier("nginx=nginx"))
		imNginx.HelmImageName = "nginx.image.name"
		imNginx.HelmImageTag = "nginx.image.tag"
		imRedis := NewImage(
			image.NewFromIdentifier("redis=redis"))
		imRedis.HelmImageName = "redis.image.name"
		imRedis.HelmImageTag = "redis.image.tag"
		applicationImages := &ApplicationImages{
			Application: app,
			Images:      ImageList{imNginx, imRedis},
			WriteBackConfig: &WriteBackConfig{
				Target: "./test-values.yaml",
			},
		}

		yaml, err := marshalParamsOverride(context.Background(), applicationImages, originalData)
		require.NoError(t, err)
		assert.NotEmpty(t, yaml)
		assert.Equal(t, strings.TrimSpace(strings.ReplaceAll(expected, "\t", "  ")), strings.TrimSpace(string(yaml)))

		// when nginx.* and redis.* fields are missing in the target helm value file,
		// they should be auto created without corrupting any other pre-existing elements.
		originalData = []byte("test-value1: one")
		expected = `
test-value1: one
nginx:
  image:
    tag: v1.0.0
    name: nginx
redis:
  image:
    tag: v1.0.0
    name: redis
`
		yaml, err = marshalParamsOverride(context.Background(), applicationImages, originalData)
		require.NoError(t, err)
		assert.NotEmpty(t, yaml)
		assert.Equal(t, strings.TrimSpace(strings.ReplaceAll(expected, "\t", "  ")), strings.TrimSpace(string(yaml)))
	})

	t.Run("Valid Helm source with Helm values file with multiple aliases", func(t *testing.T) {
		expected := `
foo.image.name: nginx
foo.image.tag: v1.0.0
bar.image.name: nginx
bar.image.tag: v1.0.0
bbb.image.name: nginx
bbb.image.tag: v1.0.0
replicas: 1
`
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Sources: []v1alpha1.ApplicationSource{
					{
						Chart: "my-app",
						Helm: &v1alpha1.ApplicationSourceHelm{
							ReleaseName: "my-app",
							ValueFiles:  []string{"$values/some/dir/values.yaml"},
							Parameters: []v1alpha1.HelmParameter{
								{
									Name:        "foo.image.name",
									Value:       "nginx",
									ForceString: true,
								},
								{
									Name:        "foo.image.tag",
									Value:       "v1.0.0",
									ForceString: true,
								},
								{
									Name:        "bar.image.name",
									Value:       "nginx",
									ForceString: true,
								},
								{
									Name:        "bar.image.tag",
									Value:       "v1.0.0",
									ForceString: true,
								},
								{
									Name:        "bbb.image.name",
									Value:       "nginx",
									ForceString: true,
								},
								{
									Name:        "bbb.image.tag",
									Value:       "v1.0.0",
									ForceString: true,
								},
							},
						},
						RepoURL:        "https://example.com/example",
						TargetRevision: "main",
					},
					{
						Ref:            "values",
						RepoURL:        "https://example.com/example2",
						TargetRevision: "main",
					},
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceTypes: []v1alpha1.ApplicationSourceType{
					v1alpha1.ApplicationSourceTypeHelm,
					"",
				},
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{
						"nginx:v0.0.0",
					},
				},
			},
		}

		originalData := []byte(`
foo.image.name: nginx
foo.image.tag: v0.0.0
bar.image.name: nginx
bar.image.tag: v0.0.0
bbb.image.name: nginx
bbb.image.tag: v0.0.0
replicas: 1
`)
		imFoo := NewImage(
			image.NewFromIdentifier("foo=nginx"))
		imFoo.HelmImageName = "foo.image.name"
		imFoo.HelmImageTag = "foo.image.tag"
		imBar := NewImage(
			image.NewFromIdentifier("bar=nginx"))
		imBar.HelmImageName = "bar.image.name"
		imBar.HelmImageTag = "bar.image.tag"
		imBbb := NewImage(
			image.NewFromIdentifier("bbb=nginx"))
		imBbb.HelmImageName = "bbb.image.name"
		imBbb.HelmImageTag = "bbb.image.tag"
		applicationImages := &ApplicationImages{
			Application: app,
			Images:      ImageList{imFoo, imBar, imBbb},
			WriteBackConfig: &WriteBackConfig{
				Target: "./test-values.yaml",
			},
		}

		yaml, err := marshalParamsOverride(context.Background(), applicationImages, originalData)
		require.NoError(t, err)
		assert.NotEmpty(t, yaml)
		assert.Equal(t, strings.TrimSpace(strings.ReplaceAll(expected, "\t", "  ")), strings.TrimSpace(string(yaml)))
	})

	t.Run("Failed to setValue image parameter name", func(t *testing.T) {
		expected := `
test-value1: one
image:
  name: nginx
  tag: v1.0.0
replicas: 1
`

		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "https://example.com/example",
					TargetRevision: "main",
					Helm: &v1alpha1.ApplicationSourceHelm{
						Parameters: []v1alpha1.HelmParameter{
							{
								Name:        "image.name",
								Value:       "nginx",
								ForceString: true,
							},
							{
								Name:        "image.tag",
								Value:       "v1.0.0",
								ForceString: true,
							},
						},
					},
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeHelm,
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{
						"nginx:v0.0.0",
					},
				},
			},
		}

		originalData := []byte(`
test-value1: one
image:
  name: nginx
replicas: 1
`)

		im := NewImage(
			image.NewFromIdentifier("nginx"))
		im.HelmImageName = "image.name"
		im.HelmImageTag = "image.tag"
		applicationImages := &ApplicationImages{
			Application: app,
			Images:      ImageList{im},
			WriteBackConfig: &WriteBackConfig{
				Target: "./test-values.yaml",
			},
		}

		yaml, err := marshalParamsOverride(context.Background(), applicationImages, originalData)
		require.NoError(t, err)
		assert.NotEmpty(t, yaml)
		assert.Equal(t, strings.TrimSpace(strings.ReplaceAll(expected, "\t", "  ")), strings.TrimSpace(string(yaml)))
	})

	t.Run("Failed to setValue image parameter version", func(t *testing.T) {
		expected := `
image:
  tag: v1.0.0
  name: nginx
replicas: 1
`
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "https://example.com/example",
					TargetRevision: "main",
					Helm: &v1alpha1.ApplicationSourceHelm{
						Parameters: []v1alpha1.HelmParameter{
							{
								Name:        "image.name",
								Value:       "nginx",
								ForceString: true,
							},
							{
								Name:        "image.tag",
								Value:       "v1.0.0",
								ForceString: true,
							},
						},
					},
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeHelm,
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{
						"nginx:v0.0.0",
					},
				},
			},
		}

		originalData := []byte(`
image:
  tag: v0.0.0
replicas: 1
`)

		im := NewImage(
			image.NewFromIdentifier("nginx"))
		im.HelmImageName = "image.name"
		im.HelmImageTag = "image.tag"
		applicationImages := &ApplicationImages{
			Application: app,
			Images:      ImageList{im},
			WriteBackConfig: &WriteBackConfig{
				Target: "./test-values.yaml",
			},
		}

		yaml, err := marshalParamsOverride(context.Background(), applicationImages, originalData)

		require.NoError(t, err)
		assert.NotEmpty(t, yaml)
		assert.Equal(t, strings.TrimSpace(strings.ReplaceAll(expected, "\t", "  ")), strings.TrimSpace(string(yaml)))
	})

	t.Run("Missing image-tag for helmvalues write-back-target", func(t *testing.T) {
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "https://example.com/example",
					TargetRevision: "main",
					Helm: &v1alpha1.ApplicationSourceHelm{
						Parameters: []v1alpha1.HelmParameter{
							{
								Name:        "dockerimage.name",
								Value:       "nginx",
								ForceString: true,
							},
							{
								Name:        "dockerimage.tag",
								Value:       "v1.0.0",
								ForceString: true,
							},
						},
					},
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeHelm,
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{
						"nginx:v0.0.0",
					},
				},
			},
		}

		originalData := []byte(`random: yaml`)
		im := NewImage(
			image.NewFromIdentifier("nginx"))
		im.HelmImageName = "image.name"
		applicationImages := &ApplicationImages{
			Application: app,
			Images:      ImageList{im},
			WriteBackConfig: &WriteBackConfig{
				Target: "./test-values.yaml",
			},
		}

		_, err := marshalParamsOverride(context.Background(), applicationImages, originalData)
		assert.Error(t, err)
		assert.Equal(t, "could not find an image-tag for image nginx", err.Error())
	})

	t.Run("Missing image-name for helmvalues write-back-target", func(t *testing.T) {
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "https://example.com/example",
					TargetRevision: "main",
					Helm: &v1alpha1.ApplicationSourceHelm{
						Parameters: []v1alpha1.HelmParameter{
							{
								Name:        "image.name",
								Value:       "nginx",
								ForceString: true,
							},
							{
								Name:        "image.tag",
								Value:       "v1.0.0",
								ForceString: true,
							},
						},
					},
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeHelm,
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{
						"nginx:v0.0.0",
					},
				},
			},
		}

		originalData := []byte(`random: yaml`)
		im := NewImage(
			image.NewFromIdentifier("nginx"))
		im.HelmImageTag = "image.tag"
		applicationImages := &ApplicationImages{
			Application: app,
			Images:      ImageList{im},
			WriteBackConfig: &WriteBackConfig{
				Target: "./test-values.yaml",
			},
		}

		_, err := marshalParamsOverride(context.Background(), applicationImages, originalData)
		assert.Error(t, err)
		assert.Equal(t, "could not find an image-name for image nginx", err.Error())
	})

	t.Run("Image-name value not found in Helm source parameters list", func(t *testing.T) {
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "https://example.com/example",
					TargetRevision: "main",
					Helm: &v1alpha1.ApplicationSourceHelm{
						Parameters: []v1alpha1.HelmParameter{
							{
								Name:        "image.name",
								Value:       "nginx",
								ForceString: true,
							},
							{
								Name:        "image.tag",
								Value:       "v1.0.0",
								ForceString: true,
							},
						},
					},
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeHelm,
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{
						"nginx:v0.0.0",
					},
				},
			},
		}

		originalData := []byte(`random: yaml`)
		im := NewImage(
			image.NewFromIdentifier("nginx"))
		im.HelmImageName = "wrongimage.name"
		im.HelmImageTag = "image.tag"
		applicationImages := &ApplicationImages{
			Application: app,
			Images:      ImageList{im},
			WriteBackConfig: &WriteBackConfig{
				Target: "./test-values.yaml",
			},
		}

		_, err := marshalParamsOverride(context.Background(), applicationImages, originalData)
		assert.Error(t, err)
	})

	t.Run("Image-tag value not found in Helm source parameters list", func(t *testing.T) {
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "https://example.com/example",
					TargetRevision: "main",
					Helm: &v1alpha1.ApplicationSourceHelm{
						Parameters: []v1alpha1.HelmParameter{
							{
								Name:        "image.name",
								Value:       "nginx",
								ForceString: true,
							},
							{
								Name:        "image.tag",
								Value:       "v1.0.0",
								ForceString: true,
							},
						},
					},
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeHelm,
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{
						"nginx:v0.0.0",
					},
				},
			},
		}

		originalData := []byte(`random: yaml`)
		im := NewImage(
			image.NewFromIdentifier("nginx"))
		im.HelmImageName = "image.name"
		im.HelmImageTag = "wrongimage.tag"
		applicationImages := &ApplicationImages{
			Application: app,
			Images:      ImageList{im},
			WriteBackConfig: &WriteBackConfig{
				Target: "./test-values.yaml",
			},
		}

		_, err := marshalParamsOverride(context.Background(), applicationImages, originalData)
		assert.Error(t, err)
		assert.Equal(t, "wrongimage.tag parameter not found", err.Error())
	})

	t.Run("Invalid parameters merge for Helm source with Helm values file", func(t *testing.T) {
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "https://example.com/example",
					TargetRevision: "main",
					Helm: &v1alpha1.ApplicationSourceHelm{
						Parameters: []v1alpha1.HelmParameter{
							{
								Name:        "image.name",
								Value:       "nginx",
								ForceString: true,
							},
							{
								Name:        "image.tag",
								Value:       "v1.0.0",
								ForceString: true,
							},
						},
					},
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeHelm,
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{
						"nginx:v0.0.0",
					},
				},
			},
		}

		originalData := []byte(`random content`)
		im := NewImage(
			image.NewFromIdentifier("nginx"))
		im.HelmImageName = "image.name"
		im.HelmImageTag = "image.tag"
		applicationImages := &ApplicationImages{
			Application: app,
			Images:      ImageList{im},
			WriteBackConfig: &WriteBackConfig{
				Method: WriteBackGit,
				Target: "./test-values.yaml",
			},
		}

		_, err := marshalParamsOverride(context.Background(), applicationImages, originalData)
		assert.Error(t, err)
	})

	t.Run("Nil source merge for Helm source with Helm values file", func(t *testing.T) {
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "https://example.com/example",
					TargetRevision: "main",
					Helm: &v1alpha1.ApplicationSourceHelm{
						Parameters: []v1alpha1.HelmParameter{
							{
								Name:        "image.name",
								Value:       "nginx",
								ForceString: true,
							},
							{
								Name:        "image.tag",
								Value:       "v1.0.0",
								ForceString: true,
							},
						},
					},
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeHelm,
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{
						"nginx:v0.0.0",
					},
				},
			},
		}
		im := NewImage(
			image.NewFromIdentifier("nginx"))
		im.HelmImageName = "image.name"
		im.HelmImageTag = "image.tag"
		applicationImages := &ApplicationImages{
			Application: app,
			Images:      ImageList{im},
			WriteBackConfig: &WriteBackConfig{
				Method: WriteBackGit,
				Target: "./test-values.yaml",
			},
		}
		_, err := marshalParamsOverride(context.Background(), applicationImages, nil)
		assert.NoError(t, err)
	})

	t.Run("Unknown source", func(t *testing.T) {
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "https://example.com/example",
					TargetRevision: "main",
					Kustomize: &v1alpha1.ApplicationSourceKustomize{
						Images: v1alpha1.KustomizeImages{
							"foo",
							"bar",
						},
					},
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeDirectory,
			},
		}

		applicationImages := &ApplicationImages{
			Application: app,
			Images:      ImageList{},
		}

		_, err := marshalParamsOverride(context.Background(), applicationImages, nil)
		assert.Error(t, err)
	})

	t.Run("Whitespace only values file from helm source does not cause error", func(t *testing.T) {
		expected := `
# auto generated by argocd image updater

nginx:
  image:
    tag: v1.0.0
    name: nginx
`
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
				Annotations: map[string]string{
					"argocd-image-updater.argoproj.io/image-list":            "nginx=nginx, redis=redis",
					"argocd-image-updater.argoproj.io/write-back-method":     "git",
					"argocd-image-updater.argoproj.io/write-back-target":     "helmvalues:./test-values.yaml",
					"argocd-image-updater.argoproj.io/nginx.helm.image-name": "nginx.image.name",
					"argocd-image-updater.argoproj.io/nginx.helm.image-tag":  "nginx.image.tag",
				},
			},
			Spec: v1alpha1.ApplicationSpec{
				Sources: []v1alpha1.ApplicationSource{
					{
						Chart: "my-app",
						Helm: &v1alpha1.ApplicationSourceHelm{
							ReleaseName: "my-app",
							ValueFiles:  []string{"$values/some/dir/values.yaml"},
							Parameters: []v1alpha1.HelmParameter{
								{
									Name:        "nginx.image.name",
									Value:       "nginx",
									ForceString: true,
								},
								{
									Name:        "nginx.image.tag",
									Value:       "v1.0.0",
									ForceString: true,
								},
							},
						},
						RepoURL:        "https://example.com/example",
						TargetRevision: "main",
					},
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceTypes: []v1alpha1.ApplicationSourceType{
					v1alpha1.ApplicationSourceTypeHelm,
					"",
				},
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{
						"nginx:v0.0.0",
					},
				},
			},
		}

		originalData := []byte(`
`)
		im := NewImage(image.NewFromIdentifier("nginx"))
		im.ImageAlias = "nginx"
		im.HelmImageName = "nginx.image.name"
		im.HelmImageTag = "nginx.image.tag"

		applicationImages := &ApplicationImages{
			Application: app,
			Images:      ImageList{im},
			WriteBackConfig: &WriteBackConfig{
				Target: "./test-values.yaml",
			},
		}
		yaml, err := marshalParamsOverride(context.Background(), applicationImages, originalData)
		require.NoError(t, err)
		assert.NotEmpty(t, yaml)
		assert.Equal(t, strings.TrimSpace(strings.ReplaceAll(expected, "\t", "  ")), strings.TrimSpace(string(yaml)))
	})

	t.Run("Original data with short form image name and long form in new - should convert to short form", func(t *testing.T) {
		expected := `
image:
  registry: docker.io
  name: bitnami/nginx
  tag: v1.0.0
replicas: 1
`
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "https://example.com/example",
					TargetRevision: "main",
					Helm: &v1alpha1.ApplicationSourceHelm{
						Parameters: []v1alpha1.HelmParameter{
							{
								Name:        "image.name",
								Value:       "docker.io/bitnami/nginx",
								ForceString: true,
							},
							{
								Name:        "image.tag",
								Value:       "v1.0.0",
								ForceString: true,
							},
						},
					},
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeHelm,
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{
						"nginx:v0.0.0",
					},
				},
			},
		}

		// Original has short form - should convert long form to short form
		originalData := []byte(`
image:
  registry: docker.io
  name: bitnami/nginx
  tag: v0.0.0
replicas: 1
`)
		im := NewImage(image.NewFromIdentifier("nginx"))
		im.ImageAlias = "nginx"
		im.HelmImageName = "image.name"
		im.HelmImageTag = "image.tag"
		applicationImages := &ApplicationImages{
			Application: app,
			Images:      ImageList{im},
			WriteBackConfig: &WriteBackConfig{
				Method: WriteBackGit,
				Target: "./test-values.yaml",
			},
		}

		yamlOutput, err := marshalParamsOverride(context.Background(), applicationImages, originalData)
		require.NoError(t, err)
		assert.NotEmpty(t, yamlOutput)
		assert.Equal(t, strings.TrimSpace(expected), strings.TrimSpace(string(yamlOutput)))
	})

	t.Run("Original data with long form image name - should use long form", func(t *testing.T) {
		expected := `
image:
  name: docker.io/bitnami/nginx
  tag: v1.0.0
replicas: 1
`
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "https://example.com/example",
					TargetRevision: "main",
					Helm: &v1alpha1.ApplicationSourceHelm{
						Parameters: []v1alpha1.HelmParameter{
							{
								Name:        "image.name",
								Value:       "docker.io/bitnami/nginx",
								ForceString: true,
							},
							{
								Name:        "image.tag",
								Value:       "v1.0.0",
								ForceString: true,
							},
						},
					},
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeHelm,
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{
						"nginx:v0.0.0",
					},
				},
			},
		}

		// Original has long form - should keep long form
		originalData := []byte(`
image:
  name: docker.io/bitnami/nginx
  tag: v0.0.0
replicas: 1
`)
		im := NewImage(image.NewFromIdentifier("nginx"))
		im.ImageAlias = "nginx"
		im.HelmImageName = "image.name"
		im.HelmImageTag = "image.tag"
		applicationImages := &ApplicationImages{
			Application: app,
			Images:      ImageList{im},
			WriteBackConfig: &WriteBackConfig{
				Method: WriteBackGit,
				Target: "./test-values.yaml",
			},
		}

		yamlOutput, err := marshalParamsOverride(context.Background(), applicationImages, originalData)
		require.NoError(t, err)
		assert.NotEmpty(t, yamlOutput)
		assert.Equal(t, strings.TrimSpace(expected), strings.TrimSpace(string(yamlOutput)))
	})

	t.Run("Non-empty original data with short form in original and short form in new - should use short form", func(t *testing.T) {
		expected := `
image:
  registry: docker.io
  name: bitnami/nginx
  tag: v1.0.0
replicas: 1
`
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "https://example.com/example",
					TargetRevision: "main",
					Helm: &v1alpha1.ApplicationSourceHelm{
						Parameters: []v1alpha1.HelmParameter{
							{
								Name:        "image.name",
								Value:       "bitnami/nginx",
								ForceString: true,
							},
							{
								Name:        "image.tag",
								Value:       "v1.0.0",
								ForceString: true,
							},
						},
					},
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeHelm,
				Summary: v1alpha1.ApplicationSummary{
					Images: []string{
						"nginx:v0.0.0",
					},
				},
			},
		}

		// Original has short form and new has short form - should use short form
		originalData := []byte(`
image:
  registry: docker.io
  name: bitnami/nginx
  tag: v0.0.0
replicas: 1
`)
		im := NewImage(image.NewFromIdentifier("nginx"))
		im.ImageAlias = "nginx"
		im.HelmImageName = "image.name"
		im.HelmImageTag = "image.tag"
		applicationImages := &ApplicationImages{
			Application: app,
			Images:      ImageList{im},
			WriteBackConfig: &WriteBackConfig{
				Method: WriteBackGit,
				Target: "./test-values.yaml",
			},
		}

		yamlOutput, err := marshalParamsOverride(context.Background(), applicationImages, originalData)
		require.NoError(t, err)
		assert.NotEmpty(t, yamlOutput)
		assert.Equal(t, strings.TrimSpace(expected), strings.TrimSpace(string(yamlOutput)))
	})
}

func Test_GetHelmValue(t *testing.T) {
	t.Run("Get nested path value", func(t *testing.T) {
		inputData := []byte(`
image:
  attributes:
    name: repo-name
    tag: v1.0.0
`)
		input := yaml.Node{}
		err := yaml.Unmarshal(inputData, &input)
		require.NoError(t, err)

		value, err := getHelmValue(&input, "image.attributes.tag")
		require.NoError(t, err)
		assert.Equal(t, "v1.0.0", value)
	})

	t.Run("Get literal key with dots", func(t *testing.T) {
		inputData := []byte(`image.attributes.tag: v1.0.0`)
		input := yaml.Node{}
		err := yaml.Unmarshal(inputData, &input)
		require.NoError(t, err)

		value, err := getHelmValue(&input, "image.attributes.tag")
		require.NoError(t, err)
		assert.Equal(t, "v1.0.0", value)
	})

	t.Run("Get root level key", func(t *testing.T) {
		inputData := []byte(`
name: repo-name
tag: v1.0.0
`)
		input := yaml.Node{}
		err := yaml.Unmarshal(inputData, &input)
		require.NoError(t, err)

		value, err := getHelmValue(&input, "name")
		require.NoError(t, err)
		assert.Equal(t, "repo-name", value)
	})

	t.Run("Get nested path when literal key also exists", func(t *testing.T) {
		inputData := []byte(`
image:
  attributes:
    tag: nested-value
image.attributes.tag: literal-value
`)
		input := yaml.Node{}
		err := yaml.Unmarshal(inputData, &input)
		require.NoError(t, err)

		// Should prefer nested path
		value, err := getHelmValue(&input, "image.attributes.tag")
		require.NoError(t, err)
		assert.Equal(t, "nested-value", value)
	})

	t.Run("Get literal key when nested path doesn't exist", func(t *testing.T) {
		inputData := []byte(`
image.attributes.tag: literal-value
other:
  field: value
`)
		input := yaml.Node{}
		err := yaml.Unmarshal(inputData, &input)
		require.NoError(t, err)

		value, err := getHelmValue(&input, "image.attributes.tag")
		require.NoError(t, err)
		assert.Equal(t, "literal-value", value)
	})

	t.Run("Get value from alias node", func(t *testing.T) {
		inputData := []byte(`
image:
  attributes: &attrs
    name: repo-name
    tag: v1.0.0
other:
  attributes: *attrs
`)
		input := yaml.Node{}
		err := yaml.Unmarshal(inputData, &input)
		require.NoError(t, err)

		value, err := getHelmValue(&input, "other.attributes.tag")
		require.NoError(t, err)
		assert.Equal(t, "v1.0.0", value)
	})

	t.Run("Get value from array with index", func(t *testing.T) {
		inputData := []byte(`
images:
  - name: image1
    tag: v1.0.0
  - name: image2
    tag: v2.0.0
`)
		input := yaml.Node{}
		err := yaml.Unmarshal(inputData, &input)
		require.NoError(t, err)

		value, err := getHelmValue(&input, "images[1].tag")
		require.NoError(t, err)
		assert.Equal(t, "v2.0.0", value)
	})

	t.Run("Key not found returns error", func(t *testing.T) {
		inputData := []byte(`
image:
  attributes:
    name: repo-name
`)
		input := yaml.Node{}
		err := yaml.Unmarshal(inputData, &input)
		require.NoError(t, err)

		_, err = getHelmValue(&input, "image.attributes.tag")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("Empty document node returns error", func(t *testing.T) {
		input := yaml.Node{
			Kind: yaml.DocumentNode,
		}

		_, err := getHelmValue(&input, "key")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty document node")
	})

	t.Run("Invalid root type returns error", func(t *testing.T) {
		input := yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: "not-a-map",
		}

		_, err := getHelmValue(&input, "key")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unexpected type")
	})

	t.Run("Literal key found but not scalar returns error", func(t *testing.T) {
		inputData := []byte(`
image.attributes.tag:
  nested: value
`)
		input := yaml.Node{}
		err := yaml.Unmarshal(inputData, &input)
		require.NoError(t, err)

		_, err = getHelmValue(&input, "image.attributes.tag")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "not a scalar value")
	})

	t.Run("Deep nested path", func(t *testing.T) {
		inputData := []byte(`
level1:
  level2:
    level3:
      level4:
        value: deep-value
`)
		input := yaml.Node{}
		err := yaml.Unmarshal(inputData, &input)
		require.NoError(t, err)

		value, err := getHelmValue(&input, "level1.level2.level3.level4.value")
		require.NoError(t, err)
		assert.Equal(t, "deep-value", value)
	})

	t.Run("Nested path with non-scalar final value falls back to literal", func(t *testing.T) {
		inputData := []byte(`
image:
  attributes: value-not-nested
image.attributes.tag: literal-value
`)
		input := yaml.Node{}
		err := yaml.Unmarshal(inputData, &input)
		require.NoError(t, err)

		// When image.attributes is a scalar (not a map), nested path fails
		// Should fall back to literal key if it exists
		value, err := getHelmValue(&input, "image.attributes.tag")
		require.NoError(t, err)
		assert.Equal(t, "literal-value", value)
	})
}

func Test_SetHelmValue(t *testing.T) {
	t.Run("Update existing Key", func(t *testing.T) {
		expected := `
image:
  attributes:
    name: repo-name
    tag: v2.0.0
`

		inputData := []byte(`
image:
  attributes:
    name: repo-name
    tag: v1.0.0
`)
		input := yaml.Node{}
		err := yaml.Unmarshal(inputData, &input)
		require.NoError(t, err)

		key := "image.attributes.tag"
		value := "v2.0.0"

		err = setHelmValue(&input, key, value)
		require.NoError(t, err)

		output, err := marshalWithIndent(&input, defaultIndent)
		require.NoError(t, err)
		assert.Equal(t, strings.TrimSpace(expected), strings.TrimSpace(string(output)))
	})

	t.Run("Update Key with dots", func(t *testing.T) {
		expected := `image.attributes.tag: v2.0.0`

		inputData := []byte(`image.attributes.tag: v1.0.0`)
		input := yaml.Node{}
		err := yaml.Unmarshal(inputData, &input)
		require.NoError(t, err)

		key := "image.attributes.tag"
		value := "v2.0.0"

		err = setHelmValue(&input, key, value)
		require.NoError(t, err)

		output, err := marshalWithIndent(&input, defaultIndent)
		require.NoError(t, err)
		assert.Equal(t, strings.TrimSpace(expected), strings.TrimSpace(string(output)))
	})

	t.Run("Key not found", func(t *testing.T) {
		expected := `
image:
  attributes:
    name: repo-name
    tag: v2.0.0
`

		inputData := []byte(`
image:
  attributes:
    name: repo-name
`)
		input := yaml.Node{}
		err := yaml.Unmarshal(inputData, &input)
		require.NoError(t, err)

		key := "image.attributes.tag"
		value := "v2.0.0"

		err = setHelmValue(&input, key, value)
		require.NoError(t, err)

		output, err := marshalWithIndent(&input, defaultIndent)
		require.NoError(t, err)
		assert.Equal(t, strings.TrimSpace(expected), strings.TrimSpace(string(output)))
	})

	t.Run("Root key not found", func(t *testing.T) {
		expected := `
name: repo-name
tag: v2.0.0
`

		inputData := []byte(`name: repo-name`)
		input := yaml.Node{}
		err := yaml.Unmarshal(inputData, &input)
		require.NoError(t, err)

		key := "tag"
		value := "v2.0.0"

		err = setHelmValue(&input, key, value)
		require.NoError(t, err)

		output, err := marshalWithIndent(&input, defaultIndent)
		require.NoError(t, err)
		assert.Equal(t, strings.TrimSpace(expected), strings.TrimSpace(string(output)))
	})

	t.Run("Empty values with deep key", func(t *testing.T) {
		// this uses inline syntax because the input data
		// needed is an empty map, which can only be expressed as {}.
		expected := `{image: {attributes: {tag: v2.0.0}}}`

		inputData := []byte(`{}`)
		input := yaml.Node{}
		err := yaml.Unmarshal(inputData, &input)
		require.NoError(t, err)

		key := "image.attributes.tag"
		value := "v2.0.0"

		err = setHelmValue(&input, key, value)
		require.NoError(t, err)

		output, err := marshalWithIndent(&input, defaultIndent)
		require.NoError(t, err)
		assert.Equal(t, strings.TrimSpace(expected), strings.TrimSpace(string(output)))
	})

	t.Run("Unexpected type for key", func(t *testing.T) {
		inputData := []byte(`
image:
  attributes: v1.0.0
`)
		input := yaml.Node{}
		err := yaml.Unmarshal(inputData, &input)
		require.NoError(t, err)

		key := "image.attributes.tag"
		value := "v2.0.0"

		err = setHelmValue(&input, key, value)
		assert.Error(t, err)
		assert.Equal(t, "unexpected type ScalarNode for key attributes", err.Error())
	})

	t.Run("Aliases, comments, and multiline strings are preserved", func(t *testing.T) {
		expected := `
image:
  attributes:
    name: &repo repo-name
    tag: v2.0.0
    # this is a comment
    multiline: |
      one
      two
      three
    alias: *repo
`

		inputData := []byte(`
image:
  attributes:
    name: &repo repo-name
    tag: v1.0.0
    # this is a comment
    multiline: |
      one
      two
      three
    alias: *repo
`)
		input := yaml.Node{}
		err := yaml.Unmarshal(inputData, &input)
		require.NoError(t, err)

		key := "image.attributes.tag"
		value := "v2.0.0"

		err = setHelmValue(&input, key, value)
		require.NoError(t, err)

		output, err := marshalWithIndent(&input, defaultIndent)
		require.NoError(t, err)
		assert.Equal(t, strings.TrimSpace(expected), strings.TrimSpace(string(output)))
	})

	t.Run("Aliases to mappings are followed", func(t *testing.T) {
		expected := `
global:
  attributes: &attrs
    name: &repo repo-name
    tag: v2.0.0
image:
  attributes: *attrs
`

		inputData := []byte(`
global:
  attributes: &attrs
    name: &repo repo-name
    tag: v1.0.0
image:
  attributes: *attrs
`)
		input := yaml.Node{}
		err := yaml.Unmarshal(inputData, &input)
		require.NoError(t, err)

		key := "image.attributes.tag"
		value := "v2.0.0"

		err = setHelmValue(&input, key, value)
		require.NoError(t, err)

		output, err := marshalWithIndent(&input, defaultIndent)
		require.NoError(t, err)
		assert.Equal(t, strings.TrimSpace(expected), strings.TrimSpace(string(output)))
	})

	t.Run("Aliases to scalars are followed", func(t *testing.T) {
		expected := `
image:
  attributes:
    name: repo-name
    version: &ver v2.0.0
    tag: *ver
`

		inputData := []byte(`
image:
  attributes:
    name: repo-name
    version: &ver v1.0.0
    tag: *ver
`)
		input := yaml.Node{}
		err := yaml.Unmarshal(inputData, &input)
		require.NoError(t, err)

		key := "image.attributes.tag"
		value := "v2.0.0"

		err = setHelmValue(&input, key, value)
		require.NoError(t, err)

		output, err := marshalWithIndent(&input, defaultIndent)
		require.NoError(t, err)
		assert.Equal(t, strings.TrimSpace(expected), strings.TrimSpace(string(output)))
	})

	t.Run("yaml list is correctly parsed", func(t *testing.T) {
		expected := `
images:
- name: image-1
  attributes:
    name: repo-name
    tag: 2.0.0
`

		inputData := []byte(`
images:
- name: image-1
  attributes:
    name: repo-name
    tag: 1.0.0
`)
		input := yaml.Node{}
		err := yaml.Unmarshal(inputData, &input)
		require.NoError(t, err)

		key := "images[0].attributes.tag"
		value := "2.0.0"

		err = setHelmValue(&input, key, value)
		require.NoError(t, err)

		output, err := marshalWithIndent(&input, defaultIndent)
		require.NoError(t, err)
		assert.Equal(t, strings.TrimSpace(expected), strings.TrimSpace(string(output)))
	})

	t.Run("yaml list is correctly parsed when multiple values", func(t *testing.T) {
		expected := `
images:
- name: image-1
  attributes:
    name: repo-name
    tag: 1.0.0
- name: image-2
  attributes:
    name: repo-name
    tag: 2.0.0
`

		inputData := []byte(`
images:
- name: image-1
  attributes:
    name: repo-name
    tag: 1.0.0
- name: image-2
  attributes:
    name: repo-name
    tag: 1.0.0
`)
		input := yaml.Node{}
		err := yaml.Unmarshal(inputData, &input)
		require.NoError(t, err)

		key := "images[1].attributes.tag"
		value := "2.0.0"

		err = setHelmValue(&input, key, value)
		require.NoError(t, err)

		output, err := marshalWithIndent(&input, defaultIndent)
		require.NoError(t, err)
		assert.Equal(t, strings.TrimSpace(expected), strings.TrimSpace(string(output)))
	})

	t.Run("yaml list is correctly parsed when inside map", func(t *testing.T) {
		expected := `
extraContainers:
  images:
  - name: image-1
    attributes:
      name: repo-name
      tag: 2.0.0
`

		inputData := []byte(`
extraContainers:
  images:
  - name: image-1
    attributes:
      name: repo-name
      tag: 1.0.0
`)
		input := yaml.Node{}
		err := yaml.Unmarshal(inputData, &input)
		require.NoError(t, err)

		key := "extraContainers.images[0].attributes.tag"
		value := "2.0.0"

		err = setHelmValue(&input, key, value)
		require.NoError(t, err)

		output, err := marshalWithIndent(&input, defaultIndent)
		require.NoError(t, err)
		assert.Equal(t, strings.TrimSpace(expected), strings.TrimSpace(string(output)))
	})

	t.Run("yaml list is correctly parsed when list name contains digits", func(t *testing.T) {
		expected := `
extraContainers:
  images123:
  - name: image-1
    attributes:
      name: repo-name
      tag: 2.0.0
`

		inputData := []byte(`
extraContainers:
  images123:
  - name: image-1
    attributes:
      name: repo-name
      tag: 1.0.0
`)
		input := yaml.Node{}
		err := yaml.Unmarshal(inputData, &input)
		require.NoError(t, err)

		key := "extraContainers.images123[0].attributes.tag"
		value := "2.0.0"

		err = setHelmValue(&input, key, value)
		require.NoError(t, err)

		output, err := marshalWithIndent(&input, defaultIndent)
		require.NoError(t, err)
		assert.Equal(t, strings.TrimSpace(expected), strings.TrimSpace(string(output)))
	})

	t.Run("id for yaml list is lower than 0", func(t *testing.T) {
		inputData := []byte(`
images:
- name: image-1
  attributes:
    name: repo-name
    tag: 1.0.0
`)
		input := yaml.Node{}
		err := yaml.Unmarshal(inputData, &input)
		require.NoError(t, err)

		key := "images[-1].attributes.tag"
		value := "2.0.0"

		err = setHelmValue(&input, key, value)

		require.Error(t, err)
		assert.Equal(t, "id -1 is out of range [0, 1)", err.Error())
	})

	t.Run("id for yaml list is greater than length of list", func(t *testing.T) {
		inputData := []byte(`
images:
- name: image-1
  attributes:
    name: repo-name
    tag: 1.0.0
`)
		input := yaml.Node{}
		err := yaml.Unmarshal(inputData, &input)
		require.NoError(t, err)

		key := "images[1].attributes.tag"
		value := "2.0.0"

		err = setHelmValue(&input, key, value)

		require.Error(t, err)
		assert.Equal(t, "id 1 is out of range [0, 1)", err.Error())
	})

	t.Run("id for YAML list is not a valid integer", func(t *testing.T) {
		inputData := []byte(`
images:
- name: image-1
  attributes:
    name: repo-name
    tag: 1.0.0
`)
		input := yaml.Node{}
		err := yaml.Unmarshal(inputData, &input)
		require.NoError(t, err)

		key := "images[invalid].attributes.tag"
		value := "2.0.0"

		err = setHelmValue(&input, key, value)

		require.Error(t, err)
		assert.Equal(t, "id \"invalid\" in yaml array must match pattern ^(.*)\\[(.*)\\]$", err.Error())
	})

	t.Run("no id for yaml list given", func(t *testing.T) {
		inputData := []byte(`
images:
- name: image-1
  attributes:
    name: repo-name
    tag: 1.0.0
`)
		input := yaml.Node{}
		err := yaml.Unmarshal(inputData, &input)
		require.NoError(t, err)

		key := "images.attributes.tag"
		value := "2.0.0"

		err = setHelmValue(&input, key, value)

		require.Error(t, err)
		assert.Equal(t, "no id provided for yaml array \"images\"", err.Error())
	})

	t.Run("id given when node is not an yaml list", func(t *testing.T) {
		inputData := []byte(`
image:
  attributes:
    name: repo-name
    tag: 1.0.0
`)
		input := yaml.Node{}
		err := yaml.Unmarshal(inputData, &input)
		require.NoError(t, err)

		key := "image[0].attributes.tag"
		value := "2.0.0"

		err = setHelmValue(&input, key, value)

		require.Error(t, err)
		assert.Equal(t, "id 0 provided when \"image\" is not an yaml array", err.Error())
	})

	t.Run("invalid id given when node is not an yaml list", func(t *testing.T) {
		inputData := []byte(`
image:
  attributes:
    name: repo-name
    tag: 1.0.0
`)
		input := yaml.Node{}
		err := yaml.Unmarshal(inputData, &input)
		require.NoError(t, err)

		key := "image[invalid].attributes.tag"
		value := "2.0.0"

		err = setHelmValue(&input, key, value)

		require.Error(t, err)
		assert.Equal(t, "id \"invalid\" in yaml array must match pattern ^(.*)\\[(.*)\\]$", err.Error())
	})
}

func Test_GetWriteBackConfig(t *testing.T) {
	t.Run("Valid write-back config - git", func(t *testing.T) {
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "https://example.com/example",
					TargetRevision: "main",
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeKustomize,
			},
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)

		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeKubeClient(),
			},
		}

		// Create iuapi.WriteBackConfig that represents the same configuration as the annotations
		settings := &iuapi.WriteBackConfig{
			Method: stringPtr("git"),
			GitConfig: &iuapi.GitConfig{
				Branch: stringPtr("mybranch:mytargetbranch"),
			},
		}

		wbc, err := newWBCFromSettings(context.Background(), &app, &kubeClient, settings)
		require.NoError(t, err)
		require.NotNil(t, wbc)
		assert.Equal(t, wbc.Method, WriteBackGit)
		assert.Equal(t, "mybranch", wbc.GitBranch)
		assert.Equal(t, "mytargetbranch", wbc.GitWriteBranch)
	})

	t.Run("Valid git branch name determiniation - write branch only", func(t *testing.T) {
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "https://example.com/example",
					TargetRevision: "main",
				},
			},
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)

		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeKubeClient(),
			},
		}

		// Create iuapi.WriteBackConfig that represents the same configuration as the annotations
		settings := &iuapi.WriteBackConfig{
			Method: stringPtr("git"),
			GitConfig: &iuapi.GitConfig{
				Branch: stringPtr(":mytargetbranch"),
			},
		}

		wbc, err := newWBCFromSettings(context.Background(), &app, &kubeClient, settings)
		require.NoError(t, err)
		require.NotNil(t, wbc)
		assert.Equal(t, "", wbc.GitBranch)
		assert.Equal(t, "mytargetbranch", wbc.GitWriteBranch)
	})

	t.Run("Valid git branch name determiniation - base branch only", func(t *testing.T) {
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "https://example.com/example",
					TargetRevision: "main",
				},
			},
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)

		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeKubeClient(),
			},
		}

		// Create iuapi.WriteBackConfig that represents the same configuration as the annotations
		settings := &iuapi.WriteBackConfig{
			Method: stringPtr("git"),
			GitConfig: &iuapi.GitConfig{
				Branch: stringPtr("mybranch"),
			},
		}

		wbc, err := newWBCFromSettings(context.Background(), &app, &kubeClient, settings)
		require.NoError(t, err)
		require.NotNil(t, wbc)
		assert.Equal(t, "mybranch", wbc.GitBranch)
		assert.Equal(t, "", wbc.GitWriteBranch)
	})

	t.Run("Valid write-back config - argocd", func(t *testing.T) {
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "https://example.com/example",
					TargetRevision: "main",
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeKustomize,
			},
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)

		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeKubeClient(),
			},
		}

		// Create iuapi.WriteBackConfig that represents the same configuration as the annotations
		settings := &iuapi.WriteBackConfig{
			Method: stringPtr("argocd"),
		}

		wbc, err := newWBCFromSettings(context.Background(), &app, &kubeClient, settings)
		require.NoError(t, err)
		require.NotNil(t, wbc)
		assert.Equal(t, wbc.Method, WriteBackApplication)
	})

	t.Run("kustomization write-back config", func(t *testing.T) {
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "https://example.com/example",
					TargetRevision: "main",
					Path:           "config/foo",
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeKustomize,
			},
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)

		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeKubeClient(),
			},
		}

		// Create iuapi.WriteBackConfig that represents the same configuration as the annotations
		settings := &iuapi.WriteBackConfig{
			Method: stringPtr("git"),
			GitConfig: &iuapi.GitConfig{
				Branch:          stringPtr("mybranch:mytargetbranch"),
				WriteBackTarget: stringPtr("kustomization:../bar"),
			},
		}

		wbc, err := newWBCFromSettings(context.Background(), &app, &kubeClient, settings)
		require.NoError(t, err)
		require.NotNil(t, wbc)
		assert.Equal(t, wbc.Method, WriteBackGit)
		assert.Equal(t, wbc.KustomizeBase, "config/bar")
	})

	t.Run("helmvalues write-back config with relative path", func(t *testing.T) {
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "https://example.com/example",
					TargetRevision: "main",
					Path:           "config/foo",
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeHelm,
			},
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)

		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeKubeClient(),
			},
		}

		// Create iuapi.WriteBackConfig that represents the same configuration as the annotations
		settings := &iuapi.WriteBackConfig{
			Method: stringPtr("git"),
			GitConfig: &iuapi.GitConfig{
				Branch:          stringPtr("mybranch:mytargetbranch"),
				WriteBackTarget: stringPtr("helmvalues:../bar/values.yaml"),
			},
		}

		wbc, err := newWBCFromSettings(context.Background(), &app, &kubeClient, settings)
		require.NoError(t, err)
		require.NotNil(t, wbc)
		assert.Equal(t, wbc.Method, WriteBackGit)
		assert.Equal(t, wbc.Target, "config/bar/values.yaml")
	})

	t.Run("helmvalues write-back config without path", func(t *testing.T) {
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "https://example.com/example",
					TargetRevision: "main",
					Path:           "config/foo",
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeHelm,
			},
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)

		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeKubeClient(),
			},
		}

		// Create iuapi.WriteBackConfig that represents the same configuration as the annotations
		settings := &iuapi.WriteBackConfig{
			Method: stringPtr("git"),
			GitConfig: &iuapi.GitConfig{
				Branch:          stringPtr("mybranch:mytargetbranch"),
				WriteBackTarget: stringPtr("helmvalues"),
			},
		}

		wbc, err := newWBCFromSettings(context.Background(), &app, &kubeClient, settings)
		require.NoError(t, err)
		require.NotNil(t, wbc)
		assert.Equal(t, wbc.Method, WriteBackGit)
		assert.Equal(t, wbc.Target, "config/foo/values.yaml")
	})

	t.Run("helmvalues write-back config with absolute path", func(t *testing.T) {
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "https://example.com/example",
					TargetRevision: "main",
					Path:           "config/foo",
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeHelm,
			},
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)

		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeKubeClient(),
			},
		}

		// Create iuapi.WriteBackConfig that represents the same configuration as the annotations
		settings := &iuapi.WriteBackConfig{
			Method: stringPtr("git"),
			GitConfig: &iuapi.GitConfig{
				Branch:          stringPtr("mybranch:mytargetbranch"),
				WriteBackTarget: stringPtr("helmvalues:/helm/app/values.yaml"),
			},
		}

		wbc, err := newWBCFromSettings(context.Background(), &app, &kubeClient, settings)
		require.NoError(t, err)
		require.NotNil(t, wbc)
		assert.Equal(t, wbc.Method, WriteBackGit)
		assert.Equal(t, wbc.Target, "helm/app/values.yaml")
	})

	t.Run("Plain write back target without kustomize or helm types", func(t *testing.T) {
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "https://example.com/example",
					TargetRevision: "main",
					Path:           "config/foo",
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeHelm,
			},
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)

		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeKubeClient(),
			},
		}

		// Create iuapi.WriteBackConfig that represents the same configuration as the annotations
		settings := &iuapi.WriteBackConfig{
			Method: stringPtr("git"),
			GitConfig: &iuapi.GitConfig{
				Branch:          stringPtr("mybranch:mytargetbranch"),
				WriteBackTarget: stringPtr("target/folder/app-parameters.yaml"),
			},
		}

		wbc, err := newWBCFromSettings(context.Background(), &app, &kubeClient, settings)
		require.NoError(t, err)
		require.NotNil(t, wbc)
		assert.Equal(t, wbc.Method, WriteBackGit)
		assert.Equal(t, wbc.Target, "target/folder/app-parameters.yaml")
	})

	t.Run("Unknown credentials", func(t *testing.T) {
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "https://example.com/example",
					TargetRevision: "main",
					Path:           "config/foo",
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeHelm,
			},
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)

		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeKubeClient(),
			},
		}

		// Create iuapi.WriteBackConfig that represents the same configuration as the annotations
		settings := &iuapi.WriteBackConfig{
			Method: stringPtr("git:error:argocd-image-updater/git-creds"),
			GitConfig: &iuapi.GitConfig{
				Branch: stringPtr("mybranch:mytargetbranch"),
			},
		}

		_, err := newWBCFromSettings(context.Background(), &app, &kubeClient, settings)
		assert.Error(t, err)
	})

	t.Run("Default write-back config - argocd", func(t *testing.T) {
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "https://example.com/example",
					TargetRevision: "main",
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeKustomize,
			},
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)

		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeKubeClient(),
			},
		}

		// Create iuapi.WriteBackConfig that represents the same configuration as the annotations
		settings := &iuapi.WriteBackConfig{
			Method: stringPtr("argocd"),
			GitConfig: &iuapi.GitConfig{
				Branch: stringPtr("mybranch:mytargetbranch"),
			},
		}

		wbc, err := newWBCFromSettings(context.Background(), &app, &kubeClient, settings)
		require.NoError(t, err)
		require.NotNil(t, wbc)
		assert.Equal(t, wbc.Method, WriteBackApplication)
	})

	t.Run("Invalid write-back config - unknown", func(t *testing.T) {
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "https://example.com/example",
					TargetRevision: "main",
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeKustomize,
			},
		}

		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)

		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeKubeClient(),
			},
		}

		// Create iuapi.WriteBackConfig that represents the same configuration as the annotations
		settings := &iuapi.WriteBackConfig{
			Method: stringPtr("unknown"),
			GitConfig: &iuapi.GitConfig{
				Branch: stringPtr("mybranch:mytargetbranch"),
			},
		}

		wbc, err := newWBCFromSettings(context.Background(), &app, &kubeClient, settings)
		require.Error(t, err)
		require.Nil(t, wbc)
	})

}

func Test_GetGitCreds(t *testing.T) {
	t.Run("HTTP user creds from a secret", func(t *testing.T) {
		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)
		secret := fixture.NewSecret("argocd-image-updater", "git-creds", map[string][]byte{
			"username": []byte("foo"),
			"password": []byte("bar"),
		})
		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeClientsetWithResources(secret),
			},
		}
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "https://example.com/example",
					TargetRevision: "main",
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeKustomize,
			},
		}
		// Create iuapi.WriteBackConfig that represents the same configuration as the annotations
		settings := &iuapi.WriteBackConfig{
			Method: stringPtr("git:secret:argocd-image-updater/git-creds"),
			GitConfig: &iuapi.GitConfig{
				Branch: stringPtr("mybranch:mytargetbranch"),
			},
		}

		wbc, err := newWBCFromSettings(context.Background(), &app, &kubeClient, settings)
		require.NoError(t, err)

		creds, err := wbc.GetCreds(&app)
		require.NoError(t, err)
		require.NotNil(t, creds)
		// Must have HTTPS user creds
		_, ok := creds.(git.HTTPSCreds)
		require.True(t, ok)
	})

	t.Run("HTTP GitHub App creds from a secret", func(t *testing.T) {
		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)
		secret := fixture.NewSecret("argocd-image-updater", "git-creds", map[string][]byte{
			"githubAppID":             []byte("12345678"),
			"githubAppInstallationID": []byte("87654321"),
			"githubAppPrivateKey":     []byte("foo"),
		})
		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeClientsetWithResources(secret),
			},
		}
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "https://example.com/example",
					TargetRevision: "main",
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeKustomize,
			},
		}
		// Create iuapi.WriteBackConfig that represents the same configuration as the annotations
		settings := &iuapi.WriteBackConfig{
			Method: stringPtr("git:secret:argocd-image-updater/git-creds"),
			GitConfig: &iuapi.GitConfig{
				Branch: stringPtr("mybranch:mytargetbranch"),
			},
		}

		wbc, err := newWBCFromSettings(context.Background(), &app, &kubeClient, settings)
		require.NoError(t, err)

		creds, err := wbc.GetCreds(&app)
		require.NoError(t, err)
		require.NotNil(t, creds)
		// Must have HTTPS GitHub App creds
		_, ok := creds.(git.GitHubAppCreds)
		require.True(t, ok)

		// invalid secrete data in GitHub App creds
		invalidSecretEntries := []map[string][]byte{
			{ // missing githubAppPrivateKey
				"githubAppID":             []byte("12345678"),
				"githubAppInstallationID": []byte("87654321"),
			}, { // missing githubAppInstallationID
				"githubAppID":         []byte("12345678"),
				"githubAppPrivateKey": []byte("foo"),
			}, { // missing githubAppID
				"githubAppInstallationID": []byte("87654321"),
				"githubAppPrivateKey":     []byte("foo"),
			}, { // ID should be a number
				"githubAppID":             []byte("NaN"),
				"githubAppInstallationID": []byte("87654321"),
				"githubAppPrivateKey":     []byte("foo"),
			}, {
				"githubAppID":             []byte("12345678"),
				"githubAppInstallationID": []byte("NaN"),
				"githubAppPrivateKey":     []byte("foo"),
			},
		}
		for _, secretEntry := range invalidSecretEntries {
			secret = fixture.NewSecret("argocd-image-updater", "git-creds", secretEntry)
			kubeClient = kube.ImageUpdaterKubernetesClient{
				KubeClient: &registryKube.KubernetesClient{
					Clientset: fake.NewFakeClientsetWithResources(secret),
				},
			}
			// Create iuapi.WriteBackConfig that represents the same configuration as the annotations
			settings := &iuapi.WriteBackConfig{
				Method: stringPtr("git"),
				GitConfig: &iuapi.GitConfig{
					Branch: stringPtr("mybranch:mytargetbranch"),
				},
			}

			wbc, err = newWBCFromSettings(context.Background(), &app, &kubeClient, settings)
			require.NoError(t, err)
			_, err = wbc.GetCreds(&app)
			require.Error(t, err)
		}
	})

	t.Run("SSH creds from a secret", func(t *testing.T) {
		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)
		secret := fixture.NewSecret("argocd-image-updater", "git-creds", map[string][]byte{
			"sshPrivateKey": []byte("foo"),
		})
		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeClientsetWithResources(secret),
			},
		}
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "git@example.com:example",
					TargetRevision: "main",
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeKustomize,
			},
		}
		// Create iuapi.WriteBackConfig that represents the same configuration as the annotations
		settings := &iuapi.WriteBackConfig{
			Method: stringPtr("git:secret:argocd-image-updater/git-creds"),
			GitConfig: &iuapi.GitConfig{
				Branch: stringPtr("mybranch:mytargetbranch"),
			},
		}

		wbc, err := newWBCFromSettings(context.Background(), &app, &kubeClient, settings)
		require.NoError(t, err)

		creds, err := wbc.GetCreds(&app)
		require.NoError(t, err)
		require.NotNil(t, creds)
		// Must have SSH creds
		_, ok := creds.(git.SSHCreds)
		require.True(t, ok)
	})

	t.Run("HTTP creds from Argo CD settings", func(t *testing.T) {
		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)
		secret := fixture.NewSecret("argocd", "git-creds", map[string][]byte{
			"username": []byte("foo"),
			"password": []byte("bar"),
		})
		repoSecret := fixture.NewSecret("argocd", "repo-https-example-com-example", map[string][]byte{
			"type":     []byte("git"),
			"url":      []byte("https://example.com/example"),
			"username": []byte("foo"),
			"password": []byte("bar"),
		})
		repoSecret.Labels = map[string]string{
			"argocd.argoproj.io/secret-type": "repository",
		}
		fixture.AddPartOfArgoCDLabel(secret, repoSecret)

		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeClientsetWithResources(secret, repoSecret),
				Namespace: "argocd",
			},
		}
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "https://example.com/example",
					TargetRevision: "main",
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeKustomize,
			},
		}
		// Create iuapi.WriteBackConfig that represents the same configuration as the annotations
		settings := &iuapi.WriteBackConfig{
			Method: stringPtr("git"),
			GitConfig: &iuapi.GitConfig{
				Branch: stringPtr("mybranch:mytargetbranch"),
			},
		}

		wbc, err := newWBCFromSettings(context.Background(), &app, &kubeClient, settings)
		require.NoError(t, err)

		creds, err := wbc.GetCreds(&app)
		require.NoError(t, err)
		require.NotNil(t, creds)
		// Must have HTTPS creds
		_, ok := creds.(git.HTTPSCreds)
		require.True(t, ok)
	})

	t.Run("Invalid fields in secret", func(t *testing.T) {
		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)
		secret := fixture.NewSecret("argocd-image-updater", "git-creds", map[string][]byte{
			"sshPrivateKex": []byte("foo"),
		})
		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeClientsetWithResources(secret),
			},
		}
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "git@example.com:example",
					TargetRevision: "main",
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeKustomize,
			},
		}
		// Create iuapi.WriteBackConfig that represents the same configuration as the annotations
		settings := &iuapi.WriteBackConfig{
			Method: stringPtr("git"),
			GitConfig: &iuapi.GitConfig{
				Branch: stringPtr("mybranch:mytargetbranch"),
			},
		}

		wbc, err := newWBCFromSettings(context.Background(), &app, &kubeClient, settings)
		require.NoError(t, err)

		creds, err := wbc.GetCreds(&app)
		require.Error(t, err)
		require.Nil(t, creds)
	})

	t.Run("Invalid secret reference", func(t *testing.T) {
		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)
		secret := fixture.NewSecret("argocd-image-updater", "git-creds", map[string][]byte{
			"sshPrivateKey": []byte("foo"),
		})
		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeClientsetWithResources(secret),
			},
		}
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "git@example.com:example",
					TargetRevision: "main",
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeKustomize,
			},
		}
		// Create iuapi.WriteBackConfig that represents the same configuration as the annotations
		settings := &iuapi.WriteBackConfig{
			Method: stringPtr("git"),
			GitConfig: &iuapi.GitConfig{
				Branch: stringPtr("mybranch:mytargetbranch"),
			},
		}

		wbc, err := newWBCFromSettings(context.Background(), &app, &kubeClient, settings)
		require.NoError(t, err)

		creds, err := wbc.GetCreds(&app)
		require.Error(t, err)
		require.Nil(t, creds)
	})

	t.Run("Secret does not exist", func(t *testing.T) {
		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)
		secret := fixture.NewSecret("argocd-image-updater", "git-creds", map[string][]byte{
			"sshPrivateKey": []byte("foo"),
		})
		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeClientsetWithResources(secret),
			},
		}
		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "git@example.com:example",
					TargetRevision: "main",
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeKustomize,
			},
		}
		// Create iuapi.WriteBackConfig that represents the same configuration as the annotations
		settings := &iuapi.WriteBackConfig{
			Method: stringPtr("git"),
			GitConfig: &iuapi.GitConfig{
				Branch: stringPtr("mybranch:mytargetbranch"),
			},
		}

		wbc, err := newWBCFromSettings(context.Background(), &app, &kubeClient, settings)
		require.NoError(t, err)

		creds, err := wbc.GetCreds(&app)
		require.Error(t, err)
		require.Nil(t, creds)
	})

	t.Run("SSH creds from Argo CD settings with Helm Chart repoURL", func(t *testing.T) {
		argoClient := argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)
		secret := fixture.NewSecret("argocd-image-updater", "git-creds", map[string][]byte{
			"sshPrivateKey": []byte("foo"),
		})
		kubeClient := kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{
				Clientset: fake.NewFakeClientsetWithResources(secret),
			},
		}

		app := v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{
				Name: "testapp",
			},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					RepoURL:        "https://example-helm-repo.com/example",
					TargetRevision: "main",
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeKustomize,
			},
		}

		// Create iuapi.WriteBackConfig that represents the same configuration as the annotations
		settings := &iuapi.WriteBackConfig{
			Method: stringPtr("git:secret:argocd-image-updater/git-creds"),
			GitConfig: &iuapi.GitConfig{
				Branch:     stringPtr("mybranch:mytargetbranch"),
				Repository: stringPtr("git@github.com:example/example.git"),
			},
		}

		wbc, err := newWBCFromSettings(context.Background(), &app, &kubeClient, settings)
		require.NoError(t, err)
		require.Equal(t, wbc.GitRepo, "git@github.com:example/example.git")

		creds, err := wbc.GetCreds(&app)
		require.NoError(t, err)
		require.NotNil(t, creds)
		// Must have SSH creds
		_, ok := creds.(git.SSHCreds)
		require.True(t, ok)
	})
}

func Test_CommitUpdates(t *testing.T) {
	argoClient := argomock.ArgoCD{}
	argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)
	secret := fixture.NewSecret("argocd-image-updater", "git-creds", map[string][]byte{
		"sshPrivateKey": []byte("foo"),
	})
	kubeClient := kube.ImageUpdaterKubernetesClient{
		KubeClient: &registryKube.KubernetesClient{
			Clientset: fake.NewFakeClientsetWithResources(secret),
		},
	}
	app := v1alpha1.Application{
		ObjectMeta: v1.ObjectMeta{
			Name: "testapp",
		},
		Spec: v1alpha1.ApplicationSpec{
			Source: &v1alpha1.ApplicationSource{
				RepoURL:        "git@example.com:example",
				TargetRevision: "main",
			},
		},
		Status: v1alpha1.ApplicationStatus{
			SourceType: v1alpha1.ApplicationSourceTypeKustomize,
		},
	}

	t.Run("Good commit to target revision", func(t *testing.T) {
		gitMock, _, cleanup := mockGit(t)
		defer cleanup()
		gitMock.On("Checkout", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
			args.Assert(t, "main", false)
		}).Return(nil)
		gitMock.On("Add", mock.Anything).Return(nil)
		gitMock.On("Commit", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		gitMock.On("Push", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		ctx := context.Background()
		// Create iuapi.WriteBackConfig that represents the same configuration as the annotations
		// Pass nil settings to test the default target revision fallback
		wbc, err := newWBCFromSettings(ctx, &app, &kubeClient, nil)
		require.NoError(t, err)
		wbc.Method = WriteBackGit
		wbc.GetCreds = func(app *v1alpha1.Application) (git.Creds, error) {
			return git.NopCreds{}, nil
		}
		wbc.GitClient = gitMock

		applicationImages := &ApplicationImages{
			Application:     app,
			Images:          ImageList{},
			WriteBackConfig: wbc,
		}
		err = commitChanges(ctx, applicationImages, nil)
		assert.NoError(t, err)
	})

	t.Run("Good commit to configured branch", func(t *testing.T) {
		gitMock, _, cleanup := mockGit(t)
		defer cleanup()
		gitMock.On("Checkout", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
			args.Assert(t, "mybranch", false)
		}).Return(nil)
		gitMock.On("Add", mock.Anything).Return(nil)
		gitMock.On("Commit", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		gitMock.On("Push", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		gitMock.On("SymRefToBranch", mock.Anything).Return("mydefaultbranch", nil)

		ctx := context.Background()
		wbc, err := newWBCFromSettings(ctx, &app, &kubeClient, nil)
		require.NoError(t, err)
		wbc.Method = WriteBackGit
		wbc.GetCreds = func(app *v1alpha1.Application) (git.Creds, error) {
			return git.NopCreds{}, nil
		}
		wbc.GitClient = gitMock
		wbc.GitBranch = "mybranch"

		applicationImages := &ApplicationImages{
			Application:     app,
			Images:          ImageList{},
			WriteBackConfig: wbc,
		}
		err = commitChanges(ctx, applicationImages, nil)
		assert.NoError(t, err)
	})

	t.Run("Good commit to default branch", func(t *testing.T) {
		app := app.DeepCopy()
		gitMock, _, cleanup := mockGit(t)
		defer cleanup()
		gitMock.On("Checkout", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
			args.Assert(t, "mydefaultbranch", false)
		}).Return(nil)
		gitMock.On("Add", mock.Anything).Return(nil)
		gitMock.On("Commit", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		gitMock.On("Push", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		gitMock.On("SymRefToBranch", mock.Anything).Return("mydefaultbranch", nil)

		ctx := context.Background()
		wbc, err := newWBCFromSettings(ctx, app, &kubeClient, nil)
		require.NoError(t, err)
		wbc.Method = WriteBackGit
		wbc.GetCreds = func(app *v1alpha1.Application) (git.Creds, error) {
			return git.NopCreds{}, nil
		}
		require.NoError(t, err)
		wbc.GitClient = gitMock
		app.Spec.Source.TargetRevision = "HEAD"
		wbc.GitBranch = ""

		applicationImages := &ApplicationImages{
			Application:     *app,
			Images:          ImageList{},
			WriteBackConfig: wbc,
		}
		err = commitChanges(ctx, applicationImages, nil)
		assert.NoError(t, err)
	})

	t.Run("Good commit to different than base branch", func(t *testing.T) {
		gitMock, _, cleanup := mockGit(t)
		defer cleanup()
		gitMock.On("Add", mock.Anything).Return(nil)
		gitMock.On("Branch", mock.Anything, mock.Anything).Return(nil)
		gitMock.On("Commit", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		gitMock.On("Push", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		gitMock.On("SymRefToBranch", mock.Anything).Return("mydefaultbranch", nil)
		ctx := context.Background()
		wbc, err := newWBCFromSettings(ctx, &app, &kubeClient, nil)
		require.NoError(t, err)
		wbc.Method = WriteBackGit
		wbc.GetCreds = func(app *v1alpha1.Application) (git.Creds, error) {
			return git.NopCreds{}, nil
		}
		require.NoError(t, err)
		wbc.GitClient = gitMock
		wbc.GitBranch = "mydefaultbranch"
		wbc.GitWriteBranch = "image-updater{{range .Images}}-{{.Name}}-{{.NewTag}}{{end}}"

		cl := []ChangeEntry{
			{
				Image:  image.NewFromIdentifier("foo/bar"),
				OldTag: tag.NewImageTag("1.0", time.Now(), ""),
				NewTag: tag.NewImageTag("1.1", time.Now(), ""),
			},
		}
		gitMock.On("Checkout", TemplateBranchName(ctx, wbc.GitWriteBranch, cl), mock.Anything).Return(nil)

		applicationImages := &ApplicationImages{
			Application:     app,
			Images:          ImageList{},
			WriteBackConfig: wbc,
		}
		err = commitChanges(ctx, applicationImages, cl)
		assert.NoError(t, err)
	})

	t.Run("Good commit to helm override", func(t *testing.T) {
		app := app.DeepCopy()
		app.Status.SourceType = "Helm"
		app.Spec.Source.Helm = &v1alpha1.ApplicationSourceHelm{Parameters: []v1alpha1.HelmParameter{
			{Name: "bar", Value: "bar", ForceString: true},
			{Name: "baz", Value: "baz", ForceString: true},
		}}
		gitMock, dir, cleanup := mockGit(t)
		defer cleanup()
		of := filepath.Join(dir, ".argocd-source-testapp.yaml")
		assert.NoError(t, os.WriteFile(of, []byte(`
helm:
  parameters:
  - name: foo
    value: foo
    forcestring: true
`), os.ModePerm))

		gitMock.On("Checkout", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
			args.Assert(t, "mydefaultbranch", false)
		}).Return(nil)
		gitMock.On("Add", mock.Anything).Return(nil)
		gitMock.On("Commit", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		gitMock.On("Push", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		gitMock.On("SymRefToBranch", mock.Anything).Return("mydefaultbranch", nil)

		ctx := context.Background()
		wbc, err := newWBCFromSettings(ctx, app, &kubeClient, nil)
		wbc.Method = WriteBackGit
		wbc.GetCreds = func(app *v1alpha1.Application) (git.Creds, error) {
			return git.NopCreds{}, nil
		}
		require.NoError(t, err)
		wbc.GitClient = gitMock
		app.Spec.Source.TargetRevision = "HEAD"
		wbc.GitBranch = ""

		applicationImages := &ApplicationImages{
			Application:     *app,
			Images:          ImageList{},
			WriteBackConfig: wbc,
		}
		err = commitChanges(ctx, applicationImages, nil)
		assert.NoError(t, err)
		override, err := os.ReadFile(of)
		assert.NoError(t, err)
		assert.YAMLEq(t, `
helm:
  parameters:
  - name: foo
    value: foo
    forcestring: true
  - name: bar
    value: bar
    forcestring: true
  - name: baz
    value: baz
    forcestring: true
`, string(override))
	})

	t.Run("Good commit to helm override with argocd namespace", func(t *testing.T) {
		kubeClient.KubeClient.Namespace = "argocd"
		app := app.DeepCopy()
		app.Status.SourceType = "Helm"
		app.ObjectMeta.Namespace = "argocd"
		app.Spec.Source.Helm = &v1alpha1.ApplicationSourceHelm{Parameters: []v1alpha1.HelmParameter{
			{Name: "bar", Value: "bar", ForceString: true},
			{Name: "baz", Value: "baz", ForceString: true},
		}}
		gitMock, dir, cleanup := mockGit(t)
		defer cleanup()
		of := filepath.Join(dir, ".argocd-source-testapp.yaml")
		assert.NoError(t, os.WriteFile(of, []byte(`
helm:
  parameters:
  - name: foo
    value: foo
    forcestring: true
`), os.ModePerm))

		gitMock.On("Checkout", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
			args.Assert(t, "mydefaultbranch", false)
		}).Return(nil)
		gitMock.On("Add", mock.Anything).Return(nil)
		gitMock.On("Commit", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		gitMock.On("Push", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		gitMock.On("SymRefToBranch", mock.Anything).Return("mydefaultbranch", nil)

		ctx := context.Background()
		wbc, err := newWBCFromSettings(ctx, app, &kubeClient, nil)
		wbc.Method = WriteBackGit
		wbc.GetCreds = func(app *v1alpha1.Application) (git.Creds, error) {
			return git.NopCreds{}, nil
		}
		require.NoError(t, err)
		wbc.GitClient = gitMock
		app.Spec.Source.TargetRevision = "HEAD"
		wbc.GitBranch = ""

		applicationImages := &ApplicationImages{
			Application:     *app,
			Images:          ImageList{},
			WriteBackConfig: wbc,
		}
		err = commitChanges(ctx, applicationImages, nil)
		assert.NoError(t, err)
		override, err := os.ReadFile(of)
		assert.NoError(t, err)
		assert.YAMLEq(t, `
helm:
  parameters:
  - name: foo
    value: foo
    forcestring: true
  - name: bar
    value: bar
    forcestring: true
  - name: baz
    value: baz
    forcestring: true
`, string(override))
	})

	t.Run("Good commit to helm override with another namespace", func(t *testing.T) {
		kubeClient.KubeClient.Namespace = "argocd"
		app := app.DeepCopy()
		app.Status.SourceType = "Helm"
		app.ObjectMeta.Namespace = "testNS"
		app.Spec.Source.Helm = &v1alpha1.ApplicationSourceHelm{Parameters: []v1alpha1.HelmParameter{
			{Name: "bar", Value: "bar", ForceString: true},
			{Name: "baz", Value: "baz", ForceString: true},
		}}
		gitMock, dir, cleanup := mockGit(t)
		defer cleanup()
		of := filepath.Join(dir, ".argocd-source-testNS_testapp.yaml")
		assert.NoError(t, os.WriteFile(of, []byte(`
helm:
  parameters:
  - name: foo
    value: foo
    forcestring: true
`), os.ModePerm))

		gitMock.On("Checkout", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
			args.Assert(t, "mydefaultbranch", false)
		}).Return(nil)
		gitMock.On("Add", mock.Anything).Return(nil)
		gitMock.On("Commit", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		gitMock.On("Push", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		gitMock.On("SymRefToBranch", mock.Anything).Return("mydefaultbranch", nil)

		ctx := context.Background()
		wbc, err := newWBCFromSettings(ctx, app, &kubeClient, nil)
		require.NoError(t, err)
		wbc.Method = WriteBackGit
		wbc.GetCreds = func(app *v1alpha1.Application) (git.Creds, error) {
			return git.NopCreds{}, nil
		}
		wbc.GitClient = gitMock
		app.Spec.Source.TargetRevision = "HEAD"
		wbc.GitBranch = ""

		applicationImages := &ApplicationImages{
			Application:     *app,
			Images:          ImageList{},
			WriteBackConfig: wbc,
		}
		err = commitChanges(ctx, applicationImages, nil)
		assert.NoError(t, err)
		override, err := os.ReadFile(of)
		assert.NoError(t, err)
		assert.YAMLEq(t, `
helm:
  parameters:
  - name: foo
    value: foo
    forcestring: true
  - name: bar
    value: bar
    forcestring: true
  - name: baz
    value: baz
    forcestring: true
`, string(override))
	})

	t.Run("Good commit to kustomization", func(t *testing.T) {
		app := app.DeepCopy()
		app.Spec.Source.Kustomize = &v1alpha1.ApplicationSourceKustomize{Images: v1alpha1.KustomizeImages{"foo=bar", "bar=baz:123"}}
		gitMock, dir, cleanup := mockGit(t)
		defer cleanup()
		kf := filepath.Join(dir, "kustomization.yml")
		assert.NoError(t, os.WriteFile(kf, []byte(`
kind: Kustomization
apiVersion: kustomize.config.k8s.io/v1beta1

replacements: []
`), os.ModePerm))

		gitMock.On("Checkout", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
			args.Assert(t, "mydefaultbranch", false)
		}).Return(nil)
		gitMock.On("Add", mock.Anything).Return(nil)
		gitMock.On("Commit", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		gitMock.On("Push", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		gitMock.On("SymRefToBranch", mock.Anything).Return("mydefaultbranch", nil)

		ctx := context.Background()
		wbc, err := newWBCFromSettings(ctx, app, &kubeClient, nil)
		require.NoError(t, err)
		wbc.Method = WriteBackGit
		wbc.GetCreds = func(app *v1alpha1.Application) (git.Creds, error) {
			return git.NopCreds{}, nil
		}
		wbc.GitClient = gitMock
		app.Spec.Source.TargetRevision = "HEAD"
		wbc.GitBranch = ""
		// Set the kustomize base for kustomization write-back
		wbc.KustomizeBase = "."

		applicationImages := &ApplicationImages{
			Application:     *app,
			Images:          ImageList{},
			WriteBackConfig: wbc,
		}
		err = commitChanges(ctx, applicationImages, nil)
		assert.NoError(t, err)
		kust, err := os.ReadFile(kf)
		assert.NoError(t, err)
		assert.YAMLEq(t, `
kind: Kustomization
apiVersion: kustomize.config.k8s.io/v1beta1
images:
  - name: foo
    newName: bar
  - name: bar
    newName: baz
    newTag: "123"

replacements: []
`, string(kust))

		// test the merge case too
		app.Spec.Source.Kustomize.Images = v1alpha1.KustomizeImages{"foo:123", "bar=qux"}
		applicationImages = &ApplicationImages{
			Application:     *app,
			Images:          ImageList{},
			WriteBackConfig: wbc,
		}
		err = commitChanges(ctx, applicationImages, nil)
		assert.NoError(t, err)
		kust, err = os.ReadFile(kf)
		assert.NoError(t, err)
		assert.YAMLEq(t, `
kind: Kustomization
apiVersion: kustomize.config.k8s.io/v1beta1
images:
  - name: foo
    newTag: "123"
  - name: bar
    newName: qux

replacements: []
`, string(kust))
	})

	t.Run("Good commit with author information", func(t *testing.T) {
		app := app.DeepCopy()
		gitMock, _, cleanup := mockGit(t)
		defer cleanup()
		gitMock.On("Checkout", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
			args.Assert(t, "mydefaultbranch", false)
		}).Return(nil)
		gitMock.On("Add", mock.Anything).Return(nil)
		gitMock.On("Commit", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		gitMock.On("Push", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		gitMock.On("SymRefToBranch", mock.Anything).Return("mydefaultbranch", nil)
		gitMock.On("Config", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
			args.Assert(t, "someone", "someone@example.com")
		}).Return(nil)

		ctx := context.Background()
		wbc, err := newWBCFromSettings(ctx, app, &kubeClient, nil)
		wbc.Method = WriteBackGit
		wbc.GetCreds = func(app *v1alpha1.Application) (git.Creds, error) {
			return git.NopCreds{}, nil
		}
		require.NoError(t, err)
		wbc.GitClient = gitMock
		app.Spec.Source.TargetRevision = "HEAD"
		wbc.GitBranch = ""
		wbc.GitCommitUser = "someone"
		wbc.GitCommitEmail = "someone@example.com"

		applicationImages := &ApplicationImages{
			Application:     *app,
			Images:          ImageList{},
			WriteBackConfig: wbc,
		}
		err = commitChanges(ctx, applicationImages, nil)
		assert.NoError(t, err)
	})

	t.Run("Cannot set author information", func(t *testing.T) {
		app := app.DeepCopy()
		gitMock := &gitmock.Client{}
		gitMock.On("Init").Return(nil)
		gitMock.On("Root").Return(t.TempDir())
		gitMock.On("ShallowFetch", mock.Anything, mock.Anything).Return(nil)
		gitMock.On("Checkout", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
			args.Assert(t, "mydefaultbranch", false)
		}).Return(nil)
		gitMock.On("Add", mock.Anything).Return(nil)
		gitMock.On("Commit", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		gitMock.On("Push", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		gitMock.On("SymRefToBranch", mock.Anything).Return("mydefaultbranch", nil)
		gitMock.On("Config", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
			args.Assert(t, "someone", "someone@example.com")
		}).Return(fmt.Errorf("could not configure git"))
		// Create iuapi.WriteBackConfig that represents the same configuration as the annotations
		settings := &iuapi.WriteBackConfig{
			Method: stringPtr("git"),
			GitConfig: &iuapi.GitConfig{
				Branch: stringPtr("mybranch:mytargetbranch"),
			},
		}

		ctx := context.Background()
		wbc, err := newWBCFromSettings(ctx, app, &kubeClient, settings)
		require.NoError(t, err)
		wbc.GitClient = gitMock
		app.Spec.Source.TargetRevision = "HEAD"
		wbc.GitBranch = ""
		wbc.GitCommitUser = "someone"
		wbc.GitCommitEmail = "someone@example.com"

		applicationImages := &ApplicationImages{
			Application:     *app,
			Images:          ImageList{},
			WriteBackConfig: wbc,
		}
		err = commitChanges(ctx, applicationImages, nil)
		assert.Errorf(t, err, "could not configure git")
	})

	t.Run("Cannot init", func(t *testing.T) {
		gitMock := &gitmock.Client{}
		gitMock.On("Init").Return(fmt.Errorf("cannot init"))
		gitMock.On("ShallowFetch", mock.Anything, mock.Anything).Return(nil)
		gitMock.On("Checkout", mock.Anything, mock.Anything).Return(nil)
		gitMock.On("Commit", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		gitMock.On("Push", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		// Create iuapi.WriteBackConfig that represents the same configuration as the annotations
		settings := &iuapi.WriteBackConfig{
			Method: stringPtr("git"),
			GitConfig: &iuapi.GitConfig{
				Branch: stringPtr("mybranch:mytargetbranch"),
			},
		}
		ctx := context.Background()
		wbc, err := newWBCFromSettings(ctx, &app, &kubeClient, settings)
		require.NoError(t, err)
		wbc.GitClient = gitMock

		applicationImages := &ApplicationImages{
			Application:     app,
			Images:          ImageList{},
			WriteBackConfig: wbc,
		}
		err = commitChanges(ctx, applicationImages, nil)
		assert.Errorf(t, err, "cannot init")
	})

	t.Run("Cannot fetch", func(t *testing.T) {
		gitMock := &gitmock.Client{}
		gitMock.On("Init").Return(nil)
		gitMock.On("ShallowFetch", mock.Anything, mock.Anything).Return(fmt.Errorf("cannot fetch"))
		gitMock.On("Checkout", mock.Anything, mock.Anything).Return(nil)
		gitMock.On("Commit", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		gitMock.On("Push", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		// Create iuapi.WriteBackConfig that represents the same configuration as the annotations
		settings := &iuapi.WriteBackConfig{
			Method: stringPtr("git"),
			GitConfig: &iuapi.GitConfig{
				Branch: stringPtr("mybranch:mytargetbranch"),
			},
		}
		ctx := context.Background()
		wbc, err := newWBCFromSettings(ctx, &app, &kubeClient, settings)
		require.NoError(t, err)
		wbc.GitClient = gitMock

		applicationImages := &ApplicationImages{
			Application:     app,
			Images:          ImageList{},
			WriteBackConfig: wbc,
		}
		err = commitChanges(ctx, applicationImages, nil)
		assert.Errorf(t, err, "cannot init")
	})
	t.Run("Cannot checkout", func(t *testing.T) {
		gitMock := &gitmock.Client{}
		gitMock.On("Init").Return(nil)
		gitMock.On("ShallowFetch", mock.Anything, mock.Anything).Return(nil)
		gitMock.On("Checkout", mock.Anything, mock.Anything).Return(fmt.Errorf("cannot checkout"))
		gitMock.On("Commit", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		gitMock.On("Push", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		// Create iuapi.WriteBackConfig that represents the same configuration as the annotations
		settings := &iuapi.WriteBackConfig{
			Method: stringPtr("git"),
			GitConfig: &iuapi.GitConfig{
				Branch: stringPtr("mybranch:mytargetbranch"),
			},
		}
		ctx := context.Background()
		wbc, err := newWBCFromSettings(ctx, &app, &kubeClient, settings)
		require.NoError(t, err)
		wbc.GitClient = gitMock

		applicationImages := &ApplicationImages{
			Application:     app,
			Images:          ImageList{},
			WriteBackConfig: wbc,
		}
		err = commitChanges(ctx, applicationImages, nil)
		assert.Errorf(t, err, "cannot checkout")
	})

	t.Run("Cannot commit", func(t *testing.T) {
		gitMock, _, cleanup := mockGit(t)
		defer cleanup()
		gitMock.On("Checkout", mock.Anything, mock.Anything).Return(nil)
		gitMock.On("Add", mock.Anything).Return(nil)
		gitMock.On("Commit", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("cannot commit"))
		gitMock.On("Push", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		// Create iuapi.WriteBackConfig that represents the same configuration as the annotations
		settings := &iuapi.WriteBackConfig{
			Method: stringPtr("git"),
			GitConfig: &iuapi.GitConfig{
				Branch: stringPtr("mybranch:mytargetbranch"),
			},
		}
		ctx := context.Background()
		wbc, err := newWBCFromSettings(ctx, &app, &kubeClient, settings)
		require.NoError(t, err)
		wbc.GitClient = gitMock

		applicationImages := &ApplicationImages{
			Application:     app,
			Images:          ImageList{},
			WriteBackConfig: wbc,
		}
		err = commitChanges(ctx, applicationImages, nil)
		assert.Errorf(t, err, "cannot commit")
	})

	t.Run("Cannot push", func(t *testing.T) {
		gitMock, _, cleanup := mockGit(t)
		defer cleanup()
		gitMock.On("Checkout", mock.Anything, mock.Anything).Return(nil)
		gitMock.On("Add", mock.Anything).Return(nil)
		gitMock.On("Commit", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		gitMock.On("Push", mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("cannot push"))
		// Create iuapi.WriteBackConfig that represents the same configuration as the annotations
		settings := &iuapi.WriteBackConfig{
			Method: stringPtr("git"),
			GitConfig: &iuapi.GitConfig{
				Branch: stringPtr("mybranch:mytargetbranch"),
			},
		}
		ctx := context.Background()
		wbc, err := newWBCFromSettings(ctx, &app, &kubeClient, settings)
		require.NoError(t, err)
		wbc.GitClient = gitMock

		applicationImages := &ApplicationImages{
			Application:     app,
			Images:          ImageList{},
			WriteBackConfig: wbc,
		}
		err = commitChanges(ctx, applicationImages, nil)
		assert.Errorf(t, err, "cannot push")
	})

	t.Run("Cannot resolve default branch", func(t *testing.T) {
		app := app.DeepCopy()
		gitMock, _, cleanup := mockGit(t)
		defer cleanup()
		gitMock.On("Checkout", mock.Anything, mock.Anything).Return(nil)
		gitMock.On("Add", mock.Anything).Return(nil)
		gitMock.On("Commit", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
		gitMock.On("Push", mock.Anything, mock.Anything, mock.Anything).Return(nil)
		gitMock.On("SymRefToBranch", mock.Anything).Return("", fmt.Errorf("failed to resolve ref"))
		// Create iuapi.WriteBackConfig that represents the same configuration as the annotations
		settings := &iuapi.WriteBackConfig{
			Method: stringPtr("git"),
			GitConfig: &iuapi.GitConfig{
				Branch: stringPtr("mybranch:mytargetbranch"),
			},
		}

		ctx := context.Background()
		wbc, err := newWBCFromSettings(ctx, app, &kubeClient, settings)
		require.NoError(t, err)
		wbc.GitClient = gitMock
		app.Spec.Source.TargetRevision = "HEAD"
		wbc.GitBranch = ""

		applicationImages := &ApplicationImages{
			Application:     *app,
			Images:          ImageList{},
			WriteBackConfig: wbc,
		}
		err = commitChanges(ctx, applicationImages, nil)
		assert.Errorf(t, err, "failed to resolve ref")
	})
}

func Test_parseKustomizeBase(t *testing.T) {
	cases := []struct {
		name     string
		expected string
		target   string
		path     string
	}{
		{"default", ".", "kustomization", ""},
		{"explicit default", ".", "kustomization:.", "."},
		{"default path, explicit target", ".", "kustomization:.", ""},
		{"default target with path", "foo/bar", "kustomization", "foo/bar"},
		{"default both", ".", "kustomization", ""},
		{"absolute path", "foo", "kustomization:/foo", "bar"},
		{"relative path", "bar/foo", "kustomization:foo", "bar"},
		{"sibling path", "bar/baz", "kustomization:../baz", "bar/foo"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, parseKustomizeBase(tt.target, tt.path))
		})
	}
}

func Test_parseTarget(t *testing.T) {
	cases := []struct {
		name     string
		expected string
		target   string
		path     string
	}{
		{"default", "values.yaml", "helmvalues", ""},
		{"explicit default", "values.yaml", "helmvalues:./values.yaml", "."},
		{"default path, explicit target", "values.yaml", "helmvalues:./values.yaml", ""},
		{"default target with path", "foo/bar/values.yaml", "helmvalues", "foo/bar"},
		{"default both", "values.yaml", "helmvalues", ""},
		{"absolute path", "foo/app-values.yaml", "helmvalues:/foo/app-values.yaml", "bar"},
		{"relative path", "bar/foo/app-values.yaml", "helmvalues:foo/app-values.yaml", "bar"},
		{"sibling path", "bar/baz/app-values.yaml", "helmvalues:../baz/app-values.yaml", "bar/foo"},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, parseTarget(tt.target, tt.path))
		})
	}
}

func mockGit(t *testing.T) (gitMock *gitmock.Client, dir string, cleanup func()) {
	dir, err := os.MkdirTemp("", "wb-kust")
	assert.NoError(t, err)
	gitMock = &gitmock.Client{}
	gitMock.On("Root").Return(dir)
	gitMock.On("Init").Return(nil)
	gitMock.On("ShallowFetch", mock.Anything, mock.Anything).Return(nil)
	return gitMock, dir, func() {
		_ = os.RemoveAll(dir)
	}
}

func Test_GetRepositoryLock(t *testing.T) {
	state := NewSyncIterationState()

	// Test case 1: Get lock for a repository that doesn't exist in the state
	repo1 := "repo1"
	lock1 := state.GetRepositoryLock(repo1)
	require.NotNil(t, lock1)
	require.Equal(t, lock1, state.repositoryLocks[repo1])

	// Test case 2: Get lock for the same repository again, should return the same lock
	lock2 := state.GetRepositoryLock(repo1)
	require.Equal(t, lock1, lock2)

	// Test case 3: Get lock for a different repository, should return a different lock
	repo2 := "repo2"
	lock3 := state.GetRepositoryLock(repo2)
	require.NotNil(t, lock3)
	require.NotNil(t, state.repositoryLocks[repo2])
	require.Equal(t, lock3, state.repositoryLocks[repo2])
}

func Test_mergeKustomizeOverride(t *testing.T) {
	tests := []struct {
		name     string
		existing *v1alpha1.KustomizeImages
		new      *v1alpha1.KustomizeImages
		expected *v1alpha1.KustomizeImages
	}{
		{"with-tag", &v1alpha1.KustomizeImages{"nginx:foo"},
			&v1alpha1.KustomizeImages{"nginx:foo"},
			&v1alpha1.KustomizeImages{"nginx:foo"}},
		{"no-tag", &v1alpha1.KustomizeImages{"nginx:foo"},
			&v1alpha1.KustomizeImages{"nginx"},
			&v1alpha1.KustomizeImages{"nginx:foo"}},
		{"with-tag-1", &v1alpha1.KustomizeImages{"nginx"},
			&v1alpha1.KustomizeImages{"nginx:latest"},
			&v1alpha1.KustomizeImages{"nginx:latest"}},
		{"with-tag-sha", &v1alpha1.KustomizeImages{"nginx:latest"},
			&v1alpha1.KustomizeImages{"nginx:latest@sha256:91734281c0ebfc6f1aea979cffeed5079cfe786228a71cc6f1f46a228cde6e34"},
			&v1alpha1.KustomizeImages{"nginx:latest@sha256:91734281c0ebfc6f1aea979cffeed5079cfe786228a71cc6f1f46a228cde6e34"}},

		{"2-images", &v1alpha1.KustomizeImages{"nginx:latest",
			"bitnami/nginx:latest@sha256:1a2fe3f9f6d1d38d5a7ee35af732fdb7d15266ec3dbc79bbc0355742cd24d3ec"},
			&v1alpha1.KustomizeImages{"nginx:latest@sha256:91734281c0ebfc6f1aea979cffeed5079cfe786228a71cc6f1f46a228cde6e34",
				"bitnami/nginx@sha256:1a2fe3f9f6d1d38d5a7ee35af732fdb7d15266ec3dbc79bbc0355742cd24d3ec"},
			&v1alpha1.KustomizeImages{"nginx:latest@sha256:91734281c0ebfc6f1aea979cffeed5079cfe786228a71cc6f1f46a228cde6e34",
				"bitnami/nginx:latest@sha256:1a2fe3f9f6d1d38d5a7ee35af732fdb7d15266ec3dbc79bbc0355742cd24d3ec"}},

		{"with-registry", &v1alpha1.KustomizeImages{"quay.io/nginx:latest"},
			&v1alpha1.KustomizeImages{"quay.io/nginx:latest"},
			&v1alpha1.KustomizeImages{"quay.io/nginx:latest"}},
		{"with-registry-1", &v1alpha1.KustomizeImages{"quay.io/nginx:latest"},
			&v1alpha1.KustomizeImages{"docker.io/nginx:latest"},
			&v1alpha1.KustomizeImages{"docker.io/nginx:latest", "quay.io/nginx:latest"}},
		{"o_is_nil", &v1alpha1.KustomizeImages{"nginx:foo"},
			nil,
			&v1alpha1.KustomizeImages{"nginx:foo"}},
		{"t_is_nil", nil,
			&v1alpha1.KustomizeImages{"nginx:foo"},
			&v1alpha1.KustomizeImages{"nginx:foo"}},
		{"both_are_nil", nil,
			nil,
			nil},
		{"add_to_empty", &v1alpha1.KustomizeImages{},
			&v1alpha1.KustomizeImages{"nginx:foo"},
			&v1alpha1.KustomizeImages{"nginx:foo"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			existingImages := kustomizeOverride{
				Kustomize: kustomizeImages{
					Images: tt.existing,
				},
			}
			newImages := kustomizeOverride{
				Kustomize: kustomizeImages{
					Images: tt.new,
				},
			}
			expectedImages := kustomizeOverride{
				Kustomize: kustomizeImages{
					Images: tt.expected,
				},
			}

			mergeKustomizeOverride(&existingImages, &newImages)
			if expectedImages.Kustomize.Images == nil {
				assert.Nil(t, existingImages.Kustomize.Images)
			} else {
				require.NotNil(t, existingImages.Kustomize.Images)
				assert.ElementsMatch(t, *expectedImages.Kustomize.Images, *existingImages.Kustomize.Images)
			}
		})
	}
}

// Helper function to create string pointers for testing
func stringPtr(s string) *string {
	return &s
}
