package cluster

import (
	"context"
	"errors"
	"net/http"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: cluster][delete] Cluster Deletion Lifecycle",
	ginkgo.Label(labels.Tier0),
	func() {
		var h *helper.Helper
		var clusterID string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			ginkgo.By("creating cluster and waiting for Reconciled")
			var err error
			clusterID, err = h.GetTestCluster(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create cluster")

			ginkgo.DeferCleanup(func(ctx context.Context) {
				if cluster, err := h.Client.GetCluster(ctx, clusterID); err == nil && cluster.DeletedTime == nil {
					if _, err := h.Client.DeleteCluster(ctx, clusterID); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: API delete failed for cluster %s: %v\n", clusterID, err)
					}
				}
				if err := h.CleanupTestCluster(ctx, clusterID); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: cleanup failed for cluster %s: %v\n", clusterID, err)
				}
			})

			Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))
		})

		ginkgo.It("should complete full deletion lifecycle from soft-delete through hard-delete", func(ctx context.Context) {
			clusterBefore, err := h.Client.GetCluster(ctx, clusterID)
			Expect(err).NotTo(HaveOccurred())

			ginkgo.By("soft-deleting the cluster")
			deletedCluster, err := h.Client.DeleteCluster(ctx, clusterID)
			Expect(err).NotTo(HaveOccurred(), "DELETE request should succeed with 202")
			Expect(deletedCluster.DeletedTime).NotTo(BeNil(), "soft-deleted cluster should have deleted_time set")
			Expect(deletedCluster.Generation).To(Equal(clusterBefore.Generation+1), "generation should increment after soft-delete")

			ginkgo.By("waiting for cluster adapters to finalize and cluster to be hard-deleted")
			// Hard-delete executes atomically within the POST /adapter_statuses request that
			// computes Reconciled=True, so there is no observable window to see Finalized=True
			// on the statuses endpoint. Accept either Finalized=True OR 404 (already hard-deleted).
			Eventually(func(g Gomega) {
				var httpErr *client.HTTPError
				_, err := h.Client.GetCluster(ctx, clusterID)
				if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
					return
				}
				g.Expect(err).NotTo(HaveOccurred())
				statuses, err := h.Client.GetClusterStatuses(ctx, clusterID)
				if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
					return
				}
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(statuses).To(helper.HaveAllAdaptersWithCondition(
					h.Cfg.Adapters.Cluster, client.ConditionTypeFinalized, openapi.AdapterConditionStatusTrue))
			}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

			ginkgo.By("confirming cluster is hard-deleted")
			Eventually(h.PollClusterHTTPStatus(ctx, clusterID), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
				Should(Equal(http.StatusNotFound))

			ginkgo.By("verifying downstream K8s namespace is cleaned up")
			Eventually(h.PollNamespacesByPrefix(ctx, clusterID), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
				Should(BeEmpty())
		})

		ginkgo.It("should return 409 Conflict when PATCHing a soft-deleted cluster", ginkgo.Label(labels.Negative), func(ctx context.Context) {
			ginkgo.By("soft-deleting the cluster")
			deletedCluster, err := h.Client.DeleteCluster(ctx, clusterID)
			Expect(err).NotTo(HaveOccurred(), "DELETE request should succeed with 202")
			Expect(deletedCluster.DeletedTime).NotTo(BeNil(), "soft-deleted cluster should have deleted_time set")
			deletedGeneration := deletedCluster.Generation

			ginkgo.By("attempting PATCH on the soft-deleted cluster")
			patchReq := openapi.ClusterPatchRequest{
				Spec: &openapi.ClusterSpec{"updated-key": "should-not-work"},
			}
			_, patchErr := h.Client.PatchCluster(ctx, clusterID, patchReq)
			Expect(patchErr).To(HaveOccurred(), "PATCH on soft-deleted cluster should be rejected")
			var httpErr *client.HTTPError
			Expect(errors.As(patchErr, &httpErr)).To(BeTrue(), "error should be an HTTP error")
			Expect(httpErr.StatusCode).To(Or(Equal(http.StatusConflict), Equal(http.StatusNotFound)),
				"PATCH should be rejected once cluster deletion has started")

			if httpErr.StatusCode == http.StatusConflict {
				ginkgo.By("verifying cluster state is unchanged after rejected PATCH")
				cluster, err := h.Client.GetCluster(ctx, clusterID)
				Expect(err).NotTo(HaveOccurred())
				Expect(cluster.Generation).To(Equal(deletedGeneration), "generation should not change after rejected PATCH")
				Expect(cluster.DeletedTime).NotTo(BeNil(), "cluster should still be marked as deleted")
			}
		})

	},
)

