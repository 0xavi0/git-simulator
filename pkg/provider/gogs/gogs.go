// Package gogs implements core.Provider for Gogs push webhooks.
// Its init() registers the provider via provider.Get("gogs").
//
// Gogs sends GitHub-compatible headers as well as X-Gogs-Event, so Fleet
// checks X-Gogs-Event before X-GitHub-Event to route these correctly.
// This provider always sets X-Gogs-Event and omits X-GitHub-Event.
package gogs

import (
	"net/http"

	"github.com/rancher/gitsim/pkg/core"
	"github.com/rancher/gitsim/pkg/provider"
)

func init() {
	provider.Register(Provider{})
}

// Provider implements core.Provider for Gogs.
type Provider struct{}

// Name returns "gogs".
func (Provider) Name() string { return "gogs" }

// RepoURL returns https://<host>/<owner>/<repo>.
// This value goes into the payload's Repository.HTMLURL field and must match
// GitRepo.Spec.Repo in Fleet.
func (Provider) RepoURL(host, owner, repo string) string {
	return "https://" + host + "/" + owner + "/" + repo
}

// BuildWebhook returns headers and JSON body for a Gogs push event.
// When secret is non-empty, X-Gogs-Signature contains the HMAC-SHA256 hex
// digest of the body keyed with secret (no prefix — the Gogs parser checks
// the raw hex string).
func (Provider) BuildWebhook(event core.PushEvent, secret string) (http.Header, []byte, error) {
	return buildWebhook(event, secret)
}

// APIRoutes is a no-op: Gogs has no REST "latest commit" endpoint that Fleet polls.
func (Provider) APIRoutes(_ *http.ServeMux, _ core.CommitResolver) {}
