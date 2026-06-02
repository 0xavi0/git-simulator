package store

import (
	"fmt"
	"sync"

	"github.com/rancher/gitsim/pkg/core"
)

// Store is the thread-safe registry of simulated git repositories.
// It implements core.CommitResolver so Providers can resolve visible commits
// without importing this package directly.
type Store struct {
	mu      sync.RWMutex
	repos   map[repoKey]*Repo
	clock   core.Clock
	content core.ContentProvider
}

type repoKey struct {
	host, owner, name string
}

// New creates a Store backed by the given clock and content provider.
func New(clock core.Clock, content core.ContentProvider) *Store {
	return &Store{
		repos:   make(map[repoKey]*Repo),
		clock:   clock,
		content: content,
	}
}

// CreateRepo registers a new repository and seeds it with an initial commit on defaultBranch.
// Returns ErrAlreadyExists if a repo with that identity already exists.
func (s *Store) CreateRepo(host, owner, name, defaultBranch string) (*Repo, error) {
	key := repoKey{host, owner, name}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, exists := s.repos[key]; exists {
		return nil, fmt.Errorf("%w: %s/%s/%s", core.ErrAlreadyExists, host, owner, name)
	}

	r, err := newRepo(host, owner, name, defaultBranch, s.clock, s.content)
	if err != nil {
		return nil, err
	}
	s.repos[key] = r
	return r, nil
}

// GetRepo returns the named repo or ErrNotFound.
func (s *Store) GetRepo(host, owner, name string) (*Repo, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	r, ok := s.repos[repoKey{host, owner, name}]
	if !ok {
		return nil, fmt.Errorf("%w: %s/%s/%s", core.ErrNotFound, host, owner, name)
	}
	return r, nil
}

// List returns all registered repos in unspecified order.
func (s *Store) List() []*Repo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	out := make([]*Repo, 0, len(s.repos))
	for _, r := range s.repos {
		out = append(out, r)
	}
	return out
}

// VisibleCommit implements core.CommitResolver.
func (s *Store) VisibleCommit(host, owner, name, branch string) (string, bool) {
	r, err := s.GetRepo(host, owner, name)
	if err != nil {
		return "", false
	}
	return r.VisibleCommit(branch)
}
