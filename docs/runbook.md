# HyperFleet E2E Test Runbook

> **Audience:** Developers running E2E tests locally

This runbook provides step-by-step instructions for running and troubleshooting HyperFleet E2E tests in a local development environment.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Running E2E Tests](#running-e2e-tests)
- [Troubleshooting](#troubleshooting)
- [CI Jobs](#ci-jobs)

## Prerequisites

**Environment Setup:**

Before running tests, you need a running HyperFleet environment. See the [Setup Guide](setup.md) for complete instructions on deploying HyperFleet using:

- **Kind (local):** Fast setup, no cloud dependencies, uses port-forwarding
- **GCP:** Cloud environment, requires GCP access, uses LoadBalancer services

The environment guide covers:
- Tool installation and verification
- HyperFleet deployment (Kind or GCP)
- Port-forwarding / LoadBalancer setup
- Environment variable configuration
- Optional image settings override

**Required environment variables** (set during environment setup):

- `HYPERFLEET_API_URL` - HyperFleet API endpoint
- `MAESTRO_URL` - Maestro API endpoint  
- `NAMESPACE` - Deployment namespace
- `source env/env.local` (Optional for tier 2 tests)

## Running E2E Tests

### Build the E2E Binary

```bash
# Generate API client from OpenAPI spec and build
make build

# Verify the build
./bin/hyperfleet-e2e --help
```

### Run Tests

Make sure you've set the required environment variables from the [Prerequisites](#prerequisites) section:

- `HYPERFLEET_API_URL`
- `MAESTRO_URL`
- `NAMESPACE`

**Run tests by tier:**

```bash
# Run tier0 tests (critical path)
./bin/hyperfleet-e2e test --label-filter=tier0

# Run tier1 tests (important features)
./bin/hyperfleet-e2e test --label-filter=tier1

# Run tier2 tests (edge cases - requires sourcing env/env.local first)
source env/env.local && ./bin/hyperfleet-e2e test --label-filter=tier2
```

**Run tests by suite:**

```bash
# Run all cluster tests
./bin/hyperfleet-e2e test --focus "\[Suite: cluster\]"

# Run all nodepool tests
./bin/hyperfleet-e2e test --focus "\[Suite: nodepool\]"

# Run all adapter tests
./bin/hyperfleet-e2e test --focus "\[Suite: adapter\]"
```

**Run specific tests by description:**

```bash
./bin/hyperfleet-e2e test --focus "Create Cluster via API"
```

**View available options:**

```bash
# Show all commands
./bin/hyperfleet-e2e --help

# Show test command options
./bin/hyperfleet-e2e test --help
```

## Troubleshooting

### Debugging Tools

The following tools can help debug and interact with HyperFleet components:

| Tool | Purpose | Link |
|------|---------|------|
| **HyperFleet Explorer** | View cluster/nodepool API responses in a UI | [hyperfleet-explorer](https://github.com/rh-amarin/hyperfleet-explorer) |
| **HyperFleet Scripts** | Interact with component APIs and perform operations | [hyperfleet-scripts](https://github.com/openshift-hyperfleet/hyperfleet-scripts) |
| **k9s** | Kubernetes CLI to manage clusters | [k9scli.io](https://k9scli.io/) |

### Common Issues

#### 1. Namespace Mismatch

**Problem:** Tests fail to find adapters or create resources.

**Cause:** The `NAMESPACE` environment variable doesn't match the deployment namespace. Some tests deploy adapters dynamically and must target the same namespace where HyperFleet components are running.

**Solution:**

```bash
export NAMESPACE=<your-deployment-namespace>
./bin/hyperfleet-e2e test --label-filter=tier0
```

#### 2. Timeout Errors

**Problem:** Test failures with timeout errors:

```
[FAILED] cluster creation failed
Unexpected error:
  failed to create cluster: Post "http://34.9.19.133:8000/api/hyperfleet/v1/clusters":
  context deadline exceeded (Client.Timeout exceeded while awaiting headers)
```

**Solution:**

1. **Verify all pods are running:**

   ```bash
   kubectl get pods -n ${NAMESPACE}
   ```

   Expected output — all pods should show `Running` with `READY 1/1`:

   ```
   NAME                                 READY   STATUS    RESTARTS   AGE
   hyperfleet-api-xxx                   1/1     Running   0          10m
   hyperfleet-sentinel-xxx              1/1     Running   0          10m
   cl-namespace-adapter-xxx             1/1     Running   0          10m
   cl-job-adapter-xxx                   1/1     Running   0          10m
   ```

2. **Check pod logs for errors:**

   ```bash
   # API logs
   kubectl logs -n ${NAMESPACE} deployment/hyperfleet-api --tail=50

   # Sentinel logs
   kubectl logs -n ${NAMESPACE} deployment/hyperfleet-sentinel --tail=50

   # Adapter logs (example)
   kubectl logs -n ${NAMESPACE} deployment/cl-namespace-adapter --tail=50
   ```

3. **Test API connectivity:**

   ```bash
   curl -f -X GET ${HYPERFLEET_API_URL}/api/hyperfleet/v1/clusters/
   ```

   Expected: HTTP 200 response with JSON

4. **Check service endpoints:**

   ```bash
   # For GCP deployments - verify LoadBalancer has external IP
   kubectl get svc -n ${NAMESPACE} hyperfleet-api

   # For Kind deployments - verify port-forwarding is active
   lsof -i :${API_LOCAL_PORT}
   ```

#### 3. Image Pull Errors

**Problem:** Pods stuck in `ImagePullBackOff` or `ErrImagePull` status.

**Solution:**

1. Check if `env/env.local` image settings match your infra deployment. See [Configure Test Settings](setup.md#configure-test-settings) in the Setup guide for how to override image settings.
2. Verify image registry credentials are configured in your cluster
3. Check pod events:

   ```bash
   kubectl describe pod <pod-name> -n ${NAMESPACE}
   ```

#### 4. Port-Forward Connection Refused (Kind)

**Problem:** Tests fail with "connection refused" when using Kind.

**Solution:**

1. Verify port-forward processes are running:

   ```bash
   ps aux | grep "port-forward"
   ```

2. Restart port-forwarding in separate terminals (see [Kind setup](setup.md#option-1-kind-local) in the Setup guide)

3. Verify services exist:

   ```bash
   kubectl get svc -n maestro maestro
   kubectl get svc -n ${NAMESPACE} hyperfleet-api
   ```

## CI Jobs

The test cases you run locally are automatically picked up and executed in nightly CI jobs to ensure continuous validation of the system.

**Job Configuration:** [openshift-hyperfleet-hyperfleet-e2e-main__e2e.yaml](https://github.com/openshift/release/blob/main/ci-operator/config/openshift-hyperfleet/hyperfleet-e2e/openshift-hyperfleet-hyperfleet-e2e-main__e2e.yaml)

| Job Name | Test Tier | Schedule | Description |
|----------|-----------|----------|-------------|
| **tier0-nightly** | tier0 | Daily | Critical: Core user journey broken, fix immediately, blocks release |
| **tier1-nightly** | tier1 | Daily | Major: Important features affected, should be addressed |
| **tier2-nightly** | tier2 | Daily | Minor: Edge cases or low-frequency scenarios, can be deferred |

**Based on [`pkg/labels/labels.go`](../pkg/labels/labels.go) file tags**

### Job Configuration and Management

For comprehensive information about CI jobs, see the [Add HyperFleet E2E CI Job in Prow](https://github.com/openshift-hyperfleet/architecture/blob/main/hyperfleet/docs/test-release/add-hyperfleet-e2e-ci-job-in-prow.md) documentation, which covers:

- How CI jobs are configured in Prow
- Viewing job results
- Debugging job failures
---

To trigger the nightly or RC E2E jobs on demand via the Gangway API (including image-tag overrides), see [Trigger HyperFleet E2E Jobs via Gangway API](https://github.com/openshift-hyperfleet/architecture/blob/main/hyperfleet/docs/release/test-release/trigger-e2e-jobs-via-gangway.md).

## Changelog

All notable changes to this document are documented below.

Format based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

### 2026-06-10

#### Changed
- Restructured runbook for clarity and consistency
- Separated Kind and GCP setup into distinct sections with clearer step-by-step instructions into setup.md
- Improved troubleshooting section with numbered common issues and solutions
- Streamlined test execution instructions with better examples
- Cleaned up formatting and removed duplicate content

### 2026-03-30

#### Added
- Initial runbook with prerequisites, environment setup, test execution, troubleshooting, and CI coverage sections
- Prerequisites section with required tools and verification steps
- Prepare Test Environment section with Terraform and GKE cluster setup
- Deploy CLM section with HyperFleet component deployment instructions
- Running E2E Tests Locally section with build and execution commands
- Common Failure Modes and Troubleshooting section with debugging tools and tips
- Test Coverage in CI section documenting nightly jobs and Prow integration

