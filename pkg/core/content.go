package core

// ContentProvider produces the file tree for a commit.
// v1 is static; later stages can supply dynamic content by implementing this interface.
type ContentProvider interface {
	// Files returns path→bytes for the given repo/branch/commit context.
	Files(ctx ContentContext) (map[string][]byte, error)
}

// ContentContext carries the context in which content is requested.
type ContentContext struct {
	Repo      string
	Branch    string
	CommitSHA string
	// Index is the Nth commit on the branch, for future dynamic content.
	Index int
}

// StaticContent is a ContentProvider that always returns the same file tree.
type StaticContent struct {
	files map[string][]byte
}

// NewStaticContent creates a StaticContent with the given files (path→bytes).
func NewStaticContent(files map[string][]byte) *StaticContent {
	cp := make(map[string][]byte, len(files))
	for k, v := range files {
		cp[k] = v
	}
	return &StaticContent{files: cp}
}

func (s *StaticContent) Files(_ ContentContext) (map[string][]byte, error) {
	out := make(map[string][]byte, len(s.files))
	for k, v := range s.files {
		out[k] = v
	}
	return out, nil
}
