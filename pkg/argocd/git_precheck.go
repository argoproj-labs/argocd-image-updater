package argocd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5/plumbing"

	"github.com/argoproj-labs/argocd-image-updater/ext/git"
	"github.com/argoproj-labs/argocd-image-updater/registry-scanner/pkg/log"

	"github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
)

// GitWriteBackCache remembers, per repository and target file, the branch a
// git write-back last built on, the commit it saw at the head of that branch
// and the content of the target file in that commit. It lives as long as the
// process and holds at most one entry per target file.
//
// An entry is a fact about a commit (the target file has this content in this
// commit), so it never becomes wrong. Whether the commit is still the head of
// the branch is decided against the remote refs listed in the current update
// iteration.
type GitWriteBackCache struct {
	lock    sync.Mutex
	entries map[gitWriteBackKey]gitWriteBackEntry
}

type gitWriteBackKey struct {
	repo   string
	target string
}

type gitWriteBackEntry struct {
	ref     string
	head    string
	content []byte
}

// NewGitWriteBackCache returns an empty write-back cache. Passing it to
// SyncIterationState.WithGitWriteBackCache enables the write-back pre-check.
func NewGitWriteBackCache() *GitWriteBackCache {
	return &GitWriteBackCache{entries: make(map[gitWriteBackKey]gitWriteBackEntry)}
}

func (c *GitWriteBackCache) get(key gitWriteBackKey) (gitWriteBackEntry, bool) {
	c.lock.Lock()
	defer c.lock.Unlock()
	e, ok := c.entries[key]
	return e, ok
}

func (c *GitWriteBackCache) set(key gitWriteBackKey, e gitWriteBackEntry) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.entries[key] = e
}

// remoteRefs holds the refs of each remote repository, listed at most once per
// update iteration, so that every application writing back to the same
// repository can look up its branch head without another ls-remote.
type remoteRefs struct {
	lock  sync.Mutex
	repos map[string]*repoRefs
	// list returns all refs of a remote repository. Replaced in tests.
	list func(ctx context.Context, repo string, creds git.Creds) ([]*plumbing.Reference, error)
	// timeout bounds a listing.
	timeout time.Duration
}

type repoRefs struct {
	lock   sync.Mutex
	loaded bool
	// err is set when the listing failed. It is not retried in this
	// iteration, so that a slow or broken remote delays one write-back only.
	err  error
	refs map[plumbing.ReferenceName]*plumbing.Reference
}

// remoteRefsTimeout is the time allowed for listing the refs of a repository.
const remoteRefsTimeout = 30 * time.Second

func newRemoteRefs() *remoteRefs {
	return &remoteRefs{repos: make(map[string]*repoRefs), list: listRemoteRefs, timeout: remoteRefsTimeout}
}

func listRemoteRefs(ctx context.Context, repo string, creds git.Creds) ([]*plumbing.Reference, error) {
	return git.ListRemoteRefs(ctx, repo, creds, false, "")
}

type listResult struct {
	refs []*plumbing.Reference
	err  error
}

// listWithTimeout lists the refs of repo, giving up after r.timeout even if
// the transport does not honour the context. A listing that times out keeps
// running in the background and its result is dropped.
func (r *remoteRefs) listWithTimeout(ctx context.Context, repo string, creds git.Creds) ([]*plumbing.Reference, error) {
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	ch := make(chan listResult, 1)
	go func() {
		refs, err := r.list(ctx, repo, creds)
		ch <- listResult{refs: refs, err: err}
	}()
	select {
	case res := <-ch:
		return res.refs, res.err
	case <-ctx.Done():
		return nil, fmt.Errorf("listing refs of %s: %w", repo, ctx.Err())
	}
}

func (r *remoteRefs) forRepo(repo string) *repoRefs {
	r.lock.Lock()
	defer r.lock.Unlock()
	rr, ok := r.repos[repo]
	if !ok {
		rr = &repoRefs{}
		r.repos[repo] = rr
	}
	return rr
}

// head returns the commit at the head of ref in the remote repository. ref is a
// branch name or HEAD. The refs are listed with the credentials returned by
// getCreds on the first call for a repository in this iteration.
func (r *remoteRefs) head(ctx context.Context, repo string, ref string, getCreds func() (git.Creds, error)) (string, error) {
	rr := r.forRepo(repo)
	rr.lock.Lock()
	defer rr.lock.Unlock()
	if rr.err != nil {
		return "", rr.err
	}
	if !rr.loaded {
		creds, err := getCreds()
		if err != nil {
			return "", err
		}
		start := time.Now()
		refs, err := r.listWithTimeout(ctx, repo, creds)
		if err != nil {
			rr.err = err
			log.LoggerFromContext(ctx).Warnf("Could not list remote refs of %s for git write-back pre-check, cloning for this cycle: %v", repo, err)
			return "", err
		}
		rr.refs = make(map[plumbing.ReferenceName]*plumbing.Reference, len(refs))
		for _, ref := range refs {
			rr.refs[ref.Name()] = ref
		}
		rr.loaded = true
		log.LoggerFromContext(ctx).Infof("Listed %d remote refs of %s for git write-back pre-check in %s", len(refs), repo, time.Since(start))
	}

	name := plumbing.HEAD
	if ref != "HEAD" {
		name = plumbing.NewBranchReferenceName(ref)
	}
	// Follow a symbolic HEAD to the branch it points to.
	for i := 0; i < 2; i++ {
		target, ok := rr.refs[name]
		if !ok {
			return "", fmt.Errorf("ref %s not found in remote %s", name, repo)
		}
		if target.Type() == plumbing.HashReference {
			return target.Hash().String(), nil
		}
		name = target.Target()
	}
	return "", fmt.Errorf("could not resolve ref %s in remote %s", ref, repo)
}

