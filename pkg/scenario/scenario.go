// Package scenario provides reusable test scenarios for the git-simulator SDK.
// Each scenario is a declarative spec struct paired with a Run* function that
// orchestrates the Stage-5 SDK primitives (PushCommit + FireFunc) to reproduce
// a specific Fleet behaviour under test.
//
// Scenarios never import pkg/gitsim or any lower-level package; they work through
// the Repo interface and the FireFunc callback so callers can supply either the
// real Simulator (in-process tests) or a custom adapter (black-box use).
package scenario

import "time"

// Repo is the view of a simulated repository that scenarios need.
type Repo interface {
	ID() string
	CloneURL() string
	DefaultBranch() string
	VisibleCommit(branch string) (string, bool)
	// PushCommit adds a commit on branch and returns its SHA.
	// The new SHA's visibility is deferred by delay (0 = immediate).
	PushCommit(branch string, delay time.Duration) (string, error)
}

// WebhookResult is the outcome of a single webhook delivery.
type WebhookResult struct {
	StatusCode int    `json:"statusCode"`
	Body       []byte `json:"body,omitempty"`
	Err        error  `json:"error,omitempty"`
}

// FireFunc posts one push webhook for sha to target.
// sendAfter is a relative delay hint; the implementation may honour it
// via the injected clock or real time.
type FireFunc func(sha, target, secret string, sendAfter time.Duration) (WebhookResult, error)
