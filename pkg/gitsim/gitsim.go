// Package gitsim is the public SDK façade for the git-simulator.
// It wires together the store, git smart-HTTP server, vendor provider APIs,
// and webhook emitter into a single in-process test double.
//
// Typical test usage:
//
//	import _ "github.com/rancher/gitsim/pkg/provider/github"
//
//	sim := gitsim.New(gitsim.WithProviders("github"), gitsim.WithClock(clock))
//	srv := httptest.NewServer(sim.Handler())
//	t.Cleanup(srv.Close)
//	sim.SetBaseURL(srv.URL)
//
//	repo, _ := sim.CreateRepo("github", "acme", "app", "main")
//	sha, _  := repo.Push(gitsim.OnBranch("main"), gitsim.HeadDelay(300*time.Millisecond))
//	sim.Webhook(repo, sha, gitsim.Target(fleetURL), gitsim.Secret("s3cr3t"))
package gitsim

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/rancher/gitsim/pkg/core"
	"github.com/rancher/gitsim/pkg/emitter"
	"github.com/rancher/gitsim/pkg/gitserver"
	"github.com/rancher/gitsim/pkg/store"
)

// Simulator is the central coordinator: one store, one mux, one emitter.
type Simulator struct {
	cfg simConfig
	st  *store.Store
	em  *emitter.Emitter

	mu      sync.RWMutex
	baseURL string              // "http://host:port"
	host    string              // "host:port"  (parsed from baseURL)
	repos   map[string]*SimRepo // repoKey → SimRepo
}

// New creates a Simulator with the given options.
// Defaults: RealClock, static README.md content, no providers.
func New(opts ...Option) *Simulator {
	cfg := simConfig{
		clock:   core.RealClock{},
		content: core.NewStaticContent(map[string][]byte{"README.md": []byte("# gitsim")}),
	}
	for _, o := range opts {
		o(&cfg)
	}
	s := &Simulator{
		cfg:   cfg,
		repos: make(map[string]*SimRepo),
	}
	s.st = store.New(cfg.clock, cfg.content)
	s.em = emitter.New(cfg.clock)
	return s
}

// SetBaseURL records the simulator's own externally-reachable URL
// (e.g. the URL returned by httptest.NewServer). It must be called
// before CreateRepo so that repo URLs and the store host are consistent.
func (s *Simulator) SetBaseURL(rawURL string) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		panic(fmt.Sprintf("gitsim: invalid base URL %q: %v", rawURL, err))
	}
	s.mu.Lock()
	s.baseURL = strings.TrimSuffix(rawURL, "/")
	s.host = parsed.Host
	s.mu.Unlock()
}

// Handler returns an http.Handler that serves:
//   - Git smart-HTTP (/{owner}/{repo}.git/...)         — for ls-remote and clone
//   - Vendor REST APIs (e.g. /repos/{o}/{r}/commits)  — for Fleet's commits polling
//   - Control plane (/control/...)                     — for driving scenarios
//   - GET /healthz                                     — liveness probe
//
// Handler may be called before SetBaseURL. Git routing is dynamic:
// it uses the request's Host header to locate repos in the store.
func (s *Simulator) Handler() http.Handler {
	mux := http.NewServeMux()

	// Mount each enabled vendor's REST routes (e.g. GitHub commits API).
	for _, p := range s.cfg.providers {
		p.APIRoutes(mux, s.st)
	}

	// Control-plane API.
	mountControlAPI(mux, s)

	// Health check.
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, "ok")
	})

	// Git smart-HTTP catch-all: any path containing ".git" is a git request.
	// Using r.Host means repos registered with the server's actual host:port
	// are resolved correctly without needing a fixed host at handler creation.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, ".git") {
			gitserver.NewHandler(r.Host, s.st).ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
	})

	return mux
}

// CreateRepo registers a new simulated repository and returns a handle.
// SetBaseURL must have been called first.
func (s *Simulator) CreateRepo(vendorName, owner, repoName, defaultBranch string) (*SimRepo, error) {
	s.mu.RLock()
	host := s.host
	s.mu.RUnlock()

	if host == "" {
		return nil, fmt.Errorf("gitsim: SetBaseURL must be called before CreateRepo")
	}

	p, err := providerByName(s.cfg.providers, vendorName)
	if err != nil {
		return nil, err
	}

	repo, err := s.st.CreateRepo(host, owner, repoName, defaultBranch)
	if err != nil {
		return nil, err
	}

	sr := &SimRepo{
		sim:       s,
		repo:      repo,
		vendor:    vendorName,
		prov:      p,
		shaBranch: make(map[string]string),
		prevSHA:   make(map[string]string),
	}

	id := repoKey(vendorName, owner, repoName)
	s.mu.Lock()
	s.repos[id] = sr
	s.mu.Unlock()

	return sr, nil
}

