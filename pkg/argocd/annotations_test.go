// Assisted-by: Claude AI

package argocd

import (
	"testing"

	argocdapi "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func Test_getImageUpdateStrategyAnnotations(t *testing.T) {
	t.Run("should return application-wide annotations when alias is empty", func(t *testing.T) {
		result := getImageUpdateStrategyAnnotations("")

		assert.Equal(t, ImageUpdaterAnnotationPrefix+"/allow-tags", result.AllowTags)
		assert.Equal(t, ImageUpdaterAnnotationPrefix+"/ignore-tags", result.IgnoreTags)
		assert.Equal(t, ImageUpdaterAnnotationPrefix+"/force-update", result.ForceUpdate)
		assert.Equal(t, ImageUpdaterAnnotationPrefix+"/update-strategy", result.UpdateStrategy)
		assert.Equal(t, ImageUpdaterAnnotationPrefix+"/pull-secret", result.PullSecret)
		assert.Empty(t, result.Platforms, "Platforms should be empty for application-wide annotations")
	})

	t.Run("should return image-specific annotations when alias is provided", func(t *testing.T) {
		alias := "web"
		result := getImageUpdateStrategyAnnotations(alias)

		assert.Equal(t, ImageUpdaterAnnotationPrefix+"/web.allow-tags", result.AllowTags)
		assert.Equal(t, ImageUpdaterAnnotationPrefix+"/web.ignore-tags", result.IgnoreTags)
		assert.Equal(t, ImageUpdaterAnnotationPrefix+"/web.force-update", result.ForceUpdate)
		assert.Equal(t, ImageUpdaterAnnotationPrefix+"/web.update-strategy", result.UpdateStrategy)
		assert.Equal(t, ImageUpdaterAnnotationPrefix+"/web.pull-secret", result.PullSecret)
		assert.Equal(t, ImageUpdaterAnnotationPrefix+"/web.platforms", result.Platforms)
	})

	t.Run("should handle alias with special characters", func(t *testing.T) {
		alias := "my-app"
		result := getImageUpdateStrategyAnnotations(alias)

		assert.Equal(t, ImageUpdaterAnnotationPrefix+"/my-app.allow-tags", result.AllowTags)
		assert.Equal(t, ImageUpdaterAnnotationPrefix+"/my-app.ignore-tags", result.IgnoreTags)
		assert.Equal(t, ImageUpdaterAnnotationPrefix+"/my-app.force-update", result.ForceUpdate)
		assert.Equal(t, ImageUpdaterAnnotationPrefix+"/my-app.update-strategy", result.UpdateStrategy)
		assert.Equal(t, ImageUpdaterAnnotationPrefix+"/my-app.pull-secret", result.PullSecret)
		assert.Equal(t, ImageUpdaterAnnotationPrefix+"/my-app.platforms", result.Platforms)
	})
}

