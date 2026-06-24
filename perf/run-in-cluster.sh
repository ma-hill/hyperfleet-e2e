#!/bin/bash
# Build, push, and run perf tests inside the cluster.
#
# Usage:
#   ./perf/run-in-cluster.sh   # uses QUAY_USER from env/env.local or the current shell

set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
[[ -f "$REPO_DIR/env/env.local" ]] && set -a && source "$REPO_DIR/env/env.local" && set +a
NAMESPACE="${NAMESPACE:-hyperfleet}"

if [[ -z "${QUAY_USER}" ]]; then
  echo "ERROR: QUAY_USER is not set"
  echo "Add QUAY_USER=myuser to env/env.local or export it"
  exit 1
fi

if ! command -v kubectl &>/dev/null; then
  echo "ERROR: kubectl is not installed"
  exit 1
fi

if ! kubectl cluster-info &>/dev/null; then
  echo "ERROR: Cannot connect to Kubernetes cluster. Check your kubeconfig and context."
  exit 1
fi

if ! kubectl get svc hyperfleet-api -n "$NAMESPACE" &>/dev/null; then
  echo "ERROR: hyperfleet-api service not found in namespace '$NAMESPACE'"
  echo "Make sure the HyperFleet stack (API, Sentinel, adapters, broker) is deployed."
  echo "See: https://github.com/openshift-hyperfleet/hyperfleet-infra"
  exit 1
fi

if ! command -v podman &>/dev/null && ! command -v docker &>/dev/null; then
  echo "ERROR: podman or docker is required to build the image"
  exit 1
fi

echo "=== Building and pushing image ==="
cd "$REPO_DIR"
IMAGE=$(make image-dev 2>&1 | tee /dev/stderr | grep "Dev image pushed:" | awk '{print $NF}')
if [[ -z "$IMAGE" ]]; then
  echo "ERROR: Failed to extract image reference from make image-dev output"
  exit 1
fi
echo "Image: $IMAGE"
echo ""

RESULTS_DIR="$REPO_DIR/perf/results"
mkdir -p "$RESULTS_DIR"
OUTPUT_FILE="$RESULTS_DIR/perf-baseline-$(date +%Y%m%d-%H%M%S).txt"

kubectl delete pod perf-tests -n "$NAMESPACE" --ignore-not-found

echo "=== Running perf tests in cluster ==="
echo "Output: $OUTPUT_FILE"
echo ""
kubectl run perf-tests --rm -i \
  --image="$IMAGE" \
  --restart=Never \
  --image-pull-policy=Always \
  -n "$NAMESPACE" \
  -- test --label-filter=perf --api-url=http://hyperfleet-api.$NAMESPACE.svc.cluster.local:8000 \
  2>&1 | tee "$OUTPUT_FILE"

echo ""
echo "Results saved to: $OUTPUT_FILE"
