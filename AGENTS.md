# HyperFleet E2E — Agent Instructions

Black-box E2E testing framework for HyperFleet cluster lifecycle management. Tests hit the HyperFleet API, create ephemeral clusters, verify adapter execution and K8s resource creation, then clean up. Built with Go 1.25, Ginkgo v2, Gomega, and an OpenAPI-generated client.

Test suites: `e2e/cluster/`, `e2e/nodepool/`, `e2e/adapter/`.

## Verification

Run `make check` before declaring work done. It runs everything in order:

| Target | What it does |
|---|---|
| `make check` | `generate` → `fmt-check` → `vet` → `lint` → `test` (all-in-one) |
| `make build` | `generate` → compile binary to `bin/hyperfleet-e2e` |
| `make fmt` | Format code and imports (`golangci-lint fmt`) |
| `make test` | Unit tests only (`./pkg/...`) |
| `make lint` | `golangci-lint` (config: `.golangci.yml`) |
| `make generate` | Regenerate OpenAPI client from spec |

Pre-flight order: `make check` then `make build`.

## Source of Truth

| Topic | Location |
|---|---|
| Getting started | `docs/getting-started.md` |
| Architecture | `docs/architecture.md` |
| Test writing guide | `docs/development.md` |
| Debugging | `docs/debugging.md` |
| Local kind setup | `docs/local-kind-setup.md` |
| Runbook | `docs/runbook.md` |
| Contributing | `CONTRIBUTING.md` |
| Test case templates | `test-design/templates/` |
| Test case documents | `test-design/testcases/` |
| User journey maps | `test-design/user-journeys/` |
| Config defaults | `pkg/config/defaults.go` |
| Config struct & validation | `pkg/config/config.go` |
| Pollers | `pkg/helper/pollers.go` |
| Custom matchers | `pkg/helper/matchers.go` |
| Helper core (New, TestDataPath, CleanupTestCluster) | `pkg/helper/helper.go` + `pkg/helper/suite.go` |
| Synchronous validators (HasResourceCondition, AdapterNameToConditionType) | `pkg/helper/validation.go` |
| Payload template vars | `pkg/client/payload.go` (`templateVars` struct) |
| Labels | `pkg/labels/labels.go` |
| Condition type constants | `pkg/client/constants.go` |
| Config file | `configs/config.yaml` |

## Test Conventions

### File naming and structure

- **IMPORTANT:** Test files use `.go` extension, NOT `_test.go`. E2E tests are compiled into the binary, not run via `go test`.
- Location: `e2e/{suite}/descriptive-name.go` (package matches directory name)
- Test name format: `[Suite: component][category] Description` (e.g., `[Suite: cluster][baseline] Cluster Resource Type Lifecycle`). Known categories: `baseline`, `update`, `delete`, `concurrent`, `negative`.
- Test suites auto-register via blank import in `e2e/e2e.go`

### Labels

Every test MUST have exactly one severity label from `pkg/labels`:

- `labels.Tier0` — critical path, blocks release
- `labels.Tier1` — important features
- `labels.Tier2` — edge cases, can defer

Optional: `labels.Negative`, `labels.Performance`, `labels.Upgrade`, `labels.Disruptive`, `labels.Slow`

### Async operations — pollers + custom matchers

**IMPORTANT:** Use pollers with custom matchers. Do NOT create `WaitFor*` wrapper functions that hide `Eventually` inside helpers.

```go
// Wait for resource condition
Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
    Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))

// Wait for all adapters at generation
Eventually(h.PollClusterAdapterStatuses(ctx, clusterID), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
    Should(helper.HaveAllAdaptersAtGeneration(h.Cfg.Adapters.Cluster, expectedGen))

// Wait for hard-delete (404)
Eventually(h.PollClusterHTTPStatus(ctx, clusterID), timeout, h.Cfg.Polling.Interval).
    Should(Equal(http.StatusNotFound))

// Wait for namespace cleanup
Eventually(h.PollNamespacesByPrefix(ctx, clusterID), timeout, h.Cfg.Polling.Interval).
    Should(BeEmpty())
```

Available pollers: `PollCluster`, `PollNodePool`, `PollClusterAdapterStatuses`, `PollNodePoolAdapterStatuses`, `PollClusterHTTPStatus`, `PollNodePoolHTTPStatus`, `PollNamespacesByPrefix`.

Available matchers: `HaveResourceCondition`, `HaveAllAdaptersWithCondition`, `HaveAllAdaptersAtGeneration`.

For one-off complex assertions, use `Eventually(func(g Gomega) { ... }).Should(Succeed())` with `g.Expect()` (not bare `Expect()`).

### Cleanup

Every test MUST clean up resources with `ginkgo.DeferCleanup` inline right after resource creation.

### Payload templates

Resolve payload paths via `h.TestDataPath()` — never hardcode `testdata/` as a prefix (breaks when `TESTDATA_DIR` is overridden, e.g., in CI):

```go
h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
```

Payloads in `testdata/payloads/` support Go templates. Available variables (defined in `pkg/client/payload.go`):

- `.Random` — 8-char random hex
- `.UUID` — full UUID v4
- `.Timestamp` — Unix seconds
- `.TimestampMs` — Unix milliseconds

### Step markers

Use `ginkgo.By()` for major steps. **IMPORTANT:** Never use `ginkgo.By()` inside `Eventually` closures.

### Timeouts and intervals

Always use config values: `h.Cfg.Timeouts.Cluster.Reconciled`, `h.Cfg.Timeouts.NodePool.Reconciled`, `h.Cfg.Timeouts.Adapter.Processing`, `h.Cfg.Polling.Interval`. Never hardcode durations.

## Boundaries

### DON'T

- Use `_test.go` suffix for E2E test files
- Hardcode timeout durations — use `h.Cfg.Timeouts.*`
- Skip cleanup (`DeferCleanup`)
- Use `ginkgo.By()` inside `Eventually` closures
- Import `e2e/*` packages from `pkg/` code

## Gotchas

- `Validate()` in `pkg/config/config.go` returns `error`, does not panic — only checks that `API.URL` is non-empty
- `helper.New()` calls `log.Fatalf` if config is nil — tests must call `SetSuiteConfig` before running
- Config priority: CLI flags > env vars (`HYPERFLEET_*` prefix) > `configs/config.yaml` > built-in defaults (see `pkg/config/defaults.go`)
- Config file path priority: `--config` flag > `HYPERFLEET_CONFIG` env > `./configs/config.yaml` auto-detect
- Adapter names come from `h.Cfg.Adapters.Cluster` and `h.Cfg.Adapters.NodePool` at runtime — never hardcode adapter names. Values in `configs/config.yaml` (e.g., `cl-namespace`) override compiled defaults in `pkg/config/defaults.go` (e.g., `clusters-namespace`)
- `e2e-ci` Makefile target sets `TESTDATA_DIR` to absolute path and writes JUnit XML to `output/`
