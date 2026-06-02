package github

import (
	"fmt"
	"net/http"

	"github.com/rancher/gitsim/pkg/core"
)

const acceptSHA = "application/vnd.github.v3.sha"

// commitsHandler serves GET /repos/{owner}/{repo}/commits/{branch}.
// With Accept: application/vnd.github.v3.sha it responds with the bare SHA.
// This mirrors what Fleet's latestCommitFromCommitsURL calls against github.com.
func commitsHandler(w http.ResponseWriter, r *http.Request, resolve core.CommitResolver) {
	owner := r.PathValue("owner")
	repo := r.PathValue("repo")
	branch := r.PathValue("branch")

	// The host key matches how repos were registered in the store.
	// Callers must ensure the request Host header equals the store host.
	host := r.Host

	sha, ok := resolve.VisibleCommit(host, owner, repo, branch)
	if !ok {
		http.Error(w, fmt.Sprintf("not found: %s/%s@%s", owner, repo, branch), http.StatusNotFound)
		return
	}

	if r.Header.Get("Accept") == acceptSHA {
		w.Header().Set("Content-Type", "application/vnd.github.v3.sha")
		fmt.Fprint(w, sha)
		return
	}

	// Fall back to minimal JSON for clients that don't send the SHA accept header.
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"sha":%q}`, sha)
}
