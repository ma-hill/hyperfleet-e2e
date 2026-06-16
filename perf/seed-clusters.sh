#!/bin/bash
# Seed the database with clusters for realistic perf baselines.
#
# Usage:
#   ./perf/seed-clusters.sh              # seed 1000 clusters (default)
#   ./perf/seed-clusters.sh 100          # seed 100 clusters
#   ./perf/seed-clusters.sh status       # show cluster counts in the database
#   ./perf/seed-clusters.sh cleanup      # delete seeded clusters only
#   ./perf/seed-clusters.sh reset        # delete ALL clusters (clean slate)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"
[[ -f "$REPO_DIR/.env" ]] && set -a && source "$REPO_DIR/.env" && set +a

API_URL="${HYPERFLEET_API_URL:?ERROR: HYPERFLEET_API_URL is not set}"
API_BASE="$API_URL/api/hyperfleet/v1"
COUNT="${1:-1000}"
SEED_LABEL="perf-seed"
CURL_OPTS="--connect-timeout 10 --max-time 30"

# --- Functions ----------------------------------------------------------------

# Create a single cluster with a unique name via POST /clusters.
create_cluster() {
  local i=$1
  local name="perf-seed-$(printf '%04d' "$i")-$(head -c 4 /dev/urandom | od -An -tx1 | tr -d ' ')"
  local payload
  payload=$(jq --arg name "$name" --arg label "$SEED_LABEL" \
    '.name = $name | .labels[$label] = "true"' \
    "$REPO_DIR/testdata/payloads/clusters/cluster-request.json")

  local status
  status=$(curl -s -o /dev/null -w "%{http_code}" $CURL_OPTS \
    -X POST "$API_BASE/clusters" \
    --http1.1 \
    -H "Content-Type: application/json" \
    -H "Accept: application/json" \
    -d "$payload")

  if [[ "$status" == "201" ]]; then
    return 0
  else
    echo "WARN: cluster $i returned HTTP $status" >&2
    return 1
  fi
}

# Fetch and delete clusters in batches. Takes a curl query as arguments.
# Usage: delete_in_batches                          (all clusters)
#        delete_in_batches --data-urlencode "search=name like 'perf-seed-%'"
delete_in_batches() {
  local deleted=0

  while true; do
    local clusters
    clusters=$(curl -G -s $CURL_OPTS "$API_BASE/clusters" \
      --data-urlencode "pageSize=1000" \
      "$@" \
      --http1.1 -H "Accept: application/json")

    local ids
    ids=$(echo "$clusters" | jq -r '.items[]?.id // empty')
    local batch
    batch=$(echo "$ids" | grep -c . || true)

    if [[ "$batch" -eq 0 ]]; then
      break
    fi

    while IFS= read -r id; do
      [[ -z "$id" ]] && continue
      [[ "$id" =~ ^[a-zA-Z0-9_-]+$ ]] || continue
      local http_code
      http_code=$(curl -s -o /dev/null -w '%{http_code}' $CURL_OPTS -X DELETE "$API_BASE/clusters/$id" --http1.1)
      if [[ "$http_code" =~ ^2 ]]; then
        deleted=$((deleted + 1))
      else
        echo "  WARN: DELETE $id returned HTTP $http_code"
      fi
      if (( deleted % 50 == 0 )); then
        echo "  Deleted $deleted"
      fi
    done <<< "$ids"
  done

  if [[ "$deleted" -eq 0 ]]; then
    echo "No clusters found"
  else
    echo "Deleted $deleted clusters"
  fi
}

# Delete only perf-seed-* clusters (safe for shared environments).
cleanup_clusters() {
  echo "=== Cleaning up seeded clusters ==="
  delete_in_batches --data-urlencode "search=name like 'perf-seed-%'"
}

# Show active and seeded cluster counts.
status_clusters() {
  local active
  active=$(curl -s $CURL_OPTS "$API_BASE/clusters?pageSize=1" --http1.1 -H "Accept: application/json" | jq '.total // 0')

  local seeded
  seeded=$(curl -G -s $CURL_OPTS "$API_BASE/clusters" \
    --data-urlencode "search=name like 'perf-seed-%'" \
    --data-urlencode "pageSize=1" \
    --http1.1 -H "Accept: application/json" | jq '.total // 0')

  echo "=== Database status ==="
  echo "Active clusters:  $active"
  echo "  Seeded (perf-seed-*): $seeded"
  echo "  Other: $(( active - seeded ))"
}

# Delete ALL clusters in batches (includes soft-deleted).
cleanup_all() {
  echo "=== Cleaning up ALL clusters ==="
  delete_in_batches
}

# --- Subcommand dispatch ------------------------------------------------------

if [[ "$COUNT" == "status" ]]; then
  status_clusters
  exit 0
fi

if [[ "$COUNT" == "cleanup" ]]; then
  cleanup_clusters
  exit 0
fi

if [[ "$COUNT" == "reset" ]]; then
  total=$(curl -s $CURL_OPTS "$API_BASE/clusters?pageSize=1" --http1.1 -H "Accept: application/json" | jq '.total // 0')
  echo "WARNING: This will delete ALL $total clusters at $API_URL"
  echo "kubectl context: $(kubectl config current-context 2>/dev/null || echo 'unknown')"
  read -r -p "Are you sure? (y/N) " confirm
  if [[ "$confirm" =~ ^[Yy]$ ]]; then
    cleanup_all
  else
    echo "Aborted."
  fi
  exit 0
fi

if ! [[ "$COUNT" =~ ^[0-9]+$ ]]; then
  echo "ERROR: Invalid argument '$COUNT'. Expected a number, 'status', or 'cleanup'."
  echo ""
  echo "Usage:"
  echo "  $0              # seed 1000 clusters (default)"
  echo "  $0 100          # seed 100 clusters"
  echo "  $0 status       # show cluster counts"
  echo "  $0 cleanup      # delete seeded clusters only"
  echo "  $0 reset        # delete ALL clusters (clean slate)"
  exit 1
fi

# --- Seed clusters (default) --------------------------------------------------

existing=$(curl -G -s $CURL_OPTS "$API_BASE/clusters" \
  --data-urlencode "search=name like 'perf-seed-%'" \
  --data-urlencode "pageSize=1" \
  --http1.1 -H "Accept: application/json" | jq '.total // 0')

if [[ "$existing" -ge "$COUNT" ]]; then
  echo "Already have $existing seeded clusters (target: $COUNT). Nothing to do."
  exit 0
fi

to_create=$((COUNT - existing))
echo "=== Seeding $to_create clusters (existing: $existing, target: $COUNT) ==="
echo "API: $API_URL"
echo ""

created=0
failed=0
for i in $(seq 1 "$to_create"); do
  if create_cluster "$i"; then
    created=$((created + 1))
  else
    failed=$((failed + 1))
  fi
  if (( i % 50 == 0 )); then
    echo "  Progress: $i / $to_create (created: $created, failed: $failed)"
  fi
done

echo ""
echo "=== Seeding complete ==="
echo "Created: $created"
echo "Failed:  $failed"
echo ""
echo "To clean up: ./perf/seed-clusters.sh cleanup"
