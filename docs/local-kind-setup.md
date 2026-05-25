# Local E2E Testing with kind

Run E2E tests locally using [kind](https://kind.sigs.k8s.io/) and RabbitMQ — no GCP dependencies.

## Prerequisites

- **Go** 1.25+ — [go.dev](https://go.dev/doc/install)
- **Docker** — [docker.com](https://www.docker.com/) or **Podman** — [podman.io](https://podman.io/)
- **kind** — [kind.sigs.k8s.io](https://kind.sigs.k8s.io/docs/user/quick-start/#installation)
- **kubectl** 1.28+ — [kubernetes.io](https://kubernetes.io/docs/tasks/tools/)
- **helm** 3+ — [helm.sh](https://helm.sh/docs/intro/install/)

## Clone Repositories

All component repos are required — images are built locally.

```bash
for repo in hyperfleet-e2e hyperfleet-infra hyperfleet-api hyperfleet-sentinel hyperfleet-adapter; do
  git clone https://github.com/openshift-hyperfleet/${repo}.git ~/projects/${repo}
done
```

> Repos outside `~/projects`? Set `PROJECTS_DIR` in your `.env` — see [Configuration](#configuration).

## Quick Start

```bash
# Copy config template
cp deploy-scripts/.env.example deploy-scripts/.env
# Uncomment HYPERFLEET_API_URL and MAESTRO_URL at the bottom

# One command: cluster + images + deploy + port-forward
make local-up

# Run tests
make e2e
```

For individual steps:

```bash
./deploy-scripts/kind-local.sh setup        # Cluster + RabbitMQ + Maestro + images
./deploy-scripts/kind-local.sh deploy       # Deploy API + sentinels + adapters
./deploy-scripts/kind-local.sh port-forward # Forward API (:8000) + Maestro (:8100)
./deploy-scripts/kind-local.sh rebuild      # Rebuild all images + restart
./deploy-scripts/kind-local.sh down         # Remove everything
```

## Rebuilding After Code Changes

```bash
# Rebuild one component
./deploy-scripts/kind-local.sh rebuild hyperfleet-adapter

# Force rebuild without cache (after git pull)
./deploy-scripts/kind-local.sh rebuild --no-cache hyperfleet-adapter

# Rebuild everything
./deploy-scripts/kind-local.sh rebuild --no-cache
```

Or via Make:

```bash
make local-rebuild C=hyperfleet-adapter
make local-rebuild C=hyperfleet-adapter NO_CACHE=1
```

## Running Specific Tests

With `HYPERFLEET_API_URL` and `MAESTRO_URL` set in `.env`, just run:

```bash
./bin/hyperfleet-e2e test --focus="\[Suite: cluster\]" --log-level=info
```

## Configuration

All config lives in `deploy-scripts/.env` (gitignored). Copy from `.env.example` and uncomment what you need:

```bash
cp deploy-scripts/.env.example deploy-scripts/.env
```

Local kind settings are at the bottom of the file:

| Variable | Default | Description |
|----------|---------|-------------|
| `PROJECTS_DIR` | `~/projects` | Parent directory containing component repos |
| `INFRA_DIR` | `~/projects/hyperfleet-infra` | Path to hyperfleet-infra repo |
| `KIND_CLUSTER` | `kind` | Kind cluster name |
| `NAMESPACE` | `hyperfleet-local` | Kubernetes namespace |
| `HYPERFLEET_API_URL` | — | API URL for tests (`http://localhost:8000`) |
| `MAESTRO_URL` | — | Maestro URL for tests (`http://localhost:8100`) |

## Troubleshooting

**ImagePullBackOff** — Image not loaded into kind. Run `kind load docker-image <image>`. With Podman: `podman save <image> | kind load image-archive /dev/stdin`.

**db-migrate crashing** — API binary doesn't match Helm chart: `./deploy-scripts/kind-local.sh rebuild --no-cache hyperfleet-api`

**Container build cache stale** — Use `--no-cache` after `git pull`.

**Connection refused** — Port-forwards died: `./deploy-scripts/kind-local.sh port-forward`

**`make local-down`** removes components but leaves kind cluster. Full cleanup: `kind delete cluster`.
