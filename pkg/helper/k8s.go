package helper

import (
	"context"
	"fmt"
	"sort"
	"strings"

	k8sclient "github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client/kubernetes"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/logger"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// VerifyNamespaceActive verifies a namespace exists and is in Active phase
func (h *Helper) VerifyNamespaceActive(ctx context.Context, name string, expectedLabels, expectedAnnotations map[string]string) error {
	logger.Info("verifying namespace status", "namespace", name)

	// Fetch namespace
	ns, err := h.K8sClient.FetchNamespace(ctx, name)
	if err != nil {
		return err
	}

	// Check phase
	if !k8sclient.HasNamespacePhase(ns, corev1.NamespaceActive) {
		return fmt.Errorf("namespace %s phase is %v, expected Active", ns.Name, ns.Status.Phase)
	}

	// Verify labels
	if err := verifyMapContains(ns.Labels, expectedLabels, "label"); err != nil {
		return fmt.Errorf("namespace %s: %w", name, err)
	}

	// Verify annotations
	if err := verifyMapContains(ns.Annotations, expectedAnnotations, "annotation"); err != nil {
		return fmt.Errorf("namespace %s: %w", name, err)
	}

	logger.Info("namespace verified successfully", "namespace", name, "phase", ns.Status.Phase)
	return nil
}

// VerifyJobComplete verifies a job exists and has completed successfully.
// Uses expectedLabels to find the job via label selector - if the list returns a job,
// it's guaranteed to have those labels (no need to verify them again).
func (h *Helper) VerifyJobComplete(ctx context.Context, namespace string, expectedLabels, expectedAnnotations map[string]string) error {
	labelSelector := labels.SelectorFromSet(expectedLabels).String()
	logger.Info("verifying job status", "namespace", namespace, "label_selector", labelSelector)

	// Get job (handles uniqueness validation internally)
	job, err := h.K8sClient.GetUniqueJobByLabels(ctx, namespace, expectedLabels)
	if err != nil {
		return err
	}

	// Check completion
	if !k8sclient.HasJobCondition(job, batchv1.JobComplete, corev1.ConditionTrue) {
		return fmt.Errorf("job %s in namespace %s has not completed successfully (conditions: %+v)",
			job.Name, namespace, job.Status.Conditions)
	}

	// Verify annotations
	if err := verifyMapContains(job.Annotations, expectedAnnotations, "annotation"); err != nil {
		return fmt.Errorf("job %s in namespace %s: %w", job.Name, namespace, err)
	}

	logger.Info("job verified successfully",
		"namespace", namespace,
		"job", job.Name,
		"succeeded", job.Status.Succeeded,
		"active", job.Status.Active,
		"failed", job.Status.Failed)
	return nil
}

// VerifyDeploymentAvailable verifies a deployment exists and is available.
// Uses expectedLabels to find the deployment via label selector - if the list returns a deployment,
// it's guaranteed to have those labels (no need to verify them again).
func (h *Helper) VerifyDeploymentAvailable(ctx context.Context, namespace string, expectedLabels, expectedAnnotations map[string]string) error {
	labelSelector := labels.SelectorFromSet(expectedLabels).String()
	logger.Info("verifying deployment status", "namespace", namespace, "label_selector", labelSelector)

	// Get deployment (handles uniqueness validation internally)
	deploy, err := h.K8sClient.GetUniqueDeploymentByLabels(ctx, namespace, expectedLabels)
	if err != nil {
		return err
	}

	// Check availability
	if !k8sclient.HasDeploymentCondition(deploy, appsv1.DeploymentAvailable, corev1.ConditionTrue) {
		return fmt.Errorf("deployment %s in namespace %s is not available (availableReplicas=%d, conditions: %+v)",
			deploy.Name, namespace, deploy.Status.AvailableReplicas, deploy.Status.Conditions)
	}

	// Verify annotations
	if err := verifyMapContains(deploy.Annotations, expectedAnnotations, "annotation"); err != nil {
		return fmt.Errorf("deployment %s in namespace %s: %w", deploy.Name, namespace, err)
	}

	logger.Info("deployment verified successfully",
		"namespace", namespace,
		"deployment", deploy.Name,
		"available_replicas", deploy.Status.AvailableReplicas)
	return nil
}

// VerifyConfigMap verifies a configmap exists with expected labels and annotations.
// Uses expectedLabels to find the configmap via label selector - if the list returns a configmap,
// it's guaranteed to have those labels (no need to verify them again).
func (h *Helper) VerifyConfigMap(ctx context.Context, namespace string, expectedLabels, expectedAnnotations map[string]string) error {
	labelSelector := labels.SelectorFromSet(expectedLabels).String()
	logger.Info("verifying configmap status", "namespace", namespace, "label_selector", labelSelector)

	// Get configmap (handles uniqueness validation internally)
	cm, err := h.K8sClient.GetUniqueConfigMapByLabels(ctx, namespace, expectedLabels)
	if err != nil {
		return err
	}

	// Verify annotations
	if err := verifyMapContains(cm.Annotations, expectedAnnotations, "annotation"); err != nil {
		return fmt.Errorf("configmap %s in namespace %s: %w", cm.Name, namespace, err)
	}

	logger.Info("configmap verified successfully",
		"namespace", namespace,
		"configmap", cm.Name)
	return nil
}

// GetNamespace retrieves a namespace by name.
// Returns the Namespace object so you can check its labels, annotations, and status.
func (h *Helper) GetNamespace(ctx context.Context, name string) (*corev1.Namespace, error) {
	logger.Info("fetching namespace", "namespace", name)

	ns, err := h.K8sClient.FetchNamespace(ctx, name)
	if err != nil {
		return nil, err
	}

	logger.Info("namespace fetched successfully",
		"namespace", ns.Name,
		"phase", ns.Status.Phase)
	return ns, nil
}

// GetConfigMap retrieves a configmap by name from the specified namespace.
// Returns the ConfigMap object so you can check its labels, annotations, and data.
func (h *Helper) GetConfigMap(ctx context.Context, namespace, name string) (*corev1.ConfigMap, error) {
	logger.Info("fetching configmap", "namespace", namespace, "name", name)

	cm, err := h.K8sClient.FetchConfigMap(ctx, namespace, name)
	if err != nil {
		return nil, err
	}

	logger.Info("configmap fetched successfully",
		"namespace", namespace,
		"name", cm.Name)
	return cm, nil
}

// ScaleDeployment scales a deployment to the specified number of replicas.
func (h *Helper) ScaleDeployment(ctx context.Context, namespace, name string, replicas int32) error {
	logger.Info("scaling deployment", "namespace", namespace, "name", name, "replicas", replicas)

	if err := h.K8sClient.ScaleDeployment(ctx, namespace, name, replicas); err != nil {
		return err
	}

	logger.Info("deployment scaled successfully", "namespace", namespace, "name", name, "replicas", replicas)
	return nil
}

// GetDeploymentName finds the deployment name for a Helm release by listing deployments with the release label.
func (h *Helper) GetDeploymentName(ctx context.Context, namespace, releaseName string) (string, error) {
	deployments, err := h.K8sClient.FetchDeploymentsByLabels(ctx, namespace, map[string]string{
		"app.kubernetes.io/instance": releaseName,
	})
	if err != nil {
		return "", fmt.Errorf("failed to find deployment for release %s: %w", releaseName, err)
	}
	if len(deployments) == 0 {
		return "", fmt.Errorf("no deployment found for release %s in namespace %s", releaseName, namespace)
	}
	if len(deployments) > 1 {
		return "", fmt.Errorf("found %d deployments for release %s in namespace %s, expected 1", len(deployments), releaseName, namespace)
	}
	return deployments[0].Name, nil
}

// verifyMapContains checks if actual map contains all expected key-value pairs
func verifyMapContains(actual, expected map[string]string, mapType string) error {
	missing := make([]string, 0, len(expected))
	mismatched := make([]string, 0, len(expected))

	for key, expectedValue := range expected {
		actualValue, exists := actual[key]
		if !exists {
			missing = append(missing, key)
			continue
		}

		if actualValue != expectedValue {
			mismatched = append(mismatched, fmt.Sprintf("%s (expected: %s, actual: %s)",
				key, expectedValue, actualValue))
		}
	}

	if len(missing) > 0 || len(mismatched) > 0 {
		var errParts []string
		if len(missing) > 0 {
			sort.Strings(missing)
			errParts = append(errParts, fmt.Sprintf("missing %ss: %s", mapType, strings.Join(missing, ", ")))
		}
		if len(mismatched) > 0 {
			sort.Strings(mismatched)
			errParts = append(errParts, fmt.Sprintf("mismatched %ss: %s", mapType, strings.Join(mismatched, "; ")))
		}
		return fmt.Errorf("%s", strings.Join(errParts, "; "))
	}

	return nil
}
