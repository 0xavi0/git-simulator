// Package gitlab implements core.Provider for GitLab push webhooks.
// Its init() registers the provider so callers can obtain it via provider.Get("gitlab").
package gitlab

import (
	"net/http"

	"github.com/rancher/gitsim/pkg/core"
	"github.com/rancher/gitsim/pkg/provider"
)

func init() {
	provider.Register(Provider{})
}

// Provider implements core.Provider for GitLab.
type Provider struct{}

// Name returns "gitlab".
func (Provider) Name() string { return "gitlab" }

// RepoURL returns https://<host>/<owner>/<repo>.
// This is placed in the webhook payload as Project.WebURL and must match
// GitRepo.Spec.Repo in Fleet.
func (Provider) RepoURL(host, owner, repo string) string {
	return "https://" + host + "/" + owner + "/" + repo
}

// BuildWebhook returns headers and JSON body for a GitLab push (or tag) event.
// When secret is non-empty it is placed verbatim in X-Gitlab-Token; the
// go-playground parser compares sha512 hashes of the header value and the
// configured secret, so we just forward it as plain text.
func (Provider) BuildWebhook(event core.PushEvent, secret string) (http.Header, []byte, error) {
	return buildWebhook(event, secret)
}

// APIRoutes is a no-op: GitLab does not expose a REST "latest commit" endpoint
// that Fleet polls (Fleet polls via git smart-HTTP ls-remote for GitLab).
func (Provider) APIRoutes(_ *http.ServeMux, _ core.CommitResolver) {}
