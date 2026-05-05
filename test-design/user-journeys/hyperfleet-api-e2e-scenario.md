# HyperFleet API E2E Scenarios
**Status**: Active
**Owner**: Ying Zhang
**Last Updated**: 2025-11-18
---

## API Endpoints Summary (MVP)

### Cluster Endpoints
- `POST /clusters` - Create a new cluster
- `GET /clusters` - List all clusters (with filtering support)
- `GET /clusters/{id}` - Get cluster resource with aggregated status
- `GET /clusters/{id}/statuses` - Get detailed adapter status statuses

### NodePool Endpoints
- `POST /clusters/{cluster_id}/nodepools` - Create a new nodepool
- `GET /clusters/{cluster_id}/nodepools` - List all nodepools (with filtering support)
- `GET /clusters/{cluster_id}/nodepools/{id}` - Get nodepool details with aggregated status
- `GET /clusters/{cluster_id}/nodepools/{id}/statuses` - Get detailed adapter status statuses

---

## 1. MVP-Critical E2E Test Scenarios (Happy Path)

### Part 1: Cluster Lifecycle

### E2E-001: Full Cluster Creation Flow on GCP

**Endpoint**: `POST /api/hyperfleet/v1/clusters`

**Objective**: Validate end-to-end cluster creation from API request to Reconciled state on GCP.

**Test Steps**:
1. Submit cluster creation request via `POST /api/hyperfleet/v1/clusters`
   - **Note**: The detailed request body spec is still evolving and subject to change
   - Provider: GCP
   - Region: us-east1
   - Labels: {environment: "test", team: "platform"}

2. Verify API response
   - HTTP 201 Created
   - Cluster ID generated
   - status.conditions.Reconciled = "False"
   - status.adapters = [] (no adapters reported yet)
   - status.lastUpdated set
   - generation = 1

3. Monitor cluster status via `GET /api/hyperfleet/v1/clusters/{id}`
   - Verify Reconciled remains "False" until all adapters complete
   - Monitor status.adapters array as adapters report their status

4. Monitor adapter statuses via `GET /api/hyperfleet/v1/clusters/{id}/statuses`
   - This returns ONE ClusterStatus object containing all adapter statuses
   - Verify ClusterStatus.adapterStatuses array is populated by each adapter
   - Each adapter reports conditions: Available, Applied, Health
   - All Adapters conditions:
     - Available: False (JobRunning) → True (JobSucceeded)
     - Applied: True (JobLaunched)
     - Health: True (NoErrors)
     - Data(optional):{...}
5. Verify final state
   - Cluster status.conditions.Reconciled = "True"
   - Cluster status.adapters shows all adapters with:
     - name: adapter name
     - available: "True"
     - observedGeneration: 1 (matches cluster.generation)
   - ClusterStatus.adapterStatuses array contains all adapter statuses
   - All adapters have Available condition = True
   - Cluster API and console are accessible and functional

**Expected Duration**: Average time

**Success Criteria**:
- Cluster transitions to Reconciled=True
- All adapters complete successfully
- No errors in logs (API, Sentinel, Adapters, Jobs)
- Kubernetes Jobs complete successfully

---

### E2E-002: Cluster Configuration Update (Post-MVP)

**Endpoint**: `PATCH /api/hyperfleet/v1/clusters/{id}`

**Objective**: Validate cluster spec update triggers reconciliation and completes successfully.

**Scope**: Update cluster configuration, verify generation increments, verify all adapters reconcile changes, and cluster status.

---

### E2E-003: Cluster Deletion (Post-MVP)

**Endpoint**: `DELETE /api/hyperfleet/v1/clusters/{id}`

**Objective**: Validate complete cluster deletion and resource cleanup.

**Scope**: Delete cluster, verify adapters execute cleanup, verify cluster and all associated resources are fully deleted with no orphaned resources.

---

### Part 2: Nodepool Lifecycle

### E2E-004: Full Nodepool Creation Flow

**Endpoint**: `POST /api/hyperfleet/v1/clusters/{cluster_id}/nodepools`

