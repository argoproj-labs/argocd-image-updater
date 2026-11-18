package argocd

import (
	"context"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/distribution/distribution/v3/manifest/schema2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	iuapi "github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"
	argomock "github.com/argoproj-labs/argocd-image-updater/pkg/argocd/mocks"
	"github.com/argoproj-labs/argocd-image-updater/pkg/kube"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/image"
	registryKube "github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/kube"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/registry"
	regmock "github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/registry/mocks"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/tag"
	"github.com/argoproj-labs/argocd-image-updater/test/fake"
)

// The image is tracked in its upstream registry, but the application pulls it
// through a pull-through cache, which only lists the tags that have been pulled.
const (
	upstreamImage = "ghcr.io/letsencrypt/pebble"
	mirrorImage   = "123456789012.dkr.ecr.eu-west-1.amazonaws.com/ghcr/letsencrypt/pebble"
)

func Test_parseImageList_DeployedImageName(t *testing.T) {
	t.Run("DeployedImageOverride keeps the alias, constraint and Kustomize image", func(t *testing.T) {
		images := []iuapi.ImageConfig{{
			Alias:             "pebble",
			ImageName:         upstreamImage + ":~2.0",
			DeployedImageName: new(mirrorImage),
			ManifestTarget: &iuapi.ManifestTarget{
				Kustomize: &iuapi.KustomizeTarget{Name: new(upstreamImage)},
			},
		}}
		got := parseImageList(context.Background(), nil, "", images, nil, nil, nil)
		require.NotNil(t, got)
		require.Len(t, *got, 1)
		img := (*got)[0]

		// Tags are still looked up in the upstream registry.
		assert.Equal(t, "ghcr.io", img.RegistryURL)
		assert.Equal(t, "letsencrypt/pebble", img.ImageName)

		deployed := img.DeployedImageOverride
		require.NotNil(t, deployed)
		assert.Equal(t, "123456789012.dkr.ecr.eu-west-1.amazonaws.com", deployed.RegistryURL)
		assert.Equal(t, "ghcr/letsencrypt/pebble", deployed.ImageName)
		assert.Equal(t, "pebble", deployed.ImageAlias)
		require.NotNil(t, deployed.ImageTag)
		assert.Equal(t, "~2.0", deployed.ImageTag.TagName)
		require.NotNil(t, deployed.KustomizeImage)
		assert.Equal(t, upstreamImage, deployed.KustomizeImage.GetFullNameWithoutTag())
		assert.Same(t, deployed, img.DeployedImage())
	})

	t.Run("No DeployedImageOverride when the field is not set", func(t *testing.T) {
		images := []iuapi.ImageConfig{{Alias: "pebble", ImageName: upstreamImage + ":~2.0"}}
		got := parseImageList(context.Background(), nil, "", images, nil, nil, nil)
		require.NotNil(t, got)
		require.Len(t, *got, 1)
		assert.Nil(t, (*got)[0].DeployedImageOverride)
		assert.Same(t, (*got)[0].ContainerImage, (*got)[0].DeployedImage())
	})

	t.Run("Webhooks still match the upstream repository", func(t *testing.T) {
		images := []iuapi.ImageConfig{{
			Alias:             "pebble",
			ImageName:         upstreamImage + ":~2.0",
			DeployedImageName: new(mirrorImage),
		}}
		got := parseImageList(context.Background(), nil, "", images, nil, nil,
			&WebhookEvent{RegistryURL: "ghcr.io", Repository: "letsencrypt/pebble"})
		require.NotNil(t, got)
		assert.Len(t, *got, 1)
	})

	t.Run("An invalid deployed image name skips the image", func(t *testing.T) {
		for _, name := range []string{
			"pebble=" + mirrorImage,
			mirrorImage + ":2.0.1",
			mirrorImage + "@sha256:0000000000000000000000000000000000000000000000000000000000000000",
			"mirror.invalid/LetsEncrypt/pebble",
			" " + mirrorImage,
		} {
			t.Run(name, func(t *testing.T) {
				images := []iuapi.ImageConfig{{
					Alias:             "pebble",
					ImageName:         upstreamImage + ":~2.0",
					DeployedImageName: new(name),
				}}
				got := parseImageList(context.Background(), nil, "", images, nil, nil, nil)
				require.NotNil(t, got)
				assert.Empty(t, *got)
			})
		}
	})
}

