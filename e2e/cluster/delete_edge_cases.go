package cluster

import (
	"context"
	"errors"
	"net/http"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: cluster][delete] Re-DELETE Idempotency and API Boundary Tests",
	ginkgo.Label(labels.Tier1),
	func() {
		var h *helper.Helper
		var clusterID string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			ginkgo.By("creating cluster and waiting for Reconciled")
			var err error
			clusterID, err = h.GetTestCluster(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create cluster")

			Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))
		})

		ginkgo.It("should handle re-DELETE idempotently without changing deleted_time or generation",
			ginkgo.Label(labels.Disruptive),
			func(ctx context.Context) {
				ginkgo.By("Scaling down sentinel cluster instances to prevent hard-delete between DELETE calls")
				err := h.ScaleDeploymentBySelector(ctx, h.Cfg.Namespace, "app.kubernetes.io/instance in (sentinel-clusters,clusters)", 0)
				Expect(err).NotTo(HaveOccurred(), "failed to scale sentinel to 0")
				ginkgo.DeferCleanup(func(ctx context.Context) {
					ginkgo.By("restoring sentinel-clusters to 1 replica")
					if err := h.ScaleDeploymentBySelector(ctx, h.Cfg.Namespace, "app.kubernetes.io/instance in (sentinel-clusters,clusters)", 1); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: failed to restore sentinel: %v\n", err)
					}
				})

				ginkgo.By("sending first DELETE request")
				firstDelete, err := h.Client.DeleteCluster(ctx, clusterID)
				Expect(err).NotTo(HaveOccurred(), "first DELETE should succeed with 202")
				Expect(firstDelete.DeletedTime).NotTo(BeNil(), "first DELETE should set deleted_time")
				originalDeletedTime := *firstDelete.DeletedTime
				originalGeneration := firstDelete.Generation

				ginkgo.By("sending second DELETE request")
				secondDelete, err := h.Client.DeleteCluster(ctx, clusterID)
				Expect(err).NotTo(HaveOccurred(), "second DELETE should succeed with 202")
				Expect(secondDelete.DeletedTime).NotTo(BeNil(), "second DELETE should still have deleted_time")
				Expect(*secondDelete.DeletedTime).To(Equal(originalDeletedTime), "deleted_time should not change on re-DELETE")
				Expect(secondDelete.Generation).To(Equal(originalGeneration), "generation should not increment on re-DELETE")
			})

		ginkgo.AfterEach(func(ctx context.Context) {
			if h == nil || clusterID == "" {
				return
			}
			ginkgo.By("cleaning up cluster " + clusterID)
			if cluster, err := h.Client.GetCluster(ctx, clusterID); err == nil && cluster.DeletedTime == nil {
				if _, err := h.Client.DeleteCluster(ctx, clusterID); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: API delete failed for cluster %s: %v\n", clusterID, err)
				}
			}
			if err := h.CleanupTestCluster(ctx, clusterID); err != nil {
				ginkgo.GinkgoWriter.Printf("Warning: cleanup failed for cluster %s: %v\n", clusterID, err)
			}
		})
	},
)

var _ = ginkgo.Describe("[Suite: cluster][delete] DELETE During Update Reconciliation",
	ginkgo.Label(labels.Tier1),
	func() {
		var h *helper.Helper
		var clusterID string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			ginkgo.By("creating cluster and waiting for Reconciled at generation 1")
			cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create cluster")
			Expect(cluster.Id).NotTo(BeNil())
			clusterID = *cluster.Id

			Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))
		})

		ginkgo.It("should complete deletion when DELETE is sent during update reconciliation", func(ctx context.Context) {
			ginkgo.By("sending PATCH to trigger generation 2 (do NOT wait for reconciliation)")
			patchedCluster, err := h.Client.PatchCluster(ctx, clusterID, openapi.ClusterPatchRequest{
				Spec: &openapi.ClusterSpec{"trigger-update": "true"},
			})
			Expect(err).NotTo(HaveOccurred(), "PATCH should succeed")
			Expect(patchedCluster.Generation).To(Equal(int32(2)))

			ginkgo.By("immediately sending DELETE before update reconciliation completes")
			deletedCluster, err := h.Client.DeleteCluster(ctx, clusterID)
			Expect(err).NotTo(HaveOccurred(), "DELETE should succeed with 202")
			Expect(deletedCluster.DeletedTime).NotTo(BeNil())
			Expect(deletedCluster.Generation).To(Equal(int32(3)),
				"generation should be 3: create(1) + PATCH(2) + DELETE(3)")

			ginkgo.By("verifying cluster is hard-deleted")
			Eventually(h.PollClusterHTTPStatus(ctx, clusterID), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
				Should(Equal(http.StatusNotFound))

			ginkgo.By("verifying downstream K8s namespace is cleaned up")
			Eventually(h.PollNamespacesByPrefix(ctx, clusterID), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
				Should(BeEmpty())
		})

		ginkgo.AfterEach(func(ctx context.Context) {
			if h == nil || clusterID == "" {
				return
			}
			ginkgo.By("cleaning up cluster " + clusterID)
			if cluster, err := h.Client.GetCluster(ctx, clusterID); err == nil && cluster.DeletedTime == nil {
				if _, err := h.Client.DeleteCluster(ctx, clusterID); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: API delete failed for cluster %s: %v\n", clusterID, err)
				}
			}
			if err := h.CleanupTestCluster(ctx, clusterID); err != nil {
				ginkgo.GinkgoWriter.Printf("Warning: cleanup failed for cluster %s: %v\n", clusterID, err)
			}
		})
	},
)

var _ = ginkgo.Describe("[Suite: cluster][delete] Recreate Cluster After Hard-Delete",
	ginkgo.Label(labels.Tier1),
	func() {
		var h *helper.Helper
		var firstClusterID string
		var secondClusterID string
		var originalCluster *openapi.Cluster

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			ginkgo.By("creating first cluster and waiting for Reconciled")
			cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create cluster")
			Expect(cluster.Id).NotTo(BeNil())
			firstClusterID = *cluster.Id
			originalCluster = cluster

			Eventually(h.PollCluster(ctx, firstClusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))
		})

		ginkgo.It("should create a new cluster with the same name after the original is hard-deleted", func(ctx context.Context) {
			ginkgo.By("deleting the first cluster and waiting for hard-delete")
			_, err := h.Client.DeleteCluster(ctx, firstClusterID)
			Expect(err).NotTo(HaveOccurred())

			Eventually(h.PollClusterHTTPStatus(ctx, firstClusterID), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
				Should(Equal(http.StatusNotFound))

			ginkgo.By("waiting for namespace cleanup from first cluster")
			Eventually(h.PollNamespacesByPrefix(ctx, firstClusterID), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
				Should(BeEmpty())

			ginkgo.By("creating a new cluster with the same name")
			newCluster, err := h.Client.CreateCluster(ctx, openapi.ClusterCreateRequest{
				Kind:   "Cluster",
				Name:   originalCluster.Name,
				Labels: originalCluster.Labels,
				Spec:   originalCluster.Spec,
			})
			Expect(err).NotTo(HaveOccurred(), "creating cluster with reused name should succeed")
			Expect(newCluster.Id).NotTo(BeNil())
			secondClusterID = *newCluster.Id

			Expect(secondClusterID).NotTo(Equal(firstClusterID),
				"new cluster should have a different ID than the deleted one")
			Expect(newCluster.Generation).To(Equal(int32(1)),
				"new cluster should start at generation 1")

			ginkgo.By("waiting for the new cluster to reach Reconciled")
			Eventually(h.PollCluster(ctx, secondClusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))

			ginkgo.By("verifying the old cluster is still gone")
			_, err = h.Client.GetCluster(ctx, firstClusterID)
			var httpErr *client.HTTPError
			Expect(errors.As(err, &httpErr)).To(BeTrue())
			Expect(httpErr.StatusCode).To(Equal(http.StatusNotFound),
				"old cluster should remain 404 after recreate")
		})

		ginkgo.AfterEach(func(ctx context.Context) {
			if h == nil {
				return
			}
			for _, id := range []string{firstClusterID, secondClusterID} {
				if id == "" {
					continue
				}
				ginkgo.By("cleaning up cluster " + id)
				if cluster, err := h.Client.GetCluster(ctx, id); err == nil && cluster.DeletedTime == nil {
					if _, err := h.Client.DeleteCluster(ctx, id); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: API delete failed for cluster %s: %v\n", id, err)
					}
				}
				if err := h.CleanupTestCluster(ctx, id); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: cleanup failed for cluster %s: %v\n", id, err)
				}
			}
		})
	},
)
