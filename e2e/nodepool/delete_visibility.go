package nodepool

import (
	"context"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: nodepool][delete] Soft-Deleted Nodepool Visibility",
	ginkgo.Label(labels.Tier1, labels.Disruptive),
	ginkgo.Serial,
	func() {
		var h *helper.Helper
		var clusterID string
		var activeNPID string
		var deletedNPID string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			ginkgo.By("creating cluster and waiting for Reconciled")
			var err error
			clusterID, err = h.GetTestCluster(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create cluster")

			Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))

			ginkgo.By("creating two nodepools and waiting for Reconciled")
			np1, err := h.Client.CreateNodePoolFromPayload(ctx, clusterID, h.TestDataPath("payloads/nodepools/nodepool-request.json"))
			Expect(err).NotTo(HaveOccurred())
			Expect(np1.Id).NotTo(BeNil())
			activeNPID = *np1.Id

			np2, err := h.Client.CreateNodePoolFromPayload(ctx, clusterID, h.TestDataPath("payloads/nodepools/nodepool-request.json"))
			Expect(err).NotTo(HaveOccurred())
			Expect(np2.Id).NotTo(BeNil())
			deletedNPID = *np2.Id

			Eventually(h.PollNodePool(ctx, clusterID, activeNPID), h.Cfg.Timeouts.NodePool.Reconciled, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))

			Eventually(h.PollNodePool(ctx, clusterID, deletedNPID), h.Cfg.Timeouts.NodePool.Reconciled, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))
		})

		ginkgo.It("should remain visible via GET and LIST before hard-delete", func(ctx context.Context) {
			ginkgo.By("pausing sentinel-nodepools to freeze reconciliation before soft-delete")
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

			ginkgo.By("soft-deleting one nodepool")
			deletedNP, err := h.Client.DeleteNodePool(ctx, clusterID, deletedNPID)
			Expect(err).NotTo(HaveOccurred())
			Expect(deletedNP.DeletedTime).NotTo(BeNil())

			ginkgo.By("verifying GET returns the soft-deleted nodepool with deleted_time")
			Eventually(func(g Gomega) {
				np, err := h.Client.GetNodePool(ctx, clusterID, deletedNPID)
				g.Expect(err).NotTo(HaveOccurred(), "GET should return 200, not 404")
				g.Expect(np.DeletedTime).NotTo(BeNil(), "nodepool should have deleted_time set")
			}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

			ginkgo.By("verifying LIST includes both active and soft-deleted nodepools")
			Eventually(func(g Gomega) {
				npList, err := h.Client.ListNodePools(ctx, clusterID)
				g.Expect(err).NotTo(HaveOccurred())

				var foundActive, foundDeleted bool
				for _, np := range npList.Items {
					if np.Id == nil {
						continue
					}
					if *np.Id == activeNPID {
						g.Expect(np.DeletedTime).To(BeNil(), "active nodepool should not have deleted_time")
						foundActive = true
					}
					if *np.Id == deletedNPID {
						g.Expect(np.DeletedTime).NotTo(BeNil(), "deleted nodepool should have deleted_time")
						foundDeleted = true
					}
				}
				g.Expect(foundActive).To(BeTrue(), "active nodepool should appear in LIST")
				g.Expect(foundDeleted).To(BeTrue(), "soft-deleted nodepool should appear in LIST")
			}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

			ginkgo.By("verifying active nodepool is unaffected")
			activeNP, err := h.Client.GetNodePool(ctx, clusterID, activeNPID)
			Expect(err).NotTo(HaveOccurred())
			Expect(activeNP.DeletedTime).To(BeNil())

			hasReconciled := h.HasResourceCondition(activeNP.Status.Conditions, client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue)
			Expect(hasReconciled).To(BeTrue(), "active nodepool should remain Reconciled=True")
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
