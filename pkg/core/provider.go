package core

import "net/http"

// Provider is implemented once per git hosting vendor (GitHub, GitLab, …).
// It is pure formatting/signing — no I/O, no knowledge of the state store.
// Given a push event it produces the HTTP request Fleet should receive, and it
// declares the URL shapes + REST routes it owns.
type Provider interface {
	// Name returns the vendor identifier, e.g. "github".
	Name() string

	// RepoURL is what goes into webhook payloads AND must match GitRepo.Spec.Repo.
	// host/owner/repo come from the control plane when a repo is registered.
	RepoURL(host, owner, repo string) string

	// BuildWebhook returns headers+body for a push of event. secret may be empty.
	BuildWebhook(event PushEvent, secret string) (header http.Header, body []byte, err error)

	// APIRoutes registers any vendor REST endpoints (e.g. GitHub commits API) on mux.
	// Implementations read HEAD via the injected CommitResolver.
	APIRoutes(mux *http.ServeMux, resolve CommitResolver)
}

// PushEvent carries the information the simulator has about a branch push.
type PushEvent struct {
	Host, Owner, Repo string
	Branch, Tag       string // exactly one set
	Before, After     string // commit SHAs
}

// CommitResolver lets a Provider's REST routes read currently-visible refs without
// depending on the state store package (avoids import cycles).
type CommitResolver interface {
	VisibleCommit(host, owner, repo, branch string) (sha string, ok bool)
}
