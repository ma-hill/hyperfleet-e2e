# Performance Tests

Lightweight performance tests that measure baseline latencies for core HyperFleet operations. Tests run inside the cluster for production-representative numbers.

## Table of contents

- [Prerequisites](#prerequisites)
  - [Pub/Sub broker setup](#pubsub-broker-setup)
  - [Verify the stack](#verify-the-stack)
- [Seeding data](#seeding-data)
- [Running tests](#running-tests)
- [Parsing results](#parsing-results)

## Prerequisites

The full HyperFleet stack (API, Sentinel, adapters, broker) must be deployed and healthy. Use the [hyperfleet-infra](https://github.com/openshift-hyperfleet/hyperfleet-infra) project to set up the environment.

```bash
cp .env.example .env
```

### Pub/Sub broker setup

Reconciliation tests require adapters to be connected to the correct Pub/Sub topics. In the `hyperfleet-infra` project:

1. Set `use_pubsub = true` in `terraform/envs/gke/dev.tfvars`
2. Uncomment `pubsub_topic_configs` in `dev.tfvars` to define the topic/subscription topology
3. Run `make install-terraform` to create Pub/Sub resources and generate broker config files in `generated-values-from-terraform/`
4. Run `make install-adapters` to redeploy adapters with the correct broker config

### Verify the stack

Confirm the adapters are subscribed to the correct topic (not `placeholder`):

```bash
kubectl exec -n hyperfleet deploy/adapter1-hyperfleet-adapter -- env | grep HYPERFLEET_BROKER_TOPIC
```

Confirm Sentinel is publishing events:

```bash
kubectl logs -n hyperfleet -l app.kubernetes.io/name=hyperfleet-sentinel --tail=20
```

Look for `"Publishing event"` and `"Published event"` log lines.

## Seeding data

For realistic baselines, seed the database with clusters before running tests. The seeded clusters add realistic table size so query planner behavior and index performance reflect production conditions.

```bash
# Port-forward the API
kubectl port-forward -n hyperfleet svc/hyperfleet-api 8000:8000

# In another terminal, seed 1000 clusters
export HYPERFLEET_API_URL=http://localhost:8000
./perf/seed-clusters.sh 1000

# Check what's in the database
./perf/seed-clusters.sh status

# Clean up seeded clusters only
./perf/seed-clusters.sh cleanup

# Or delete ALL clusters (clean slate)
./perf/seed-clusters.sh reset
```

## Running tests

Make sure you have [seeded the database](#seeding-data) first for realistic baselines.

```bash
./perf/run-in-cluster.sh
```

This builds the image, pushes it, and runs the tests in-cluster.

## Parsing results

```bash
./perf/parse-report.sh
```

This extracts `[PERF]` and `[FAIL]` lines from the latest output file and generates a summary report.
