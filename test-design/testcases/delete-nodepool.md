# Feature: Nodepool Deletion Lifecycle

## Table of Contents

1. [Nodepool deletion happy path -- soft-delete through hard-delete](#test-title-nodepool-deletion-happy-path----soft-delete-through-hard-delete)
2. [Nodepool deletion does not affect sibling nodepools](#test-title-nodepool-deletion-does-not-affect-sibling-nodepools)
3. [Re-DELETE on already-deleted nodepool is idempotent](#test-title-re-delete-on-already-deleted-nodepool-is-idempotent)
4. [DELETE non-existent nodepool returns 404](#test-title-delete-non-existent-nodepool-returns-404)
5. [PATCH to soft-deleted nodepool returns 409 Conflict](#test-title-patch-to-soft-deleted-nodepool-returns-409-conflict)
6. [Soft-deleted nodepool remains visible via GET and LIST](#test-title-soft-deleted-nodepool-remains-visible-via-get-and-list)

---

> **Hard-delete mechanism:** Hard-delete executes inline within the `POST /adapter_statuses` request that computes `Reconciled=True`. No separate endpoint or background process — test steps simply poll until GET returns 404. See [hard-delete-design.md](https://github.com/openshift-hyperfleet/architecture/blob/main/hyperfleet/components/api-service/hard-delete-design.md).

## Test Title: Nodepool deletion happy path -- soft-delete through hard-delete

### Description

This test validates the complete nodepool deletion lifecycle. It verifies that when a DELETE request is sent for a single nodepool, the API sets `deleted_time`, nodepool adapters clean up their managed resources and report `Finalized=True`, the nodepool reaches `Reconciled=True`, and hard-delete permanently removes the nodepool record. Critically, the parent cluster must remain unaffected by the nodepool deletion.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier0 |
| **Status** | Draft |
| **Automation** | Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-15 |
| **Updated** | 2026-04-30 |

---

### Preconditions

1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API and HyperFleet Sentinel services are deployed and running successfully
3. The adapters defined in testdata/adapter-configs are all deployed successfully

---

### Test Steps

#### Step 1: Create a cluster and a nodepool, wait for Reconciled state

**Action:**
- Create a cluster:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```
- Create a nodepool under the cluster:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/nodepools/nodepool-request.json
```
- Wait for both cluster and nodepool to reach Reconciled state

**Expected Result:**
- Cluster `Reconciled` condition `status: "True"`
- Nodepool `Reconciled` condition `status: "True"`

#### Step 2: Send DELETE request for the nodepool only

**Action:**
- Submit a DELETE request for the nodepool (not the cluster):
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}
```

**Expected Result:**
- Response returns HTTP 202 (Accepted)
- Response body includes the nodepool with `deleted_time` set to a valid RFC3339 timestamp
- Nodepool `generation` is incremented

#### Step 3: Verify nodepool adapters report Finalized=True

**Action:**
- Poll nodepool adapter statuses:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}/statuses
```

**Expected Result:**
- All nodepool adapters report `Finalized` condition `status: "True"`
- `Applied` condition transitions to `status: "False"` (managed resources deleted)
- `Available` condition transitions to `status: "False"`
- `observed_generation` matches the post-DELETE generation

#### Step 4: Verify nodepool reaches Reconciled=True and is hard-deleted

**Action:**
- Poll nodepool status until hard-delete completes (executes automatically when `Reconciled=True`):
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}
```

**Expected Result:**
- Nodepool `Reconciled` condition transitions to `status: "True"`
- After hard-delete: GET returns HTTP 404 (Not Found)

#### Step 5: Verify parent cluster is unaffected

**Action:**
- Retrieve the parent cluster:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster does NOT have `deleted_time` set
- Cluster `Reconciled` condition remains `status: "True"`
- Cluster `Available` condition remains `status: "True"`
- Cluster is fully operational and unaffected by the nodepool deletion

#### Step 6: Cleanup resources

**Action:**
- Delete the cluster (which cleans up remaining resources):
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster and all remaining resources are cleaned up

---

## Test Title: Nodepool deletion does not affect sibling nodepools

### Description

This test validates isolation between sibling nodepools during deletion. When one nodepool is deleted, other nodepools under the same cluster must remain in their current state with no `deleted_time` set and no disruption to their adapter statuses.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Automated |
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
- Create a cluster and two nodepools (each call generates a unique name via `{{.Random}}` template):
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
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/nodepools/nodepool-request.json
```
- Wait for all to reach Reconciled state

**Expected Result:**
- Cluster and both nodepools reach `Reconciled` condition `status: "True"`

#### Step 2: Delete one nodepool

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id_1}
```

**Expected Result:**
- Response returns HTTP 202 (Accepted) with `deleted_time` set on nodepool_1

#### Step 3: Verify sibling nodepool is unaffected

**Action:**
- Retrieve the sibling nodepool:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id_2}
```
- Retrieve sibling nodepool adapter statuses:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id_2}/statuses
```

**Expected Result:**
- Sibling nodepool does NOT have `deleted_time` set
- Sibling nodepool `Reconciled` condition remains `status: "True"`
- Sibling nodepool adapter statuses are unchanged (`Applied: True`, `Available: True`, `Health: True`)

#### Step 4: Verify parent cluster is unaffected

**Action:**
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster does NOT have `deleted_time` set
- Cluster `Reconciled` condition remains `status: "True"`

#### Step 5: Cleanup resources

**Action:**
- Delete the cluster:
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster, remaining nodepool, and all associated resources are cleaned up

---

## Test Title: Re-DELETE on already-deleted nodepool is idempotent

### Description

This test validates that calling DELETE on a nodepool that has already been soft-deleted returns the same result without error. The `deleted_time` should remain unchanged from the first DELETE call, and the cascade uses a `WHERE deleted_time IS NULL` guard so repeat calls are safe no-ops.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Automated |
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

#### Step 1: Create a cluster and nodepool, wait for Reconciled state

**Action:**
- Create a cluster and nodepool, wait for Reconciled:
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

**Expected Result:**
- Cluster and nodepool reach `Reconciled: True`

#### Step 2: Send first DELETE request

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}
```

**Expected Result:**
- Response returns HTTP 202 (Accepted) with `deleted_time` set
- Record `{original_deleted_time}` and `{original_generation}`

#### Step 3: Send second DELETE request

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}
```

**Expected Result:**
- Response returns HTTP 202 (Accepted)
- `deleted_time` is identical to `{original_deleted_time}`
- `generation` is identical to `{original_generation}` (not incremented again)

#### Step 4: Cleanup resources

**Action:**
- Delete the cluster:
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster and all associated resources are cleaned up

---

## Test Title: DELETE non-existent nodepool returns 404

### Description

This test validates that sending a DELETE request for a nodepool ID that does not exist under a valid cluster returns HTTP 404 Not Found.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Negative |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-15 |
| **Updated** | 2026-04-15 |

---

### Preconditions

1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API is deployed and running successfully
3. A valid cluster exists (for the cluster_id path parameter)

---

### Test Steps

#### Step 1: Create a cluster (for valid cluster_id)

**Action:**
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```

**Expected Result:**
- Cluster is created with a valid `cluster_id`

#### Step 2: Send DELETE request for a non-existent nodepool ID

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/non-existent-nodepool-id-12345
```

**Expected Result:**
- Response returns HTTP 404 (Not Found)
- Response body includes an error message indicating the nodepool was not found

#### Step 3: Cleanup resources

**Action:**
- Delete the cluster:
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster and all associated resources are cleaned up

---

## Test Title: PATCH to soft-deleted nodepool returns 409 Conflict

### Description

This test validates that the API rejects mutation requests (PATCH) to nodepools that have been soft-deleted. Once a nodepool has `deleted_time` set, no spec modifications should be allowed to prevent new generation events from triggering reconciliation while deletion cleanup is in progress.

**Note:** Same mechanism as the cluster PATCH 409 test case — a PATCH on a soft-deleted nodepool bumps `generation`, creating a mismatch that blocks hard-delete until adapters re-process at the new generation. The adapter won't recreate K8s resources (deletion check short-circuits apply), but the round-trip through Sentinel and adapter delays hard-delete. A 409 guard prevents this.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Negative |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Automated |
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

#### Step 1: Create a cluster and nodepool, wait for Reconciled state

**Action:**
- Create a cluster and nodepool:
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
- Cluster and nodepool reach `Reconciled` condition `status: "True"`
- Nodepool at `generation: 1`

#### Step 2: Send DELETE request to soft-delete the nodepool

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}
```

**Expected Result:**
- Response returns HTTP 202 (Accepted) with `deleted_time` set
- Nodepool `generation` incremented to 2

#### Step 3: Attempt PATCH on the soft-deleted nodepool

**Action:**
- Send a PATCH request to modify the nodepool spec:
```bash
curl -X PATCH ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id} \
  -H "Content-Type: application/json" \
  -d '{"spec": {"updated-key": "should-not-work"}}'
```

**Expected Result:**
- Response returns HTTP 409 (Conflict)
- Response body includes an error message indicating the resource is pending deletion
- The nodepool's `generation` remains at 2 (not incremented)

#### Step 4: Verify nodepool state is unchanged

**Action:**
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}
```

**Expected Result:**
- Nodepool spec does not contain the attempted change
- `generation` remains at 2
- `deleted_time` is still set (deletion not affected)

#### Step 5: Cleanup resources

**Action:**
- Delete the cluster (cleans up remaining resources):
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster and all associated resources are cleaned up

---

## Test Title: Soft-deleted nodepool remains visible via GET and LIST

### Description

This test validates that after a nodepool is soft-deleted, it remains queryable via GET and LIST before hard-delete. The test uses a Sentinel fence (scale `sentinel-nodepools` to 0) immediately after DELETE so the visibility window is deterministic and not dependent on reconciliation timing races.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Automated |
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

#### Step 1: Create a cluster with two nodepools and wait for Reconciled state

**Action:**
- Create a cluster and two nodepools:
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
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/nodepools/nodepool-request.json
```
- Wait for all to reach Reconciled state
- Record IDs as `{active_nodepool_id}` and `{deleted_nodepool_id}`

**Expected Result:**
- Cluster and both nodepools reach `Reconciled: True`

#### Step 2: Soft-delete one nodepool

**Action:**
- Soft-delete one nodepool:
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{deleted_nodepool_id}
```
- Scale Sentinel for nodepools to 0 replicas to freeze reconciliation while visibility assertions run:
```bash
kubectl scale deployment/sentinel-nodepools -n hyperfleet --replicas=0
kubectl rollout status deployment/sentinel-nodepools -n hyperfleet --timeout=60s
```

**Expected Result:**
- Response returns HTTP 202 (Accepted) with `deleted_time` set
- `generation` on `{deleted_nodepool_id}` is incremented to the post-delete generation
- Sentinel nodepool reconciler is paused, preventing hard-delete progression during visibility checks

#### Step 3: Verify GET observes the soft-deleted nodepool before hard-delete

**Action:**
- Poll GET with `Eventually` until the soft-deleted nodepool is observed via HTTP 200 with `deleted_time` populated. While the Sentinel fence is active, HTTP 404 in this step is a failure (it means visibility was not proven). Use framework-configured polling/timeout values (for example, `500ms` interval and `10s` timeout):
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{deleted_nodepool_id}
```

**Expected Result:**
- At least one GET returns HTTP 200 (OK) with the nodepool object present and `deleted_time` populated
- This proves the nodepool remains visible in soft-deleted state while reconciliation is paused
- HTTP 404 is not an acceptable success outcome for this visibility step

**Note:** During this observation period, `Reconciled` is frequently `False` while adapters finalize the post-delete generation, but it can transition quickly depending on system timing.

#### Step 4: Verify LIST includes both active and soft-deleted nodepools before hard-delete completes

**Action:**
- Poll LIST with `Eventually` until both the active and deleted nodepools are present simultaneously. Use framework-configured polling/timeout values (for example, `500ms` interval and `10s` timeout):
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools
```

**Expected Result:**
- At least one LIST returns both `{active_nodepool_id}` and `{deleted_nodepool_id}`
- `{active_nodepool_id}` has `deleted_time` as null/absent
- `{deleted_nodepool_id}` has `deleted_time` set to a valid RFC3339 timestamp
- Both nodepools have their full resource representation (conditions, spec, labels)

#### Step 5: Verify active nodepool is unaffected

**Action:**
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{active_nodepool_id}
```

**Expected Result:**
- Active nodepool: HTTP 200, no `deleted_time`, `Reconciled: True`
- Active nodepool adapter statuses are unchanged

#### Step 6: Cleanup resources

**Action:**
- Scale Sentinel for nodepools back to 1 replica to resume reconciliation:
```bash
kubectl scale deployment/sentinel-nodepools -n hyperfleet --replicas=1
kubectl rollout status deployment/sentinel-nodepools -n hyperfleet --timeout=60s
```
- Delete the cluster (cascades to remaining nodepool):
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```
- If the deleted nodepool or parent cluster still exists after the assertions, poll until GET returns HTTP 404 (hard-delete executes automatically when `Reconciled=True`)
- The framework cleanup helpers can handle any remaining lifecycle in `AfterEach`

**Expected Result:**
- Cluster and all nodepools are eventually hard-deleted (GET returns HTTP 404)

---
