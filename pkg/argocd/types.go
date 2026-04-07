package argocd

import (
	"sync"
	"text/template"

	argocdapi "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"

	"github.com/argoproj-labs/argocd-image-updater/ext/git"
	"github.com/argoproj-labs/argocd-image-updater/pkg/kube"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/image"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/registry"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/tag"
)

// ImageUpdaterResult Stores some statistics about the results of a run
type ImageUpdaterResult struct {
	NumApplicationsProcessed int
	NumImagesFound           int
	NumImagesUpdated         int
	NumImagesConsidered      int
	NumSkipped               int
	NumErrors                int
	ApplicationsMatched      int
	Changes                  []ChangeEntry
}

type UpdateConfiguration struct {
	NewRegFN               registry.NewRegistryClient
	ArgoClient             ArgoCD
	KubeClient             *kube.ImageUpdaterKubernetesClient
	UpdateApp              *ApplicationImages
	DryRun                 bool
	GitCommitUser          string
	GitCommitEmail         string
	GitCommitMessage       *template.Template
	GitCommitSigningKey    string
	GitCommitSigningMethod string
	GitCommitSignOff       bool
	DisableKubeEvents      bool
	IgnorePlatforms        bool
	GitCreds               git.CredsStore
}

type GitCredsSource func(app *argocdapi.Application) (git.Creds, error)

type WriteBackMethod int

const (
	WriteBackApplication WriteBackMethod = 0
	WriteBackGit         WriteBackMethod = 1
)

const defaultIndent = 2

// ApplicationType Type of the application
type ApplicationType int

const (
	ApplicationTypeUnsupported ApplicationType = 0
	ApplicationTypeHelm        ApplicationType = 1
	ApplicationTypeKustomize   ApplicationType = 2
)

// WriteBackConfig holds information on how to write back the changes to an Application
type WriteBackConfig struct {
	Method     WriteBackMethod
	ArgoClient ArgoCD
	// If GitClient is not nil, the client will be used for updates. Otherwise, a new client will be created.
	GitClient              git.Client
	GetCreds               GitCredsSource
	GitBranch              string
	GitWriteBranch         string
	GitCommitUser          string
	GitCommitEmail         string
	GitCommitMessage       string
	GitCommitSigningKey    string
	GitCommitSigningMethod string
	GitCommitSignOff       bool
	KustomizeBase          string
	Target                 string
	GitRepo                string
	GitCreds               git.CredsStore
	PRProvider             PRProvider
	PullRequest            *PullRequest
	// GitCredsID identifies the credential source (e.g. "repocreds" or "secret:ns/name").
	// Used by BatchKey() to ensure only apps sharing the same credential source are batched.
	GitCredsID string
}

// RequiresLocking returns true if write-back method requires repository locking
func (wbc *WriteBackConfig) RequiresLocking() bool {
	switch wbc.Method {
	case WriteBackGit:
		return true
	default:
		return false
	}
}

// The following are helper structs to only marshal the fields we require
type kustomizeImages struct {
	Images *argocdapi.KustomizeImages `json:"images"`
}

type kustomizeOverride struct {
	Kustomize kustomizeImages `json:"kustomize"`
}

type helmParameters struct {
	Parameters []argocdapi.HelmParameter `json:"parameters"`
}

type helmOverride struct {
	Helm helmParameters `json:"helm"`
}

// ChangeEntry represents an image that has been changed by Image Updater
type ChangeEntry struct {
	Image  *image.ContainerImage
	OldTag *tag.ImageTag
	NewTag *tag.ImageTag
}

// SyncIterationState holds shared state of a running update operation
type SyncIterationState struct {
	lock            sync.Mutex
	repositoryLocks map[string]*sync.Mutex
}

// NewSyncIterationState returns a new instance of SyncIterationState
func NewSyncIterationState() *SyncIterationState {
	return &SyncIterationState{
		repositoryLocks: make(map[string]*sync.Mutex),
	}
}

// GetRepositoryLock returns the lock for a specified repository
func (state *SyncIterationState) GetRepositoryLock(repository string) *sync.Mutex {
	state.lock.Lock()
	defer state.lock.Unlock()

	lock, exists := state.repositoryLocks[repository]
	if !exists {
		lock = &sync.Mutex{}
		state.repositoryLocks[repository] = lock
	}

	return lock
}

// ApplicationImages holds an Argo CD application, its write-back config, and a list of its images
// that are allowed to be considered for updates.
type ApplicationImages struct {
	argocdapi.Application
	*WriteBackConfig
	Images ImageList
}

// Image represents a container image and its update configuration.
// It embeds the neutral ContainerImage type and adds updater-specific
// configuration. Use this struct to populate elements from ImageUpdater CR.
type Image struct {
	*image.ContainerImage

	// Update settings
	UpdateStrategy image.UpdateStrategy
	ForceUpdate    bool
	AllowTags      string
	IgnoreTags     []string
	PullSecret     string
	Platforms      []string

	// ManifestTarget settings
	HelmImageName      string
	HelmImageTag       string
	HelmImageSpec      string
	KustomizeImageName string
}

// ImageList is a list of Image objects that can be updated.
type ImageList []*Image

// NewImage creates a new Image object from a neutral ContainerImage
func NewImage(ci *image.ContainerImage) *Image {
	return &Image{
		ContainerImage: ci,
	}
}

// ToContainerImageList is a private helper that converts an ImageList to a
// neutral image.ContainerImageList. This allows us to reuse methods defined
// on ContainerImageList without duplicating code.
func (list ImageList) ToContainerImageList() image.ContainerImageList {
	cil := make(image.ContainerImageList, len(list))
	for i, img := range list {
		cil[i] = img.ContainerImage
	}
	return cil
}

// PendingWrite represents a deferred git write-back operation for a single application.
// It is produced by the registry polling phase and consumed by the batched git write phase.
type PendingWrite struct {
	// AppName is the namespaced name of the application (e.g. "namespace/appname")
	AppName string
	// App holds the application and its write-back configuration
	App *ApplicationImages
	// ChangeList records which images were changed
	ChangeList []ChangeEntry
	// Result holds the polling-phase result for this application
	Result ImageUpdaterResult
	// UpdateConf holds the full update config needed for kube events etc.
	UpdateConf *UpdateConfiguration
	// ResolvedBranch is the effective target branch resolved at polling time
	// from the app's targetRevision or explicit annotation.
	ResolvedBranch string
}

// BatchKey returns the grouping key for batching git operations.
// Operations targeting the same repository, branch, and credential source
// can share a single clone/fetch/checkout cycle. Including the credential
// source ensures apps with different git credentials are never batched.
func (pw *PendingWrite) BatchKey() string {
	wbc := pw.App.WriteBackConfig
	branch := pw.ResolvedBranch
	if branch == "" {
		branch = "_default_"
	}
	credsKey := wbc.GitCredsID
	if credsKey == "" {
		credsKey = "_unknown_"
	}
	return wbc.GitRepo + "::" + branch + "::" + credsKey
}

// WebhookEvent represents a generic webhook payload
type WebhookEvent struct {
	// RegistryURL is the URL of the registry that sent the webhook
	RegistryURL string `json:"registryUrl,omitempty"`
	// Repository is the repository name
	Repository string `json:"repository,omitempty"`
	// Tag is the image tag
	Tag string `json:"tag,omitempty"`
	// Digest is the content digest of the image
	Digest string `json:"digest,omitempty"`
}
