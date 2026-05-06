#!/usr/bin/env bash

# deploy-clm.sh - Automated CLM Components Deployment Script
#
# This script automates the installation and uninstallation of HyperFleet CLM components
# (API, Sentinel, and Adapters) using Helm for E2E testing environments.
#
# Usage:
#   ./deploy-clm.sh --action install --namespace hyperfleet-e2e
#   ./deploy-clm.sh --action uninstall --namespace hyperfleet-e2e --dry-run

set -euo pipefail

# ============================================================================
# Working Directories (must be set before loading .env)
# ============================================================================
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
WORK_DIR="${PROJECT_ROOT}/.deploy-work"
TESTDATA_DIR="${TESTDATA_DIR:-${PROJECT_ROOT}/testdata}"

# ============================================================================
# Load Environment Variables from .env file
# ============================================================================
ENV_FILE="${SCRIPT_DIR}/.env"

if [[ -f "${ENV_FILE}" ]]; then
    set -a  # automatically export all variables
    source "${ENV_FILE}"
    set +a
else
    echo "[WARNING] .env file not found at ${ENV_FILE}"
    echo "[WARNING] Using default configuration values"
fi

# ============================================================================
# Default Configuration (fallback if .env is not loaded)
# ============================================================================

ACTION="${ACTION:-}"
NAMESPACE="${NAMESPACE:-hyperfleet-e2e}"
DRY_RUN="${DRY_RUN:-false}"
VERBOSE="${VERBOSE:-false}"

# Image Registry
IMAGE_REGISTRY="${IMAGE_REGISTRY:-registry.ci.openshift.org}"

# Provider Configuration
GCP_PROJECT_ID="${GCP_PROJECT_ID:-hcm-hyperfleet}"

# API Component
API_IMAGE_REPO="${API_IMAGE_REPO:-ci/hyperfleet-api}"
API_IMAGE_TAG="${API_IMAGE_TAG:-latest}"
API_SERVICE_TYPE="${API_SERVICE_TYPE:-LoadBalancer}"
API_ADAPTERS_CLUSTER="${API_ADAPTERS_CLUSTER:-}"
API_ADAPTERS_NODEPOOL="${API_ADAPTERS_NODEPOOL:-}"

# Adapter Test Data Configuration
ADAPTERS_FILE_DIR="${ADAPTERS_FILE_DIR:-${TESTDATA_DIR}/adapter-configs}"
CLUSTER_TIER0_ADAPTERS_DEPLOYMENT="${CLUSTER_TIER0_ADAPTERS_DEPLOYMENT:-}"
NODEPOOL_TIER0_ADAPTERS_DEPLOYMENT="${NODEPOOL_TIER0_ADAPTERS_DEPLOYMENT:-}"

# Sentinel Component
SENTINEL_IMAGE_REPO="${SENTINEL_IMAGE_REPO:-ci/hyperfleet-sentinel}"
SENTINEL_IMAGE_TAG="${SENTINEL_IMAGE_TAG:-latest}"
SENTINEL_BROKER_TYPE="${SENTINEL_BROKER_TYPE:-googlepubsub}"
SENTINEL_GOOGLEPUBSUB_CREATE_TOPIC_IF_MISSING="${SENTINEL_GOOGLEPUBSUB_CREATE_TOPIC_IF_MISSING:-true}"
SENTINEL_BROKER_RABBITMQ_URL="${SENTINEL_BROKER_RABBITMQ_URL:-}"

# Adapter Component
ADAPTER_IMAGE_REPO="${ADAPTER_IMAGE_REPO:-ci/hyperfleet-adapter}"
ADAPTER_IMAGE_TAG="${ADAPTER_IMAGE_TAG:-latest}"
ADAPTER_BROKER_TYPE="${ADAPTER_BROKER_TYPE:-googlepubsub}"
ADAPTER_GOOGLEPUBSUB_CREATE_TOPIC_IF_MISSING="${ADAPTER_GOOGLEPUBSUB_CREATE_TOPIC_IF_MISSING:-true}"
ADAPTER_GOOGLEPUBSUB_CREATE_SUBSCRIPTION_IF_MISSING="${ADAPTER_GOOGLEPUBSUB_CREATE_SUBSCRIPTION_IF_MISSING:-true}"
ADAPTER_BROKER_RABBITMQ_URL="${ADAPTER_BROKER_RABBITMQ_URL:-}"

