# Architecture

HyperFleet E2E is a Ginkgo-based black-box testing framework for validating HyperFleet cluster lifecycle management.

## Design Principles

1. **Ephemeral resource management** - Tests create and cleanup temporary resources
2. **Configuration-driven execution** - Behavior controlled through config files, env vars, and CLI flags
3. **Structured logging** - All operations tracked with component, cluster ID, and error context
4. **OpenAPI client generation** - Type-safe API client auto-generated from HyperFleet OpenAPI spec

## Core Packages

```text
pkg/
├── api/                - OpenAPI generated client
├── client/             - HyperFleet API client wrapper
│   ├── kubernetes/     - Kubernetes client (client-go)
│   └── maestro/        - Maestro resource bundle client
├── config/             - Configuration loading and validation
├── e2e/                - Test execution engine (Ginkgo)
├── helper/             - Test helpers (pollers, matchers, resource management)
├── labels/             - Test label definitions
└── logger/             - Structured logging (slog)
```

## Resource Management

HyperFleet E2E creates ephemeral resources per test for complete isolation.

**Resource Lifecycle**: Per-test creation and cleanup
- Each test creates its own cluster via API
- Full isolation between tests
- Automatic cleanup after completion
- Supports parallel execution

**Workflow**:
```text
Test starts
  → Create new Helper instance (helper.New())
  → GetTestCluster() creates cluster via API
  → Poll for cluster Reconciled condition (pollers + matchers)
  → Execute test assertions
  → CleanupTestCluster() deletes cluster and namespaces
Test ends
```

**Example Configuration**:
```yaml
api:
  url: https://api.hyperfleet.example.com
timeouts:
  cluster:
    reconciled: 5m   # Default: 30m (see pkg/config/defaults.go)
```

### pkg/config

**Purpose**: Configuration loading, validation, and management

**Key Features**:
- Multi-source configuration loading (file, environment variables, CLI flags)
- Detailed validation with helpful error messages
- Configuration priority enforcement

**Configuration Priority Chain**:
```text
CLI Flags (highest priority)
  ↓
Environment Variables (HYPERFLEET_* prefix)
  ↓
Config File (configs/config.yaml)
  ↓
Built-in Defaults (lowest priority)
```

**Key Types**:
- `Config` - Top-level configuration struct
- `APIConfig`, `TimeoutsConfig`, `PollingConfig`, `LogConfig` - Nested configuration sections

**Key Functions**:
- `Load()` - Load and validate configuration
- `Validate()` - Validate configuration requirements
- `Display()` - Log merged configuration using structured logging

### pkg/client

**Purpose**: Wrapper around generated OpenAPI client with test-friendly methods

**Key Features**:
- Generic HTTP response handler
- Structured error handling
- Convenience methods for common operations
- Direct access to underlying OpenAPI client

**Key Types**:
- `HyperFleetClient` - Main client interface
- Wraps generated OpenAPI `Client` from `pkg/api/openapi`

**Key Methods**:

*Clusters*:
- `CreateCluster(ctx, req)` / `CreateClusterFromPayload(ctx, path)` - Create cluster
- `GetCluster(ctx, clusterID)` - Fetch cluster details
- `ListClusters(ctx)` - List all clusters
- `DeleteCluster(ctx, clusterID)` - Soft-delete cluster
- `PatchCluster(ctx, clusterID, req)` / `PatchClusterFromPayload(ctx, clusterID, path)` - Update cluster
- `GetClusterStatuses(ctx, clusterID)` - Fetch adapter statuses

*NodePools*:
- `CreateNodePool(ctx, clusterID, req)` / `CreateNodePoolFromPayload(ctx, clusterID, path)` - Create nodepool
- `GetNodePool(ctx, clusterID, npID)` - Fetch nodepool details
- `ListNodePools(ctx, clusterID)` - List nodepools for a cluster
- `DeleteNodePool(ctx, clusterID, npID)` - Soft-delete nodepool
- `PatchNodePool(ctx, clusterID, npID, req)` / `PatchNodePoolFromPayload(ctx, clusterID, npID, path)` - Update nodepool
- `GetNodePoolStatuses(ctx, clusterID, npID)` - Fetch adapter statuses

