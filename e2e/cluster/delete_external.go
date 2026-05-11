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

var _ = ginkgo.Describe("[Suite: cluster][delete] External K8s Resource Deletion",
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

			ginkgo.By("confirming managed K8s namespaces exist")
			namespaces, err := h.K8sClient.FindNamespacesByPrefix(ctx, clusterID)
			Expect(err).NotTo(HaveOccurred())
			Expect(namespaces).NotTo(BeEmpty(), "managed namespaces should exist after Reconciled")
		})

		ginkgo.It("should treat externally-deleted K8s resources as finalized and complete hard-delete", func(ctx context.Context) {
			ginkgo.By("externally deleting all managed K8s namespaces (bypass the API)")
			namespaces, err := h.K8sClient.FindNamespacesByPrefix(ctx, clusterID)
			Expect(err).NotTo(HaveOccurred())

			for _, ns := range namespaces {
				err := h.K8sClient.DeleteNamespaceAndWait(ctx, ns)
				Expect(err).NotTo(HaveOccurred(), "failed to delete namespace %s", ns)
			}

			ginkgo.By("verifying all namespaces are gone")
			Eventually(h.PollNamespacesByPrefix(ctx, clusterID), h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).
				Should(BeEmpty())

			ginkgo.By("sending DELETE through the API")
			deletedCluster, err := h.Client.DeleteCluster(ctx, clusterID)
			Expect(err).NotTo(HaveOccurred())
			Expect(deletedCluster.DeletedTime).NotTo(BeNil())

			ginkgo.By("verifying adapters report Finalized=True with Health=True")
			Eventually(func(g Gomega) {
				var httpErr *client.HTTPError
				statuses, err := h.Client.GetClusterStatuses(ctx, clusterID)
				if errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound {
					return
				}
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(statuses).To(helper.HaveAllAdaptersWithCondition(
					h.Cfg.Adapters.Cluster, client.ConditionTypeFinalized, openapi.AdapterConditionStatusTrue))
				g.Expect(statuses).To(helper.HaveAllAdaptersWithCondition(
					h.Cfg.Adapters.Cluster, client.ConditionTypeHealth, openapi.AdapterConditionStatusTrue))
			}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

			ginkgo.By("verifying cluster is hard-deleted")
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