**Objective**: Validate end-to-end nodepool creation from API request to Reconciled state for an existing cluster.

**Test Steps**:
1. Prerequisites: Create cluster via E2E-001 and wait for Reconciled=True
2. Submit nodepool creation request via `POST /api/hyperfleet/v1/clusters/{cluster_id}/nodepools`
   - Name: "gpu-nodepool"
   - MachineType: "n1-standard-8"
   - Replicas: 2
   - Labels: {workload: "gpu", tier: "compute"}

3. Verify API response
   - HTTP 201 Created
   - Nodepool ID generated
   - status.conditions.Reconciled = "False"
   - status.adapters = [] (no adapters reported yet)
   - status.lastUpdated set
   - generation = 1

4. Verify nodepool appears in list via `GET /api/hyperfleet/v1/clusters/{cluster_id}/nodepools`
   - Nodepool included in response
   - Can filter by labels

5. Monitor nodepool status via `GET /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{id}`
   - Verify Reconciled remains "False" until all adapters complete
   - Monitor status.adapters array as adapters report their status

6. Monitor adapter statuses via `GET /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{id}/statuses`
   - This returns ONE NodepoolStatus object containing all adapter statuses
   - Verify NodepoolStatus.adapterStatuses array is populated by each adapter
   - Each adapter reports conditions: Available, Applied, Health
   - Validation Adapter conditions:
     - Available: False (JobRunning) → True (JobSucceeded)
     - Applied: True (JobLaunched)
     - Health: True (NoErrors)
   - Nodepool Adapter conditions:
     - Available: False (JobRunning) → True (JobSucceeded)
     - Applied: True (JobLaunched)
     - Health: True (NoErrors)

7. Verify final state
   - Nodepool status.conditions.Reconciled = "True"
   - Nodepool status.adapters shows all adapters with:
     - name: adapter name
     - available: "True"
     - observedGeneration: 1 (matches nodepool.generation)
   - NodepoolStatus.adapterStatuses array contains all adapter statuses
   - All adapters have Available condition = True
   - Nodepool nodes are running and joined to cluster

**Expected Duration**: Average time

**Success Criteria**:
- Nodepool transitions to Reconciled=True
- All adapters complete successfully
- Nodes are created and healthy in the cluster
- No errors in logs (API, Sentinel, Adapters, Jobs)
- Kubernetes Jobs complete successfully

---

### E2E-005: Nodepool Configuration Update (Post-MVP)

**Endpoint**: `PATCH /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{id}`

**Objective**: Validate nodepool spec update triggers reconciliation and completes successfully.

**Scope**: Update nodepool configuration (e.g., replica count), verify generation increments, verify all adapters reconcile changes, and nodepool returns to Reconciled=True with correct node count.

---

### E2E-006: Nodepool Deletion (Post-MVP)

**Endpoint**: `DELETE /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{id}`

**Objective**: Validate complete nodepool deletion and resource cleanup.

**Scope**: Delete nodepool, verify adapters execute cleanup, verify nodepool and all associated resources are fully deleted with no orphaned nodes or Kubernetes resources.

---

## 2. Failure Scenario Tests

### E2E-FAIL-001: Cluster API Request Body Validation Failures

**Endpoint**: `POST /api/hyperfleet/v1/clusters`

**Objective**: Validate API properly validates cluster creation request body and rejects invalid requests with clear error messages.

**Test Steps**:
1.. Test invalid field values:
   - Test Scenarios like:
      - Cluster creation payload without required fileds like name
      - Cluster creation payload with un-supported filed
      - Cluster creation payload with invalid value like region: 123 (should be string)
      - Cluster creation payload with exsited cluster name 
      - Send request with invalid JSON syntax
   - Verify HTTP 400 Bad Request
   - Verify error message: "provider must be one of: GCP, AWS, Azure"

**Success Criteria**:
- API returns HTTP 400 for validation errors
- Error messages are clear and indicate which field failed validation
- Error messages indicate the expected format/values
- No resources created in database when validation fails
- API doesn't crash or return 500 errors for validation failures
- Validation happens before any adapter processing begins

