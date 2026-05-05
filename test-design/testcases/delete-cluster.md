# Feature: Cluster Deletion Lifecycle

## Table of Contents

1. [Cluster deletion happy path -- soft-delete through hard-delete](#test-title-cluster-deletion-happy-path----soft-delete-through-hard-delete)
2. [Cluster deletion cascades to child nodepools](#test-title-cluster-deletion-cascades-to-child-nodepools)
3. [Soft-deleted cluster remains visible via GET and LIST](#test-title-soft-deleted-cluster-remains-visible-via-get-and-list)
4. [Re-DELETE on already-deleted cluster is idempotent](#test-title-re-delete-on-already-deleted-cluster-is-idempotent)
5. [PATCH to soft-deleted cluster returns 409 Conflict](#test-title-patch-to-soft-deleted-cluster-returns-409-conflict)
6. [Create nodepool under soft-deleted cluster returns 409 Conflict](#test-title-create-nodepool-under-soft-deleted-cluster-returns-409-conflict)
7. [DELETE non-existent cluster returns 404](#test-title-delete-non-existent-cluster-returns-404)
8. [Stuck deletion -- adapter unable to finalize prevents hard-delete](#test-title-stuck-deletion----adapter-unable-to-finalize-prevents-hard-delete)
9. [DELETE during initial creation before cluster reaches Reconciled](#test-title-delete-during-initial-creation-before-cluster-reaches-reconciled)
10. [Simultaneous DELETE requests produce a single soft-delete record](#test-title-simultaneous-delete-requests-produce-a-single-soft-delete-record)
11. [Adapter treats externally-deleted K8s resources as finalized](#test-title-adapter-treats-externally-deleted-k8s-resources-as-finalized)
12. [DELETE during update reconciliation before adapters converge](#test-title-delete-during-update-reconciliation-before-adapters-converge)
13. [Recreate cluster with same name after hard-delete](#test-title-recreate-cluster-with-same-name-after-hard-delete)
14. [LIST returns soft-deleted clusters alongside active clusters](#test-title-list-returns-soft-deleted-clusters-alongside-active-clusters)
15. [Cascade DELETE on cluster while a child nodepool is already deleting](#test-title-cascade-delete-on-cluster-while-a-child-nodepool-is-already-deleting)
16. [Cascade DELETE on cluster while child nodepool is mid-update-reconciliation](#test-title-cascade-delete-on-cluster-while-child-nodepool-is-mid-update-reconciliation)

---

> **Hard-delete mechanism:** Hard-delete executes inline within the `POST /adapter_statuses` request that computes `Reconciled=True`. No separate endpoint or background process — test steps simply poll until GET returns 404. See [hard-delete-design.md](https://github.com/openshift-hyperfleet/architecture/blob/main/hyperfleet/components/api-service/hard-delete-design.md).

## Test Title: Cluster deletion happy path -- soft-delete through hard-delete

### Description

This test validates the complete cluster deletion lifecycle end-to-end. It verifies that when a DELETE request is sent for a cluster, the API sets `deleted_time` (soft-delete), adapters detect the deletion and clean up their managed K8s resources reporting `Finalized=True`, the API computes `Reconciled=True` from adapter statuses, hard-delete permanently removes the cluster record from the database, and the downstream K8s namespace is confirmed absent.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier0 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-15 |
| **Updated** | 2026-04-15 |

---

### Preconditions

1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API and HyperFleet Sentinel services are deployed and running successfully
3. The adapters defined in testdata/adapter-configs are all deployed successfully

---

### Test Steps

#### Step 1: Create a cluster and wait for it to reach Reconciled state

**Action:**
- Submit a POST request to create a Cluster resource:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```
- Wait for the cluster to reach Reconciled state:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster reaches `Reconciled` condition `status: "True"` with `observed_generation: 1`
- `Available` condition `status: "True"`
- All required adapters report `Applied: True`, `Available: True`, `Health: True`

#### Step 2: Send DELETE request to soft-delete the cluster

**Action:**
- Submit a DELETE request for the cluster:
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Response returns HTTP 202 (Accepted) status code
- Response body includes the cluster with `deleted_time` field set to a valid RFC3339 timestamp
- Cluster `generation` is incremented (from 1 to 2)

#### Step 3: Verify adapters complete deletion cleanup

**Action:**
- Poll adapter statuses until all adapters report `Finalized=True`:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses
```

**Expected Result:**
- All required adapters are present in the response
- Each adapter's final state after cleanup:
  - `Applied` condition `status: "False"` (managed resources deleted)
  - `Available` condition `status: "False"` (work no longer active)
  - `Finalized` condition `status: "True"` (cleanup confirmed)
  - `Health` condition `status: "True"` (adapter healthy throughout)
- **Adapter condition metadata validation** (for each condition):
  - `reason`: Non-empty string (e.g., `"ResourcesDeleted"`, `"CleanupComplete"`)
  - `message`: Non-empty string with human-readable description
  - `last_transition_time`: Valid RFC3339 timestamp
- **Adapter status metadata validation** (for each required adapter):
  - `observed_generation`: Equals 2 (matching the post-DELETE generation)

#### Step 4: Verify cluster reaches Reconciled=True and is hard-deleted

**Action:**
- Poll cluster status until `Reconciled` condition updates:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```
- Poll until cluster record is removed (hard-delete executes automatically when `Reconciled=True`):
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster `Reconciled` condition transitions to `status: "True"` (all adapters confirmed cleanup)
- After hard-delete completes: GET returns HTTP 404 (Not Found)
- Adapter statuses also return HTTP 404 or empty list

**Note:** The window between `Reconciled=True` and hard-delete may be brief. If polling observes 404 directly without capturing `Reconciled=True`, this still confirms the full lifecycle completed successfully.

#### Step 5: Verify downstream K8s namespace is cleaned up

**Action:**
- Poll using `h.GetNamespace()` within `Eventually()` until a `NotFound` error is returned:
```go
Eventually(func(g Gomega) {
    _, err := h.GetNamespace(ctx, clusterID)
    g.Expect(err).To(HaveOccurred())
    g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "expected NotFound from GetNamespace, got: %v", err)
}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())
```

**Expected Result:**
- `h.GetNamespace()` returns a `NotFound` error — the namespace and all resources within it were deleted as part of adapter finalization
- No orphaned K8s resources remain from this HyperFleet cluster

**Note:** Namespace deletion in K8s is asynchronous (namespace transitions through `Terminating` before removal). The `Eventually` wrapper prevents flaky failures when the namespace has not yet been fully garbage-collected at the time of the first check. Adapter `Finalized=True` confirms the adapter issued its K8s delete calls, but the namespace may still be terminating.

---

## Test Title: Cluster deletion cascades to child nodepools

### Description

This test validates hierarchical deletion behavior. When a cluster is deleted, the API must cascade `deleted_time` to all child nodepools simultaneously. Each nodepool's adapters must independently confirm cleanup via `Finalized=True`. The hard-delete mechanism must remove subresource records (nodepools) before removing the parent resource (cluster). After hard-delete, the cluster's downstream K8s namespace (which contained all nodepool resources) must be confirmed absent.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier0 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-15 |
| **Updated** | 2026-04-15 |

---

### Preconditions

1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API and HyperFleet Sentinel services are deployed and running successfully
3. The adapters defined in testdata/adapter-configs are all deployed successfully

---

### Test Steps

#### Step 1: Create a cluster with two nodepools and wait for Reconciled state

**Action:**
- Create a cluster:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```
- Create two nodepools under the cluster (each call generates a unique name via `{{.Random}}` template):
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/nodepools/nodepool-request.json
```
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/nodepools/nodepool-request.json
```
- Wait for cluster and both nodepools to reach Reconciled state

**Expected Result:**
- Cluster `Reconciled` condition `status: "True"`
- Both nodepools `Reconciled` condition `status: "True"`

#### Step 2: Send DELETE request for the cluster (not individual nodepools)

**Action:**
- Submit a DELETE request for the cluster:
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Response returns HTTP 202 (Accepted) with `deleted_time` set on the cluster

#### Step 3: Verify cascade -- all child nodepools have deleted_time set

**Action:**
- Retrieve each nodepool to verify cascade:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id_1}
```
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id_2}
```

**Expected Result:**
- Both nodepools have `deleted_time` set (non-nil, valid RFC3339 timestamp)
- The nodepool `deleted_time` values may differ slightly from the cluster `deleted_time`; exact equality is not required
- Each nodepool's `generation` is incremented

#### Step 4: Verify all adapters report Finalized=True and hard-delete completes

**Action:**
- Poll nodepool and cluster adapter statuses until all report `Finalized=True`:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id_1}/statuses
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id_2}/statuses
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses
```
- Poll until all records are removed (hard-delete executes automatically when `Reconciled=True` — subresources first, then resource):
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id_1}
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id_2}
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- All nodepool and cluster adapters report `Finalized=True`, `Applied=False`
- Both nodepools return HTTP 404 (Not Found)
- Cluster returns HTTP 404 (Not Found)

**Note:** The window between `Reconciled=True` and hard-delete may be brief. If polling observes 404 directly, this still confirms the full lifecycle completed successfully.

#### Step 5: Verify downstream K8s namespace is cleaned up

**Action:**
- Poll using `h.GetNamespace()` within `Eventually()` until a `NotFound` error is returned:
```go
Eventually(func(g Gomega) {
    _, err := h.GetNamespace(ctx, clusterID)
    g.Expect(err).To(HaveOccurred())
    g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "expected NotFound from GetNamespace, got: %v", err)
}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())
```

**Expected Result:**
- `h.GetNamespace()` returns a `NotFound` error — the namespace and all resources within it (cluster + both child nodepools) were deleted as part of adapter finalization
- No orphaned K8s resources remain

**Note:** See test #1 Step 5 note on namespace deletion timing.

---

## Test Title: Soft-deleted cluster remains visible via GET and LIST

### Description

This test validates that after a cluster is soft-deleted, it remains queryable via GET and LIST operations before hard-delete. The test uses a Sentinel fence (scale `sentinel-clusters` to 0) immediately after DELETE so the visibility window is deterministic and not dependent on reconciliation timing races.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-15 |
| **Updated** | 2026-04-15 |

---

### Preconditions

1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API and HyperFleet Sentinel services are deployed and running successfully
3. The adapters defined in testdata/adapter-configs are all deployed successfully

---

### Test Steps

#### Step 1: Create a cluster and wait for Reconciled state

**Action:**
- Create a cluster and wait for `Reconciled` condition `status: "True"`:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```

**Expected Result:**
- Cluster reaches `Reconciled` condition `status: "True"` with `observed_generation: 1`
- Cluster `generation` equals 1

#### Step 2: Send DELETE request to soft-delete the cluster

**Action:**
- Send DELETE request:
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```
- Scale Sentinel for clusters to 0 replicas to freeze reconciliation while visibility assertions run:
```bash
kubectl scale deployment/sentinel-clusters -n hyperfleet --replicas=0
kubectl rollout status deployment/sentinel-clusters -n hyperfleet --timeout=60s
```

**Expected Result:**
- Response returns HTTP 202 (Accepted) with `deleted_time` set
- `deleted_time` is a valid RFC3339 timestamp
- `generation` is incremented to 2
- Sentinel cluster reconciler is paused, preventing hard-delete progression during visibility checks

#### Step 3: Verify GET observes the soft-deleted cluster before hard-delete

**Action:**
- Poll GET with `Eventually` until the soft-deleted cluster is observed via HTTP 200 with `deleted_time` populated. While the Sentinel fence is active, HTTP 404 in this step is a failure (it means visibility was not proven). Use framework-configured polling/timeout values (for example, `500ms` interval and `10s` timeout):
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- At least one GET returns HTTP 200 (OK) with the cluster object present and `deleted_time` populated
- Cluster `generation` equals 2
- This proves the cluster remains visible in soft-deleted state while reconciliation is paused
- HTTP 404 is not an acceptable success outcome for this visibility step

**Note:** During this observation period, `Reconciled` is frequently `False` while adapters finalize generation 2, but it can transition quickly depending on system timing.

#### Step 4: Verify LIST includes the soft-deleted cluster before hard-delete completes

**Action:**
- Poll LIST with `Eventually` until the deleted cluster appears with `deleted_time` set. Use framework-configured polling/timeout values (for example, `500ms` interval and `10s` timeout):
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters
```

**Expected Result:**
- At least one LIST includes the deleted cluster entry
- The cluster entry has `deleted_time` populated
- This confirms LIST visibility of the soft-deleted resource while reconciliation is paused

#### Step 5: Cleanup resources

**Action:**
- Scale Sentinel for clusters back to 1 replica to resume reconciliation:
```bash
kubectl scale deployment/sentinel-clusters -n hyperfleet --replicas=1
kubectl rollout status deployment/sentinel-clusters -n hyperfleet --timeout=60s
```
- If the cluster still exists after Step 4, continue polling until GET returns HTTP 404 (hard-delete executes automatically when `Reconciled=True` after Sentinel resumes)
- The framework's `h.CleanupTestCluster()` helper can also handle the remaining lifecycle in `AfterEach`

**Expected Result:**
- Cluster is eventually hard-deleted (GET returns HTTP 404)

---

## Test Title: Re-DELETE on already-deleted cluster is idempotent

### Description

This test validates that calling DELETE on a cluster that has already been soft-deleted returns the same result without error or side effects. The `deleted_time` should remain unchanged from the first DELETE call.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-15 |
| **Updated** | 2026-04-15 |

---

### Preconditions

1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API and HyperFleet Sentinel services are deployed and running successfully
3. The adapters defined in testdata/adapter-configs are all deployed successfully

---

### Test Steps

#### Step 1: Create a cluster and wait for Reconciled state

**Action:**
- Create a cluster and wait for Reconciled:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```

**Expected Result:**
- Cluster reaches `Reconciled: True`

#### Step 2: Send first DELETE request

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Response returns HTTP 202 (Accepted) with `deleted_time` set
- Record the `deleted_time` value as `{original_deleted_time}`
- Record the `generation` value as `{original_generation}`

#### Step 3: Send second DELETE request

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Response returns HTTP 202 (Accepted)
- `deleted_time` is identical to `{original_deleted_time}` (not updated)
- `generation` is identical to `{original_generation}` (not incremented again)
- No duplicate deletion events or side effects

#### Step 4: Cleanup resources

**Action:**
- The cluster is already soft-deleted. Poll until GET returns 404 (hard-delete executes automatically when `Reconciled=True`).
- The framework's `h.CleanupTestCluster()` helper handles this automatically in `AfterEach`.

**Expected Result:**
- Cluster is hard-deleted (GET returns 404)

---

## Test Title: PATCH to soft-deleted cluster returns 409 Conflict

### Description

This test validates that the API rejects mutation requests (PATCH) to clusters that have been soft-deleted. Once a cluster has `deleted_time` set, no spec modifications should be allowed to prevent new generation events from triggering reconciliation while deletion cleanup is in progress.

**Note:** The PATCH request schema only accepts mutable fields (`spec`), so `deleted_time` cannot be cleared via PATCH. However, a PATCH on a soft-deleted resource bumps `generation` (when spec changes), creating a mismatch (`observed_generation < generation`) that blocks hard-delete until all adapters re-process and report at the new generation. The adapter's `lifecycle.delete.when` check short-circuits spec application (no K8s resources are recreated), but the unnecessary round-trip through Sentinel, adapter, and status reporting delays hard-delete completion. A 409 guard at the API boundary prevents this distributed churn entirely.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Negative |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-15 |
| **Updated** | 2026-04-28 |

---

### Preconditions

1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API and HyperFleet Sentinel services are deployed and running successfully
3. The adapters defined in testdata/adapter-configs are all deployed successfully

---

### Test Steps

#### Step 1: Create a cluster and wait for Reconciled state

**Action:**
- Create a cluster and wait for Reconciled:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```

**Expected Result:**
- Cluster reaches Reconciled state at `generation: 1`

#### Step 2: Send DELETE request to soft-delete the cluster

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Response returns HTTP 202 (Accepted) with `deleted_time` set
- `generation` incremented to 2

#### Step 3: Attempt PATCH on the soft-deleted cluster

**Action:**
- Send a PATCH request to modify the cluster spec:
```bash
curl -X PATCH ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id} \
  -H "Content-Type: application/json" \
  -d '{"spec": {"updated-key": "should-not-work"}}'
```

**Expected Result:**
- Response returns HTTP 409 (Conflict)
- Response body includes an error message indicating the resource is pending deletion
- The cluster's `generation` remains at 2 (not incremented)

#### Step 4: Verify cluster state is unchanged

**Action:**
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster spec does not contain the attempted change
- `generation` remains at 2
- `deleted_time` is still set (deletion not affected)

#### Step 5: Cleanup resources

**Action:**
- The cluster is already soft-deleted. Poll until GET returns 404 (hard-delete executes automatically when `Reconciled=True`).
- The framework's `h.CleanupTestCluster()` helper handles this automatically in `AfterEach`.

**Expected Result:**
- Cluster is hard-deleted (GET returns 404)

---

## Test Title: Create nodepool under soft-deleted cluster returns 409 Conflict

### Description

This test validates that creating new subresources (nodepools) under a soft-deleted cluster is rejected with 409 Conflict. This prevents new resources from being provisioned while the parent cluster is being cleaned up.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Negative |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-15 |
| **Updated** | 2026-04-16 |

---

### Preconditions

1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API and HyperFleet Sentinel services are deployed and running successfully
3. The adapters defined in testdata/adapter-configs are all deployed successfully

---

### Test Steps

#### Step 1: Create a cluster and wait for Reconciled state

**Action:**
- Create a cluster and wait for Reconciled:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```

**Expected Result:**
- Cluster reaches `Reconciled: True`

#### Step 2: Send DELETE request to soft-delete the cluster

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Response returns HTTP 202 (Accepted) with `deleted_time` set

#### Step 3: Attempt to create a nodepool under the soft-deleted cluster

**Action:**
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/nodepools/nodepool-request.json
```

**Expected Result:**
- Response returns HTTP 409 (Conflict)
- Response body includes an error message indicating the parent cluster is pending deletion
- No nodepool record is created

#### Step 4: Verify no nodepool was created

**Action:**
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools
```

**Expected Result:**
- Response returns an empty list

#### Step 5: Cleanup resources

**Action:**
- The cluster is already soft-deleted. Poll until GET returns 404 (hard-delete executes automatically when `Reconciled=True`).
- The framework's `h.CleanupTestCluster()` helper handles this automatically in `AfterEach`.

**Expected Result:**
- Cluster is hard-deleted (GET returns 404)

---

## Test Title: DELETE non-existent cluster returns 404

### Description

This test validates that sending a DELETE request for a cluster ID that does not exist returns HTTP 404 Not Found. This covers the scenario where a cluster has already been hard-deleted or never existed.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Negative |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-15 |
| **Updated** | 2026-04-15 |

---

### Preconditions

1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API is deployed and running successfully

---

### Test Steps

#### Step 1: Send DELETE request for a non-existent cluster ID

**Action:**
- Send a DELETE request with a random/non-existent cluster ID:
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/non-existent-cluster-id-12345
```

**Expected Result:**
- Response returns HTTP 404 (Not Found)
- Response body includes an error message indicating the cluster was not found

---

## Test Title: Stuck deletion -- adapter unable to finalize prevents hard-delete

### Description

This test validates that when an adapter is unable to complete deletion cleanup (e.g., it is crashed or unhealthy), the cluster remains in soft-deleted state indefinitely. The system must not hard-delete the cluster record while any adapter has not confirmed finalization. This covers the "stuck deletion" scenario where `Reconciled` remains `False` because at least one adapter never reports `Finalized=True`.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Negative |
| **Priority** | Tier2 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-16 |
| **Updated** | 2026-04-28 |

---

### Preconditions

1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API and HyperFleet Sentinel services are deployed and running successfully
3. The adapters defined in testdata/adapter-configs are all deployed successfully
4. A dedicated crash-adapter is available for deployment via Helm (same as used in "Cluster can reach correct status after adapter crash and recovery")

---

### Test Steps

#### Step 1: Deploy crash-adapter and create a cluster, wait for Reconciled state

**Action:**
- Deploy a dedicated crash-adapter via Helm (`${ADAPTER_DEPLOYMENT_NAME}`), separate from the normal adapters used in other tests
- Create a cluster and wait for Reconciled:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```
- Wait for cluster to reach `Reconciled` condition `status: "True"` with all adapters (including crash-adapter) reporting `Applied: True`

**Expected Result:**
- Cluster reaches `Reconciled: True`
- crash-adapter is present in adapter statuses with `Applied: True`, `Available: True`, `Health: True`

#### Step 2: Scale down crash-adapter to simulate unavailability

**Action:**
- Scale the crash-adapter deployment to 0 replicas:
```bash
kubectl scale deployment/${ADAPTER_DEPLOYMENT_NAME} -n hyperfleet --replicas=0
```
- Wait for the crash-adapter pod to terminate

**Expected Result:**
- crash-adapter becomes unavailable (no running pods)

#### Step 3: Send DELETE request to soft-delete the cluster

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Response returns HTTP 202 (Accepted) with `deleted_time` set

#### Step 4: Wait and verify cluster remains stuck in soft-deleted state

**Action:**
- Wait for `2 * h.Cfg.Timeouts.Cluster.Reconciled` to allow healthy adapters to finalize
- Poll cluster status periodically:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```
- Poll adapter statuses:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses
```

**Expected Result:**
- Cluster is still visible via GET (HTTP 200, not 404) — **not** hard-deleted
- `deleted_time` is set (soft-deleted)
- `Reconciled` condition `status: "False"` (not all adapters have finalized)
- Healthy adapters report `Finalized: True`, `Applied: False` (they completed their cleanup)
- crash-adapter either:
  - Has no status entry (it is unavailable and cannot report), or
  - Reports stale status with `Finalized` absent or `Finalized: False`
- The cluster is **not** hard-deleted because the crash-adapter has not confirmed finalization

#### Step 5: Restore crash-adapter and verify deletion completes

**Action:**
- Scale the crash-adapter back up:
```bash
kubectl scale deployment/${ADAPTER_DEPLOYMENT_NAME} -n hyperfleet --replicas=1
```
- Wait for crash-adapter to become ready:
```bash
kubectl rollout status deployment/${ADAPTER_DEPLOYMENT_NAME} -n hyperfleet --timeout=60s
```
- Poll until cluster record is removed (hard-delete executes automatically when `Reconciled=True`):
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- crash-adapter detects the soft-deleted cluster and performs cleanup
- crash-adapter reports `Finalized: True`
- `Reconciled` condition transitions to `status: "True"`
- Hard-delete executes: GET returns HTTP 404 (Not Found)

#### Step 6: Cleanup resources

**Action:**
- Uninstall the crash-adapter Helm release
- Clean up the Pub/Sub subscription created by the adapter (if using Google Pub/Sub broker):
```bash
gcloud pubsub subscriptions delete {subscription_id} --project={project_id}
```
- If the cluster was not hard-deleted (test failed), fall back to namespace deletion:
```bash
kubectl delete namespace {cluster_id} --ignore-not-found
```

**Expected Result:**
- crash-adapter deployment is removed
- Pub/Sub subscription is deleted (if applicable)
- All test resources are cleaned up

---

## Test Title: DELETE during initial creation before cluster reaches Reconciled

### Description

This test validates deletion behavior when a cluster is still mid-reconciliation (adapters have not yet reported `Applied=True`). The cluster is created and immediately deleted without waiting for Reconciled state. Adapters should detect `deleted_time` regardless of their pre-deletion state and finalize cleanup. The system must not get stuck due to adapters having stale or incomplete status from the initial creation.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier2 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-16 |
| **Updated** | 2026-04-16 |

---

### Preconditions

1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API and HyperFleet Sentinel services are deployed and running successfully
3. The adapters defined in testdata/adapter-configs are all deployed successfully

---

### Test Steps

#### Step 1: Create a cluster and immediately send DELETE without waiting for Reconciled

**Action:**
- Create a cluster:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```
- Immediately send DELETE (do NOT wait for Reconciled or any adapter status):
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- POST returns HTTP 201 with cluster created at `generation: 1`
- DELETE returns HTTP 202 (Accepted) with `deleted_time` set, `generation: 2`

#### Step 1a: Capture adapter statuses at the moment of DELETE (optional validation)

**Action:**
- Immediately after the DELETE response, capture adapter statuses to verify the edge case was exercised:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses
```

**Expected Result:**
- At least one adapter should have no status entry yet or report `Applied=False` (still mid-reconciliation from initial creation)
- If all adapters already report `Applied=True`, log a warning: the edge case was not exercised and this run is equivalent to a happy-path deletion test. The test still passes but the stale-state scenario was not validated.

#### Step 2: Verify adapters finalize despite incomplete initial reconciliation

**Action:**
- Poll adapter statuses until all adapters report `Finalized=True`:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses
```

**Expected Result:**
- All required adapters eventually report `Finalized` condition `status: "True"`
- Adapters that had not yet reported `Applied=True` (stale `Applied=False` or no status at all) still detect the soft-delete and finalize
- `observed_generation: 2` on all adapter statuses

**Note:** Some adapters may have partially applied K8s resources from the initial creation before detecting `deleted_time`. The adapter's `lifecycle.delete.when` check runs before apply on subsequent reconciliation, so these partial resources should be cleaned up during finalization.

#### Step 3: Verify cluster is hard-deleted

**Action:**
- Poll until cluster record is removed (hard-delete executes automatically when `Reconciled=True`):
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster `Reconciled` condition transitions to `status: "True"` (all adapters confirmed finalization)
- Hard-delete executes: GET returns HTTP 404 (Not Found)

#### Step 4: Cleanup resources

**Action:**
- If the cluster was not hard-deleted (test failed), fall back to namespace deletion:
```bash
kubectl delete namespace {cluster_id} --ignore-not-found
```

**Expected Result:**
- All test resources are cleaned up

---

## Test Title: Simultaneous DELETE requests produce a single soft-delete record

### Description

This test validates that when multiple DELETE requests for the same cluster are issued in parallel (as opposed to sequentially), the API handles them idempotently at the server boundary. Exactly one soft-delete is written (`deleted_time` is set once), `generation` is incremented exactly once, and the downstream reconciliation completes normally. This complements the sequential re-DELETE idempotency test by exercising the concurrency-safety property of the DELETE handler.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-16 |
| **Updated** | 2026-04-16 |

---

### Preconditions

1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API and HyperFleet Sentinel services are deployed and running successfully
3. The adapters defined in testdata/adapter-configs are all deployed successfully

---

### Test Steps

#### Step 1: Create a cluster and wait for Reconciled state

**Action:**
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```
- Wait for `Reconciled: True` at `generation: 1`

**Expected Result:**
- Cluster reaches `Reconciled: True`, `generation: 1`

#### Step 2: Send multiple DELETE requests in parallel

**Action:**
- Fire 5 DELETE requests simultaneously against the same cluster, capturing HTTP status, response body, and response time per request:
```bash
for i in $(seq 1 5); do
  curl -s -o /tmp/resp_$i.json -w "%{http_code}\n" \
    -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id} &
done
wait
```

**Expected Result:**
- Every request returns HTTP 200 or 202 -- no 5xx responses
- No request returns 404 (all observe the resource as existing at least at time of handler entry)

#### Step 3: Verify exactly one soft-delete was written

**Action:**
- Compare `deleted_time` and `generation` across all 5 response bodies:
```bash
jq -r '{deleted_time, generation}' /tmp/resp_*.json | sort -u
```
- Also GET the cluster to confirm current server-side state:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- All 5 responses carry the **same** `deleted_time` value (single RFC3339 timestamp)
- All 5 responses carry the **same** post-delete `generation` value
- Server-side GET shows `generation` incremented by exactly 1 compared to Step 1 (i.e., equals 2), **not** by the number of parallel DELETE requests
- `deleted_time` is set exactly once (no duplicate soft-delete writes)

#### Step 4: Verify deletion completes normally

**Action:**
- Poll adapter statuses and cluster GET until hard-delete completes (executes automatically when `Reconciled=True`):
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Adapters process deletion exactly once per adapter (each `observed_generation` advances to the post-delete generation)
- No duplicate cleanup events, no error logs on adapters
- `Reconciled` transitions to `status: "True"` (all adapters Finalized)
- GET returns HTTP 404 (hard-deleted)

#### Step 5: Cleanup resources

**Action:**
- The cluster is already hard-deleted. If the test failed before hard-delete, fall back to namespace deletion:
```bash
kubectl delete namespace {cluster_id} --ignore-not-found
```

**Expected Result:**
- All test resources are cleaned up

---

## Test Title: Adapter treats externally-deleted K8s resources as finalized

### Description

This test validates the adapter-side "NotFound as success" semantics. When the managed K8s resources have already been removed from the target cluster by an external actor (a human operator, another controller, a cloud-provider cleanup, etc.) *before* the adapter runs its deletion reconciliation, the adapter must treat the absence of the resource as a successful cleanup outcome and report `Finalized=True`. The API-level deletion workflow should then complete normally, regardless of the fact that the adapter did not itself issue any K8s delete calls.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-16 |
| **Updated** | 2026-04-28 |

---

### Preconditions

1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API and HyperFleet Sentinel services are deployed and running successfully
3. The adapters defined in testdata/adapter-configs are all deployed successfully
4. The tester has `kubectl` credentials sufficient to delete the namespace created by the cluster adapters (to simulate external deletion)

---

### Test Steps

#### Step 1: Create a cluster and wait for Reconciled state

**Action:**
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```
- Wait for cluster to reach `Reconciled: True`, `generation: 1`
- Confirm the managed K8s resources exist:
```bash
kubectl get namespace {cluster_id}
```

**Expected Result:**
- Cluster is Reconciled; managed namespace exists

#### Step 2: Externally delete the managed K8s resources (bypass the API)

**Action:**
- Delete the namespace directly via `kubectl`, bypassing the HyperFleet API:
```bash
kubectl delete namespace {cluster_id} --wait=true
```
- Verify the namespace is gone:
```bash
kubectl get namespace {cluster_id}
```

**Expected Result:**
- `kubectl get` returns `NotFound` (or confirms namespace is in `Terminating` then fully gone)
- Important: do **not** issue an API DELETE in this step -- the API still thinks the cluster is Reconciled

#### Step 3: Send DELETE request through the API

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Response returns HTTP 202 (Accepted) with `deleted_time` set, `generation: 2`

#### Step 4: Verify adapters report Finalized=True even though they did not delete anything

**Action:**
- Poll adapter statuses until every required adapter reports `Finalized=True`:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses
```

**Expected Result:**
- Each required adapter reports `Finalized` condition `status: "True"` with `observed_generation: 2`
- `Finalized` condition `reason`: assert present/non-nil first, then assert non-empty; do not assert on specific string values
- `Finalized` condition `message`: assert present/non-nil first, then assert non-empty; do not assert on specific string values
- `Finalized` condition `last_transition_time` is a valid RFC3339 timestamp
- `Health` condition remains `status: "True"` (adapter itself is healthy; the NotFound was not an error)
- No error-class log output is required from adapters; the NotFound path is the non-exceptional success path

#### Step 5: Verify cluster reaches Reconciled=True and is hard-deleted

**Action:**
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster `Reconciled` condition transitions to `status: "True"`
- Hard-delete executes: GET returns HTTP 404 (Not Found)
- Adapter statuses for this cluster are also removed

#### Step 6: Cleanup resources

**Action:**
- The cluster is already hard-deleted and the namespace was removed by the external action. No further cleanup required.
- If the test failed before hard-delete, fall back to API DELETE (if cluster still exists) and namespace deletion:
```bash
kubectl delete namespace {cluster_id} --ignore-not-found
```

**Expected Result:**
- All test resources are cleaned up

---

## Test Title: DELETE during update reconciliation before adapters converge

### Description

This test validates the interaction between update and delete workflows. When a cluster is updated via PATCH and immediately deleted before adapters finish reconciling the update, the deletion workflow must take priority. Adapters receive the next event, detect `deleted_time`, and switch to cleanup mode instead of continuing update reconciliation. This is distinct from [DELETE during initial creation](#test-title-delete-during-initial-creation-before-cluster-reaches-reconciled) (matrix #18) because adapters already have `Applied=True` from the previous generation and are mid-reconciliation for the new generation — a different code path in the adapter's lifecycle handler.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-16 |
| **Updated** | 2026-04-16 |

---

### Preconditions

1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API and HyperFleet Sentinel services are deployed and running successfully
3. The adapters defined in testdata/adapter-configs are all deployed successfully

---

### Test Steps

#### Step 1: Create a cluster and wait for Reconciled state at generation 1

**Action:**
- Create a cluster and wait for full convergence:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```
- Wait for `Reconciled` condition `status: "True"` at `generation: 1`

**Expected Result:**
- Cluster reaches `Reconciled: True` at `generation: 1`
- All adapters report `Applied: True`, `observed_generation: 1`

#### Step 2: Send PATCH request (do NOT wait for reconciliation to complete)

**Action:**
- Send a PATCH to trigger generation increment:
```bash
curl -X PATCH ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id} \
  -H "Content-Type: application/json" \
  -d '{"spec": {"trigger-update": "true"}}'
```

**Expected Result:**
- Response returns HTTP 200 with `generation: 2`
- `Reconciled` transitions to `status: "False"` (adapters have not yet reconciled to generation 2)

#### Step 3: Immediately send DELETE before update reconciliation completes

**Action:**
- Without waiting for adapters to reconcile to generation 2, send DELETE:
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Response returns HTTP 202 (Accepted) with `deleted_time` set
- `generation` incremented to 3

#### Step 4: Verify adapters switch to deletion mode and finalize

**Action:**
- Poll adapter statuses until all adapters report `Finalized=True`:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses
```

**Expected Result:**
- All required adapters report `Finalized` condition `status: "True"`
- `observed_generation: 3` (adapters reconciled the deletion generation, not the update generation)
- `Applied` condition `status: "False"` (managed resources deleted)
- `Available` condition `status: "False"` (work no longer active)
- Adapters did not complete the update reconciliation for generation 2 — they detected `deleted_time` and switched to cleanup mode

#### Step 5: Verify cluster is hard-deleted

**Action:**
- Poll until cluster record is removed (hard-delete executes automatically when `Reconciled=True`):
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster `Reconciled` condition transitions to `status: "True"` (all adapters confirmed finalization)
- Hard-delete executes: GET returns HTTP 404 (Not Found)

#### Step 6: Cleanup resources

**Action:**
- If the cluster was not hard-deleted (test failed), fall back to namespace deletion:
```bash
kubectl delete namespace {cluster_id} --ignore-not-found
```

**Expected Result:**
- All test resources are cleaned up

---

## Test Title: Recreate cluster with same name after hard-delete

### Description

This test validates that after a cluster is fully deleted (hard-deleted from the database), a new cluster can be created with the same name without conflicts. This is a common user scenario: delete a cluster, then recreate it with the same configuration. The system must ensure no state from the previous cluster (K8s namespace, adapter subscriptions, Sentinel state, database records) interferes with the new creation. The new cluster must reach `Reconciled=True` through a clean lifecycle, not inheriting or colliding with artifacts from the previous cluster.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-16 |
| **Updated** | 2026-04-16 |

---

### Preconditions

1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API and HyperFleet Sentinel services are deployed and running successfully
3. The adapters defined in testdata/adapter-configs are all deployed successfully

---

### Test Steps

#### Step 1: Create a cluster and wait for Reconciled state

**Action:**
- Create a cluster using the standard payload (name is generated via `{{.Random}}` template):
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```
- Wait for `Reconciled` condition `status: "True"` at `generation: 1`
- Record the `id` as `{first_cluster_id}` and the `name` as `{cluster_name}`

**Expected Result:**
- Cluster reaches `Reconciled: True`
- All adapters report `Applied: True`

#### Step 2: Delete the cluster and wait for hard-delete to complete

**Action:**
- Send DELETE request:
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{first_cluster_id}
```
- Poll until GET returns 404 (hard-delete executes automatically when `Reconciled=True`):
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{first_cluster_id}
```

**Expected Result:**
- DELETE returns HTTP 202 with `deleted_time` set
- Adapters report `Finalized: True`
- Hard-delete completes: GET returns HTTP 404

#### Step 3: Create a new cluster with the same name

**Action:**
- Create a new cluster reusing `{cluster_name}` captured from Step 1's response:
```bash
jq --arg name "{cluster_name}" '.name = $name' testdata/payloads/clusters/cluster-request.json \
| curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @-
```
- Record the `id` as `{second_cluster_id}`

**Expected Result:**
- Response returns HTTP 201 (Created)
- `{second_cluster_id}` is a new UUID, different from `{first_cluster_id}`
- `generation: 1` (fresh resource, not inheriting from the deleted cluster)

#### Step 4: Verify the new cluster reaches Reconciled=True through a clean lifecycle

**Action:**
- Wait for the new cluster to reach `Reconciled: True`:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{second_cluster_id}
```
- Verify adapter statuses:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{second_cluster_id}/statuses
```

**Expected Result:**
- Cluster `Reconciled` condition `status: "True"` at `generation: 1`
- All adapters report `Applied: True`, `Available: True`, `Health: True` with `observed_generation: 1`
- No adapter errors related to pre-existing resources, duplicate subscriptions, or namespace conflicts

#### Step 5: Verify the old cluster is still gone

**Action:**
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{first_cluster_id}
```

**Expected Result:**
- GET returns HTTP 404 (the old cluster was not resurrected by the recreate)

#### Step 6: Cleanup resources

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{second_cluster_id}
```
- Poll until GET returns 404 (hard-delete executes automatically when `Reconciled=True`).
- If cleanup fails, fall back to namespace deletion:
```bash
kubectl delete namespace {second_cluster_id} --ignore-not-found
```

**Expected Result:**
- All test resources are cleaned up

---

## Test Title: LIST returns soft-deleted clusters alongside active clusters

### Description

This test validates that soft-deleted clusters (with `deleted_time` set) remain visible in LIST responses alongside active clusters. The test uses a Sentinel fence (scale `sentinel-clusters` to 0) immediately after DELETE so the observation window is deterministic and not dependent on reconciliation races.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-17 |
| **Updated** | 2026-04-28 |

---

### Preconditions

1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API and HyperFleet Sentinel services are deployed and running successfully
3. The adapters defined in testdata/adapter-configs are all deployed successfully

---

### Test Steps

#### Step 1: Create two clusters and wait for Reconciled state

**Action:**
- Create two clusters:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```
- Wait for both to reach Reconciled state
- Record IDs as `{active_cluster_id}` and `{deleted_cluster_id}`

**Expected Result:**
- Both clusters reach `Reconciled: True`

#### Step 2: Soft-delete one cluster

**Action:**
- Soft-delete one cluster:
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{deleted_cluster_id}
```
- Scale Sentinel for clusters to 0 replicas to freeze reconciliation while visibility assertions run:
```bash
kubectl scale deployment/sentinel-clusters -n hyperfleet --replicas=0
kubectl rollout status deployment/sentinel-clusters -n hyperfleet --timeout=60s
```

**Expected Result:**
- Response returns HTTP 202 (Accepted) with `deleted_time` set
- `generation` on `{deleted_cluster_id}` is incremented to the post-delete generation
- Sentinel cluster reconciler is paused, preventing hard-delete progression during visibility checks

#### Step 3: LIST all clusters and verify both appear before hard-delete completes

**Action:**
- Poll LIST with `Eventually` until both the active and deleted clusters are present simultaneously. Use framework-configured polling/timeout values (for example, `500ms` interval and `10s` timeout):
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters
```

**Expected Result:**
- At least one LIST returns both `{active_cluster_id}` and `{deleted_cluster_id}`
- `{active_cluster_id}` has `deleted_time` as null/absent
- `{deleted_cluster_id}` has `deleted_time` set to a valid RFC3339 timestamp
- Both clusters have their full resource representation (conditions, spec, labels)

#### Step 4: Verify GET for each cluster returns the expected state before hard-delete completes

**Action:**
- Poll GET on `{deleted_cluster_id}` with `Eventually` until the soft-deleted cluster is observed via HTTP 200 with `deleted_time` populated. While the Sentinel fence is active, HTTP 404 in this step is a failure (it means visibility was not proven). Use framework-configured polling/timeout values (for example, `500ms` interval and `10s` timeout), then GET the active cluster:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{active_cluster_id}
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{deleted_cluster_id}
```

**Expected Result:**
- Active cluster: HTTP 200, no `deleted_time`, `Reconciled: True`
- Deleted cluster: observed at least once as HTTP 200 with `deleted_time` set while reconciliation is paused
- The deleted cluster remains observable long enough to distinguish it from the active cluster
- HTTP 404 is not an acceptable success outcome for this visibility step

#### Step 5: Cleanup resources

**Action:**
- Scale Sentinel for clusters back to 1 replica to resume reconciliation:
```bash
kubectl scale deployment/sentinel-clusters -n hyperfleet --replicas=1
kubectl rollout status deployment/sentinel-clusters -n hyperfleet --timeout=60s
```
- Delete the active cluster:
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{active_cluster_id}
```
- If either cluster still exists after the assertions, poll until both return HTTP 404 (hard-delete executes automatically when `Reconciled=True`)
- The framework cleanup helpers can handle any remaining lifecycle in `AfterEach`

**Expected Result:**
- Both clusters are eventually hard-deleted (GET returns HTTP 404)

---

## Test Title: Cascade DELETE on cluster while a child nodepool is already deleting

### Description

This test validates the interaction between individual nodepool deletion and parent cluster cascade deletion. When a nodepool is already soft-deleted (has `deleted_time` set) and the parent cluster is subsequently deleted, the cascade must not overwrite the nodepool's existing `deleted_time`. The nodepool's original deletion timestamp and lifecycle must be preserved. Both the nodepool and cluster must eventually complete their deletion lifecycles.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier2 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-17 |
| **Updated** | 2026-04-17 |

---

### Preconditions

1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API and HyperFleet Sentinel services are deployed and running successfully
3. The adapters defined in testdata/adapter-configs are all deployed successfully

---

### Test Steps

#### Step 1: Create a cluster with a nodepool and wait for Reconciled state

**Action:**
- Create a cluster and one nodepool:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/nodepools/nodepool-request.json
```
- Wait for both to reach Reconciled state

**Expected Result:**
- Cluster and nodepool reach `Reconciled: True`

#### Step 2: Soft-delete the nodepool first (do NOT wait for hard-delete)

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}
```
- Record the nodepool's `deleted_time` as `{nodepool_original_deleted_time}`

**Expected Result:**
- Response returns HTTP 202 (Accepted) with `deleted_time` set on the nodepool

#### Step 3: Immediately delete the parent cluster

**Action:**
- Without waiting for the nodepool deletion to complete:
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Response returns HTTP 202 (Accepted) with `deleted_time` set on the cluster

#### Step 4: Verify nodepool's deleted_time is preserved (not overwritten by cascade)

**Action:**
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}
```

**Expected Result:**
- Nodepool `deleted_time` equals `{nodepool_original_deleted_time}` (preserved from Step 2, not overwritten by the cluster cascade)
- The cascade's `WHERE deleted_time IS NULL` guard should have skipped the already-deleted nodepool

#### Step 5: Verify both resources complete deletion lifecycle

**Action:**
- Poll until both are hard-deleted (hard-delete executes automatically when `Reconciled=True`):
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Nodepool adapters report `Finalized: True`
- Cluster adapters report `Finalized: True`
- Both return HTTP 404 after hard-delete completes

#### Step 6: Cleanup resources

**Action:**
- If the test failed before hard-delete, fall back to namespace deletion:
```bash
kubectl delete namespace {cluster_id} --ignore-not-found
```

**Expected Result:**
- All test resources are cleaned up

---

## Test Title: Cascade DELETE on cluster while child nodepool is mid-update-reconciliation

### Description

This test validates the interaction between nodepool update reconciliation and parent cluster cascade deletion. When a nodepool has been updated via PATCH (generation incremented, adapters not yet reconciled) and the parent cluster is subsequently deleted, the cascade must set `deleted_time` on the nodepool. Nodepool adapters must detect the soft-delete and switch to deletion mode, abandoning the in-flight update reconciliation. This is distinct from [Cascade DELETE on cluster while a child nodepool is already deleting](#test-title-cascade-delete-on-cluster-while-a-child-nodepool-is-already-deleting) (matrix #26) because here the nodepool has a pending spec update — the adapter must prioritize deletion over update reconciliation at a generation that has been bumped by both the PATCH and the cascade DELETE.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier2 |
| **Status** | Draft |
| **Automation** | Not Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-17 |
| **Updated** | 2026-04-17 |

---

### Preconditions

1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API and HyperFleet Sentinel services are deployed and running successfully
3. The adapters defined in testdata/adapter-configs are all deployed successfully

---

### Test Steps

#### Step 1: Create a cluster with a nodepool and wait for Reconciled state

**Action:**
- Create a cluster and one nodepool:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/nodepools/nodepool-request.json
```
- Wait for both to reach Reconciled state at `generation: 1`

**Expected Result:**
- Cluster and nodepool reach `Reconciled: True` at `generation: 1`
- All adapters report `observed_generation: 1`

#### Step 2: PATCH the nodepool to trigger update reconciliation (do NOT wait for reconciliation)

**Action:**
- Send a PATCH to the nodepool to bump its generation:
```bash
curl -X PATCH ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id} \
  -H "Content-Type: application/json" \
  -d '{"spec": {"trigger-update": "true"}}'
```

**Expected Result:**
- Response returns HTTP 200 with nodepool `generation: 2`
- Nodepool adapters have not yet reconciled to generation 2 (update in flight)

#### Step 3: Immediately DELETE the parent cluster before nodepool reconciliation completes

**Action:**
- Without waiting for nodepool adapters to reconcile:
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Response returns HTTP 202 (Accepted) with `deleted_time` set on the cluster
- Cluster `generation` incremented

#### Step 4: Verify cascade sets deleted_time on the nodepool

**Action:**
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}
```

**Expected Result:**
- Nodepool has `deleted_time` set (cascaded from parent cluster)
- Nodepool `generation` is incremented beyond 2 (bumped by both the PATCH and the cascade DELETE)

#### Step 5: Verify all adapters finalize and both resources are hard-deleted

**Action:**
- Poll adapter statuses for both nodepool and cluster:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}/statuses
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses
```
- Poll until hard-delete completes (hard-delete executes automatically when `Reconciled=True`):
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Nodepool adapters report `Finalized: True` at the final generation (not the update generation)
- Nodepool adapters did not complete update reconciliation for generation 2 — they detected `deleted_time` and switched to cleanup mode
- Cluster adapters report `Finalized: True`
- Both nodepool and cluster return HTTP 404 after hard-delete

#### Step 6: Cleanup resources

**Action:**
- If the test failed before hard-delete, fall back to namespace deletion:
```bash
kubectl delete namespace {cluster_id} --ignore-not-found
```

**Expected Result:**
- All test resources are cleaned up

---
