package scenario

import (
	"context"
	"fmt"
	"time"
)

// RaceSpec is the declarative description of the webhook-before-HEAD-updates race.
//
// Sequence:
//  1. Record visibleSHA (OldSHA).
//  2. Push a commit with HeadDelayMs so the branch ref stays at OldSHA for that long.
//  3. Fire the webhook for NewSHA immediately (SendAfter = 0).
//  4. Return OldSHA, NewSHA, and the receiver's response.
//
// The caller can then assert:
//   - The webhook payload contained NewSHA.
//   - An ls-remote right after RunRace still returns OldSHA (race window open).
//   - After advancing the clock past HeadDelayMs, ls-remote returns NewSHA.
type RaceSpec struct {
	// RepoID is only used by the HTTP control API; in-process callers may omit it.
	RepoID      string `json:"repoID,omitempty"`
	Branch      string `json:"branch,omitempty"`
	HeadDelayMs int64  `json:"headDelayMs"`
	Secret      string `json:"secret,omitempty"`
	Target      string `json:"target"`
}

// RaceResult is the structured outcome of RunRace.
type RaceResult struct {
	OldSHA        string        `json:"oldSHA"`
	NewSHA        string        `json:"newSHA"`
	WebhookResult WebhookResult `json:"webhookResult"`
}

// RunRace executes the "webhook arrives before HEAD updates" scenario.
// The returned RaceResult lets the caller assert that the webhook carried NewSHA
// while the git refs / vendor API still advertise OldSHA.
func RunRace(ctx context.Context, repo Repo, fire FireFunc, spec RaceSpec) (*RaceResult, error) {
	if spec.Target == "" {
		return nil, fmt.Errorf("scenario: Race requires a Target URL")
	}

	branch := spec.Branch
	if branch == "" {
		branch = repo.DefaultBranch()
	}

	oldSHA, ok := repo.VisibleCommit(branch)
	if !ok {
		return nil, fmt.Errorf("scenario: branch %q not found", branch)
	}

	delay := time.Duration(spec.HeadDelayMs) * time.Millisecond
	newSHA, err := repo.PushCommit(branch, delay)
	if err != nil {
		return nil, fmt.Errorf("scenario: push: %w", err)
	}

	// Fire webhook immediately — HEAD is still at oldSHA.
	result, err := fire(newSHA, spec.Target, spec.Secret, 0)
	if err != nil {
		return nil, fmt.Errorf("scenario: webhook: %w", err)
	}

	return &RaceResult{
		OldSHA:        oldSHA,
		NewSHA:        newSHA,
		WebhookResult: result,
	}, nil
}
