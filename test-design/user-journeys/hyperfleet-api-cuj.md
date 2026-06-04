# HyperFleet API - Customer Critical Journey (MVP)
**Status**: Active
**Owner**: Ying Zhang
**Last Updated**: 2025-11-17

## Overview

This document maps the critical user journeys for **end users** interacting with HyperFleet API to create and manage multi-cloud clusters through a unified API.

Each journey includes user actions, system responses, and the architectural components involved in processing the request.

For partner-focused adapter customization journeys, see [HyperFleet Adapter CUJ](./hyperfleet-adapter-cuj.md).

## Persona

**Pain Points:**
- Managing multi-cloud clusters requires different tools/APIs for each provider which causes many efforts for engineers to manage

**Goals:**
- Stop doing bespoke architectures for every product, and define the CLM architecture to support multi-cloud clusters
- CLM extensibility through pipeline-based pluggable workflows to support deployments on any hyperscaler
---

## MVP Scope

### In Scope for MVP

**Hyperfleet API Happy Path Journeys (Success Scenarios):**
- [**Journey 1**: Create a New Cluster](#journey-1-create-a-new-cluster)
- [**Journey 2**: Monitor Cluster Status and Troubleshoot Issues](#journey-2-monitor-cluster-status-and-troubleshoot-issues)
- [**Journey 3**: List and Filter Clusters](#journey-3-list-and-filter-clusters)
- [**Journey 4**: Create a New NodePool](#journey-4-create-a-new-nodepool)
- [**Journey 5**: List and Filter NodePools](#journey-5-list-and-filter-nodepools)

**Hyperfleet API Sad Path Journeys (Failure Scenarios):**
- [**Journey 1 (Sad Path)**: Cluster Creation Failures](#journey-1-sad-path-cluster-creation-failures)
- [**Journey 2 (Sad Path)**: Monitoring and Troubleshooting Failures](#journey-2-sad-path-monitoring-and-troubleshooting-failures)
- [**Journey 3 (Sad Path)**: List and Filter Errors](#journey-3-sad-path-list-and-filter-errors)
- [**Journey 4 (Sad Path)**: NodePool Creation Failures](#journey-4-sad-path-nodepool-creation-failures)
- [**Journey 5 (Sad Path)**: List and Filter NodePools Errors](#journey-5-sad-path-list-and-filter-nodepools-errors)

---

# Cluster Journeys

## Prerequisites

For MVP phase, HyperFleet API supports the following configuration:

**Supported Cloud Providers:**
- **GCP (Google Cloud Platform)** - HCP (Hosted Control Plane) clusters only

**Requirements:**
- Valid API credentials/authentication token
- Access to GCP project with necessary permissions
- HyperFleet API endpoint URL

**Cluster Condition Values:**
- **`Reconciled=False`** - One or more adapters have not reported at the current generation
- **`Reconciled=True`** - All required adapters completed successfully

---

## Journey 1: Create a New Cluster

**User Action:**
- Create a cluster through HyperFleet API
```bash
POST /api/hyperfleet/v1/clusters
```

**Supported Configuration:**
- **Cluster Schema**: [Cluster Object](https://github.com/openshift-hyperfleet/hyperfleet-api-spec/blob/main/schemas/core/openapi.yaml)

**System Response / User Sees:**
- Cluster created with initial status `Reconciled=False`
- Cluster ID returned for tracking
- Cluster transitions to `Reconciled=True` when all adapters complete successfully
- Can monitor detailed adapter progress via `/clusters/{id}/statuses` endpoint

**Success Criteria:**
- Cluster reaches `Reconciled=True`
- All configures are set to corresponding resources
- All required adapters (validation, dns, infrastructure) show `Available: True`
- Zero node for cluster

---

## Journey 1 (Sad Path): Cluster Creation Failures

**User Action:**
- Attempt to create a cluster through HyperFleet API
```bash
POST /api/hyperfleet/v1/clusters
```

**Failure Scenarios:**

### Scenario 1: Invalid Configuration
**User Provides:**
- Invalid configuration (e.g., cluster name exceed the limit length of 63 characters)
- Missing required fields

**System Response / User Sees:**
- HTTP 400 Bad Request
- Clear error message indicating which field is invalid
- Example: `{"error": "Invalid region 'invalid-region' for provider GCP. Supported regions: us-east1, us-west1, ..."}`

### Scenario 2: Insufficient Permissions
**User Provides:**
- Valid cluster configuration
- API token with insufficient GCP project permissions

**System Response / User Sees:**
- Cluster created with status `Reconciled=False`
- Infrastructure adapter shows `Available: False`
- Error message: `{"reason": "PermissionDenied", "message": "Service account lacks compute.networks.create permission in GCP project"}`
- Can view detailed error via `/clusters/{id}/statuses` endpoint

### Scenario 3: Quota Exceeded
**User Provides:**
- Valid cluster configuration
- GCP project has reached quota limits

**System Response / User Sees:**
- Cluster created with status `Reconciled=False`
- Adapter shows `Available: False`
- Error message: `{"reason": "QuotaExceeded", "message": "GCP quota exceeded for resource: CPUS in region us-east1. Current: 100/100"}`
- Actionable guidance to request quota increase

### Scenario 4: Adapter Failure During Provisioning
**User Provides:**
- Valid cluster configuration

**System Response / User Sees:**
- Cluster created with initial status `Reconciled=False`
- DNS adapter fails after 5 minutes
- `/clusters/{id}/statuses` shows:
  - Validation adapter: `Available: True`
  - DNS adapter: `Available: False`, `Health: False`
  - Error message: `{"reason": "DNSZoneNotFound", "message": "Route53 hosted zone not found for domain example.com. Please create zone or update cluster spec."}`
- The following adapter: Not started (blocked by DNS adapter failure)

**Troubleshooting Actions:**
- User reviews error message via `/clusters/{id}/statuses`
- User creates missing DNS zone or updates cluster configuration
- System automatically retries adapter reconciliation
- Cluster transitions to `Reconciled=True` when issue resolved

**Success Criteria:**
- Clear, actionable error messages for all failure scenarios
- Ability to identify failing component via `/statuses` endpoint
- HTTP status codes correctly reflect error type (400 for validation, 201 + Reconciled=False for provisioning failures)

---

## Journey 2: Monitor Cluster Status and Troubleshoot Issues

**User Action:**
- Cluster resource with metadata and aggregated status
```bash
GET /api/hyperfleet/v1/clusters/{cluster_id}
```
- Detailed adapter statuses (ClusterStatus resource)
```bash
GET /api/hyperfleet/v1/clusters/{cluster_id}/statuses
```

**Supported Monitoring:**
- **High-level Status**: View cluster condition (Reconciled=False, Reconciled=True)
- **Detailed Status**: View individual adapter conditions (Available, Applied, Health)
- **Adapter Progress**: Track validation, dns ...... adapter execution
- **Error Details**: Access detailed error messages and failure reasons

**System Response / User Sees:**
- Cluster Reconciled condition indicates overall provisioning status
- A cluster starts as **Reconciled=False** and transitions to **Reconciled=True** when all required adapters complete successfully. It can transition back to **Reconciled=False** if the cluster generation changes or if adapters report failures.
- When `Reconciled=False`, can inspect `/statuses` endpoint to identify which adapter is blocking
- Each adapter shows three conditions:
  - **Available**: Work completed successfully (True = complete, False = failed/incomplete/in-progress)
  - **Applied**: Resources created successfully (True = created, False = failed/not-attempted)
  - **Health**: No unexpected errors (True = healthy, False = unexpected error)
  - **Rules for Additional Conditions**
    - **All conditions must be positive assertions**
      - GOOD: `DNSRecordsCreated` (status: True/False)
      - BAD: `DNSRecordsNotCreated` (confusing when status: False)
    - **Adapter aggregates all conditions to determine Available**
      - If any condition is False, Available should be False
      - If all conditions are True, Available should be True
- Clear error messages indicating actionable steps (e.g., "Route53 zone not found for domain example.com")
- Detailed timing information (lastTransitionTime for each condition)
- Condition Transitions: A cluster starts as **Reconciled=False** and transitions to **Reconciled=True** when all required adapters complete successfully. It can transition back to **Reconciled=False** if the cluster generation changes or if adapters report failures.

**Success Criteria:**
- Can identify failing adapter within seconds
- Clear visibility into provisioning progress

---

## Journey 2 (Sad Path): Monitoring and Troubleshooting Failures

**User Action:**
- Monitor cluster that is experiencing issues
```bash
GET /api/hyperfleet/v1/clusters/{cluster_id}
GET /api/hyperfleet/v1/clusters/{cluster_id}/statuses
```

**Failure Scenarios:**

### Scenario 1: Cluster Stuck in Reconciled=False
**User Observes:**
- Cluster condition remains `Reconciled=False` for extended period (>30 minutes)
- Polling `/clusters/{id}/statuses` to diagnose

**System Response / User Sees:**
- The broken-down adapter shows `Available: False`, `Applied: False`
- Error message: `{"reason": "Timeout", "message": "GCP cluster creation timed out after 25 minutes. Last status: Reconciled=False"}`
- `lastTransitionTime` shows when adapter last attempted reconciliation
- Clear indication that manual intervention may be required

### Scenario 2: Cluster Resource Not Found
**User Action:**
- Attempt to get cluster details for non-existent cluster ID
```bash
GET /api/hyperfleet/v1/clusters/non-existent-id
```

**System Response / User Sees:**
- HTTP 404 Not Found
- Error message: `{"error": "Cluster not found", "id": "non-existent-id"}`

### Scenario 3: Adapter Reporting Degraded State
**User Observes:**
- Cluster is `Reconciled=True` but health monitoring detects issues

**System Response / User Sees:**
- `/clusters/{id}/statuses` shows:
  - DNS adapter: `Available: True`, `Applied: True`, `Health: False`
  - Error message: `{"reason": "HealthCheckFailed", "message": "DNS records exist but health check failing. Record set may be misconfigured."}`
- Cluster remains operational but user alerted to potential issues

**Success Criteria:**
- Timeout scenarios clearly communicated with actionable next steps
- Health vs availability clearly distinguished
- Appropriate HTTP status codes for different error types

---

## Journey 3: List and Filter Clusters

**User Action:**
- Get one specified cluster
```bash
GET /api/hyperfleet/v1/clusters/{cluster_id}
```
- Get all clusters
```bash
GET /api/hyperfleet/v1/clusters
```
- Filter clusters
```bash
# Filter by condition
GET /api/hyperfleet/v1/clusters?status.conditions.Reconciled='False'

# Filter by provider
GET /api/hyperfleet/v1/clusters?provider=gcp

# Combine multiple filters
GET /api/hyperfleet/v1/clusters?status.conditions.Reconciled='True'&provider=gcp
```

**Supported Configuration:**
- **Filter by attributes**: Like status.conditions,provider,region,name......
- **Combine Multiple Filters**: Mix conditions, labels, provider, region, and name

**System Response / User Sees:**
- List response includes pagination fields: page, size, total, items
- Each cluster shows id, name, spec (provider, region), status (conditions), labels, timestamps
- Flexible filtering enables quick cluster location
- Supports complex operational queries (e.g., "all production GCP clusters in us-east1 that are Reconciled")
- Team-based isolation and multi-tenancy support through label filtering

**Success Criteria:**
- Can quickly locate specific clusters using filters
- Support operational queries without manual searching
- Enable team-based cluster management through labels

---

## Journey 3 (Sad Path): List and Filter Errors

**User Action:**
- Attempt to list or filter clusters with invalid parameters
```bash
GET /api/hyperfleet/v1/clusters?phase=InvalidPhase
GET /api/hyperfleet/v1/clusters?provider=unsupported-provider
```

**Failure Scenarios:**

### Scenario 1: Invalid Filter Parameters
**User Provides:**
- Invalid phase value (e.g., `phase=InvalidPhase`)

**System Response / User Sees:**
- HTTP 400 Bad Request
- Error message: `{"error": "Invalid filter. Use status.conditions.Reconciled='True' or status.conditions.Reconciled='False'"}`

### Scenario 2: No Results Found
**User Provides:**
- Valid filter that matches no clusters
```bash
GET /api/hyperfleet/v1/clusters?status.conditions.Reconciled='True'
```

**System Response / User Sees:**
- HTTP 200 OK
- Empty results list:
```json
{
  "page": 1,
  "size": 0,
  "total": 0,
  "items": []
}
```

### Scenario 3: Authentication/Authorization Failure
**User Provides:**
- Invalid or expired authentication token
- Token without sufficient permissions

**System Response / User Sees:**
- For insufficient permissions:
  - Request accepted (HTTP 200 OK)
  - Validation adapter checks permissions and reports failure
  - Can view detailed error via `/clusters/{id}/statuses` showing clusters with validation failures
  - Error indicated through validation adapter status: `Available: False`
  - Error message: `{"reason": "PermissionDenied", "message": "Insufficient permissions to access clusters with label 'team:platform'"}`

**Success Criteria:**
- Invalid filter parameters rejected with clear error messages
- Empty results handled gracefully with proper pagination structure
- Authentication/authorization errors clearly distinguished
- All error responses follow consistent format

---

# NodePool Journeys

**Note on NodePool Condition Values:**
NodePools use the same condition values as clusters:
- **`Reconciled=False`** - One or more adapters have not reported at the current generation
- **`Reconciled=True`** - All required adapters completed successfully

## Journey 4: Create a New NodePool

**User Action:**
- Create a nodepool for a cluster
```bash
POST /api/hyperfleet/v1/clusters/{cluster_id}/nodepools
```
- Monitor nodepool status
```bash
GET /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/{nodepool_id}
```

**Supported Configuration:**
- **Node Count**: Number of nodes to provision (e.g., 5)
- **Machine Type**: Instance type (e.g., n1-standard-4, n1-highmem-8)
- **Labels**: Custom key-value pairs (workload:compute, team:platform, etc.)

**System Response / User Sees:**
- NodePool created with initial status `Reconciled=False` (HTTP 201 Created)
- NodePool ID returned for tracking
- NodePool transitions to `Reconciled=True` when all adapters complete successfully
- Can poll nodepool status to monitor provisioning progress
- Adapters provision compute nodes in the cluster
- NodePool provisioning completed when all nodes are ready

**Success Criteria:**
- NodePool reaches `Reconciled=True`
- All required adapters show `Available: True`
- Specified number of nodes provisioned and available in cluster

---

## Journey 4 (Sad Path): NodePool Creation Failures

**User Action:**
- Attempt to create a nodepool through HyperFleet API
```bash
POST /api/hyperfleet/v1/clusters/{cluster_id}/nodepools
```

**Failure Scenarios:**

### Scenario 1: Invalid Node Count
**User Provides:**
- Node count of 0 or negative value
- Node count exceeding maximum limit

**System Response / User Sees:**
- HTTP 400 Bad Request
- Error message: `{"error": "Invalid node count: 0. Must be between 1 and 100"}`
- Or: `{"error": "Node count 150 exceeds maximum limit of 100 per nodepool"}`

### Scenario 2: Quota Exceeded During Provisioning
**User Provides:**
- Valid nodepool configuration
- GCP project quota insufficient for requested nodes

**System Response / User Sees:**
- NodePool created with status `Reconciled=False` (HTTP 201 Created)
- NodePool adapter shows `Available: False`
- Error message via `/clusters/{cluster_id}/nodepools/{id}/statuses`:
  - `{"reason": "QuotaExceeded", "message": "Insufficient quota to provision 5 nodes of type n1-standard-4. Required: 20 CPUs, Available: 10 CPUs"}`
- Actionable guidance to reduce node count or request quota increase

### Scenario 3: Adapter Failure During Node Provisioning
**User Provides:**
- Valid nodepool configuration

**System Response / User Sees:**
- NodePool created with initial status `Reconciled=False`
- Nodepool adapter fails during node provisioning
- `/clusters/{cluster_id}/nodepools/{id}/statuses` shows:
  - `Available: False`, `Applied: False`
  - Error message: `{"reason": "ProvisioningFailed", "message": "Failed to provision node 3 of 5. GCP error: ZONE_RESOURCE_POOL_EXHAUSTED"}`

**Success Criteria:**
- Provisioning failures clearly indicate which nodes failed and why
- Clear guidance on resolution steps
---

## Journey 5: List and Filter NodePools

**User Action:**
- List all nodepools for a cluster
```bash
GET /api/hyperfleet/v1/clusters/{cluster_id}/nodepools
```
- Filter nodepools
```bash
# Filter by labels
GET /api/hyperfleet/v1/clusters/{cluster_id}/nodepools?labels=workload:gpu

# Filter by condition
GET /api/hyperfleet/v1/clusters/{cluster_id}/nodepools?status.conditions.Reconciled='True'

# Combine multiple filters
GET /api/hyperfleet/v1/clusters/{cluster_id}/nodepools?labels=workload:gpu&status.conditions.Reconciled='True'
```

**Supported Configuration:**
- **Filter by Condition**: Reconciled=True, Reconciled=False
- **Filter by Labels**: Custom key-value pairs (workload:gpu, team:ml, etc.)
- **Combine Multiple Filters**: Mix conditions and labels

**System Response / User Sees:**
- List response includes pagination fields: page, size, total, items
- Each nodepool shows id, cluster_id, spec (nodeCount, machineType), status (conditions), labels, timestamps
- Flexible filtering enables quick nodepool location
- Support for workload-specific queries (e.g., "all GPU nodepools that are Reconciled")
- Team-based nodepool management through label filtering

**Success Criteria:**
- Can quickly locate specific nodepools using filters
- Support operational queries for nodepool management
- Enable workload-based nodepool organization

---

## Journey 5 (Sad Path): List and Filter NodePools Errors

**User Action:**
- Attempt to list or filter nodepools with invalid parameters
```bash
GET /api/hyperfleet/v1/clusters/{cluster_id}/nodepools?phase=InvalidPhase
GET /api/hyperfleet/v1/clusters/invalid-cluster-id/nodepools
```

**Failure Scenarios:**

### Scenario 1: Parent Cluster Not Found
**User Provides:**
- Valid filter parameters
- Non-existent cluster ID

**System Response / User Sees:**
- HTTP 404 Not Found
- Error message: `{"error": "Cluster not found", "cluster_id": "non-existent-cluster"}`

### Scenario 2: Invalid Filter Parameters
**User Provides:**
- Invalid filter value (e.g., unsupported condition query)
- Invalid label filter format

**System Response / User Sees:**
- HTTP 400 Bad Request
- Error message: `{"error": "Invalid filter. Use status.conditions.Reconciled='True' or status.conditions.Reconciled='False'"}`
- Or: `{"error": "Invalid label filter format. Expected format: key:value"}`

### Scenario 3: No NodePools Found
**User Provides:**
- Valid filter that matches no nodepools
```bash
GET /api/hyperfleet/v1/clusters/{cluster_id}/nodepools?labels=workload:nonexistent
```

**System Response / User Sees:**
- HTTP 200 OK
- Empty results list:
```json
{
  "page": 1,
  "size": 0,
  "total": 0,
  "items": []
}
```

### Scenario 4: NodePool Not Found (Single Get)
**User Action:**
- Attempt to get specific nodepool that doesn't exist
```bash
GET /api/hyperfleet/v1/clusters/{cluster_id}/nodepools/non-existent-nodepool-id
```

**System Response / User Sees:**
- HTTP 404 Not Found
- Error message: `{"error": "NodePool not found", "nodepool_id": "non-existent-nodepool-id", "cluster_id": "cluster-123"}`

### Scenario 5: Authentication/Authorization Failure
**User Provides:**
- Invalid or expired authentication token
- Token without permissions to view nodepools in cluster

**System Response / User Sees:**
- For insufficient permissions:
  - Request accepted (HTTP 200 OK)
  - Validation adapter checks permissions and reports failure
  - Can view detailed error via `/clusters/{cluster_id}/nodepools/{id}/statuses` showing nodepools with validation failures
  - Error indicated through validation adapter status: `Available: False`
  - Error message: `{"reason": "PermissionDenied", "message": "Insufficient permissions to access nodepools in cluster"}`

**Success Criteria:**
- Parent cluster validation occurs before nodepool filtering
- Invalid filter parameters rejected with clear error messages
- Empty results handled gracefully with proper pagination structure
- Authentication/authorization errors clearly distinguished
- Consistent error format across all endpoints

---

## API Endpoints Summary

### Cluster Endpoints
- `POST /clusters` - Create a new cluster
- `GET /clusters` - List all clusters (with filtering support)
- `GET /clusters/{id}` - Get cluster details and high-level status
- `GET /clusters/{id}/statuses` - Get detailed adapter status information

### NodePool Endpoints
- `POST /clusters/{cluster_id}/nodepools` - Create a new nodepool
- `GET /clusters/{cluster_id}/nodepools` - List all nodepools (with filtering support)
- `GET /clusters/{cluster_id}/nodepools/{id}` - Get nodepool details and high-level status
- `GET /clusters/{cluster_id}/nodepools/{id}/statuses` - Get detailed adapter status information