---

### E2E-FAIL-002: Nodepool API Request Body Validation Failures

**Endpoint**: `POST /api/hyperfleet/v1/clusters/{cluster_id}/nodepools`

**Objective**: Validate API properly validates nodepool creation request body and rejects invalid requests with clear error messages.

**Test Steps**:
1. Test invalid field values:
   - Test Scenarios like:
     - Nodepool creation payload without required fileds like name
     - Nodepool creation payload with Replicas: -1 (should be positive integer)
     - Nodepool creation payload MachineType: "" (empty string)
     - Send request with invalid JSON syntax
   - Verify HTTP 400 Bad Request
   - Verify error message
2. Test invalid cluster reference:
   - cluster_id: "non-existent-cluster-id"
   - Verify HTTP 404 Not Found
   - Verify error message

**Success Criteria**:
- API returns HTTP 400 for validation errors
- API returns HTTP 404 for non-existent cluster
- Error messages are clear and indicate which field failed validation
- Error messages indicate the expected format/values
- No resources created in database when validation fails
- API doesn't crash or return 500 errors for validation failures
- Validation happens before any adapter processing begins

---

### E2E-FAIL-003: Adapter Failed (Business Logic)

**Objective**: Validate system handles validation failures with proper status reporting, distinguishing from adapter health issues.

**Test Steps**:
1. Create cluster with missing prerequisite (e.g., Route53 zone not configured for specified domain)
2. Submit cluster creation request via `POST /api/hyperfleet/v1/clusters`
3. Monitor adapter status via `GET /api/hyperfleet/v1/clusters/{id}/statuses`
   - Verify Validation Adapter conditions in ClusterStatus.adapterStatuses:
     - Available: False (reason: "ValidationFailed", message: "Reasonable reason for failure (validation logic)")")
     - Applied: True (reason: "JobLaunched", message: "Kubernetes Job created successfully")
     - Health: True (reason: "NoErrors", message: "Adapter executed normally (validation logic failed, not adapter error)")
   - Verify cluster status.conditions.Reconciled = "False"
   - Verify cluster status.adapters shows validation adapter with available: "False"
4. Verify data field contains detailed validation results

**Success Criteria**:
- Validation failure reported with Health: True (business logic failure, not adapter error)
- Available: False indicates work incomplete
- Detailed validation results in data field
- Clear distinction between business logic failures and adapter health issues

---

### E2E-FAIL-004: Adapter Failed (Unexpected Error)

**Objective**: Validate system handles adapter unexpected errors with proper status reporting, distinguishing between Job creation failures (Applied: False) and Job execution failures (Applied: True, Health: False).

**Test Steps - Scenario A: Job Creation Failure (Applied: False)**:
1. Configure adapter with invalid YAML that references unknown Kubernetes custom resource
   - Example: Use invalid adapter-business YAML file that references a CR (e.g., Crossplane CR) that the Kubernetes cluster does not recognize
   - Example: Reference non-existent CRD in Job specification
2. Create cluster via `POST /api/hyperfleet/v1/clusters`
3. Monitor adapter status via `GET /api/hyperfleet/v1/clusters/{id}/statuses`
   - Verify adapter cannot create Job due to unknown resource
   - Verify adapter conditions in ClusterStatus.adapterStatuses:
     - Available: False (reason: "ResourceCreationFailed", message: "Failed to create Job: unknown custom resource")
     - Applied: False 
     - Health: False (reason: "UnexpectedError", message: "Kubernetes API rejected Job creation")
   - Verify cluster status.conditions.Reconciled = "False"
   - Verify cluster status.adapters shows adapter with available: "False"
4. Verify data field contains detailed error information

**Test Steps - Scenario B: Job Execution Failure (Applied: True, Health: False)**:
1. Configure adapter with incorrect parameter that causes Job to fail during execution
   - Example: Provide invalid credentials or configuration that causes Job container to exit with non-zero code
   - Example: Pass incorrect API endpoint or malformed parameter
