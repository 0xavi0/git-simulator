package core

// Commit represents a point-in-time snapshot in a repository.
type Commit struct {
	SHA     string
	Message string
	Author  string
	// Parents holds the SHAs of parent commits (empty for the initial commit).
	Parents []string
}

// Ref is a named pointer to a commit (branch or tag).
type Ref struct {
	Name string // e.g. "refs/heads/main" or "refs/tags/v1.0"
	SHA  string
}

// Repo is the identity of a hosted repository.
type Repo struct {
	Host  string // e.g. "github.com"
	Owner string // org or user
	Name  string // repository name
}

// Branch names a branch within a repo.
type Branch struct {
	Repo Repo
	Name string // short name, e.g. "main"
}

// Tag names a tag within a repo.
type Tag struct {
	Repo Repo
	Name string // short name, e.g. "v1.0"
}
