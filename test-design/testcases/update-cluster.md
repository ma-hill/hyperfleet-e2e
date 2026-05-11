# Feature: Cluster Update Lifecycle (PATCH)

## Table of Contents

1. [Cluster update via PATCH triggers reconciliation and reaches Reconciled](#test-title-cluster-update-via-patch-triggers-reconciliation-and-reaches-reconciled)
2. [Adapter statuses transition during update reconciliation](#test-title-adapter-statuses-transition-during-update-reconciliation)
3. [Multiple rapid updates coalesce to latest generation](#test-title-multiple-rapid-updates-coalesce-to-latest-generation)
4. [Labels-only PATCH bumps generation and triggers reconciliation](#test-title-labels-only-patch-bumps-generation-and-triggers-reconciliation)
5. [No-op PATCH does not increment generation](#test-title-no-op-patch-does-not-increment-generation)

---

## Test Title: Cluster update via PATCH triggers reconciliation and reaches Reconciled

### Description

This test validates the cluster update lifecycle end-to-end. It verifies that when a PATCH request modifies a cluster's spec, the API increments the `generation`, Sentinel detects the generation change and publishes a reconciliation event, adapters reconcile to the new generation reporting updated `observed_generation`, and the cluster reaches `Reconciled=True` and `Available=True` at the new generation. This confirms the complete update-reconciliation pipeline works correctly.

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

#### Step 1: Create a cluster and wait for Reconciled and Available state at generation 1

**Action:**
- Create a cluster and wait for full convergence:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```

**Expected Result:**
- Cluster `generation` equals 1
- Cluster `Reconciled` condition `status: "True"` with `observed_generation: 1`
- Cluster `Available` condition `status: "True"` with `observed_generation: 1`
- All required adapters report `observed_generation: 1`
- **Per-adapter conditions on cluster status**: each required adapter condition on the cluster resource has `status: "True"`

#### Step 2: Send PATCH request to update the cluster spec

**Action:**
- Submit a PATCH request to modify the cluster:
```bash
curl -X PATCH ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id} \
  -H "Content-Type: application/json" \
  -d '{"spec": {"updated-key": "new-value"}}'
```

**Expected Result:**
- Response returns HTTP 200 (OK)
- Response body shows `generation` incremented from 1 to 2
- The spec change is reflected in the response

#### Step 3: Verify adapters reconcile to the new generation

**Action:**
- Poll adapter statuses until all adapters report the new generation:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses
```

**Expected Result:**
- All required adapters report `observed_generation: 2`
- Each adapter has `Applied: True`, `Available: True`, `Health: True`
- **Adapter condition metadata validation** (for each condition):
  - `reason`: Non-empty string
  - `message`: Non-empty string
  - `last_transition_time`: Valid RFC3339 timestamp
- **Adapter status metadata validation** (for each required adapter):
  - `last_report_time`: Updated to a timestamp after the PATCH request

#### Step 4: Verify cluster reaches Reconciled=True and Available=True at new generation

**Action:**
- Retrieve the cluster status:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster `Reconciled` condition `status: "True"` with `observed_generation: 2`
- Cluster `Available` condition `status: "True"` with `observed_generation: 2`
- Cluster `id` is unchanged from Step 1
- `generation` equals 2
- **Per-adapter conditions on cluster status**: each required adapter condition on the cluster resource has `status: "True"`

#### Step 5: Cleanup resources

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster and all associated resources are cleaned up

---

## Test Title: Adapter statuses transition during update reconciliation

### Description

This test validates the intermediate status transitions during update reconciliation. When a cluster spec is updated, there is a window where adapters have not yet reconciled to the new generation. During this window, `Reconciled` should be `False` (indicating stale adapter statuses relative to the new generation). To guarantee this window is observable, a dedicated crash-adapter is deployed and scaled to 0 before the PATCH. With a stuck adapter, `Reconciled` remains `False` indefinitely, allowing reliable assertion via `Consistently`. After verification, the adapter is restored and full convergence is confirmed.

> **Automation note:** Deferred — the core mechanism (Reconciled=True requires all required adapters at current generation, ADR-0008) is already validated by the Tier 2 crash-recovery test (`e2e/cluster/crash_recovery.go`), which uses the same crash-adapter infrastructure. The only behavioral delta is _stale_ adapter (reported old generation) vs _absent_ adapter (never reported), which exercises the same aggregation code path. Automating this test would add ~180 lines of Disruptive/Serial infrastructure for minimal incremental coverage.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Deferred |
| **Version** | Post-MVP |
| **Created** | 2026-04-15 |
| **Updated** | 2026-05-11 |

---

### Preconditions

1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API and HyperFleet Sentinel services are deployed and running successfully
3. The adapters defined in testdata/adapter-configs are all deployed successfully
4. A dedicated crash-adapter is available for deployment via Helm

---

### Test Steps

#### Step 1: Deploy crash-adapter and create a cluster, wait for Reconciled at generation 1

**Action:**
- Deploy a dedicated crash-adapter via Helm (`${ADAPTER_DEPLOYMENT_NAME}`), separate from the normal adapters
- Configure API required adapters to include crash-adapter
- Create a cluster and wait for full convergence:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```

**Expected Result:**
- Cluster `Reconciled` condition `status: "True"` at `generation: 1`
- All adapters (including crash-adapter) report `observed_generation: 1`

#### Step 2: Scale down crash-adapter, then send PATCH request

**Action:**
- Scale the crash-adapter deployment to 0 replicas:
```bash
kubectl scale deployment/${ADAPTER_DEPLOYMENT_NAME} -n hyperfleet --replicas=0
```
- Wait for the crash-adapter pod to terminate
- Send PATCH to trigger generation increment:
```bash
curl -X PATCH ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id} \
  -H "Content-Type: application/json" \
  -d '{"spec": {"trigger-reconcile": "true"}}'
```

**Expected Result:**
- Response returns HTTP 200 with `generation: 2`
- crash-adapter cannot reconcile to generation 2 (it is unavailable)

#### Step 3: Verify Reconciled=False persists while crash-adapter is down

**Action:**
- Poll cluster GET repeatedly over multiple polling intervals while crash-adapter remains down:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```
- Poll adapter statuses:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses
```

**Expected Result:**
- Cluster `Reconciled` condition `status: "False"` persists over multiple polling cycles
- Healthy adapters report `observed_generation: 2` (they reconciled the update)
- crash-adapter either has no status entry or still reports `observed_generation: 1` (stale)

#### Step 4: Restore crash-adapter and verify full convergence

**Action:**
- Scale the crash-adapter back up:
```bash
kubectl scale deployment/${ADAPTER_DEPLOYMENT_NAME} -n hyperfleet --replicas=1
```
- Wait for crash-adapter to become ready:
```bash
kubectl rollout status deployment/${ADAPTER_DEPLOYMENT_NAME} -n hyperfleet --timeout=60s
```
- Poll until cluster reaches `Reconciled: True`:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- crash-adapter reconciles and reports `observed_generation: 2`
- All adapters report `observed_generation: 2`
- Cluster `Reconciled` condition transitions to `status: "True"` with `observed_generation: 2`
- Full state transition confirmed: `Reconciled: True (gen 1)` -> `Reconciled: False (gen 2 pending)` -> `Reconciled: True (gen 2)`

#### Step 5: Cleanup resources

**Action:**
- Restore API required adapters to original config
- Uninstall crash-adapter Helm release
- Clean up Pub/Sub subscription
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster and all associated resources are cleaned up
- crash-adapter deployment is removed

---

## Test Title: Multiple rapid updates coalesce to latest generation

### Description

This test validates that when multiple PATCH requests are sent in rapid succession, the system handles generation increments correctly and adapters eventually reconcile to the final generation. Intermediate generations may be skipped by adapters (coalesced), which is expected behavior since adapters reconcile the latest desired state.

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

#### Step 1: Create a cluster and wait for Reconciled at generation 1

**Action:**
- Create a cluster and wait for Reconciled:
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```

**Expected Result:**
- Cluster reaches `Reconciled: True` at `generation: 1`

#### Step 2: Send three PATCH requests in rapid succession

**Action:**
- Send three updates without waiting for reconciliation between them:
```bash
curl -X PATCH ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id} \
  -H "Content-Type: application/json" \
  -d '{"spec": {"update": "first"}}'
```
```bash
curl -X PATCH ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id} \
  -H "Content-Type: application/json" \
  -d '{"spec": {"update": "second"}}'
```
```bash
curl -X PATCH ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id} \
  -H "Content-Type: application/json" \
  -d '{"spec": {"update": "third"}}'
```

**Expected Result:**
- Each PATCH returns HTTP 200 with incrementing `generation` values: 2, 3, 4
- The final cluster state reflects the last update (`{"update": "third"}`)

#### Step 3: Wait for adapters to reconcile to the final generation

**Action:**
- Poll adapter statuses until all report the final generation:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses
```

**Expected Result:**
- All required adapters report `observed_generation: 4`
- Each adapter has `Applied: True`, `Available: True`, `Health: True`
- Adapters may skip intermediate generations (2, 3) and reconcile directly to generation 4 -- this is acceptable and expected behavior

#### Step 4: Verify cluster reaches Reconciled=True at final generation

**Action:**
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster `generation` equals 4
- Cluster `Reconciled` condition `status: "True"` with `observed_generation: 4`
- Cluster `Available` condition `status: "True"`
- Cluster spec contains `{"update": "third"}` (the last applied value)

#### Step 5: Cleanup resources

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster and all associated resources are cleaned up


## Test Title: Labels-only PATCH bumps generation and triggers reconciliation

### Description

This test validates that a PATCH request that only modifies `labels` (without changing `spec`) increments the cluster's `generation` and triggers adapter reconciliation. Generation is incremented when either `spec` or `labels` change. After a labels-only PATCH, `Reconciled` transitions to `False` (generation mismatch), adapters reconcile to the new generation, and `Reconciled` returns to `True`.

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
- All adapters report `observed_generation: 1`

#### Step 2: Send labels-only PATCH request

**Action:**
```bash
curl -X PATCH ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id} \
  -H "Content-Type: application/json" \
  -d '{"labels": {"env": "staging", "team": "fleet-management"}}'
```

**Expected Result:**
- Response returns HTTP 200 (OK)
- `generation` incremented from 1 to 2
- Labels in the response include the new values (`env: staging`, `team: fleet-management`)
- `spec` is unchanged from Step 1

#### Step 3: Verify adapters reconcile to the new generation

**Action:**
- Poll adapter statuses until all adapters report the new generation:
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses
```

**Expected Result:**
- All required adapters report `observed_generation: 2`
- Each adapter has `Applied: True`, `Available: True`, `Health: True`

#### Step 4: Verify cluster reaches Reconciled=True and Available=True at new generation

**Action:**
```bash
curl -X GET ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- `generation` equals 2
- `Reconciled` condition `status: "True"` with `observed_generation: 2`
- `Available` condition `status: "True"` with `observed_generation: 2`
- Labels reflect the PATCH update
- `spec` is unchanged

#### Step 5: Cleanup resources

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster and all associated resources are cleaned up

---

## Test Title: No-op PATCH does not increment generation

### Description

This test validates that a PATCH request with an identical spec does not increment the cluster's generation. The test captures the current spec, replays it via PATCH, and verifies the generation remains unchanged.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier1 |
| **Status** | Draft |
| **Automation** | Automated |
| **Version** | Post-MVP |
| **Created** | 2026-04-28 |
| **Updated** | 2026-04-28 |

---

### Preconditions

1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API and HyperFleet Sentinel services are deployed and running successfully
3. The adapters defined in testdata/adapter-configs are all deployed successfully

---

### Test Steps

#### Step 1: Create a cluster and wait for Reconciled state at generation 1

**Action:**
```bash
curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
  -H "Content-Type: application/json" \
  -d @testdata/payloads/clusters/cluster-request.json
```

**Expected Result:**
- Cluster reaches `Reconciled: True` at `generation: 1`

#### Step 2: PATCH with a spec change and verify generation increments

**Action:**
```bash
curl -i -X PATCH ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id} \
  -H "Content-Type: application/json" \
  -d '{"spec": {"update-trigger": "gen2"}}'
```

**Expected Result:**
- Response returns HTTP 200 (OK)
- `generation` equals `2`

#### Step 3: Replay the same spec via PATCH

**Action:**
- Send the same PATCH request as Step 2:
```bash
curl -i -X PATCH ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id} \
  -H "Content-Type: application/json" \
  -d '{"spec": {"update-trigger": "gen2"}}'
```

**Expected Result:**
- Response returns HTTP 200 (OK)
- `generation` remains `2` (unchanged)

#### Step 4: Cleanup resources

**Action:**
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```

**Expected Result:**
- Cluster and all associated resources are cleaned up

