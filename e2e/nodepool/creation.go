package nodepool

import (
	"context"
	"fmt"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: nodepool][baseline] NodePool Resource Type Lifecycle",
	ginkgo.Label(labels.Tier0),
	func() {
		var h *helper.Helper
		var clusterID string
		var nodepoolID string
		var nodepoolName string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			// Get or create cluster for nodepool tests
			var err error
			clusterID, err = h.GetTestCluster(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to get test cluster")
			ginkgo.GinkgoWriter.Printf("Using cluster ID: %s\n", clusterID)

			// Create nodepool for all tests in this suite
			nodepool, err := h.Client.CreateNodePoolFromPayload(ctx, clusterID, h.TestDataPath("payloads/nodepools/nodepool-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create nodepool")
			Expect(nodepool.Id).NotTo(BeNil(), "nodepool ID should be generated")
			Expect(nodepool.Name).NotTo(BeEmpty(), "nodepool name should be present")
			nodepoolID = *nodepool.Id
			nodepoolName = nodepool.Name
			ginkgo.GinkgoWriter.Printf("Created nodepool ID: %s, Name: %s\n", nodepoolID, nodepoolName)
		})

		ginkgo.Describe("Basic Workflow Validation", ginkgo.Label(labels.Tier0), func() {
			// This test validates the end-to-end nodepool lifecycle workflow:
			// 1. Required adapter execution with comprehensive metadata validation
			// 2. Final nodepool state verification (Reconciled and Available conditions)
			ginkgo.It("should validate complete workflow from creation to Reconciled state",
				func(ctx context.Context) {
					ginkgo.By("Verify required adapter execution results")
					// Validate required adapters from config have completed successfully
					// If an adapter fails, we can identify which specific adapter failed
					Eventually(func(g Gomega) {
						statuses, err := h.Client.GetNodePoolStatuses(ctx, clusterID, nodepoolID)
						g.Expect(err).NotTo(HaveOccurred(), "failed to get nodepool statuses")
						g.Expect(statuses.Items).NotTo(BeEmpty(), "at least one adapter should have executed")

						// Build a map of adapter statuses for easy lookup
						adapterMap := make(map[string]openapi.AdapterStatus)
						for _, adapter := range statuses.Items {
							adapterMap[adapter.Adapter] = adapter
						}

						// Validate each required adapter from config
						for _, requiredAdapter := range h.Cfg.Adapters.NodePool {
							adapter, exists := adapterMap[requiredAdapter]
							g.Expect(exists).To(BeTrue(),
								"required adapter %s should be present in adapter statuses", requiredAdapter)

							// Validate adapter-level metadata
							g.Expect(adapter.CreatedTime).NotTo(BeZero(),
								"adapter %s should have valid created_time", adapter.Adapter)
							g.Expect(adapter.LastReportTime).NotTo(BeZero(),
								"adapter %s should have valid last_report_time", adapter.Adapter)
							g.Expect(adapter.ObservedGeneration).To(Equal(int32(1)),
								"adapter %s should have observed_generation=1 for new creation request", adapter.Adapter)

							hasApplied := h.HasAdapterCondition(
								adapter.Conditions,
								client.ConditionTypeApplied,
								openapi.AdapterConditionStatusTrue,
							)
							g.Expect(hasApplied).To(BeTrue(),
								"adapter %s should have Applied=True", adapter.Adapter)

							hasAvailable := h.HasAdapterCondition(
								adapter.Conditions,
								client.ConditionTypeAvailable,
								openapi.AdapterConditionStatusTrue,
							)
							g.Expect(hasAvailable).To(BeTrue(),
								"adapter %s should have Available=True", adapter.Adapter)

							hasHealth := h.HasAdapterCondition(
								adapter.Conditions,
								client.ConditionTypeHealth,
								openapi.AdapterConditionStatusTrue,
							)
							g.Expect(hasHealth).To(BeTrue(),
								"adapter %s should have Health=True", adapter.Adapter)

							// Validate condition metadata for each condition
							for _, condition := range adapter.Conditions {
								g.Expect(condition.Reason).NotTo(BeNil(),
									"adapter %s condition %s should have non-nil reason", adapter.Adapter, condition.Type)
								g.Expect(*condition.Reason).NotTo(BeEmpty(),
									"adapter %s condition %s should have non-empty reason", adapter.Adapter, condition.Type)

								g.Expect(condition.Message).NotTo(BeNil(),
									"adapter %s condition %s should have non-nil message", adapter.Adapter, condition.Type)
								g.Expect(*condition.Message).NotTo(BeEmpty(),
									"adapter %s condition %s should have non-empty message", adapter.Adapter, condition.Type)

								g.Expect(condition.LastTransitionTime).NotTo(BeZero(),
									"adapter %s condition %s should have valid last_transition_time", adapter.Adapter, condition.Type)
							}
						}
					}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

					ginkgo.By("Verify final nodepool state")
					// Wait for nodepool Reconciled condition and verify both Reconciled and Available conditions are True
					// This confirms the nodepool has reached the desired end state
					Eventually(h.PollNodePool(ctx, clusterID, nodepoolID), h.Cfg.Timeouts.NodePool.Reconciled, h.Cfg.Polling.Interval).
						Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))

					finalNodePool, err := h.Client.GetNodePool(ctx, clusterID, nodepoolID)
					Expect(err).NotTo(HaveOccurred(), "failed to get final nodepool state")
					Expect(finalNodePool.Status).NotTo(BeNil(), "nodepool status should be present")

					hasReconciled := h.HasResourceCondition(finalNodePool.Status.Conditions,
						client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue)
					Expect(hasReconciled).To(BeTrue(), "nodepool should have Reconciled=True condition")

					hasAvailable := h.HasResourceCondition(finalNodePool.Status.Conditions,
						client.ConditionTypeAvailable, openapi.ResourceConditionStatusTrue)
					Expect(hasAvailable).To(BeTrue(), "nodepool should have Available=True condition")

					// Validate observedGeneration for Reconciled and Available conditions
					for _, condition := range finalNodePool.Status.Conditions {
						if condition.Type == client.ConditionTypeReconciled || condition.Type == client.ConditionTypeAvailable {
							Expect(condition.ObservedGeneration).To(Equal(int32(1)),
								"nodepool condition %s should have observed_generation=1 for new creation request", condition.Type)
						}
					}

					// Validate adapter-specific conditions in nodepool status
					// Each required adapter should report its own condition type (e.g., NpConfigmapSuccessful)
					// Note: This check will be removed once these adapter-specific conditions are removed in the future
					for _, adapterName := range h.Cfg.Adapters.NodePool {
						expectedCondType := h.AdapterNameToConditionType(adapterName)
						hasAdapterCondition := h.HasResourceCondition(
							finalNodePool.Status.Conditions,
							expectedCondType,
							openapi.ResourceConditionStatusTrue,
						)
						Expect(hasAdapterCondition).To(BeTrue(),
							"nodepool should have %s=True condition for adapter %s",
							expectedCondType, adapterName)
					}
				})
		})

		ginkgo.Describe("K8s Resources Check Aligned with Preinstalled NodePool Related Adapters Specified", ginkgo.Label(labels.Tier0), func() {
			// This test validates Kubernetes resource creation for all configured adapters:
			// 1. Direct K8s resource verification for each adapter (configmap)
			// 2. Validation of resource metadata (labels, annotations) and status
			ginkgo.It("should create Kubernetes resources with correct templated values for all required adapters",
				func(ctx context.Context) {
					ginkgo.By("Verify Kubernetes resources for each required adapter")

					// Map from adapter name to K8s resource verification function.
					// Labels are used to FIND resources (via kubectl label selector).
					// Annotations are explicitly verified since they're not used in selectors.
					adapterResourceVerifiers := map[string]func() error{
						"np-configmap": func() error {
							expectedLabels := map[string]string{
								"hyperfleet.io/cluster-id":    clusterID,
								"hyperfleet.io/nodepool-id":   nodepoolID,
								"hyperfleet.io/nodepool-name": nodepoolName,
								"hyperfleet.io/resource-type": "configmap",
							}
							expectedAnnotations := map[string]string{
								"hyperfleet.io/generation": "1",
							}
							return h.VerifyConfigMap(ctx, clusterID, expectedLabels, expectedAnnotations)
						},
					}

					// Verify K8s resources for each configured adapter using Eventually
					for _, adapterName := range h.Cfg.Adapters.NodePool {
						verifier, exists := adapterResourceVerifiers[adapterName]
						if !exists {
							ginkgo.Fail(fmt.Sprintf("No K8s resource verifier defined for adapter %s - test configuration error", adapterName))
						}

						ginkgo.By("Verifying Kubernetes resource for adapter: " + adapterName)
						Eventually(func() error {
							return verifier()
						}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed(),
							"Kubernetes resource for adapter %s should be created and reach desired state", adapterName)
						ginkgo.GinkgoWriter.Printf("Successfully verified K8s resource for adapter: %s\n", adapterName)
					}

					ginkgo.By("Verify Final NodePool State")
					// Wait for nodepool Reconciled condition and verify both Reconciled and Available conditions are True
					// This confirms the nodepool workflow completed successfully and all K8s resources were created
					// Without this, adapters may still be creating resources during cleanup
					Eventually(h.PollNodePool(ctx, clusterID, nodepoolID), h.Cfg.Timeouts.NodePool.Reconciled, h.Cfg.Polling.Interval).
						Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))
				})
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