# HyperFleet API Configuration
API_BASE_URL="${API_BASE_URL:-http://hyperfleet-api:8000}"

# Helm Chart Sources
API_CHART_REPO="${API_CHART_REPO:-https://github.com/openshift-hyperfleet/hyperfleet-api.git}"
API_CHART_REF="${API_CHART_REF:-main}"
API_CHART_PATH="${API_CHART_PATH:-charts}"

SENTINEL_CHART_REPO="${SENTINEL_CHART_REPO:-https://github.com/openshift-hyperfleet/hyperfleet-sentinel.git}"
SENTINEL_CHART_REF="${SENTINEL_CHART_REF:-main}"
SENTINEL_CHART_PATH="${SENTINEL_CHART_PATH:-charts}"

ADAPTER_CHART_REPO="${ADAPTER_CHART_REPO:-https://github.com/openshift-hyperfleet/hyperfleet-adapter.git}"
ADAPTER_CHART_REF="${ADAPTER_CHART_REF:-main}"
ADAPTER_CHART_PATH="${ADAPTER_CHART_PATH:-charts}"

# Component flags
INSTALL_API="${INSTALL_API:-true}"
INSTALL_SENTINEL="${INSTALL_SENTINEL:-true}"
INSTALL_ADAPTER="${INSTALL_ADAPTER:-true}"

# Uninstall options
DELETE_K8S_RESOURCES="${DELETE_K8S_RESOURCES:-false}"
DELETE_CLOUD_RESOURCES="${DELETE_CLOUD_RESOURCES:-false}"
DELETE_ALL="${DELETE_ALL:-false}"

# Debug logging
DEBUG_LOG_DIR="${DEBUG_LOG_DIR:-${PROJECT_ROOT}/.debug-work}"

# ============================================================================
# Load Library Modules
# ============================================================================

source "${SCRIPT_DIR}/lib/common.sh"
source "${SCRIPT_DIR}/lib/helm.sh"
source "${SCRIPT_DIR}/lib/api.sh"
source "${SCRIPT_DIR}/lib/sentinel.sh"
source "${SCRIPT_DIR}/lib/adapter.sh"
source "${SCRIPT_DIR}/lib/gcp.sh"

# ============================================================================
# Usage and Argument Parsing
# ============================================================================

