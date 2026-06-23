package cluster

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

var _ = ginkgo.Describe("[Suite: cluster][perf] Update-to-re-reconciled latency",
	ginkgo.Label(labels.Tier1, labels.Performance),
	func() {
		var h *helper.Helper
		var clusterID string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
			Expect(err).NotTo(HaveOccurred())
			Expect(cluster.Id).NotTo(BeNil(), "cluster ID should be set")
			clusterID = *cluster.Id

			ginkgo.DeferCleanup(func(ctx context.Context) {
				if err := h.CleanupTestCluster(ctx, clusterID); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup cluster %s: %v\n", clusterID, err)
				}
			})

			ginkgo.By("waiting for cluster to reach Reconciled before update")
			Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))
		})

		ginkgo.It("should update a cluster and reach Reconciled within acceptable latency", func(ctx context.Context) {
			ginkgo.By("patching cluster and timing until re-reconciled")
			start := time.Now()

			patchedCluster, err := h.Client.PatchClusterFromPayload(ctx, clusterID, h.TestDataPath("payloads/clusters/cluster-patch.json"))
			Expect(err).NotTo(HaveOccurred())
			expectedGen := patchedCluster.Generation

			Eventually(func(g Gomega) {
				cluster, err := h.Client.GetCluster(ctx, clusterID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(cluster.Generation).To(BeNumerically(">=", expectedGen))
				g.Expect(h.HasResourceCondition(cluster.Status.Conditions,
					client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue)).
					To(BeTrue(), "expected Reconciled=True at generation %d", expectedGen)
			}, h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).Should(Succeed())

			elapsed := time.Since(start)
			ginkgo.GinkgoWriter.Printf("[PERF] Cluster update-to-re-reconciled latency: %v\n", elapsed)
			Expect(elapsed).To(BeNumerically("<", config.ThresholdClusterUpdateReconciled),
				"cluster update-to-re-reconciled exceeded threshold")
		})
	},
)