func Test_GetImagesAndAliasesFromApplication_DeployedImageOverride(t *testing.T) {
	newImg := func() *Image {
		img := NewImage(image.NewFromIdentifier("pebble=" + upstreamImage + ":~2.0"))
		img.DeployedImageOverride = newDeployedImage(img.ContainerImage, mirrorImage)
		return img
	}
	appImages := func(img *Image, liveImages ...string) *ApplicationImages {
		return &ApplicationImages{
			Application: v1alpha1.Application{
				Status: v1alpha1.ApplicationStatus{Summary: v1alpha1.ApplicationSummary{Images: liveImages}},
			},
			Images: ImageList{img},
		}
	}

	for _, tc := range []struct {
		name string
		live []string
		want string
	}{
		{"The mirror image is live", []string{mirrorImage + ":2.0.0"}, mirrorImage + ":2.0.0"},
		{"The application still runs the upstream image", []string{upstreamImage + ":2.0.0"}, upstreamImage + ":2.0.0"},
		{"The mirror image is preferred when both are live", []string{upstreamImage + ":2.0.0", mirrorImage + ":2.0.1"}, mirrorImage + ":2.0.1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := GetImagesAndAliasesFromApplication(appImages(newImg(), tc.live...))
			require.Len(t, got, 1)
			assert.Equal(t, tc.want, got[0].ContainerImage.GetFullNameWithTag())
			assert.Equal(t, "pebble", got[0].ContainerImage.ImageAlias)
		})
	}

	t.Run("The mirror image is preferred when the Kustomize image names the upstream one", func(t *testing.T) {
		img := NewImage(image.NewFromIdentifier("pebble=" + upstreamImage + ":~2.0"))
		img.KustomizeImageName = upstreamImage
		img.ContainerImage.KustomizeImage = image.NewFromIdentifier(upstreamImage)
		img.DeployedImageOverride = newDeployedImage(img.ContainerImage, mirrorImage)

		got := GetImagesAndAliasesFromApplication(appImages(img, upstreamImage+":2.0.0", mirrorImage+":2.0.1"))
		require.Len(t, got, 1)
		assert.Equal(t, mirrorImage+":2.0.1", got[0].ContainerImage.GetFullNameWithTag())
	})
}

