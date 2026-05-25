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

var _ = ginkgo.Describe("[Suite: nodepool][update] NodePool Update Lifecycle",
	ginkgo.Label(labels.Tier0),
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

			ginkgo.DeferCleanup(func(ctx context.Context) {
				if err := h.CleanupTestCluster(ctx, clusterID); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup cluster %s: %v\n", clusterID, err)
				}
			})

			Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))

			ginkgo.By("creating nodepool and waiting for Reconciled at generation 1")
			np, err := h.Client.CreateNodePoolFromPayload(ctx, clusterID, h.TestDataPath("payloads/nodepools/nodepool-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create nodepool")
			Expect(np.Id).NotTo(BeNil(), "nodepool ID should be generated")
			nodepoolID = *np.Id

			Eventually(h.PollNodePool(ctx, clusterID, nodepoolID), h.Cfg.Timeouts.NodePool.Reconciled, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))
		})

		ginkgo.It("should update nodepool via PATCH, trigger reconciliation, and reach Reconciled at new generation", func(ctx context.Context) {
			ginkgo.By("capturing state before PATCH")
			npBefore, err := h.Client.GetNodePool(ctx, clusterID, nodepoolID)
			Expect(err).NotTo(HaveOccurred())
			Expect(npBefore.Generation).To(Equal(int32(1)), "nodepool should be at generation 1 before update")
			parentBefore, err := h.Client.GetCluster(ctx, clusterID)
			Expect(err).NotTo(HaveOccurred())

			ginkgo.By("sending PATCH to update nodepool spec")
			patchedNP, err := h.Client.PatchNodePoolFromPayload(ctx, clusterID, nodepoolID, h.TestDataPath("payloads/nodepools/nodepool-patch.json"))
			Expect(err).NotTo(HaveOccurred(), "PATCH request should succeed")
			expectedGen := npBefore.Generation + 1
			Expect(patchedNP.Generation).To(Equal(expectedGen), "generation should increment after PATCH")
			Expect(patchedNP.Spec).To(HaveKeyWithValue("replicas", BeNumerically("==", 3)),
				"PATCH response should reflect updated replicas field")

			ginkgo.By("waiting for all nodepool adapters to reconcile at new generation")
			Eventually(h.PollNodePoolAdapterStatuses(ctx, clusterID, nodepoolID), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
				Should(helper.HaveAllAdaptersAtGeneration(h.Cfg.Adapters.NodePool, expectedGen))

			ginkgo.By("verifying nodepool reaches Reconciled=True at new generation")
			Eventually(func(g Gomega) {
				finalNP, err := h.Client.GetNodePool(ctx, clusterID, nodepoolID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(finalNP.Generation).To(Equal(expectedGen), "final nodepool generation should match expected")
				g.Expect(finalNP.Status).NotTo(BeNil(), "nodepool status should be present")

				found := false
				for _, cond := range finalNP.Status.Conditions {
					if cond.Type == client.ConditionTypeReconciled && cond.Status == openapi.ResourceConditionStatusTrue {
						found = true
						g.Expect(cond.ObservedGeneration).To(Equal(expectedGen), "Reconciled condition observed_generation should match expected")
					}
				}
				g.Expect(found).To(BeTrue(), "nodepool should have Reconciled=True")
			}, h.Cfg.Timeouts.NodePool.Reconciled, h.Cfg.Polling.Interval).Should(Succeed())

			ginkgo.By("verifying parent cluster generation is unchanged")
			parentCluster, err := h.Client.GetCluster(ctx, clusterID)
			Expect(err).NotTo(HaveOccurred())
			Expect(parentCluster.Generation).To(Equal(parentBefore.Generation), "nodepool update should not affect cluster generation")

			Expect(parentCluster).To(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue),
				"parent cluster should remain Reconciled=True")
		})

	},
)