var _ = ginkgo.Describe("[Suite: cluster][delete] Cluster Cascade Deletion",
	ginkgo.Label(labels.Tier0),
	func() {
		var h *helper.Helper
		var clusterID string
		var nodepoolID1 string
		var nodepoolID2 string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			ginkgo.By("creating cluster and waiting for Reconciled")
			var err error
			clusterID, err = h.GetTestCluster(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create cluster")

			ginkgo.DeferCleanup(func(ctx context.Context) {
				if cluster, err := h.Client.GetCluster(ctx, clusterID); err == nil && cluster.DeletedTime == nil {
					if _, err := h.Client.DeleteCluster(ctx, clusterID); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: API delete failed for cluster %s: %v\n", clusterID, err)
					}
				}
				if err := h.CleanupTestCluster(ctx, clusterID); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: cleanup failed for cluster %s: %v\n", clusterID, err)
				}
			})

			Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))

			ginkgo.By("creating two nodepools")
			np1, err := h.Client.CreateNodePoolFromPayload(ctx, clusterID, h.TestDataPath("payloads/nodepools/nodepool-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create first nodepool")
			Expect(np1.Id).NotTo(BeNil())
			nodepoolID1 = *np1.Id

			np2, err := h.Client.CreateNodePoolFromPayload(ctx, clusterID, h.TestDataPath("payloads/nodepools/nodepool-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create second nodepool")
			Expect(np2.Id).NotTo(BeNil())
			nodepoolID2 = *np2.Id

			ginkgo.By("waiting for both nodepools to reach Reconciled")
			Eventually(h.PollNodePool(ctx, clusterID, nodepoolID1), h.Cfg.Timeouts.NodePool.Reconciled, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))

			Eventually(h.PollNodePool(ctx, clusterID, nodepoolID2), h.Cfg.Timeouts.NodePool.Reconciled, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))
		})

		ginkgo.It("should cascade deletion to child nodepools and hard-delete all resources", func(ctx context.Context) {
			ginkgo.By("soft-deleting the cluster")
			deletedCluster, err := h.Client.DeleteCluster(ctx, clusterID)
			Expect(err).NotTo(HaveOccurred(), "DELETE request should succeed with 202")
			Expect(deletedCluster.DeletedTime).NotTo(BeNil(), "cluster should have deleted_time set")

			ginkgo.By("verifying cascade: both child nodepools are soft-deleted or already hard-deleted")
			Eventually(func(g Gomega) {
				np1, err := h.Client.GetNodePool(ctx, clusterID, nodepoolID1)
				var httpErr *client.HTTPError
				if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
					return
				}
				g.Expect(err).NotTo(HaveOccurred(), "first nodepool should be accessible or 404")
				g.Expect(np1.DeletedTime).NotTo(BeNil(), "first nodepool should have deleted_time set via cascade")
			}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

			Eventually(func(g Gomega) {
				np2, err := h.Client.GetNodePool(ctx, clusterID, nodepoolID2)
				var httpErr *client.HTTPError
				if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
					return
				}
				g.Expect(err).NotTo(HaveOccurred(), "second nodepool should be accessible or 404")
				g.Expect(np2.DeletedTime).NotTo(BeNil(), "second nodepool should have deleted_time set via cascade")
			}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

			ginkgo.By("waiting for both nodepools to be hard-deleted")
			Eventually(h.PollNodePoolHTTPStatus(ctx, clusterID, nodepoolID1), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
				Should(Equal(http.StatusNotFound))

			Eventually(h.PollNodePoolHTTPStatus(ctx, clusterID, nodepoolID2), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
				Should(Equal(http.StatusNotFound))

			ginkgo.By("waiting for cluster to be hard-deleted after all nodepools removed")
			Eventually(h.PollClusterHTTPStatus(ctx, clusterID), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
				Should(Equal(http.StatusNotFound))

			ginkgo.By("verifying downstream K8s namespace is cleaned up")
			Eventually(h.PollNamespacesByPrefix(ctx, clusterID), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
				Should(BeEmpty())
		})

	},
)
