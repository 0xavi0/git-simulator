# Using git-simulator with Fleet

This document explains how to wire the simulator to a running Fleet instance so
you can exercise webhook and polling code paths against real Fleet controllers.

> **The simulator is a host binary, not a Kubernetes workload.** Fleet may run
> inside k3d/k8s, but it connects to the simulator as an ordinary HTTP endpoint.

---

## 1. Build and start the simulator

```bash
go build -o gitsim ./cmd/gitsim
./gitsim --listen :8080 --vendors github --git-base-url http://HOST_IP:8080
```

Replace `HOST_IP` with the IP address the k3d nodes can reach on your machine
(your LAN IP, not `127.0.0.1`).  Alternatively, on k3d the special hostname
`host.k3d.internal` resolves to the Docker host:

```bash
./gitsim --listen :8080 --vendors github \
         --git-base-url http://host.k3d.internal:8080
```

---

## 2. Create a simulated repo

```bash
REPO=$(curl -sX POST http://localhost:8080/control/repos \
  -H 'Content-Type: application/json' \
  -d '{"vendor":"github","owner":"acme","repo":"myapp","defaultBranch":"main"}')

CLONE_URL=$(echo "$REPO" | jq -r .cloneURL)   # http://host.k3d.internal:8080/acme/myapp.git
REPO_URL=$(echo  "$REPO" | jq -r .repoURL)    # https://host.k3d.internal/acme/myapp
```

---

## 3. Create a Fleet GitRepo pointing at the simulator

```yaml
apiVersion: fleet.cattle.io/v1alpha1
kind: GitRepo
metadata:
  name: sim-test
  namespace: fleet-local
spec:
  repo: <CLONE_URL>            # http://host.k3d.internal:8080/acme/myapp.git
  branch: main
  # For GitHub-style webhooks, configure the secret:
  # helmSecretName: sim-webhook-secret
```

For `repo`, use the `cloneURL` from step 2 — it is the git smart-HTTP URL Fleet's
git-job uses for `PlainClone`.

---

## 4. Wire up Fleet's webhook receiver

Fleet's git-job service exposes a webhook endpoint. Expose it on your host:

```bash
# Find the service port
kubectl -n cattle-fleet-system get svc

# Port-forward the webhook receiver to localhost
kubectl -n cattle-fleet-system port-forward svc/gitjob 9090:9090
```

The simulator will POST webhooks to this address. For the race scenario this means
the simulator must be able to reach `http://localhost:9090` (or wherever you
forward the port).

---

## 5. Run a scenario

### Option A — curl (black-box)

Push a commit and fire a race-scenario webhook at Fleet:

```bash
# Push a commit with a 500 ms HEAD lag
COMMIT=$(curl -sX POST http://localhost:8080/control/repos/github/acme/myapp/commits \
  -H 'Content-Type: application/json' \
  -d '{"branch":"main","headDelayMs":500}')
SHA=$(echo "$COMMIT" | jq -r .sha)

# Fire the webhook immediately (before HEAD promotes)
curl -sX POST http://localhost:8080/control/repos/github/acme/myapp/webhooks \
  -H 'Content-Type: application/json' \
  -d "{\"sha\":\"$SHA\",\"target\":\"http://localhost:9090\",\"secret\":\"fleet-secret\"}"
```

Observe `GitRepo.Status.WebhookCommit` in Fleet — it should show `SHA` before
`Status.Commit` does.

### Option B — full race scenario endpoint

```bash
curl -sX POST http://localhost:8080/control/scenarios/race \
  -H 'Content-Type: application/json' \
  -d '{
    "repoID":      "github/acme/myapp",
    "headDelayMs": 500,
    "target":      "http://localhost:9090",
    "secret":      "fleet-secret"
  }' | jq .
```

---

## 6. Networking summary

| Direction | What | How |
|-----------|------|-----|
| Cluster → simulator | clone / ls-remote / commits API | Bind simulator to LAN IP or use `host.k3d.internal` |
| Simulator → Fleet | POST webhook | `kubectl port-forward` the gitjob service |

Do **not** bind the simulator to `127.0.0.1` if Fleet runs inside k3d — the loopback
is not reachable from inside the Docker network.

---

## 7. Serving real manifests with `--content-dir`

By default the simulator serves a single stub `README.md` in every repo. Pass
`--content-dir` to serve a real folder from your filesystem instead:

```bash
./gitsim --listen :8080 \
         --vendors github \
         --git-base-url http://host.k3d.internal:8080 \
         --content-dir ./my-fleet-manifests
```

The directory is re-read on every `Push` / `AddCommit` call, so you can:

1. Start the simulator pointing at a folder of Kubernetes YAML or Helm charts.
2. Create a repo and let Fleet clone and process the initial commit.
3. Edit files in the folder on disk.
4. Push another commit — the new commit carries the updated tree; Fleet
   reconciles the diff exactly as it would against a real git host.

**What is excluded:**
- `.git/` and any nested `.git` file (submodule markers) are always skipped.
- Symlinks are silently skipped (a warning is logged).
- Empty directories are omitted (git does not track them).

**SDK equivalent:**

```go
import "github.com/rancher/gitsim/pkg/core"

sim := gitsim.New(
    gitsim.WithContent(core.NewLocalDirContent("/path/to/fleet-manifests")),
    gitsim.WithProviders("github"),
)
```

---

## 8. Verification checklist

- `git ls-remote http://host.k3d.internal:8080/acme/myapp.git HEAD` returns a SHA from your host shell.
- `curl http://host.k3d.internal:8080/healthz` returns `ok` from inside a k3d node (`kubectl exec`).
- After the race scenario, `kubectl get gitrepo sim-test -o jsonpath='{.status}'` shows `webhookCommit` advancing before `commit`.
