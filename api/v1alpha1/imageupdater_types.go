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
	//
	// Deprecated: This field is deprecated and will be removed in a future release.
	// The controller now uses the ImageUpdater CR's namespace (metadata.namespace)
	// to determine which namespace to search for applications. This field is ignored.
	// +optional
	Namespace *string `json:"namespace,omitempty"`

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
	// ApplicationRefs is a list of rules to select Argo CD Applications within the ImageUpdater CR's namespace.
	// Each reference can also provide specific overrides for the global settings defined above.
	// +kubebuilder:validation:MinItems=1
	// +listType=map
	// +listMapKey=namePattern
	ApplicationRefs []ApplicationRef `json:"applicationRefs"`
}

// ApplicationRef contains various criteria by which to include applications for managing by image updater
// +kubebuilder:validation:XValidation:rule="!(has(self.useAnnotations) && self.useAnnotations == true) ? (has(self.images) && size(self.images) > 0) : true",message="Either useAnnotations must be true, or images must be provided with at least one item"
type ApplicationRef struct {
	// NamePattern indicates the glob pattern for application name
	// +kubebuilder:validation:Required
	NamePattern string `json:"namePattern"`

	// LabelSelectors indicates the label selectors to apply for application selection
	// +optional
	LabelSelectors *metav1.LabelSelector `json:"labelSelectors,omitempty"`

	// UseAnnotations When true, read image configuration from Application's
	// argocd-image-updater.argoproj.io/* annotations instead of
	// requiring explicit Images[] configuration in this CR.
	// When this field is set to true, only namePattern and labelSelectors are used for
	// application selection. All other fields (CommonUpdateSettings, WriteBackConfig, Images)
	// are ignored.
	// +optional
	// +kubebuilder:default:=false
	UseAnnotations *bool `json:"useAnnotations,omitempty"`

	// --- Overrides for spec-level settings, specific to THIS ApplicationRef ---
	// NOTE: These fields are ignored when UseAnnotations is true.
	// Only namePattern and labelSelectors are used in that case.

	// CommonUpdateSettings overrides the global CommonUpdateSettings for applications
	// matched by this selector.
	// This field is ignored when UseAnnotations is true.
	// +optional
	*CommonUpdateSettings `json:"commonUpdateSettings,omitempty"`

	// WriteBackConfig overrides the global WriteBackConfig settings for applications
	// matched by this selector.
	// This field is ignored when UseAnnotations is true.
	// +optional
	*WriteBackConfig `json:"writeBackConfig,omitempty"`

	// Images contains a list of configurations that how images should be updated.
	// These rules apply to applications selected by namePattern in ApplicationRefs, and each
	// image can override global/ApplicationRef settings.
	// This field is ignored when UseAnnotations is true.
	// +optional
	// +listType=map
	// +listMapKey=alias
	Images []ImageConfig `json:"images,omitempty"`
}

// GitConfig defines parameters for Git interaction when `writeBackMethod` involves Git.
type GitConfig struct {
	// Repository URL to commit changes to.
	// If not specified here or at the spec level, the controller MUST infer it from the
	// Argo CD Application's `spec.source.repoURL`. This field allows overriding that.
	// +optional
	Repository *string `json:"repository,omitempty"`

	// Branch to commit updates to.
	// Required if write-back method is Git and this is not specified at the spec level.
	// +optional
	Branch *string `json:"branch,omitempty"`

	// WriteBackTarget defines the path and type of file to update in the Git repository.
	// Examples: "helmvalues:./helm/values.yaml", "kustomization:./kustomize/overlays/production".
	// For ApplicationSet usage, `{{ .app.path.path }}` should be resolved by ApplicationSet
	// before this CR is generated, resulting in a concrete path here.
	// Required if write-back method is Git and this is not specified at the spec level.
	// +optional
	WriteBackTarget *string `json:"writeBackTarget,omitempty"`
}

// ImageConfig defines how a specific container image should be discovered, updated,
// and how those updates should be reflected in application manifests.
type ImageConfig struct {
	// Alias is a short, user-defined name for this image configuration.
	// It MUST be unique within a single ApplicationRef's list of images.
	// This field is mandatory.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9][a-zA-Z0-9-._]*$`
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
	UpdateStrategy *string `json:"updateStrategy,omitempty"`

	// ForceUpdate specifies whether updates should be forced.
	// This acts as the default if not overridden.
	// +optional
	// +kubebuilder:default:=false
	ForceUpdate *bool `json:"forceUpdate,omitempty"`

	// AllowTags is a regex pattern for tags to allow.
	// This acts as the default if not overridden.
	// +optional
	AllowTags *string `json:"allowTags,omitempty"`

	// IgnoreTags is a list of glob-like patterns of tags to ignore.
	// This acts as the default and can be overridden at more specific levels.
	// +listType=atomic
	// +optional
	IgnoreTags []string `json:"ignoreTags,omitempty"`

	// PullSecret is the pull secret to use for images.
	// This acts as the default if not overridden.
	// +optional
	PullSecret *string `json:"pullSecret,omitempty"`

	// Platforms specifies a list of target platforms (e.g., "linux/amd64", "linux/arm64").
	// If specified, the image updater will consider these platforms when checking for new versions or digests.
	// +listType=atomic
	// +optional
	Platforms []string `json:"platforms,omitempty"`
}

