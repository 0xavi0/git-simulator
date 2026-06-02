package emitter

import (
	"context"
	"sort"
)

// sendBatch sorts deliveries by SendAt and posts them in that order, waiting
// between each one using the injected Clock so arrival order at target equals
// SendAt order, not input-slice order.
//
// Results are indexed to match the input slice (results[i] ↔ ds[i]).
func sendBatch(ctx context.Context, e *Emitter, target string, ds []Delivery) ([]Result, error) {
	type entry struct {
		origIdx int
		d       Delivery
	}

	ordered := make([]entry, len(ds))
	for i, d := range ds {
		ordered[i] = entry{i, d}
	}
	// Stable sort preserves input order for equal SendAt values.
	sort.SliceStable(ordered, func(a, b int) bool {
		return ordered[a].d.SendAt < ordered[b].d.SendAt
	})

	start := e.clock.Now()
	results := make([]Result, len(ds))

	for _, ent := range ordered {
		if ent.d.SendAt > 0 {
			elapsed := e.clock.Now().Sub(start)
			if remaining := ent.d.SendAt - elapsed; remaining > 0 {
				select {
				case <-e.clock.After(remaining):
				case <-ctx.Done():
					return results, ctx.Err()
				}
			}
		}
		r, _ := e.sendWithRetry(ctx, target, ent.d)
		results[ent.origIdx] = r
	}
	return results, nil
}
