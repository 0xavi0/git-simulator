package gitserver

import (
	"fmt"
	"strings"

	"github.com/rancher/gitsim/pkg/store"
)

// RepoStore is the subset of *store.Store that the Handler needs.
type RepoStore interface {
	GetRepo(host, owner, name string) (*store.Repo, error)
}

// resolveRepo extracts owner/repo from a URL path segment of the form
// "/owner/repo.git" (or "/owner/repo.git/...") and returns the matching Repo.
//
// It also flushes any due lazy promotions via VisibleRefs so the storer's ref
// state is current before the caller reads it. This is what makes the
// visible-vs-pending HEAD race observable over the wire.
func resolveRepo(rs RepoStore, host, urlPath string) (*store.Repo, error) {
	path := strings.TrimPrefix(urlPath, "/")

	idx := strings.Index(path, ".git")
	if idx == -1 {
		return nil, fmt.Errorf("invalid repo path %q: expected /owner/repo.git[/...]", urlPath)
	}
	repoPath := path[:idx]

	parts := strings.SplitN(repoPath, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return nil, fmt.Errorf("invalid repo path %q: expected /owner/repo.git[/...]", urlPath)
	}
	owner, repoName := parts[0], parts[1]

	repo, err := rs.GetRepo(host, owner, repoName)
	if err != nil {
		return nil, err
	}

	// Flush due promotions so the storer reflects the current visibleSHA.
	if _, err := repo.VisibleRefs(); err != nil {
		return nil, fmt.Errorf("flush refs for %s/%s: %w", owner, repoName, err)
	}
	return repo, nil
}