// WriteBackConfig defines how and where to write back image updates.
// It includes the method (e.g., git, direct Application update) and
// specific configurations for that method, like Git settings.
type WriteBackConfig struct {
	// Method defines the method for writing back updated image versions.
	// This acts as the default if not overridden. If not specified, defaults to "argocd".
	// +kubebuilder:validation:Required
	// +kubebuilder:default:="argocd"
	// +kubebuilder:validation:Pattern=`^(argocd|git|git:[a-zA-Z0-9][a-zA-Z0-9-._/:]*)$`
	Method *string `json:"method,omitempty"`

	// GitConfig provides Git configuration settings if the write-back method involves Git.
	// This can only be used when method is "git" or starts with "git:".
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
	// If neither spec nor name/tag are set, defaults to "image.name".
	// If spec is set, this field is ignored.
	// +optional
	Name *string `json:"name,omitempty"`

	// Tag is the dot-separated path to the Helm key for the image tag part.
	// Example: "image.tag", "frontend.deployment.image.version".
	// If neither spec nor name/tag are set, defaults to "image.tag".
	// If spec is set, this field is ignored.
	// +optional
	Tag *string `json:"tag,omitempty"`

	// Spec is the dot-separated path to a Helm key where the full image string
	// (e.g., "image/name:1.0") should be written.
	// Use this if your Helm chart expects the entire image reference in a single field,
	// rather than separate name/tag fields. If this is set, name and tag will be ignored.
	// +optional
	Spec *string `json:"spec,omitempty"`
}

// KustomizeTarget defines parameters for updating image references within Kustomize configurations.
type KustomizeTarget struct {
	// Name is the image name (which can include the registry and an initial tag)
	// as it appears in the `images` list of a kustomization.yaml file that needs to be updated.
	// The updater will typically change the tag or add a digest to this entry.
	// Example: "docker.io/library/nginx".
	// This field is required if the Kustomize target is used.
	Name *string `json:"name"`
}

//------------------------Status---------------------------------------------//

// ImageUpdaterStatus defines the observed state of ImageUpdater
type ImageUpdaterStatus struct {
	// Important: Run "make" to regenerate code after modifying this file

	// ObservedGeneration is the most recent generation observed by the controller.
	// +optional
	// +kubebuilder:validation:Minimum=0
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// LastCheckedAt indicates when the controller last checked for image updates.
	// +optional
	LastCheckedAt *metav1.Time `json:"lastCheckedAt,omitempty"`

	// LastUpdatedAt indicates when the controller last performed an image update.
	// +optional
	LastUpdatedAt *metav1.Time `json:"lastUpdatedAt,omitempty"`

	// ApplicationsMatched is the number of Argo CD applications matched by this CR's selectors.
	// +optional
	// +kubebuilder:validation:Minimum=0
	ApplicationsMatched int32 `json:"applicationsMatched"`

	// ImagesManaged is the number of images that were eligible for update checking.
	// +optional
	// +kubebuilder:validation:Minimum=0
	ImagesManaged int32 `json:"imagesManaged"`

	// RecentUpdates contains the list of image updates performed during the last reconciliation cycle.
	// +optional
	// +listType=atomic
	RecentUpdates []RecentUpdate `json:"recentUpdates,omitempty"`

	// Conditions represent the latest available observations of the resource's state.
	// +optional
	// +listType=map
	// +listMapKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// RecentUpdate records a single image update performed during the last reconciliation.
type RecentUpdate struct {
	// Alias is the alias of the image configuration that was updated.
	Alias string `json:"alias"`

	// Image is the full image reference.
	Image string `json:"image"`

	// NewVersion is the new tag or digest the image was updated to.
	NewVersion string `json:"newVersion"`

	// ApplicationsUpdated is the number of applications in which this image was updated.
	// +kubebuilder:validation:Minimum=0
	ApplicationsUpdated int32 `json:"applicationsUpdated"`

	// UpdatedAt is the timestamp when the update was applied.
	UpdatedAt metav1.Time `json:"updatedAt"`

	// Message provides a human-readable description of the update action.
	// +optional
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Apps",type=integer,JSONPath=`.status.applicationsMatched`
// +kubebuilder:printcolumn:name="Images",type=integer,JSONPath=`.status.imagesManaged`
// +kubebuilder:printcolumn:name="Last Checked",type=date,JSONPath=`.status.lastCheckedAt`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`

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
