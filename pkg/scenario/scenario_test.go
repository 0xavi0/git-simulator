package scenario_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"

	"github.com/rancher/gitsim/pkg/core"
	"github.com/rancher/gitsim/pkg/gitsim"
	_ "github.com/rancher/gitsim/pkg/provider/github"
	"github.com/rancher/gitsim/pkg/scenario"
)

// lsRemote does a git ls-remote against rawURL and returns the ref map.
func lsRemote(t *testing.T, rawURL string) map[plumbing.ReferenceName]plumbing.Hash {
	t.Helper()
	remote := gogit.NewRemote(memory.NewStorage(), &config.RemoteConfig{URLs: []string{rawURL}})
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

// shaReceiver returns an httptest.Server that records "after" SHAs in arrival order.
func shaReceiver(t *testing.T) (*httptest.Server, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var arrivals []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var pl struct {
			After string `json:"after"`
		}
		if err := json.NewDecoder(r.Body).Decode(&pl); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		mu.Lock()
		arrivals = append(arrivals, pl.After)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, func() []string {
		mu.Lock()
		defer mu.Unlock()
		cp := make([]string, len(arrivals))
		copy(cp, arrivals)
		return cp
	}
}

// makeFireFunc builds a scenario.FireFunc backed by sim+repo, converting
// emitter.Result to scenario.WebhookResult.
func makeFireFunc(sim *gitsim.Simulator, repo *gitsim.SimRepo) scenario.FireFunc {
	return func(sha, target, secret string, sendAfter time.Duration) (scenario.WebhookResult, error) {
		opts := []gitsim.WebhookOption{gitsim.Target(target), gitsim.SendAfter(sendAfter)}
		if secret != "" {
			opts = append(opts, gitsim.Secret(secret))
		}
		result, err := sim.Webhook(repo, sha, opts...)
		return scenario.WebhookResult{
			StatusCode: result.StatusCode,
			Body:       result.Body,
			Err:        result.Err,
		}, err
	}
}

// ─── PushN tests ──────────────────────────────────────────────────────────────

func TestRunPushN_AscOrder(t *testing.T) {
	sim := gitsim.New(gitsim.WithProviders("github"))
	srv := httptest.NewServer(sim.Handler())
	t.Cleanup(srv.Close)
	sim.SetBaseURL(srv.URL)

	repo, err := sim.CreateRepo("github", "acme", "app", "main")
	if err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}

	receiver, getArrivals := shaReceiver(t)

	result, err := scenario.RunPushN(context.Background(), repo, makeFireFunc(sim, repo), scenario.PushNSpec{
		N:      3,
		Order:  scenario.OrderAsc,
		Target: receiver.URL,
	})
	if err != nil {
		t.Fatalf("RunPushN: %v", err)
	}

	if len(result.Commits) != 3 {
		t.Fatalf("Commits: got %d want 3", len(result.Commits))
	}
	if len(result.DeliveryOrder) != 3 {
		t.Fatalf("DeliveryOrder: got %d want 3", len(result.DeliveryOrder))
	}
	for i, want := range []int{0, 1, 2} {
		if result.DeliveryOrder[i] != want {
			t.Errorf("DeliveryOrder[%d]: got %d want %d", i, result.DeliveryOrder[i], want)
		}
	}

	arrivals := getArrivals()
	if len(arrivals) != 3 {
		t.Fatalf("arrivals: got %d want 3", len(arrivals))
	}
	for i, sha := range result.Commits {
		if arrivals[i] != sha {
			t.Errorf("arrival[%d]: got %q want %q", i, arrivals[i], sha)
		}
	}
}

func TestRunPushN_DescOrder(t *testing.T) {
	sim := gitsim.New(gitsim.WithProviders("github"))
	srv := httptest.NewServer(sim.Handler())
	t.Cleanup(srv.Close)
	sim.SetBaseURL(srv.URL)

	repo, err := sim.CreateRepo("github", "acme", "app", "main")
	if err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}

	receiver, getArrivals := shaReceiver(t)

	result, err := scenario.RunPushN(context.Background(), repo, makeFireFunc(sim, repo), scenario.PushNSpec{
		N:      3,
		Order:  scenario.OrderDesc,
		Target: receiver.URL,
	})
	if err != nil {
		t.Fatalf("RunPushN: %v", err)
	}

	// Desc = newest-first: indices [2, 1, 0].
	for i, want := range []int{2, 1, 0} {
		if result.DeliveryOrder[i] != want {
			t.Errorf("DeliveryOrder[%d]: got %d want %d", i, result.DeliveryOrder[i], want)
		}
	}

	arrivals := getArrivals()
	for i, idx := range result.DeliveryOrder {
		if arrivals[i] != result.Commits[idx] {
			t.Errorf("arrival[%d]: got %q want %q", i, arrivals[i], result.Commits[idx])
		}
	}
}

