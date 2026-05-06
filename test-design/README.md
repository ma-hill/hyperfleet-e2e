# HyperFleet E2E Test Design

This directory contains test case specifications, user journey maps, and templates for the HyperFleet E2E testing framework.

## Contents

| Directory | Description |
|-----------|-------------|
| `testcases/` | Test case specifications organized by resource type |
| `templates/` | Templates for writing new test cases |
| `user-journeys/` | Critical user journey maps that drive test design |

## Tier Classification Guide

Every E2E test case must be assigned exactly one severity tier. The tier determines CI/CD gating behavior:

| Tier | CI/CD Gate | Response SLA |
|------|-----------|--------------|
| **Tier0** | Blocks release | Fix immediately |
| **Tier1** | Advisory (non-blocking) | Fix before next sprint ends |
| **Tier2** | Informational | Fix when capacity allows |

### Tier0 -- Critical

A Tier0 failure means **core user operations are broken**. Users cannot complete fundamental workflows.

#### Criteria (any one is sufficient)

- **Core lifecycle path**: The test validates a primary CRUD operation (create, read, update, delete) for a first-class resource (cluster, nodepool)
- **Data integrity**: Failure means resources are created incorrectly, K8s resources don't match API state, or data is lost
- **Platform guarantee**: The test validates a fundamental platform contract (adapter dependency ordering, generation-based idempotency, reconciliation convergence)
- **Blast radius**: Failure affects every user of the system, not just specific configurations

#### Decision questions

- "If this test fails in production, can users still create/update/delete clusters and nodepools?" -- If **no**, it's Tier0
- "Does this test validate the happy path of a primary user operation?" -- If **yes**, it's Tier0
- "Would a failure here mean data inconsistency between the API and Kubernetes?" -- If **yes**, it's Tier0

#### Examples from existing test cases

