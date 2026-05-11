package cluster

import (
	"context"
	"errors"
	"net/http"
	"sync"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: cluster][delete] Re-DELETE Idempotency and API Boundary Tests",
	ginkgo.Label(labels.Tier1),
	func() {
		var h *helper.Helper
		var clusterID string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			ginkgo.By("creating cluster and waiting for Reconciled")
			var err error
			clusterID, err = h.GetTestCluster(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create cluster")

			Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))
		})

		ginkgo.It("should handle re-DELETE idempotently without changing deleted_time or generation",
			ginkgo.Label(labels.Disruptive),
			func(ctx context.Context) {
				ginkgo.By("pausing sentinel to prevent hard-delete between DELETE calls")
				sentinelDeployment, err := h.GetDeploymentName(ctx, h.Cfg.Namespace, helper.SentinelClustersRelease)
				Expect(err).NotTo(HaveOccurred(), "failed to find sentinel-clusters deployment")
				err = h.ScaleDeployment(ctx, h.Cfg.Namespace, sentinelDeployment, 0)
				Expect(err).NotTo(HaveOccurred(), "failed to scale sentinel to 0")
				ginkgo.DeferCleanup(func(ctx context.Context) {
					ginkgo.By("restoring sentinel-clusters to 1 replica")
					if err := h.ScaleDeployment(ctx, h.Cfg.Namespace, sentinelDeployment, 1); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: failed to restore sentinel: %v\n", err)
					}
				})

				ginkgo.By("sending first DELETE request")
				firstDelete, err := h.Client.DeleteCluster(ctx, clusterID)
				Expect(err).NotTo(HaveOccurred(), "first DELETE should succeed with 202")
				Expect(firstDelete.DeletedTime).NotTo(BeNil(), "first DELETE should set deleted_time")
				originalDeletedTime := *firstDelete.DeletedTime
				originalGeneration := firstDelete.Generation

				ginkgo.By("sending second DELETE request")
				secondDelete, err := h.Client.DeleteCluster(ctx, clusterID)
				Expect(err).NotTo(HaveOccurred(), "second DELETE should succeed with 202")
				Expect(secondDelete.DeletedTime).NotTo(BeNil(), "second DELETE should still have deleted_time")
				Expect(*secondDelete.DeletedTime).To(Equal(originalDeletedTime), "deleted_time should not change on re-DELETE")
				Expect(secondDelete.Generation).To(Equal(originalGeneration), "generation should not increment on re-DELETE")
			})

		ginkgo.It("should return 409 Conflict when creating nodepool under soft-deleted cluster",
			ginkgo.Label(labels.Negative),
			func(ctx context.Context) {
				ginkgo.By("soft-deleting the cluster")
				deletedCluster, err := h.Client.DeleteCluster(ctx, clusterID)
				Expect(err).NotTo(HaveOccurred(), "DELETE should succeed with 202")
				Expect(deletedCluster.DeletedTime).NotTo(BeNil())

				ginkgo.By("attempting to create a nodepool under the soft-deleted cluster")
				_, err = h.Client.CreateNodePoolFromPayload(ctx, clusterID, h.TestDataPath("payloads/nodepools/nodepool-request.json"))
				var httpErr *client.HTTPError
				Expect(errors.As(err, &httpErr)).To(BeTrue(), "error should be HTTPError")
				Expect(httpErr.StatusCode).To(Equal(http.StatusConflict),
					"creating nodepool under soft-deleted cluster should return 409")

				ginkgo.By("verifying no nodepool was created")
				npList, err := h.Client.ListNodePools(ctx, clusterID)
				Expect(err).NotTo(HaveOccurred())
				Expect(npList.Items).To(BeEmpty(), "no nodepools should exist under soft-deleted cluster")
			},
		)

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

