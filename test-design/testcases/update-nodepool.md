# Feature: Nodepool Update Lifecycle (PATCH)

## Table of Contents

1. [Nodepool update via PATCH triggers reconciliation and reaches Reconciled](#test-title-nodepool-update-via-patch-triggers-reconciliation-and-reaches-reconciled)
2. [Labels-only PATCH bumps generation and triggers reconciliation](#test-title-labels-only-patch-bumps-generation-and-triggers-reconciliation)

---

## Test Title: Nodepool update via PATCH triggers reconciliation and reaches Reconciled

### Description

This test validates the nodepool update lifecycle. It verifies that when a PATCH request modifies a nodepool's spec, the nodepool's `generation` is incremented independently of the parent cluster, nodepool adapters reconcile to the new generation, and the nodepool reaches `Reconciled=True` and `Available=True`. The parent cluster must remain unaffected.

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

#### Step 1: Create a cluster and nodepool, wait for Reconciled and Available state

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
- Nodepool `generation` equals 1
- Nodepool `Reconciled` condition `status: "True"` with `observed_generation: 1`
- Nodepool `Available` condition `status: "True"` with `observed_generation: 1`
- All required nodepool adapters report `observed_generation: 1`
- **Per-adapter conditions on nodepool status**: each required adapter condition on the nodepool resource has `status: "True"`

#### Step 2: Send PATCH request to update the nodepool spec

**Action:**
```bash
curl -X PATCH ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id} \
  -H "Content-Type: application/json" \
  -d '{"spec": {"updated-key": "new-value"}}'
```

**Expected Result:**
- Response returns HTTP 200 (OK)
- Nodepool `generation` incremented from 1 to 2

#### Step 3: Verify nodepool adapters reconcile to the new generation

**Action:**
- Poll nodepool adapter statuses:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}/statuses
```

**Expected Result:**
- All nodepool adapters report `observed_generation: 2`
- Each adapter has `Applied: True`, `Available: True`, `Health: True`

#### Step 4: Verify nodepool reaches Reconciled=True and Available=True at new generation

**Action:**
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}
```

**Expected Result:**
- Nodepool `Reconciled` condition `status: "True"` with `observed_generation: 2`
- Nodepool `Available` condition `status: "True"` with `observed_generation: 2`
- Nodepool `id` is unchanged from Step 1
- Nodepool `cluster_id` is unchanged from Step 1
- `generation` equals 2
- **Per-adapter conditions on nodepool status**: each required adapter condition on the nodepool resource has `status: "True"`

#### Step 5: Verify parent cluster is unaffected

**Action:**
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster `generation` remains at 1 (unchanged)
- Cluster `Reconciled` condition `status: "True"` with `observed_generation: 1`
- Cluster `Available` condition `status: "True"` with `observed_generation: 1`

#### Step 6: Cleanup resources

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster, nodepool, and all associated resources are cleaned up


## Test Title: Labels-only PATCH bumps generation and triggers reconciliation

### Description

This test validates that a PATCH request that only modifies a nodepool's `labels` (without changing `spec`) increments the nodepool's `generation` and triggers adapter reconciliation. Generation is incremented when either `spec` or `labels` change. This mirrors the cluster-level labels PATCH behavior. The parent cluster must remain unaffected.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-17 |
| **Updated** | 2026-04-20 |

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
- Both at `generation: 1`, `Reconciled: True`

#### Step 2: Send labels-only PATCH request to the nodepool

**Action:**
```bash
curl -X PATCH ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id} \
  -H "Content-Type: application/json" \
  -d '{"labels": {"env": "staging", "pool-type": "gpu"}}'
```

**Expected Result:**
- Response returns HTTP 200 (OK)
- `generation` incremented from 1 to 2
- Labels in the response include the new values (`env: staging`, `pool-type: gpu`)
- `spec` is unchanged from Step 1

#### Step 3: Verify nodepool adapters reconcile to the new generation

**Action:**
- Poll nodepool adapter statuses until all report the new generation:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}/statuses
```

**Expected Result:**
- All nodepool adapters report `observed_generation: 2`
- Each adapter has `Applied: True`, `Available: True`, `Health: True`

#### Step 4: Verify nodepool reaches Reconciled=True and Available=True at new generation

**Action:**
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}
```

**Expected Result:**
- Nodepool `generation` equals 2
- Nodepool `Reconciled` condition `status: "True"` with `observed_generation: 2`
- Nodepool `Available` condition `status: "True"` with `observed_generation: 2`
- Labels reflect the PATCH update
- `spec` is unchanged

#### Step 5: Verify parent cluster is unaffected

**Action:**
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster `generation` remains at 1 (unchanged)
- Cluster `Reconciled` condition `status: "True"` with `observed_generation: 1`

#### Step 6: Cleanup resources

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster, nodepool, and all associated resources are cleaned up

