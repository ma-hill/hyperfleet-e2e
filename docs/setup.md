# Setup Guide

This guide covers setting up a HyperFleet environment for running E2E tests locally.

## Table of Contents

- [Deployment Options](#deployment-options)
  - [Option 1: Kind (Local)](#option-1-kind-local)
  - [Option 2: GCP](#option-2-gcp)
- [Configure Test Settings](#configure-test-settings)
- [Troubleshooting](#troubleshooting)

## Deployment Options

Clone the infrastructure repository:

```bash
git clone https://github.com/openshift-hyperfleet/hyperfleet-infra/
cd hyperfleet-infra/terraform
```

Choose one of the following deployment options based on your needs:

- **Kind (local):** Fast setup, no cloud dependencies, uses port-forwarding
- **GCP:** Cloud environment, requires GCP access, slower setup, uses LoadBalancer services

### Option 1: Kind (Local)

**1. Deploy HyperFleet to Kind cluster:**

```bash
# option 1: Export namespace and helmfile env
export NAMESPACE=<your-dev-namespace> ; export HELMFILE_ENV=e2e-kind ; make local-up-kind
# option 2: Set in command line
NAMESPACE=<your-dev-namespace> HELMFILE_ENV=e2e-kind make local-up-kind
```

**2. Set up port-forwarding in two separate terminals:**

```bash
# Terminal 1 - Port-forward Maestro API
export MAESTRO_LOCAL_PORT=8100
kubectl port-forward -n maestro svc/maestro ${MAESTRO_LOCAL_PORT}:8000

# Terminal 2 - Port-forward HyperFleet API
export API_LOCAL_PORT=8000
kubectl port-forward -n ${NAMESPACE} svc/hyperfleet-api ${API_LOCAL_PORT}:8000
```

**3. Configure environment variables:**

```bash
export MAESTRO_URL=http://localhost:${MAESTRO_LOCAL_PORT}
export HYPERFLEET_API_URL=http://localhost:${API_LOCAL_PORT}
export NAMESPACE=<your-dev-namespace>
```

**4. Verify deployment:**

```bash
# Check Helm releases
helm list -n ${NAMESPACE}

# Verify all pods are running
kubectl get pods -n ${NAMESPACE}

# Test API connectivity
curl -f -X GET ${HYPERFLEET_API_URL}/api/hyperfleet/v1/clusters/
```

### Option 2: GCP

**1. Deploy HyperFleet to GCP cluster:**

> Note: Make sure your terraform files are up to date. See [hyperfleet-infra/CONTRIBUTING.md](https://github.com/openshift-hyperfleet/hyperfleet-infra/blob/main/CONTRIBUTING.md#development-setup) for details.
- terraform/envs/gke/dev.tfbackend
- terraform/envs/gke/dev.tfvars
> See [hyperfleet-infra/README.md](https://github.com/openshift-hyperfleet/hyperfleet-infra/blob/main/README.md) for infrastructure deployment details.

```bash
# option 1: Export namespace and helmfile env
export NAMESPACE=<your-dev-namespace> ; export HELMFILE_ENV=e2e-gcp ; make local-up-gcp
# option 2: Set in command line
NAMESPACE=<your-dev-namespace> HELMFILE_ENV=e2e-gcp make local-up-gcp
```

**2. Expose Maestro service via LoadBalancer:**

```bash
# Patch Maestro service to expose external IP
kubectl patch svc maestro -n maestro -p '{"spec":{"type":"LoadBalancer"}}'

# Wait for external IPs to be assigned (may take 1-2 minutes)
kubectl get svc maestro -n maestro -w
kubectl get svc hyperfleet-api -n ${NAMESPACE} -w
```

**3. Configure environment variables:**

```bash
export API_EXTERNAL_IP=$(kubectl get svc hyperfleet-api -n ${NAMESPACE} -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
export MAESTRO_EXTERNAL_IP=$(kubectl get svc maestro -n maestro -o jsonpath='{.status.loadBalancer.ingress[0].ip}')
export HYPERFLEET_API_URL=http://${API_EXTERNAL_IP}:8000
export MAESTRO_URL=http://${MAESTRO_EXTERNAL_IP}:8000
export NAMESPACE=<your-dev-namespace>
```

**4. Verify deployment:**

```bash
# Check Helm releases
helm list -n ${NAMESPACE}

# Verify all pods are running
kubectl get pods -n ${NAMESPACE}

# Test API connectivity
curl -f -X GET ${HYPERFLEET_API_URL}/api/hyperfleet/v1/clusters/
```

## Configure Test Settings

### Override Image Settings (Optional)

If your deployment uses custom image settings, update `env/env.local` in this repo to match your infrastructure deployment settings:

- **Kind deployments:** Match settings from [`env.kind`](https://github.com/openshift-hyperfleet/hyperfleet-infra/blob/main/env.kind#L18-L21)
- **GCP deployments:** Match settings from [`env.gcp`](https://github.com/openshift-hyperfleet/hyperfleet-infra/blob/main/env.gcp#L18-L21)

**Update `env/env.local`:**

```bash
# env/env.local
export IMAGE_REGISTRY=<registry>
export <COMPONENT>_IMAGE_REPO=<repo>
export <COMPONENT>_IMAGE_TAG=<tag>
```

**Source the configuration:**

```bash
source env/env.local
```

This configuration is required for running tier2 tests.

## Troubleshooting

### Infrastructure Setup Issues

For additional help with infrastructure deployment and configuration, see:

- [hyperfleet-infra README](https://github.com/openshift-hyperfleet/hyperfleet-infra/blob/main/README.md) - Main infrastructure documentation

For test-specific troubleshooting (timeouts, API errors, namespace mismatches), see the [Runbook Troubleshooting](runbook.md#troubleshooting) section.

---

**Next Steps:** Once your environment is set up, see the [Runbook](runbook.md) for running tests and troubleshooting.
