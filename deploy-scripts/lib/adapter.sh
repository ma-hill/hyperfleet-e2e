#!/usr/bin/env bash

# adapter.sh - HyperFleet Adapter component deployment functions
#
# This module handles discovery, installation, and uninstallation of adapters
# from the ${ADAPTERS_FILE_DIR} directory (defaults to ${TESTDATA_DIR}/adapter-configs)

# ============================================================================
# Adapter Discovery Functions
# ============================================================================

discover_adapters() {
  # Use ADAPTERS_FILE_DIR env var, fallback to default
  local adapter_configs_dir="${ADAPTERS_FILE_DIR:-${TESTDATA_DIR}/adapter-configs}"

  if [[ ! -d "${adapter_configs_dir}" ]]; then
    log_verbose "Adapter configs directory not found: ${adapter_configs_dir}" >&2
    return 1
  fi

  # Read adapter names from environment variables
  local cluster_adapters="${CLUSTER_TIER0_ADAPTERS_DEPLOYMENT:-}"
  local nodepool_adapters="${NODEPOOL_TIER0_ADAPTERS_DEPLOYMENT:-}"

  if [[ -z "${cluster_adapters}" && -z "${nodepool_adapters}" ]]; then
    log_error "No adapters specified. Set CLUSTER_TIER0_ADAPTERS_DEPLOYMENT and/or NODEPOOL_TIER0_ADAPTERS_DEPLOYMENT" >&2
    return 1
  fi

  # Build list of adapter directories from environment variables
  local adapter_dirs=()

  # Add cluster adapters
  if [[ -n "${cluster_adapters}" ]]; then
    IFS=',' read -ra cluster_adapter_array <<<"${cluster_adapters}"
    for adapter_name in "${cluster_adapter_array[@]}"; do
      # Trim whitespace
      adapter_name=$(echo "${adapter_name}" | xargs)
      # Validate adapter name is not empty (prevents issues from trailing commas)
      if [[ -z "${adapter_name}" ]]; then
        log_error "Empty adapter name in CLUSTER_TIER0_ADAPTERS_DEPLOYMENT (check for trailing commas)" >&2
        return 1
      fi
      if [[ -d "${adapter_configs_dir}/${adapter_name}" ]]; then
        adapter_dirs+=("clusters|${adapter_name}")
      else
        log_error "Cluster adapter directory not found: ${adapter_configs_dir}/${adapter_name}" >&2
        return 1
      fi
    done
  fi

  # Add nodepool adapters
  if [[ -n "${nodepool_adapters}" ]]; then
    IFS=',' read -ra nodepool_adapter_array <<<"${nodepool_adapters}"
    for adapter_name in "${nodepool_adapter_array[@]}"; do
      # Trim whitespace
      adapter_name=$(echo "${adapter_name}" | xargs)
      # Validate adapter name is not empty (prevents issues from trailing commas)
      if [[ -z "${adapter_name}" ]]; then
        log_error "Empty adapter name in NODEPOOL_TIER0_ADAPTERS_DEPLOYMENT (check for trailing commas)" >&2
        return 1
      fi
      if [[ -d "${adapter_configs_dir}/${adapter_name}" ]]; then
        adapter_dirs+=("nodepools|${adapter_name}")
      else
        log_error "NodePool adapter directory not found: ${adapter_configs_dir}/${adapter_name}" >&2
        return 1
      fi
    done
  fi

  if [[ ${#adapter_dirs[@]} -eq 0 ]]; then
    log_verbose "No adapter configurations found" >&2
    return 1
  fi

  log_info "Found ${#adapter_dirs[@]} adapter(s) to deploy:" >&2
  for dir in "${adapter_dirs[@]}"; do
    log_info "  - ${dir}" >&2
  done

  # Export for use in other functions
  # Format: resource_type|adapter_name (e.g., "clusters|cl-namespace")
  printf '%s\n' "${adapter_dirs[@]}"
}

# ============================================================================
# Adapter Installation Functions
# ============================================================================

install_adapter_instance() {
  local dir_name="$1"

  log_section "Installing Adapter: ${dir_name}"

  # Extract resource_type and adapter_name from format: resource_type|adapter_name
  local resource_type="${dir_name%%|*}"
  local adapter_name="${dir_name##*|}"

  # Validate the descriptor format and ensure both parts are non-empty
  if [[ -z "${resource_type}" || -z "${adapter_name}" || "${dir_name}" != *"|"* ]]; then
    log_error "Invalid adapter descriptor '${dir_name}'. Expected format: resource_type|adapter_name"
    return 1
  fi

  log_info "Resource type: ${resource_type}"
  log_info "Adapter name: ${adapter_name}"

  # Temporary workaround for installation - HYPERFLEET-1097
  local release_name="${adapter_name}"

  log_info "Release name: ${release_name} (length: ${#release_name})"

  # Source adapter config directory (using ADAPTERS_FILE_DIR env var)
  local adapter_configs_dir="${ADAPTERS_FILE_DIR:-${TESTDATA_DIR}/adapter-configs}"
  local source_adapter_dir="${adapter_configs_dir}/${adapter_name}"

  if [[ ! -d "${source_adapter_dir}" ]]; then
    log_error "Adapter config directory not found: ${source_adapter_dir}"
    return 1
  fi

  # Chart path
  local full_chart_path="${WORK_DIR}/adapter/${ADAPTER_CHART_PATH}"

  # Copy adapter config folder to chart directory
  local dest_adapter_dir="${full_chart_path}/${adapter_name}"
  log_info "Copying adapter config from ${source_adapter_dir} to ${dest_adapter_dir}"

  if [[ -d "${dest_adapter_dir}" ]]; then
    # Safety check: ensure dest_adapter_dir contains adapter_name to prevent accidental deletion
    if [[ "${dest_adapter_dir}" != *"${adapter_name}" || "${dest_adapter_dir}" == "/" || "${dest_adapter_dir}" == "${full_chart_path}" ]]; then
      log_error "Safety check failed: refusing to delete suspicious path: ${dest_adapter_dir}"
      return 1
    fi
    log_verbose "Removing existing adapter config directory: ${dest_adapter_dir}"
    rm -rf "${dest_adapter_dir}"
  fi

  cp -r "${source_adapter_dir}" "${dest_adapter_dir}"

  # Patch imagePullPolicy in task resource YAMLs (what adapters create at runtime, e.g. Jobs/Deployments);
  # helm --set image.pullPolicy only controls the adapter pod itself — these are separate concerns
  if [[ "${IMAGE_PULL_POLICY}" != "Always" ]]; then
    log_verbose "Patching imagePullPolicy to ${IMAGE_PULL_POLICY} in adapter resource manifests"
    find "${dest_adapter_dir}" -name '*.yaml' -exec \
      sed -i.bak "s/imagePullPolicy: Always/imagePullPolicy: ${IMAGE_PULL_POLICY}/g" {} +
    find "${dest_adapter_dir}" -name '*.bak' -delete 2>/dev/null
  fi

  # Values file path (now in the chart directory)
  local values_file="${dest_adapter_dir}/values.yaml"
  if [[ ! -f "${values_file}" ]]; then
    log_error "Values file not found: ${values_file}"
    return 1
  fi

  # Construct subscription ID and topic names
  # Allow override from environment variables, otherwise use auto-generated defaults
  local subscription_id="${ADAPTER_SUBSCRIPTION_ID:-${NAMESPACE}-${resource_type}-${adapter_name}}"
  local topic="${ADAPTER_TOPIC:-${NAMESPACE}-${resource_type}}"
  local dead_letter_topic="${ADAPTER_DEAD_LETTER_TOPIC:-${NAMESPACE}-${resource_type}-dlq}"

  if [[ "${ADAPTER_BROKER_TYPE}" == "rabbitmq" && -z "${ADAPTER_BROKER_RABBITMQ_URL}" ]]; then
    log_error "ADAPTER_BROKER_RABBITMQ_URL must be set when ADAPTER_BROKER_TYPE=rabbitmq"
    return 1
  fi

  if [[ "${DRY_RUN}" == "true" ]]; then
    log_info "[DRY-RUN] Would install adapter with:"
    log_info "  Release name: ${release_name}"
    log_info "  Namespace: ${NAMESPACE}"
    log_info "  Chart path: ${full_chart_path}"
    log_info "  Values file: ${values_file}"
    log_info "  Image: ${IMAGE_REGISTRY}/${ADAPTER_IMAGE_REPO}:${ADAPTER_IMAGE_TAG}"
    log_info "  Subscription ID: ${subscription_id}"
    log_info "  Topic: ${topic}"
    log_info "  Dead Letter Topic: ${dead_letter_topic}"
    return 0
  fi

  # Build helm command with labels to track adapter metadata
  local helm_cmd=(
    helm upgrade --install
    "${release_name}"
    "${full_chart_path}"
    --namespace "${NAMESPACE}"
    --create-namespace
    --wait
    --timeout 5m
    -f "${values_file}"
    --set "fullnameOverride=${release_name}"
    --set "image.registry=${IMAGE_REGISTRY}"
    --set "image.repository=${ADAPTER_IMAGE_REPO}"
    --set "image.tag=${ADAPTER_IMAGE_TAG}"
    --set "image.pullPolicy=${IMAGE_PULL_POLICY}"
    --set "broker.type=${ADAPTER_BROKER_TYPE}"
    --set "broker.googlepubsub.projectId=${GCP_PROJECT_ID}"
    --set "broker.googlepubsub.createTopicIfMissing=${ADAPTER_GOOGLEPUBSUB_CREATE_TOPIC_IF_MISSING}"
    --set "broker.googlepubsub.createSubscriptionIfMissing=${ADAPTER_GOOGLEPUBSUB_CREATE_SUBSCRIPTION_IF_MISSING}"
    --set "broker.googlepubsub.subscriptionId=${subscription_id}"
    --set "broker.googlepubsub.topic=${topic}"
    --set "broker.googlepubsub.deadLetterTopic=${dead_letter_topic}"
    --labels "adapter-resource-type=${resource_type},adapter-name=${adapter_name}"
  )

  if [[ "${ADAPTER_BROKER_TYPE}" == "rabbitmq" && -n "${ADAPTER_BROKER_RABBITMQ_URL}" ]]; then
    local rabbitmq_queue="${ADAPTER_RABBITMQ_QUEUE:-${subscription_id}}"
    local rabbitmq_exchange="${ADAPTER_RABBITMQ_EXCHANGE:-${topic}}"
    local rabbitmq_routing_key="${ADAPTER_RABBITMQ_ROUTING_KEY:-#}"
    helm_cmd+=(
      --set "broker.rabbitmq.url=${ADAPTER_BROKER_RABBITMQ_URL}"
      --set "broker.rabbitmq.queue=${rabbitmq_queue}"
      --set "broker.rabbitmq.exchange=${rabbitmq_exchange}"
      --set "broker.rabbitmq.routingKey=${rabbitmq_routing_key}"
    )
  fi

  log_info "Executing Helm command:"
  log_info "${helm_cmd[*]}"
  echo

  if "${helm_cmd[@]}"; then
    log_success "Adapter ${adapter_name} for ${resource_type} Helm release created successfully"

    # Verify pod health
    log_info "Verifying pod health..."
    if verify_pod_health "${NAMESPACE}" "app.kubernetes.io/instance=${release_name}" "${adapter_name}" 120 5; then
      log_success "Adapter ${adapter_name} for ${resource_type} is running and healthy"
    else
      log_error "Adapter ${adapter_name} for ${resource_type} deployment failed health check"

      # Capture debug logs before cleanup
      local debug_log_dir="${DEBUG_LOG_DIR:-${WORK_DIR}/debug-logs}"
      capture_debug_logs "${NAMESPACE}" "app.kubernetes.io/instance=${release_name}" "${release_name}" "${debug_log_dir}"

      # Cleanup failed deployment
      log_warning "Cleaning up failed adapter deployment: ${release_name}"
      if helm uninstall "${release_name}" -n "${NAMESPACE}" --wait --timeout 5m; then
        log_info "Failed adapter deployment cleaned up successfully"
      else
        log_warning "Failed to cleanup adapter deployment, it may need manual cleanup"
      fi
      return 1
    fi
  else
    log_error "Failed to install adapter ${adapter_name} for ${resource_type}"

    # Check if release was created (partial deployment) and cleanup
    if helm list -n "${NAMESPACE}" 2>/dev/null | grep -q "^${release_name}"; then
      # Capture debug logs before cleanup
      local debug_log_dir="${DEBUG_LOG_DIR:-${WORK_DIR}/debug-logs}"
      capture_debug_logs "${NAMESPACE}" "app.kubernetes.io/instance=${release_name}" "${release_name}" "${debug_log_dir}"

      log_warning "Cleaning up failed adapter deployment: ${release_name}"
      if helm uninstall "${release_name}" -n "${NAMESPACE}" --wait --timeout 5m; then
        log_info "Failed adapter deployment cleaned up successfully"
      else
        log_warning "Failed to cleanup adapter deployment, it may need manual cleanup"
      fi
    fi
    return 1
  fi
}

install_adapters() {
  log_section "Deploying All Adapters"

  # Discover adapters
  local adapters
  if ! adapters=$(discover_adapters); then
    log_warning "No adapters found to deploy"
    return 0
  fi

  # Install each adapter
  local failed=0
  while IFS= read -r adapter_dir; do
    if ! install_adapter_instance "${adapter_dir}"; then
      log_warning "Failed to install adapter: ${adapter_dir}"
      ((failed++))
    fi
  done <<<"${adapters}"

  if [[ ${failed} -gt 0 ]]; then
    log_error "${failed} adapter(s) failed to install"
    return 1
  else
    log_success "All adapters deployed successfully"
  fi
}

# ============================================================================
# Adapter Uninstallation Functions
# ============================================================================

uninstall_adapter_instance() {
  local dir_name="$1"

  log_section "Uninstalling Adapter: ${dir_name}"

  # Extract resource_type and adapter_name from format: resource_type|adapter_name
  local resource_type="${dir_name%%|*}"
  local adapter_name="${dir_name##*|}"

  # Validate the descriptor format and ensure both parts are non-empty
  if [[ -z "${resource_type}" || -z "${adapter_name}" || "${dir_name}" != *"|"* ]]; then
    log_error "Invalid adapter descriptor '${dir_name}'. Expected format: resource_type|adapter_name"
    return 1
  fi

  log_info "Resource type: ${resource_type}"
  log_info "Adapter name: ${adapter_name}"

  # Find all releases by searching for Helm labels (avoids pattern matching issues with truncated names)
  log_info "Searching for releases with labels: adapter-resource-type=${resource_type}, adapter-name=${adapter_name}"
  local matching_releases
  matching_releases=$(helm list -n "${NAMESPACE}" --selector "adapter-resource-type=${resource_type},adapter-name=${adapter_name}" -q 2>/dev/null)

  if [[ -z "${matching_releases}" ]]; then
    # Fallback: search by name prefix for releases created before labels were added
    log_info "No releases found with labels. Trying fallback search by name prefix..."
    local name_prefix="adapter-${resource_type}-${adapter_name}"
    matching_releases=$(helm list -n "${NAMESPACE}" -q 2>/dev/null | grep "^${name_prefix}" || true)

    if [[ -z "${matching_releases}" ]]; then
      log_warning "No releases found for adapter-resource-type=${resource_type}, adapter-name=${adapter_name} in namespace '${NAMESPACE}'"
      return 0
    else
      log_info "Found releases using name prefix fallback: ${matching_releases}"
    fi
  fi

  # Uninstall all matching releases
  local uninstall_errors=0
  while IFS= read -r release_name; do
    if [[ "${DRY_RUN}" == "true" ]]; then
      log_info "[DRY-RUN] Would uninstall adapter (release: ${release_name})"
    else
      log_info "Uninstalling adapter ${adapter_name} for ${resource_type} (release: ${release_name})..."
      log_info "Executing: helm uninstall ${release_name} -n ${NAMESPACE} --wait --timeout 5m"

      if helm uninstall "${release_name}" -n "${NAMESPACE}" --wait --timeout 5m; then
        log_success "Adapter ${adapter_name} for ${resource_type} (release: ${release_name}) uninstalled successfully"
      else
        log_error "Failed to uninstall adapter ${adapter_name} for ${resource_type} (release: ${release_name})"
        ((uninstall_errors++))
      fi
    fi
  done <<<"${matching_releases}"

  if [[ ${uninstall_errors} -gt 0 ]]; then
    return 1
  fi
  return 0
}

uninstall_adapters() {
  log_section "Uninstalling All Adapters"

  # Discover adapters
  local adapters
  if ! adapters=$(discover_adapters); then
    log_warning "No adapters found to uninstall"
    return 0
  fi

  # Uninstall each adapter
  local failed=0
  while IFS= read -r adapter_dir; do
    if ! uninstall_adapter_instance "${adapter_dir}"; then
      log_warning "Failed to uninstall adapter: ${adapter_dir}"
      ((failed++))
    fi
  done <<<"${adapters}"

  if [[ ${failed} -gt 0 ]]; then
    log_error "${failed} adapter(s) failed to uninstall"
    return 1
  else
    log_success "All adapters uninstalled successfully"
  fi
}
