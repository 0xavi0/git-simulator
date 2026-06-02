package core

import "errors"

// Sentinel errors shared across packages.
var (
	// ErrNotFound is returned when a repo, branch, or commit does not exist.
	ErrNotFound = errors.New("not found")

	// ErrAlreadyExists is returned when creating a resource that already exists.
	ErrAlreadyExists = errors.New("already exists")

	// ErrNoCommits is returned when a branch has no commits yet.
	ErrNoCommits = errors.New("no commits")
)
