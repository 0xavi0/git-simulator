// Package bitbucketserver implements core.Provider for Bitbucket Server (Data Center) push webhooks.
// Its init() registers the provider via provider.Get("bitbucketserver").
//
// Detection note: Fleet checks X-Event-Key after X-Hook-UUID, so payloads from
// this provider must NOT include X-Hook-UUID to avoid being misidentified as
// Bitbucket Cloud.
package bitbucketserver

import (
	"net/http"

	"github.com/rancher/gitsim/pkg/core"
	"github.com/rancher/gitsim/pkg/provider"
)

func init() {
	provider.Register(Provider{})
}

// Provider implements core.Provider for Bitbucket Server.
type Provider struct{}

// Name returns "bitbucketserver".
func (Provider) Name() string { return "bitbucketserver" }

// RepoURL returns the HTTP clone URL for a repo in Bitbucket Server format:
// http://<host>/<owner>/<repo>.git
// This value is placed in the webhook payload's clone link href and must match
// GitRepo.Spec.Repo in Fleet.
func (Provider) RepoURL(host, owner, repo string) string {
	return "http://" + host + "/" + owner + "/" + repo + ".git"
}

// BuildWebhook returns headers and JSON body for a Bitbucket Server repo:refs_changed event.
// When secret is non-empty, X-Hub-Signature contains sha256=<HMAC-SHA256(body, secret)>.
func (Provider) BuildWebhook(event core.PushEvent, secret string) (http.Header, []byte, error) {
	return buildWebhook(event, secret)
}

// APIRoutes is a no-op: Bitbucket Server has no REST "latest commit" endpoint
// that Fleet polls.
func (Provider) APIRoutes(_ *http.ServeMux, _ core.CommitResolver) {}