// Webhook builds and posts a vendor-specific push webhook for sha.
// The Target option is required. SendAfter uses the simulator's clock for the delay.
func (s *Simulator) Webhook(repo *SimRepo, sha string, wopts ...WebhookOption) (emitter.Result, error) {
	cfg := &webhookConfig{}
	for _, o := range wopts {
		o(cfg)
	}
	if cfg.target == "" {
		return emitter.Result{}, fmt.Errorf("gitsim: Webhook requires the Target option")
	}

	event := repo.buildEvent(sha, cfg.branch)
	d := emitter.Delivery{
		Provider: repo.prov,
		Event:    event,
		Secret:   cfg.secret,
		SendAt:   cfg.sendAfter,
	}

	ctx := context.Background()
	if cfg.sendAfter > 0 {
		results, err := s.em.SendBatch(ctx, cfg.target, []emitter.Delivery{d})
		if err != nil {
			return emitter.Result{}, err
		}
		return results[0], nil
	}
	return s.em.Send(ctx, cfg.target, d)
}

func (s *Simulator) getRepo(id string) (*SimRepo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sr, ok := s.repos[id]
	return sr, ok
}

func providerByName(providers []core.Provider, name string) (core.Provider, error) {
	for _, p := range providers {
		if p.Name() == name {
			return p, nil
		}
	}
	return nil, fmt.Errorf("gitsim: provider %q not enabled (add it via WithProviders)", name)
}

func repoKey(vendor, owner, repo string) string {
	return vendor + "/" + owner + "/" + repo
}

// ─── SimRepo ─────────────────────────────────────────────────────────────────

// SimRepo is a handle to a single simulated repository.
type SimRepo struct {
	sim    *Simulator
	repo   *store.Repo
	vendor string
	prov   core.Provider

	mu        sync.Mutex
	shaBranch map[string]string // sha → branch (recorded by Push)
	prevSHA   map[string]string // branch → SHA before its last push
}

// ID returns a stable string identifier for use with the HTTP control API.
func (r *SimRepo) ID() string {
	_, owner, name := r.repo.Identity()
	return repoKey(r.vendor, owner, name)
}

// RepoURL returns the value to set as GitRepo.Spec.Repo when testing with Fleet.
func (r *SimRepo) RepoURL() string {
	host, owner, name := r.repo.Identity()
	return r.prov.RepoURL(host, owner, name)
}

// CloneURL returns the git smart-HTTP URL for cloning this repo.
func (r *SimRepo) CloneURL() string {
	r.sim.mu.RLock()
	base := r.sim.baseURL
	r.sim.mu.RUnlock()

	_, owner, name := r.repo.Identity()
	return base + "/" + owner + "/" + name + ".git"
}

// DefaultBranch returns the repo's default branch name.
func (r *SimRepo) DefaultBranch() string { return r.repo.DefaultBranch() }

// VisibleCommit returns the SHA currently visible on branch (applying any due HEAD promotions).
func (r *SimRepo) VisibleCommit(branch string) (string, bool) {
	return r.repo.VisibleCommit(branch)
}

// PushCommit adds a commit on branch with the given HEAD delay and returns the SHA.
// It satisfies the scenario.Repo interface.
func (r *SimRepo) PushCommit(branch string, delay time.Duration) (string, error) {
	return r.Push(OnBranch(branch), HeadDelay(delay))
}

// Push adds a commit on the given branch and returns its SHA.
// The new SHA's visibility is deferred by HeadDelay (default: immediate).
func (r *SimRepo) Push(opts ...PushOption) (string, error) {
	cfg := &pushConfig{branch: r.repo.DefaultBranch()}
	for _, o := range opts {
		o(cfg)
	}

	prevSHA, _ := r.repo.VisibleCommit(cfg.branch)

	sha, err := r.repo.AddCommit(cfg.branch, cfg.delay)
	if err != nil {
		return "", err
	}

	r.mu.Lock()
	r.shaBranch[sha] = cfg.branch
	r.prevSHA[cfg.branch] = prevSHA
	r.mu.Unlock()

	return sha, nil
}

// buildEvent constructs the PushEvent for a webhook delivery.
// branch overrides the sha→branch lookup when non-empty.
func (r *SimRepo) buildEvent(sha, branch string) core.PushEvent {
	host, owner, name := r.repo.Identity()

	r.mu.Lock()
	if branch == "" {
		branch = r.shaBranch[sha]
	}
	prev := r.prevSHA[branch]
	r.mu.Unlock()

	if branch == "" {
		branch = r.repo.DefaultBranch()
	}

	return core.PushEvent{
		Host:   host,
		Owner:  owner,
		Repo:   name,
		Branch: branch,
		Before: prev,
		After:  sha,
	}
}