func Test_UpdateApplication_DeployedImageOverride(t *testing.T) {
	// newCfg returns an update configuration whose registry client fails the
	// test if tags are listed anywhere but in the upstream repository.
	newCfg := func(t *testing.T, app *ApplicationImages) *UpdateConfiguration {
		argoClient := &argomock.ArgoCD{}
		argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)
		return &UpdateConfiguration{
			NewRegFN: func(endpoint *registry.RegistryEndpoint, username, password string) (registry.RegistryClient, error) {
				assert.Equal(t, "ghcr.io", endpoint.RegistryPrefix)
				regMock := regmock.RegistryClient{}
				regMock.On("NewRepository", mock.Anything, "letsencrypt/pebble").Return(nil)
				regMock.On("Tags", mock.Anything).Return([]string{"2.0.0", "2.0.1"}, nil)
				return &regMock, nil
			},
			ArgoClient: argoClient,
			KubeClient: &kube.ImageUpdaterKubernetesClient{
				KubeClient: &registryKube.KubernetesClient{Clientset: fake.NewFakeKubeClient()},
			},
			UpdateApp: app,
		}
	}

	helmImage := func(withOverride bool) *Image {
		img := NewImage(image.NewFromIdentifier("pebble=" + upstreamImage + ":~2.0"))
		img.HelmImageName = "image.repository"
		img.HelmImageTag = "image.tag"
		if withOverride {
			img.DeployedImageOverride = newDeployedImage(img.ContainerImage, mirrorImage)
		}
		return img
	}

	helmApp := func(img *Image, liveImages ...string) *ApplicationImages {
		return &ApplicationImages{
			Application: v1alpha1.Application{
				ObjectMeta: v1.ObjectMeta{Name: "pebble", Namespace: "argocd"},
				Spec: v1alpha1.ApplicationSpec{
					Source: &v1alpha1.ApplicationSource{
						Helm: &v1alpha1.ApplicationSourceHelm{
							Parameters: []v1alpha1.HelmParameter{
								{Name: "image.repository", Value: mirrorImage},
								{Name: "image.tag", Value: "2.0.0"},
							},
						},
					},
				},
				Status: v1alpha1.ApplicationStatus{
					SourceType: v1alpha1.ApplicationSourceTypeHelm,
					Summary:    v1alpha1.ApplicationSummary{Images: liveImages},
				},
			},
			WriteBackConfig: &WriteBackConfig{Method: WriteBackApplication},
			Images:          ImageList{img},
		}
	}

	helmParam := func(app *ApplicationImages, name string) string {
		p := getHelmParam(app.Application.Spec.Source.Helm.Parameters, name)
		if p == nil {
			return ""
		}
		return p.Value
	}

	t.Run("Helm: tags come from upstream, the mirror is matched and written back", func(t *testing.T) {
		app := helmApp(helmImage(true), mirrorImage+":2.0.0")
		res := UpdateApplication(context.Background(), newCfg(t, app), NewSyncIterationState())

		assert.Equal(t, 0, res.NumErrors)
		assert.Equal(t, 0, res.NumSkipped)
		assert.Equal(t, 1, res.NumImagesConsidered)
		assert.Equal(t, 1, res.NumImagesUpdated)
		assert.Equal(t, mirrorImage, helmParam(app, "image.repository"))
		assert.Equal(t, "2.0.1", helmParam(app, "image.tag"))
		require.Len(t, res.Changes, 1)
		assert.Equal(t, mirrorImage+":2.0.1", res.Changes[0].Image.GetFullNameWithTag())
		assert.Equal(t, "2.0.0", res.Changes[0].OldTag.TagName)
	})

	t.Run("Helm: without a deployed image the mirrored image is not found", func(t *testing.T) {
		app := helmApp(helmImage(false), mirrorImage+":2.0.0")
		res := UpdateApplication(context.Background(), newCfg(t, app), NewSyncIterationState())

		assert.Equal(t, 1, res.NumSkipped)
		assert.Equal(t, 0, res.NumImagesConsidered)
		assert.Equal(t, "2.0.0", helmParam(app, "image.tag"))
	})

	t.Run("Helm: an application that runs the upstream image is moved to the mirror", func(t *testing.T) {
		app := helmApp(helmImage(true), upstreamImage+":2.0.0")
		app.Application.Spec.Source.Helm.Parameters[0].Value = upstreamImage
		res := UpdateApplication(context.Background(), newCfg(t, app), NewSyncIterationState())

		assert.Equal(t, 0, res.NumErrors)
		assert.Equal(t, 0, res.NumSkipped)
		assert.Equal(t, 1, res.NumImagesUpdated)
		assert.Equal(t, mirrorImage, helmParam(app, "image.repository"))
		assert.Equal(t, "2.0.1", helmParam(app, "image.tag"))
	})

	t.Run("Helm: an upstream image on the latest tag is still moved to the mirror", func(t *testing.T) {
		app := helmApp(helmImage(true), upstreamImage+":2.0.1")
		app.Application.Spec.Source.Helm.Parameters[0].Value = upstreamImage
		app.Application.Spec.Source.Helm.Parameters[1].Value = "2.0.1"
		cfg := newCfg(t, app)

		res := UpdateApplication(context.Background(), cfg, NewSyncIterationState())
		assert.Equal(t, 0, res.NumErrors)
		assert.Equal(t, 1, res.NumImagesUpdated)
		assert.Equal(t, mirrorImage, helmParam(app, "image.repository"))
		assert.Equal(t, "2.0.1", helmParam(app, "image.tag"))

		// Until the application syncs, the upstream image is still running, but
		// the mirror is already in the spec and must not be written again.
		res = UpdateApplication(context.Background(), cfg, NewSyncIterationState())
		assert.Equal(t, 0, res.NumErrors)
		assert.Equal(t, 0, res.NumImagesUpdated)
	})

	t.Run("Helm: up to date keeps the mirror repository", func(t *testing.T) {
		app := helmApp(helmImage(true), mirrorImage+":2.0.1")
		res := UpdateApplication(context.Background(), newCfg(t, app), NewSyncIterationState())

		assert.Equal(t, 0, res.NumErrors)
		assert.Equal(t, 1, res.NumImagesConsidered)
		assert.Equal(t, 0, res.NumImagesUpdated)
		assert.Equal(t, mirrorImage, helmParam(app, "image.repository"))
		assert.Equal(t, "2.0.1", helmParam(app, "image.tag"))
	})

	t.Run("Helm: forceUpdate adds the mirror image when it is not running", func(t *testing.T) {
		img := helmImage(true)
		img.ForceUpdate = true
		app := helmApp(img)
		res := UpdateApplication(context.Background(), newCfg(t, app), NewSyncIterationState())

		assert.Equal(t, 0, res.NumErrors)
		assert.Equal(t, 1, res.NumImagesConsidered)
		assert.Equal(t, 1, res.NumImagesUpdated)
		assert.Equal(t, mirrorImage, helmParam(app, "image.repository"))
		assert.Equal(t, "2.0.1", helmParam(app, "image.tag"))
	})

	t.Run("Helm: forceUpdate finds a running upstream image with its tag", func(t *testing.T) {
		img := helmImage(true)
		img.ForceUpdate = true
		app := helmApp(img, upstreamImage+":2.0.0")
		app.Application.Spec.Source.Helm.Parameters[0].Value = upstreamImage
		res := UpdateApplication(context.Background(), newCfg(t, app), NewSyncIterationState())

		assert.Equal(t, 0, res.NumErrors)
		assert.Equal(t, 1, res.NumImagesUpdated)
		assert.Equal(t, mirrorImage, helmParam(app, "image.repository"))
		assert.Equal(t, "2.0.1", helmParam(app, "image.tag"))
		require.Len(t, res.Changes, 1)
		assert.Equal(t, "2.0.0", res.Changes[0].OldTag.TagName)
	})

	t.Run("Helm: an upstream image is not moved when only the tag is written back", func(t *testing.T) {
		img := helmImage(true)
		img.HelmImageName = ""
		app := helmApp(img, upstreamImage+":2.0.0")
		app.Application.Spec.Source.Helm.Parameters = []v1alpha1.HelmParameter{{Name: "image.tag", Value: "2.0.0"}}
		cfg := newCfg(t, app)

		for range 2 {
			res := UpdateApplication(context.Background(), cfg, NewSyncIterationState())
			assert.Equal(t, 1, res.NumErrors)
			assert.Equal(t, 0, res.NumImagesUpdated)
			assert.Equal(t, "2.0.0", helmParam(app, "image.tag"))
		}
	})

	kustomizeApp := func(img *Image) *ApplicationImages {
		return &ApplicationImages{
			Application: v1alpha1.Application{
				ObjectMeta: v1.ObjectMeta{Name: "pebble", Namespace: "argocd"},
				Spec: v1alpha1.ApplicationSpec{
					Source: &v1alpha1.ApplicationSource{
						Kustomize: &v1alpha1.ApplicationSourceKustomize{},
					},
				},
				Status: v1alpha1.ApplicationStatus{
					SourceType: v1alpha1.ApplicationSourceTypeKustomize,
					Summary:    v1alpha1.ApplicationSummary{Images: []string{mirrorImage + ":2.0.0"}},
				},
			},
			WriteBackConfig: &WriteBackConfig{Method: WriteBackApplication},
			Images:          ImageList{img},
		}
	}

	// kustomizeImage returns an image with a DeployedImageOverride, written
	// under the Kustomize image name kustomizeName if that is not empty. The
	// override is made last, as it copies the Kustomize image.
	kustomizeImage := func(kustomizeName string) *Image {
		img := NewImage(image.NewFromIdentifier("pebble=" + upstreamImage + ":~2.0"))
		if kustomizeName != "" {
			img.KustomizeImageName = kustomizeName
			img.ContainerImage.KustomizeImage = image.NewFromIdentifier(kustomizeName)
		}
		img.DeployedImageOverride = newDeployedImage(img.ContainerImage, mirrorImage)
		return img
	}

	t.Run("Kustomize: the mirror is written under the Kustomize image name, and recognised once in the spec", func(t *testing.T) {
		app := kustomizeApp(kustomizeImage(upstreamImage))
		cfg := newCfg(t, app)

		res := UpdateApplication(context.Background(), cfg, NewSyncIterationState())
		assert.Equal(t, 0, res.NumErrors)
		assert.Equal(t, 1, res.NumImagesUpdated)
		assert.Equal(t, v1alpha1.KustomizeImages{upstreamImage + "=" + mirrorImage + ":2.0.1"},
			app.Application.Spec.Source.Kustomize.Images)

		res = UpdateApplication(context.Background(), cfg, NewSyncIterationState())
		assert.Equal(t, 0, res.NumErrors)
		assert.Equal(t, 0, res.NumImagesUpdated)
	})

	t.Run("Kustomize: a write-back already in the spec is recognised", func(t *testing.T) {
		app := kustomizeApp(kustomizeImage(""))
		cfg := newCfg(t, app)

		res := UpdateApplication(context.Background(), cfg, NewSyncIterationState())
		assert.Equal(t, 1, res.NumImagesUpdated)
		assert.Equal(t, v1alpha1.KustomizeImages{mirrorImage + ":2.0.1"}, app.Application.Spec.Source.Kustomize.Images)

		// The application has not synced yet, so the old tag is still running,
		// but the new one is already in the spec and must not be written again.
		res = UpdateApplication(context.Background(), cfg, NewSyncIterationState())
		assert.Equal(t, 0, res.NumErrors)
		assert.Equal(t, 0, res.NumImagesUpdated)
	})

	t.Run("Kustomize: an entry under another Kustomize image name is not taken as already set", func(t *testing.T) {
		app := kustomizeApp(kustomizeImage(""))
		app.Application.Spec.Source.Kustomize.Images = v1alpha1.KustomizeImages{
			v1alpha1.KustomizeImage("other=" + mirrorImage + ":2.0.1"),
		}
		cfg := newCfg(t, app)

		res := UpdateApplication(context.Background(), cfg, NewSyncIterationState())
		assert.Equal(t, 0, res.NumErrors)
		assert.Equal(t, 1, res.NumImagesUpdated)
		// What happens to the entry for "other" is up to SetKustomizeImage,
		// which is not what this test is about.
		assert.Contains(t, app.Application.Spec.Source.Kustomize.Images, v1alpha1.KustomizeImage(mirrorImage+":2.0.1"))

		res = UpdateApplication(context.Background(), cfg, NewSyncIterationState())
		assert.Equal(t, 0, res.NumErrors)
		assert.Equal(t, 0, res.NumImagesUpdated)
	})

	t.Run("Kustomize: an upstream image is not moved without a Kustomize image name", func(t *testing.T) {
		app := kustomizeApp(kustomizeImage(""))
		app.Application.Status.Summary.Images = []string{upstreamImage + ":2.0.0"}

		res := UpdateApplication(context.Background(), newCfg(t, app), NewSyncIterationState())

		assert.Equal(t, 1, res.NumErrors)
		assert.Equal(t, 0, res.NumImagesUpdated)
		assert.Empty(t, app.Application.Spec.Source.Kustomize.Images)
	})
}

