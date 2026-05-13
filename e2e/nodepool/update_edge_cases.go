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

var _ = ginkgo.Describe("[Suite: nodepool][update] Labels-Only PATCH",
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

			ginkgo.By("creating nodepool and waiting for Reconciled at generation 1")
			np, err := h.Client.CreateNodePoolFromPayload(ctx, clusterID, h.TestDataPath("payloads/nodepools/nodepool-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create nodepool")
			Expect(np.Id).NotTo(BeNil())
			nodepoolID = *np.Id

			Eventually(h.PollNodePool(ctx, clusterID, nodepoolID), h.Cfg.Timeouts.NodePool.Reconciled, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))
		})

		ginkgo.It("should bump generation and trigger reconciliation from a labels-only PATCH", func(ctx context.Context) {
			ginkgo.By("capturing state before labels-only PATCH")
			npBefore, err := h.Client.GetNodePool(ctx, clusterID, nodepoolID)
			Expect(err).NotTo(HaveOccurred())
			specBefore := npBefore.Spec
			parentBefore, err := h.Client.GetCluster(ctx, clusterID)
			Expect(err).NotTo(HaveOccurred())

			ginkgo.By("sending labels-only PATCH to nodepool (preserving existing labels)")
			newLabels := make(map[string]string)
			if npBefore.Labels != nil {
				for k, v := range *npBefore.Labels {
					newLabels[k] = v
				}
			}
			newLabels["env"] = "staging"
			newLabels["pool-type"] = "gpu"
			patchedNP, err := h.Client.PatchNodePool(ctx, clusterID, nodepoolID, openapi.NodePoolPatchRequest{
				Labels: &newLabels,
			})
			Expect(err).NotTo(HaveOccurred(), "labels-only PATCH should succeed")
			Expect(patchedNP.Generation).To(Equal(int32(2)),
				"generation should increment after labels-only PATCH")

			ginkgo.By("waiting for all nodepool adapters to reconcile at generation 2")
			Eventually(h.PollNodePoolAdapterStatuses(ctx, clusterID, nodepoolID), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
				Should(helper.HaveAllAdaptersAtGeneration(h.Cfg.Adapters.NodePool, int32(2)))

			ginkgo.By("verifying nodepool reaches Reconciled=True and LastKnownReconciled=True")
			Eventually(h.PollNodePool(ctx, clusterID, nodepoolID), h.Cfg.Timeouts.NodePool.Reconciled, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))
			Eventually(h.PollNodePool(ctx, clusterID, nodepoolID), h.Cfg.Timeouts.NodePool.Reconciled, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeLastKnownReconciled, openapi.ResourceConditionStatusTrue))

			finalNP, err := h.Client.GetNodePool(ctx, clusterID, nodepoolID)
			Expect(err).NotTo(HaveOccurred())
			Expect(finalNP.Labels).NotTo(BeNil(), "nodepool should have labels")
			Expect((*finalNP.Labels)["env"]).To(Equal("staging"))
			Expect((*finalNP.Labels)["pool-type"]).To(Equal("gpu"))
			Expect(finalNP.Spec).To(Equal(specBefore),
				"spec should be unchanged after labels-only PATCH")

			ginkgo.By("verifying parent cluster generation is unchanged")
			parentCluster, err := h.Client.GetCluster(ctx, clusterID)
			Expect(err).NotTo(HaveOccurred())
			Expect(parentCluster.Generation).To(Equal(parentBefore.Generation),
				"nodepool labels PATCH should not affect cluster generation")

			hasParentReconciled := h.HasResourceCondition(parentCluster.Status.Conditions, client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue)
			Expect(hasParentReconciled).To(BeTrue(), "parent cluster should remain Reconciled=True")
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
