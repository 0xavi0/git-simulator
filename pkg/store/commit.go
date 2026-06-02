package store

import (
	"sort"
	"strings"
	"time"

	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	gogitstorage "github.com/go-git/go-git/v5/storage"
)

// buildCommit writes blob, tree, and commit objects into s and returns the commit SHA.
// parentSHA may be empty for the initial commit.
func buildCommit(s gogitstorage.Storer, parentSHA string, files map[string][]byte, message string, when time.Time) (string, error) {
	treeHash, err := buildTree(s, files)
	if err != nil {
		return "", err
	}

	sig := object.Signature{
		Name:  "gitsim",
		Email: "gitsim@example.com",
		When:  when,
	}
	c := &object.Commit{
		TreeHash:  treeHash,
		Author:    sig,
		Committer: sig,
		Message:   message,
	}
	if parentSHA != "" {
		c.ParentHashes = []plumbing.Hash{plumbing.NewHash(parentSHA)}
	}

	obj := s.NewEncodedObject()
	if err := c.Encode(obj); err != nil {
		return "", err
	}
	hash, err := s.SetEncodedObject(obj)
	if err != nil {
		return "", err
	}
	return hash.String(), nil
}

// buildTree recursively writes tree objects for the given files map and returns the tree hash.
// Keys in files may contain "/" to represent nested directories.
func buildTree(s gogitstorage.Storer, files map[string][]byte) (plumbing.Hash, error) {
	rootFiles := map[string][]byte{}
	subdirs := map[string]map[string][]byte{}

	for path, content := range files {
		idx := strings.Index(path, "/")
		if idx == -1 {
			rootFiles[path] = content
		} else {
			dir := path[:idx]
			rest := path[idx+1:]
			if subdirs[dir] == nil {
				subdirs[dir] = map[string][]byte{}
			}
			subdirs[dir][rest] = content
		}
	}

	entries := make([]object.TreeEntry, 0, len(rootFiles)+len(subdirs))

	for name, content := range rootFiles {
		blobHash, err := writeBlob(s, content)
		if err != nil {
			return plumbing.ZeroHash, err
		}
		entries = append(entries, object.TreeEntry{
			Name: name,
			Mode: filemode.Regular,
			Hash: blobHash,
		})
	}

	for dir, subfiles := range subdirs {
		subHash, err := buildTree(s, subfiles)
		if err != nil {
			return plumbing.ZeroHash, err
		}
		entries = append(entries, object.TreeEntry{
			Name: dir,
			Mode: filemode.Dir,
			Hash: subHash,
		})
	}

	// git sorts tree entries as if directory names have a trailing "/".
	// go-git's Tree.Encode validates this exact order, so we must match it.
	sort.Slice(entries, func(i, j int) bool {
		ni, nj := entries[i].Name, entries[j].Name
		if entries[i].Mode == filemode.Dir {
			ni += "/"
		}
		if entries[j].Mode == filemode.Dir {
			nj += "/"
		}
		return ni < nj
	})

	tree := &object.Tree{Entries: entries}
	obj := s.NewEncodedObject()
	if err := tree.Encode(obj); err != nil {
		return plumbing.ZeroHash, err
	}
	return s.SetEncodedObject(obj)
}

func writeBlob(s gogitstorage.Storer, content []byte) (plumbing.Hash, error) {
	obj := s.NewEncodedObject()
	obj.SetType(plumbing.BlobObject)
	w, err := obj.Writer()
	if err != nil {
		return plumbing.ZeroHash, err
	}
	if _, err := w.Write(content); err != nil {
		_ = w.Close()
		return plumbing.ZeroHash, err
	}
	if err := w.Close(); err != nil {
		return plumbing.ZeroHash, err
	}
	return s.SetEncodedObject(obj)
}
