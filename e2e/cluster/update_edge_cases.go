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

var _ = ginkgo.Describe("[Suite: cluster][update] Rapid Update Coalescing",
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

		ginkgo.It("should coalesce multiple rapid updates and reconcile to the latest generation", func(ctx context.Context) {
			ginkgo.By("sending three PATCH requests in rapid succession")
			patch1, err := h.Client.PatchCluster(ctx, clusterID, openapi.ClusterPatchRequest{
				Spec: &openapi.ClusterSpec{"update": "first"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(patch1.Generation).To(Equal(int32(2)))

			patch2, err := h.Client.PatchCluster(ctx, clusterID, openapi.ClusterPatchRequest{
				Spec: &openapi.ClusterSpec{"update": "second"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(patch2.Generation).To(Equal(int32(3)))

			patch3, err := h.Client.PatchCluster(ctx, clusterID, openapi.ClusterPatchRequest{
				Spec: &openapi.ClusterSpec{"update": "third"},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(patch3.Generation).To(Equal(int32(4)))

			ginkgo.By("waiting for all adapters to reconcile at the final generation")
			Eventually(h.PollClusterAdapterStatuses(ctx, clusterID), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
				Should(helper.HaveAllAdaptersAtGeneration(h.Cfg.Adapters.Cluster, int32(4)))

			ginkgo.By("verifying cluster reaches Reconciled=True at final generation")
			Eventually(func(g Gomega) {
				finalCluster, err := h.Client.GetCluster(ctx, clusterID)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(finalCluster.Generation).To(Equal(int32(4)))

				found := false
				for _, cond := range finalCluster.Status.Conditions {
					if cond.Type == client.ConditionTypeReconciled && cond.Status == openapi.ResourceConditionStatusTrue {
						found = true
						g.Expect(cond.ObservedGeneration).To(Equal(int32(4)))
					}
				}
				g.Expect(found).To(BeTrue(), "cluster should have Reconciled=True")
			}, h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).Should(Succeed())
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

var _ = ginkgo.Describe("[Suite: cluster][update] Labels-Only PATCH",
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

		ginkgo.It("should bump generation and trigger reconciliation from a labels-only PATCH", func(ctx context.Context) {
			ginkgo.By("capturing spec before labels-only PATCH")
			clusterBefore, err := h.Client.GetCluster(ctx, clusterID)
			Expect(err).NotTo(HaveOccurred())
			specBefore := clusterBefore.Spec

			ginkgo.By("sending labels-only PATCH (preserving existing labels)")
			newLabels := make(map[string]string)
			if clusterBefore.Labels != nil {
				for k, v := range *clusterBefore.Labels {
					newLabels[k] = v
				}
			}
			newLabels["env"] = "staging"
			newLabels["team"] = "fleet-management"
			patchedCluster, err := h.Client.PatchCluster(ctx, clusterID, openapi.ClusterPatchRequest{
				Labels: &newLabels,
			})
			Expect(err).NotTo(HaveOccurred(), "labels-only PATCH should succeed")
			Expect(patchedCluster.Generation).To(Equal(int32(2)),
				"generation should increment after labels-only PATCH")

			ginkgo.By("waiting for all adapters to reconcile at generation 2")
			Eventually(h.PollClusterAdapterStatuses(ctx, clusterID), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
				Should(helper.HaveAllAdaptersAtGeneration(h.Cfg.Adapters.Cluster, int32(2)))

			ginkgo.By("verifying cluster reaches Reconciled=True and Available=True")
			Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))
			Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
				Should(helper.HaveResourceCondition(client.ConditionTypeAvailable, openapi.ResourceConditionStatusTrue))

			finalCluster, err := h.Client.GetCluster(ctx, clusterID)
			Expect(err).NotTo(HaveOccurred())
			Expect(finalCluster.Labels).NotTo(BeNil(), "cluster should have labels")
			Expect((*finalCluster.Labels)["env"]).To(Equal("staging"))
			Expect((*finalCluster.Labels)["team"]).To(Equal("fleet-management"))
			Expect(finalCluster.Spec).To(Equal(specBefore),
				"spec should be unchanged after labels-only PATCH")
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

var _ = ginkgo.Describe("[Suite: cluster][update] No-Op PATCH",
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

		ginkgo.It("should not increment generation when PATCHing with identical spec", func(ctx context.Context) {
			ginkgo.By("PATCHing with a spec change to bump generation to 2")
			spec := openapi.ClusterSpec{"update-trigger": "gen2"}
			changed, err := h.Client.PatchCluster(ctx, clusterID, openapi.ClusterPatchRequest{
				Spec: &spec,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(changed.Generation).To(Equal(int32(2)), "generation should bump after spec change")

			ginkgo.By("replaying the same spec via PATCH")
			replayed, err := h.Client.PatchCluster(ctx, clusterID, openapi.ClusterPatchRequest{
				Spec: &spec,
			})
			Expect(err).NotTo(HaveOccurred(), "no-op PATCH should succeed")
			Expect(replayed.Generation).To(Equal(int32(2)),
				"generation should not increment for identical spec PATCH")
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