func Test_getWriteBackConfigFromAnnotations(t *testing.T) {
	t.Run("should return nil when no annotations are present", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
			},
		}

		result := getWriteBackConfigFromAnnotations(app)
		assert.Nil(t, result)
	})

	t.Run("should return nil when all annotation values are empty", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					WriteBackMethodAnnotation: "",
					WriteBackTargetAnnotation: "",
					GitBranchAnnotation:       "",
					GitRepositoryAnnotation:   "",
				},
			},
		}

		result := getWriteBackConfigFromAnnotations(app)
		assert.Nil(t, result)
	})

	t.Run("should return config with only method when only write-back-method is set", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					WriteBackMethodAnnotation: "argocd",
				},
			},
		}

		result := getWriteBackConfigFromAnnotations(app)
		require.NotNil(t, result)
		assert.NotNil(t, result.Method)
		assert.Equal(t, "argocd", *result.Method)
		assert.Nil(t, result.GitConfig)
	})

	t.Run("should trim whitespace from method value", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					WriteBackMethodAnnotation: "  git  ",
				},
			},
		}

		result := getWriteBackConfigFromAnnotations(app)
		require.NotNil(t, result)
		assert.NotNil(t, result.Method)
		assert.Equal(t, "git", *result.Method)
	})

	t.Run("should return config with GitConfig when only write-back-target is set", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					WriteBackTargetAnnotation: "helmvalues:./values.yaml",
				},
			},
		}

		result := getWriteBackConfigFromAnnotations(app)
		require.NotNil(t, result)
		require.NotNil(t, result.GitConfig)
		assert.NotNil(t, result.GitConfig.WriteBackTarget)
		assert.Equal(t, "helmvalues:./values.yaml", *result.GitConfig.WriteBackTarget)
		assert.Nil(t, result.GitConfig.Repository)
		assert.Nil(t, result.GitConfig.Branch)
		assert.Nil(t, result.Method)
	})

	t.Run("should return config with all git-related annotations", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					WriteBackTargetAnnotation: "kustomization:./overlays/prod",
					GitBranchAnnotation:       "main:feature-branch",
					GitRepositoryAnnotation:   "https://github.com/example/repo.git",
				},
			},
		}

		result := getWriteBackConfigFromAnnotations(app)
		require.NotNil(t, result)
		require.NotNil(t, result.GitConfig)
		assert.NotNil(t, result.GitConfig.WriteBackTarget)
		assert.Equal(t, "kustomization:./overlays/prod", *result.GitConfig.WriteBackTarget)
		assert.NotNil(t, result.GitConfig.Branch)
		assert.Equal(t, "main:feature-branch", *result.GitConfig.Branch)
		assert.NotNil(t, result.GitConfig.Repository)
		assert.Equal(t, "https://github.com/example/repo.git", *result.GitConfig.Repository)
	})

	t.Run("should return config with method and all git-related annotations", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					WriteBackMethodAnnotation: "git",
					WriteBackTargetAnnotation: "helmvalues:./helm/values.yaml",
					GitBranchAnnotation:       "main",
					GitRepositoryAnnotation:   "https://github.com/example/repo.git",
				},
			},
		}

		result := getWriteBackConfigFromAnnotations(app)
		require.NotNil(t, result)
		assert.NotNil(t, result.Method)
		assert.Equal(t, "git", *result.Method)
		require.NotNil(t, result.GitConfig)
		assert.NotNil(t, result.GitConfig.WriteBackTarget)
		assert.Equal(t, "helmvalues:./helm/values.yaml", *result.GitConfig.WriteBackTarget)
		assert.NotNil(t, result.GitConfig.Branch)
		assert.Equal(t, "main", *result.GitConfig.Branch)
		assert.NotNil(t, result.GitConfig.Repository)
		assert.Equal(t, "https://github.com/example/repo.git", *result.GitConfig.Repository)
	})

	t.Run("should ignore empty git-related annotations when others are present", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					WriteBackMethodAnnotation: "git",
					WriteBackTargetAnnotation: "helmvalues:./values.yaml",
					GitBranchAnnotation:       "", // Empty, should be ignored
					GitRepositoryAnnotation:   "https://github.com/example/repo.git",
				},
			},
		}

		result := getWriteBackConfigFromAnnotations(app)
		require.NotNil(t, result)
		assert.NotNil(t, result.Method)
		assert.Equal(t, "git", *result.Method)
		require.NotNil(t, result.GitConfig)
		assert.NotNil(t, result.GitConfig.WriteBackTarget)
		assert.Equal(t, "helmvalues:./values.yaml", *result.GitConfig.WriteBackTarget)
		assert.Nil(t, result.GitConfig.Branch) // Should be nil, not empty string
		assert.NotNil(t, result.GitConfig.Repository)
		assert.Equal(t, "https://github.com/example/repo.git", *result.GitConfig.Repository)
	})

	t.Run("should initialize GitConfig only once when multiple git annotations are present", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					WriteBackTargetAnnotation: "helmvalues:./values.yaml",
					GitBranchAnnotation:       "main",
					GitRepositoryAnnotation:   "https://github.com/example/repo.git",
				},
			},
		}

		result := getWriteBackConfigFromAnnotations(app)
		require.NotNil(t, result)
		require.NotNil(t, result.GitConfig)
		// All three git-related fields should be set
		assert.NotNil(t, result.GitConfig.WriteBackTarget)
		assert.NotNil(t, result.GitConfig.Branch)
		assert.NotNil(t, result.GitConfig.Repository)
	})

	t.Run("should handle method with git prefix", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					WriteBackMethodAnnotation: "git:https://github.com/example/repo.git",
				},
			},
		}

		result := getWriteBackConfigFromAnnotations(app)
		require.NotNil(t, result)
		assert.NotNil(t, result.Method)
		assert.Equal(t, "git:https://github.com/example/repo.git", *result.Method)
	})

	t.Run("should handle complex write-back-target with kustomization", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					WriteBackTargetAnnotation: "kustomization:./overlays/production",
				},
			},
		}

		result := getWriteBackConfigFromAnnotations(app)
		require.NotNil(t, result)
		require.NotNil(t, result.GitConfig)
		assert.NotNil(t, result.GitConfig.WriteBackTarget)
		assert.Equal(t, "kustomization:./overlays/production", *result.GitConfig.WriteBackTarget)
	})

	t.Run("should handle git branch with write branch format", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					GitBranchAnnotation: "main:update-branch",
				},
			},
		}

		result := getWriteBackConfigFromAnnotations(app)
		require.NotNil(t, result)
		require.NotNil(t, result.GitConfig)
		assert.NotNil(t, result.GitConfig.Branch)
		assert.Equal(t, "main:update-branch", *result.GitConfig.Branch)
	})
}

