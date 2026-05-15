# HyperFleet E2E - AI Agent Instructions

This file provides context for AI agents working with the HyperFleet E2E testing framework.

## Project Overview

Black-box E2E testing framework for HyperFleet cluster lifecycle management. Built with Go, Ginkgo, and OpenAPI-generated clients. Tests create ephemeral clusters for complete isolation.

## Build and Test Commands

### Build Binary

```bash
make build
```

Binary output: `bin/hyperfleet-e2e`

### Run All Checks

```bash
make check
```

This runs: format check, vet, lint, and unit tests.

### Individual Quality Checks

```bash
make fmt           # Format code
make fmt-check     # Verify formatting
make vet           # Run go vet
make lint          # Run golangci-lint
make test          # Run unit tests
```

### Generate API Client

Required after OpenAPI schema updates:

```bash
make generate
```

Extracts schema from `hyperfleet-api-spec` Go module via `hack/extract-schema/` (uses `embed.FS`) and regenerates `pkg/api/openapi/`.

### Run E2E Tests

```bash
export HYPERFLEET_API_URL=https://api.hyperfleet.example.com
make build
./bin/hyperfleet-e2e test --label-filter=tier0
```

### Clean Build Artifacts

```bash
make clean
```

## Validation Checklist

Before submitting changes, verify:

1. **Format**: `make fmt`
2. **Generate**: `make generate` (if OpenAPI schema or config changed)
3. **Lint**: `make lint` (must pass with zero errors)
4. **Vet**: `make vet` (must pass)
5. **Unit Tests**: `make test` (all tests must pass)
6. **Build**: `make build` (binary must compile)
7. **E2E Tests**: Optional, but recommended for test changes

## Code Conventions

### Test Files

- **Extension**: Use `.go` NOT `_test.go`
- **Location**: `e2e/{resource-type}/descriptive-name.go`
- **Package**: Match directory name (e.g., package `cluster` for `e2e/cluster/`)
- **Test Name**: Format as `[Suite: component] Description` (e.g., `[Suite: cluster] Create Cluster via API`)

Example:
```go
package cluster

var testName = "[Suite: cluster] Create Cluster via API"
```

### Labels

Every test MUST have exactly one severity label:

```go
import "github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"

var _ = ginkgo.Describe(testName,
    ginkgo.Label(labels.Tier0),  // Critical severity
    func() { ... }
)
```

Available labels:
- **Severity** (required): `Tier0`, `Tier1`, `Tier2`
- **Optional**: `Negative`, `Performance`, `Upgrade`, `Disruptive`, `Slow`

### Test Structure

Required structure:

```go
var _ = ginkgo.Describe(testName, ginkgo.Label(...), func() {
    var h *helper.Helper
    var resourceID string

    ginkgo.BeforeEach(func() {
        h = helper.New()
    })

    ginkgo.It("description", func(ctx context.Context) {
        ginkgo.By("step description")
        // test logic
    })

    ginkgo.AfterEach(func(ctx context.Context) {
        if h == nil || resourceID == "" {
            return
        }
        if err := h.CleanupTestCluster(ctx, resourceID); err != nil {
            ginkgo.GinkgoWriter.Printf("Warning: cleanup failed: %v\n", err)
        }
    })
})
```

### Step Markers

Use `ginkgo.By()` for major steps ONLY. Do NOT use inside `Eventually` closures:

```go
// CORRECT
ginkgo.By("waiting for cluster to become Reconciled")
Eventually(h.PollCluster(ctx, clusterID), timeout, h.Cfg.Polling.Interval).
    Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))

// INCORRECT - never do this
Eventually(func() {
    ginkgo.By("checking status")  // ❌ Wrong
    // ...
}).Should(Succeed())
```

### Async Operations — Pollers + Custom Matchers

Use **pollers** (thin functions returning current state) with **custom matchers** (reusable assertions). This keeps `Eventually` visible at the call site and avoids combinatorial helper function explosion.

**Wait for a resource condition** (cluster or nodepool):
```go
Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
    Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))

Eventually(h.PollNodePool(ctx, clusterID, npID), h.Cfg.Timeouts.NodePool.Reconciled, h.Cfg.Polling.Interval).
    Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))
```

**Wait for adapter conditions** (works for both cluster and nodepool adapters):
```go
Eventually(h.PollClusterAdapterStatuses(ctx, clusterID), timeout, h.Cfg.Polling.Interval).
    Should(helper.HaveAllAdaptersWithCondition(h.Cfg.Adapters.Cluster, client.ConditionTypeFinalized, openapi.AdapterConditionStatusTrue))

Eventually(h.PollNodePoolAdapterStatuses(ctx, clusterID, npID), timeout, h.Cfg.Polling.Interval).
    Should(helper.HaveAllAdaptersAtGeneration(h.Cfg.Adapters.NodePool, expectedGen))
```

**Wait for hard-delete** (resource returns 404):
```go
Eventually(h.PollClusterHTTPStatus(ctx, clusterID), timeout, h.Cfg.Polling.Interval).
    Should(Equal(http.StatusNotFound))
```

