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
├── api/          - OpenAPI generated client
├── client/       - HyperFleet API client wrapper
├── config/       - Configuration loading and validation
├── e2e/          - Test execution engine (Ginkgo)
├── helper/       - Test helper utilities (waits, assertions)
├── labels/       - Test label definitions
└── logger/       - Structured logging (slog)
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
  → Create new Helper instance
  → GetTestCluster() creates cluster via API
  → Wait for cluster Reconciled condition
  → Execute test assertions
  → CleanupTestCluster() deletes cluster
Test ends
```

**Example Configuration**:
```yaml
api:
  url: https://api.hyperfleet.example.com
resources:
  keep: false
timeouts:
  cluster:
    reconciled: 5m
```

## Core Packages

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
- `GetCluster(ctx, clusterID)` - Fetch cluster details
- `CreateCluster(ctx, payload)` - Create new cluster
- `DeleteCluster(ctx, clusterID)` - Delete cluster
- `GetNodePool(ctx, clusterID, nodePoolID)` - Fetch nodepool details
- Similar methods for all HyperFleet resources

### pkg/helper

**Purpose**: Test helper utilities for resource management

**Key Features**:
- Resource lifecycle management (create, wait, cleanup)
- Condition polling and validation
- Per-test helper instance creation

**Key Types**:
- `Helper` - Main helper struct with resource management methods

**Key Methods**:

**Resource Management**:
- `GetTestCluster(ctx, payloadPath)` - Create temporary test cluster
- `CleanupTestCluster(ctx, clusterID)` - Delete test cluster
- `GetTestNodePool(ctx, clusterID, payloadPath)` - Create nodepool
- `CleanupTestNodePool(ctx, clusterID, nodePoolID)` - Delete nodepool

**Wait Operations**:
- `WaitForClusterCondition(ctx, clusterID, conditionType, expectedStatus, timeout)` - Poll until cluster condition matches
- `WaitForAllAdapterConditions(ctx, clusterID, conditions)` - Wait for adapter conditions

**Condition Validation**:
- `ValidateAdapterConditions(ctx, clusterID, expectedConditions)` - Check adapter status

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
- `RunTests(cfg)` - Main entry point for test execution
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
│    Example: timeout: 30m, poll: 5s                               │
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
│ • Discover all e2e/*_test.go        │
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
- Automatic retry logic (future)
- Request/response logging

**Example**:
```go
// Generated client (low-level)
apiClient := openapi.NewClient(...)
resp, httpResp, err := apiClient.ClustersAPI.GetCluster(ctx, clusterID).Execute()

// Wrapped client (test-friendly)
client := client.NewHyperFleetClient(apiURL)
cluster, err := client.GetCluster(ctx, clusterID)
```

### OpenAPI Spec Location

The OpenAPI specification is typically maintained in the main HyperFleet repository and referenced during code generation. Updates to the API require regenerating the client:

```bash
make generate
```

This ensures the test framework stays in sync with API changes.