func Test_getManifestTargetsFromAnnotations(t *testing.T) {
	t.Run("should return nil when no annotations are present", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
			},
		}

		result, err := getManifestTargetsFromAnnotations(app, "web")
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("should return nil when all annotation values are empty", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotationPrefix + "/web.kustomize.image-name": "",
					ImageUpdaterAnnotationPrefix + "/web.helm.image-name":      "",
					ImageUpdaterAnnotationPrefix + "/web.helm.image-tag":       "",
					ImageUpdaterAnnotationPrefix + "/web.helm.image-spec":      "",
				},
			},
		}

		result, err := getManifestTargetsFromAnnotations(app, "web")
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("should return config with only kustomize name when only kustomize annotation is set", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotationPrefix + "/web.kustomize.image-name": "docker.io/library/nginx",
				},
			},
		}

		result, err := getManifestTargetsFromAnnotations(app, "web")
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.Kustomize)
		assert.NotNil(t, result.Kustomize.Name)
		assert.Equal(t, "docker.io/library/nginx", *result.Kustomize.Name)
		assert.Nil(t, result.Helm)
	})

	t.Run("should return config with only helm name when only helm name annotation is set", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotationPrefix + "/web.helm.image-name": "image.repository",
				},
			},
		}

		result, err := getManifestTargetsFromAnnotations(app, "web")
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.Helm)
		assert.NotNil(t, result.Helm.Name)
		assert.Equal(t, "image.repository", *result.Helm.Name)
		assert.Nil(t, result.Helm.Tag)
		assert.Nil(t, result.Helm.Spec)
		assert.Nil(t, result.Kustomize)
	})

	t.Run("should return config with helm name and tag when both are set", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotationPrefix + "/web.helm.image-name": "image.repository",
					ImageUpdaterAnnotationPrefix + "/web.helm.image-tag":  "image.tag",
				},
			},
		}

		result, err := getManifestTargetsFromAnnotations(app, "web")
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.Helm)
		assert.NotNil(t, result.Helm.Name)
		assert.Equal(t, "image.repository", *result.Helm.Name)
		assert.NotNil(t, result.Helm.Tag)
		assert.Equal(t, "image.tag", *result.Helm.Tag)
		assert.Nil(t, result.Helm.Spec)
		assert.Nil(t, result.Kustomize)
	})

	t.Run("should return config with helm spec when helm spec is set", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotationPrefix + "/web.helm.image-spec": "image.full",
				},
			},
		}

		result, err := getManifestTargetsFromAnnotations(app, "web")
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.Helm)
		assert.NotNil(t, result.Helm.Spec)
		assert.Equal(t, "image.full", *result.Helm.Spec)
		assert.Nil(t, result.Helm.Name)
		assert.Nil(t, result.Helm.Tag)
		assert.Nil(t, result.Kustomize)
	})

	t.Run("should return config with all helm parameters when all are set", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotationPrefix + "/web.helm.image-name": "image.repository",
					ImageUpdaterAnnotationPrefix + "/web.helm.image-tag":  "image.tag",
					ImageUpdaterAnnotationPrefix + "/web.helm.image-spec": "image.full",
				},
			},
		}

		result, err := getManifestTargetsFromAnnotations(app, "web")
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.Helm)
		assert.NotNil(t, result.Helm.Name)
		assert.Equal(t, "image.repository", *result.Helm.Name)
		assert.NotNil(t, result.Helm.Tag)
		assert.Equal(t, "image.tag", *result.Helm.Tag)
		assert.NotNil(t, result.Helm.Spec)
		assert.Equal(t, "image.full", *result.Helm.Spec)
		assert.Nil(t, result.Kustomize)
	})

	t.Run("should return error when both helm and kustomize are configured", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotationPrefix + "/web.kustomize.image-name": "docker.io/library/nginx",
					ImageUpdaterAnnotationPrefix + "/web.helm.image-name":      "image.repository",
				},
			},
		}

		result, err := getManifestTargetsFromAnnotations(app, "web")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "both helm and kustomize manifest targets configured for alias")
		assert.Nil(t, result)
	})

	t.Run("should trim whitespace from kustomize name value", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotationPrefix + "/web.kustomize.image-name": "  docker.io/library/nginx  ",
				},
			},
		}

		result, err := getManifestTargetsFromAnnotations(app, "web")
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.Kustomize)
		assert.NotNil(t, result.Kustomize.Name)
		assert.Equal(t, "docker.io/library/nginx", *result.Kustomize.Name)
	})

	t.Run("should handle different aliases", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotationPrefix + "/backend.kustomize.image-name": "docker.io/library/postgres",
				},
			},
		}

		result, err := getManifestTargetsFromAnnotations(app, "backend")
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.Kustomize)
		assert.NotNil(t, result.Kustomize.Name)
		assert.Equal(t, "docker.io/library/postgres", *result.Kustomize.Name)
	})

	t.Run("should handle alias with special characters", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotationPrefix + "/my-app.helm.image-name": "frontend.image.repository",
				},
			},
		}

		result, err := getManifestTargetsFromAnnotations(app, "my-app")
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.Helm)
		assert.NotNil(t, result.Helm.Name)
		assert.Equal(t, "frontend.image.repository", *result.Helm.Name)
	})

	t.Run("should ignore empty helm annotations when others are present", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotationPrefix + "/web.helm.image-name": "image.repository",
					ImageUpdaterAnnotationPrefix + "/web.helm.image-tag":  "", // Empty, should be ignored
				},
			},
		}

		result, err := getManifestTargetsFromAnnotations(app, "web")
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.Helm)
		assert.NotNil(t, result.Helm.Name)
		assert.Equal(t, "image.repository", *result.Helm.Name)
		assert.Nil(t, result.Helm.Tag) // Should be nil, not empty string
	})

	t.Run("should initialize Helm only once when multiple helm annotations are present", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotationPrefix + "/web.helm.image-name": "image.repository",
					ImageUpdaterAnnotationPrefix + "/web.helm.image-tag":  "image.tag",
				},
			},
		}

		result, err := getManifestTargetsFromAnnotations(app, "web")
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.Helm)
		// Both helm fields should be set
		assert.NotNil(t, result.Helm.Name)
		assert.NotNil(t, result.Helm.Tag)
	})

	t.Run("should handle complex helm name path", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotationPrefix + "/web.helm.image-name": "frontend.deployment.image.name",
				},
			},
		}

		result, err := getManifestTargetsFromAnnotations(app, "web")
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.Helm)
		assert.NotNil(t, result.Helm.Name)
		assert.Equal(t, "frontend.deployment.image.name", *result.Helm.Name)
	})

	t.Run("should handle complex kustomize image name", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotationPrefix + "/web.kustomize.image-name": "quay.io/prometheus/node-exporter",
				},
			},
		}

		result, err := getManifestTargetsFromAnnotations(app, "web")
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.Kustomize)
		assert.NotNil(t, result.Kustomize.Name)
		assert.Equal(t, "quay.io/prometheus/node-exporter", *result.Kustomize.Name)
	})
}

