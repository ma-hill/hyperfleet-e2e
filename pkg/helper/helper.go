package helper

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	k8sclient "github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client/kubernetes"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client/maestro"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/logger"
)

// Helper provides utility functions for e2e tests
type Helper struct {
	Cfg           *config.Config
	Client        *client.HyperFleetClient
	K8sClient     *k8sclient.Client
	MaestroClient *maestro.Client
}

// TestDataPath resolves a relative path within the testdata directory
// This ensures testdata paths work correctly whether invoked via go test or the e2e binary
func (h *Helper) TestDataPath(relativePath string) string {
	return filepath.Join(h.Cfg.TestDataDir, relativePath)
}

// GetTestCluster creates a new temporary test cluster
func (h *Helper) GetTestCluster(ctx context.Context, payloadPath string) (string, error) {
	cluster, err := h.Client.CreateClusterFromPayload(ctx, payloadPath)
	if err != nil {
		return "", err
	}
	if cluster == nil {
		return "", fmt.Errorf("CreateClusterFromPayload returned nil")
	}
	if cluster.Id == nil {
		return "", fmt.Errorf("created cluster has no ID")
	}
	return *cluster.Id, nil
}

// CleanupTestCluster deletes the temporary test cluster and resources created by adapters from CLUSTER_TIER0_ADAPTERS_DEPLOYMENT
// TODO: Replace this workaround with API DELETE once HyperFleet API supports
// DELETE operations for clusters resource type:
//
//	return h.Client.DeleteCluster(ctx, clusterID)
//
// Temporary workaround: delete the Kubernetes namespace and adapter resources using client-go
//
// IMPORTANT: This function continues cleanup even if errors occur, to ensure maximum cleanup effort.
// However, all errors are accumulated and returned at the end to avoid hiding failures that could
// cause test pollution (e.g., stale Maestro state being read by subsequent tests).
func (h *Helper) CleanupTestCluster(ctx context.Context, clusterID string) error {
	logger.Info("cleaning up cluster resources", "cluster_id", clusterID)

	// Guard against nil K8sClient
	if h == nil || h.K8sClient == nil {
		err := fmt.Errorf("K8sClient is nil, cannot delete resources")
		logger.Error("K8sClient is nil", "cluster_id", clusterID)
		return err
	}

	// Accumulate errors but continue cleanup to maximize cleanup effort
	var cleanupErrors []error

	// Step 1: Delete all Maestro resource bundles (ManifestWorks) for this cluster
	// Multiple adapters may create ManifestWorks for the same cluster (e.g., cl-maestro, cl-m-wrong-ds)
	maestroClient := h.GetMaestroClient()
	if maestroClient != nil {
		logger.Info("attempting to delete all maestro resource bundles for cluster", "cluster_id", clusterID)

		rbs, err := maestroClient.FindAllResourceBundlesByClusterID(ctx, clusterID)
		if err != nil {
			logger.Error("failed to find maestro resource bundles", "cluster_id", clusterID, "error", err)
			cleanupErrors = append(cleanupErrors, fmt.Errorf("failed to find maestro resource bundles: %w", err))
		} else if len(rbs) > 0 {
			logger.Info("found maestro resource bundles", "cluster_id", clusterID, "count", len(rbs))

			// Delete all resource bundles - this triggers Maestro agent to clean up K8s resources
			for _, rb := range rbs {
				if err := maestroClient.DeleteResourceBundle(ctx, rb.ID); err != nil {
					logger.Error("failed to delete maestro resource bundle", "cluster_id", clusterID, "resource_bundle_id", rb.ID, "error", err)
					cleanupErrors = append(cleanupErrors, fmt.Errorf("failed to delete maestro resource bundle %s: %w", rb.ID, err))
				} else {
					logger.Info("successfully deleted maestro resource bundle", "cluster_id", clusterID, "resource_bundle_id", rb.ID)
				}
			}
			// Wait for Maestro agent to clean up K8s resources
			// The agent watches ManifestWork deletions and removes applied resources
			logger.Info("waiting for Maestro agent to clean up K8s resources", "cluster_id", clusterID)

			// Poll for up to 30 seconds to verify resources are being cleaned up
			maxWait := 30 * time.Second
			pollInterval := 2 * time.Second
			startTime := time.Now()

			for time.Since(startTime) < maxWait {
				// Check if any namespaces still exist
				namespaces, err := h.K8sClient.FindNamespacesByPrefix(ctx, clusterID)
				if err == nil && len(namespaces) == 0 {
					logger.Info("all namespaces cleaned up by Maestro agent", "cluster_id", clusterID)
					break
				}

				// Check if resources are being deleted (pods/deployments going away)
				// This is a good indicator that Maestro agent is working
				allClean := true
				for _, ns := range namespaces {
					// Quick check: if namespace has minimal resources, it's likely clean
					pods, _ := h.K8sClient.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
					deployments, _ := h.K8sClient.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
					jobs, _ := h.K8sClient.BatchV1().Jobs(ns).List(ctx, metav1.ListOptions{})

					if (pods != nil && len(pods.Items) > 0) ||
					   (deployments != nil && len(deployments.Items) > 0) ||
					   (jobs != nil && len(jobs.Items) > 0) {
						allClean = false
						break
					}
				}

				if allClean {
					logger.Info("resources cleaned up by Maestro agent", "cluster_id", clusterID)
					break
				}

				time.Sleep(pollInterval)
			}

			logger.Info("finished waiting for Maestro agent cleanup", "cluster_id", clusterID, "elapsed", time.Since(startTime))
		}
	}

	// Step 2: Find and delete all namespaces associated with this cluster
	// Adapters create namespaces with pattern: {clusterId}-{adapterName}-namespace or just {clusterId}
	logger.Info("finding all namespaces for cluster", "cluster_id", clusterID)
	namespaces, err := h.K8sClient.FindNamespacesByPrefix(ctx, clusterID)
	if err != nil {
		logger.Error("failed to find namespaces for cluster", "cluster_id", clusterID, "error", err)
		cleanupErrors = append(cleanupErrors, fmt.Errorf("failed to find namespaces: %w", err))
	} else {
		logger.Info("found namespaces for cluster", "cluster_id", clusterID, "count", len(namespaces))
		for _, ns := range namespaces {
			logger.Info("deleting namespace", "cluster_id", clusterID, "namespace", ns)
			if err := h.K8sClient.DeleteNamespaceAndWait(ctx, ns); err != nil {
				logger.Error("failed to delete namespace", "cluster_id", clusterID, "namespace", ns, "error", err)
				cleanupErrors = append(cleanupErrors, fmt.Errorf("failed to delete namespace %s: %w", ns, err))
			} else {
				logger.Info("successfully deleted namespace", "cluster_id", clusterID, "namespace", ns)
			}
		}
	}

	// Return accumulated errors if any occurred
	if len(cleanupErrors) > 0 {
		logger.Error("cleanup completed with errors", "cluster_id", clusterID, "error_count", len(cleanupErrors))
		// Combine all errors into a single error message
		var errMsg string
		for i, err := range cleanupErrors {
			if i > 0 {
				errMsg += "; "
			}
			errMsg += err.Error()
		}
		return fmt.Errorf("cleanup errors: %s", errMsg)
	}

	logger.Info("successfully cleaned up cluster resources", "cluster_id", clusterID)
	return nil
}

// GetTestNodePool creates a nodepool on the specified cluster from a payload file
func (h *Helper) GetTestNodePool(ctx context.Context, clusterID, payloadPath string) (*openapi.NodePool, error) {
	return h.Client.CreateNodePoolFromPayload(ctx, clusterID, payloadPath)
}

// CleanupTestNodePool cleans up test nodepool
func (h *Helper) CleanupTestNodePool(ctx context.Context, clusterID, nodepoolID string) error {
	return h.Client.DeleteNodePool(ctx, clusterID, nodepoolID)
}

// GetMaestroClient returns the Maestro client, initializing it lazily on first access
// This avoids the overhead of K8s service discovery for test suites that don't use Maestro
func (h *Helper) GetMaestroClient() *maestro.Client {
	if h.MaestroClient == nil {
		h.MaestroClient = maestro.NewClient("")
	}
	return h.MaestroClient
}
