package argocd

import (
	"fmt"
	"strconv"
	"strings"

	argocdapi "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"

	iuapi "github.com/argoproj-labs/argocd-image-updater/api/v1alpha1"
)

// getImagesFromAnnotations parses the legacy argocd-image-updater.argoproj.io/image-list
// annotation from an ArgoCD Application and converts it into a list of ImageConfig objects.
// Each image can optionally have an alias (format: "alias=image:tag" or just "image:tag").
// For images with aliases, it also parses image-specific settings (update strategy, allow-tags, etc.)
// and manifest targets (Helm/Kustomize parameters) from corresponding annotations.
// Images without aliases will only have ImageName set; their settings will be inherited from
// application-wide annotations when processed.
func getImagesFromAnnotations(app *argocdapi.Application) ([]iuapi.ImageConfig, error) {
	imageListValue, ok := app.Annotations[ImageUpdaterAnnotation]
	if !ok {
		return nil, fmt.Errorf("%s annotation not found on app %s/%s", ImageUpdaterAnnotation, app.Namespace, app.Name)
	}

	aliasImagePairs := strings.Split(imageListValue, ",")
	results := make([]iuapi.ImageConfig, 0, len(aliasImagePairs))
	for _, aliasImagePair := range aliasImagePairs {
		aliasImagePair = strings.TrimSpace(aliasImagePair)
		if aliasImagePair == "" {
			continue
		}
		img := iuapi.ImageConfig{}
		if strings.Contains(aliasImagePair, "=") {
			n := strings.SplitN(aliasImagePair, "=", 2)
			img.Alias = strings.TrimSpace(n[0])
			img.ImageName = strings.TrimSpace(n[1])
			if img.ImageName == "" {
				return nil, fmt.Errorf("empty image name in %s annotation on app %s/%s", ImageUpdaterAnnotation, app.Namespace, app.Name)
			}
			if img.Alias != "" {
				imageUpdateStrategyAnnotations := getImageUpdateStrategyAnnotations(img.Alias)
				var err error
				img.CommonUpdateSettings, err = getCommonUpdateSettingsFromAnnotations(app, imageUpdateStrategyAnnotations)
				if err != nil {
					return nil, fmt.Errorf("parse common update settings for alias %q: %w", img.Alias, err)
				}

				img.ManifestTarget, err = getManifestTargetsFromAnnotations(app, img.Alias)
				if err != nil {
					return nil, fmt.Errorf("parse manifest targets for alias %q: %w", img.Alias, err)
				}
			}
		} else {
			img.ImageName = aliasImagePair
			img.Alias = ""
		}
		results = append(results, img)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("%s annotation is empty on app %s/%s", ImageUpdaterAnnotation, app.Namespace, app.Name)
	}
	return results, nil
}

// getCommonUpdateSettingsFromAnnotations extracts CommonUpdateSettings from an ArgoCD Application's
// annotations based on the provided UpdateStrategyAnnotations mapping. The mapping determines
// whether to read application-wide annotations (when alias is empty) or image-specific annotations
// (when alias is provided). Returns a CommonUpdateSettings struct populated with values from
// the annotations, or an error if parsing fails (e.g., invalid boolean value for force-update).
func getCommonUpdateSettingsFromAnnotations(app *argocdapi.Application, updateStrategyAnnotations UpdateStrategyAnnotations) (*iuapi.CommonUpdateSettings, error) {
	result := &iuapi.CommonUpdateSettings{}
	hasAny := false
	if updateStrategy, ok := app.Annotations[updateStrategyAnnotations.UpdateStrategy]; ok {
		updateStrategy = strings.TrimSpace(updateStrategy)
		if updateStrategy != "" {
			result.UpdateStrategy = &updateStrategy
			hasAny = true
		}
	}

	if forceUpdateStr, ok := app.Annotations[updateStrategyAnnotations.ForceUpdate]; ok {
		forceUpdateStr = strings.TrimSpace(forceUpdateStr)
		val, err := strconv.ParseBool(forceUpdateStr)
		if err == nil {
			result.ForceUpdate = &val
			hasAny = true
		} else {
			return nil, fmt.Errorf("error parsing force update settings: %v", err)
		}
	}
	if allowTags, ok := app.Annotations[updateStrategyAnnotations.AllowTags]; ok {
		allowTags = strings.TrimSpace(allowTags)
		if allowTags != "" {
			result.AllowTags = &allowTags
			hasAny = true
		}
	}

	if ignoreTagsStr, ok := app.Annotations[updateStrategyAnnotations.IgnoreTags]; ok {
		var ignoreTags []string
		for _, ignoreTag := range strings.Split(ignoreTagsStr, ",") {
			ignoreTag = strings.TrimSpace(ignoreTag)
			// Preserve empty strings in ignore-tags to match expected behavior
			ignoreTags = append(ignoreTags, ignoreTag)
		}
		if len(ignoreTags) > 0 {
			result.IgnoreTags = ignoreTags
			hasAny = true
		}
	}

	if pullSecret, ok := app.Annotations[updateStrategyAnnotations.PullSecret]; ok {
		pullSecret = strings.TrimSpace(pullSecret)
		if pullSecret != "" {
			result.PullSecret = &pullSecret
			hasAny = true
		}
	}

	if updateStrategyAnnotations.Platforms != "" {
		if platformsStr, ok := app.Annotations[updateStrategyAnnotations.Platforms]; ok {
			var platforms []string
			for _, platform := range strings.Split(platformsStr, ",") {
				platform = strings.TrimSpace(platform)
				if platform != "" {
					platforms = append(platforms, platform)
				}
			}
			if len(platforms) > 0 {
				result.Platforms = platforms
				hasAny = true
			}
		}
	}

	if !hasAny {
		return nil, nil
	}
	return result, nil
}