func TestRunPushN_CustomOrder(t *testing.T) {
	sim := gitsim.New(gitsim.WithProviders("github"))
	srv := httptest.NewServer(sim.Handler())
	t.Cleanup(srv.Close)
	sim.SetBaseURL(srv.URL)

	repo, err := sim.CreateRepo("github", "acme", "app", "main")
	if err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}

	receiver, getArrivals := shaReceiver(t)

	customOrder := []int{1, 0, 2}
	result, err := scenario.RunPushN(context.Background(), repo, makeFireFunc(sim, repo), scenario.PushNSpec{
		N:           3,
		Order:       scenario.OrderCustom,
		CustomOrder: customOrder,
		Target:      receiver.URL,
	})
	if err != nil {
		t.Fatalf("RunPushN: %v", err)
	}

	for i, want := range customOrder {
		if result.DeliveryOrder[i] != want {
			t.Errorf("DeliveryOrder[%d]: got %d want %d", i, result.DeliveryOrder[i], want)
		}
	}

	arrivals := getArrivals()
	for i, idx := range result.DeliveryOrder {
		if arrivals[i] != result.Commits[idx] {
			t.Errorf("arrival[%d]: got %q want %q", i, arrivals[i], result.Commits[idx])
		}
	}
}

// ─── Race test ────────────────────────────────────────────────────────────────

// TestRunRace is the headline deterministic test.
// It uses a ManualClock so the HEAD lag is fully controlled:
//  1. RunRace pushes a commit with a 300ms HEAD delay and fires the webhook immediately.
//  2. ls-remote must still return OldSHA (clock not advanced).
//  3. Advance clock → ls-remote catches up to NewSHA.
func TestRunRace(t *testing.T) {
	const headDelay = 300 * time.Millisecond

	t0 := time.Unix(1_000_000, 0)
	clock := core.NewManualClock(t0)

	sim := gitsim.New(gitsim.WithClock(clock), gitsim.WithProviders("github"))
	srv := httptest.NewServer(sim.Handler())
	t.Cleanup(srv.Close)
	sim.SetBaseURL(srv.URL)

	repo, err := sim.CreateRepo("github", "acme", "race-test", "main")
	if err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}

	var mu sync.Mutex
	var receivedSHA string
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var pl struct {
			After string `json:"after"`
		}
		json.NewDecoder(r.Body).Decode(&pl) //nolint:errcheck
		mu.Lock()
		receivedSHA = pl.After
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(receiver.Close)

	result, err := scenario.RunRace(context.Background(), repo, makeFireFunc(sim, repo), scenario.RaceSpec{
		HeadDelayMs: headDelay.Milliseconds(),
		Target:      receiver.URL,
	})
	if err != nil {
		t.Fatalf("RunRace: %v", err)
	}

	if result.OldSHA == result.NewSHA {
		t.Fatalf("OldSHA == NewSHA (%q) — race scenario requires two distinct commits", result.OldSHA)
	}
	if result.WebhookResult.StatusCode != http.StatusOK {
		t.Fatalf("webhook status %d: %s", result.WebhookResult.StatusCode, result.WebhookResult.Body)
	}

	mu.Lock()
	got := receivedSHA
	mu.Unlock()
	if got != result.NewSHA {
		t.Errorf("webhook SHA: got %q want %q", got, result.NewSHA)
	}

	// ── Before promotion: ls-remote must still report OldSHA. ──
	refs := lsRemote(t, repo.CloneURL())
	if refs["refs/heads/main"].String() != result.OldSHA {
		t.Errorf("before promotion — ls-remote: got %s want %s", refs["refs/heads/main"], result.OldSHA)
	}

	// ── Advance clock → HEAD promotes → ls-remote catches up. ──
	clock.Advance(headDelay + time.Millisecond)

	refs = lsRemote(t, repo.CloneURL())
	if refs["refs/heads/main"].String() != result.NewSHA {
		t.Errorf("after promotion — ls-remote: got %s want %s", refs["refs/heads/main"], result.NewSHA)
	}
}

// ─── Mixed test ───────────────────────────────────────────────────────────────

func TestRunMixed_AllImmediate(t *testing.T) {
	sim := gitsim.New(gitsim.WithProviders("github"))
	srv := httptest.NewServer(sim.Handler())
	t.Cleanup(srv.Close)
	sim.SetBaseURL(srv.URL)

	repo, err := sim.CreateRepo("github", "acme", "mixed-test", "main")
	if err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}

	receiver, getArrivals := shaReceiver(t)

	result, err := scenario.RunMixed(context.Background(), repo, makeFireFunc(sim, repo), scenario.MixedSpec{
		Commits: []scenario.MixedCommit{
			{HeadDelayMs: 0, SendAfterMs: 0},
			{HeadDelayMs: 0, SendAfterMs: 0},
			{HeadDelayMs: 0, SendAfterMs: 0},
		},
		Target: receiver.URL,
	})
	if err != nil {
		t.Fatalf("RunMixed: %v", err)
	}

	if len(result.SHAs) != 3 {
		t.Fatalf("SHAs: got %d want 3", len(result.SHAs))
	}
	for i, r := range result.WebhookResults {
		if r.StatusCode != http.StatusOK {
			t.Errorf("commit %d: webhook status %d", i, r.StatusCode)
		}
	}

	arrivals := getArrivals()
	if len(arrivals) != 3 {
		t.Fatalf("arrivals: got %d want 3", len(arrivals))
	}
	// With all sendAfter=0, commits are delivered in creation order.
	for i, sha := range result.SHAs {
		if arrivals[i] != sha {
			t.Errorf("arrival[%d]: got %q want %q", i, arrivals[i], sha)
		}
	}
}

