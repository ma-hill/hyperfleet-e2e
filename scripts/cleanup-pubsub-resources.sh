#!/usr/bin/env bash

set -euo pipefail

# ============================================================================
# Configuration
# ============================================================================

NAMESPACE=""
GCP_PROJECT_ID="${GCP_PROJECT_ID:-}"
DRY_RUN=false
RESOURCE_TYPES=("clusters" "nodepools")

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

log_verbose() {
    echo "[VERBOSE] $*"
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
Usage: $0 -n <namespace> [OPTIONS]

Required:
  -n, --namespace      Kubernetes namespace (used to match Pub/Sub resources)
  -p, --project-id     GCP project ID (or set GCP_PROJECT_ID env var)

Options:
  -d, --dry-run        Show what would be deleted without deleting
  -h, --help           Show this help message

Examples:
  $0 -n hyperfleet-e2e-<build_id> -p my-gcp-project
  $0 -n hyperfleet-e2e-<build_id> -p my-gcp-project -d

Environment Variables:
  GCP_PROJECT_ID     GCP project ID (can be overridden with -p)
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
        -p|--project-id)
            shift
            GCP_PROJECT_ID="${1:?Option -p/--project-id requires an argument}"
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
elif [[ ! "$NAMESPACE" =~ ^[a-z0-9]([-a-z0-9]*[a-z0-9])?$ ]]; then
    log_error "Namespace must be DNS-1123 compliant (lowercase alphanumeric and hyphens)"
    exit 1
fi

if [[ -z "$GCP_PROJECT_ID" ]]; then
    log_error "GCP project ID is required (use -p or set GCP_PROJECT_ID env var)"
    usage 1
fi

# Check dependencies
if ! command -v gcloud &> /dev/null; then
    log_error "gcloud CLI not found"
    log_error "Install Google Cloud SDK: https://cloud.google.com/sdk/docs/install"
    exit 1
fi

# ============================================================================
# Pub/Sub Deletion Functions
# ============================================================================

# ============================================================================
# GCP Pub/Sub Discovery Functions
# ============================================================================

