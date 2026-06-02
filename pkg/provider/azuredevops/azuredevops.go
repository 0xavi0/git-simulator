// Package azuredevops implements core.Provider for Azure DevOps git.push webhooks.
// Its init() registers the provider via provider.Get("azuredevops").
package azuredevops

import (
	"net/http"

	"github.com/rancher/gitsim/pkg/core"
	"github.com/rancher/gitsim/pkg/provider"
)

func init() {
	provider.Register(Provider{})
}

// Provider implements core.Provider for Azure DevOps.
type Provider struct{}

// Name returns "azuredevops".
func (Provider) Name() string { return "azuredevops" }

// RepoURL returns https://<host>/<owner>/_git/<repo>.
// This value goes into Resource.Repository.RemoteURL and must match
// GitRepo.Spec.Repo in Fleet.
func (Provider) RepoURL(host, owner, repo string) string {
	return "https://" + host + "/" + owner + "/_git/" + repo
}

// BuildWebhook returns headers and JSON body for an Azure DevOps git.push event.
// The secret parameter is used as the HTTP Basic auth password (empty username).
// Pass the same value in the parser's Options.BasicAuth("", secret).
func (Provider) BuildWebhook(event core.PushEvent, secret string) (http.Header, []byte, error) {
	return buildWebhook(event, secret)
}

// APIRoutes is a no-op: Azure DevOps has no REST "latest commit" endpoint
// used by Fleet's vendor-polling path.
func (Provider) APIRoutes(_ *http.ServeMux, _ core.CommitResolver) {}
