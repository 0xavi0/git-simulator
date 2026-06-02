// Package examples_test contains runnable examples for the git-simulator SDK.
// Run them with:
//
//	go test -v github.com/rancher/gitsim/examples
package examples_test

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	ghwebhooks "github.com/go-playground/webhooks/v6/github"
	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"

	"github.com/rancher/gitsim/pkg/core"
	"github.com/rancher/gitsim/pkg/gitsim"
	_ "github.com/rancher/gitsim/pkg/provider/github"
)

// TestExample_WebhookBeforeHead reproduces Fleet issue #4837:
// a webhook carrying commit C arrives at Fleet before the remote's advertised
// HEAD has been updated to C.
//
// Sequence:
//  1. Start the simulator and create a GitHub repo.
//  2. Push commit C with a 300 ms HEAD delay (the branch ref stays at the initial SHA).
//  3. Fire the webhook immediately — Fleet receives SHA C before ls-remote shows it.
//  4. Assert ls-remote still returns the initial SHA (race window open).
//  5. Advance the clock — the branch ref promotes to C.
//  6. Assert ls-remote now returns C.
func TestExample_WebhookBeforeHead(t *testing.T) {
	const secret = "fleet-webhook-secret"
	const headDelay = 300 * time.Millisecond

	// Use a ManualClock so the HEAD promotion is deterministic (no real sleep needed).
	t0 := time.Unix(1_000_000, 0)
	clock := core.NewManualClock(t0)

	// ── 1. Start simulator ────────────────────────────────────────────────────
	sim := gitsim.New(
		gitsim.WithClock(clock),
		gitsim.WithProviders("github"),
	)
	srv := httptest.NewServer(sim.Handler())
	t.Cleanup(srv.Close)
	sim.SetBaseURL(srv.URL)

	repo, err := sim.CreateRepo("github", "acme", "myapp", "main")
	if err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}

	initialSHA, _ := repo.VisibleCommit("main")

	// ── 2. Push with a HEAD delay ─────────────────────────────────────────────
	// The commit object is stored immediately; the branch ref stays at initialSHA
	// until clock advances past headDelay.
	newSHA, err := repo.Push(gitsim.OnBranch("main"), gitsim.HeadDelay(headDelay))
	if err != nil {
		t.Fatalf("Push: %v", err)
	}
	t.Logf("initial SHA: %s", initialSHA)
	t.Logf("new SHA:     %s", newSHA)

	// ── 3. Start a fake Fleet webhook receiver ────────────────────────────────
	var mu sync.Mutex
	var receivedSHA string

	hook, _ := ghwebhooks.New(ghwebhooks.Options.Secret(secret))
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, err := hook.Parse(r, ghwebhooks.PushEvent)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		push := payload.(ghwebhooks.PushPayload)
		mu.Lock()
		receivedSHA = push.After
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(receiver.Close)

	// Fire the webhook immediately (SendAfter = 0).
	result, err := sim.Webhook(repo, newSHA,
		gitsim.Target(receiver.URL),
		gitsim.SendAfter(0),
		gitsim.Secret(secret),
	)
	if err != nil {
		t.Fatalf("Webhook: %v", err)
	}
	if result.StatusCode != http.StatusOK {
		t.Fatalf("webhook receiver returned %d: %s", result.StatusCode, result.Body)
	}

	mu.Lock()
	got := receivedSHA
	mu.Unlock()
	if got != newSHA {
		t.Errorf("webhook After: got %q want %q", got, newSHA)
	}

	// ── 4. ls-remote still shows the initial SHA ──────────────────────────────
	// Fleet's controller, polling right after receiving the webhook, would still
	// see the old HEAD — the race window is open.
	refs := lsRemote(t, repo.CloneURL())
	if tip := refs["refs/heads/main"]; tip.String() != initialSHA {
		t.Errorf("before promotion — ls-remote HEAD: got %s want %s", tip, initialSHA)
	}

	// ── 5 & 6. Advance clock → HEAD promotes → ls-remote catches up ──────────
	clock.Advance(headDelay + time.Millisecond)

	refs = lsRemote(t, repo.CloneURL())
	if tip := refs["refs/heads/main"]; tip.String() != newSHA {
		t.Errorf("after promotion — ls-remote HEAD: got %s want %s", tip, newSHA)
	}
}

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
