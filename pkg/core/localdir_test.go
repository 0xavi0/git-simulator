package core_test

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"github.com/rancher/gitsim/pkg/core"
	"github.com/rancher/gitsim/pkg/gitsim"
	_ "github.com/rancher/gitsim/pkg/provider/github"
)

// writeTempDir creates a temporary directory populated with the given files
// (path keys use forward slashes; subdirectories are created automatically).
func writeTempDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for path, content := range files {
		full := filepath.Join(dir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("MkdirAll %q: %v", filepath.Dir(full), err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("WriteFile %q: %v", full, err)
		}
	}
	return dir
}

// fileKeys returns the keys of a file map for test diagnostics.
func fileKeys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestLocalDirContent_FlatFiles(t *testing.T) {
	want := map[string]string{
		"README.md":   "# hello",
		"config.yaml": "key: value",
	}
	dir := writeTempDir(t, want)

	got, err := core.NewLocalDirContent(dir).Files(core.ContentContext{})
	if err != nil {
		t.Fatalf("Files: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("want %d files, got %d: %v", len(want), len(got), fileKeys(got))
	}
	for path, content := range want {
		if string(got[path]) != content {
			t.Errorf("%q: want %q got %q", path, content, got[path])
		}
	}
}

func TestLocalDirContent_NestedDirs(t *testing.T) {
	want := map[string]string{
		"README.md":                 "nested test",
		"charts/values.yaml":        "replicaCount: 1",
		"charts/templates/pod.yaml": "apiVersion: v1",
	}
	dir := writeTempDir(t, want)

	got, err := core.NewLocalDirContent(dir).Files(core.ContentContext{})
	if err != nil {
		t.Fatalf("Files: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("want %d files, got %d: %v", len(want), len(got), fileKeys(got))
	}
	for path, content := range want {
		if string(got[path]) != content {
			t.Errorf("%q: want %q got %q", path, content, got[path])
		}
	}
}

func TestLocalDirContent_GitDirExcluded(t *testing.T) {
	want := map[string]string{
		"README.md": "# project",
	}
	dir := writeTempDir(t, want)

	// Simulate a real .git directory with nested content.
	gitDir := filepath.Join(dir, ".git")
	if err := os.MkdirAll(filepath.Join(gitDir, "refs", "heads"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []struct{ name, body string }{
		{"config", "[core]\n\trepositoryformatversion = 0\n"},
		{"HEAD", "ref: refs/heads/main\n"},
	} {
		if err := os.WriteFile(filepath.Join(gitDir, f.name), []byte(f.body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := core.NewLocalDirContent(dir).Files(core.ContentContext{})
	if err != nil {
		t.Fatalf("Files: %v", err)
	}
	if len(got) != len(want) {
		t.Fatalf("want %d files (no .git entries), got %d: %v", len(want), len(got), fileKeys(got))
	}
	if string(got["README.md"]) != "# project" {
		t.Errorf("README.md: want %q got %q", "# project", got["README.md"])
	}
}

func TestLocalDirContent_SymlinkSkipped(t *testing.T) {
	want := map[string]string{
		"real.txt":  "real file",
		"other.txt": "another real file",
	}
	dir := writeTempDir(t, want)

	// Symlink points at a real file inside the same dir.
	if err := os.Symlink(filepath.Join(dir, "real.txt"), filepath.Join(dir, "link.txt")); err != nil {
		t.Skip("symlinks not supported on this platform:", err)
	}

	got, err := core.NewLocalDirContent(dir).Files(core.ContentContext{})
	if err != nil {
		t.Fatalf("Files must not error on symlinks, got: %v", err)
	}
	if _, ok := got["link.txt"]; ok {
		t.Error("symlink must be excluded from the returned file map")
	}
	if len(got) != len(want) {
		t.Fatalf("want %d files (symlink excluded), got %d: %v", len(want), len(got), fileKeys(got))
	}
}

func TestLocalDirContent_NonExistentDir(t *testing.T) {
	_, err := core.NewLocalDirContent("/no/such/directory/xyz-localdir-test").Files(core.ContentContext{})
	if err == nil {
		t.Fatal("expected error for non-existent directory, got nil")
	}
}

func TestLocalDirContent_ConcurrentCalls(t *testing.T) {
	dir := writeTempDir(t, map[string]string{
		"a.txt": "alpha",
		"b.txt": "beta",
	})
	cp := core.NewLocalDirContent(dir)

	const goroutines = 8
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for range goroutines {
		go func() {
			defer wg.Done()
			if _, err := cp.Files(core.ContentContext{}); err != nil {
				t.Errorf("concurrent Files: %v", err)
			}
		}()
	}
	wg.Wait()
}

// TestLocalDirContent_Integration verifies the end-to-end path:
// LocalDirContent → gitsim → PlainClone → worktree matches the source dir.
func TestLocalDirContent_Integration(t *testing.T) {
	srcFiles := map[string]string{
		"README.md":             "# integration test",
		"manifests/deploy.yaml": "apiVersion: apps/v1\nkind: Deployment",
		"manifests/svc.yaml":    "apiVersion: v1\nkind: Service",
	}
	srcDir := writeTempDir(t, srcFiles)

	sim := gitsim.New(
		gitsim.WithProviders("github"),
		gitsim.WithContent(core.NewLocalDirContent(srcDir)),
	)
	srv := httptest.NewServer(sim.Handler())
	t.Cleanup(srv.Close)
	sim.SetBaseURL(srv.URL)

	repo, err := sim.CreateRepo("github", "acme", "localdir-int", "main")
	if err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}
	expectedSHA, ok := repo.VisibleCommit("main")
	if !ok {
		t.Fatal("initial commit not visible")
	}

	cloneDir := t.TempDir()
	_, err = gogit.PlainClone(cloneDir, false, &gogit.CloneOptions{
		URL:           repo.CloneURL(),
		Depth:         1,
		SingleBranch:  true,
		ReferenceName: plumbing.NewBranchReferenceName("main"),
		Tags:          gogit.NoTags,
	})
	if err != nil {
		t.Fatalf("PlainClone: %v", err)
	}

	cloned, err := gogit.PlainOpen(cloneDir)
	if err != nil {
		t.Fatalf("PlainOpen: %v", err)
	}
	head, err := cloned.Head()
	if err != nil {
		t.Fatalf("HEAD: %v", err)
	}
	if head.Hash().String() != expectedSHA {
		t.Errorf("clone HEAD: got %s want %s", head.Hash(), expectedSHA)
	}

	for path := range srcFiles {
		full := filepath.Join(cloneDir, filepath.FromSlash(path))
		if _, statErr := os.Stat(full); statErr != nil {
			t.Errorf("expected file %q in cloned worktree: %v", path, statErr)
		}
	}
}
