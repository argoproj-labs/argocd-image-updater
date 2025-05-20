/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ImageUpdaterSpec defines the desired state of ImageUpdater
// It specifies which applications to target, default update strategies,
// and a list of images to manage.
type ImageUpdaterSpec struct {
	// Namespace indicates the target namespace of the applications.
	// This is the namespace where the controller will look for Argo CD Applications
	// matching the criteria in ApplicationRefs.
	// +kubebuilder:validation:Required
	Namespace string `json:"namespace"`

	// CommonUpdateSettings provides global default settings for update strategies,
	// tag filtering, pull secrets, etc., for all applications matched by this CR.
	// These can be overridden at the ApplicationRef or ImageConfig level.
	// +optional
	*CommonUpdateSettings `json:"commonUpdateSettings,omitempty"`

	// WriteBackConfig provides global default settings for how and where to write back image updates.
	// This can be overridden at the ApplicationRef level.
	// +optional
	*WriteBackConfig `json:"writeBackConfig,omitempty"`

	// ApplicationRefs indicates the set of applications to be managed.
	// ApplicationRefs is a list of rules to select Argo CD Applications within the `spec.namespace`.
	// Each reference can also provide specific overrides for the global settings defined above.
	// +kubebuilder:validation:MinItems=1
	ApplicationRefs []ApplicationRef `json:"applicationRefs"`

	// Images contains a list of configurations that how images should be updated.
	// These rules apply to all applications selected by ApplicationRefs, and each
	// image can override global/ApplicationRef settings.
	// +kubebuilder:validation:MinItems=1
	Images []ImageConfig `json:"images"`
}

// ApplicationRef contains various criteria by which to include applications for managing by image updater
type ApplicationRef struct {
	// NamePattern indicates the glob pattern for application name
	// +kubebuilder:validation:Required
	NamePattern string `json:"namePattern"`

	// LabelSelectors indicates the label selectors to apply for application selection
	// +optional
	LabelSelectors *metav1.LabelSelector `json:"labelSelectors,omitempty"`

	// --- Overrides for spec-level settings, specific to THIS ApplicationRef ---

	// CommonUpdateSettings overrides the global CommonUpdateSettings for applications
	// matched by this selector.
	// +optional
	*CommonUpdateSettings `json:"commonUpdateSettings,omitempty"`

	// WriteBackConfig overrides the global WriteBackConfig settings for applications
	// matched by this selector.
	// +optional
	*WriteBackConfig `json:"writeBackConfig,omitempty"`
}

// GitConfig defines parameters for Git interaction when `writeBackMethod` involves Git.
type GitConfig struct {
	// Repository URL to commit changes to.
	// If not specified here or at the spec level, the controller MUST infer it from the
	// Argo CD Application's `spec.source.repoURL`. This field allows overriding that.
	// +optional
	Repository string `json:"repository,omitempty"`

	// Branch to commit updates to.
	// Required if write-back method is Git and this is not specified at the spec level.
	// +optional
	Branch string `json:"branch,omitempty"`

	// WriteBackTarget defines the path and type of file to update in the Git repository.
	// Examples: "helmvalues:./helm/values.yaml", "kustomization:./kustomize/overlays/production".
	// For ApplicationSet usage, `{{ .app.path.path }}` should be resolved by ApplicationSet
	// before this CR is generated, resulting in a concrete path here.
	// Required if write-back method is Git and this is not specified at the spec level.
	// +optional
	WriteBackTarget string `json:"writeBackTarget,omitempty"`
}

// ImageConfig defines how a specific container image should be discovered, updated,
// and how those updates should be reflected in application manifests.
type ImageConfig struct {
	// Alias is a short, user-defined name for this image configuration.
	// This field is mandatory.
	// +kubebuilder:validation:Required
	Alias string `json:"alias"`

	// ImageName is the full identifier of the image to be tracked,
	// including the registry (if not Docker Hub), the image name, and an initial/current tag or version.
	// This is the string used to query the container registry and also as a base for finding updates.
	// Example: "docker.io/library/nginx:1.17.10", "quay.io/prometheus/node-exporter:v1.5.0".
	// This field is mandatory.
	// +kubebuilder:validation:Required
	ImageName string `json:"imageName"`

	// --- Overrides for spec-level or ApplicationRef-level defaults, specific to THIS image ---

	// CommonUpdateSettings overrides the effective default CommonUpdateSettings for this specific image.
	// +optional
	*CommonUpdateSettings `json:"commonUpdateSettings,omitempty"`

	// Platforms specifies a list of target platforms (e.g., "linux/amd64", "linux/arm64").
	// If specified, the image updater will consider these platforms when checking for new versions or digests.
	// +listType=atomic
	// +optional
	Platforms []string `json:"platforms,omitempty"`

	// ManifestTarget defines how and where to update this image in Kubernetes manifests.
	// Only one of Helm or Kustomize should be specified within this block.
	// This whole block is optional if the image update isn't written to a manifest in a structured way.
	// +optional
	*ManifestTarget `json:"manifestTargets,omitempty"`
}

