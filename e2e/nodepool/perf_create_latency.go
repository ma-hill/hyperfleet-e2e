package nodepool

import (
	"context"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: nodepool][perf] Create-to-reconciled latency",
	ginkgo.Label(labels.Tier1, labels.Performance),
	func() {
		var h *helper.Helper
		var clusterID string

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
		})

		ginkgo.It("should create a nodepool and reach Reconciled within acceptable latency", func(ctx context.Context) {
			ginkgo.By("creating a nodepool and timing until Reconciled")
			start := time.Now()

			nodepool, err := h.Client.CreateNodePoolFromPayload(ctx, clusterID, h.TestDataPath("payloads/nodepools/nodepool-request.json"))
			Expect(err).NotTo(HaveOccurred())
			Expect(nodepool.Id).NotTo(BeNil(), "nodepool ID should be set")
			nodepoolID := *nodepool.Id

			ginkgo.DeferCleanup(func(ctx context.Context) {
				if err := h.CleanupTestNodePool(ctx, clusterID, nodepoolID); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup nodepool %s: %v\n", nodepoolID, err)
				}
			})

			Eventually(h.PollNodePool(ctx, clusterID, nodepoolID), h.Cfg.Timeouts.NodePool.Reconciled, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))

			elapsed := time.Since(start)
			ginkgo.GinkgoWriter.Printf("[PERF] NodePool create-to-reconciled latency: %v\n", elapsed)
			Expect(elapsed).To(BeNumerically("<", config.ThresholdNodePoolCreateReconciled),
				"nodepool create-to-reconciled exceeded threshold")
		})
	},
)
