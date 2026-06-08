package nodepool

import (
	"context"
	"net/http"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: nodepool][baseline] NodePool Full Lifecycle Smoke",
	ginkgo.Label(labels.Tier0),
	func() {
		ginkgo.It("should complete create → Reconciled → update → re-Reconciled → delete → hard-delete in a single pass",
			func(ctx context.Context) {
				h := helper.New()

				ginkgo.By("creating a parent cluster and waiting for Reconciled")
				clusterID, err := h.GetTestCluster(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
				Expect(err).NotTo(HaveOccurred(), "failed to create cluster")

				ginkgo.DeferCleanup(func(ctx context.Context) {
					if err := h.CleanupTestCluster(ctx, clusterID); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: cleanup failed for cluster %s: %v\n", clusterID, err)
					}
				})

				Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
					Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))

				ginkgo.By("creating a nodepool")
				np, err := h.Client.CreateNodePoolFromPayload(ctx, clusterID, h.TestDataPath("payloads/nodepools/nodepool-request.json"))
				Expect(err).NotTo(HaveOccurred(), "create should succeed")
				Expect(np).NotTo(BeNil(), "create should return nodepool object")
				Expect(np.Id).NotTo(BeNil(), "nodepool ID should be assigned")
				nodepoolID := *np.Id

				ginkgo.By("waiting for Reconciled=True at generation 1")
				Eventually(h.PollNodePool(ctx, clusterID, nodepoolID), h.Cfg.Timeouts.NodePool.Reconciled, h.Cfg.Polling.Interval).
					Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))

				ginkgo.By("PATCHing nodepool spec to trigger generation bump")
				patched, err := h.Client.PatchNodePool(ctx, clusterID, nodepoolID, openapi.NodePoolPatchRequest{
					Spec: &openapi.NodePoolSpec{"lifecycle-test": "updated"},
				})
				Expect(err).NotTo(HaveOccurred(), "PATCH should succeed")
				Expect(patched).NotTo(BeNil(), "PATCH should return nodepool object")
				Expect(patched.Generation).To(Equal(int32(2)), "generation should increment after PATCH")

				ginkgo.By("waiting for Reconciled=True at generation 2")
				Eventually(h.PollNodePoolAdapterStatuses(ctx, clusterID, nodepoolID),
					h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
					Should(helper.HaveAllAdaptersAtGeneration(h.Cfg.Adapters.NodePool, int32(2)))

				Eventually(h.PollNodePool(ctx, clusterID, nodepoolID), h.Cfg.Timeouts.NodePool.Reconciled, h.Cfg.Polling.Interval).
					Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))

				ginkgo.By("soft-deleting the nodepool")
				deletedNP, err := h.Client.DeleteNodePool(ctx, clusterID, nodepoolID)
				Expect(err).NotTo(HaveOccurred(), "DELETE should return 202")
				Expect(deletedNP).NotTo(BeNil(), "DELETE should return nodepool object")
				Expect(deletedNP.DeletedTime).NotTo(BeNil())

				ginkgo.By("waiting for hard-delete (404)")
				Eventually(h.PollNodePoolHTTPStatus(ctx, clusterID, nodepoolID),
					h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
					Should(Equal(http.StatusNotFound))

				ginkgo.By("verifying parent cluster is unaffected")
				parentCluster, err := h.Client.GetCluster(ctx, clusterID)
				Expect(err).NotTo(HaveOccurred(), "parent cluster should still exist")
				Expect(parentCluster).NotTo(BeNil(), "GET should return parent cluster object")
				Expect(parentCluster.DeletedTime).To(BeNil(), "parent cluster should not be deleted")
				Expect(parentCluster).To(helper.HaveResourceCondition(
					client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue),
					"parent cluster should remain Reconciled=True")
			})
	},
)
