package kubernetes

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
)

// Client wraps kubernetes.Interface and provides business logic methods
type Client struct {
	kubernetes.Interface
}

// NewClient initializes a Kubernetes clientset from kubeconfig
func NewClient() (*Client, error) {
	// Build config from KUBECONFIG env var or default ~/.kube/config
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	config, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(
		loadingRules, &clientcmd.ConfigOverrides{}).ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	// Set user agent for observability in API server logs
	config.UserAgent = "hyperfleet-e2e-tests"

	// Create clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return &Client{Interface: clientset}, nil
}

// DeleteNamespaceAndWait deletes a namespace and waits for it to be fully removed
func (c *Client) DeleteNamespaceAndWait(ctx context.Context, namespace string) error {
	// Check if namespace exists first
	_, err := c.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return nil // Already deleted
	}

	// Delete all resources in the namespace to speed up cleanup
	// This helps avoid timeout issues when namespaces have many resources
	gracePeriod := int64(0)
	propagationPolicy := metav1.DeletePropagationForeground
	deleteOpts := metav1.DeleteOptions{
		GracePeriodSeconds: &gracePeriod,
		PropagationPolicy:  &propagationPolicy,
	}

	// Delete deployments
	_ = c.AppsV1().Deployments(namespace).DeleteCollection(ctx, deleteOpts, metav1.ListOptions{})

	// Delete jobs
	_ = c.BatchV1().Jobs(namespace).DeleteCollection(ctx, deleteOpts, metav1.ListOptions{})

	// Delete configmaps
	_ = c.CoreV1().ConfigMaps(namespace).DeleteCollection(ctx, deleteOpts, metav1.ListOptions{})

	// Delete pods
	_ = c.CoreV1().Pods(namespace).DeleteCollection(ctx, deleteOpts, metav1.ListOptions{})

	// Delete namespace with foreground propagation to ensure resources are cleaned up
	nsDeleteOpts := metav1.DeleteOptions{
		PropagationPolicy: &propagationPolicy,
	}
	err = c.CoreV1().Namespaces().Delete(ctx, namespace, nsDeleteOpts)
	if err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete namespace %s: %w", namespace, err)
	}

	// Wait for namespace to be fully deleted (garbage collection finalization)
	backoff := wait.Backoff{
		Duration: 500 * time.Millisecond,
		Factor:   1.5,
		Jitter:   0.1,
		Steps:    30,                // Increased from 20 to give more time
		Cap:      15 * time.Second, // Increased cap for better handling of stuck resources
	}
	err = wait.ExponentialBackoffWithContext(ctx, backoff, func(ctx context.Context) (bool, error) {
		_, err := c.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return true, nil // Namespace fully deleted
		}
		if err != nil {
			return false, err // Unexpected error
		}
		return false, nil // Still exists, keep polling
	})
	if err != nil {
		return fmt.Errorf("timeout waiting for namespace %s deletion: %w", namespace, err)
	}

	return nil
}

// FetchNamespace gets a namespace by name using k8s client-go
func (c *Client) FetchNamespace(ctx context.Context, name string) (*corev1.Namespace, error) {
	ns, err := c.CoreV1().Namespaces().Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("namespace %s not found", name)
		}
		return nil, fmt.Errorf("failed to get namespace %s: %w", name, err)
	}
	return ns, nil
}

// FindNamespacesByPrefix finds all namespaces whose name starts with the given prefix
func (c *Client) FindNamespacesByPrefix(ctx context.Context, prefix string) ([]string, error) {
	namespaces, err := c.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list namespaces: %w", err)
	}

	var matchingNamespaces []string
	for _, ns := range namespaces.Items {
		if len(ns.Name) >= len(prefix) && ns.Name[:len(prefix)] == prefix {
			matchingNamespaces = append(matchingNamespaces, ns.Name)
		}
	}

	return matchingNamespaces, nil
}

// FetchConfigMap gets a configmap by name in the specified namespace
func (c *Client) FetchConfigMap(ctx context.Context, namespace, name string) (*corev1.ConfigMap, error) {
	cm, err := c.CoreV1().ConfigMaps(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("configmap %s not found in namespace %s", name, namespace)
		}
		return nil, fmt.Errorf("failed to get configmap %s in namespace %s: %w", name, namespace, err)
	}
	return cm, nil
}

// FetchJobsByLabels lists jobs matching label selector in namespace
func (c *Client) FetchJobsByLabels(ctx context.Context, namespace string, labelMap map[string]string) ([]batchv1.Job, error) {
	labelSelector := labels.SelectorFromSet(labelMap).String()
	jobs, err := c.BatchV1().Jobs(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list jobs in namespace %s with selector %s: %w",
			namespace, labelSelector, err)
	}
	return jobs.Items, nil
}

