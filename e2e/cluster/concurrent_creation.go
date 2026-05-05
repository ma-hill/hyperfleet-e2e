package cluster

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

const concurrentClusterCount = 5

var _ = ginkgo.Describe("[Suite: cluster][concurrent] System can process concurrent cluster creations without resource conflicts",
	ginkgo.Label(labels.Tier1),
	func() {
		var h *helper.Helper
		var clusterIDs []string

		ginkgo.BeforeEach(func() {
			h = helper.New()
			clusterIDs = nil
		})

		ginkgo.It("should create multiple clusters concurrently and all reach Reconciled state with isolated resources",
			func(ctx context.Context) {
				ginkgo.By(fmt.Sprintf("Submit %d cluster creation requests simultaneously", concurrentClusterCount))

				type clusterResult struct {
					id   string
					name string
					err  error
				}

				results := make([]clusterResult, concurrentClusterCount)
				var wg sync.WaitGroup
				wg.Add(concurrentClusterCount)

				for i := 0; i < concurrentClusterCount; i++ {
					go func(idx int) {
						defer wg.Done()
						defer ginkgo.GinkgoRecover()

						cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
						if err != nil {
							results[idx] = clusterResult{err: fmt.Errorf("failed to create cluster %d: %w", idx, err)}
							return
						}
						if cluster.Id == nil {
							results[idx] = clusterResult{err: fmt.Errorf("cluster %d has nil ID", idx)}
							return
						}
						results[idx] = clusterResult{
							id:   *cluster.Id,
							name: cluster.Name,
						}
					}(i)
				}
				wg.Wait()

				// Collect all successful IDs first to ensure AfterEach can clean up all created clusters
				for _, r := range results {
					if r.err == nil && r.id != "" {
						clusterIDs = append(clusterIDs, r.id)
					}
				}

				// Verify all creations succeeded
				for i, r := range results {
					Expect(r.err).NotTo(HaveOccurred(), "cluster creation %d failed", i)
					Expect(r.id).NotTo(BeEmpty(), "cluster %d should have a non-empty ID", i)
					ginkgo.GinkgoWriter.Printf("Created cluster %d: ID=%s, Name=%s\n", i, r.id, r.name)
				}

				// Verify all cluster IDs are unique
				idSet := make(map[string]bool, len(clusterIDs))
				for _, id := range clusterIDs {
					Expect(idSet[id]).To(BeFalse(), "duplicate cluster ID detected: %s", id)
					idSet[id] = true
				}

				ginkgo.By("Wait for all clusters to reach Reconciled=True and Available=True")
				for i, clusterID := range clusterIDs {
					ginkgo.GinkgoWriter.Printf("Waiting for cluster %d (%s) to become Reconciled...\n", i, clusterID)
					err := h.WaitForClusterCondition(
						ctx,
						clusterID,
						client.ConditionTypeReconciled,
						openapi.ResourceConditionStatusTrue,
						h.Cfg.Timeouts.Cluster.Reconciled,
					)
					Expect(err).NotTo(HaveOccurred(), "cluster %d (%s) should reach Reconciled=True", i, clusterID)

					cluster, err := h.Client.GetCluster(ctx, clusterID)
					Expect(err).NotTo(HaveOccurred(), "failed to get cluster %d (%s)", i, clusterID)

					hasAvailable := h.HasResourceCondition(cluster.Status.Conditions,
						client.ConditionTypeAvailable, openapi.ResourceConditionStatusTrue)
					Expect(hasAvailable).To(BeTrue(),
						"cluster %d (%s) should have Available=True", i, clusterID)

					ginkgo.GinkgoWriter.Printf("Cluster %d (%s) reached Reconciled=True, Available=True\n", i, clusterID)
				}

				ginkgo.By("Verify each cluster has isolated Kubernetes resources (separate namespaces)")
				for i, clusterID := range clusterIDs {
					expectedLabels := map[string]string{
						"hyperfleet.io/cluster-id": clusterID,
					}
					err := h.VerifyNamespaceActive(ctx, clusterID, expectedLabels, nil)
					Expect(err).NotTo(HaveOccurred(),
						"cluster %d (%s) should have its own active namespace", i, clusterID)
					ginkgo.GinkgoWriter.Printf("Cluster %d (%s) has isolated namespace\n", i, clusterID)
				}

				ginkgo.By("Verify all adapter statuses are complete for each cluster")
				for i, clusterID := range clusterIDs {
					Eventually(func(g Gomega) {
						statuses, err := h.Client.GetClusterStatuses(ctx, clusterID)
						g.Expect(err).NotTo(HaveOccurred(), "failed to get cluster statuses for cluster %d (%s)", i, clusterID)
						g.Expect(statuses.Items).NotTo(BeEmpty(), "cluster %d (%s) should have adapter statuses", i, clusterID)

						// Build adapter status map
						adapterMap := make(map[string]openapi.AdapterStatus)
						for _, adapter := range statuses.Items {
							adapterMap[adapter.Adapter] = adapter
						}

						// Verify each required adapter has completed successfully
						for _, requiredAdapter := range h.Cfg.Adapters.Cluster {
							adapter, exists := adapterMap[requiredAdapter]
							g.Expect(exists).To(BeTrue(),
								"cluster %d (%s): required adapter %s should be present", i, clusterID, requiredAdapter)

							g.Expect(h.HasAdapterCondition(adapter.Conditions,
								client.ConditionTypeApplied, openapi.AdapterConditionStatusTrue)).To(BeTrue(),
								"cluster %d (%s): adapter %s should have Applied=True", i, clusterID, requiredAdapter)

							g.Expect(h.HasAdapterCondition(adapter.Conditions,
								client.ConditionTypeAvailable, openapi.AdapterConditionStatusTrue)).To(BeTrue(),
								"cluster %d (%s): adapter %s should have Available=True", i, clusterID, requiredAdapter)

							g.Expect(h.HasAdapterCondition(adapter.Conditions,
								client.ConditionTypeHealth, openapi.AdapterConditionStatusTrue)).To(BeTrue(),
								"cluster %d (%s): adapter %s should have Health=True", i, clusterID, requiredAdapter)
						}
					}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

					ginkgo.GinkgoWriter.Printf("Cluster %d (%s) has all adapter statuses complete\n", i, clusterID)
				}

				ginkgo.GinkgoWriter.Printf("Successfully validated %d concurrent cluster creations with resource isolation\n", concurrentClusterCount)
			})

		ginkgo.AfterEach(func(ctx context.Context) {
			if h == nil || len(clusterIDs) == 0 {
				return
			}

			ginkgo.By(fmt.Sprintf("Cleaning up %d test clusters", len(clusterIDs)))
			var cleanupErrors []error
			for _, clusterID := range clusterIDs {
				ginkgo.By("cleaning up cluster " + clusterID)
				if err := h.CleanupTestCluster(ctx, clusterID); err != nil {
					ginkgo.GinkgoWriter.Printf("ERROR: failed to cleanup cluster %s: %v\n", clusterID, err)
					cleanupErrors = append(cleanupErrors, err)
				}
			}
			Expect(cleanupErrors).To(BeEmpty(), "some clusters failed to cleanup")
		})
	},
)
