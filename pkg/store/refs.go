package store

import (
	"time"

	"github.com/rancher/gitsim/pkg/core"
)

// branchState tracks the visible ref and any pending (deferred) commit for a branch.
type branchState struct {
	visibleSHA  string
	pendingSHA  string    // non-empty while a promotion is pending
	promoteAt   time.Time // when pendingSHA becomes visible; only meaningful if pendingSHA != ""
	commitIndex int       // total commits added; passed to ContentProvider.Index
}

// resolve lazily promotes a pending SHA if the clock has reached promoteAt.
// Returns (visibleSHA, promoted). Must be called with the repo lock held.
func (b *branchState) resolve(clock core.Clock) (string, bool) {
	if b.pendingSHA != "" && !clock.Now().Before(b.promoteAt) {
		b.visibleSHA = b.pendingSHA
		b.pendingSHA = ""
		b.promoteAt = time.Time{}
		return b.visibleSHA, true
	}
	return b.visibleSHA, false
}

// tagState describes a tag created on a repo.
type tagState struct {
	commitSHA string // the commit this tag points to
	annotated bool
	tagObjSHA string // SHA of the tag object (annotated only)
}
