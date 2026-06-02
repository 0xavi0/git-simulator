package store

import (
	"fmt"
	"sync"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	gogitstorage "github.com/go-git/go-git/v5/storage"
	"github.com/go-git/go-git/v5/storage/memory"

	"github.com/rancher/gitsim/pkg/core"
)

// Repo is a simulated git repository. All exported methods are thread-safe.
type Repo struct {
	host, owner, name string
	defaultBranch     string

	mu       sync.Mutex
	storer   gogitstorage.Storer
	branches map[string]*branchState
	tags     map[string]*tagState

	clock   core.Clock
	content core.ContentProvider
}

// Identity returns (host, owner, name).
func (r *Repo) Identity() (host, owner, name string) {
	return r.host, r.owner, r.name
}

// DefaultBranch returns the repo's default branch name.
func (r *Repo) DefaultBranch() string { return r.defaultBranch }

// Storer returns the underlying go-git storer so Stage 2 can serve git-smart-HTTP.
// The storer reflects promoted refs; callers that need current state should call
// VisibleCommit (or VisibleRefs) first to ensure lazy promotions are flushed.
func (r *Repo) Storer() gogitstorage.Storer { return r.storer }

func newRepo(host, owner, name, defaultBranch string, clock core.Clock, content core.ContentProvider) (*Repo, error) {
	r := &Repo{
		host:          host,
		owner:         owner,
		name:          name,
		defaultBranch: defaultBranch,
		storer:        memory.NewStorage(),
		branches:      make(map[string]*branchState),
		tags:          make(map[string]*tagState),
		clock:         clock,
		content:       content,
	}

	ctx := core.ContentContext{
		Repo:   fmt.Sprintf("%s/%s/%s", host, owner, name),
		Branch: defaultBranch,
		Index:  0,
	}
	files, err := content.Files(ctx)
	if err != nil {
		return nil, fmt.Errorf("content for initial commit: %w", err)
	}

	sha, err := buildCommit(r.storer, "", files, "initial commit", clock.Now())
	if err != nil {
		return nil, fmt.Errorf("build initial commit: %w", err)
	}

	refName := plumbing.ReferenceName("refs/heads/" + defaultBranch)
	if err := r.storer.SetReference(plumbing.NewHashReference(refName, plumbing.NewHash(sha))); err != nil {
		return nil, fmt.Errorf("set initial ref: %w", err)
	}

	headRef := plumbing.NewSymbolicReference(plumbing.HEAD, refName)
	if err := r.storer.SetReference(headRef); err != nil {
		return nil, fmt.Errorf("set HEAD: %w", err)
	}

	r.branches[defaultBranch] = &branchState{
		visibleSHA:  sha,
		commitIndex: 0,
	}
	return r, nil
}

// AddCommit writes a real git commit object on top of branch's current tip and returns its SHA.
// The branch's visible ref is NOT advanced until delay elapses (lazy promotion).
// delay == 0 promotes immediately so the new SHA is visible at once.
func (r *Repo) AddCommit(branch string, delay time.Duration) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	bs, ok := r.branches[branch]
	if !ok {
		return "", fmt.Errorf("%w: branch %q in %s/%s/%s", core.ErrNotFound, branch, r.host, r.owner, r.name)
	}

	// Resolve any due promotion before using the current tip as parent.
	parentSHA, promoted := bs.resolve(r.clock)
	if promoted {
		r.syncRef(branch, parentSHA)
	}

	newIndex := bs.commitIndex + 1
	ctx := core.ContentContext{
		Repo:      fmt.Sprintf("%s/%s/%s", r.host, r.owner, r.name),
		Branch:    branch,
		CommitSHA: parentSHA,
		Index:     newIndex,
	}
	files, err := r.content.Files(ctx)
	if err != nil {
		return "", fmt.Errorf("content for commit %d: %w", newIndex, err)
	}

	msg := fmt.Sprintf("commit %d on branch %s", newIndex, branch)
	sha, err := buildCommit(r.storer, parentSHA, files, msg, r.clock.Now())
	if err != nil {
		return "", fmt.Errorf("build commit: %w", err)
	}

	bs.commitIndex = newIndex

	if delay == 0 {
		bs.visibleSHA = sha
		bs.pendingSHA = ""
		bs.promoteAt = time.Time{}
		r.syncRef(branch, sha)
	} else {
		bs.pendingSHA = sha
		bs.promoteAt = r.clock.Now().Add(delay)
	}
	return sha, nil
}

// VisibleCommit returns the SHA currently visible for branch (lazily promoting if due).
func (r *Repo) VisibleCommit(branch string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	bs, ok := r.branches[branch]
	if !ok {
		return "", false
	}

	sha, promoted := bs.resolve(r.clock)
	if promoted {
		r.syncRef(branch, sha)
	}
	return sha, sha != ""
}

// VisibleRefs returns all refs after applying any due lazy promotions.
// Stage 2 calls this before serving ls-remote to ensure the storer is up to date.
func (r *Repo) VisibleRefs() ([]*plumbing.Reference, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	for branch, bs := range r.branches {
		sha, promoted := bs.resolve(r.clock)
		if promoted {
			r.syncRef(branch, sha)
		}
	}

	iter, err := r.storer.IterReferences()
	if err != nil {
		return nil, err
	}
	defer iter.Close()

	var refs []*plumbing.Reference
	err = iter.ForEach(func(ref *plumbing.Reference) error {
		cp := *ref
		refs = append(refs, &cp)
		return nil
	})
	return refs, err
}

// AddTag creates a tag pointing to the current visible commit on branch.
// If annotated is true, an annotated tag object is written; otherwise lightweight.
func (r *Repo) AddTag(tagName, branch string, annotated bool) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	bs, ok := r.branches[branch]
	if !ok {
		return "", fmt.Errorf("%w: branch %q", core.ErrNotFound, branch)
	}

	commitSHA, promoted := bs.resolve(r.clock)
	if promoted {
		r.syncRef(branch, commitSHA)
	}

	var refTarget plumbing.Hash
	ts := &tagState{commitSHA: commitSHA}

	if annotated {
		sig := object.Signature{
			Name:  "gitsim",
			Email: "gitsim@example.com",
			When:  r.clock.Now(),
		}
		tag := &object.Tag{
			Name:       tagName,
			Tagger:     sig,
			Message:    "tag " + tagName,
			TargetType: plumbing.CommitObject,
			Target:     plumbing.NewHash(commitSHA),
		}
		obj := r.storer.NewEncodedObject()
		if err := tag.Encode(obj); err != nil {
			return "", fmt.Errorf("encode tag object: %w", err)
		}
		hash, err := r.storer.SetEncodedObject(obj)
		if err != nil {
			return "", fmt.Errorf("store tag object: %w", err)
		}
		ts.annotated = true
		ts.tagObjSHA = hash.String()
		refTarget = hash
	} else {
		refTarget = plumbing.NewHash(commitSHA)
	}

	refName := plumbing.ReferenceName("refs/tags/" + tagName)
	if err := r.storer.SetReference(plumbing.NewHashReference(refName, refTarget)); err != nil {
		return "", fmt.Errorf("set tag ref: %w", err)
	}
	r.tags[tagName] = ts
	return refTarget.String(), nil
}

// syncRef updates refs/heads/<branch> in the storer to match sha.
// Must be called with r.mu held.
func (r *Repo) syncRef(branch, sha string) {
	refName := plumbing.ReferenceName("refs/heads/" + branch)
	_ = r.storer.SetReference(plumbing.NewHashReference(refName, plumbing.NewHash(sha)))
}