// setHead records a push to branch made in this iteration. It does nothing when
// the refs of the repository have not been listed yet, because the next listing
// will see the push.
func (r *remoteRefs) setHead(repo string, branch string, sha string) {
	rr := r.forRepo(repo)
	rr.lock.Lock()
	defer rr.lock.Unlock()
	if !rr.loaded {
		return
	}
	name := plumbing.NewBranchReferenceName(branch)
	rr.refs[name] = plumbing.NewHashReference(name, plumbing.NewHash(sha))
}

// gitWriteBackPrecheck decides, before taking the repository lock and cloning,
// whether a git write-back would leave the target file unchanged. It only
// covers the parameter override and Helm values files (writeOverrides) in
// direct push mode.
type gitWriteBackPrecheck struct {
	cache *GitWriteBackCache
	refs  *remoteRefs
}

// writeBackRef returns the ref whose head the write-back builds on, as named in
// the configuration: the push branch when a write branch is configured, else
// the checkout branch, with an empty target revision meaning HEAD.
func writeBackRef(ctx context.Context, app *v1alpha1.Application, wbc *WriteBackConfig, pushBranch string) string {
	if wbc.GitWriteBranch != "" {
		return pushBranch
	}
	ref := wbc.GitBranch
	if ref == "" {
		ref = getWriteBackBranch(ctx, app, wbc)
	}
	if ref == "" {
		ref = "HEAD"
	}
	return ref
}

// unchanged returns true when the remote head of the write-back ref is the
// commit cached for this target and applying the application's parameters to
// the cached file content gives the same content.
func (p *gitWriteBackPrecheck) unchanged(ctx context.Context, applicationImages *ApplicationImages, changeList []ChangeEntry) bool {
	logCtx := log.LoggerFromContext(ctx)
	app := applicationImages.Application
	wbc := applicationImages.WriteBackConfig

	pushBranch := wbc.GitWriteBranch
	if strings.Contains(pushBranch, "{{") || len(pushBranch) > 255 {
		pushBranch = TemplateBranchName(ctx, wbc.GitWriteBranch, "", "", "", changeList)
		if pushBranch == "" {
			return false
		}
	}
	ref := writeBackRef(ctx, &app, wbc, pushBranch)
	key := gitWriteBackKey{repo: wbc.GitRepo, target: wbc.Target}

	entry, ok := p.cache.get(key)
	if !ok || entry.ref != ref {
		logCtx.Debugf("git write-back pre-check: no cached state for %s on %s", key.target, ref)
		return false
	}
	head, err := p.refs.head(ctx, key.repo, ref, func() (git.Creds, error) { return wbc.GetCreds(&app) })
	if err != nil {
		logCtx.Debugf("git write-back pre-check: could not get remote head of %s: %v", ref, err)
		return false
	}
	if head != entry.head {
		logCtx.Debugf("git write-back pre-check: %s moved from %s to %s", ref, entry.head, head)
		return false
	}
	override, err := marshalParamsOverride(ctx, applicationImages, entry.content)
	if err != nil {
		logCtx.Debugf("git write-back pre-check: could not marshal parameter overrides: %v", err)
		return false
	}
	if !bytes.Equal(override, entry.content) {
		logCtx.Debugf("git write-back pre-check: %s would change", key.target)
		return false
	}
	logCtx.Infof("Skipping git write-back: %s on %s is up to date at %s", key.target, ref, head)
	return true
}

// remember stores the head and target file content of the checked out
// write-back branch after the write-back finished. pushed is true when the
// write-back pushed a new commit to pushBranch from the local clone.
func (p *gitWriteBackPrecheck) remember(ctx context.Context, applicationImages *ApplicationImages, gitC git.Client, pushBranch string, pushed bool) {
	if p == nil {
		return
	}
	logCtx := log.LoggerFromContext(ctx)
	app := applicationImages.Application
	wbc := applicationImages.WriteBackConfig
	head, err := gitC.CommitSHA(ctx)
	if err != nil {
		logCtx.Debugf("git write-back pre-check: could not get head commit: %v", err)
		return
	}
	content, err := os.ReadFile(path.Join(gitC.Root(), wbc.Target))
	if err != nil {
		logCtx.Debugf("git write-back pre-check: could not read %s: %v", wbc.Target, err)
		return
	}
	key := gitWriteBackKey{repo: wbc.GitRepo, target: wbc.Target}
	p.cache.set(key, gitWriteBackEntry{ref: writeBackRef(ctx, &app, wbc, pushBranch), head: head, content: content})
	if pushed {
		p.refs.setHead(wbc.GitRepo, pushBranch, head)
	}
}