print_usage() {
    cat << EOF
Usage: ${0##*/} --action <install|uninstall> [OPTIONS]

Automated deployment script for HyperFleet CLM components (API, Sentinel, Adapter)

CONFIGURATION:
    This script loads configuration from ${SCRIPT_DIR}/.env file.
    You can override any .env value using command-line flags.

REQUIRED FLAGS:
    --action <action>               Action to perform: install or uninstall

OPTIONAL FLAGS:
    --namespace <namespace>         Kubernetes namespace (default: hyperfleet-e2e)

    # Component Selection
    --skip-api                      Skip API installation
    --skip-sentinel                 Skip Sentinel installation
    --skip-adapter                  Skip Adapter installation

    # Image Configuration
    --image-registry <registry>     Image registry (default: ${IMAGE_REGISTRY})
    --api-image-repo <repo>         API image repository (default: ${API_IMAGE_REPO})
    --api-image-tag <tag>           API image tag (default: ${API_IMAGE_TAG})
    --sentinel-image-repo <repo>    Sentinel image repository (default: ${SENTINEL_IMAGE_REPO})
    --sentinel-image-tag <tag>      Sentinel image tag (default: ${SENTINEL_IMAGE_TAG})
    --adapter-image-repo <repo>     Adapter image repository (default: ${ADAPTER_IMAGE_REPO})
    --adapter-image-tag <tag>       Adapter image tag (default: ${ADAPTER_IMAGE_TAG})

    # API Configuration
    --api-base-url <url>            HyperFleet API base URL for Sentinel and Adapter
                                    (default: http://hyperfleet-api.<namespace>.svc.cluster.local:8000)
    --api-adapters-cluster <list>   Comma-separated list of cluster adapters for API config (e.g., "cl-namespace,cl-job")
    --api-adapters-nodepool <list>  Comma-separated list of nodepool adapters for API config (e.g., "np-configmap")

    # Adapter Deployment Configuration
    --cluster-tier0-adapters <list>  Comma-separated list of cluster-level adapters to deploy (e.g., "cl-namespace,cl-job")
    --nodepool-tier0-adapters <list> Comma-separated list of nodepool-level adapters to deploy (e.g., "np-configmap")
    --adapters-file-dir <path>       Base directory containing adapter test data folders (default: ${TESTDATA_DIR}/adapter-configs)

    # Uninstall Options (only for --action uninstall)
    --delete-k8s-resources          Delete Kubernetes resources (Helm releases + namespace)
    --delete-cloud-resources        Delete GCP Pub/Sub topics and subscriptions
    --all                           Delete everything (k8s resources + cloud resources)

    # Execution Options
    --dry-run                       Print commands without executing
    --verbose                       Enable verbose logging
    --debug-log-dir <path>          Directory to save debug logs on deployment failures
                                    (default: ${WORK_DIR}/debug-logs)
    --help                          Show this help message

ENVIRONMENT VARIABLES:
    All configuration can be set in the .env file located at: ${SCRIPT_DIR}/.env

    Common variables:
    - NAMESPACE                              Kubernetes namespace
    - IMAGE_REGISTRY                         Container image registry
    - API_IMAGE_TAG                          API image tag
    - SENTINEL_IMAGE_TAG                     Sentinel image tag
    - ADAPTER_IMAGE_TAG                      Adapter image tag
    - GCP_PROJECT_ID                         Google Cloud Project ID for Pub/Sub
    - TESTDATA_DIR                           Base directory for test data (default: PROJECT_ROOT/testdata)
    - CLUSTER_TIER0_ADAPTERS_DEPLOYMENT      Cluster-level adapters to deploy (comma-separated)
    - NODEPOOL_TIER0_ADAPTERS_DEPLOYMENT     NodePool-level adapters to deploy (comma-separated)
    - ADAPTERS_FILE_DIR                      Base directory for adapter test data (default: TESTDATA_DIR/adapter-configs)
    - API_ADAPTERS_CLUSTER                   Adapters for API cluster config (set per test case)
    - API_ADAPTERS_NODEPOOL                  Adapters for API nodepool config (set per test case)

    RabbitMQ broker (must be provisioned externally before running this script):
    - SENTINEL_BROKER_TYPE=rabbitmq          Use RabbitMQ instead of Google Pub/Sub for Sentinel
    - SENTINEL_BROKER_RABBITMQ_URL           RabbitMQ AMQP URL for Sentinel (e.g., amqp://user:pass@host:5672/)
    - ADAPTER_BROKER_TYPE=rabbitmq           Use RabbitMQ instead of Google Pub/Sub for Adapters
    - ADAPTER_BROKER_RABBITMQ_URL            RabbitMQ AMQP URL for Adapters

EXAMPLES:
    # Install all components with default settings
    ${0##*/} --action install --namespace hyperfleet-e2e

    # Install with custom image tags
    ${0##*/} --action install \\
        --namespace test-env \\
        --api-image-tag v1.0.0 \\
        --sentinel-image-tag v1.0.0 \\
        --adapter-image-tag v1.0.0

    # Install only API and Sentinel
    ${0##*/} --action install --skip-adapter

    # Dry-run uninstallation
    ${0##*/} --action uninstall --namespace hyperfleet-e2e --dry-run --verbose

    # Delete Kubernetes resources (Helm releases + namespace)
    ${0##*/} --action uninstall --namespace hyperfleet-e2e --delete-k8s-resources

    # Delete GCP Pub/Sub resources (topics and subscriptions)
    ${0##*/} --action uninstall --namespace hyperfleet-e2e --delete-cloud-resources

    # Complete cleanup: delete everything (k8s + cloud resources)
    ${0##*/} --action uninstall --namespace hyperfleet-e2e --all

    # Or explicitly specify both
    ${0##*/} --action uninstall --namespace hyperfleet-e2e \\
        --delete-k8s-resources \\
        --delete-cloud-resources

    # Install with custom image repositories
    ${0##*/} --action install \\
        --api-image-repo myregistry.io/hyperfleet-api \\
        --api-image-tag dev-123

EOF
}

parse_arguments() {
    if [[ $# -eq 0 ]]; then
        print_usage
        exit 1
    fi

    while [[ $# -gt 0 ]]; do
        case "$1" in
            --action)
                ACTION="$2"
                shift 2
                ;;
            --namespace)
                NAMESPACE="$2"
                shift 2
                ;;
            --skip-api)
                INSTALL_API=false
                shift
                ;;
            --skip-sentinel)
                INSTALL_SENTINEL=false
                shift
                ;;
            --skip-adapter)
                INSTALL_ADAPTER=false
                shift
                ;;
            --image-registry)
                IMAGE_REGISTRY="$2"
                shift 2
                ;;
            --api-image-repo)
                API_IMAGE_REPO="$2"
                shift 2
                ;;
            --api-image-tag)
                API_IMAGE_TAG="$2"
                shift 2
                ;;
            --sentinel-image-repo)
                SENTINEL_IMAGE_REPO="$2"
                shift 2
                ;;
            --sentinel-image-tag)
                SENTINEL_IMAGE_TAG="$2"
                shift 2
                ;;
            --adapter-image-repo)
                ADAPTER_IMAGE_REPO="$2"
                shift 2
                ;;
            --adapter-image-tag)
                ADAPTER_IMAGE_TAG="$2"
                shift 2
                ;;
            --api-base-url)
                API_BASE_URL="$2"
                shift 2
                ;;
            --api-adapters-cluster)
                API_ADAPTERS_CLUSTER="$2"
                shift 2
                ;;
            --api-adapters-nodepool)
                API_ADAPTERS_NODEPOOL="$2"
                shift 2
                ;;
            --cluster-tier0-adapters)
                CLUSTER_TIER0_ADAPTERS_DEPLOYMENT="$2"
                shift 2
                ;;
            --nodepool-tier0-adapters)
                NODEPOOL_TIER0_ADAPTERS_DEPLOYMENT="$2"
                shift 2
                ;;
            --adapters-file-dir)
                ADAPTERS_FILE_DIR="$2"
                shift 2
                ;;
            --delete-k8s-resources)
                DELETE_K8S_RESOURCES=true
                shift
                ;;
            --delete-cloud-resources)
                DELETE_CLOUD_RESOURCES=true
                shift
                ;;
            --all)
                DELETE_ALL=true
                DELETE_K8S_RESOURCES=true
                DELETE_CLOUD_RESOURCES=true
                shift
                ;;
            --dry-run)
                DRY_RUN=true
                shift
                ;;
            --verbose)
                VERBOSE=true
                shift
                ;;
            --debug-log-dir)
                DEBUG_LOG_DIR="$2"
                shift 2
                ;;
            --help|-h)
                print_usage
                exit 0
                ;;
            *)
                log_error "Unknown option: $1"
                echo
                print_usage
                exit 1
                ;;
        esac
    done

    # Validate required arguments
    if [[ -z "${ACTION}" ]]; then
        log_error "Missing required flag: --action"
        echo
        print_usage
        exit 1
    fi

    if [[ "${ACTION}" != "install" && "${ACTION}" != "uninstall" ]]; then
        log_error "Invalid action: ${ACTION}. Must be 'install' or 'uninstall'"
        exit 1
    fi

    # Validate at least one component is selected
    if [[ "${INSTALL_API}" == "false" && "${INSTALL_SENTINEL}" == "false" && "${INSTALL_ADAPTER}" == "false" ]]; then
        log_error "At least one component must be selected for installation"
        exit 1
    fi
}

