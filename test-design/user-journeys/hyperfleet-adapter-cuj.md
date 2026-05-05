# HyperFleet Adapter - Customer Critical Journey (MVP)
**Status**: Active
**Owner**: Ying Zhang
**Last Updated**: 2025-11-17

## Overview

This document describes the critical user journeys for **partners building custom adapters** in the CLM. It focuses on the adapter lifecycle from the adapter's perspective and showcases CLM's **pluggable adapter framework** as the key differentiator.

**Target Audience:**
- **Partners** - Building custom adapters to extend cluster lifecycle with provider-specific or custom workflows
- **Stakeholders** - Understanding the customization capability and partner value proposition

**Key Innovation:** HyperFleet's adapter framework enables partners to customize cluster provisioning workflows without modifying the core platform.

---

## MVP Scope

### In Scope for MVP

**Dedicated Adapter Journeys (Adapter Lifecycle):**
- [**Journey A1**: Adapter Processes Event and Creates Resources Successfully](#journey-a1-adapter-processes-event-and-creates-resources-successfully)
- [**Journey A2**: Adapter Monitors Workload Progress and Reports Success](#journey-a2-adapter-monitors-workload-progress-and-reports-success)
- [**Journey A3**: Adapter Skips Work Due to Preconditions Not Met](#journey-a3-adapter-skips-work-due-to-preconditions-not-met)
- [**Journey A4 (Sad Path)**: Adapter Resource Creation Failures](#journey-a4-sad-path-adapter-resource-creation-failures)
- [**Journey A5 (Sad Path)**: Adapter Workload Execution Failures](#journey-a5-sad-path-adapter-workload-execution-failures)
- [**Journey A6 (Sad Path)**: Adapter Health Errors](#journey-a6-sad-path-adapter-health-errors)

---

# Dedicated Adapter Journeys

## What Makes Adapters Special: Customization Power

**The Key Innovation:** HyperFleet's adapter framework enables **partners to design custom adapters** for their specific requirements without changing the core platform. Each adapter is a **declarative YAML configuration** that defines:
- **When to act** (preconditions: dependencies, provider-specific checks, resource creation)
- **What to do** (create Jobs, Deployments, or any Kubernetes resource)
- **How to verify success** (postconditions: custom validation logic)
- **What to report** (status conditions visible to users)

This means partners can:
- Build adapters for ANY cloud provider (AWS, GCP, Oracle ...)
- Implement custom workflows
- Define their own success criteria and error handling
- Extend cluster lifecycle without platform code changes

For detailed adapter architecture, see:
- [Adapter Configuration Framework](https://github.com/openshift-hyperfleet/architecture/blob/main/hyperfleet/components/adapter/framework/adapter-config-template.yaml)
- [Adapter Flow Diagrams](https://github.com/openshift-hyperfleet/architecture/blob/main/hyperfleet/docs/adapter-flow-diagrams.md)
- [Status Reporting Guide](https://github.com/openshift-hyperfleet/architecture/blob/main/hyperfleet/docs/status-guide.md)

---

## Generic Adapter Workflow

**Standard Workflow (applies to all adapters):**
1. **Event Trigger** - Receives CloudEvent when cluster/nodepool is created or updated
2. **Precondition Check** - Evaluates if dependencies are met and adapter should proceed
3. **Create Resources** - Creates Kubernetes workloads (Jobs/Deployments) to execute adapter-specific logic
4. **Monitor Progress** - Evaluates postconditions to determine if work completed successfully
5. **Report Status** - Updates cluster/nodepool with three required conditions: Available, Applied, Health

**Key Points:**
- Each adapter defines its own preconditions, resource templates, and postconditions via YAML configuration
- Adapters can be for any purpose: infrastructure provisioning, DNS setup, security scanning, compliance checks, etc.
- All adapters follow the same lifecycle pattern but execute different business logic

---

## Standard Adapter Journeys

### Journey A1: Adapter Processes Event and Creates Resources Successfully

**Scenario:** Adapter receives cluster/nodepool creation event and begins work

**User Journey:**
1. User creates cluster/nodepool via HyperFleet API
2. HyperFleet creates resource with status `Reconciled=False`
3. **Adapter receives CloudEvent** with resource ID
4. **Adapter fetches resource details** from HyperFleet API
5. **Precondition evaluation:**
   - Checks if dependencies are met (e.g., previous adapters completed)
   - Validates resource matches adapter's supported criteria (e.g., provider, region)
6. **Preconditions met** - Adapter proceeds
7. **Resource creation:**
   - Creates Kubernetes workload (Job or Deployment) based on adapter configuration
   - Workload runs partner-specific logic (provisioning, validation, configuration, etc.)
8. **Adapter reports status:**
   - `Applied: True` (Kubernetes resources created successfully)
   - `Available: False` (work in progress)
   - `Health: True` (no unexpected errors)
   - Updates `lastUpdated` timestamp

**User Sees:**
- `/clusters/{id}/statuses` (or `/nodepools/{id}/statuses`) shows adapter status `Applied: True`
- Resource remains `Reconciled=False` until work completes

**Success Criteria:**
- Kubernetes workload created in adapter namespace
- Status reported to HyperFleet API
- No errors in adapter logs

---

### Journey A2: Adapter Monitors Workload Progress and Reports Success

**Scenario:** Adapter monitors workload completion (continues from A1)

**User Journey:**
1. **Adapter monitors workload status** via Kubernetes API
2. **Postcondition evaluation:**
   - Checks workload completion status
   - Evaluates adapter-specific validation logic (defined in postconditions)
   - Confirms work completed successfully per adapter's criteria
3. **Postconditions met** - Work completed successfully
4. **Adapter reports final status:**
   - `Available: True` (work completed)
   - `Applied: True` (resources exist)
   - `Health: True` (no errors)
   - Adds adapter-specific custom conditions
5. **Optional cleanup:**
   - Adapter deletes completed workload (if configured in adapter YAML)
   - Prevents resource accumulation

**User Sees:**
- `/clusters/{id}/statuses` (or `/nodepools/{id}/statuses`) shows adapter `Available: True`
- Resource transitions to `Reconciled=True` when all required adapters complete
- Can proceed with resource usage

**Success Criteria:**
- Adapter-specific work completed successfully
- Resource status updated to `Reconciled=True`
- All adapter conditions show `True`

---

### Journey A3: Adapter Skips Work Due to Preconditions Not Met

**Scenario:** Adapter receives event but dependencies not reconciled yet

**User Journey:**
1. **Adapter receives CloudEvent** for resource update
2. **Adapter fetches resource details** from HyperFleet API
3. **Precondition evaluation:**
   - Checks if dependencies are met (e.g., previous adapter completed)
   - Finds precondition(s) not satisfied 
4. **Preconditions NOT met** - Adapter skips work
5. **Adapter reports status anyway (critical for preventing loops):**
   - `Applied: False` (no resources created)
   - `Available: False` (work not done)
   - `Health: True` (no errors, just waiting)
   - Updates `lastUpdated` timestamp
   - Message: "Waiting for dependencies to be met" (with specific details)

**User Sees:**
- `/clusters/{id}/statuses` (or `/nodepools/{id}/statuses`) shows adapter waiting
- Clear dependency chain visible in status conditions
- Resource remains `Reconciled=False`

**Success Criteria:**
- Adapter does not create resources prematurely
- Status updated with clear waiting message
- `lastUpdated` prevents infinite event loops
- Adapter will retry when dependencies are satisfied

---

### Journey A4 (Sad Path): Adapter Resource Creation Failures

**Scenario:** Adapter fails to create Kubernetes workload due to infrastructure issues

**User Journey:**
1. **Adapter receives CloudEvent** for new resource
2. **Preconditions met** - Adapter attempts resource creation
3. **Kubernetes API call fails:**
   - Error: "Exceeded quota: pods in namespace hyperfleet-adapters"
   - Or: "ImagePullBackOff: unable to pull container image"
   - Or: Other infrastructure-level failures
4. **Adapter reports failure status:**
   - `Applied: False` (failed to create resources)
   - `Available: False` (work not completed)
   - `Health: False` (unexpected infrastructure error)
   - Updates `lastUpdated` timestamp
   - Message: "Failed to create workload: [specific error details]"

**User Sees:**
- `/clusters/{id}/statuses` (or `/nodepools/{id}/statuses`) shows adapter failure
- Error message with actionable details
- Resource stuck in `Reconciled=False`
- **Troubleshooting:** Platform team resolves infrastructure issue (quota, image registry, etc.)

**Success Criteria:**
- Clear error message with remediation steps
- Health=False distinguishes from business logic failures
- User/platform team can identify and resolve infrastructure issue

---

### Journey A5 (Sad Path): Adapter Workload Execution Failures

**Scenario:** Adapter workload runs but fails due to business logic error

**User Journey:**
1. **Adapter creates workload successfully** (`Applied: True`)
2. **Workload executes partner-specific logic**
3. **Workload fails:**
   - Error: Business logic failure (e.g., invalid credentials, provider API error, validation failure)
   - Exit code: non-zero
4. **Postcondition evaluation:**
   - Workload completed but failed
   - Expected outcome NOT achieved
5. **Adapter reports failure status:**
   - `Applied: True` (resources created successfully)
   - `Available: False` (work failed)
   - `Health: True` (adapter infrastructure healthy, business logic failed)
   - Updates `lastUpdated` timestamp
   - Adds adapter-specific failure conditions
   - Message: "[Adapter-specific error]: [actionable details]"

**User Sees:**
- `/clusters/{id}/statuses` (or `/nodepools/{id}/statuses`) shows adapter business logic failure
- Clear distinction: adapter is healthy but work failed
- Actionable error message guiding resolution

**Troubleshooting Actions:**
- User updates resource spec to fix configuration issue
- HyperFleet sends new event to adapter
- Adapter retries with updated configuration

**Success Criteria:**
- Business logic failures clearly distinguished (Health=True, Available=False)
- Error messages guide user to resolution
- Adapter can retry after user fixes configuration

---

### Journey A6 (Sad Path): Adapter Health Errors

**Scenario:** Adapter encounters unexpected infrastructure errors during operation

**User Journey:**
1. **Adapter receives CloudEvent**
2. **Adapter attempts to perform operations**
3. **Unexpected infrastructure failure:**
   - Error: "Cannot connect to Kubernetes API"
   - Or: "HyperFleet API unreachable"
   - Or: Network partition or service unavailable
4. **Adapter reports health failure:**
   - `Applied: False` (could not create resources)
   - `Available: False` (work not completed)
   - `Health: False` (unexpected infrastructure error)
   - Updates `lastUpdated` timestamp
   - Message: "[Specific infrastructure error details]"

**User Sees:**
- `/clusters/{id}/statuses` (or `/nodepools/{id}/statuses`) shows adapter unhealthy
- Platform-level issue affecting adapter infrastructure
- Multiple adapters may show similar errors if widespread

**Platform Team Actions:**
- Investigate platform infrastructure health
- Check adapter deployment status
- Verify network connectivity and service availability
- Resolve infrastructure issue

**Success Criteria:**
- Health errors clearly indicate platform-level issues
- Distinguishable from business logic failures
- Platform team alerted to investigate infrastructure

---