// CommonUpdateSettings groups common update strategy settings that can be applied
// globally, per ApplicationRef, or per ImageConfig.
type CommonUpdateSettings struct {
	// UpdateStrategy defines the update strategy to apply.
	// Examples: "semver", "latest", "digest", "name".
	// This acts as the default if not overridden at a more specific level.
	// +optional
	// +kubebuilder:default:="semver"
	UpdateStrategy string `json:"updateStrategy,omitempty"`

	// ForceUpdate specifies whether updates should be forced.
	// This acts as the default if not overridden.
	// +optional
	// +kubebuilder:default:=false
	ForceUpdate bool `json:"forceUpdate,omitempty"`

	// AllowTags is a regex pattern for tags to allow.
	// This acts as the default if not overridden.
	// +optional
	AllowTags string `json:"allowTags,omitempty"`

	// IgnoreTags is a list of glob-like patterns of tags to ignore.
	// This acts as the default and can be overridden at more specific levels.
	// +listType=atomic
	// +optional
	IgnoreTags []string `json:"ignoreTags,omitempty"`

	// PullSecret is the pull secret to use for images.
	// This acts as the default if not overridden.
	// +optional
	PullSecret string `json:"pullSecret,omitempty"`
}

// WriteBackConfig defines how and where to write back image updates.
// It includes the method (e.g., git, direct Application update) and
// specific configurations for that method, like Git settings.
type WriteBackConfig struct {
	// Method defines the method for writing back updated image versions.
	// This acts as the default if not overridden.
	// +optional
	// +kubebuilder:default:="argocd"
	Method string `json:"method,omitempty"`

	// GitConfig provides Git configuration settings if the write-back method involves Git.
	// +optional
	*GitConfig `json:"gitConfig,omitempty"`
}

// ManifestTarget specifies the mechanism and details for updating image references in application manifests.
// Only one of the fields (Helm, Kustomize) should be set, dictating the update method.
// +kubebuilder:validation:XValidation:rule="has(self.helm) ? !has(self.kustomize) : has(self.kustomize)",message="Exactly one of helm or kustomize must be specified within manifestTargets if the block is present."
type ManifestTarget struct {
	// Helm specifies update parameters if the target manifest is managed by Helm
	// and updates are to be made to Helm values files.
	// +optional
	Helm *HelmTarget `json:"helm,omitempty"`

	// Kustomize specifies update parameters if the target manifest is managed by Kustomize
	// and updates involve changing image tags in Kustomize configurations.
	// +optional
	Kustomize *KustomizeTarget `json:"kustomize,omitempty"`
}

// HelmTarget defines parameters for updating image references within Helm values.
type HelmTarget struct {
	// Name is the dot-separated path to the Helm key for the image repository/name part.
	// Example: "image.repository", "frontend.deployment.image.name".
	// This field is required if the Helm target is used.
	Name string `json:"name"`

	// Tag is the dot-separated path to the Helm key for the image tag part.
	// Example: "image.tag", "frontend.deployment.image.version".
	// This field is required if the Helm target is used.
	Tag string `json:"tag"`

	// Spec is an optional dot-separated path to a Helm key where the full image string
	// (e.g., "image/name:1.0") should be written.
	// Use this if your Helm chart expects the entire image reference in a single field,
	// rather than separate name/tag fields. If this is set, other Helm parameter-related
	// options will be ignored.
	// +optional
	Spec string `json:"spec,omitempty"`
}

// KustomizeTarget defines parameters for updating image references within Kustomize configurations.
type KustomizeTarget struct {
	// Name is the image name (which can include the registry and an initial tag)
	// as it appears in the `images` list of a kustomization.yaml file that needs to be updated.
	// The updater will typically change the tag or add a digest to this entry.
	// Example: "docker.io/library/nginx".
	// This field is required if the Kustomize target is used.
	Name string `json:"name"`
}

//------------------------Status---------------------------------------------//

// ImageUpdaterStatus defines the observed state of ImageUpdater
type ImageUpdaterStatus struct {
	// Important: Run "make" to regenerate code after modifying this file

	// LastUpdatedAt indicates when the image updater last ran
	LastUpdatedAt *metav1.Time `json:"reconciledAt,omitempty"`

	// ImageStatus indicates the detailed status for the list of managed images
	ImageStatus []ImageStatus `json:"imageStatus,omitempty"`

	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ImageStatus contains information for an image:version and its update status in hosting applications
type ImageStatus struct {
	// Name indicates the image name
	Name string `json:"name"`

	// Version indicates the image version
	Version string `json:"version"`

	// Applications contains a list of applications and when the image was last updated therein
	Applications []ImageApplicationLastUpdated `json:"applications,omitempty"`
}

// ImageApplicationLastUpdated contains information for an application and when the image was last updated therein
type ImageApplicationLastUpdated struct {
	// AppName indicates and namespace and the application name
	AppName string `json:"appName"`

	// LastUpdatedAt indicates when the image in this application was last updated
	LastUpdatedAt metav1.Time `json:"lastUpdatedAt,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// ImageUpdater is the Schema for the imageupdaters API
type ImageUpdater struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ImageUpdaterSpec   `json:"spec,omitempty"`
	Status ImageUpdaterStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// ImageUpdaterList contains a list of ImageUpdater
type ImageUpdaterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ImageUpdater `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ImageUpdater{}, &ImageUpdaterList{})
}