func Test_UpdateApplication_DeployedImageOverride_DigestWhileMoving(t *testing.T) {
	const (
		sidecarDigest = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
		stableDigest  = "sha256:2222222222222222222222222222222222222222222222222222222222222222"
	)
	latestDigest := [32]byte{0x33}

	argoClient := &argomock.ArgoCD{}
	argoClient.On("UpdateSpec", mock.Anything, mock.Anything).Return(nil, nil)
	cfg := &UpdateConfiguration{
		NewRegFN: func(endpoint *registry.RegistryEndpoint, username, password string) (registry.RegistryClient, error) {
			regMock := regmock.RegistryClient{}
			regMock.On("NewRepository", mock.Anything, "letsencrypt/pebble").Return(nil)
			regMock.On("Tags", mock.Anything).Return([]string{"sidecar", "stable"}, nil)
			regMock.On("ManifestForTag", mock.Anything, mock.Anything).Return(&schema2.DeserializedManifest{}, nil)
			regMock.On("TagMetadata", mock.Anything, mock.Anything, mock.Anything).Return(&tag.TagInfo{
				CreatedAt: time.Now(),
				Digest:    latestDigest,
			}, nil)
			return &regMock, nil
		},
		ArgoClient: argoClient,
		KubeClient: &kube.ImageUpdaterKubernetesClient{
			KubeClient: &registryKube.KubernetesClient{Clientset: fake.NewFakeKubeClient()},
		},
	}

	img := NewImage(image.NewFromIdentifier("pebble=" + upstreamImage + ":stable"))
	img.UpdateStrategy = image.StrategyDigest
	img.HelmImageName = "image.repository"
	img.HelmImageTag = "image.tag"
	img.DeployedImageOverride = newDeployedImage(img.ContainerImage, mirrorImage)

	// Only the upstream images are live, and another container's image comes
	// first, so the first match for the repository is the wrong container.
	cfg.UpdateApp = &ApplicationImages{
		Application: v1alpha1.Application{
			ObjectMeta: v1.ObjectMeta{Name: "pebble", Namespace: "argocd"},
			Spec: v1alpha1.ApplicationSpec{
				Source: &v1alpha1.ApplicationSource{
					Helm: &v1alpha1.ApplicationSourceHelm{
						Parameters: []v1alpha1.HelmParameter{
							{Name: "image.repository", Value: upstreamImage},
							{Name: "image.tag", Value: "stable@" + stableDigest},
						},
					},
				},
			},
			Status: v1alpha1.ApplicationStatus{
				SourceType: v1alpha1.ApplicationSourceTypeHelm,
				Summary: v1alpha1.ApplicationSummary{Images: []string{
					upstreamImage + ":sidecar@" + sidecarDigest,
					upstreamImage + ":stable@" + stableDigest,
				}},
			},
		},
		WriteBackConfig: &WriteBackConfig{Method: WriteBackApplication},
		Images:          ImageList{img},
	}

	res := UpdateApplication(context.Background(), cfg, NewSyncIterationState())
	assert.Equal(t, 0, res.NumErrors)
	assert.Equal(t, 1, res.NumImagesUpdated)
	require.Len(t, res.Changes, 1)
	assert.Equal(t, "stable", res.Changes[0].OldTag.TagName)
	assert.Equal(t, stableDigest, res.Changes[0].OldTag.TagDigest)
}

