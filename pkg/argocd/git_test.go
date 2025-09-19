package argocd

import (
    "testing"
    "sync/atomic"
    "github.com/argoproj-labs/argocd-image-updater/pkg/common"
    v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    v1alpha1 "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
    extgit "github.com/argoproj-labs/argocd-image-updater/ext/git"
)

// Test that grouping does not mix different branches in one commit/push
func Test_groupIntentsByBranch(t *testing.T) {
    appA := &v1alpha1.Application{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{common.GitBranchAnnotation: "main:appA-branch"}}}
    appB := &v1alpha1.Application{ObjectMeta: v1.ObjectMeta{Annotations: map[string]string{common.GitBranchAnnotation: "main:appB-branch"}}}
    wbcA := &WriteBackConfig{GitRepo: "https://example/repo.git", GitWriteBranch: "appA-branch"}
    wbcB := &WriteBackConfig{GitRepo: "https://example/repo.git", GitWriteBranch: "appB-branch"}

    by := groupIntentsByBranch([]writeIntent{
        {app: appA, wbc: wbcA, changeList: []ChangeEntry{{}}, writeFn: writeOverrides},
        {app: appB, wbc: wbcB, changeList: []ChangeEntry{{}}, writeFn: writeOverrides},
        {app: appA, wbc: wbcA, changeList: []ChangeEntry{{}}, writeFn: writeOverrides},
    })

    if len(by["appA-branch"]) != 2 {
        t.Fatalf("expected 2 intents for appA-branch, got %d", len(by["appA-branch"]))
    }
    if len(by["appB-branch"]) != 1 {
        t.Fatalf("expected 1 intent for appB-branch, got %d", len(by["appB-branch"]))
    }
}

type fakeGitClient struct{ pushes int32 }
func (f *fakeGitClient) Root() string { return "/tmp" }
func (f *fakeGitClient) Init() error { return nil }
func (f *fakeGitClient) Fetch(revision string) error { return nil }
func (f *fakeGitClient) ShallowFetch(revision string, depth int) error { return nil }
func (f *fakeGitClient) Submodule() error { return nil }
func (f *fakeGitClient) Checkout(revision string, submoduleEnabled bool) error { return nil }
func (f *fakeGitClient) LsRefs() (*extgit.Refs, error) { return &extgit.Refs{}, nil }
func (f *fakeGitClient) LsRemote(revision string) (string, error) { return "", nil }
func (f *fakeGitClient) LsFiles(path string, enableNewGitFileGlobbing bool) ([]string, error) { return nil, nil }
func (f *fakeGitClient) LsLargeFiles() ([]string, error) { return nil, nil }
func (f *fakeGitClient) CommitSHA() (string, error) { return "", nil }
func (f *fakeGitClient) RevisionMetadata(revision string) (*extgit.RevisionMetadata, error) { return nil, nil }
func (f *fakeGitClient) VerifyCommitSignature(string) (string, error) { return "", nil }
func (f *fakeGitClient) IsAnnotatedTag(string) bool { return false }
func (f *fakeGitClient) ChangedFiles(revision string, targetRevision string) ([]string, error) { return nil, nil }
func (f *fakeGitClient) Commit(path string, opts *extgit.CommitOptions) error { return nil }
func (f *fakeGitClient) Branch(from, to string) error { return nil }
func (f *fakeGitClient) Push(remote, branch string, force bool) error { atomic.AddInt32(&f.pushes, 1); return nil }
func (f *fakeGitClient) Add(path string) error { return nil }
func (f *fakeGitClient) SymRefToBranch(symRef string) (string, error) { return "main", nil }
func (f *fakeGitClient) Config(username string, email string) error { return nil }

func Test_repoWriter_BatchesPerBranch(t *testing.T) {
    // stub git client factory
    old := newGitClient
    defer func(){ newGitClient = old }()
    fg := &fakeGitClient{}
    newGitClient = func(rawRepoURL string, root string, creds extgit.Creds, insecure bool, enableLfs bool, proxy string, opts ...extgit.ClientOpts) (extgit.Client, error) {
        return fg, nil
    }

    appMain := &v1alpha1.Application{}
    appDev := &v1alpha1.Application{ObjectMeta: appMain.ObjectMeta}
    wbcMain := &WriteBackConfig{GitRepo: "https://example/repo.git", GitWriteBranch: "main", Method: WriteBackGit, GetCreds: func(a *v1alpha1.Application) (extgit.Creds, error) { return extgit.NopCreds{}, nil }}
    wbcDev := &WriteBackConfig{GitRepo: "https://example/repo.git", GitWriteBranch: "dev", Method: WriteBackGit, GetCreds: func(a *v1alpha1.Application) (extgit.Creds, error) { return extgit.NopCreds{}, nil }}

    rw := &repoWriter{repoURL: wbcMain.GitRepo, intentsCh: make(chan writeIntent, 10), flushEvery: 0, maxBatch: 100, stopCh: make(chan struct{})}
    // Directly call flushBatch to avoid goroutine timing
    rw.flushBatch([]writeIntent{
        {app: appMain, wbc: wbcMain, changeList: []ChangeEntry{{}}, writeFn: func(a *v1alpha1.Application, w *WriteBackConfig, c extgit.Client) (error, bool) { return nil, false }},
        {app: appDev, wbc: wbcDev, changeList: []ChangeEntry{{}}, writeFn: func(a *v1alpha1.Application, w *WriteBackConfig, c extgit.Client) (error, bool) { return nil, false }},
    })

    if fg.pushes != 2 {
        t.Fatalf("expected 2 pushes (one per branch), got %d", fg.pushes)
    }
}
