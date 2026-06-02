// Package bitbucket implements core.Provider for Bitbucket Cloud push webhooks.
// Its init() registers the provider via provider.Get("bitbucket").
package bitbucket

import (
	"net/http"

	"github.com/rancher/gitsim/pkg/core"
	"github.com/rancher/gitsim/pkg/provider"
)

func init() {
	provider.Register(Provider{})
}

// Provider implements core.Provider for Bitbucket Cloud.
type Provider struct{}

// Name returns "bitbucket".
func (Provider) Name() string { return "bitbucket" }

// RepoURL returns https://<host>/<owner>/<repo>.
// This value goes into payload.Repository.Links.HTML.Href and must match
// GitRepo.Spec.Repo when Fleet correlates the webhook.
func (Provider) RepoURL(host, owner, repo string) string {
	return "https://" + host + "/" + owner + "/" + repo
}

// BuildWebhook builds a Bitbucket Cloud repo:push webhook.
// The secret parameter is used as the X-Hook-UUID value; the Bitbucket Cloud
// parser verifies that the request header UUID matches the configured UUID.
// Pass the same value in both BuildWebhook and the parser's Options.UUID.
func (Provider) BuildWebhook(event core.PushEvent, secret string) (http.Header, []byte, error) {
	return buildWebhook(event, secret)
}

// APIRoutes is a no-op: Bitbucket Cloud has no REST "latest commit" endpoint
// that Fleet polls.
func (Provider) APIRoutes(_ *http.ServeMux, _ core.CommitResolver) {}