func Test_marshalParamsOverride_DeployedImageOverride(t *testing.T) {
	// The values file has the upstream Docker Hub image without a registry.
	const values = "image:\n  repository: bitnami/nginx\n  tag: 1.0.0\n"
	const mirror = "mirror.example/bitnami/nginx"

	img := NewImage(image.NewFromIdentifier("nginx=bitnami/nginx"))
	img.HelmImageName = "image.repository"
	img.HelmImageTag = "image.tag"
	img.DeployedImageOverride = newDeployedImage(img.ContainerImage, mirror)

	app := v1alpha1.Application{
		ObjectMeta: v1.ObjectMeta{Name: "testapp"},
		Spec: v1alpha1.ApplicationSpec{Source: &v1alpha1.ApplicationSource{
			RepoURL:        "https://example.com/example",
			TargetRevision: "main",
			Helm: &v1alpha1.ApplicationSourceHelm{Parameters: []v1alpha1.HelmParameter{
				{Name: "image.repository", Value: mirror, ForceString: true},
				{Name: "image.tag", Value: "1.0.1", ForceString: true},
			}},
		}},
		Status: v1alpha1.ApplicationStatus{
			SourceType: v1alpha1.ApplicationSourceTypeHelm,
			Summary:    v1alpha1.ApplicationSummary{Images: []string{"bitnami/nginx:1.0.0"}},
		},
	}

	result, err := marshalParamsOverride(context.Background(), &ApplicationImages{
		Application:     app,
		Images:          ImageList{img},
		WriteBackConfig: &WriteBackConfig{Method: WriteBackGit, Target: "./values.yaml"},
	}, []byte(values))
	require.NoError(t, err)
	assert.Equal(t, "image:\n  repository: "+mirror+"\n  tag: 1.0.1\n", string(result))
}