### pkg/helper

**Purpose**: Test helper utilities — resource management, pollers, matchers, K8s verification

**Key Features**:
- Per-test helper instance creation (`New()`)
- Resource lifecycle management (create, cleanup)
- Pollers for async assertions with `Eventually`
- Custom Gomega matchers for resource and adapter conditions
- Kubernetes resource verification (namespaces, deployments, jobs, configmaps)
- Adapter deployment/uninstall via Helm

**Key Types**:
- `Helper` - Main struct with `Cfg`, `Client`, `K8sClient`, `MaestroClient`

**Key Methods**:

*Resource Management* (`helper.go`):
- `GetTestCluster(ctx, payloadPath)` - Create temporary test cluster
- `CleanupTestCluster(ctx, clusterID)` - Delete cluster, Maestro bundles, and namespaces
- `GetTestNodePool(ctx, clusterID, payloadPath)` - Create nodepool

*Pollers* (`pollers.go`) — thin functions returning current state for use with `Eventually`:
- `PollCluster(ctx, id)` - Returns `(*Cluster, error)`
- `PollNodePool(ctx, clusterID, npID)` - Returns `(*NodePool, error)`
- `PollClusterAdapterStatuses(ctx, clusterID)` - Returns `(*AdapterStatusList, error)`
- `PollNodePoolAdapterStatuses(ctx, clusterID, npID)` - Returns `(*AdapterStatusList, error)`
- `PollClusterHTTPStatus(ctx, id)` - Returns HTTP status code (200/404)
- `PollNodePoolHTTPStatus(ctx, clusterID, npID)` - Returns HTTP status code (200/404)
- `PollNamespacesByPrefix(ctx, prefix)` - Returns `([]string, error)`

*Custom Matchers* (`matchers.go`) — reusable Gomega matchers:
- `HaveResourceCondition(condType, status)` - Matches `*Cluster` or `*NodePool` with given condition
- `HaveAllAdaptersWithCondition(adapters, condType, status)` - All required adapters have condition
- `HaveAllAdaptersAtGeneration(adapters, gen)` - All adapters at generation with Applied/Available/Health=True

*Condition Validation* (`validation.go`):
- `HasResourceCondition(conditions, condType, status)` - Synchronous condition check
- `HasAdapterCondition(conditions, condType, status)` - Synchronous adapter condition check
- `AllConditionsTrue(conditions, condTypes)` - All specified conditions are True
- `AdapterNameToConditionType(adapterName)` - Convert adapter name to condition type string

*Kubernetes Verification* (`k8s.go`):
- `VerifyNamespaceActive(ctx, name, labels, annotations)` - Namespace exists and Active
- `VerifyDeploymentAvailable(ctx, ns, labels, annotations)` - Deployment is Available
- `VerifyJobComplete(ctx, ns, labels, annotations)` - Job has completed
- `VerifyConfigMap(ctx, ns, labels, annotations)` - ConfigMap exists with expected metadata

*Adapter Operations* (`adapter.go`):
- `DeployAdapter(ctx, opts)` - Deploy adapter via Helm upgrade --install
- `UninstallAdapter(ctx, releaseName, namespace)` - Uninstall adapter via Helm

### pkg/logger

**Purpose**: Structured logging based on Go's `log/slog` package

**Key Features**:
- Structured logging with automatic fields (component, version, hostname)
- Context-aware methods for cluster and error logging
- Configurable output format (text, JSON)
- Configurable log level (debug, info, warn, error)

**Key Functions**:
- `Init(cfg, buildVersion)` - Initialize logger with configuration
- `InfoWithCluster(clusterID, msg, fields...)` - Log with cluster context
- `ErrorWithError(msg, err, fields...)` - Log with error details

