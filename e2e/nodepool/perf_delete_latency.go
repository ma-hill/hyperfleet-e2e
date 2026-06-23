package nodepool

import (
	"context"
	"net/http"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: nodepool][perf] Delete-to-hard-delete latency",
	ginkgo.Label(labels.Tier1, labels.Performance),
	func() {
		var h *helper.Helper
		var clusterID string
		var nodepoolID string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			var err error
			clusterID, err = h.GetTestCluster(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
			Expect(err).NotTo(HaveOccurred())

			ginkgo.DeferCleanup(func(ctx context.Context) {
				if err := h.CleanupTestCluster(ctx, clusterID); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup cluster %s: %v\n", clusterID, err)
				}
			})

			nodepool, err := h.Client.CreateNodePoolFromPayload(ctx, clusterID, h.TestDataPath("payloads/nodepools/nodepool-request.json"))
			Expect(err).NotTo(HaveOccurred())
			Expect(nodepool.Id).NotTo(BeNil(), "nodepool ID should be set")
			nodepoolID = *nodepool.Id

			ginkgo.DeferCleanup(func(ctx context.Context) {
				if err := h.CleanupTestNodePool(ctx, clusterID, nodepoolID); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup nodepool %s: %v\n", nodepoolID, err)
				}
			})

			ginkgo.By("waiting for nodepool to reach Reconciled before delete")
			Eventually(h.PollNodePool(ctx, clusterID, nodepoolID), h.Cfg.Timeouts.NodePool.Reconciled, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))
		})

		ginkgo.It("should delete a nodepool and reach hard-delete within acceptable latency", func(ctx context.Context) {
			ginkgo.By("deleting nodepool and timing until hard-delete (404)")
			start := time.Now()

			_, err := h.Client.DeleteNodePool(ctx, clusterID, nodepoolID)
			Expect(err).NotTo(HaveOccurred())

			Eventually(h.PollNodePoolHTTPStatus(ctx, clusterID, nodepoolID), h.Cfg.Timeouts.NodePool.Deleted, h.Cfg.Polling.Interval).
				Should(Equal(http.StatusNotFound))

			elapsed := time.Since(start)
			ginkgo.GinkgoWriter.Printf("[PERF] NodePool delete-to-hard-delete latency: %v\n", elapsed)
			Expect(elapsed).To(BeNumerically("<", config.ThresholdNodePoolDeleted),
				"nodepool delete-to-hard-delete exceeded threshold")
		})
	},
)
