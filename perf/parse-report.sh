#!/bin/bash
# Generate a performance baseline report from perf test output.
#
# Usage:
#   ./perf/parse-report.sh                   # uses most recent result
#   ./perf/parse-report.sh output.txt        # parse a specific file

set -euo pipefail

REPO_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
[[ -f "$REPO_DIR/.env" ]] && set -a && source "$REPO_DIR/.env" && set +a

RESULTS_DIR="$REPO_DIR/perf/results"

if [[ -n "${1:-}" ]]; then
  if [[ -f "$1" ]]; then
    INPUT="$1"
    SOURCE="$1"
  else
    echo "ERROR: Input file not found: $1"
    exit 1
  fi
else
  INPUT=$(ls -t "$RESULTS_DIR"/perf-baseline-*.txt 2>/dev/null | grep -v "\-report\.txt$" | head -1 || true)
  if [[ -z "$INPUT" || ! -f "$INPUT" ]]; then
    echo "ERROR: No results found in $RESULTS_DIR"
    echo "Run ./perf/run-in-cluster.sh first, or pass a file:"
    echo "  ./perf/parse-report.sh <output-file>"
    exit 1
  fi
  SOURCE="$INPUT (latest)"
fi

BASE_NAME="${INPUT%-report.txt}"
BASE_NAME="${BASE_NAME%.txt}"
REPORT_FILE="${BASE_NAME}-report.txt"

{

echo "============================================"
echo "  HyperFleet Performance Baseline Report"
echo "============================================"
echo ""

echo "--- Run Metadata ---"
echo "Source:    $SOURCE"
echo "Generated: $(date '+%Y-%m-%d %H:%M:%S')"
KUBE_CTX=$(kubectl config current-context 2>/dev/null || echo "not available")
echo "Cluster:   $KUBE_CTX"
echo ""

echo "--- Test Summary ---"
SUMMARY=$(grep -E "[0-9]+ Passed" "$INPUT" 2>/dev/null | tail -1 || true)
if [[ -n "$SUMMARY" ]]; then
  echo "$SUMMARY"
else
  echo "No summary line found"
fi
DURATION=$(grep -E "^Ran [0-9]+" "$INPUT" 2>/dev/null | tail -1 || true)
if [[ -n "$DURATION" ]]; then
  echo "$DURATION"
fi
echo ""

echo "--- Baseline Latencies ---"
grep "\[PERF\]" "$INPUT" | sed 's/^[[:space:]]*/  /' || echo "  No latency data found"
echo ""

FAILURES=$(grep -c "\[FAIL\]" "$INPUT" 2>/dev/null || true)
if [[ "${FAILURES:-0}" -gt 0 ]]; then
  echo "--- Failures ---"
  grep "\[FAIL\]" "$INPUT" | sed 's/^[[:space:]]*/  /'
  echo ""
fi

echo "============================================"

} | tee "$REPORT_FILE"

echo ""
echo "Report saved to: $REPORT_FILE"