// The CRD pattern for deployedImageName approximates the reference grammar
// that validateDeployedImageName checks at runtime. Names the CRD admits but
// the controller rejects are skipped with only a warning in its log.
func Test_DeployedImageName_CRDPatternMatchesRuntimeValidation(t *testing.T) {
	crd, err := os.ReadFile("../../config/crd/bases/argocd-image-updater.argoproj.io_imageupdaters.yaml")
	require.NoError(t, err)
	_, field, found := strings.Cut(string(crd), "deployedImageName:")
	require.True(t, found)
	m := regexp.MustCompile(`(?m)^\s*pattern: (.*)$`).FindStringSubmatch(field)
	require.NotNil(t, m)
	pattern := regexp.MustCompile(m[1])

	for _, name := range []string{
		mirrorImage,
		"mirror.example/team/app",
		"mirror.example:5000/team/app",
		"localhost:5000/app",
		"[fd00::1]:5000/team/app",
		"[fd00::1]/team/app",
		"team/app",
		"app",
		"mirror.example/team/my_app",
		"mirror.example/team/my__app",
		"mirror.example/team/my--app",
		"mirror.example/team/my.app",
		"mirror.example/team//app",
		"mirror.example/team/app/",
		"mirror.example/team/my___app",
		"mirror.example/team/my._app",
		"mirror.example/Team/app",
		"mirror.example/team/app:1.0.0",
		"mirror.example/team/app@sha256:0000000000000000000000000000000000000000000000000000000000000000",
		"app=mirror.example/team/app",
		" mirror.example/team/app",
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, validateDeployedImageName(name) == nil, pattern.MatchString(name))
		})
	}
}

func Test_writesRepository(t *testing.T) {
	app := func(sourceType v1alpha1.ApplicationSourceType) *v1alpha1.Application {
		return &v1alpha1.Application{
			Spec:   v1alpha1.ApplicationSpec{Source: &v1alpha1.ApplicationSource{}},
			Status: v1alpha1.ApplicationStatus{SourceType: sourceType},
		}
	}
	wbc := &WriteBackConfig{Method: WriteBackApplication}

	t.Run("A plugin target writes the repository, whatever the application type", func(t *testing.T) {
		img := NewImage(image.NewFromIdentifier("pebble=" + upstreamImage))
		img.PluginEnvName = "IMAGE"
		assert.True(t, writesRepository(app(v1alpha1.ApplicationSourceTypeDirectory), wbc, img))
	})

	t.Run("Other application types do not", func(t *testing.T) {
		img := NewImage(image.NewFromIdentifier("pebble=" + upstreamImage))
		assert.False(t, writesRepository(app(v1alpha1.ApplicationSourceTypeDirectory), wbc, img))
	})
}
