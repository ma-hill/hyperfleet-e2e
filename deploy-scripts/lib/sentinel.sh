#!/usr/bin/env bash

# sentinel.sh - HyperFleet Sentinel component deployment functions
#
# This module handles installation and uninstallation of HyperFleet Sentinel instances
# for both clusters and nodepools resource types

# ============================================================================
# Sentinel Component Functions
# ============================================================================

install_sentinel_instance() {
  local resource_type="$1" # "clusters" or "nodepools"

  local component_name="Sentinel (${resource_type})"
  local release_name="sentinel-${resource_type}"
  local full_chart_path="${WORK_DIR}/sentinel/${SENTINEL_CHART_PATH}"

  log_section "Installing ${component_name}"

  # Determine API base URL
  local api_url="${API_BASE_URL}"

  if [[ "${DRY_RUN}" == "true" ]]; then
    log_info "[DRY-RUN] Would install ${component_name} with:"
    log_info "  Release name: ${release_name}"
    log_info "  Namespace: ${NAMESPACE}"
    log_info "  Chart path: ${full_chart_path}"
    log_info "  Image: ${IMAGE_REGISTRY}/${SENTINEL_IMAGE_REPO}:${SENTINEL_IMAGE_TAG}"
    log_info "  API base URL: ${api_url} (config.clients.hyperfleetApi.baseUrl)"
    log_info "  Broker type: ${SENTINEL_BROKER_TYPE}"
    log_info "  Resource type: ${resource_type}"
    log_info "  Google Pub/Sub Project ID: ${GCP_PROJECT_ID}"
    log_info "  Google Pub/Sub Create Topic If Missing: ${SENTINEL_GOOGLEPUBSUB_CREATE_TOPIC_IF_MISSING}"
    return 0
  fi

  log_info "Installing ${component_name}..."
  log_verbose "Release name: ${release_name}"
  log_verbose "Image: ${IMAGE_REGISTRY}/${SENTINEL_IMAGE_REPO}:${SENTINEL_IMAGE_TAG}"
  log_verbose "API base URL: ${api_url}"
  log_verbose "Resource type: ${resource_type}"

  # Build helm command
  local helm_cmd=(
    helm upgrade --install
    "${release_name}"
    "${full_chart_path}"
    --namespace "${NAMESPACE}"
    --create-namespace
    --wait
    --timeout 3m
    --set "image.registry=${IMAGE_REGISTRY}"
    --set "image.repository=${SENTINEL_IMAGE_REPO}"
    --set "image.tag=${SENTINEL_IMAGE_TAG}"
    --set "config.clients.hyperfleetApi.baseUrl=${api_url}"
    --set "config.resourceType=${resource_type}"
    --set "broker.type=${SENTINEL_BROKER_TYPE}"
    --set "broker.googlepubsub.projectId=${GCP_PROJECT_ID}"
    --set "config.broker.googlepubsub.createTopicIfMissing=${SENTINEL_GOOGLEPUBSUB_CREATE_TOPIC_IF_MISSING}"
  )

  # Add message_data.owner_references configuration for nodepools resource type
  # This enables the sentinel to include ownerReferences from the Kubernetes resource
  # in the message data sent to adapters, which is required for nodepools management
  if [[ "${resource_type}" == "nodepools" ]]; then
    helm_cmd+=(
      --set "config.messageData.owner_references.id=resource.owner_references.id"
      --set "config.messageData.owner_references.href=resource.owner_references.href"
      --set "config.messageData.owner_references.kind=resource.owner_references.kind"
    )
  fi

  log_info "Executing: ${helm_cmd[*]}"

  if "${helm_cmd[@]}"; then
    log_success "${component_name} Helm release created successfully"

    # Verify pod health
    log_info "Verifying pod health..."
    if verify_pod_health "${NAMESPACE}" "app.kubernetes.io/instance=${release_name}" "${component_name}" 120 5; then
      log_success "${component_name} is running and healthy"
    else
      log_error "${component_name} deployment failed health check"

      # Capture debug logs before cleanup
      local debug_log_dir="${DEBUG_LOG_DIR:-${WORK_DIR}/debug-logs}"
      capture_debug_logs "${NAMESPACE}" "app.kubernetes.io/instance=${release_name}" "${release_name}" "${debug_log_dir}"

      # Cleanup failed deployment
      log_warning "Cleaning up failed ${component_name} deployment: ${release_name}"
      if helm uninstall "${release_name}" -n "${NAMESPACE}" --wait --timeout 5m; then
        log_info "Failed ${component_name} deployment cleaned up successfully"
      else
        log_warning "Failed to cleanup ${component_name} deployment, it may need manual cleanup"
      fi
      return 1
    fi
  else
    log_error "Failed to install ${component_name}"

    # Check if release was created (partial deployment) and cleanup
    if helm list -n "${NAMESPACE}" 2>/dev/null | grep -q "^${release_name}"; then
      # Capture debug logs before cleanup
      local debug_log_dir="${DEBUG_LOG_DIR:-${WORK_DIR}/debug-logs}"
      capture_debug_logs "${NAMESPACE}" "app.kubernetes.io/instance=${release_name}" "${release_name}" "${debug_log_dir}"

      log_warning "Cleaning up failed ${component_name} deployment: ${release_name}"
      if helm uninstall "${release_name}" -n "${NAMESPACE}" --wait --timeout 5m; then
        log_info "Failed ${component_name} deployment cleaned up successfully"
      else
        log_warning "Failed to cleanup ${component_name} deployment, it may need manual cleanup"
      fi
    fi
    return 1
  fi
}

install_sentinel() {

  install_sentinel_instance "clusters" || return 1
  install_sentinel_instance "nodepools" || return 1
}

uninstall_sentinel_instance() {
  local resource_type="$1" # "clusters" or "nodepools"

  # Capitalize first letter for display
  local resource_type_display
  if [[ "${resource_type}" == "clusters" ]]; then
    resource_type_display="Clusters"
  else
    resource_type_display="Nodepools"
  fi

  local component_name="Sentinel (${resource_type_display})"
  local release_name="sentinel-${resource_type}"

  log_section "Uninstalling ${component_name}"

  # Check if release exists
  if ! helm list -n "${NAMESPACE}" | grep -q "^${release_name}"; then
    log_warning "Release '${release_name}' not found in namespace '${NAMESPACE}'"
    return 0
  fi

  if [[ "${DRY_RUN}" == "true" ]]; then
    log_info "[DRY-RUN] Would uninstall ${component_name} (release: ${release_name})"
    return 0
  fi

  log_info "Uninstalling ${component_name}..."
  log_info "Executing: helm uninstall ${release_name} -n ${NAMESPACE} --wait --timeout 5m"

  if helm uninstall "${release_name}" -n "${NAMESPACE}" --wait --timeout 5m; then
    log_success "${component_name} uninstalled successfully"
  else
    log_error "Failed to uninstall ${component_name}"
    return 1
  fi
}

uninstall_sentinel() {
  # Uninstall in reverse order
  uninstall_sentinel_instance "nodepools" || log_warning "Failed to uninstall Sentinel (Nodepools)"
  uninstall_sentinel_instance "clusters" || log_warning "Failed to uninstall Sentinel (Clusters)"
}
