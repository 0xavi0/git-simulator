# git-simulator

A standalone testing binary that emulates git-hosting providers (GitHub,
GitLab, Bitbucket Cloud/Server, Gogs, Azure DevOps). It lets you exercise
git-webhook handlers and git pollers deterministically — including
hard-to-reproduce race conditions like "webhook arrives before the remote
HEAD updates".

Two usage modes:
- **In-process Go SDK** — fast, fully deterministic, no ports, clock-controlled.
- **Standalone binary** — drive it from any HTTP client (curl, integration tests
  in any language).

---

## Quickstart — Go SDK

```go
import (
    "net/http/httptest"
    "time"

    "github.com/rancher/gitsim/pkg/core"
    "github.com/rancher/gitsim/pkg/gitsim"
    _ "github.com/rancher/gitsim/pkg/provider/github"
)

func TestRace(t *testing.T) {
    clock := core.NewManualClock(time.Now())

    sim := gitsim.New(
        gitsim.WithClock(clock),
        gitsim.WithProviders("github"),
    )
    srv := httptest.NewServer(sim.Handler())
    t.Cleanup(srv.Close)
    sim.SetBaseURL(srv.URL)

    repo, _ := sim.CreateRepo("github", "acme", "myapp", "main")
    oldSHA, _ := repo.VisibleCommit("main")

    // Push a commit — HEAD stays at oldSHA for 300 ms.
    newSHA, _ := repo.Push(gitsim.OnBranch("main"), gitsim.HeadDelay(300*time.Millisecond))

    // Fire webhook immediately (race window is open).
    sim.Webhook(repo, newSHA,
        gitsim.Target("http://webhook-receiver:9090"),
        gitsim.SendAfter(0),
        gitsim.Secret("hook-secret"),
    )

    // ls-remote still returns oldSHA.
    // clock.Advance(301ms) → ls-remote returns newSHA.
}
```

See [`examples/sdk_race_test.go`](examples/sdk_race_test.go) for a fully runnable
annotated example.

---

## Quickstart — standalone binary

```bash
# Build
go build -o gitsim ./cmd/gitsim

# Start (binds to :8080, GitHub vendor enabled by default)
./gitsim --listen :8080 --vendors github --git-base-url http://localhost:8080

# Create a repo
curl -sX POST http://localhost:8080/control/repos \
  -H 'Content-Type: application/json' \
  -d '{"vendor":"github","owner":"acme","repo":"myapp","defaultBranch":"main"}'

# Push a commit
COMMIT=$(curl -sX POST http://localhost:8080/control/repos/github/acme/myapp/commits \
  -H 'Content-Type: application/json' \
  -d '{"branch":"main"}')
SHA=$(echo "$COMMIT" | python3 -c "import sys,json; print(json.load(sys.stdin)['sha'])")

# Fire a webhook
curl -sX POST http://localhost:8080/control/repos/github/acme/myapp/webhooks \
  -H 'Content-Type: application/json' \
  -d "{\"sha\":\"$SHA\",\"target\":\"http://localhost:9090\",\"secret\":\"hook-secret\"}"

# Run the race scenario in one call
curl -sX POST http://localhost:8080/control/scenarios/race \
  -H 'Content-Type: application/json' \
  -d '{"repoID":"github/acme/myapp","headDelayMs":300,"target":"http://localhost:9090"}'
```

See [`examples/curl_scenarios.sh`](examples/curl_scenarios.sh) for more examples.

---

## Build & test

```bash
make build    # go build ./...
make test     # go test ./...
make race     # go test -race ./...
make lint     # golangci-lint run
```

---

## Serving real manifests

By default every repo contains a single stub `README.md`. To clone real
Kubernetes YAML, Helm charts, or Kustomize overlays, point the simulator at a
host directory:

**Binary:**
```bash
./gitsim --content-dir ./my-manifests
```

**Go SDK:**
```go
import "github.com/rancher/gitsim/pkg/core"

sim := gitsim.New(
    gitsim.WithContent(core.NewLocalDirContent("./my-manifests")),
    gitsim.WithProviders("github"),
)
```

The directory is re-read on each `Push`, so editing files between two pushes
produces commits with distinct trees — enough for a real reconciliation loop on
the consumer side. `.git/` directories and symlinks are automatically excluded.

See [`docs/using-with-fleet.md`](docs/using-with-fleet.md#7-serving-real-manifests-with---content-dir) for an end-to-end example.

---

## Vendor matrix

| Vendor | Polling (ls-remote) | Polling (REST API) | Webhooks | Auth |
|--------|--------------------|--------------------|----------|------|
| GitHub | ✓ | ✓ `GET /repos/{o}/{r}/commits/{branch}` | ✓ | HMAC-SHA256 (`X-Hub-Signature-256`) |
| GitLab | ✓ | — | ✓ | Shared token (`X-Gitlab-Token`) |
| Bitbucket Cloud | ✓ | — | ✓ | UUID match (`X-Hook-UUID`) |
| Bitbucket Server | ✓ | — | ✓ | HMAC-SHA256 (`X-Hub-Signature`) |
| Gogs | ✓ | — | ✓ | HMAC-SHA256 (`X-Gogs-Signature`) |
| Azure DevOps | ✓ | — | ✓ | HTTP Basic auth |

Enable a vendor by blank-importing its package and naming it in `WithProviders`:

```go
import _ "github.com/rancher/gitsim/pkg/provider/gitlab"

sim := gitsim.New(gitsim.WithProviders("github", "gitlab"))
```

Or pass `--vendors github,gitlab` to the binary.

---

## Scenarios

| Scenario | What it tests |
|----------|---------------|
| `push-n` | N commits, webhook delivery in `asc` / `desc` / `shuffle` / `custom` order |
| `race` | Webhook for commit C arrives while `ls-remote` still reports the previous SHA |
| `mixed` | Per-commit HEAD delays and send-after timings for complex overlap patterns |

See [`docs/scenarios.md`](docs/scenarios.md) for knobs and examples.

---

## Example: wiring it to Fleet

[Fleet](https://fleet.rancher.io/) is one consumer that uses git polling and
webhooks. [`docs/using-with-fleet.md`](docs/using-with-fleet.md) walks through
the full setup as a worked example — binding the binary so k3d nodes can reach
it, creating a `GitRepo` that points at the simulator's clone URL, wiring the
webhook emitter to Fleet's receiver, and observing the race scenario take
effect. The same shape (bind, point a config object at the clone URL, wire the
webhook) works for any other consumer.

---

## Portability

The module is self-contained:

```bash
go build ./...          # must succeed
go test -race ./...     # must be green
```

- Module path: `github.com/rancher/gitsim` (change freely — nothing else imports it)
- No `replace` directives pointing at a parent project
- No Dockerfile, Helm chart, or Kubernetes manifests — pure HTTP binary

---

## Design constraints

- **No coupling to any specific consumer.** Consumers depend on the simulator; the simulator depends on no consumer.
- **Plain binary, no Kubernetes.** Runs as a single OS process over HTTP.
- **Real git smart-HTTP.** Backed by go-git; `PlainClone` and `ls-remote` work without a `git` binary.
- **Deterministic clock.** Inject `core.ManualClock` in tests; HEAD delays are controlled without real sleeps.