discover_pubsub_topics() {
    local namespace="$1"
    local project_id="${GCP_PROJECT_ID}"

    log_verbose "Discovering Pub/Sub topics for namespace: ${namespace}"

    if [[ -z "${project_id}" ]]; then
        log_error "GCP_PROJECT_ID is not set"
        return 1
    fi

    # List topics that match the namespace pattern
    # NAMESPACE must be unique and DNS-1123 compliant (default: hyperfleet-e2e-$USER when using .env)
    # Topics are named:
    #   - ${NAMESPACE}-${resource_type}  (e.g., hyperfleet-e2e-jdoe-clusters, hyperfleet-e2e-jdoe-nodepools)
    #   - ${NAMESPACE}-${resource_type}-dlq  (e.g., hyperfleet-e2e-jdoe-clusters-dlq)
    #   - ${NAMESPACE}-${resource_type}-${adapter_name}-dlq  (e.g., hyperfleet-e2e-jdoe-clusters-adapter1-dlq)
    local topics=()
    local all_topics

    if ! all_topics=$(gcloud pubsub topics list --project="${project_id}" --format="value(name)" 2>/dev/null); then
        log_error "Failed to list Pub/Sub topics in project ${project_id}"
        log_error "Make sure you have authenticated with: gcloud auth login"
        return 1
    fi

    while IFS= read -r topic; do
        if [[ -z "${topic}" ]]; then
            continue
        fi

        # Extract topic name from full path (projects/{project}/topics/{topic-name})
        local topic_name="${topic##*/}"

        # Match topics with all naming patterns:
        # 1. Main topics: ${namespace}-${resource_type}
        # 2. DLQ topics (intended): ${namespace}-${resource_type}-dlq
        # 3. DLQ topics (temporary/Helm bug): ${namespace}-${resource_type}-${adapter_name}-dlq
        local matched=false
        for resource_type in "${RESOURCE_TYPES[@]}"; do
            if [[ "${topic_name}" == "${namespace}-${resource_type}" ]] || \
               [[ "${topic_name}" == "${namespace}-${resource_type}-dlq" ]] || \
               [[ "${topic_name}" =~ ^${namespace}-${resource_type}-.+-dlq$ ]]; then
                matched=true
                break
            fi
        done

        if [[ "${matched}" == "true" ]]; then
            topics+=("${topic_name}")
        fi
    done <<< "${all_topics}"

    if [[ ${#topics[@]} -eq 0 ]]; then
        log_verbose "No Pub/Sub topics found for namespace: ${namespace}" >&2
        return 1
    fi

    log_info "Found ${#topics[@]} Pub/Sub topic(s) for namespace ${namespace}:" >&2
    for topic in "${topics[@]}"; do
        log_info "  - ${topic}" >&2
    done

    # Export for use in other functions (stdout only)
    printf '%s\n' "${topics[@]}"
}

discover_pubsub_subscriptions() {
    local namespace="$1"
    local project_id="${GCP_PROJECT_ID}"

    log_verbose "Discovering Pub/Sub subscriptions for namespace: ${namespace}"

    if [[ -z "${project_id}" ]]; then
        log_error "GCP_PROJECT_ID is not set"
        return 1
    fi

    # List subscriptions that match the namespace pattern
    # NAMESPACE must be unique and DNS-1123 compliant (default: hyperfleet-e2e-$USER when using .env)
    # Subscriptions are named: ${NAMESPACE}-${resource_type}-${adapter_name}
    # Example: hyperfleet-e2e-jdoe-clusters-adapter1, <unique-namespace>-clusters-adapter1
    local subscriptions=()
    local all_subscriptions

    if ! all_subscriptions=$(gcloud pubsub subscriptions list --project="${project_id}" --format="value(name)" 2>/dev/null); then
        log_error "Failed to list Pub/Sub subscriptions in project ${project_id}"
        log_error "Make sure you have authenticated with: gcloud auth login"
        return 1
    fi

    while IFS= read -r subscription; do
        if [[ -z "${subscription}" ]]; then
            continue
        fi

        # Extract subscription name from full path (projects/{project}/subscriptions/{subscription-name})
        local subscription_name="${subscription##*/}"

        # Match subscriptions with the expected naming pattern:
        # ${namespace}-${resource_type}-${adapter_name}
        local matched=false
        for resource_type in "${RESOURCE_TYPES[@]}"; do
            if [[ "${subscription_name}" =~ ^${namespace}-${resource_type}-.+ ]]; then
                matched=true
                break
            fi
        done

        if [[ "${matched}" == "true" ]]; then
            subscriptions+=("${subscription_name}")
        fi
    done <<< "${all_subscriptions}"

    if [[ ${#subscriptions[@]} -eq 0 ]]; then
        log_verbose "No Pub/Sub subscriptions found for namespace: ${namespace}" >&2
        return 1
    fi

    log_info "Found ${#subscriptions[@]} Pub/Sub subscription(s) for namespace ${namespace}:" >&2
    for subscription in "${subscriptions[@]}"; do
        log_info "  - ${subscription}" >&2
    done

    # Export for use in other functions (stdout only)
    printf '%s\n' "${subscriptions[@]}"
}

# ============================================================================
# GCP Pub/Sub Deletion Functions
# ============================================================================

delete_pubsub_subscription() {
    local subscription_name="$1"
    local project_id="${GCP_PROJECT_ID}"

    if [[ "${DRY_RUN}" == "true" ]]; then
        log_info "[DRY-RUN] Would delete subscription: ${subscription_name}"
        return 0
    fi

    log_info "Deleting subscription: ${subscription_name}"

    if gcloud pubsub subscriptions delete "${subscription_name}" \
        --project="${project_id}" \
        --quiet 2>/dev/null; then
        log_success "Deleted subscription: ${subscription_name}"
        return 0
    else
        log_error "Failed to delete subscription: ${subscription_name}"
        return 1
    fi
}

delete_pubsub_topic() {
    local topic_name="$1"
    local project_id="${GCP_PROJECT_ID}"

    if [[ "${DRY_RUN}" == "true" ]]; then
        log_info "[DRY-RUN] Would delete topic: ${topic_name}"
        return 0
    fi

    log_info "Deleting topic: ${topic_name}"

    if gcloud pubsub topics delete "${topic_name}" \
        --project="${project_id}" \
        --quiet 2>/dev/null; then
        log_success "Deleted topic: ${topic_name}"
        return 0
    else
        log_error "Failed to delete topic: ${topic_name}"
        return 1
    fi
}

delete_all_pubsub_subscriptions() {
    local namespace="$1"

    log_section "Deleting Pub/Sub Subscriptions"

    # Discover subscriptions (stdout only contains resource names, stderr has logs)
    local subscriptions
    if ! subscriptions=$(discover_pubsub_subscriptions "${namespace}"); then
        log_info "No Pub/Sub subscriptions to delete"
        return 0
    fi

    # Delete each subscription
    local failed=0
    while IFS= read -r subscription; do
        if [[ -n "${subscription}" ]]; then
            if ! delete_pubsub_subscription "${subscription}"; then
                log_error "Failed to delete subscription: ${subscription}"
                ((failed++))
            fi
        fi
    done <<< "${subscriptions}"

    if [[ ${failed} -gt 0 ]]; then
        log_error "${failed} subscription(s) failed to delete"
        return 1
    else
        log_success "All subscriptions deleted successfully"
        return 0
    fi
}

delete_all_pubsub_topics() {
    local namespace="$1"

    log_section "Deleting Pub/Sub Topics"

    # Discover topics (stdout only contains resource names, stderr has logs)
    local topics
    if ! topics=$(discover_pubsub_topics "${namespace}"); then
        log_info "No Pub/Sub topics to delete"
        return 0
    fi

    # Delete each topic
    local failed=0
    while IFS= read -r topic; do
        if [[ -n "${topic}" ]]; then
            if ! delete_pubsub_topic "${topic}"; then
                log_error "Failed to delete topic: ${topic}"
                ((failed++))
            fi
        fi
    done <<< "${topics}"

    if [[ ${failed} -gt 0 ]]; then
        log_error "${failed} topic(s) failed to delete"
        return 1
    else
        log_success "All topics deleted successfully"
        return 0
    fi
}




# ============================================================================
# Main Execution
# ============================================================================

main() {
    log_section "HyperFleet Pub/Sub Cleanup"
    log_info "Namespace: ${NAMESPACE}"
    log_info "GCP Project: ${GCP_PROJECT_ID}"
    log_info "Dry Run: ${DRY_RUN}"

    local exit_code=0

    # Delete subscriptions first (they depend on topics)
    delete_all_pubsub_subscriptions "${NAMESPACE}" || exit_code=$?

    # Delete topics
    delete_all_pubsub_topics "${NAMESPACE}" || exit_code=$?

    log_section "Cleanup Complete"
    if [[ ${exit_code} -eq 0 ]]; then
        log_success "All cleanup operations completed successfully"
    else
        log_warning "Cleanup completed with errors"
    fi

    exit ${exit_code}
}

main
