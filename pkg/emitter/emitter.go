// Package emitter sends provider-built webhook requests to a target URL with
// explicit control over ordering, per-delivery delay, and optional retries.
package emitter

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rancher/gitsim/pkg/core"
)

// Emitter posts Provider-built webhook requests to a target URL.
type Emitter struct {
	client     *http.Client
	clock      core.Clock
	MaxRetries int           // retries after a transient failure (0 = try once)
	RetryWait  time.Duration // initial wait between retries; doubles on each attempt
}

// New creates an Emitter backed by the given clock.
func New(clock core.Clock) *Emitter {
	return &Emitter{
		client:    &http.Client{Timeout: 10 * time.Second},
		clock:     clock,
		RetryWait: 50 * time.Millisecond,
	}
}

// Delivery describes a single webhook to emit.
type Delivery struct {
	Provider core.Provider
	Event    core.PushEvent
	Secret   string
	SendAt   time.Duration // delay from batch start; 0 means send immediately
}

// Result is the outcome of one delivery attempt.
type Result struct {
	Delivery   Delivery
	StatusCode int
	Body       []byte
	Err        error
}

// Send posts d to target immediately and returns the result.
func (e *Emitter) Send(ctx context.Context, target string, d Delivery) (Result, error) {
	return e.sendWithRetry(ctx, target, d)
}

// SendBatch posts deliveries honouring each SendAt so that arrival order at
// target is determined by SendAt, not by slice position. Results are returned
// in the same order as the input slice. See schedule.go for ordering logic.
func (e *Emitter) SendBatch(ctx context.Context, target string, ds []Delivery) ([]Result, error) {
	return sendBatch(ctx, e, target, ds)
}

func (e *Emitter) sendWithRetry(ctx context.Context, target string, d Delivery) (Result, error) {
	header, body, err := d.Provider.BuildWebhook(d.Event, d.Secret)
	if err != nil {
		r := Result{Delivery: d, Err: fmt.Errorf("build webhook: %w", err)}
		return r, r.Err
	}

	wait := e.RetryWait
	var last Result
	for attempt := 0; attempt <= e.MaxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-e.clock.After(wait):
				wait *= 2
			case <-ctx.Done():
				r := Result{Delivery: d, Err: ctx.Err()}
				return r, r.Err
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, target, bytes.NewReader(body))
		if err != nil {
			r := Result{Delivery: d, Err: fmt.Errorf("new request: %w", err)}
			return r, r.Err
		}
		for k, vs := range header {
			for _, v := range vs {
				req.Header.Add(k, v)
			}
		}

		resp, err := e.client.Do(req)
		if err != nil {
			last = Result{Delivery: d, Err: err}
			continue
		}
		respBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()

		last = Result{Delivery: d, StatusCode: resp.StatusCode, Body: respBody}
		if resp.StatusCode < 500 {
			return last, nil
		}
	}

	return last, last.Err
}