// getManifestTargetsFromAnnotations extracts manifest target configuration (Helm or Kustomize
// parameters) from an ArgoCD Application's annotations for a specific image alias.
// It reads Helm parameters (image-name, image-tag, image-spec) and Kustomize image name
// from annotations prefixed with the image alias. Returns a ManifestTarget struct with
// the parsed configuration.
func getManifestTargetsFromAnnotations(app *argocdapi.Application, alias string) (*iuapi.ManifestTarget, error) {
	result := &iuapi.ManifestTarget{}
	hasAny := false

	if kustomizeName, ok := app.Annotations[ImageUpdaterAnnotationPrefix+fmt.Sprintf(KustomizeApplicationNameAnnotationSuffix, alias)]; ok {
		kustomizeName = strings.TrimSpace(kustomizeName)
		if kustomizeName != "" {
			if result.Kustomize == nil {
				result.Kustomize = &iuapi.KustomizeTarget{}
			}
			result.Kustomize.Name = &kustomizeName
			hasAny = true
		}
	}

	if helmName, ok := app.Annotations[ImageUpdaterAnnotationPrefix+fmt.Sprintf(HelmParamImageNameAnnotationSuffix, alias)]; ok {
		helmName = strings.TrimSpace(helmName)
		if helmName != "" {
			if result.Helm == nil {
				result.Helm = &iuapi.HelmTarget{}
			}
			result.Helm.Name = &helmName
			hasAny = true
		}
	}
	if helmTag, ok := app.Annotations[ImageUpdaterAnnotationPrefix+fmt.Sprintf(HelmParamImageTagAnnotationSuffix, alias)]; ok {
		helmTag = strings.TrimSpace(helmTag)
		if helmTag != "" {
			if result.Helm == nil {
				result.Helm = &iuapi.HelmTarget{}
			}
			result.Helm.Tag = &helmTag
			hasAny = true
		}
	}
	if helmSpec, ok := app.Annotations[ImageUpdaterAnnotationPrefix+fmt.Sprintf(HelmParamImageSpecAnnotationSuffix, alias)]; ok {
		helmSpec = strings.TrimSpace(helmSpec)
		if helmSpec != "" {
			if result.Helm == nil {
				result.Helm = &iuapi.HelmTarget{}
			}
			result.Helm.Spec = &helmSpec
			hasAny = true
		}
	}

	if !hasAny {
		return nil, nil
	}
	if result.Helm != nil && result.Kustomize != nil {
		return nil, fmt.Errorf("both helm and kustomize manifest targets configured for alias %q", alias)
	}
	return result, nil
}

