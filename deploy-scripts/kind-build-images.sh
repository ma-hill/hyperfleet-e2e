#!/usr/bin/env bash
#
# kind-build-images.sh — Build and load HyperFleet images into kind
#
# Builds component images from local repos under PROJECTS_DIR and loads
# them into the kind cluster. No args = build all. Named args = build
# only those (use full repo names).
#
# Usage:
#   ./deploy-scripts/kind-build-images.sh                          # Build all
#   ./deploy-scripts/kind-build-images.sh hyperfleet-adapter       # Build one
#   ./deploy-scripts/kind-build-images.sh --no-cache               # Force rebuild
#
# Env vars:
#   PROJECTS_DIR   Parent dir containing component repos (default: ~/projects)
#   KIND_CLUSTER   Kind cluster name (default: kind)

set -euo pipefail

PROJECTS_DIR="${PROJECTS_DIR:-${HOME}/projects}"
CI_REGISTRY="registry.ci.openshift.org/ci"
KIND_CLUSTER="${KIND_CLUSTER:-kind}"
CONTAINER_TOOL="${CONTAINER_TOOL:-$(command -v podman 2>/dev/null || command -v docker 2>/dev/null || true)}"
if [[ -z "${CONTAINER_TOOL}" ]]; then
  echo "[ERROR] No container tool found (podman or docker). Install one or set CONTAINER_TOOL."
  exit 1
fi
NO_CACHE=""

# The three platform components — each maps 1:1 to a Docker image.
# Adapter configs in testdata/ all share the same hyperfleet-adapter image.
COMPONENTS=("hyperfleet-api" "hyperfleet-sentinel" "hyperfleet-adapter")

# ============================================================================
# Parse args
# ============================================================================

TARGETS=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --no-cache) NO_CACHE="--no-cache"; shift ;;
    -h|--help)
      echo "Usage: $0 [--no-cache] [COMPONENT...]"
      echo ""
      echo "Builds and loads HyperFleet images into kind from local repos."
      echo "No args = build all. Named args = build only those."
      echo ""
      echo "Components: ${COMPONENTS[*]}"
      echo ""
      echo "Env: PROJECTS_DIR=${PROJECTS_DIR}  KIND_CLUSTER=${KIND_CLUSTER}  CONTAINER_TOOL=${CONTAINER_TOOL}"
      exit 0
      ;;
    -*) echo "Unknown option: $1"; exit 1 ;;
    *) TARGETS+=("$1"); shift ;;
  esac
done

# Default: build all components
if [[ ${#TARGETS[@]} -eq 0 ]]; then
  TARGETS=("${COMPONENTS[@]}")
fi

# ============================================================================
# Build and load
# ============================================================================

echo "=== Building HyperFleet images (cluster: ${KIND_CLUSTER}) ==="

for name in "${TARGETS[@]}"; do
  dir="${PROJECTS_DIR}/${name}"

  if [[ ! -d "${dir}" ]]; then
    echo "[ERROR] ${name} not found at ${dir}"
    echo "        Clone it: git clone https://github.com/openshift-hyperfleet/${name}.git ${dir}"
    echo "        Or set PROJECTS_DIR to the parent directory containing your repos."
    exit 1
  fi

  echo "[BUILD] ${name}..."
  "${CONTAINER_TOOL}" build ${NO_CACHE} -t "${CI_REGISTRY}/${name}:latest" "${dir}"

  echo "[LOAD]  ${name} -> kind..."
  if [[ "$(basename "${CONTAINER_TOOL}")" == "podman" ]]; then
    "${CONTAINER_TOOL}" save "${CI_REGISTRY}/${name}:latest" | kind load image-archive /dev/stdin --name "${KIND_CLUSTER}"
  else
    kind load docker-image "${CI_REGISTRY}/${name}:latest" --name "${KIND_CLUSTER}"
  fi
  echo ""
done

echo "=== Done ==="