// FetchDeploymentsByLabels lists deployments matching label selector in namespace
func (c *Client) FetchDeploymentsByLabels(ctx context.Context, namespace string, labelMap map[string]string) ([]appsv1.Deployment, error) {
	labelSelector := labels.SelectorFromSet(labelMap).String()
	deployments, err := c.AppsV1().Deployments(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list deployments in namespace %s with selector %s: %w",
			namespace, labelSelector, err)
	}
	return deployments.Items, nil
}

// FetchConfigMapsByLabels lists configmaps matching label selector in namespace
func (c *Client) FetchConfigMapsByLabels(ctx context.Context, namespace string, labelMap map[string]string) ([]corev1.ConfigMap, error) {
	labelSelector := labels.SelectorFromSet(labelMap).String()
	configmaps, err := c.CoreV1().ConfigMaps(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list configmaps in namespace %s with selector %s: %w",
			namespace, labelSelector, err)
	}
	return configmaps.Items, nil
}

// GetUniqueJobByLabels fetches exactly one job matching labels in namespace.
// Returns error if zero or multiple jobs are found.
func (c *Client) GetUniqueJobByLabels(ctx context.Context, namespace string, labelMap map[string]string) (*batchv1.Job, error) {
	jobs, err := c.FetchJobsByLabels(ctx, namespace, labelMap)
	if err != nil {
		return nil, err
	}

	labelSelector := labels.SelectorFromSet(labelMap).String()

	if len(jobs) == 0 {
		return nil, fmt.Errorf("no job found in namespace %s with selector %s", namespace, labelSelector)
	}
	if len(jobs) > 1 {
		return nil, fmt.Errorf("multiple jobs (%d) found in namespace %s with selector %s - expected exactly one",
			len(jobs), namespace, labelSelector)
	}

	return &jobs[0], nil
}

// GetUniqueDeploymentByLabels fetches exactly one deployment matching labels in namespace.
// Returns error if zero or multiple deployments are found.
func (c *Client) GetUniqueDeploymentByLabels(ctx context.Context, namespace string, labelMap map[string]string) (*appsv1.Deployment, error) {
	deployments, err := c.FetchDeploymentsByLabels(ctx, namespace, labelMap)
	if err != nil {
		return nil, err
	}

	labelSelector := labels.SelectorFromSet(labelMap).String()

	if len(deployments) == 0 {
		return nil, fmt.Errorf("no deployment found in namespace %s with selector %s", namespace, labelSelector)
	}
	if len(deployments) > 1 {
		return nil, fmt.Errorf("multiple deployments (%d) found in namespace %s with selector %s - expected exactly one",
			len(deployments), namespace, labelSelector)
	}

	return &deployments[0], nil
}

// GetUniqueConfigMapByLabels fetches exactly one configmap matching labels in namespace.
// Returns error if zero or multiple configmaps are found.
func (c *Client) GetUniqueConfigMapByLabels(ctx context.Context, namespace string, labelMap map[string]string) (*corev1.ConfigMap, error) {
	configmaps, err := c.FetchConfigMapsByLabels(ctx, namespace, labelMap)
	if err != nil {
		return nil, err
	}

	labelSelector := labels.SelectorFromSet(labelMap).String()

	if len(configmaps) == 0 {
		return nil, fmt.Errorf("no configmap found in namespace %s with selector %s", namespace, labelSelector)
	}
	if len(configmaps) > 1 {
		return nil, fmt.Errorf("multiple configmaps (%d) found in namespace %s with selector %s - expected exactly one",
			len(configmaps), namespace, labelSelector)
	}

	return &configmaps[0], nil
}

// HasNamespacePhase checks if namespace is in the specified phase
func HasNamespacePhase(ns *corev1.Namespace, phase corev1.NamespacePhase) bool {
	return ns.Status.Phase == phase
}

// HasJobCondition checks if job has the specified condition with expected status
func HasJobCondition(job *batchv1.Job, condType batchv1.JobConditionType, status corev1.ConditionStatus) bool {
	for _, cond := range job.Status.Conditions {
		if cond.Type == condType && cond.Status == status {
			return true
		}
	}
	return false
}

// HasDeploymentCondition checks if deployment has the specified condition with expected status
func HasDeploymentCondition(deploy *appsv1.Deployment, condType appsv1.DeploymentConditionType, status corev1.ConditionStatus) bool {
	for _, cond := range deploy.Status.Conditions {
		if cond.Type == condType && cond.Status == status {
			return true
		}
	}
	return false
}
