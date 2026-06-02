package gitsim

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/rancher/gitsim/pkg/core"
	"github.com/rancher/gitsim/pkg/scenario"
)

// mountControlAPI registers the control-plane endpoints on mux.
func mountControlAPI(mux *http.ServeMux, s *Simulator) {
	mux.HandleFunc("POST /control/repos", s.handleCreateRepo)
	mux.HandleFunc("POST /control/repos/{vendor}/{owner}/{repo}/commits", s.handleAddCommit)
	mux.HandleFunc("POST /control/repos/{vendor}/{owner}/{repo}/webhooks", s.handleFireWebhook)

	mux.HandleFunc("POST /control/scenarios/push-n", s.handleScenarioPushN)
	mux.HandleFunc("POST /control/scenarios/race", s.handleScenarioRace)
	mux.HandleFunc("POST /control/scenarios/mixed", s.handleScenarioMixed)
}

// ─── POST /control/repos ─────────────────────────────────────────────────────

type createRepoReq struct {
	Vendor        string `json:"vendor"`
	Owner         string `json:"owner"`
	Repo          string `json:"repo"`
	DefaultBranch string `json:"defaultBranch"`
}

type createRepoResp struct {
	ID       string `json:"id"`
	RepoURL  string `json:"repoURL"`
	CloneURL string `json:"cloneURL"`
}

func (s *Simulator) handleCreateRepo(w http.ResponseWriter, r *http.Request) {
	var req createRepoReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if req.DefaultBranch == "" {
		req.DefaultBranch = "main"
	}

	sr, err := s.CreateRepo(req.Vendor, req.Owner, req.Repo, req.DefaultBranch)
	if err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, core.ErrAlreadyExists) {
			code = http.StatusConflict
		}
		http.Error(w, err.Error(), code)
		return
	}
	writeJSON(w, http.StatusCreated, createRepoResp{
		ID:       sr.ID(),
		RepoURL:  sr.RepoURL(),
		CloneURL: sr.CloneURL(),
	})
}

// ─── POST /control/repos/{vendor}/{owner}/{repo}/commits ─────────────────────

type addCommitReq struct {
	Branch      string `json:"branch"`
	HeadDelayMs int64  `json:"headDelayMs"`
}

type addCommitResp struct {
	SHA string `json:"sha"`
}

func (s *Simulator) handleAddCommit(w http.ResponseWriter, r *http.Request) {
	id := repoKey(r.PathValue("vendor"), r.PathValue("owner"), r.PathValue("repo"))
	sr, ok := s.getRepo(id)
	if !ok {
		http.Error(w, "repo not found: "+id, http.StatusNotFound)
		return
	}

	var req addCommitReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var opts []PushOption
	if req.Branch != "" {
		opts = append(opts, OnBranch(req.Branch))
	}
	if req.HeadDelayMs > 0 {
		opts = append(opts, HeadDelay(time.Duration(req.HeadDelayMs)*time.Millisecond))
	}

	sha, err := sr.Push(opts...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, addCommitResp{SHA: sha})
}

// ─── POST /control/repos/{vendor}/{owner}/{repo}/webhooks ────────────────────

type fireWebhookReq struct {
	SHA         string `json:"sha"`
	Target      string `json:"target"`
	SendAfterMs int64  `json:"sendAfterMs"`
	Secret      string `json:"secret"`
	Branch      string `json:"branch"`
}

type fireWebhookResp struct {
	StatusCode int    `json:"statusCode"`
	Body       string `json:"body"`
}

func (s *Simulator) handleFireWebhook(w http.ResponseWriter, r *http.Request) {
	id := repoKey(r.PathValue("vendor"), r.PathValue("owner"), r.PathValue("repo"))
	sr, ok := s.getRepo(id)
	if !ok {
		http.Error(w, "repo not found: "+id, http.StatusNotFound)
		return
	}

	var req fireWebhookReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	wopts := []WebhookOption{
		Target(req.Target),
		SendAfter(time.Duration(req.SendAfterMs) * time.Millisecond),
	}
	if req.Secret != "" {
		wopts = append(wopts, Secret(req.Secret))
	}
	if req.Branch != "" {
		wopts = append(wopts, WithBranch(req.Branch))
	}

	result, err := s.Webhook(sr, req.SHA, wopts...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, fireWebhookResp{
		StatusCode: result.StatusCode,
		Body:       string(result.Body),
	})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v) //nolint:errcheck
}

// ─── Scenario helpers ─────────────────────────────────────────────────────────

// makeFireFunc returns a scenario.FireFunc backed by this Simulator + repo.
func (s *Simulator) makeFireFunc(sr *SimRepo) scenario.FireFunc {
	return func(sha, target, secret string, sendAfter time.Duration) (scenario.WebhookResult, error) {
		opts := []WebhookOption{Target(target), SendAfter(sendAfter)}
		if secret != "" {
			opts = append(opts, Secret(secret))
		}
		result, err := s.Webhook(sr, sha, opts...)
		return scenario.WebhookResult{
			StatusCode: result.StatusCode,
			Body:       result.Body,
			Err:        result.Err,
		}, err
	}
}

// ─── POST /control/scenarios/push-n ─────────────────────────────────────────

func (s *Simulator) handleScenarioPushN(w http.ResponseWriter, r *http.Request) {
	var spec scenario.PushNSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	sr, ok := s.getRepo(spec.RepoID)
	if !ok {
		http.Error(w, "repo not found: "+spec.RepoID, http.StatusNotFound)
		return
	}
	result, err := scenario.RunPushN(r.Context(), sr, s.makeFireFunc(sr), spec)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// ─── POST /control/scenarios/race ────────────────────────────────────────────

func (s *Simulator) handleScenarioRace(w http.ResponseWriter, r *http.Request) {
	var spec scenario.RaceSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	sr, ok := s.getRepo(spec.RepoID)
	if !ok {
		http.Error(w, "repo not found: "+spec.RepoID, http.StatusNotFound)
		return
	}
	result, err := scenario.RunRace(r.Context(), sr, s.makeFireFunc(sr), spec)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// ─── POST /control/scenarios/mixed ───────────────────────────────────────────

func (s *Simulator) handleScenarioMixed(w http.ResponseWriter, r *http.Request) {
	var spec scenario.MixedSpec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	sr, ok := s.getRepo(spec.RepoID)
	if !ok {
		http.Error(w, "repo not found: "+spec.RepoID, http.StatusNotFound)
		return
	}
	result, err := scenario.RunMixed(r.Context(), sr, s.makeFireFunc(sr), spec)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
