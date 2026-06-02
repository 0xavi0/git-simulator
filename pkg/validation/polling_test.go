package validation_test

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
	"github.com/rancher/gitsim/pkg/gitsim"
	_ "github.com/rancher/gitsim/pkg/provider/github"
)

// lsRemote mirrors Fleet's pkg/git/remote.go: LatestBranchCommit uses
// go-git remote.List() which speaks git smart-HTTP.
func lsRemote(t *testing.T, rawURL string) map[plumbing.ReferenceName]plumbing.Hash {
	t.Helper()
	remote := gogit.NewRemote(memory.NewStorage(), &config.RemoteConfig{
		URLs: []string{rawURL},
	})
	refs, err := remote.List(&gogit.ListOptions{})
	if err != nil {
		t.Fatalf("ls-remote %s: %v", rawURL, err)
	}
	m := make(map[plumbing.ReferenceName]plumbing.Hash, len(refs))
	for _, ref := range refs {
		m[ref.Name()] = ref.Hash()
	}
	return m
}

// TestPolling_LsRemoteLag exercises the exact code path Fleet's controller uses
// when polling for the latest commit on a branch:
//  1. Push a new commit with a HEAD delay (ref stays at the previous SHA).
//  2. ls-remote must still advertise the old SHA (race window is open).
//  3. After the clock advances past the delay, ls-remote catches up to the new SHA.
func TestPolling_LsRemoteLag(t *testing.T) {
	const headDelay = 200 * time.Millisecond

	t0 := time.Unix(2_000_000, 0)
	clock := core.NewManualClock(t0)

	sim := gitsim.New(gitsim.WithClock(clock), gitsim.WithProviders("github"))
	srv := httptest.NewServer(sim.Handler())
	t.Cleanup(srv.Close)
	sim.SetBaseURL(srv.URL)

	repo, err := sim.CreateRepo("github", "acme", "poll-lag", "main")
	if err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}

	oldSHA, ok := repo.VisibleCommit("main")
	if !ok {
		t.Fatal("initial commit not visible")
	}

	newSHA, err := repo.Push(gitsim.OnBranch("main"), gitsim.HeadDelay(headDelay))
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	if newSHA == oldSHA {
		t.Fatal("Push must return a new SHA")
	}

	// ── Before promotion: Fleet would poll and still see the old SHA. ──
	refs := lsRemote(t, repo.CloneURL())
	if got := refs["refs/heads/main"]; got.String() != oldSHA {
		t.Errorf("before promotion — ls-remote: got %s want %s", got, oldSHA)
	}

	// ── Advance clock past the delay: HEAD promotes. ──
	clock.Advance(headDelay + time.Millisecond)

	refs = lsRemote(t, repo.CloneURL())
	if got := refs["refs/heads/main"]; got.String() != newSHA {
		t.Errorf("after promotion — ls-remote: got %s want %s", got, newSHA)
	}
}

// TestPolling_Clone exercises the code path Fleet's git-job uses:
// gogit.PlainClone with Depth:1, SingleBranch:true.
// It verifies that HEAD points at the expected SHA and the content files are present.
func TestPolling_Clone(t *testing.T) {
	files := map[string][]byte{
		"README.md":  []byte("# gitsim example"),
		"config.yml": []byte("key: value"),
	}

	sim := gitsim.New(
		gitsim.WithProviders("github"),
		gitsim.WithContent(core.NewStaticContent(files)),
	)
	srv := httptest.NewServer(sim.Handler())
	t.Cleanup(srv.Close)
	sim.SetBaseURL(srv.URL)

	repo, err := sim.CreateRepo("github", "acme", "clone-test", "main")
	if err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}
	expectedSHA, _ := repo.VisibleCommit("main")

	dir := t.TempDir()
	_, err = gogit.PlainClone(dir, false, &gogit.CloneOptions{
		URL:           repo.CloneURL(),
		Depth:         1,
		SingleBranch:  true,
		ReferenceName: plumbing.NewBranchReferenceName("main"),
		Tags:          gogit.NoTags,
	})
	if err != nil {
		t.Fatalf("PlainClone: %v", err)
	}

	cloned, err := gogit.PlainOpen(dir)
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

	for path := range files {
		full := filepath.Join(dir, filepath.FromSlash(path))
		if _, err := os.Stat(full); err != nil {
			t.Errorf("expected file %q in worktree: %v", path, err)
		}
	}
}
