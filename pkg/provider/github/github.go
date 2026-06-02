// Package github implements core.Provider for GitHub.
// Its init() function registers the provider in the global registry so callers
// can obtain it via provider.Get("github").
package github

import (
	"net/http"

	"github.com/rancher/gitsim/pkg/core"
	"github.com/rancher/gitsim/pkg/provider"
)

func init() {
	provider.Register(Provider{})
}

// Provider implements core.Provider for GitHub webhooks and the GitHub commits REST API.
type Provider struct{}

// Name returns "github".
func (Provider) Name() string { return "github" }

// RepoURL returns the HTML URL for a repository in the form
// https://<host>/<owner>/<repo> (no .git suffix).
// This is the value placed in webhook payloads as Repository.HTMLURL and must
// match the GitRepo.Spec.Repo that Fleet uses for correlation.
func (Provider) RepoURL(host, owner, repo string) string {
	return "https://" + host + "/" + owner + "/" + repo
}

// BuildWebhook returns headers and JSON body for a GitHub push event.
// When secret is non-empty the response includes X-Hub-Signature-256.
func (Provider) BuildWebhook(event core.PushEvent, secret string) (http.Header, []byte, error) {
	return buildPushWebhook(event, secret)
}

// APIRoutes registers the GitHub REST endpoint used by Fleet's vendor polling:
//
//	GET /repos/{owner}/{repo}/commits/{branch}
//
// with Accept: application/vnd.github.v3.sha it returns the bare SHA.
// The host key for CommitResolver lookups is taken from the HTTP request's Host
// header, so repos must be registered in the store with the same host string
// the HTTP client will send.
func (Provider) APIRoutes(mux *http.ServeMux, resolve core.CommitResolver) {
	mux.HandleFunc("GET /repos/{owner}/{repo}/commits/{branch}", func(w http.ResponseWriter, r *http.Request) {
		commitsHandler(w, r, resolve)
	})
}
