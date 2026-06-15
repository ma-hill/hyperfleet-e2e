# HyperFleet E2E Test Runbook

> **Audience:** Developers running e2e tests locally

This runbook provides step-by-step instructions for setting up, running, and troubleshooting HyperFleet E2E tests in a local development environment.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Prepare Test Environment](#prepare-test-environment)
- [Deploy CLM to Your Created GKE Cluster](#deploy-clm-to-your-created-gke-cluster)
- [Running E2E Tests Locally](#running-e2e-tests-locally)
- [Common Failure Modes and Troubleshooting](#common-failure-modes-and-troubleshooting)
- [Test Coverage in CI](#test-coverage-in-ci)

## Prerequisites

### Required Tools

The following tools must be installed on your local machine:

| Tool | Minimum Version | Purpose | Installation |
|------|----------------|---------|--------------|
| **Go** | 1.25+ | Build and run the E2E framework | [go.dev](https://go.dev/doc/install) |
| **kubectl** | 1.28+ | Interact with Kubernetes clusters | [kubernetes.io](https://kubernetes.io/docs/tasks/tools/) |
| **helm** | 3.0+ | Deploy HyperFleet components | [helm.sh](https://helm.sh/docs/intro/install/) |
| **git** | 2.30+ | Clone repositories and manage Helm charts | [git-scm.com](https://git-scm.com/downloads) |
| **podman** or **docker** | Latest | Build container images (optional) | [podman.io](https://podman.io/) or [docker.com](https://www.docker.com/) |


### Verify Prerequisites

Run these commands to verify your setup:

```bash
# Check Go version
go version  # Should show 1.25 or higher

# Check kubectl
kubectl version --client

# Check Helm
helm version

# Check Git
git --version

# Check container tool (optional)
podman --version || docker --version
```

## Prepare Test Environment

### Clone and Configure Terraform

First, clone the infrastructure repository and navigate to the terraform directory:

```bash
git clone https://github.com/openshift-hyperfleet/hyperfleet-infra/
cd hyperfleet-infra/terraform
```

### Install GKE Cluster

Run the following Terraform commands to deploy your GKE cluster.

#### Terraform Commands

```bash
# Copy and update the terraform variable file
cp envs/gke/dev.tfvars.example envs/gke/dev-<your name>.tfvars
# Update the following settings in your tfvars file
# developer_name - set to your name, use_pubsub=false, enable_dead_letter=false

# Copy and update the terraform backend file
cp envs/gke/dev.tfbackend.example envs/gke/dev-<your name>.tfbackend
# update the prefix field with your name

# Initialize terraform with your backend configuration
terraform init -backend-config=envs/gke/dev-<your name>.tfbackend

# Preview the infrastructure changes
terraform plan -var-file=envs/gke/dev-<your name>.tfvars

# Apply the infrastructure changes
terraform apply -var-file=envs/gke/dev-<your name>.tfvars
```
### Install Maestro

After deploying the GKE cluster, install Maestro and create a consumer:

```bash
# Install Maestro
make install-maestro

# Create Maestro consumer (default: cluster1, test adapter are configured with it)
make create-maestro-consumer MAESTRO_CONSUMER=cluster1

# Patch the service type to LoadBalancer to expose a external IP
kubectl patch svc maestro -n maestro -p '{"spec":{"type":"LoadBalancer"}}'
```

### Login to Cluster

After the deployment completes, log in to the cluster locally using the output command (replace your name):

```bash
gcloud container clusters get-credentials hyperfleet-dev-<your name> --zone us-central1-a --project hcm-hyperfleet
```

## Deploy CLM to Your Created GKE Cluster

### Clone the Repository

```bash
git clone https://github.com/openshift-hyperfleet/hyperfleet-e2e.git
cd hyperfleet-e2e
```

### Deploy HyperFleet Components

The E2E tests require a running HyperFleet environment (API, Sentinel, and Adapters).

```bash
# 1. Copy the example configuration
cd deploy-scripts/
cp .env.example .env

# 2. Edit .env with your settings
vim .env
source .env

# 3. Deploy with custom configuration
./deploy-clm.sh --action install --namespace "${NAMESPACE}"

```

**Key Configuration Parameters** (in `.env`):

```bash
# GCP configuration (required for Pub/Sub)
export GCP_PROJECT_ID="${GCP_PROJECT_ID:-hcm-hyperfleet}"

# Image configuration (optional - defaults to latest)
export API_IMAGE_TAG="${API_IMAGE_TAG:-latest}"
export SENTINEL_IMAGE_TAG="${SENTINEL_IMAGE_TAG:-latest}"
export ADAPTER_IMAGE_TAG="${ADAPTER_IMAGE_TAG:-latest}"

# Adapters to deploy (optional)
export CLUSTER_TIER0_ADAPTERS_DEPLOYMENT="${CLUSTER_TIER0_ADAPTERS_DEPLOYMENT:-cl-namespace,cl-job,cl-deployment,cl-maestro}"
export NODEPOOL_TIER0_ADAPTERS_DEPLOYMENT="${NODEPOOL_TIER0_ADAPTERS_DEPLOYMENT:-np-configmap}"

# Adapters for API cluster/nodepool configuration
export API_ADAPTERS_CLUSTER="${API_ADAPTERS_CLUSTER:-cl-namespace,cl-job,cl-deployment,cl-maestro}"
export API_ADAPTERS_NODEPOOL="${API_ADAPTERS_NODEPOOL:-np-configmap}"


# NAMESPACE must be unique to prevent GCP Pub/Sub topic/subscription collisions.
# Set in the .env.example file as:
export NAMESPACE="${NAMESPACE:-hyperfleet-e2e-$(echo ${USER:-default} | tr '[:upper:]' '[:lower:]')}"
# Or can manually set it with as the namespace is DNS-1123 compliant
export NAMESPACE=<unique_namespace>

```

#### Verify Deployment

```bash
# Check Helm releases
helm list -n "${NAMESPACE}"

# Verify all pods are running
kubectl get pods -n "${NAMESPACE}"

# Check pod logs if any issues
kubectl logs -n "${NAMESPACE}" <pod-name>
```

**Expected State**: All pods should show status `Running` with `READY 1/1`.


## Running E2E Tests Locally

### Build the E2E Framework

```bash
# Generate API client from OpenAPI spec
make generate

# Build the hyperfleet-e2e binary
make build

# Verify the build
./bin/hyperfleet-e2e --help
```
### Configure API Access

If the Maestro and Hyperfleet API services are not exposed via LoadBalancer, you'll need to port-forward them locally:

```bash
# Terminal 1 - Port-forward Maestro API (local port 8000)
kubectl port-forward -n maestro svc/maestro 8000:8000

# Terminal 2 - Port-forward Hyperfleet API (local port 8001)
kubectl port-forward -n ${NAMESPACE} svc/hyperfleet-api 8001:8000
```

Then configure your environment variables:

```bash
export MAESTRO_URL=http://localhost:8000
export HYPERFLEET_API_URL=http://localhost:8001
```

### Basic Test Execution

```bash
# Run tests with specific label
./bin/hyperfleet-e2e test --label-filter=tier0

# Run tests for specific suite
./bin/hyperfleet-e2e test --focus "\[Suite: cluster\]"

# Run specific test by description
./bin/hyperfleet-e2e test --focus "Create Cluster via API"

```

**Example:**

```bash
# Using environment variable
export HYPERFLEET_API_URL=<value>
export MAESTRO_URL=<value>
export NAMESPACE=<NAMESPACE>
# Run all tier0 cases
./bin/hyperfleet-e2e test --label-filter=tier0

# Run all tier1 cases
./bin/hyperfleet-e2e test --label-filter=tier1
```

### View All Options

```bash
# Show all available commands
./bin/hyperfleet-e2e --help

# Show test command options
./bin/hyperfleet-e2e test --help
```

## Common Failure Modes and Troubleshooting

### Tools and Tips

The following tools are available to help debug and interact with HyperFleet components:

| Tool | Purpose | Link |
|------|---------|------|
| **Hyperfleet Explorer** | View cluster/nodepool API responses | [https://github.com/rh-amarin/hyperfleet-explorer](https://github.com/rh-amarin/hyperfleet-explorer) |
| **Scripts** | Interact with various component APIs and perform operations | [https://github.com/rh-amarin/hyperfleet-scripts](https://github.com/rh-amarin/hyperfleet-scripts) |
| **k9s** | Kubernetes CLI to manage your clusters in style! | [https://k9scli.io/](https://k9scli.io/) |

### General Troubleshooting

#### Namespace Configuration

**Important:** Set the `NAMESPACE` environment variable to match the namespace used during deployment. Some test cases deploy adapters dynamically and need to target the same namespace where your HyperFleet components are running.

```bash
# Set NAMESPACE if you deployed to a unique namespace
export NAMESPACE=<unique_namespace>
./bin/hyperfleet-e2e test --label-filter=tier0
```

#### Timeout Errors

If you encounter timeout errors like this:

```
[FAILED] cluster creation failed
Unexpected error:
  failed to create cluster: Post "http://34.9.19.133:8000/api/hyperfleet/v1/clusters":
  context deadline exceeded (Client.Timeout exceeded while awaiting headers)
```

**Troubleshooting steps:**

1. **Check if all pods are running:**
   ```bash
   kubectl get pods -n ${NAMESPACE}
   ```

   Expected output - all pods should show `Running` with `READY 1/1`:
   ```
   NAME                                 READY   STATUS    RESTARTS   AGE
   hyperfleet-api-xxx                   1/1     Running   0          10m
   hyperfleet-sentinel-xxx              1/1     Running   0          10m
   cl-namespace-adapter-xxx             1/1     Running   0          10m
   cl-job-adapter-xxx                   1/1     Running   0          10m
   ```

2. **Check pod logs for errors:**
   ```bash
   # Check API logs
   kubectl logs -n ${NAMESPACE} deployment/hyperfleet-api --tail=50

   # Check Sentinel logs
   kubectl logs -n ${NAMESPACE} deployment/hyperfleet-sentinel --tail=50

   # Check adapter logs
   kubectl logs -n ${NAMESPACE} deployment/cl-namespace-adapter --tail=50
   ```

3. **Verify API connectivity:**
   ```bash
   # Test API endpoint
   curl -f -X GET ${HYPERFLEET_API_URL}/api/hyperfleet/v1/clusters/
   ```

4. **Check service endpoints:**
   ```bash
   # Verify LoadBalancer has external IP
   kubectl get svc -n ${NAMESPACE} hyperfleet-api
   ```


## Test Coverage in CI

### How Your Tests Run in CI

The test cases you run locally are automatically picked up and executed in nightly CI jobs to ensure continuous validation of the system.

**Job Configuration File:** All job definitions can be found in the [openshift-hyperfleet-hyperfleet-e2e-main__e2e.yaml](https://github.com/openshift/release/blob/main/ci-operator/config/openshift-hyperfleet/hyperfleet-e2e/openshift-hyperfleet-hyperfleet-e2e-main__e2e.yaml) configuration file.

| Job Name | Test Tier | Schedule | Description |
|----------|-----------|----------|-------------|
| **tier0-nightly** | tier0 | Daily | Runs basic smoke tests and happy critical path validations |
| **tier1-nightly** | tier1 | Daily | Runs extended test suite |

### Job Configuration and Management

For comprehensive information about CI jobs, see the [Add HyperFleet E2E CI Job in Prow](https://github.com/openshift-hyperfleet/architecture/blob/main/hyperfleet/docs/test-release/add-hyperfleet-e2e-ci-job-in-prow.md) documentation, which covers:

- How CI jobs are configured in Prow
- Viewing job results
- Debugging job failures

To trigger the nightly or RC E2E jobs on demand via the Gangway API (including image-tag overrides), see [Trigger HyperFleet E2E Jobs via Gangway API](https://github.com/openshift-hyperfleet/architecture/blob/main/hyperfleet/docs/release/test-release/trigger-e2e-jobs-via-gangway.md).

## Changelog

All notable changes to this document will be documented in this section.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/).

### 2026-03-30

#### Added
- Initial runbook with prerequisites, environment setup, test execution, troubleshooting, and CI coverage sections
- Prerequisites section with required tools and verification steps
- Prepare Test Environment section with Terraform and GKE cluster setup
- Deploy CLM section with HyperFleet component deployment instructions
- Running E2E Tests Locally section with build and execution commands
- Common Failure Modes and Troubleshooting section with debugging tools and tips
- Test Coverage in CI section documenting nightly jobs and Prow integration