2. Create cluster via `POST /api/hyperfleet/v1/clusters`
3. Monitor adapter status via `GET /api/hyperfleet/v1/clusters/{id}/statuses`
   - Verify adapter creates Job successfully
   - Verify Job runs but exits with failure (exit code -1 or non-zero)
   - Verify adapter conditions in ClusterStatus.adapterStatuses:
     - Available: False (reason: "JobFailed", message: "Job completed with errors")
     - Applied: True (reason: "JobCreated", message: "Kubernetes Job created successfully")
     - Health: False (reason: "JobExecutionFailed", message: "Job container exited with code -1")
   - Verify cluster status.conditions.Reconciled = "False"
   - Verify cluster status.adapters shows adapter with available: "False"
4. Verify data field contains detailed error information

**Success Criteria**:
- Scenario A (Applied: False): Job creation failure detected and reported
  - Applied: False indicates Job was NOT created
  - Health: False indicates infrastructure/configuration error
  - Clear error showing which resource/CRD is missing
- Scenario B (Applied: True, Health: False): Job execution failure detected and reported
  - Applied: True indicates Job was created successfully
  - Health: False indicates Job execution failed
  - Available: False indicates work did not complete successfully
  - Exit code and container logs captured in error details
- Clear distinction between creation failures vs. execution failures
- Detailed error information in data field with error type and context

---

### E2E-FAIL-005: Adapter Precondition Not Met

**Objective**: Validate adapter correctly skips execution when preconditions not met.

**Test Steps**:
1. Create cluster
2. Monitor HyperShift Adapter (depends on DNS, Placement completing)
3. Simulate DNS Adapter stuck in "Running" phase
   - Make DNS resources can't be created correctly
4. Verify HyperShift Adapter(behind DNS adpater) behavior
   - Consumes event from broker
   - Evaluates preconditions
   - Preconditions not met (DNS not Complete)
   - Does NOT create Job
   - Acknowledges message
5. Complete DNS Adapter
   - Create the required DNS resource manually
6. Verify HyperShift Adapter processes next event
   - Preconditions now met
   - Job created and executed

**Success Criteria**:
- Adapter correctly evaluates preconditions
- No Job created when preconditions not met
- Adapter processes event when preconditions met
- No deadlocks or stuck states

---

### E2E-FAIL-006: Database Connection Failure

**Objective**: Validate API handles database connection failures gracefully.

**Test Steps**:
1. Simulate database connection failure (stop PostgreSQL)
2. Attempt cluster operations via API
   - GET /clusters (should return 503)
   - POST /clusters (should return 503)
3. Verify API error responses
   - HTTP 503 Service Unavailable
   - Appropriate error messages
4. Restore database connection
5. Verify API operations resume normally
6. Create cluster and verify success

**Success Criteria**:
- API returns 503 errors during outage
- API doesn't crash
- Operations resume after recovery
- No data corruption

---

### E2E-FAIL-007: Cluster Sentinel Operator Crash and Recovery

**Objective**: Validate system continues functioning after Sentinel restarts.

**Test Steps**:
1. Create 2 clusters (both in progress, not Reconciled)
2. Kill Sentinel Operator pod
3. Monitor cluster progress
   - Verify no new events published during Sentinel downtime
   - Verify adapters continue processing existing events
4. Kubernetes restarts Sentinel pod
5. Monitor Sentinel recovery
   - Sentinel resumes polling API
   - Sentinel publishes events for both clusters
6. Verify both clusters eventually reach Reconciled=True

**Success Criteria**:
- Clusters continue progressing during downtime
- Sentinel recovers automatically (Kubernetes restart)
- No events lost
- Clusters complete successfully

---

### E2E-FAIL-008: Nodepool Sentinel Operator Crash and Recovery (Post-MVP)

**Objective**: Validate system continues functioning for nodepool operations after Sentinel restarts.

**Scope**: Create multiple nodepools, kill Sentinel pod, verify nodepools continue progressing, verify Sentinel recovers automatically, and both nodepools complete successfully.

---