# ============================================================================
# Main Installation Flow
# ============================================================================

perform_install() {
    log_section "Starting CLM Components Installation"

    # Validate environment
    check_dependencies || exit 1
    validate_kubectl_context || exit 1

    # Prepare working directory
    log_section "Preparing Working Directory"
    mkdir -p "${WORK_DIR}"
    log_verbose "Work directory: ${WORK_DIR}"

    # Clone Helm charts
    log_section "Cloning Helm Charts"

    if [[ "${INSTALL_API}" == "true" ]]; then
        clone_helm_chart "api" "${API_CHART_REPO}" "${API_CHART_REF}" "${API_CHART_PATH}" || exit 1
    fi

    if [[ "${INSTALL_SENTINEL}" == "true" ]]; then
        clone_helm_chart "sentinel" "${SENTINEL_CHART_REPO}" "${SENTINEL_CHART_REF}" "${SENTINEL_CHART_PATH}" || exit 1
    fi

    if [[ "${INSTALL_ADAPTER}" == "true" ]]; then
        clone_helm_chart "adapter" "${ADAPTER_CHART_REPO}" "${ADAPTER_CHART_REF}" "${ADAPTER_CHART_PATH}" || exit 1
    fi

    # Install components in order: API -> Sentinel -> Adapter
    if [[ "${INSTALL_API}" == "true" ]]; then
        install_api || exit 1
    fi

    if [[ "${INSTALL_SENTINEL}" == "true" ]]; then
        install_sentinel || exit 1
    fi

    if [[ "${INSTALL_ADAPTER}" == "true" ]]; then
        install_adapters || {
            log_error "Adapter installation failed"
            log_section "Installation Failed"
            exit 1
        }
    fi

    # Final status
    log_section "Installation Complete"

    if [[ "${DRY_RUN}" == "false" ]]; then
        log_info "Deployed components:"
        helm list -n "${NAMESPACE}"

        echo
        log_info "Pod status:"
        kubectl get pods -n "${NAMESPACE}"

        echo
        log_success "All components installed successfully!"
        log_info "Namespace: ${NAMESPACE}"
        log_info "To view logs: kubectl logs -n ${NAMESPACE} -l app.kubernetes.io/name=<component>"

        # Display API external IP if available
        if [[ "${INSTALL_API}" == "true" ]]; then
            local external_ip
            external_ip=$(kubectl get svc "hyperfleet-api" -n "${NAMESPACE}" -o jsonpath='{.status.loadBalancer.ingress[0].ip}' 2>/dev/null)
            if [[ -n "${external_ip}" ]]; then
                echo
                log_info "HyperFleet API External IP: ${external_ip}"
                log_info "API URL: http://${external_ip}:8000"
            fi
        fi
    else
        log_info "[DRY-RUN] Installation simulation complete"
    fi

    # Clean up work directory
    if [[ "${DRY_RUN}" == "false" && "${VERBOSE}" == "false" ]]; then
        log_verbose "Cleaning up work directory"
        rm -rf "${WORK_DIR}"
    fi
}

