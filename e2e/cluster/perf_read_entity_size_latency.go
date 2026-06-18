package cluster

import (
	"context"
	"slices"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: cluster][perf] API read latency by entity size",
	ginkgo.Label(labels.Tier1, labels.Performance),
	func() {
		var h *helper.Helper

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()
		})

		sizes := []struct {
			name    string
			payload string
		}{
			{"small", "payloads/clusters/cluster-request-small.json"},
			{"medium", "payloads/clusters/cluster-request.json"},
			{"large", "payloads/clusters/cluster-request-large.json"},
		}

		for _, size := range sizes {
			ginkgo.It("should read a "+size.name+" cluster within acceptable latency", func(ctx context.Context) {
				ginkgo.By("creating a " + size.name + " cluster")
				cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath(size.payload))
				Expect(err).NotTo(HaveOccurred())
				Expect(cluster.Id).NotTo(BeNil(), "cluster ID should be set")
				clusterID := *cluster.Id

				ginkgo.DeferCleanup(func(ctx context.Context) {
					if err := h.CleanupTestCluster(ctx, clusterID); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup cluster %s: %v\n", clusterID, err)
					}
				})

				ginkgo.By("waiting for cluster to reach Reconciled before read")
				Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
					Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))

				ginkgo.By("warming up with untimed read")
				_, err = h.Client.GetCluster(ctx, clusterID)
				Expect(err).NotTo(HaveOccurred())

				ginkgo.By("measuring GET /clusters/{id} response time for " + size.name + " entity")
				const samples = 5
				durations := make([]time.Duration, samples)
				for i := range samples {
					start := time.Now()
					_, err = h.Client.GetCluster(ctx, clusterID)
					Expect(err).NotTo(HaveOccurred())
					durations[i] = time.Since(start)
				}
				slices.Sort(durations)
				median := durations[samples/2]
				ginkgo.GinkgoWriter.Printf("[PERF] GET /clusters/%s (%s entity) latency: %v (median of %d samples)\n", clusterID, size.name, median, samples)
			})
		}
	},
)