**Wait for namespace cleanup**:
```go
Eventually(h.PollNamespacesByPrefix(ctx, clusterID), timeout, h.Cfg.Polling.Interval).
    Should(BeEmpty())
```

**For one-off complex assertions**, use `Eventually` with `func(g Gomega)` and `g.Expect()` (not `Expect()`):
```go
Eventually(func(g Gomega) {
    statuses, err := h.Client.GetClusterStatuses(ctx, clusterID)
    g.Expect(err).NotTo(HaveOccurred())
    // complex multi-field validation...
}, timeout, h.Cfg.Polling.Interval).Should(Succeed())
```

Available pollers: `PollCluster`, `PollNodePool`, `PollClusterAdapterStatuses`, `PollNodePoolAdapterStatuses`, `PollClusterHTTPStatus`, `PollNodePoolHTTPStatus`, `PollNamespacesByPrefix` — see `pkg/helper/pollers.go`.

Available matchers: `HaveResourceCondition`, `HaveAllAdaptersWithCondition`, `HaveAllAdaptersAtGeneration` — see `pkg/helper/matchers.go`.

**Do NOT** create `WaitFor*` wrapper functions that hide `Eventually` inside helpers.

### Resource Cleanup

ALWAYS implement cleanup in `AfterEach`:

```go
ginkgo.AfterEach(func(ctx context.Context) {
    if h == nil || clusterID == "" {
        return
    }
    if err := h.CleanupTestCluster(ctx, clusterID); err != nil {
        ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup cluster %s: %v\n", clusterID, err)
    }
})
```

### Payload Templates

Test payloads in `testdata/payloads/` support Go templates for dynamic values:

```json
{
  "name": "cluster-{{.Random}}",
  "labels": {
    "created-at": "{{.Timestamp}}"
  }
}
```

Available variables: `.Random`, `.Timestamp`. See `pkg/client/payload.go`.

## Boundary Statements

### DO NOT

- **Modify generated code**: Never edit files in `pkg/api/openapi/` - these are generated by `make generate`
- **Use `_test.go` suffix**: Test files must use `.go` extension
- **Hardcode timeouts**: Use `h.Cfg.Timeouts.*` values from config
- **Skip cleanup**: Always implement `AfterEach` cleanup
- **Commit without checks**: Always run `make check` before committing
- **Use `ginkgo.By()` in `Eventually`**: Only use at top-level test steps
- **Import test packages**: Do NOT import `e2e/*` packages in production code
- **Edit OpenAPI schema**: Schema is maintained in hyperfleet-api-spec repo
- **Create `WaitFor*` wrapper functions**: Use pollers + custom matchers instead (see Async Operations)

### DO

- **Use pollers + matchers**: Prefer `Eventually(h.PollCluster(...)).Should(helper.HaveResourceCondition(...))` over raw `Eventually` with inline closures
- **Use config values**: `h.Cfg.Timeouts.*` for timeouts, `h.Cfg.Polling.*` for intervals
- **Store resource IDs**: Save IDs in variables for cleanup
- **Check errors**: Use `Expect(err).NotTo(HaveOccurred())`
- **Use context**: All test functions receive `context.Context`

## Development Workflow

### Adding a New Test

1. Create file: `e2e/{suite}/descriptive-name.go`
2. Copy structure from existing test
3. Update test name, labels, and logic
4. Run validation checklist
5. Test locally before submitting PR

### Updating API Client

When hyperfleet-api-spec changes:

```bash
# Bump spec module version
go get github.com/openshift-hyperfleet/hyperfleet-api-spec@vX.Y.Z

# Regenerate client code
make generate

# Verify changes compile
make build
```

### Local Testing

```bash
# Build and run specific tests
make build
./bin/hyperfleet-e2e test --focus "\[Suite: cluster\]"

# Run critical tests only
./bin/hyperfleet-e2e test --label-filter=tier0

# Debug mode
./bin/hyperfleet-e2e test --log-level=debug
```

## Configuration

Priority (highest to lowest):
1. CLI flags: `--api-url`, `--log-level`
2. Environment variables: `HYPERFLEET_API_URL`
3. Config file: `configs/config.yaml`
4. Built-in defaults

## Common Patterns

### Create Resource

```go
cluster, err := h.Client.CreateClusterFromPayload(ctx, "testdata/payloads/clusters/cluster-request.json")
Expect(err).NotTo(HaveOccurred())
clusterID = *cluster.Id
```

### Wait for Condition

```go
Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
    Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))
```

### Wait for All Adapters

```go
Eventually(h.PollClusterAdapterStatuses(ctx, clusterID), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
    Should(helper.HaveAllAdaptersAtGeneration(h.Cfg.Adapters.Cluster, expectedGen))
```

### Verify Conditions (synchronous)

```go
hasReconciled := h.HasResourceCondition(cluster.Status.Conditions, client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue)
Expect(hasReconciled).To(BeTrue())
```

## Documentation

- [Getting Started](docs/getting-started.md) - Run first test in 10 minutes
- [Architecture](docs/architecture.md) - Framework design
- [Development](docs/development.md) - Detailed test writing guide
- [CONTRIBUTING.md](CONTRIBUTING.md) - Contribution guidelines
