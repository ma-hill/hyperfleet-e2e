package cluster

import (
	"context"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: cluster][perf] API list latency with filters and pagination",
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
		})

		ginkgo.It("should list clusters with search filter within acceptable latency", func(ctx context.Context) {
			ginkgo.By("measuring GET /clusters?search=... response time")
			filter := openapi.SearchParams("labels.environment='test'")
			start := time.Now()
			_, err := h.Client.ListClustersWithParams(ctx, &openapi.GetClustersParams{
				Search: &filter,
			})
			Expect(err).NotTo(HaveOccurred())
			elapsed := time.Since(start)
			ginkgo.GinkgoWriter.Printf("[PERF] GET /clusters (search filter) latency: %v\n", elapsed)
			Expect(elapsed).To(BeNumerically("<", config.ThresholdAPIList),
				"GET /clusters with search filter exceeded threshold")
		})

		ginkgo.It("should list clusters with page size limit within acceptable latency", func(ctx context.Context) {
			ginkgo.By("measuring GET /clusters?pageSize=10 response time")
			pageSize := openapi.QueryParamsPageSize(10)
			start := time.Now()
			_, err := h.Client.ListClustersWithParams(ctx, &openapi.GetClustersParams{
				PageSize: &pageSize,
			})
			Expect(err).NotTo(HaveOccurred())
			elapsed := time.Since(start)
			ginkgo.GinkgoWriter.Printf("[PERF] GET /clusters (pageSize=10) latency: %v\n", elapsed)
			Expect(elapsed).To(BeNumerically("<", config.ThresholdAPIList),
				"GET /clusters with pageSize exceeded threshold")
		})

		ginkgo.It("should list clusters with pagination within acceptable latency", func(ctx context.Context) {
			ginkgo.By("measuring GET /clusters?page=1&pageSize=10 response time")
			page := openapi.QueryParamsPage(1)
			pageSize := openapi.QueryParamsPageSize(10)
			start := time.Now()
			_, err := h.Client.ListClustersWithParams(ctx, &openapi.GetClustersParams{
				Page:     &page,
				PageSize: &pageSize,
			})
			Expect(err).NotTo(HaveOccurred())
			elapsed := time.Since(start)
			ginkgo.GinkgoWriter.Printf("[PERF] GET /clusters (page=1, pageSize=10) latency: %v\n", elapsed)
			Expect(elapsed).To(BeNumerically("<", config.ThresholdAPIList),
				"GET /clusters with pagination exceeded threshold")
		})
	},
)