| Test Case | Why Tier0 |
|-----------|-----------|
| [Cluster Basic Workflow Validation](testcases/cluster.md#test-title-clusters-resource-type---basic-workflow-validation) | Core lifecycle -- validates the complete create-to-Ready path that every cluster goes through |
| [Cluster deletion happy path](testcases/delete-cluster.md#test-title-cluster-deletion-happy-path----soft-delete-through-hard-delete) | Core lifecycle -- validates the complete delete path (soft-delete, adapter finalization, hard-delete) |
| [Cluster update via PATCH triggers reconciliation](testcases/update-cluster.md#test-title-cluster-update-via-patch-triggers-reconciliation-and-reaches-reconciled) | Core lifecycle -- validates that spec changes propagate through the reconciliation pipeline |
| [Adapter Dependency Relationships](testcases/cluster.md#test-title-clusters-resource-type---adapter-dependency-relationships-workflow-validation) | Platform guarantee -- validates that the workflow engine enforces adapter execution ordering |
| [ManifestWork creation and status via Maestro](testcases/adapter-with-maestro-transport.md#test-title-adapter-can-create-manifestwork-and-report-status-via-maestro-transport) | Core transport -- validates the primary Maestro transport path that production adapters use |

### Tier1 -- Major

A Tier1 failure means **important features are affected but core operations still work**. The system functions but behaves incorrectly in specific scenarios.

#### Criteria (any one is sufficient)

- **Error handling**: The test validates that the system correctly reports errors, returns proper HTTP status codes (409, 404), or handles misconfigured components
- **Secondary workflow**: The test validates a workflow that supplements but is not part of the primary CRUD path (concurrent operations, edge cases of core operations)
- **API contract enforcement**: The test validates that the API rejects invalid operations (mutating a deleted resource, creating under a deleted parent)
- **Operational visibility**: Failure means errors are silently swallowed or misreported, but the system still functions

#### Decision questions

- "If this test fails, does the core create/update/delete path still work for standard use cases?" -- If **yes**, it's likely Tier1
- "Does this test validate error handling, edge cases, or secondary workflows?" -- If **yes**, it's Tier1
- "Is this testing a configuration error scenario that affects one adapter/consumer, not the whole platform?" -- If **yes**, it's Tier1

#### Examples from existing test cases

| Test Case | Why Tier1 |
|-----------|-----------|
| [Cluster reflects adapter failure in top-level status](testcases/cluster.md#test-title-cluster-can-reflect-adapter-failure-in-top-level-status) | Error handling -- validates failure reporting, not the happy path. Core operations work; this tests that failures are visible |
| [PATCH to soft-deleted cluster returns 409](testcases/delete-cluster.md#test-title-patch-to-soft-deleted-cluster-returns-409-conflict) | API contract -- validates rejection of invalid mutations. The delete path works; this tests that the API guards against misuse |
| [Concurrent cluster creations without conflicts](testcases/concurrent-processing.md#test-title-system-can-process-concurrent-cluster-creations-without-resource-conflicts) | Secondary workflow -- concurrent creation is important but not the primary single-cluster path |
| [ManifestWork apply fails for unregistered consumer](testcases/adapter-with-maestro-transport.md#test-title-manifestwork-apply-fails-when-targeting-unregistered-consumer) | Error handling -- validates graceful failure for a misconfigured adapter, not the normal transport path |
| [Adapter statuses transition during update reconciliation](testcases/update-cluster.md#test-title-adapter-statuses-transition-during-update-reconciliation) | Secondary workflow -- validates intermediate state transitions, not the final converged state |
| [Stuck deletion -- adapter unable to finalize](testcases/delete-cluster.md#test-title-stuck-deletion----adapter-unable-to-finalize-prevents-hard-delete) | Operational visibility -- adapter permanently fails to finalize, causing silent resource leak that compounds over time if undetected |

### Tier2 -- Minor

A Tier2 failure means **edge cases or rare scenarios don't work as expected**. The system functions correctly for standard use cases.

#### Criteria (any one is sufficient)

- **Infrastructure recovery**: The test involves simulating infrastructure outages (crash, network partition) and verifying self-healing
- **Race condition / timing**: The test exercises non-deterministic timing scenarios (DELETE during creation, cascade during update)
- **Low frequency**: The scenario is unlikely to occur in normal operations and requires specific conditions to trigger
- **Manual intervention available**: If the scenario occurs in production, operators can recover manually

#### Decision questions

- "Does this test require simulating infrastructure failures (killing pods, scaling to zero, network partitions)?" -- If **yes**, it's likely Tier2
- "Is the scenario timing-dependent or non-deterministic?" -- If **yes**, it's likely Tier2
- "Would this scenario require unusual or unlikely conditions to occur in production?" -- If **yes**, it's likely Tier2

#### Examples from existing test cases

| Test Case | Why Tier2 |
|-----------|-----------|
| [Cluster reaches correct status after adapter crash and recovery](testcases/cluster.md#test-title-cluster-can-reach-correct-status-after-adapter-crash-and-recovery) | Infrastructure recovery -- involves killing adapter pods and verifying self-healing. Crashes are rare; operators can restart pods manually |
| [Maestro server unavailability graceful handling](testcases/adapter-with-maestro-transport.md#test-title-adapter-can-handle-maestro-server-unavailability-gracefully) | Infrastructure recovery -- simulates Maestro outage. Server failures are rare and recoverable |
| [DELETE during initial creation before Reconciled](testcases/delete-cluster.md#test-title-delete-during-initial-creation-before-cluster-reaches-reconciled) | Race condition -- user deletes a cluster before it finishes creating. Unusual timing scenario |
| [Cascade DELETE while child nodepool is mid-update](testcases/delete-cluster.md#test-title-cascade-delete-on-cluster-while-child-nodepool-is-mid-update-reconciliation) | Race condition -- cluster deletion while nodepool is being updated. Requires specific timing overlap |

### Tier Decision Flowchart

```text
Does this test validate a core Tier0 concern?
(lifecycle happy path, data integrity, platform guarantee, or system-wide blast radius)
  |
  +-- YES --> Tier0
  |
  +-- NO --> Does it validate error handling or API contract enforcement?
               |
               +-- YES --> Tier1
               |
               +-- NO --> Does it involve infrastructure failure, crash recovery, or race conditions?
                            |
                            +-- YES --> Tier2
                            |
                            +-- NO --> Is the code path exercised frequently in normal operations?
                                         |
                                         +-- YES --> Tier1
                                         |
                                         +-- NO --> Tier2
```

### Additional Labels

Beyond the severity tier, tests can carry additional labels that describe other dimensions. See `pkg/labels/labels.go` for the full list:

| Label | Dimension | Description |
|-------|-----------|-------------|
| `negative` | Scenario | Error handling and failure scenarios |
| `perf` | Scenario | Performance and stress tests |
| `upgrade` | Feature | Version compatibility tests |
| `disruptive` | Constraint | Destructive testing (fault injection, pod killing) |
| `slow` | Constraint | Execution time exceeds 5-10 minutes |

These labels are orthogonal to the tier -- a test can be `Tier0 + negative` or `Tier2 + disruptive + slow`.

## Writing a New Test Case

1. Copy the template from [`templates/testcase-template.md`](templates/testcase-template.md)
2. Assign a tier using the [decision flowchart](#tier-decision-flowchart) above
3. Place the file in `testcases/` following the naming convention: `{resource-type}.md` for grouped test cases or `{action}-{resource}.md` for standalone cases
4. If unsure about the tier assignment, start with Tier1 and discuss with the team during review
