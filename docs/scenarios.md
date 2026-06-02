# Scenarios

The simulator ships three built-in scenarios for exercising Fleet's webhook and
polling code paths. Each scenario can be driven via the in-process Go SDK or the
HTTP control API.

---

## push-N

Push N commits to a branch and deliver their webhooks in a controlled order.
Useful for testing how Fleet handles out-of-order deliveries.

### Knobs

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `n` | `int` | — | Number of commits to create |
| `branch` | `string` | repo default | Branch to push to |
| `order` | `string` | `"asc"` | Delivery order: `asc`, `desc`, `shuffle`, `custom` |
| `customOrder` | `[]int` | — | Explicit index permutation when `order` is `custom` (0-based) |
| `gapMs` | `int64` | `0` | Milliseconds between successive webhook deliveries |
| `target` | `string` | — | URL to POST webhooks to |
| `secret` | `string` | `""` | HMAC/token signing secret |

### SDK example

```go
result, err := scenario.RunPushN(ctx, repo, fireFunc, scenario.PushNSpec{
    N:      5,
    Order:  scenario.OrderDesc,   // newest first
    Target: receiverURL,
    Secret: "s3cr3t",
})
// result.Commits       — ordered list of SHAs
// result.DeliveryOrder — index permutation applied
```

### HTTP control API

```bash
curl -X POST http://localhost:8080/control/scenarios/push-n \
  -H 'Content-Type: application/json' \
  -d '{
    "repoID": "github/acme/myapp",
    "n": 5,
    "order": "desc",
    "target": "http://localhost:9090",
    "secret": "fleet-secret"
  }'
```

---

## race — webhook-before-HEAD

The headline scenario for Fleet issue #4837.

Push a commit C with a configurable HEAD delay. The webhook for C is fired
immediately while the branch ref still advertises the previous SHA. This
reproduces the window where Fleet receives a webhook for C but `ls-remote`
or the vendor commits API keep returning the old SHA.

### Knobs

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `headDelayMs` | `int64` | — | How long (ms) the branch ref lags behind the new commit |
| `branch` | `string` | repo default | Branch to push to |
| `target` | `string` | — | URL to POST the webhook to |
| `secret` | `string` | `""` | HMAC/token signing secret |

### SDK example

```go
// Use a ManualClock for full determinism in tests.
clock := core.NewManualClock(time.Now())
sim := gitsim.New(gitsim.WithClock(clock), gitsim.WithProviders("github"))
// ...
result, err := scenario.RunRace(ctx, repo, fireFunc, scenario.RaceSpec{
    HeadDelayMs: 300,
    Target:      receiverURL,
})
// Before clock advance: ls-remote returns result.OldSHA
// After clock.Advance(300ms + 1ms): ls-remote returns result.NewSHA
```

### HTTP control API

```bash
curl -X POST http://localhost:8080/control/scenarios/race \
  -H 'Content-Type: application/json' \
  -d '{
    "repoID":      "github/acme/myapp",
    "headDelayMs": 300,
    "target":      "http://localhost:9090",
    "secret":      "fleet-secret"
  }'
# Response: {"oldSHA":"...","newSHA":"...","webhookResult":{...}}
```

---

## mixed

Fine-grained per-commit control over HEAD delays and webhook delivery timings.
Models complex race overlaps: commit A's HEAD is still lagging when commit B's
webhook arrives.

### Knobs

The top-level `MixedSpec` carries a `target` (and optional `secret`), plus a
`commits` slice where each entry has:

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `headDelayMs` | `int64` | `0` | HEAD lag for this commit |
| `sendAfterMs` | `int64` | `0` | Delay before firing this commit's webhook |

### SDK example

```go
result, err := scenario.RunMixed(ctx, repo, fireFunc, scenario.MixedSpec{
    Commits: []scenario.MixedCommit{
        {HeadDelayMs: 500, SendAfterMs: 0},   // commit A: big HEAD lag, webhook fires first
        {HeadDelayMs:   0, SendAfterMs: 0},   // commit B: immediate
        {HeadDelayMs: 200, SendAfterMs: 100}, // commit C: small lag, delayed webhook
    },
    Target: receiverURL,
})
```

### HTTP control API

```bash
curl -X POST http://localhost:8080/control/scenarios/mixed \
  -H 'Content-Type: application/json' \
  -d '{
    "repoID": "github/acme/myapp",
    "commits": [
      {"headDelayMs": 500, "sendAfterMs": 0},
      {"headDelayMs":   0, "sendAfterMs": 0},
      {"headDelayMs": 200, "sendAfterMs": 100}
    ],
    "target": "http://localhost:9090"
  }'
```

---

## Clock control

When using the Go SDK, inject a `core.ManualClock` to control time without real
sleeps — HEAD delays become purely deterministic:

```go
clock := core.NewManualClock(time.Unix(1_000_000, 0))
sim := gitsim.New(gitsim.WithClock(clock), gitsim.WithProviders("github"))
// ...

// After a Push with HeadDelay(300ms), the branch ref is still at the old SHA.
clock.Advance(300*time.Millisecond + time.Millisecond)
// Now ls-remote returns the new SHA.
```

When driving the simulator via the binary (HTTP control API), the real clock is
used and `headDelayMs` / `sendAfterMs` values are actual wall-clock delays.
