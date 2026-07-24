package argocd

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"text/template"

	argocdapi "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"github.com/argoproj/argo-cd/v3/util/db"

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
	ArgocdDB               db.ArgoDB
	UpdateApp              *ApplicationImages
	DryRun                 bool
	GitCommitUser          string
	GitCommitEmail         string
	GitCommitMessage       *template.Template
	GitCommitSigningKey    string
	GitCommitSigningMethod string
	GitCommitSignOff       bool
	GitCommitMethod        string
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

// WriteBackMethodArgoCD is the string name of the ArgoCD write-back method as used in the CR spec.
// It is the default when no method is specified.
const WriteBackMethodArgoCD = "argocd"

// Supported values for the git commit method (--git-commit-method).
const (
	// GitCommitMethodGit commits and pushes using the local git command line (default).
	GitCommitMethodGit = "git"
	// GitCommitMethodAPI creates commits through the GitHub API. Commits made
	// with GitHub App credentials are then signed by GitHub ("Verified").
	GitCommitMethodAPI = "api"
)

// IsValidGitCommitMethod validates a --git-commit-method flag value. The
// empty string is valid and treated as GitCommitMethodGit.
func IsValidGitCommitMethod(m string) bool {
	return m == "" || m == GitCommitMethodGit || m == GitCommitMethodAPI
}

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
	ArgocdDB   db.ArgoDB
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
	GitCommitMethod        string
	KustomizeBase          string
	Target                 string
	GitRepo                string
	GitCreds               git.CredsStore
	PRProvider             PRProvider
	PullRequest            *PullRequest
}

// WriteBackTargetKey returns a short hash that uniquely identifies the
// write-back target for PR deduplication. Two applications sharing the same
// git repo, base branch, and target path produce the same key.
func (wbc *WriteBackConfig) WriteBackTargetKey() string {
	target := wbc.Target
	if target == "" {
		target = wbc.KustomizeBase
	}
	h := sha256.Sum256([]byte(wbc.GitRepo + "|" + wbc.GitBranch + "|" + target))
	return hex.EncodeToString(h[:])[:8]
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
	prCreated       map[string]bool
}

// NewSyncIterationState returns a new instance of SyncIterationState
func NewSyncIterationState() *SyncIterationState {
	return &SyncIterationState{
		repositoryLocks: make(map[string]*sync.Mutex),
		prCreated:       make(map[string]bool),
	}
}

// MarkPRCreated records that a PR has been created for the given write-back
// target key. Returns true on the first call for a key (caller should proceed
// with PR creation) and false on subsequent calls (caller should skip).
func (state *SyncIterationState) MarkPRCreated(targetKey string) bool {
	state.lock.Lock()
	defer state.lock.Unlock()
	if state.prCreated[targetKey] {
		return false
	}
	state.prCreated[targetKey] = true
	return true
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

	// verify image signature settings
	EnableVerification bool
	*image.Verify
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
