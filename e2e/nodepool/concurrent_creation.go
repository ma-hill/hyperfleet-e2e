package nodepool

import (
	"context"
	"fmt"
	"sync"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

const concurrentNodePoolCount = 3

var _ = ginkgo.Describe("[Suite: nodepool][concurrent] Multiple nodepools can coexist under same cluster without conflicts",
	ginkgo.Label(labels.Tier1),
	func() {
		var h *helper.Helper
		var clusterID string
		var nodepoolIDs []string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()
			nodepoolIDs = nil

			// Get or create a cluster for nodepool tests
			var err error
			clusterID, err = h.GetTestCluster(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to get test cluster")
			ginkgo.GinkgoWriter.Printf("Using cluster ID: %s\n", clusterID)

		})

		ginkgo.It("should create multiple nodepools under the same cluster and all reach Reconciled state with isolated resources",
			func(ctx context.Context) {
				ginkgo.By(fmt.Sprintf("Submit %d nodepool creation requests simultaneously", concurrentNodePoolCount))

				type nodepoolResult struct {
					id   string
					name string
					err  error
				}

				results := make([]nodepoolResult, concurrentNodePoolCount)
				var wg sync.WaitGroup
				wg.Add(concurrentNodePoolCount)

				for i := 0; i < concurrentNodePoolCount; i++ {
					go func(idx int) {
						defer wg.Done()
						defer ginkgo.GinkgoRecover()

						nodepool, err := h.Client.CreateNodePoolFromPayload(ctx, clusterID, h.TestDataPath("payloads/nodepools/nodepool-request.json"))
						if err != nil {
							results[idx] = nodepoolResult{err: fmt.Errorf("failed to create nodepool %d: %w", idx, err)}
							return
						}
						if nodepool.Id == nil {
							results[idx] = nodepoolResult{err: fmt.Errorf("nodepool %d has nil ID", idx)}
							return
						}
						results[idx] = nodepoolResult{
							id:   *nodepool.Id,
							name: nodepool.Name,
						}
					}(i)
				}
				wg.Wait()

				// Verify all creations succeeded and collect IDs
				for i, r := range results {
					Expect(r.err).NotTo(HaveOccurred(), "nodepool creation %d failed", i)
					Expect(r.id).NotTo(BeEmpty(), "nodepool %d should have a non-empty ID", i)
					nodepoolIDs = append(nodepoolIDs, r.id)
					ginkgo.GinkgoWriter.Printf("Created nodepool %d: ID=%s, Name=%s\n", i, r.id, r.name)
				}

				// Verify all nodepool IDs are unique
				idSet := make(map[string]bool, len(nodepoolIDs))
				for _, id := range nodepoolIDs {
					Expect(idSet[id]).To(BeFalse(), "duplicate nodepool ID detected: %s", id)
					idSet[id] = true
				}

				ginkgo.By("Verify all nodepools appear in the list API")
				nodepoolList, err := h.Client.ListNodePools(ctx, clusterID)
				Expect(err).NotTo(HaveOccurred(), "failed to list nodepools")

				listedIDs := make(map[string]bool)
				for _, np := range nodepoolList.Items {
					if np.Id != nil {
						listedIDs[*np.Id] = true
					}
				}
				for i, npID := range nodepoolIDs {
					Expect(listedIDs[npID]).To(BeTrue(),
						"nodepool %d (%s) should appear in the list API", i, npID)
				}
				ginkgo.GinkgoWriter.Printf("All %d nodepools found in list API\n", concurrentNodePoolCount)

				ginkgo.By("Wait for all nodepools to reach Reconciled=True and Available=True")
				for i, npID := range nodepoolIDs {
					ginkgo.GinkgoWriter.Printf("Waiting for nodepool %d (%s) to become Reconciled...\n", i, npID)
					Eventually(h.PollNodePool(ctx, clusterID, npID), h.Cfg.Timeouts.NodePool.Reconciled, h.Cfg.Polling.Interval).
						Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))

					np, err := h.Client.GetNodePool(ctx, clusterID, npID)
					Expect(err).NotTo(HaveOccurred(), "failed to get nodepool %d (%s)", i, npID)

					hasAvailable := h.HasResourceCondition(np.Status.Conditions,
						client.ConditionTypeLastKnownReconciled, openapi.ResourceConditionStatusTrue)
					Expect(hasAvailable).To(BeTrue(),
						"nodepool %d (%s) should have LastKnownReconciled=True", i, npID)

					ginkgo.GinkgoWriter.Printf("Nodepool %d (%s) reached Reconciled=True, Available=True\n", i, npID)
				}

				ginkgo.By("Verify Kubernetes resources are isolated per nodepool")
				for i, npID := range nodepoolIDs {
					expectedLabels := map[string]string{
						"hyperfleet.io/cluster-id":  clusterID,
						"hyperfleet.io/nodepool-id": npID,
					}
					Eventually(func() error {
						return h.VerifyConfigMap(ctx, clusterID, expectedLabels, nil)
					}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed(),
						"nodepool %d (%s) should have its own configmap resource", i, npID)
					ginkgo.GinkgoWriter.Printf("Nodepool %d (%s) has isolated K8s resources\n", i, npID)
				}

				ginkgo.By("Verify all adapter statuses are complete for each nodepool")
				for i, npID := range nodepoolIDs {
					Eventually(func(g Gomega) {
						statuses, err := h.Client.GetNodePoolStatuses(ctx, clusterID, npID)
						g.Expect(err).NotTo(HaveOccurred(), "failed to get nodepool statuses for nodepool %d (%s)", i, npID)
						g.Expect(statuses.Items).NotTo(BeEmpty(), "nodepool %d (%s) should have adapter statuses", i, npID)

						adapterMap := make(map[string]openapi.AdapterStatus)
						for _, adapter := range statuses.Items {
							adapterMap[adapter.Adapter] = adapter
						}

						for _, requiredAdapter := range h.Cfg.Adapters.NodePool {
							adapter, exists := adapterMap[requiredAdapter]
							g.Expect(exists).To(BeTrue(),
								"nodepool %d (%s): required adapter %s should be present", i, npID, requiredAdapter)

							g.Expect(h.HasAdapterCondition(adapter.Conditions,
								client.ConditionTypeApplied, openapi.AdapterConditionStatusTrue)).To(BeTrue(),
								"nodepool %d (%s): adapter %s should have Applied=True", i, npID, requiredAdapter)

							g.Expect(h.HasAdapterCondition(adapter.Conditions,
								client.ConditionTypeAvailable, openapi.AdapterConditionStatusTrue)).To(BeTrue(),
								"nodepool %d (%s): adapter %s should have Available=True", i, npID, requiredAdapter)

							g.Expect(h.HasAdapterCondition(adapter.Conditions,
								client.ConditionTypeHealth, openapi.AdapterConditionStatusTrue)).To(BeTrue(),
								"nodepool %d (%s): adapter %s should have Health=True", i, npID, requiredAdapter)
						}
					}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

					ginkgo.GinkgoWriter.Printf("Nodepool %d (%s) has all adapter statuses complete\n", i, npID)
				}

				ginkgo.GinkgoWriter.Printf("Successfully validated %d nodepools coexisting under cluster %s with resource isolation\n",
					concurrentNodePoolCount, clusterID)
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