# ============================================================================
# Main Uninstallation Flow
# ============================================================================

perform_uninstall() {
    log_section "Starting CLM Components Uninstallation"

    # Validate environment
    check_dependencies || exit 1
    validate_kubectl_context || exit 1

    # Display uninstall configuration
    log_info "Uninstall Configuration:"
    log_info "  Delete K8s Resources (including namespace): ${DELETE_K8S_RESOURCES}"
    log_info "  Delete Cloud Resources: ${DELETE_CLOUD_RESOURCES}"

    local uninstall_errors=0

    # Uninstall Kubernetes resources (in reverse order: Adapter -> Sentinel -> API)
    if [[ "${DELETE_K8S_RESOURCES}" == "true" ]]; then
        log_section "Uninstalling Kubernetes Resources"

        if [[ "${INSTALL_ADAPTER}" == "true" ]]; then
            if ! uninstall_adapters; then
                ((uninstall_errors++))
            fi
        fi

        if [[ "${INSTALL_SENTINEL}" == "true" ]]; then
            if ! uninstall_sentinel; then
                ((uninstall_errors++))
            fi
        fi

        if [[ "${INSTALL_API}" == "true" ]]; then
            if ! uninstall_api; then
                log_warning "Failed to uninstall API"
                ((uninstall_errors++))
            fi
        fi

        # Delete namespace (this will remove any remaining k8s resources)
        if ! delete_namespace "${NAMESPACE}"; then
            log_warning "Failed to delete namespace"
            ((uninstall_errors++))
        fi
    else
        log_info "Skipping Kubernetes resource deletion (use --delete-k8s-resources to enable)"
    fi

    # Delete GCP resources (topics and subscriptions)
    if [[ "${DELETE_CLOUD_RESOURCES}" == "true" ]]; then
        log_section "Deleting Cloud Provider Resources"
        if ! cleanup_gcp_resources "${NAMESPACE}"; then
            log_warning "Some GCP resources failed to delete"
            ((uninstall_errors++))
        fi
    else
        log_info "Skipping cloud resource deletion (use --delete-cloud-resources to enable)"
    fi

    # Final status
    log_section "Uninstallation Complete"

    if [[ "${DRY_RUN}" == "false" ]]; then
        # Show summary of what was deleted
        echo
        log_info "Summary:"
        [[ "${DELETE_K8S_RESOURCES}" == "true" ]] && log_info "  ✓ K8s resources and namespace"
        [[ "${DELETE_CLOUD_RESOURCES}" == "true" ]] && log_info "  ✓ Cloud resources"

        echo
        if [[ ${uninstall_errors} -eq 0 ]]; then
            log_success "Uninstallation completed successfully!"
        else
            log_error "Uninstallation completed with ${uninstall_errors} error(s)"
            log_error "Please check the logs above for details"
            exit 1
        fi
    else
        log_info "[DRY-RUN] Uninstallation simulation complete"
    fi

    # Clean up work directory
    if [[ -d "${WORK_DIR}" ]]; then
        log_verbose "Cleaning up work directory"
        rm -rf "${WORK_DIR}"
    fi
}

# ============================================================================
# Main Entry Point
# ============================================================================

main() {
    parse_arguments "$@"

    log_section "CLM Components Deployment Script"
    log_info "Action: ${ACTION}"
    log_info "Namespace: ${NAMESPACE}"
    log_info "Dry-run: ${DRY_RUN}"
    log_info "Verbose: ${VERBOSE}"

    if [[ "${VERBOSE}" == "true" ]]; then
        echo
        log_verbose "Component Configuration:"
        log_verbose "  API: ${INSTALL_API} (${API_IMAGE_REPO}:${API_IMAGE_TAG})"
        log_verbose "  Sentinel: ${INSTALL_SENTINEL} (${SENTINEL_IMAGE_REPO}:${SENTINEL_IMAGE_TAG})"
        log_verbose "  Adapter: ${INSTALL_ADAPTER} (${ADAPTER_IMAGE_REPO}:${ADAPTER_IMAGE_TAG})"
    fi

    case "${ACTION}" in
        install)
            perform_install
            ;;
        uninstall)
            perform_uninstall
            ;;
    esac
}

# Run main function
main "$@"
