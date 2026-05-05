# Writing E2E Tests

This guide explains how to write E2E tests for HyperFleet.

## Before You Start

### 1. Understand the Test Structure

Tests are organized by resource type:
```text
e2e/
├── e2e.go              # Test suite registration
├── cluster/
│   └── creation.go     # Cluster lifecycle tests
└── nodepool/
    └── creation.go     # NodePool lifecycle tests
```

### 2. Read Existing Tests

Start by reading existing tests to understand the patterns:
- [`e2e/cluster/creation.go`](../e2e/cluster/creation.go) - Cluster creation example
- [`e2e/nodepool/creation.go`](../e2e/nodepool/creation.go) - NodePool creation example

### 3. Prepare Test Data

Test payloads are stored in `testdata/payloads/`:
```
testdata/payloads/
├── clusters/
│   └── cluster-request.json        # resource cluster payload
└── nodepools/
    └── nodepool-request.json        # resource nodepool payload
```

#### Payload Templates

Payload files support Go template syntax for dynamic values. This prevents naming conflicts when running tests multiple times in long-running environments.

**Example** (`testdata/payloads/clusters/cluster-request.json`):
```json
{
  "kind": "Cluster",
  "name": "hp-cluster-{{.Random}}",
  "labels": {
    "environment": "production",
    "created-at": "{{.Timestamp}}"
  },
  "spec": { ... }
}
```

Each time the payload is loaded, template variables are replaced with fresh values, ensuring unique resource names. See `pkg/client/payload.go` for available template variables.

## Test File Format

### File Naming Convention

- **File extension**: Use `.go` (NOT `_test.go`)
- **File name**: Descriptive, e.g., `creation.go`, `lifecycle.go`
- **Location**: Under `e2e/{resource-type}/`

### Basic Test Structure

```go
package cluster

import (
    "context"

    "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
    "github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
    "github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var testName = "[Suite: cluster] Create Cluster via API"

var _ = ginkgo.Describe(testName,
    ginkgo.Label(labels.Tier0),
    func() {
        var h *helper.Helper
        var clusterID string

        ginkgo.BeforeEach(func() {
            h = helper.New()
        })

        ginkgo.It("should create cluster successfully", func(ctx context.Context) {
            ginkgo.By("submitting cluster creation request")
            cluster, err := h.Client.CreateClusterFromPayload(ctx, "testdata/payloads/clusters/cluster-request.json")
            Expect(err).NotTo(HaveOccurred())
            clusterID = *cluster.Id

            ginkgo.By("waiting for cluster to become Reconciled")
            err = h.WaitForClusterCondition(ctx, clusterID, client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue, h.Cfg.Timeouts.Cluster.Reconciled)
            Expect(err).NotTo(HaveOccurred())
        })

        ginkgo.AfterEach(func(ctx context.Context) {
            if h == nil || clusterID == "" {
                return
            }
            if err := h.CleanupTestCluster(ctx, clusterID); err != nil {
                ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup cluster %s: %v\n", clusterID, err)
            }
        })
    },
)
```

## Required Elements

### 1. Test Name

```go
var lifecycleTestName = "[Suite: cluster] Full Cluster Creation Flow"
```

- Format: `[Suite: component] Description`
- Suite represents the HyperFleet component being tested (cluster, nodepool, api, adapter, etc.)
- Use clear, descriptive names

### 2. Labels

All tests must use labels for categorization. See `pkg/labels/labels.go` for complete definitions.

**Required labels (1)**:
- **Severity**: `Tier0` | `Tier1` | `Tier2`

**Optional labels**:
- **Scenario**: `Negative` | `Performance`
- **Functionality**: `Upgrade`
- **Constraint**: `Disruptive` | `Slow`

**Example**:

```go
import "github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"

var testName = "[Suite: cluster] Full Cluster Creation Flow"
var _ = ginkgo.Describe(testName,
    ginkgo.Label(labels.Tier0),
    func() { ... }
)
```

**Example with optional labels**:

```go
// Negative test case with slow execution
var _ = ginkgo.Describe(testName,
    ginkgo.Label(labels.Tier1, labels.Negative, labels.Slow),
    func() { ... }
)
```

### 3. BeforeEach Setup

```go
ginkgo.BeforeEach(func() {
    h = helper.New()
})
```

- Create Helper instance (automatically loads configuration)
- Initialize test context

### 4. Test Steps with ginkgo.By

```go
ginkgo.By("submitting cluster creation request")
// ... perform action

ginkgo.By("waiting for cluster to become Reconciled")
// ... wait for condition

ginkgo.By("verifying adapter conditions")
// ... verify conditions
```

- Use `ginkgo.By()` to mark major test steps
- Makes test output readable
- **DO NOT** use `ginkgo.By()` inside `Eventually` closures

### 5. AfterEach Cleanup

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

