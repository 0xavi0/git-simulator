package core

import (
	"fmt"
	"io/fs"
	"log"
	"os"
)

// LocalDirContent is a ContentProvider that reads a host-filesystem directory
// on every Files() call. Re-reading on each call means modifications to dir
// between two consecutive pushes produce commits with distinct trees.
type LocalDirContent struct {
	dir string
}

// NewLocalDirContent returns a ContentProvider backed by dir.
// The struct holds only the dir path (immutable); no mutex is needed.
func NewLocalDirContent(dir string) *LocalDirContent {
	return &LocalDirContent{dir: dir}
}

// Files walks dir and returns a forward-slash path→bytes map.
// It skips .git entries (directory or submodule file), symlinks, and
// directories (git does not track empty dirs; non-empty ones are implied
// by their contained files).
// Returns an error if dir does not exist or is not a directory.
func (l *LocalDirContent) Files(_ ContentContext) (map[string][]byte, error) {
	info, err := os.Stat(l.dir)
	if err != nil {
		return nil, fmt.Errorf("localdir: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("localdir: %q is not a directory", l.dir)
	}
	return filesFromFS(os.DirFS(l.dir))
}

// filesFromFS collects all regular files from fsys, using fs.ReadFile so
// callers can substitute an fstest.MapFS in unit tests.
func filesFromFS(fsys fs.FS) (map[string][]byte, error) {
	out := make(map[string][]byte)
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Skip .git at any depth (git directory or submodule marker file).
		if d.Name() == ".git" {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		// Directories are implicit in file paths; skip them.
		if d.IsDir() {
			return nil
		}
		// Skip symlinks — not followed in v1.
		if d.Type()&fs.ModeSymlink != 0 {
			log.Printf("localdir: skipping symlink %q", path)
			return nil
		}
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return fmt.Errorf("localdir: reading %q: %w", path, err)
		}
		out[path] = data
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}
