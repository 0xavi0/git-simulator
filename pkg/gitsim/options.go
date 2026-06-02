package gitsim

import (
	"fmt"
	"time"

	"github.com/rancher/gitsim/pkg/core"
	"github.com/rancher/gitsim/pkg/provider"
)

// ─── Simulator options ───────────────────────────────────────────────────────

type simConfig struct {
	clock     core.Clock
	content   core.ContentProvider
	providers []core.Provider
}

// Option configures a Simulator.
type Option func(*simConfig)

// WithClock injects a custom clock (e.g. ManualClock for deterministic tests).
func WithClock(c core.Clock) Option {
	return func(cfg *simConfig) { cfg.clock = c }
}

// WithContent sets the ContentProvider used for all repo commits.
// Default: a static README.md.
func WithContent(cp core.ContentProvider) Option {
	return func(cfg *simConfig) { cfg.content = cp }
}

// WithProviders enables the named git vendor providers.
// Each name must correspond to a provider registered via its package init(),
// e.g. _ "github.com/rancher/gitsim/pkg/provider/github".
// Panics if a name is not registered.
func WithProviders(names ...string) Option {
	return func(cfg *simConfig) {
		for _, name := range names {
			p, err := provider.Get(name)
			if err != nil {
				panic(fmt.Sprintf("gitsim: %v (import the provider package to register it)", err))
			}
			cfg.providers = append(cfg.providers, p)
		}
	}
}

// ─── Push options ────────────────────────────────────────────────────────────

type pushConfig struct {
	branch string
	delay  time.Duration
}

// PushOption modifies a SimRepo.Push call.
type PushOption func(*pushConfig)

// OnBranch sets the branch to commit to (default: repo's default branch).
func OnBranch(name string) PushOption {
	return func(cfg *pushConfig) { cfg.branch = name }
}

// HeadDelay defers visibility of the new commit by d.
// Fleet's ls-remote and commits API keep reporting the previous SHA until d elapses.
func HeadDelay(d time.Duration) PushOption {
	return func(cfg *pushConfig) { cfg.delay = d }
}

// ─── Webhook options ─────────────────────────────────────────────────────────

type webhookConfig struct {
	target    string
	sendAfter time.Duration
	secret    string
	branch    string // overrides the sha→branch map when set
}

// WebhookOption modifies a Simulator.Webhook call.
type WebhookOption func(*webhookConfig)

// Target sets the URL to POST the webhook to. Required.
func Target(url string) WebhookOption {
	return func(cfg *webhookConfig) { cfg.target = url }
}

// SendAfter delays the webhook delivery by d via the injected Clock.
func SendAfter(d time.Duration) WebhookOption {
	return func(cfg *webhookConfig) { cfg.sendAfter = d }
}

// Secret sets the HMAC signing secret embedded in the webhook headers.
func Secret(s string) WebhookOption {
	return func(cfg *webhookConfig) { cfg.secret = s }
}

// WithBranch explicitly sets the branch name in the push event.
// If omitted, the branch is inferred from which branch Push() was called on.
func WithBranch(name string) WebhookOption {
	return func(cfg *webhookConfig) { cfg.branch = name }
}