func Test_getCommonUpdateSettingsFromAnnotations(t *testing.T) {
	t.Run("should return nil when no annotations are present", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
			},
		}

		updateStrategyAnnotations := getImageUpdateStrategyAnnotations("")
		result, err := getCommonUpdateSettingsFromAnnotations(app, updateStrategyAnnotations)
		require.NoError(t, err)
		assert.Nil(t, result)
	})

	t.Run("should return config with only update-strategy when only update-strategy is set", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotationPrefix + "/update-strategy": "semver",
				},
			},
		}

		updateStrategyAnnotations := getImageUpdateStrategyAnnotations("")
		result, err := getCommonUpdateSettingsFromAnnotations(app, updateStrategyAnnotations)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.NotNil(t, result.UpdateStrategy)
		assert.Equal(t, "semver", *result.UpdateStrategy)
		assert.Nil(t, result.ForceUpdate)
		assert.Nil(t, result.AllowTags)
		assert.Nil(t, result.IgnoreTags)
		assert.Nil(t, result.PullSecret)
		assert.Nil(t, result.Platforms)
	})

	t.Run("should return config with all settings when all annotations are set", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotationPrefix + "/update-strategy": "latest",
					ImageUpdaterAnnotationPrefix + "/force-update":    "false",
					ImageUpdaterAnnotationPrefix + "/allow-tags":      "v2.*",
					ImageUpdaterAnnotationPrefix + "/ignore-tags":     "dev,test",
					ImageUpdaterAnnotationPrefix + "/pull-secret":     "registry-secret",
				},
			},
		}

		updateStrategyAnnotations := getImageUpdateStrategyAnnotations("")
		result, err := getCommonUpdateSettingsFromAnnotations(app, updateStrategyAnnotations)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.NotNil(t, result.UpdateStrategy)
		assert.Equal(t, "latest", *result.UpdateStrategy)
		assert.NotNil(t, result.ForceUpdate)
		assert.False(t, *result.ForceUpdate)
		assert.NotNil(t, result.AllowTags)
		assert.Equal(t, "v2.*", *result.AllowTags)
		require.NotNil(t, result.IgnoreTags)
		assert.Equal(t, []string{"dev", "test"}, result.IgnoreTags)
		assert.NotNil(t, result.PullSecret)
		assert.Equal(t, "registry-secret", *result.PullSecret)
		assert.Nil(t, result.Platforms)
	})

	t.Run("should return error when force-update has invalid boolean value", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotationPrefix + "/force-update": "invalid",
				},
			},
		}

		updateStrategyAnnotations := getImageUpdateStrategyAnnotations("")
		result, err := getCommonUpdateSettingsFromAnnotations(app, updateStrategyAnnotations)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "error parsing force update settings")
		assert.Nil(t, result)
	})

	t.Run("should handle force-update with true value", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotationPrefix + "/force-update": "true",
				},
			},
		}

		updateStrategyAnnotations := getImageUpdateStrategyAnnotations("")
		result, err := getCommonUpdateSettingsFromAnnotations(app, updateStrategyAnnotations)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.NotNil(t, result.ForceUpdate)
		assert.True(t, *result.ForceUpdate)
	})

	t.Run("should handle force-update with false value", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotationPrefix + "/force-update": "false",
				},
			},
		}

		updateStrategyAnnotations := getImageUpdateStrategyAnnotations("")
		result, err := getCommonUpdateSettingsFromAnnotations(app, updateStrategyAnnotations)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.NotNil(t, result.ForceUpdate)
		assert.False(t, *result.ForceUpdate)
	})

	t.Run("should trim whitespace from ignore-tags values", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotationPrefix + "/ignore-tags": " dev , test , latest ",
				},
			},
		}

		updateStrategyAnnotations := getImageUpdateStrategyAnnotations("")
		result, err := getCommonUpdateSettingsFromAnnotations(app, updateStrategyAnnotations)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.IgnoreTags)
		assert.Equal(t, []string{"dev", "test", "latest"}, result.IgnoreTags)
	})

	t.Run("should handle ignore-tags with single value", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotationPrefix + "/ignore-tags": "latest",
				},
			},
		}

		updateStrategyAnnotations := getImageUpdateStrategyAnnotations("")
		result, err := getCommonUpdateSettingsFromAnnotations(app, updateStrategyAnnotations)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.IgnoreTags)
		assert.Equal(t, []string{"latest"}, result.IgnoreTags)
	})

	t.Run("should handle ignore-tags with empty values", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotationPrefix + "/ignore-tags": "dev,,test",
				},
			},
		}

		updateStrategyAnnotations := getImageUpdateStrategyAnnotations("")
		result, err := getCommonUpdateSettingsFromAnnotations(app, updateStrategyAnnotations)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.IgnoreTags)
		assert.Equal(t, []string{"dev", "", "test"}, result.IgnoreTags)
	})

	t.Run("should handle image-specific annotations when alias is provided", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotationPrefix + "/web.update-strategy": "digest",
					ImageUpdaterAnnotationPrefix + "/web.force-update":    "true",
					ImageUpdaterAnnotationPrefix + "/web.allow-tags":      "v3.*",
					ImageUpdaterAnnotationPrefix + "/web.ignore-tags":     "rc,alpha",
					ImageUpdaterAnnotationPrefix + "/web.pull-secret":     "web-secret",
					ImageUpdaterAnnotationPrefix + "/web.platforms":       "linux/amd64,linux/arm64",
				},
			},
		}

		updateStrategyAnnotations := getImageUpdateStrategyAnnotations("web")
		result, err := getCommonUpdateSettingsFromAnnotations(app, updateStrategyAnnotations)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.NotNil(t, result.UpdateStrategy)
		assert.Equal(t, "digest", *result.UpdateStrategy)
		assert.NotNil(t, result.ForceUpdate)
		assert.True(t, *result.ForceUpdate)
		assert.NotNil(t, result.AllowTags)
		assert.Equal(t, "v3.*", *result.AllowTags)
		require.NotNil(t, result.IgnoreTags)
		assert.Equal(t, []string{"rc", "alpha"}, result.IgnoreTags)
		assert.NotNil(t, result.PullSecret)
		assert.Equal(t, "web-secret", *result.PullSecret)
		require.NotNil(t, result.Platforms)
		assert.Equal(t, []string{"linux/amd64", "linux/arm64"}, result.Platforms)
	})

	t.Run("should handle platforms annotation for image-specific annotations", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotationPrefix + "/web.platforms": "linux/amd64,linux/arm64,linux/arm",
				},
			},
		}

		updateStrategyAnnotations := getImageUpdateStrategyAnnotations("web")
		result, err := getCommonUpdateSettingsFromAnnotations(app, updateStrategyAnnotations)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.Platforms)
		assert.Equal(t, []string{"linux/amd64", "linux/arm64", "linux/arm"}, result.Platforms)
	})

	t.Run("should trim whitespace from platforms values", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotationPrefix + "/web.platforms": " linux/amd64 , linux/arm64 ",
				},
			},
		}

		updateStrategyAnnotations := getImageUpdateStrategyAnnotations("web")
		result, err := getCommonUpdateSettingsFromAnnotations(app, updateStrategyAnnotations)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.Platforms)
		assert.Equal(t, []string{"linux/amd64", "linux/arm64"}, result.Platforms)
	})

	t.Run("should handle platforms with single value", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotationPrefix + "/web.platforms": "linux/amd64",
				},
			},
		}

		updateStrategyAnnotations := getImageUpdateStrategyAnnotations("web")
		result, err := getCommonUpdateSettingsFromAnnotations(app, updateStrategyAnnotations)
		require.NoError(t, err)
		require.NotNil(t, result)
		require.NotNil(t, result.Platforms)
		assert.Equal(t, []string{"linux/amd64"}, result.Platforms)
	})

	t.Run("should not include platforms for application-wide annotations", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotationPrefix + "/update-strategy": "semver",
				},
			},
		}

		updateStrategyAnnotations := getImageUpdateStrategyAnnotations("")
		result, err := getCommonUpdateSettingsFromAnnotations(app, updateStrategyAnnotations)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.NotNil(t, result.UpdateStrategy)
		assert.Equal(t, "semver", *result.UpdateStrategy)
		assert.Nil(t, result.Platforms, "Platforms should be nil for application-wide annotations")
	})

	t.Run("should handle alias with special characters", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotationPrefix + "/my-app.update-strategy": "name",
					ImageUpdaterAnnotationPrefix + "/my-app.force-update":    "true",
				},
			},
		}

		updateStrategyAnnotations := getImageUpdateStrategyAnnotations("my-app")
		result, err := getCommonUpdateSettingsFromAnnotations(app, updateStrategyAnnotations)
		require.NoError(t, err)
		require.NotNil(t, result)
		assert.NotNil(t, result.UpdateStrategy)
		assert.Equal(t, "name", *result.UpdateStrategy)
		assert.NotNil(t, result.ForceUpdate)
		assert.True(t, *result.ForceUpdate)
	})
}