// TestRunMixed_RaceOverlap reproduces the overlap scenario: commit B's webhook
// arrives before commit A's HEAD has been promoted (both HEAD delays are controlled
// by a ManualClock). SendAfterMs=0 for all so deliveries happen sequentially.
func TestRunMixed_RaceOverlap(t *testing.T) {
	const delay = 500 * time.Millisecond
	t0 := time.Unix(2_000_000, 0)
	clock := core.NewManualClock(t0)

	sim := gitsim.New(gitsim.WithClock(clock), gitsim.WithProviders("github"))
	srv := httptest.NewServer(sim.Handler())
	t.Cleanup(srv.Close)
	sim.SetBaseURL(srv.URL)

	repo, err := sim.CreateRepo("github", "acme", "overlap-test", "main")
	if err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}

	receiver, _ := shaReceiver(t)

	// Two commits, both with HEAD delays. Webhooks are fired immediately
	// (sendAfterMs=0), so Fleet would receive both SHAs while their HEADs are lagging.
	result, err := scenario.RunMixed(context.Background(), repo, makeFireFunc(sim, repo), scenario.MixedSpec{
		Commits: []scenario.MixedCommit{
			{HeadDelayMs: delay.Milliseconds(), SendAfterMs: 0},
			{HeadDelayMs: delay.Milliseconds(), SendAfterMs: 0},
		},
		Target: receiver.URL,
	})
	if err != nil {
		t.Fatalf("RunMixed: %v", err)
	}

	// Before advancing the clock, the branch tip must still be the initial commit
	// (both commits are pending).
	initialSHA, _ := repo.VisibleCommit("main")
	if initialSHA == result.SHAs[0] || initialSHA == result.SHAs[1] {
		t.Errorf("expected HEAD to lag; got visible %q", initialSHA)
	}

	// After advancing the clock, both commits promote and HEAD becomes the last SHA.
	clock.Advance(delay + time.Millisecond)
	promoted, _ := repo.VisibleCommit("main")
	if promoted != result.SHAs[1] {
		t.Errorf("after promotion: HEAD got %q want %q", promoted, result.SHAs[1])
	}
}

// ─── HTTP control API: scenario endpoints ─────────────────────────────────────

func TestHTTP_ScenarioRace(t *testing.T) {
	t0 := time.Unix(3_000_000, 0)
	clock := core.NewManualClock(t0)

	sim := gitsim.New(gitsim.WithClock(clock), gitsim.WithProviders("github"))
	srv := httptest.NewServer(sim.Handler())
	t.Cleanup(srv.Close)
	sim.SetBaseURL(srv.URL)

	// Create a repo via the control API.
	createBody, _ := json.Marshal(map[string]any{
		"vendor": "github", "owner": "acme", "repo": "http-race", "defaultBranch": "main",
	})
	resp, err := http.Post(srv.URL+"/control/repos", "application/json",
		mustReader(createBody))
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("create repo: status %d, err %v", resp.StatusCode, err)
	}
	resp.Body.Close()

	// Webhook receiver.
	var mu sync.Mutex
	var receivedSHA string
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var pl struct {
			After string `json:"after"`
		}
		json.NewDecoder(r.Body).Decode(&pl) //nolint:errcheck
		mu.Lock()
		receivedSHA = pl.After
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(receiver.Close)

	// Run the race scenario via HTTP.
	specBody, _ := json.Marshal(map[string]any{
		"repoID":      "github/acme/http-race",
		"headDelayMs": 200,
		"target":      receiver.URL,
	})
	resp2, err := http.Post(srv.URL+"/control/scenarios/race", "application/json",
		mustReader(specBody))
	if err != nil {
		t.Fatalf("POST /control/scenarios/race: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("race scenario status %d", resp2.StatusCode)
	}

	var raceResult scenario.RaceResult
	if err := json.NewDecoder(resp2.Body).Decode(&raceResult); err != nil {
		t.Fatalf("decode result: %v", err)
	}
	if raceResult.OldSHA == raceResult.NewSHA {
		t.Fatal("OldSHA == NewSHA")
	}

	mu.Lock()
	got := receivedSHA
	mu.Unlock()
	if got != raceResult.NewSHA {
		t.Errorf("webhook SHA: got %q want %q", got, raceResult.NewSHA)
	}
}

func mustReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }
