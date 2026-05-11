package nodepool

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

var _ = ginkgo.Describe("[Suite: nodepool][delete] Sibling Nodepool Isolation During Deletion",
	ginkgo.Label(labels.Tier1),
	func() {
		var h *helper.Helper
		var clusterID string
		var nodepoolID1 string
		var nodepoolID2 string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			ginkgo.By("creating cluster and waiting for Reconciled")
			var err error
			clusterID, err = h.GetTestCluster(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create cluster")

			Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))

			ginkgo.By("creating two nodepools")
			np1, err := h.Client.CreateNodePoolFromPayload(ctx, clusterID, h.TestDataPath("payloads/nodepools/nodepool-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create first nodepool")
			Expect(np1.Id).NotTo(BeNil())
			nodepoolID1 = *np1.Id

			np2, err := h.Client.CreateNodePoolFromPayload(ctx, clusterID, h.TestDataPath("payloads/nodepools/nodepool-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create second nodepool")
			Expect(np2.Id).NotTo(BeNil())
			nodepoolID2 = *np2.Id

			ginkgo.By("waiting for both nodepools to reach Reconciled")
			Eventually(h.PollNodePool(ctx, clusterID, nodepoolID1), h.Cfg.Timeouts.NodePool.Reconciled, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))

			Eventually(h.PollNodePool(ctx, clusterID, nodepoolID2), h.Cfg.Timeouts.NodePool.Reconciled, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))
		})

		ginkgo.It("should not affect sibling nodepool when one is deleted", func(ctx context.Context) {
			ginkgo.By("deleting the first nodepool")
			deletedNP, err := h.Client.DeleteNodePool(ctx, clusterID, nodepoolID1)
			Expect(err).NotTo(HaveOccurred(), "DELETE should succeed with 202")
			Expect(deletedNP.DeletedTime).NotTo(BeNil())

			ginkgo.By("waiting for the deleted nodepool to be hard-deleted")
			Eventually(h.PollNodePoolHTTPStatus(ctx, clusterID, nodepoolID1), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
				Should(Equal(http.StatusNotFound))

			ginkgo.By("verifying sibling nodepool is unaffected")
			siblingNP, err := h.Client.GetNodePool(ctx, clusterID, nodepoolID2)
			Expect(err).NotTo(HaveOccurred(), "sibling nodepool should still be accessible")
			Expect(siblingNP.DeletedTime).To(BeNil(), "sibling nodepool should not have deleted_time")

			hasReconciled := h.HasResourceCondition(siblingNP.Status.Conditions, client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue)
			Expect(hasReconciled).To(BeTrue(), "sibling nodepool should remain Reconciled=True")

			ginkgo.By("verifying sibling nodepool adapter statuses are intact")
			Eventually(h.PollNodePoolAdapterStatuses(ctx, clusterID, nodepoolID2), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
				Should(helper.HaveAllAdaptersWithCondition(
					h.Cfg.Adapters.NodePool, client.ConditionTypeApplied, openapi.AdapterConditionStatusTrue))

			ginkgo.By("verifying parent cluster is unaffected")
			parentCluster, err := h.Client.GetCluster(ctx, clusterID)
			Expect(err).NotTo(HaveOccurred(), "parent cluster should still exist")
			Expect(parentCluster.DeletedTime).To(BeNil(), "parent cluster should not have deleted_time")

			hasParentReconciled := h.HasResourceCondition(parentCluster.Status.Conditions, client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue)
			Expect(hasParentReconciled).To(BeTrue(), "parent cluster should remain Reconciled=True")
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

var _ = ginkgo.Describe("[Suite: nodepool][delete] Re-DELETE Idempotency",
	ginkgo.Label(labels.Tier1),
	func() {
		var h *helper.Helper
		var clusterID string
		var nodepoolID string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			ginkgo.By("creating cluster and waiting for Reconciled")
			var err error
			clusterID, err = h.GetTestCluster(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create cluster")

			Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))

			ginkgo.By("creating nodepool and waiting for Reconciled")
			np, err := h.Client.CreateNodePoolFromPayload(ctx, clusterID, h.TestDataPath("payloads/nodepools/nodepool-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create nodepool")
			Expect(np.Id).NotTo(BeNil())
			nodepoolID = *np.Id

			Eventually(h.PollNodePool(ctx, clusterID, nodepoolID), h.Cfg.Timeouts.NodePool.Reconciled, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))
		})

		ginkgo.It("should handle re-DELETE on nodepool idempotently without changing deleted_time or generation",
			ginkgo.Label(labels.Disruptive),
			func(ctx context.Context) {
				ginkgo.By("pausing sentinel to prevent hard-delete between DELETE calls")
				sentinelDeployment, err := h.GetDeploymentName(ctx, h.Cfg.Namespace, helper.SentinelNodePoolsRelease)
				Expect(err).NotTo(HaveOccurred(), "failed to find sentinel-nodepools deployment")
				err = h.ScaleDeployment(ctx, h.Cfg.Namespace, sentinelDeployment, 0)
				Expect(err).NotTo(HaveOccurred(), "failed to scale sentinel-nodepools to 0")
				ginkgo.DeferCleanup(func(ctx context.Context) {
					ginkgo.By("restoring sentinel-nodepools to 1 replica")
					if err := h.ScaleDeployment(ctx, h.Cfg.Namespace, sentinelDeployment, 1); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: failed to restore sentinel-nodepools: %v\n", err)
					}
				})

				ginkgo.By("sending first DELETE request")
				firstDelete, err := h.Client.DeleteNodePool(ctx, clusterID, nodepoolID)
				Expect(err).NotTo(HaveOccurred(), "first DELETE should succeed with 202")
				Expect(firstDelete.DeletedTime).NotTo(BeNil())
				originalDeletedTime := *firstDelete.DeletedTime
				originalGeneration := firstDelete.Generation

				ginkgo.By("sending second DELETE request")
				secondDelete, err := h.Client.DeleteNodePool(ctx, clusterID, nodepoolID)
				Expect(err).NotTo(HaveOccurred(), "second DELETE should succeed with 202")
				Expect(secondDelete.DeletedTime).NotTo(BeNil())
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

var _ = ginkgo.Describe("[Suite: nodepool][delete] DELETE Non-Existent Nodepool",
	ginkgo.Label(labels.Tier1, labels.Negative),
	func() {
		var h *helper.Helper
		var clusterID string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			ginkgo.By("creating cluster for valid cluster_id path parameter")
			var err error
			clusterID, err = h.GetTestCluster(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create cluster")
		})

		ginkgo.It("should return 404 when deleting a non-existent nodepool", func(ctx context.Context) {
			ginkgo.By("sending DELETE for a non-existent nodepool ID")
			_, err := h.Client.DeleteNodePool(ctx, clusterID, "non-existent-nodepool-id-12345")
			var httpErr *client.HTTPError
			Expect(errors.As(err, &httpErr)).To(BeTrue(), "error should be HTTPError")
			Expect(httpErr.StatusCode).To(Equal(http.StatusNotFound),
				"DELETE on non-existent nodepool should return 404")
		})

		ginkgo.AfterEach(func(ctx context.Context) {
			if h == nil || clusterID == "" {
				return
			}
			ginkgo.By("cleaning up cluster " + clusterID)
			if err := h.CleanupTestCluster(ctx, clusterID); err != nil {
				ginkgo.GinkgoWriter.Printf("Warning: cleanup failed for cluster %s: %v\n", clusterID, err)
			}
		})
	},
)