func Test_getImagesFromAnnotations(t *testing.T) {
	t.Run("should return error when image-list annotation is not present", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
			},
		}

		result, err := getImagesFromAnnotations(app)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "annotation not found")
		assert.Nil(t, result)
	})

	t.Run("should return error when image-list annotation is empty", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotation: "",
				},
			},
		}

		result, err := getImagesFromAnnotations(app)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "annotation is empty")
		assert.Nil(t, result)
	})

	t.Run("should return error when image-list annotation contains only whitespace", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotation: "  ,  ,  ",
				},
			},
		}

		result, err := getImagesFromAnnotations(app)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "annotation is empty")
		assert.Nil(t, result)
	})

	t.Run("should parse single image without alias", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotation: "docker.io/library/nginx:1.21",
				},
			},
		}

		result, err := getImagesFromAnnotations(app)
		require.NoError(t, err)
		require.Len(t, result, 1)
		assert.Equal(t, "docker.io/library/nginx:1.21", result[0].ImageName)
		assert.Empty(t, result[0].Alias)
		assert.Nil(t, result[0].CommonUpdateSettings)
		assert.Nil(t, result[0].ManifestTarget)
	})

	t.Run("should parse single image with alias", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotation: "web=docker.io/library/nginx:1.21",
				},
			},
		}

		result, err := getImagesFromAnnotations(app)
		require.NoError(t, err)
		require.Len(t, result, 1)
		assert.Equal(t, "web", result[0].Alias)
		assert.Equal(t, "docker.io/library/nginx:1.21", result[0].ImageName)
		// CommonUpdateSettings is nil when no annotations are present
		assert.Nil(t, result[0].CommonUpdateSettings)
		// ManifestTarget is nil when no annotations are present
		assert.Nil(t, result[0].ManifestTarget)
	})

	t.Run("should parse multiple images without aliases", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotation: "docker.io/library/nginx:1.21,quay.io/prometheus/node-exporter:v1.5.0",
				},
			},
		}

		result, err := getImagesFromAnnotations(app)
		require.NoError(t, err)
		require.Len(t, result, 2)
		assert.Equal(t, "docker.io/library/nginx:1.21", result[0].ImageName)
		assert.Empty(t, result[0].Alias)
		assert.Equal(t, "quay.io/prometheus/node-exporter:v1.5.0", result[1].ImageName)
		assert.Empty(t, result[1].Alias)
	})

	t.Run("should parse multiple images with aliases", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotation: "web=docker.io/library/nginx:1.21,api=quay.io/myorg/api:v2.0",
				},
			},
		}

		result, err := getImagesFromAnnotations(app)
		require.NoError(t, err)
		require.Len(t, result, 2)
		assert.Equal(t, "web", result[0].Alias)
		assert.Equal(t, "docker.io/library/nginx:1.21", result[0].ImageName)
		assert.Equal(t, "api", result[1].Alias)
		assert.Equal(t, "quay.io/myorg/api:v2.0", result[1].ImageName)
	})

	t.Run("should parse mixed images with and without aliases", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotation: "web=docker.io/library/nginx:1.21,docker.io/library/postgres:14,api=quay.io/myorg/api:v2.0",
				},
			},
		}

		result, err := getImagesFromAnnotations(app)
		require.NoError(t, err)
		require.Len(t, result, 3)
		assert.Equal(t, "web", result[0].Alias)
		assert.Equal(t, "docker.io/library/nginx:1.21", result[0].ImageName)
		assert.Empty(t, result[1].Alias)
		assert.Equal(t, "docker.io/library/postgres:14", result[1].ImageName)
		assert.Equal(t, "api", result[2].Alias)
		assert.Equal(t, "quay.io/myorg/api:v2.0", result[2].ImageName)
	})

	t.Run("should trim whitespace from image entries", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotation: "  web = docker.io/library/nginx:1.21  ,  api = quay.io/myorg/api:v2.0  ",
				},
			},
		}

		result, err := getImagesFromAnnotations(app)
		require.NoError(t, err)
		require.Len(t, result, 2)
		assert.Equal(t, "web", result[0].Alias)
		assert.Equal(t, "docker.io/library/nginx:1.21", result[0].ImageName)
		assert.Equal(t, "api", result[1].Alias)
		assert.Equal(t, "quay.io/myorg/api:v2.0", result[1].ImageName)
	})

	t.Run("should skip empty entries between commas", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotation: "web=docker.io/library/nginx:1.21,,api=quay.io/myorg/api:v2.0",
				},
			},
		}

		result, err := getImagesFromAnnotations(app)
		require.NoError(t, err)
		require.Len(t, result, 2)
		assert.Equal(t, "web", result[0].Alias)
		assert.Equal(t, "docker.io/library/nginx:1.21", result[0].ImageName)
		assert.Equal(t, "api", result[1].Alias)
		assert.Equal(t, "quay.io/myorg/api:v2.0", result[1].ImageName)
	})

	t.Run("should return error when image name is empty after alias", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotation: "web=",
				},
			},
		}

		result, err := getImagesFromAnnotations(app)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "empty image name")
		assert.Nil(t, result)
	})

	t.Run("should parse image with alias and include common update settings", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotation:                                "web=docker.io/library/nginx:1.21",
					ImageUpdaterAnnotationPrefix + "/web.update-strategy": "semver",
					ImageUpdaterAnnotationPrefix + "/web.force-update":    "true",
					ImageUpdaterAnnotationPrefix + "/web.allow-tags":      "v1.*",
				},
			},
		}

		result, err := getImagesFromAnnotations(app)
		require.NoError(t, err)
		require.Len(t, result, 1)
		assert.Equal(t, "web", result[0].Alias)
		assert.Equal(t, "docker.io/library/nginx:1.21", result[0].ImageName)
		require.NotNil(t, result[0].CommonUpdateSettings)
		assert.NotNil(t, result[0].CommonUpdateSettings.UpdateStrategy)
		assert.Equal(t, "semver", *result[0].CommonUpdateSettings.UpdateStrategy)
		assert.NotNil(t, result[0].CommonUpdateSettings.ForceUpdate)
		assert.True(t, *result[0].CommonUpdateSettings.ForceUpdate)
		assert.NotNil(t, result[0].CommonUpdateSettings.AllowTags)
		assert.Equal(t, "v1.*", *result[0].CommonUpdateSettings.AllowTags)
	})

	t.Run("should parse image with alias and include manifest targets", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotation:                                "web=docker.io/library/nginx:1.21",
					ImageUpdaterAnnotationPrefix + "/web.helm.image-name": "image.repository",
					ImageUpdaterAnnotationPrefix + "/web.helm.image-tag":  "image.tag",
				},
			},
		}

		result, err := getImagesFromAnnotations(app)
		require.NoError(t, err)
		require.Len(t, result, 1)
		assert.Equal(t, "web", result[0].Alias)
		assert.Equal(t, "docker.io/library/nginx:1.21", result[0].ImageName)
		require.NotNil(t, result[0].ManifestTarget)
		require.NotNil(t, result[0].ManifestTarget.Helm)
		assert.NotNil(t, result[0].ManifestTarget.Helm.Name)
		assert.Equal(t, "image.repository", *result[0].ManifestTarget.Helm.Name)
		assert.NotNil(t, result[0].ManifestTarget.Helm.Tag)
		assert.Equal(t, "image.tag", *result[0].ManifestTarget.Helm.Tag)
	})

	t.Run("should parse image with alias and include kustomize manifest target", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotation: "web=docker.io/library/nginx:1.21",
					ImageUpdaterAnnotationPrefix + "/web.kustomize.image-name": "docker.io/library/nginx",
				},
			},
		}

		result, err := getImagesFromAnnotations(app)
		require.NoError(t, err)
		require.Len(t, result, 1)
		assert.Equal(t, "web", result[0].Alias)
		assert.Equal(t, "docker.io/library/nginx:1.21", result[0].ImageName)
		require.NotNil(t, result[0].ManifestTarget)
		require.NotNil(t, result[0].ManifestTarget.Kustomize)
		assert.NotNil(t, result[0].ManifestTarget.Kustomize.Name)
		assert.Equal(t, "docker.io/library/nginx", *result[0].ManifestTarget.Kustomize.Name)
	})

	t.Run("should return error when common update settings parsing fails", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotation:                             "web=docker.io/library/nginx:1.21",
					ImageUpdaterAnnotationPrefix + "/web.force-update": "invalid-boolean",
				},
			},
		}

		result, err := getImagesFromAnnotations(app)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse common update settings")
		assert.Nil(t, result)
	})

	t.Run("should return error when manifest targets parsing fails", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotation:                                     "web=docker.io/library/nginx:1.21",
					ImageUpdaterAnnotationPrefix + "/web.helm.image-name":      "image.repository",
					ImageUpdaterAnnotationPrefix + "/web.kustomize.image-name": "docker.io/library/nginx",
				},
			},
		}

		result, err := getImagesFromAnnotations(app)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "parse manifest targets")
		assert.Nil(t, result)
	})

	t.Run("should handle alias with special characters", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotation: "my-app=docker.io/library/nginx:1.21",
				},
			},
		}

		result, err := getImagesFromAnnotations(app)
		require.NoError(t, err)
		require.Len(t, result, 1)
		assert.Equal(t, "my-app", result[0].Alias)
		assert.Equal(t, "docker.io/library/nginx:1.21", result[0].ImageName)
	})

	t.Run("should handle image with registry and tag", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotation: "quay.io/prometheus/node-exporter:v1.5.0",
				},
			},
		}

		result, err := getImagesFromAnnotations(app)
		require.NoError(t, err)
		require.Len(t, result, 1)
		assert.Equal(t, "quay.io/prometheus/node-exporter:v1.5.0", result[0].ImageName)
		assert.Empty(t, result[0].Alias)
	})

	t.Run("should handle image with digest", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotation: "web=docker.io/library/nginx@sha256:abc123",
				},
			},
		}

		result, err := getImagesFromAnnotations(app)
		require.NoError(t, err)
		require.Len(t, result, 1)
		assert.Equal(t, "web", result[0].Alias)
		assert.Equal(t, "docker.io/library/nginx@sha256:abc123", result[0].ImageName)
	})

	t.Run("should handle alias with empty string after trimming", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotation: "  =docker.io/library/nginx:1.21",
				},
			},
		}

		result, err := getImagesFromAnnotations(app)
		require.NoError(t, err)
		require.Len(t, result, 1)
		assert.Empty(t, result[0].Alias)
		assert.Equal(t, "docker.io/library/nginx:1.21", result[0].ImageName)
		// When alias is empty after trimming, it should not parse update settings
		// So both should be nil (not initialized)
		assert.Nil(t, result[0].CommonUpdateSettings)
		assert.Nil(t, result[0].ManifestTarget)
	})

	t.Run("should handle complex scenario with multiple images and all settings", func(t *testing.T) {
		app := &argocdapi.Application{
			ObjectMeta: v1.ObjectMeta{
				Name:      "test-app",
				Namespace: "argocd",
				Annotations: map[string]string{
					ImageUpdaterAnnotation:                                     "web=docker.io/library/nginx:1.21,api=quay.io/myorg/api:v2.0,db=docker.io/library/postgres:14",
					ImageUpdaterAnnotationPrefix + "/web.update-strategy":      "semver",
					ImageUpdaterAnnotationPrefix + "/web.helm.image-name":      "image.repository",
					ImageUpdaterAnnotationPrefix + "/api.force-update":         "true",
					ImageUpdaterAnnotationPrefix + "/api.kustomize.image-name": "quay.io/myorg/api",
				},
			},
		}

		result, err := getImagesFromAnnotations(app)
		require.NoError(t, err)
		require.Len(t, result, 3)

		// First image: web with helm target
		assert.Equal(t, "web", result[0].Alias)
		assert.Equal(t, "docker.io/library/nginx:1.21", result[0].ImageName)
		require.NotNil(t, result[0].CommonUpdateSettings)
		assert.NotNil(t, result[0].CommonUpdateSettings.UpdateStrategy)
		assert.Equal(t, "semver", *result[0].CommonUpdateSettings.UpdateStrategy)
		require.NotNil(t, result[0].ManifestTarget)
		require.NotNil(t, result[0].ManifestTarget.Helm)
		assert.Equal(t, "image.repository", *result[0].ManifestTarget.Helm.Name)

		// Second image: api with kustomize target
		assert.Equal(t, "api", result[1].Alias)
		assert.Equal(t, "quay.io/myorg/api:v2.0", result[1].ImageName)
		require.NotNil(t, result[1].CommonUpdateSettings)
		assert.NotNil(t, result[1].CommonUpdateSettings.ForceUpdate)
		assert.True(t, *result[1].CommonUpdateSettings.ForceUpdate)
		require.NotNil(t, result[1].ManifestTarget)
		require.NotNil(t, result[1].ManifestTarget.Kustomize)
		assert.Equal(t, "quay.io/myorg/api", *result[1].ManifestTarget.Kustomize.Name)

		// Third image: db without specific settings
		assert.Equal(t, "db", result[2].Alias)
		assert.Equal(t, "docker.io/library/postgres:14", result[2].ImageName)
		// CommonUpdateSettings is nil when no annotations are present
		assert.Nil(t, result[2].CommonUpdateSettings)
		// ManifestTarget is nil when no annotations are present
		assert.Nil(t, result[2].ManifestTarget)
	})
}
