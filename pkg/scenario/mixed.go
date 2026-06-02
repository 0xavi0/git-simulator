package scenario

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// MixedCommit describes one commit in a Mixed scenario.
type MixedCommit struct {
	HeadDelayMs int64 `json:"headDelayMs,omitempty"`
	SendAfterMs int64 `json:"sendAfterMs,omitempty"`
}

// MixedSpec is the declarative description of the mixed/interleaved scenario.
//
// Each commit has its own HeadDelayMs (controls when the branch ref catches up)
// and SendAfterMs (controls when the webhook is delivered, relative to the start
// of the RunMixed call). This lets you reproduce overlapping races such as:
// commit B's webhook arrives while commit A's HEAD is still lagging.
type MixedSpec struct {
	// RepoID is only used by the HTTP control API; in-process callers may omit it.
	RepoID  string        `json:"repoID,omitempty"`
	Branch  string        `json:"branch,omitempty"`
	Commits []MixedCommit `json:"commits"`
	Secret  string        `json:"secret,omitempty"`
	Target  string        `json:"target"`
}

// MixedResult is the structured outcome of RunMixed.
type MixedResult struct {
	// SHAs are indexed to match MixedSpec.Commits (creation order).
	SHAs []string `json:"shas"`
	// WebhookResults are indexed to match MixedSpec.Commits (NOT delivery order).
	WebhookResults []WebhookResult `json:"webhookResults"`
}

// RunMixed creates each commit with its own HeadDelay, then delivers webhooks
// in SendAfterMs order. WebhookResults are indexed to match the input commits,
// not delivery order, so callers can assert per-commit receiver responses.
func RunMixed(ctx context.Context, repo Repo, fire FireFunc, spec MixedSpec) (*MixedResult, error) {
	if len(spec.Commits) == 0 {
		return nil, fmt.Errorf("scenario: Mixed requires at least one commit")
	}
	if spec.Target == "" {
		return nil, fmt.Errorf("scenario: Mixed requires a Target URL")
	}

	branch := spec.Branch
	if branch == "" {
		branch = repo.DefaultBranch()
	}

	n := len(spec.Commits)
	shas := make([]string, n)
	for i, c := range spec.Commits {
		sha, err := repo.PushCommit(branch, time.Duration(c.HeadDelayMs)*time.Millisecond)
		if err != nil {
			return nil, fmt.Errorf("scenario: push commit %d: %w", i, err)
		}
		shas[i] = sha
	}

	// Sort commits by SendAfterMs so webhooks arrive in the specified relative order.
	type job struct {
		origIdx int
		sha     string
		sendAt  time.Duration
	}
	jobs := make([]job, n)
	for i, c := range spec.Commits {
		jobs[i] = job{
			origIdx: i,
			sha:     shas[i],
			sendAt:  time.Duration(c.SendAfterMs) * time.Millisecond,
		}
	}
	sort.SliceStable(jobs, func(a, b int) bool {
		return jobs[a].sendAt < jobs[b].sendAt
	})

	results := make([]WebhookResult, n)
	start := time.Now()

	for _, j := range jobs {
		if j.sendAt > 0 {
			elapsed := time.Since(start)
			if remaining := j.sendAt - elapsed; remaining > 0 {
				select {
				case <-ctx.Done():
					return nil, ctx.Err()
				case <-time.After(remaining):
				}
			}
		}
		result, err := fire(j.sha, spec.Target, spec.Secret, 0)
		if err != nil {
			return nil, fmt.Errorf("scenario: webhook for commit %d: %w", j.origIdx, err)
		}
		results[j.origIdx] = result
	}

	return &MixedResult{
		SHAs:           shas,
		WebhookResults: results,
	}, nil
}
