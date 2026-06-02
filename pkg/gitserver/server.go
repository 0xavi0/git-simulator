// Package gitserver serves git smart-HTTP (ls-remote + clone) for simulated repos.
//
// URL convention: /{owner}/{repo}.git  (host is supplied at handler creation time)
//
//	GET  /{owner}/{repo}.git/info/refs?service=git-upload-pack  → ref advertisement
//	POST /{owner}/{repo}.git/git-upload-pack                    → packfile
package gitserver

import (
	"net/http"
	"strings"
)

// Handler is an http.Handler that serves git smart-HTTP for all repos in a RepoStore.
// The host must match the host string used when repos were registered in the store.
type Handler struct {
	host  string
	store RepoStore
}

// NewHandler creates a Handler for the given host and repo store.
// host should be "hostname" or "hostname:port" matching CreateRepo's host argument.
func NewHandler(host string, rs RepoStore) *Handler {
	return &Handler{host: host, store: rs}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/info/refs"):
		h.handleInfoRefs(w, r)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/git-upload-pack"):
		h.handleUploadPack(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (h *Handler) handleInfoRefs(w http.ResponseWriter, r *http.Request) {
	repoPath := strings.TrimSuffix(r.URL.Path, "/info/refs")
	repo, err := resolveRepo(h.store, h.host, repoPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	infoRefs(w, r, repo.Storer())
}

func (h *Handler) handleUploadPack(w http.ResponseWriter, r *http.Request) {
	repoPath := strings.TrimSuffix(r.URL.Path, "/git-upload-pack")
	repo, err := resolveRepo(h.store, h.host, repoPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	uploadPack(w, r, repo.Storer())
}
