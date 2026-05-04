# Feature: Concurrent Processing

## Table of Contents

1. [System can process concurrent cluster creations without resource conflicts](#test-title-system-can-process-concurrent-cluster-creations-without-resource-conflicts)
2. [Multiple nodepools can coexist under same cluster without conflicts](#test-title-multiple-nodepools-can-coexist-under-same-cluster-without-conflicts)

---

## Test Title: System can process concurrent cluster creations without resource conflicts

### Description

This test validates that the system can handle multiple cluster creation requests submitted simultaneously without resource conflicts or processing failures. It ensures that all clusters are correctly processed and reach their expected final state.

---

| **Field** | **Value** |
|-----------|-----------|
| **Pos/Neg** | Positive |
| **Priority** | Tier1 |
| **Status** | Automated |
| **Automation** | Automated |
| **Version** | MVP |
| **Created** | 2026-02-11 |
| **Updated** | 2026-03-20 |


---

### Preconditions
1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API, Sentinel, and Adapter services are deployed and running successfully
3. The adapters defined in testdata/adapter-configs are all deployed successfully

---

### Test Steps

#### Step 1: Submit 5 cluster creation requests simultaneously
**Action:**
- Submit 5 POST requests in parallel (each call generates a unique name via `{{.Random}}` template):
```bash
for i in $(seq 1 5); do
  curl -X POST ${API_URL}/api/hyperfleet/v1/clusters \
    -H "Content-Type: application/json" \
    -d @testdata/payloads/clusters/cluster-request.json &
done
wait
```

**Expected Result:**
- All 5 requests return successful responses (HTTP 200/201)
- Each response contains a unique cluster ID
- No request is rejected or fails due to concurrency

#### Step 2: Wait for all clusters to be processed
**Action:**
- For each cluster created in Step 1, poll its status until Reconciled state or a timeout is reached:
```bash
curl -s ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id} | jq '.conditions'
```

**Expected Result:**
- All 5 clusters eventually reach Reconciled=True and Available=True
- No cluster is stuck in a pending or processing state indefinitely

#### Step 3: Verify Kubernetes resources for all clusters
**Action:**
- For each cluster created in Step 1, check that it has its own namespace and expected resources:
```bash
kubectl get namespace {cluster_id}
kubectl get jobs -n {cluster_id}
```

**Expected Result:**
- 5 separate namespaces exist (one per cluster)
- Each namespace contains the expected jobs/resources created by adapters
- No cross-contamination between clusters (resources are isolated)

#### Step 4: Verify adapter statuses for all clusters
**Action:**
- For each cluster created in Step 1, check that it has complete adapter status reports:
```bash
curl -s ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/statuses | jq '.items | length'
```

**Expected Result:**
- Each cluster has the expected number of adapter status entries
- All adapters report Applied=True, Available=True, Health=True for each cluster
- No missing status reports

#### Step 5: Cleanup resources
**Action:**
- For each cluster created in Step 1, delete the cluster via the API:
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```
- Wait for hard-delete to complete (cluster returns 404)
- If cleanup fails, fall back to namespace deletion:
```bash
kubectl delete namespace {cluster_id} --ignore-not-found
```

**Expected Result:**
- All clusters and associated resources are cleaned up

---

## Test Title: Multiple nodepools can coexist under same cluster without conflicts

### Description

This test validates that multiple nodepools can be created under the same cluster and coexist without conflicts. It verifies that each nodepool is processed independently by the adapters, has its own set of Kubernetes resources, and reports its own status without interfering with other nodepools.

---

| **Field** | **Value**     |
|-----------|---------------|
| **Pos/Neg** | Positive      |
| **Priority** | Tier1         |
| **Status** | Automated     |
| **Automation** | Automated     |
| **Version** | MVP           |
| **Created** | 2026-02-11    |
| **Updated** | 2026-03-24    |


---

### Preconditions

1. Environment is prepared using [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) with all required platform resources
2. HyperFleet API and HyperFleet Sentinel services are deployed and running successfully
3. The adapters defined in testdata/adapter-configs are all deployed successfully
4. A cluster resource has been created and its cluster_id is available
    - **Cleanup**: Cluster resource cleanup should be handled in test suite teardown where cluster was created

---

### Test Steps

#### Step 1: Create multiple nodepools under the same cluster
**Action:**
- Submit 3 POST requests in parallel to create NodePool resources (each call generates a unique name via `{{.Random}}` template):
```bash
for i in 1 2 3; do
  curl -X POST ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools \
    -H "Content-Type: application/json" \
    -d @testdata/payloads/nodepools/nodepool-request.json &
done
wait
```

**Expected Result:**
- All 3 nodepools are created successfully
- Each returns a unique nodepool ID

#### Step 2: Verify all nodepools appear in the list
**Action:**
```bash
curl -s ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools | jq '.items | length'
```

**Expected Result:**
- List contains all 3 nodepools
- Each nodepool has a distinct ID and name

#### Step 3: Verify each nodepool reaches Reconciled state independently
**Action:**
- For each nodepool created in Step 1, check its conditions:
```bash
curl -s ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id} | jq '.conditions'
```

**Expected Result:**
- All 3 nodepools eventually reach Reconciled=True and Available=True
- Each nodepool's adapter status is independent (one nodepool's failure does not block others)

#### Step 4: Verify Kubernetes resources are isolated per nodepool
**Action:**
- Check that each nodepool has its own set of resources:
```bash
kubectl get configmaps -n {cluster_id} -l hyperfleet.io/nodepool-id
```

**Expected Result:**
- Each nodepool's resources are labeled/named distinctly
- No resource name collisions between nodepools
- Resources for one nodepool do not overwrite resources of another

#### Step 5: Cleanup resources

**Action:**
- For each nodepool created in Step 1, delete the nodepool via the API:
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}
```
- After all nodepools are deleted, delete the cluster via the API:
```bash
curl -X DELETE ${API_URL}/api/hyperfleet/v1/clusters/{cluster_id}
```
- Wait for hard-delete to complete
- If cleanup fails, fall back to namespace deletion:
```bash
kubectl delete namespace {cluster_id} --ignore-not-found
```

**Expected Result:**
- Nodepools, cluster, and all associated resources are cleaned up

---