- Clean up resources after test
- Skip cleanup if helper not initialized or no cluster created
- Log cleanup failures as warnings

## Writing Assertions

### Use Gomega Matchers

```go
// Basic assertions
Expect(err).NotTo(HaveOccurred())
Expect(cluster.ID).NotTo(BeEmpty())
Expect(h.HasResourceCondition(cluster.Status.Conditions, client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue)).To(BeTrue())

// Eventually for async operations
Eventually(func(g Gomega) {
    cluster, err := h.Client.GetCluster(ctx, clusterID)
    g.Expect(err).NotTo(HaveOccurred())
    g.Expect(h.HasResourceCondition(cluster.Status.Conditions, client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue)).To(BeTrue())
}, h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).Should(Succeed())
```

**Important**: Inside `Eventually` closures, use `g.Expect()` instead of `Expect()`

## Using Helper Functions

### Wait for Cluster Reconciled

```go
err = h.WaitForClusterCondition(ctx, clusterID, client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue, h.Cfg.Timeouts.Cluster.Reconciled)
Expect(err).NotTo(HaveOccurred())
```

### Check Adapter Conditions

```go
statuses, err := h.Client.GetClusterStatuses(ctx, clusterID)
Expect(err).NotTo(HaveOccurred())

for _, adapter := range statuses.Items {
    hasApplied := h.HasCondition(adapter.Conditions, client.ConditionTypeApplied, openapi.True)
    Expect(hasApplied).To(BeTrue())
}
```

## Best Practices

### DO ✅

- Use descriptive test names and labels
- Mark major steps with `ginkgo.By()`
- Use `Eventually` for async operations
- Clean up resources in `AfterEach`
- Use timeout values from config
- Store resource IDs for cleanup
- Use helper functions when available

### DON'T ❌

- Don't use `_test.go` suffix (use `.go`)
- Don't use `ginkgo.By()` inside `Eventually` closures
- Don't hardcode timeouts (use config values)
- Don't skip cleanup (unless debugging)
- Don't ignore errors

## Adding New Tests

### 1. Create Test File

```bash
# For cluster tests
touch e2e/cluster/my-new-test.go

# For nodepool tests
touch e2e/nodepool/my-new-test.go
```

### 2. Follow the Template

Copy from existing tests and modify:
- Change test name and ID
- Update labels
- Implement test logic
- Add cleanup

### 3. Register Test (Automatic)

Tests are automatically registered via the package import in `e2e/e2e.go`:

```go
package e2e

import (
    _ "github.com/openshift-hyperfleet/hyperfleet-e2e/e2e/cluster"
    _ "github.com/openshift-hyperfleet/hyperfleet-e2e/e2e/nodepool"
)
```

No need to manually register tests.

### 4. Run Your Test

```bash
# Run all cluster tests
make build
./bin/hyperfleet-e2e test --focus "\[Suite: cluster\]"

# Run specific test by description
./bin/hyperfleet-e2e test --focus "Create Cluster via API"

# Or run by label
./bin/hyperfleet-e2e test --label-filter "critical && lifecycle"
```

## Common Patterns

### Create Resource from Payload

```go
cluster, err := h.Client.CreateClusterFromPayload(ctx, "testdata/payloads/clusters/cluster-request.json")
Expect(err).NotTo(HaveOccurred())
```

### Wait for Condition Transition

```go
Eventually(func(g Gomega) {
    cluster, err := h.Client.GetCluster(ctx, clusterID)
    g.Expect(err).NotTo(HaveOccurred())
    g.Expect(h.HasResourceCondition(cluster.Status.Conditions, client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue)).To(BeTrue())
}, timeout, pollInterval).Should(Succeed())
```

### Verify All Adapter Conditions

```go
statuses, err := h.Client.GetClusterStatuses(ctx, clusterID)
Expect(err).NotTo(HaveOccurred())

for _, adapter := range statuses.Items {
    adapterName := adapter.Adapter
    ginkgo.By(fmt.Sprintf("verifying adapter %s conditions", adapterName))

    hasApplied := h.HasCondition(adapter.Conditions, client.ConditionTypeApplied, openapi.True)
    Expect(hasApplied).To(BeTrue(), "adapter %s should have Applied=True", adapterName)

    hasAvailable := h.HasCondition(adapter.Conditions, client.ConditionTypeAvailable, openapi.True)
    Expect(hasAvailable).To(BeTrue(), "adapter %s should have Available=True", adapterName)
}
```

## Next Steps

- **Architecture**: Understand the framework design in [Architecture](architecture.md)
- **Configuration**: Customize behavior in [Configuration Reference](config.md)
- **Debug Tests**: Learn debugging techniques in [Troubleshooting Guide](troubleshooting.md)
- **CLI Reference**: Full command documentation in [CLI Reference](cli-reference.md)