var _ = ginkgo.Describe("[Suite: cluster][delete] DELETE Non-Existent Cluster",
	ginkgo.Label(labels.Tier1, labels.Negative),
	func() {
		var h *helper.Helper

		ginkgo.BeforeEach(func() {
			h = helper.New()
		})

		ginkgo.It("should return 404 when deleting a non-existent cluster", func(ctx context.Context) {
			ginkgo.By("sending DELETE for a non-existent cluster ID")
			_, err := h.Client.DeleteCluster(ctx, "non-existent-cluster-id-12345")
			var httpErr *client.HTTPError
			Expect(errors.As(err, &httpErr)).To(BeTrue(), "error should be HTTPError")
			Expect(httpErr.StatusCode).To(Equal(http.StatusNotFound),
				"DELETE on non-existent cluster should return 404")
		})
	},
)

var _ = ginkgo.Describe("[Suite: cluster][delete] Concurrent Deletion",
	ginkgo.Label(labels.Tier1),
	func() {
		var h *helper.Helper
		var clusterID string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			ginkgo.By("creating cluster and waiting for Reconciled")
			var err error
			clusterID, err = h.GetTestCluster(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create cluster")

			Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))
		})

		ginkgo.It("should produce a single soft-delete record from simultaneous DELETE requests", func(ctx context.Context) {
			ginkgo.By("capturing generation before deletion")
			clusterBefore, err := h.Client.GetCluster(ctx, clusterID)
			Expect(err).NotTo(HaveOccurred())
			genBefore := clusterBefore.Generation

			ginkgo.By("firing 5 concurrent DELETE requests")
			const concurrency = 5
			type deleteResult struct {
				cluster *openapi.Cluster
				err     error
			}
			results := make([]deleteResult, concurrency)
			var wg sync.WaitGroup
			wg.Add(concurrency)
			for i := range concurrency {
				go func(idx int) {
					defer wg.Done()
					defer ginkgo.GinkgoRecover()
					c, e := h.Client.DeleteCluster(ctx, clusterID)
					results[idx] = deleteResult{cluster: c, err: e}
				}(i)
			}
			wg.Wait()

			ginkgo.By("verifying all requests succeeded with consistent state")
			for i, r := range results {
				Expect(r.err).NotTo(HaveOccurred(), "DELETE request %d should succeed", i)
				Expect(r.cluster.DeletedTime).NotTo(BeNil(), "DELETE request %d should have deleted_time", i)
			}

			// All responses should carry identical deleted_time and generation
			referenceTime := *results[0].cluster.DeletedTime
			referenceGen := results[0].cluster.Generation
			for i := 1; i < concurrency; i++ {
				Expect(*results[i].cluster.DeletedTime).To(Equal(referenceTime),
					"all DELETE responses should have the same deleted_time")
				Expect(results[i].cluster.Generation).To(Equal(referenceGen),
					"all DELETE responses should have the same generation")
			}

			ginkgo.By("verifying generation incremented exactly once")
			Expect(referenceGen).To(Equal(genBefore+1),
				"generation should increment by exactly 1, not by the number of concurrent requests")

			ginkgo.By("verifying cluster completes deletion lifecycle")
			Eventually(h.PollClusterHTTPStatus(ctx, clusterID), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
				Should(Equal(http.StatusNotFound))
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

var _ = ginkgo.Describe("[Suite: cluster][delete] DELETE During Update Reconciliation",
	ginkgo.Label(labels.Tier1),
	func() {
		var h *helper.Helper
		var clusterID string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			ginkgo.By("creating cluster and waiting for Reconciled at generation 1")
			cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create cluster")
			Expect(cluster.Id).NotTo(BeNil())
			clusterID = *cluster.Id

			Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))
		})

		ginkgo.It("should complete deletion when DELETE is sent during update reconciliation", func(ctx context.Context) {
			ginkgo.By("sending PATCH to trigger generation 2 (do NOT wait for reconciliation)")
			patchedCluster, err := h.Client.PatchCluster(ctx, clusterID, openapi.ClusterPatchRequest{
				Spec: &openapi.ClusterSpec{"trigger-update": "true"},
			})
			Expect(err).NotTo(HaveOccurred(), "PATCH should succeed")
			Expect(patchedCluster.Generation).To(Equal(int32(2)))

			ginkgo.By("immediately sending DELETE before update reconciliation completes")
			deletedCluster, err := h.Client.DeleteCluster(ctx, clusterID)
			Expect(err).NotTo(HaveOccurred(), "DELETE should succeed with 202")
			Expect(deletedCluster.DeletedTime).NotTo(BeNil())
			Expect(deletedCluster.Generation).To(Equal(int32(3)),
				"generation should be 3: create(1) + PATCH(2) + DELETE(3)")

			ginkgo.By("verifying cluster is hard-deleted")
			Eventually(h.PollClusterHTTPStatus(ctx, clusterID), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
				Should(Equal(http.StatusNotFound))

			ginkgo.By("verifying downstream K8s namespace is cleaned up")
			Eventually(h.PollNamespacesByPrefix(ctx, clusterID), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
				Should(BeEmpty())
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

var _ = ginkgo.Describe("[Suite: cluster][delete] Recreate Cluster After Hard-Delete",
	ginkgo.Label(labels.Tier1),
	func() {
		var h *helper.Helper
		var firstClusterID string
		var secondClusterID string
		var originalCluster *openapi.Cluster

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			ginkgo.By("creating first cluster and waiting for Reconciled")
			cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create cluster")
			Expect(cluster.Id).NotTo(BeNil())
			firstClusterID = *cluster.Id
			originalCluster = cluster

			Eventually(h.PollCluster(ctx, firstClusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))
		})

		ginkgo.It("should create a new cluster with the same name after the original is hard-deleted", func(ctx context.Context) {
			ginkgo.By("deleting the first cluster and waiting for hard-delete")
			_, err := h.Client.DeleteCluster(ctx, firstClusterID)
			Expect(err).NotTo(HaveOccurred())

			Eventually(h.PollClusterHTTPStatus(ctx, firstClusterID), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
				Should(Equal(http.StatusNotFound))

			ginkgo.By("waiting for namespace cleanup from first cluster")
			Eventually(h.PollNamespacesByPrefix(ctx, firstClusterID), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
				Should(BeEmpty())

			ginkgo.By("creating a new cluster with the same name")
			kind := "Cluster"
			newCluster, err := h.Client.CreateCluster(ctx, openapi.ClusterCreateRequest{
				Kind:   &kind,
				Name:   originalCluster.Name,
				Labels: originalCluster.Labels,
				Spec:   originalCluster.Spec,
			})
			Expect(err).NotTo(HaveOccurred(), "creating cluster with reused name should succeed")
			Expect(newCluster.Id).NotTo(BeNil())
			secondClusterID = *newCluster.Id

			Expect(secondClusterID).NotTo(Equal(firstClusterID),
				"new cluster should have a different ID than the deleted one")
			Expect(newCluster.Generation).To(Equal(int32(1)),
				"new cluster should start at generation 1")

			ginkgo.By("waiting for the new cluster to reach Reconciled")
			Eventually(h.PollCluster(ctx, secondClusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))

			ginkgo.By("verifying the old cluster is still gone")
			_, err = h.Client.GetCluster(ctx, firstClusterID)
			var httpErr *client.HTTPError
			Expect(errors.As(err, &httpErr)).To(BeTrue())
			Expect(httpErr.StatusCode).To(Equal(http.StatusNotFound),
				"old cluster should remain 404 after recreate")
		})

		ginkgo.AfterEach(func(ctx context.Context) {
			if h == nil {
				return
			}
			for _, id := range []string{firstClusterID, secondClusterID} {
				if id == "" {
					continue
				}
				ginkgo.By("cleaning up cluster " + id)
				if cluster, err := h.Client.GetCluster(ctx, id); err == nil && cluster.DeletedTime == nil {
					if _, err := h.Client.DeleteCluster(ctx, id); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: API delete failed for cluster %s: %v\n", id, err)
					}
				}
				if err := h.CleanupTestCluster(ctx, id); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: cleanup failed for cluster %s: %v\n", id, err)
				}
			}
		})
	},
)
