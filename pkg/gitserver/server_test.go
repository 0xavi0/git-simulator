package gitserver_test

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"

	"github.com/rancher/gitsim/pkg/core"
	"github.com/rancher/gitsim/pkg/gitserver"
	"github.com/rancher/gitsim/pkg/store"
)

const testHost = "sim.test"

var testFiles = map[string][]byte{
	"README.md":   []byte("# hello"),
	"src/main.go": []byte("package main\nfunc main() {}"),
	"src/util.go": []byte("package main\nfunc util() {}"),
}

func newTestEnv(t *testing.T) (*store.Store, *core.ManualClock, *httptest.Server) {
	t.Helper()
	base := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	mc := core.NewManualClock(base)
	mc.Set(base)
	s := store.New(mc, core.NewStaticContent(testFiles))
	srv := httptest.NewServer(gitserver.NewHandler(testHost, s))
	t.Cleanup(srv.Close)
	return s, mc, srv
}

func repoURL(srv *httptest.Server, owner, repo string) string {
	return srv.URL + "/" + owner + "/" + repo + ".git"
}

func listRefs(t *testing.T, rawURL string) map[plumbing.ReferenceName]plumbing.Hash {
	t.Helper()
	remote := gogit.NewRemote(memory.NewStorage(), &config.RemoteConfig{
		URLs: []string{rawURL},
	})
	refs, err := remote.List(&gogit.ListOptions{})
	if err != nil {
		t.Fatalf("ls-remote %s: %v", rawURL, err)
	}
	m := make(map[plumbing.ReferenceName]plumbing.Hash, len(refs))
	for _, r := range refs {
		m[r.Name()] = r.Hash()
	}
	return m
}

// TestLsRemote verifies that go-git's remote.List() returns the branch SHA
// that matches the store's VisibleCommit.
func TestLsRemote(t *testing.T) {
	s, _, srv := newTestEnv(t)

	r, err := s.CreateRepo(testHost, "acme", "widget", "main")
	if err != nil {
		t.Fatal(err)
	}
	expectedSHA, _ := r.VisibleCommit("main")

	refs := listRefs(t, repoURL(srv, "acme", "widget"))

	got, ok := refs[plumbing.ReferenceName("refs/heads/main")]
	if !ok {
		t.Fatalf("refs/heads/main not in advertisement; got %v", refs)
	}
	if got.String() != expectedSHA {
		t.Fatalf("ls-remote: want %s got %s", expectedSHA, got)
	}
}

// TestRace is the headline test: AddCommit with delay > 0 keeps the old SHA
// visible to ls-remote until the clock advances past promoteAt.
func TestRace(t *testing.T) {
	s, mc, srv := newTestEnv(t)

	r, err := s.CreateRepo(testHost, "acme", "widget", "main")
	if err != nil {
		t.Fatal(err)
	}
	oldSHA, _ := r.VisibleCommit("main")

	newSHA, err := r.AddCommit("main", 300*time.Millisecond)
	if err != nil {
		t.Fatalf("AddCommit: %v", err)
	}
	if newSHA == oldSHA {
		t.Fatal("AddCommit returned same SHA as parent")
	}

	url := repoURL(srv, "acme", "widget")

	// Before promotion: ls-remote must still advertise the old SHA.
	refs := listRefs(t, url)
	if got := refs[plumbing.ReferenceName("refs/heads/main")]; got.String() != oldSHA {
		t.Fatalf("before promotion: want %s got %s", oldSHA, got)
	}

	// Advance clock past the delay.
	mc.Advance(400 * time.Millisecond)

	// After promotion: ls-remote must advertise the new SHA.
	refs = listRefs(t, url)
	if got := refs[plumbing.ReferenceName("refs/heads/main")]; got.String() != newSHA {
		t.Fatalf("after promotion: want %s got %s", newSHA, got)
	}
}

// TestClone verifies that gogit.PlainClone with Depth:1, SingleBranch:true
// succeeds and the worktree contains the StaticContent files.
func TestClone(t *testing.T) {
	s, _, srv := newTestEnv(t)

	r, err := s.CreateRepo(testHost, "acme", "widget", "main")
	if err != nil {
		t.Fatal(err)
	}
	expectedSHA, _ := r.VisibleCommit("main")

	dir := t.TempDir()
	_, err = gogit.PlainClone(dir, false, &gogit.CloneOptions{
		URL:           repoURL(srv, "acme", "widget"),
		Depth:         1,
		SingleBranch:  true,
		ReferenceName: plumbing.ReferenceName("refs/heads/main"),
		Tags:          gogit.NoTags,
	})
	if err != nil {
		t.Fatalf("PlainClone: %v", err)
	}

	// Verify HEAD in the cloned repo points to the expected commit.
	cloned, err := gogit.PlainOpen(dir)
	if err != nil {
		t.Fatalf("PlainOpen: %v", err)
	}
	head, err := cloned.Head()
	if err != nil {
		t.Fatalf("HEAD: %v", err)
	}
	if head.Hash().String() != expectedSHA {
		t.Fatalf("clone HEAD: want %s got %s", expectedSHA, head.Hash())
	}

	// Verify StaticContent files are present in the worktree.
	for path := range testFiles {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(path))); err != nil {
			t.Errorf("file %q missing from worktree: %v", path, err)
		}
	}
}

// TestClonePreservesHistory verifies that after two immediate commits, a full
// clone contains both commits — proving the server preserves history and an
// older revision (not the branch tip) remains accessible by SHA.
func TestClonePreservesHistory(t *testing.T) {
	s, _, srv := newTestEnv(t)

	r, err := s.CreateRepo(testHost, "acme", "widget", "main")
	if err != nil {
		t.Fatal(err)
	}
	sha0, _ := r.VisibleCommit("main")

	sha1, err := r.AddCommit("main", 0)
	if err != nil {
		t.Fatalf("AddCommit: %v", err)
	}

	dir := t.TempDir()
	cloned, err := gogit.PlainClone(dir, false, &gogit.CloneOptions{
		URL:           repoURL(srv, "acme", "widget"),
		SingleBranch:  true,
		ReferenceName: plumbing.ReferenceName("refs/heads/main"),
		Tags:          gogit.NoTags,
	})
	if err != nil {
		t.Fatalf("PlainClone: %v", err)
	}

	// HEAD should be sha1 (the latest commit).
	head, _ := cloned.Head()
	if head.Hash().String() != sha1 {
		t.Fatalf("clone HEAD: want %s got %s", sha1, head.Hash())
	}

	// sha0 (the initial commit) must also be present — older revisions are
	// preserved and accessible even after the branch tip has moved.
	if _, err := cloned.CommitObject(plumbing.NewHash(sha0)); err != nil {
		t.Fatalf("initial commit %s not in cloned repo: %v", sha0, err)
	}
}

// TestNotFound verifies that an unknown repo returns 404.
func TestNotFound(t *testing.T) {
	_, _, srv := newTestEnv(t)

	remote := gogit.NewRemote(memory.NewStorage(), &config.RemoteConfig{
		URLs: []string{repoURL(srv, "nobody", "ghost")},
	})
	_, err := remote.List(&gogit.ListOptions{})
	if err == nil {
		t.Fatal("expected error for unknown repo, got nil")
	}
}
