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

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// ImageUpdaterSpec defines the desired state of ImageUpdater
type ImageUpdaterSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// Foo is an example field of ImageUpdater. Edit imageupdater_types.go to remove/update
	// Foo string `json:"foo,omitempty"`

	// ApplicationRef indicates the set of applications to be managed
	ApplicationRefs []ApplicationRef `json:"applicationRefs,omitempty"`

	// Images contains a list of configurations that how images should be updated
	Images []ImageUpdateConfig `json:"images,omitempty"`
}

// ImageUpdaterStatus defines the observed state of ImageUpdater
type ImageUpdaterStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// LastUpdatedAt indicates when the image updater last ran
	LastUpdatedAt *metav1.Time `json:"reconciledAt,omitempty"`

	// ImageStatus indicates the detailed status for the list of managed images
	ImageStatus []ImageStatus `json:"imageStatus,omitempty"`
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

// ApplicationRef contains various criteria by which to include applications for managing by image updater
type ApplicationRef struct {
	// DestinationPattern indicates the glob pattern for destination cluster
	DestinationPattern string `json:"destinationPattern,omitempty"`

	// Namespace indicates the target namespace of the application
	Namespace *string `json:"namespace,omitempty"`

	// NamePattern indicates the glob pattern for application name
	NamePattern *string `json:"namePattern,omitempty"`

	// LabelSelectors indicates the label selectors to apply for application selection
	LabelSelectors *metav1.LabelSelector `json:"labelSelectors,omitempty"`
}

// ImageUpdateConfig specifies how a particular image should be updated
type ImageUpdateConfig struct {
	// Name indicates the image name
	Name string `json:"name"`

	// Version indicates the version constraint for the update
	Version string `json:"version"`

	// Path indicates the path to the image in the workload spec
	Path string `json:"path"`

	// Strategy indicates the update strategy for this image
	Strategy string `json:"strategy"`
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

func init() {
	SchemeBuilder.Register(&ImageUpdater{}, &ImageUpdaterList{})
}
