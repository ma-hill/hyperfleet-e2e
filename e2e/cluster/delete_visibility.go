package cluster

import (
	"context"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: cluster][delete] Soft-Deleted Cluster Visibility",
	ginkgo.Label(labels.Tier1, labels.Disruptive),
	ginkgo.Serial,
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

		ginkgo.It("should remain visible via GET and LIST before hard-delete", func(ctx context.Context) {
			ginkgo.By("pausing sentinel to freeze reconciliation before soft-delete")
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

			ginkgo.By("soft-deleting the cluster")
			deletedCluster, err := h.Client.DeleteCluster(ctx, clusterID)
			Expect(err).NotTo(HaveOccurred())
			Expect(deletedCluster.DeletedTime).NotTo(BeNil())

			ginkgo.By("verifying GET returns the soft-deleted cluster with deleted_time")
			Eventually(func(g Gomega) {
				cluster, err := h.Client.GetCluster(ctx, clusterID)
				g.Expect(err).NotTo(HaveOccurred(), "GET should return 200, not 404")
				g.Expect(cluster.DeletedTime).NotTo(BeNil(), "cluster should have deleted_time set")
			}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

			ginkgo.By("verifying LIST includes the soft-deleted cluster")
			Eventually(func(g Gomega) {
				clusterList, err := h.Client.ListClusters(ctx)
				g.Expect(err).NotTo(HaveOccurred())

				found := false
				for _, c := range clusterList.Items {
					if c.Id != nil && *c.Id == clusterID {
						g.Expect(c.DeletedTime).NotTo(BeNil(), "cluster in LIST should have deleted_time")
						found = true
					}
				}
				g.Expect(found).To(BeTrue(), "soft-deleted cluster should appear in LIST")
			}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())
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

var _ = ginkgo.Describe("[Suite: cluster][delete] LIST Shows Active and Soft-Deleted Clusters",
	ginkgo.Label(labels.Tier1, labels.Disruptive),
	ginkgo.Serial,
	func() {
		var h *helper.Helper
		var activeClusterID string
		var deletedClusterID string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			ginkgo.By("creating two clusters and waiting for Reconciled")
			var err error
			activeClusterID, err = h.GetTestCluster(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create active cluster")

			deletedClusterID, err = h.GetTestCluster(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create cluster to delete")

			Eventually(h.PollCluster(ctx, activeClusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))

			Eventually(h.PollCluster(ctx, deletedClusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))
		})

		ginkgo.It("should return both active and soft-deleted clusters in LIST", func(ctx context.Context) {
			ginkgo.By("pausing sentinel to freeze reconciliation before soft-delete")
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

			ginkgo.By("soft-deleting one cluster")
			_, err = h.Client.DeleteCluster(ctx, deletedClusterID)
			Expect(err).NotTo(HaveOccurred())

			ginkgo.By("verifying LIST returns both clusters simultaneously")
			Eventually(func(g Gomega) {
				clusterList, err := h.Client.ListClusters(ctx)
				g.Expect(err).NotTo(HaveOccurred())

				var foundActive, foundDeleted bool
				for _, c := range clusterList.Items {
					if c.Id == nil {
						continue
					}
					if *c.Id == activeClusterID {
						g.Expect(c.DeletedTime).To(BeNil(), "active cluster should not have deleted_time")
						foundActive = true
					}
					if *c.Id == deletedClusterID {
						g.Expect(c.DeletedTime).NotTo(BeNil(), "deleted cluster should have deleted_time")
						foundDeleted = true
					}
				}
				g.Expect(foundActive).To(BeTrue(), "active cluster should appear in LIST")
				g.Expect(foundDeleted).To(BeTrue(), "soft-deleted cluster should appear in LIST")
			}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

			ginkgo.By("verifying GET returns correct state for each cluster")
			activeCluster, err := h.Client.GetCluster(ctx, activeClusterID)
			Expect(err).NotTo(HaveOccurred())
			Expect(activeCluster.DeletedTime).To(BeNil())

			Eventually(func(g Gomega) {
				deletedCluster, err := h.Client.GetCluster(ctx, deletedClusterID)
				g.Expect(err).NotTo(HaveOccurred(), "GET on soft-deleted cluster should return 200")
				g.Expect(deletedCluster.DeletedTime).NotTo(BeNil())
			}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())
		})

		ginkgo.AfterEach(func(ctx context.Context) {
			if h == nil {
				return
			}
			for _, id := range []string{activeClusterID, deletedClusterID} {
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
