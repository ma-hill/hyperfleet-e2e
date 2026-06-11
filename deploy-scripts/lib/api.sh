#!/usr/bin/env bash

# api.sh - HyperFleet API component deployment functions
#
# This module handles installation and uninstallation of the HyperFleet API component

# ============================================================================
# API Component Functions
# ============================================================================

install_api() {
    log_section "Installing API"

    local release_name="hyperfleet-api"
    local full_chart_path="${WORK_DIR}/api/${API_CHART_PATH}"

    # Use API_ADAPTERS_* environment variables for API configuration
    # These should be set dynamically based on specific test case requirements
    local cluster_adapters="${API_ADAPTERS_CLUSTER:-}"
    local nodepool_adapters="${API_ADAPTERS_NODEPOOL:-}"

    log_info "API Adapter Configuration:"
    log_info "  Cluster adapters: ${cluster_adapters:-<none>}"
    log_info "  NodePool adapters: ${nodepool_adapters:-<none>}"

    if [[ "${DRY_RUN}" == "true" ]]; then
        log_info "[DRY-RUN] Would install API with:"
        log_info "  Release name: ${release_name}"
        log_info "  Namespace: ${NAMESPACE}"
        log_info "  Chart path: ${full_chart_path}"
        log_info "  Image: ${IMAGE_REGISTRY}/${API_IMAGE_REPO}:${API_IMAGE_TAG}"
        log_info "  Service type: ${API_SERVICE_TYPE}"
        [[ -n "${cluster_adapters}" ]] && log_info "  Cluster adapters: ${cluster_adapters}"
        [[ -n "${nodepool_adapters}" ]] && log_info "  Nodepool adapters: ${nodepool_adapters}"
        return 0
    fi

    log_info "Installing API..."
    log_verbose "Release name: ${release_name}"
    log_verbose "Image: ${IMAGE_REGISTRY}/${API_IMAGE_REPO}:${API_IMAGE_TAG}"

    # Build helm command with image overrides
    local helm_cmd=(
        helm upgrade --install
        "${release_name}"
        "${full_chart_path}"
        --namespace "${NAMESPACE}"
        --create-namespace
        --wait
        --timeout 3m
        --set "image.registry=${IMAGE_REGISTRY}"
        --set "image.repository=${API_IMAGE_REPO}"
        --set "image.tag=${API_IMAGE_TAG}"
        --set "image.pullPolicy=${IMAGE_PULL_POLICY}"
        --set "service.type=${API_SERVICE_TYPE}"
    )

    # Add adapter configurations (always set both, use empty if not discovered)
    # The API chart requires both config.adapters.required.cluster and config.adapters.required.nodepool to be set
    if [[ -n "${cluster_adapters}" ]]; then
        helm_cmd+=(--set "config.adapters.required.cluster={${cluster_adapters}}")
        log_verbose "Cluster adapters (API): ${cluster_adapters}"
    else
        helm_cmd+=(--set "config.adapters.required.cluster={}")
        log_verbose "Cluster adapters (API): none"
    fi

    if [[ -n "${nodepool_adapters}" ]]; then
        helm_cmd+=(--set "config.adapters.required.nodepool={${nodepool_adapters}}")
        log_verbose "Nodepool adapters (API): ${nodepool_adapters}"
    else
        helm_cmd+=(--set "config.adapters.required.nodepool={}")
        log_verbose "Nodepool adapters (API): none"
    fi

    log_info "Executing: ${helm_cmd[*]}"

    if "${helm_cmd[@]}"; then
        log_success "API Helm release created successfully"

        # Verify pod health
        log_info "Verifying pod health..."
        if verify_pod_health "${NAMESPACE}" "app.kubernetes.io/instance=${release_name}" "API" 120 5; then
            log_success "API is running and healthy"
        else
            log_error "API deployment failed health check"

            # Capture debug logs before cleanup
            local debug_log_dir="${DEBUG_LOG_DIR:-${WORK_DIR}/debug-logs}"
            capture_debug_logs "${NAMESPACE}" "app.kubernetes.io/instance=${release_name}" "${release_name}" "${debug_log_dir}"

            # Cleanup failed deployment
            log_warning "Cleaning up failed API deployment: ${release_name}"
            if helm uninstall "${release_name}" -n "${NAMESPACE}" --wait --timeout 5m; then
                log_info "Failed API deployment cleaned up successfully"
            else
                log_warning "Failed to cleanup API deployment, it may need manual cleanup"
            fi
            return 1
        fi
    else
        log_error "Failed to install API"

        # Check if release was created (partial deployment) and cleanup
        if helm list -n "${NAMESPACE}" 2>/dev/null | grep -q "^${release_name}"; then
            # Capture debug logs before cleanup
            local debug_log_dir="${DEBUG_LOG_DIR:-${WORK_DIR}/debug-logs}"
            capture_debug_logs "${NAMESPACE}" "app.kubernetes.io/instance=${release_name}" "${release_name}" "${debug_log_dir}"

            log_warning "Cleaning up failed API deployment: ${release_name}"
            if helm uninstall "${release_name}" -n "${NAMESPACE}" --wait --timeout 5m; then
                log_info "Failed API deployment cleaned up successfully"
            else
                log_warning "Failed to cleanup API deployment, it may need manual cleanup"
            fi
        fi
        return 1
    fi
}

uninstall_api() {
    log_section "Uninstalling API"

    local release_name="hyperfleet-api"

    # Check if release exists
    if [[ -z "$(helm list -n "${NAMESPACE}" -q -f "^${release_name}$")" ]]; then
        log_warning "Release '${release_name}' not found in namespace '${NAMESPACE}'"
        return 0
    fi

    if [[ "${DRY_RUN}" == "true" ]]; then
        log_info "[DRY-RUN] Would uninstall API (release: ${release_name})"
        return 0
    fi

    log_info "Uninstalling API..."
    log_info "Executing: helm uninstall ${release_name} -n ${NAMESPACE} --wait --timeout 5m"

    if helm uninstall "${release_name}" -n "${NAMESPACE}" --wait --timeout 5m; then
        log_success "API uninstalled successfully"
    else
        log_error "Failed to uninstall API"
        return 1
    fi
}
