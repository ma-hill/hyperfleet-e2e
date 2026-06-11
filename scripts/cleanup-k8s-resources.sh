#!/usr/bin/env bash

set -euo pipefail

# ============================================================================
# Configuration
# ============================================================================

NAMESPACE=""
JSON_FILE=""
DRY_RUN=false

# ============================================================================
# Logging Functions
# ============================================================================

log_info() {
    echo "[INFO] $*"
}

log_success() {
    echo "[SUCCESS] $*"
}

log_warning() {
    echo "[WARNING] $*" >&2
}

log_error() {
    echo "[ERROR] $*" >&2
}

log_section() {
    echo ""
    echo "========================================"
    echo "$*"
    echo "========================================"
}

# ============================================================================
# Argument Parsing
# ============================================================================

usage() {
    cat <<EOF
Usage: $0 -n <namespace> [-f <json-file>] [OPTIONS]

Required:
  -n, --namespace        Kubernetes namespace

Optional:
  -f, --json-file        Helmfile releases JSON file (if not provided, uses helm list)

Options:
  -d, --dry-run          Show what would be deleted without deleting
  -h, --help             Show this help message

Examples:
  $0 -n hyperfleet-e2e-<build_id> -f helmfile-releases.json
  $0 -n hyperfleet-e2e-<build_id> -f helmfile-releases.json -d
  $0 -n hyperfleet-e2e-<build_id>  # Best-effort cleanup without JSON file
EOF
    exit "${1:-0}"
}

while [[ $# -gt 0 ]]; do
    case $1 in
        -n|--namespace)
            shift
            NAMESPACE="${1:?Option -n/--namespace requires an argument}"
            shift
            ;;
        -f|--json-file)
            shift
            JSON_FILE="${1:?Option -f/--json-file requires an argument}"
            shift
            ;;
        -d|--dry-run)
            DRY_RUN=true
            shift
            ;;
        -h|--help)
            usage
            ;;
        *)
            log_error "Unknown option: $1"
            usage 1
            ;;
    esac
done


# Validate required arguments
if [[ -z "$NAMESPACE" ]]; then
    log_error "Namespace is required"
    usage 1
fi

# Validate JSON file if provided
if [[ -n "$JSON_FILE" ]] && [[ ! -f "$JSON_FILE" ]]; then
    log_error "File '$JSON_FILE' not found"
    exit 1
fi

# ============================================================================
# Dependency Checking Functions
# ============================================================================

check_dependencies() {
    local missing=()

    command -v jq &> /dev/null || missing+=("jq")
    command -v kubectl &> /dev/null || missing+=("kubectl")
    command -v helm &> /dev/null || missing+=("helm")

    if [[ ${#missing[@]} -gt 0 ]]; then
        log_error "Missing required dependencies: ${missing[*]}"
        return 1
    fi
    return 0
}


# ============================================================================
# Helm Release Functions
# ============================================================================
uninstall_hyperfleet_releases_best_effort() {
    local namespace="$1"

    log_section "Best-Effort Helm Release Cleanup"
    log_warning "JSON file not available, attempting cleanup using helm list"

    # List all releases in the namespace
    local releases
    releases=$(helm list -n "${namespace}" --short 2>/dev/null || true)

    if [[ -z "$releases" ]]; then
        log_info "No Helm releases found in namespace ${namespace}"
        return 0
    fi

    log_info "Found releases in namespace ${namespace}:"
    echo "$releases"

    local count=0
    local failed=0

    while read -r name; do
        [[ -z "$name" ]] && continue
        ((count++))

        if [[ "${DRY_RUN}" == true ]]; then
            log_info "[DRY-RUN] Would uninstall $name from ${namespace}"
        else
            log_info "Uninstalling $name from ${namespace}"
            if helm uninstall "$name" -n "${namespace}"; then
                log_success "Uninstalled $name"
            else
                log_error "Failed to uninstall $name"
                ((failed++))
            fi
        fi
    done <<< "$releases"

    if [[ $failed -gt 0 ]]; then
        log_warning "$failed of $count releases failed to uninstall"
        return 1
    else
        log_success "All $count release(s) uninstalled successfully"
        return 0
    fi
}

uninstall_hyperfleet_releases() {
    local json_file="$1"

    log_section "Uninstalling Helm Releases"

    local releases
    releases=$(jq -r '.[] | select(.labels | contains("group:hyperfleet")) | "\(.name) \(.namespace)"' "$json_file")

    if [[ -z "$releases" ]]; then
        log_info "No Helm releases found with label group:hyperfleet"
        return 0
    fi

    local count=0
    local failed=0

    while read -r name namespace; do
        [[ -z "$name" ]] && continue
        ((count++))

        if [[ "${DRY_RUN}" == true ]]; then
            log_info "[DRY-RUN] Would uninstall $name from $namespace"
        else
            log_info "Uninstalling $name from $namespace"
            if helm uninstall "$name" -n "$namespace"; then
                log_success "Uninstalled $name"
            else
                log_error "Failed to uninstall $name"
                ((failed++))
            fi
        fi
    done <<< "$releases"

    if [[ $failed -gt 0 ]]; then
        log_error "$failed of $count releases failed to uninstall"
        return 1
    else
        log_success "All $count release(s) uninstalled successfully"
        return 0
    fi
}

# ============================================================================
# Namespace Functions
# ============================================================================

delete_namespace() {
    local namespace="$1"

    log_section "Deleting Namespace"

    if [[ "${DRY_RUN}" == true ]]; then
        log_info "[DRY-RUN] Would delete namespace: ${namespace}"
        return 0
    fi

    if ! kubectl get namespace "${namespace}" &> /dev/null; then
        log_warning "Namespace '${namespace}' does not exist"
        return 0
    fi

    log_info "Deleting namespace: ${namespace}"
    if kubectl delete namespace "${namespace}" --wait --timeout=5m; then
        log_success "Namespace deleted successfully"
        return 0
    else
        log_error "Failed to delete namespace '${namespace}'"
        log_info "You may need to manually remove finalizers or check for stuck resources"
        return 1
    fi
}

# ============================================================================
# Main Execution
# ============================================================================

main() {
    log_section "HyperFleet K8s Cleanup"
    log_info "Namespace: ${NAMESPACE}"
    log_info "JSON File: ${JSON_FILE:-<not provided>}"
    log_info "Dry Run: ${DRY_RUN}"

    local exit_code=0

    if ! check_dependencies; then
        exit 1
    fi

    # Uninstall Helm releases
    if [[ -n "$JSON_FILE" ]] && [[ -f "$JSON_FILE" ]]; then
        uninstall_hyperfleet_releases "${JSON_FILE}" || exit_code=$?
    else
        uninstall_hyperfleet_releases_best_effort "${NAMESPACE}" || exit_code=$?
    fi

    # Delete namespace
    delete_namespace "${NAMESPACE}" || exit_code=$?

    log_section "Cleanup Complete"
    if [[ ${exit_code} -eq 0 ]]; then
        log_success "All cleanup operations completed successfully"
    else
        log_warning "Cleanup completed with errors"
    fi

    exit ${exit_code}
}

main