// getWriteBackConfigFromAnnotations extracts write-back configuration from an ArgoCD Application's
// annotations. It reads the write-back method (argocd or git) and git-related settings
// (repository, branch, write-back target) from the application's annotations. GitConfig is only
// initialized if at least one git-related annotation is present. Returns a WriteBackConfig struct
// with the parsed configuration, or nil if no annotations are found.
func getWriteBackConfigFromAnnotations(app *argocdapi.Application) *iuapi.WriteBackConfig {
	result := &iuapi.WriteBackConfig{}
	hasAny := false

	if method, ok := app.Annotations[WriteBackMethodAnnotation]; ok {
		method = strings.TrimSpace(method)
		if method != "" {
			result.Method = &method
			hasAny = true
		}
	}

	if target, ok := app.Annotations[WriteBackTargetAnnotation]; ok {
		target = strings.TrimSpace(target)
		if target != "" {
			if result.GitConfig == nil {
				result.GitConfig = &iuapi.GitConfig{}
			}
			result.GitConfig.WriteBackTarget = &target
			hasAny = true
		}
	}

	if gitBranch, ok := app.Annotations[GitBranchAnnotation]; ok {
		gitBranch = strings.TrimSpace(gitBranch)
		if gitBranch != "" {
			if result.GitConfig == nil {
				result.GitConfig = &iuapi.GitConfig{}
			}
			result.GitConfig.Branch = &gitBranch
			hasAny = true
		}
	}

	if gitRepository, ok := app.Annotations[GitRepositoryAnnotation]; ok {
		gitRepository = strings.TrimSpace(gitRepository)
		if gitRepository != "" {
			if result.GitConfig == nil {
				result.GitConfig = &iuapi.GitConfig{}
			}
			result.GitConfig.Repository = &gitRepository
			hasAny = true
		}
	}

	if !hasAny {
		return nil
	}
	return result
}

// ImageUpdaterAnnotationPrefix is the base prefix for all argocd-image-updater annotations.
const ImageUpdaterAnnotationPrefix = "argocd-image-updater.argoproj.io"

// ImageUpdaterAnnotation is an annotation on the application resources to indicate the list of images
// allowed for updates.
const ImageUpdaterAnnotation = ImageUpdaterAnnotationPrefix + "/image-list"

// Application update configuration related annotations
const (
	WriteBackMethodAnnotation = ImageUpdaterAnnotationPrefix + "/write-back-method"
	GitBranchAnnotation       = ImageUpdaterAnnotationPrefix + "/git-branch"
	GitRepositoryAnnotation   = ImageUpdaterAnnotationPrefix + "/git-repository"
	WriteBackTargetAnnotation = ImageUpdaterAnnotationPrefix + "/write-back-target"
)

// Helm related annotations
const (
	HelmParamImageNameAnnotationSuffix = "/%s.helm.image-name"
	HelmParamImageTagAnnotationSuffix  = "/%s.helm.image-tag"
	HelmParamImageSpecAnnotationSuffix = "/%s.helm.image-spec"
)

// KustomizeApplicationNameAnnotationSuffix Kustomize related annotations
const (
	KustomizeApplicationNameAnnotationSuffix = "/%s.kustomize.image-name"
)

// UpdateStrategyAnnotations holds the annotation key mappings for update strategy settings.
type UpdateStrategyAnnotations struct {
	AllowTags      string
	IgnoreTags     string
	ForceUpdate    string
	UpdateStrategy string
	PullSecret     string
	Platforms      string
}

// getImageUpdateStrategyAnnotations returns a mapping of annotation keys to their full annotation
// names for update strategy settings. When alias is empty, it returns application-wide annotation
// keys (e.g., "argocd-image-updater.argoproj.io/update-strategy"). When alias is provided, it
// returns image-specific annotation keys (e.g., "argocd-image-updater.argoproj.io/app.update-strategy").
// Note: Platforms annotation is only available for image-specific annotations (not application-wide)
// to match legacy annotation behavior.
func getImageUpdateStrategyAnnotations(alias string) UpdateStrategyAnnotations {
	if alias == "" {
		return UpdateStrategyAnnotations{
			AllowTags:      ImageUpdaterAnnotationPrefix + "/allow-tags",
			IgnoreTags:     ImageUpdaterAnnotationPrefix + "/ignore-tags",
			ForceUpdate:    ImageUpdaterAnnotationPrefix + "/force-update",
			UpdateStrategy: ImageUpdaterAnnotationPrefix + "/update-strategy",
			PullSecret:     ImageUpdaterAnnotationPrefix + "/pull-secret",
			// Platforms was only image-specific in legacy annotations, not application-wide
		}
	}
	return UpdateStrategyAnnotations{
		AllowTags:      ImageUpdaterAnnotationPrefix + fmt.Sprintf("/%s.allow-tags", alias),
		IgnoreTags:     ImageUpdaterAnnotationPrefix + fmt.Sprintf("/%s.ignore-tags", alias),
		ForceUpdate:    ImageUpdaterAnnotationPrefix + fmt.Sprintf("/%s.force-update", alias),
		UpdateStrategy: ImageUpdaterAnnotationPrefix + fmt.Sprintf("/%s.update-strategy", alias),
		PullSecret:     ImageUpdaterAnnotationPrefix + fmt.Sprintf("/%s.pull-secret", alias),
		Platforms:      ImageUpdaterAnnotationPrefix + fmt.Sprintf("/%s.platforms", alias),
	}
}
