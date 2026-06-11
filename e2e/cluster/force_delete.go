package cluster

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

var _ = ginkgo.Describe("[Suite: cluster][delete] Force-Delete Cluster Stuck in Finalizing",
	ginkgo.Serial,
	ginkgo.Label(labels.Tier2),
	func() {
		var h *helper.Helper

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()
		})

		ginkgo.It("should force-delete a cluster stuck in Finalizing and remove all its nodepools",
			func(ctx context.Context) {
				// --- Create resources in a healthy state ---

				ginkgo.By("Create cluster and wait for Reconciled")
				cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
				Expect(err).NotTo(HaveOccurred(), "failed to create cluster")
				Expect(cluster.Id).NotTo(BeNil())
				clusterID := *cluster.Id
				ginkgo.GinkgoWriter.Printf("Created cluster ID: %s, Name: %s\n", clusterID, cluster.Name)
				h.DeferClusterCleanup(clusterID)

				Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
					Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))

				ginkgo.By("Create a nodepool and wait for Reconciled")
				np, err := h.Client.CreateNodePoolFromPayload(ctx, clusterID, h.TestDataPath("payloads/nodepools/nodepool-request.json"))
				Expect(err).NotTo(HaveOccurred(), "failed to create nodepool")
				Expect(np.Id).NotTo(BeNil())
				nodepoolID := *np.Id

				Eventually(h.PollNodePool(ctx, clusterID, nodepoolID), h.Cfg.Timeouts.NodePool.Reconciled, h.Cfg.Polling.Interval).
					Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))

				// --- Simulate stuck deletion by scaling down an existing cluster adapter ---

				Expect(h.Cfg.Adapters.Cluster).NotTo(BeEmpty(), "cluster adapter config is required for this test")
				clAdapterName := h.Cfg.Adapters.Cluster[0]
				// Standard adapters are deployed with release name = adapter name (e.g., "cl-namespace", not "adapter-clusters-cl-namespace")
				clReleaseName := clAdapterName

				deploymentName, err := h.GetDeploymentName(ctx, h.Cfg.Namespace, clReleaseName)
				Expect(err).NotTo(HaveOccurred(), "failed to find cluster adapter deployment")

				ginkgo.By("Scale down cluster adapter to simulate unavailability")
				err = h.ScaleDeployment(ctx, h.Cfg.Namespace, deploymentName, 0)
				Expect(err).NotTo(HaveOccurred(), "failed to scale down cluster adapter")

				ginkgo.DeferCleanup(func(ctx context.Context) {
					ginkgo.By("Restore cluster adapter to 1 replica")
					if err := h.ScaleDeployment(ctx, h.Cfg.Namespace, deploymentName, 1); err != nil {
						ginkgo.GinkgoWriter.Printf("CRITICAL: failed to restore cluster adapter %s: %v\n", clAdapterName, err)
					}
				})

				ginkgo.By("Soft-delete the cluster")
				deletedCluster, err := h.Client.DeleteCluster(ctx, clusterID)
				Expect(err).NotTo(HaveOccurred(), "DELETE should succeed with 202")
				Expect(deletedCluster.DeletedTime).NotTo(BeNil(), "cluster should have deleted_time set")

				// Consistently proves the cluster remains stuck over time (not just a race)
				ginkgo.By("Verify cluster is stuck in Finalizing (not hard-deleted)")
				Consistently(func(g Gomega) {
					cl, err := h.Client.GetCluster(ctx, clusterID)
					g.Expect(err).NotTo(HaveOccurred(), "cluster should still be accessible")
					g.Expect(cl.DeletedTime).NotTo(BeNil(), "cluster should still be soft-deleted")
					g.Expect(h.HasResourceCondition(cl.Status.Conditions,
						client.ConditionTypeReconciled, openapi.ResourceConditionStatusFalse)).To(BeTrue(),
						"Reconciled should be False while stuck")
				}, h.Cfg.Timeouts.Adapter.Processing/2, h.Cfg.Polling.Interval).Should(Succeed())

				// --- Force-delete and verify cascade removal ---

				ginkgo.By("Force-delete the cluster")
				err = h.Client.ForceDeleteCluster(ctx, clusterID, "E2E test: cluster adapter unavailable")
				Expect(err).NotTo(HaveOccurred(), "force-delete should succeed with 204")

				ginkgo.By("Verify cluster is hard-deleted (404)")
				Eventually(h.PollClusterHTTPStatus(ctx, clusterID), h.Cfg.Timeouts.Cluster.Deleted, h.Cfg.Polling.Interval).
					Should(Equal(http.StatusNotFound))

				// Force-delete cascades: child nodepools are removed in the same transaction
				ginkgo.By("Verify child nodepool is also removed (404)")
				Eventually(h.PollNodePoolHTTPStatus(ctx, clusterID, nodepoolID), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
					Should(Equal(http.StatusNotFound))

				ginkgo.GinkgoWriter.Printf("Verified: force-delete removed cluster %s and nodepool %s\n", clusterID, nodepoolID)
			})
	},
)
