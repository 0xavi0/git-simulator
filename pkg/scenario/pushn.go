package scenario

import (
	"context"
	"fmt"
	"math/rand"
	"time"
)

// PushNOrder defines the webhook delivery order for PushN.
type PushNOrder string

const (
	OrderAsc     PushNOrder = "asc"     // natural creation order (default)
	OrderDesc    PushNOrder = "desc"    // newest-first
	OrderShuffle PushNOrder = "shuffle" // random permutation
	OrderCustom  PushNOrder = "custom"  // explicit index list in CustomOrder
)

// PushNSpec is the declarative description of a push-N scenario.
type PushNSpec struct {
	// RepoID is only used by the HTTP control API to look up the repo.
	// In-process callers pass the Repo directly and may leave this empty.
	RepoID      string     `json:"repoID,omitempty"`
	Branch      string     `json:"branch,omitempty"`
	N           int        `json:"n"`
	Order       PushNOrder `json:"order,omitempty"`
	CustomOrder []int      `json:"customOrder,omitempty"`
	GapMs       int64      `json:"gapMs,omitempty"`
	Secret      string     `json:"secret,omitempty"`
	Target      string     `json:"target"`
}

// PushNResult is the structured outcome of RunPushN.
type PushNResult struct {
	// Commits holds the SHAs in creation order (index 0 = oldest).
	Commits []string `json:"commits"`
	// DeliveryOrder holds the indices into Commits in the actual delivery order.
	// e.g. [2,1,0] means newest-first.
	DeliveryOrder []int `json:"deliveryOrder"`
	// Responses are indexed to match DeliveryOrder.
	Responses []WebhookResult `json:"responses"`
}

// RunPushN creates N commits on the branch, then delivers their webhooks in the
// order specified by spec.Order. Results contain the SHAs, delivery order, and
// receiver responses so the caller can assert whatever Fleet behaviour is needed.
func RunPushN(ctx context.Context, repo Repo, fire FireFunc, spec PushNSpec) (*PushNResult, error) {
	if spec.N <= 0 {
		return nil, fmt.Errorf("scenario: PushN requires N > 0, got %d", spec.N)
	}
	if spec.Target == "" {
		return nil, fmt.Errorf("scenario: PushN requires a Target URL")
	}

	branch := spec.Branch
	if branch == "" {
		branch = repo.DefaultBranch()
	}

	shas := make([]string, spec.N)
	for i := range shas {
		sha, err := repo.PushCommit(branch, 0)
		if err != nil {
			return nil, fmt.Errorf("scenario: push commit %d: %w", i, err)
		}
		shas[i] = sha
	}

	order := resolveOrder(spec.N, spec.Order, spec.CustomOrder)

	responses := make([]WebhookResult, spec.N)
	for i, idx := range order {
		if i > 0 && spec.GapMs > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(spec.GapMs) * time.Millisecond):
			}
		}
		result, err := fire(shas[idx], spec.Target, spec.Secret, 0)
		if err != nil {
			return nil, fmt.Errorf("scenario: webhook for commit %d: %w", idx, err)
		}
		responses[i] = result
	}

	return &PushNResult{
		Commits:       shas,
		DeliveryOrder: order,
		Responses:     responses,
	}, nil
}

// resolveOrder returns the delivery indices for the given order spec.
func resolveOrder(n int, order PushNOrder, custom []int) []int {
	indices := make([]int, n)
	for i := range indices {
		indices[i] = i
	}
	switch order {
	case OrderDesc:
		for i, j := 0, n-1; i < j; i, j = i+1, j-1 {
			indices[i], indices[j] = indices[j], indices[i]
		}
	case OrderShuffle:
		rand.Shuffle(n, func(i, j int) { indices[i], indices[j] = indices[j], indices[i] })
	case OrderCustom:
		if len(custom) == n {
			cp := make([]int, n)
			copy(cp, custom)
			return cp
		}
	}
	return indices
}