**Automatic Fields**:
- `component` - Package/module name
- `version` - Framework version
- `hostname` - Execution host
- `cluster_id` - Cluster ID (when using InfoWithCluster)
- `error` - Error details (when using ErrorWithError)

### pkg/e2e

**Purpose**: Test execution engine and Ginkgo configuration

**Key Features**:
- Ginkgo suite configuration
- JUnit report generation
- Test filtering (labels, focus, skip)
- Suite timeout management

**Key Functions**:
- `RunTests(ctx)` - Main entry point for test execution
- Configures Ginkgo reporters, timeouts, and filters
- Handles suite-level setup and teardown

## Configuration Priority

Configuration values are applied in this priority order:

```text
┌──────────────────────────────────────────────────────────────────┐
│ 1. CLI Flags (highest priority)                                  │
│    Example: --api-url, --log-level                               │
└──────────────────────────────────────────────────────────────────┘
                              ↓
┌──────────────────────────────────────────────────────────────────┐
│ 2. Environment Variables                                         │
│    Example: HYPERFLEET_API_URL, HYPERFLEET_LOG_LEVEL             │
└──────────────────────────────────────────────────────────────────┘
                              ↓
┌──────────────────────────────────────────────────────────────────┐
│ 3. Config File                                                   │
│    Example: configs/config.yaml                                  │
└──────────────────────────────────────────────────────────────────┘
                              ↓
┌──────────────────────────────────────────────────────────────────┐
│ 4. Built-in Defaults (lowest priority)                           │
│    Example: timeout: 30m, poll: 10s                              │
└──────────────────────────────────────────────────────────────────┘
```

**Example**: If you set `HYPERFLEET_API_URL=https://env.example.com` and also pass `--api-url=https://flag.example.com`, the CLI flag value (`https://flag.example.com`) wins.

## Test Execution Flow

```text
CLI Invoked (hyperfleet-e2e test)
  ↓
┌─────────────────────────────────────┐
│ Load Configuration                  │
│ • Read config file                  │
│ • Apply environment variables       │
│ • Apply CLI flags                   │
│ • Validate configuration            │
└─────────────────────────────────────┘
  ↓
┌─────────────────────────────────────┐
│ Initialize Logger                   │
│ • Setup structured logging          │
│ • Display merged configuration      │
└─────────────────────────────────────┘
  ↓
┌─────────────────────────────────────┐
│ Configure Ginkgo                    │
│ • Apply label filters               │
│ • Setup JUnit reporter              │
│ • Configure timeouts                │
└─────────────────────────────────────┘
  ↓
┌─────────────────────────────────────┐
│ Run Test Suites                     │
│ • Discover all e2e/*/*.go            │
│ • Execute matched tests             │
│ • Collect results                   │
└─────────────────────────────────────┘
  ↓
┌─────────────────────────────────────┐
│ Generate Reports                    │
│ • JUnit XML (if configured)         │
│ • Console output                    │
│ • Exit code (0 = pass, 1 = fail)    │
└─────────────────────────────────────┘
```

## OpenAPI Integration

The framework uses code generation to maintain type-safe API clients.

### Client Generation

- **Source**: HyperFleet OpenAPI specification (YAML/JSON)
- **Generator**: oapi-codegen (Go client)
- **Output**: `pkg/api/openapi/` package
- **Regeneration**: `make generate` (when API spec changes)

### Generated Client Usage

The generated client is wrapped by `pkg/client.HyperFleetClient` to provide:
- Simplified error handling
- Test-friendly method signatures
- Request/response logging

**Example**:
```go
// Wrapped client (test-friendly) — used in tests
client, _ := client.NewHyperFleetClient(apiURL, nil)
cluster, err := client.GetCluster(ctx, clusterID)
```

### OpenAPI Spec Location

The OpenAPI specification is typically maintained in the main HyperFleet repository and referenced during code generation. Updates to the API require regenerating the client:

```bash
make generate
```

This ensures the test framework stays in sync with API changes.
