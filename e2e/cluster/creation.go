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

var _ = ginkgo.Describe("[Suite: cluster][baseline] Cluster Resource Type Lifecycle",
	ginkgo.Label(labels.Tier0),
	func() {
		var h *helper.Helper
		var clusterID string
		var clusterName string

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			// Create cluster for all tests in this suite
			cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create cluster")
			Expect(cluster.Id).NotTo(BeNil(), "cluster ID should be generated")
			Expect(cluster.Name).NotTo(BeEmpty(), "cluster name should be present")
			clusterID = *cluster.Id
			clusterName = cluster.Name
			ginkgo.GinkgoWriter.Printf("Created cluster ID: %s, Name: %s\n", clusterID, clusterName)

			ginkgo.DeferCleanup(func(ctx context.Context) {
				if err := h.CleanupTestCluster(ctx, clusterID); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup cluster %s: %v\n", clusterID, err)
				}
			})
		})

		ginkgo.Describe("Basic Workflow Validation", ginkgo.Label(labels.Tier0), func() {
			// This test validates the end-to-end cluster lifecycle workflow:
			// 1. Required adapter execution with comprehensive metadata validation
			// 2. Final cluster state verification (Reconciled and Available conditions)
			ginkgo.It("should validate complete workflow from creation to Reconciled state",
				func(ctx context.Context) {
					ginkgo.By("Verify required adapter execution results")
					// Validate required adapters from config have completed successfully
					// If an adapter fails, we can identify which specific adapter failed
					Eventually(func(g Gomega) {
						statuses, err := h.Client.GetClusterStatuses(ctx, clusterID)
						g.Expect(err).NotTo(HaveOccurred(), "failed to get cluster statuses")
						g.Expect(statuses.Items).NotTo(BeEmpty(), "at least one adapter should have executed")

						// Build a map of adapter statuses for easy lookup
						adapterMap := make(map[string]openapi.AdapterStatus)
						for _, adapter := range statuses.Items {
							adapterMap[adapter.Adapter] = adapter
						}

						// Validate each required adapter from config
						for _, requiredAdapter := range h.Cfg.Adapters.Cluster {
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

					ginkgo.By("Verify final cluster state")
					// Wait for cluster Reconciled condition and verify both Reconciled and Available conditions are True
					// This confirms the cluster has reached the desired end state
					Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
						Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))

					finalCluster, err := h.Client.GetCluster(ctx, clusterID)
					Expect(err).NotTo(HaveOccurred(), "failed to get final cluster state")
					Expect(finalCluster.Status).NotTo(BeNil(), "cluster status should be present")

					hasReconciled := h.HasResourceCondition(finalCluster.Status.Conditions,
						client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue)
					Expect(hasReconciled).To(BeTrue(), "cluster should have Reconciled=True condition")

					hasAvailable := h.HasResourceCondition(finalCluster.Status.Conditions,
						client.ConditionTypeLastKnownReconciled, openapi.ResourceConditionStatusTrue)
					Expect(hasAvailable).To(BeTrue(), "cluster should have LastKnownReconciled=True condition")

					// Validate observedGeneration for Reconciled and LastKnownReconciled conditions
					for _, condition := range finalCluster.Status.Conditions {
						if condition.Type == client.ConditionTypeReconciled || condition.Type == client.ConditionTypeLastKnownReconciled {
							Expect(condition.ObservedGeneration).To(Equal(int32(1)),
								"cluster condition %s should have observed_generation=1 for new creation request", condition.Type)
						}
					}

					// Validate adapter-specific conditions in cluster status
					// Each required adapter should report its own condition type (e.g., ClNamespaceSuccessful, ClJobSuccessful)
					for _, adapterName := range h.Cfg.Adapters.Cluster {
						expectedCondType := h.AdapterNameToConditionType(adapterName)
						hasAdapterCondition := h.HasResourceCondition(
							finalCluster.Status.Conditions,
							expectedCondType,
							openapi.ResourceConditionStatusTrue,
						)
						Expect(hasAdapterCondition).To(BeTrue(),
							"cluster should have %s=True condition for adapter %s",
							expectedCondType, adapterName)
					}
				})
		})

		ginkgo.Describe("K8s Resources Check Aligned with Preinstalled Clusters Related Adapters Specified", ginkgo.Label(labels.Tier0), func() {
			// This test validates Kubernetes resource creation for adapters that create K8s resources:
			// 1. Direct K8s resource verification for each adapter (namespace, job, deployment)
			// 2. Validation of resource metadata (labels, annotations) and status
			// 3. Final cluster state verification
			//
			// Note: Not all adapters create K8s resources (e.g., cl-maestro interacts with Maestro service).
			// Adapters without K8s resources are verified via adapter status in "Basic Workflow Validation" test.
			ginkgo.It("should create Kubernetes resources with correct templated values for adapters that create K8s resources",
				func(ctx context.Context) {
					ginkgo.By("Verify Kubernetes resources for each required adapter")

					// Map from adapter name to K8s resource verification function.
					// Labels are used to FIND resources (via kubectl label selector).
					// Annotations are explicitly verified since they're not used in selectors.
					adapterResourceVerifiers := map[string]func() error{
						"cl-namespace": func() error {
							expectedLabels := map[string]string{
								"hyperfleet.io/cluster-id":     clusterID,
								"hyperfleet.io/cluster-name":   clusterName,
								"e2e.hyperfleet.io/managed-by": "test-framework",
							}
							expectedAnnotations := map[string]string{
								"hyperfleet.io/generation": "1",
							}
							return h.VerifyNamespaceActive(ctx, clusterID, expectedLabels, expectedAnnotations)
						},
						"cl-job": func() error {
							expectedLabels := map[string]string{
								"hyperfleet.io/cluster-id":    clusterID,
								"hyperfleet.io/resource-type": "job",
							}
							expectedAnnotations := map[string]string{
								"hyperfleet.io/generation": "1",
							}
							return h.VerifyJobComplete(ctx, clusterID, expectedLabels, expectedAnnotations)
						},
						"cl-deployment": func() error {
							expectedLabels := map[string]string{
								"hyperfleet.io/cluster-id":    clusterID,
								"hyperfleet.io/resource-type": "deployment",
							}
							expectedAnnotations := map[string]string{
								"hyperfleet.io/generation": "1",
							}
							return h.VerifyDeploymentAvailable(ctx, clusterID, expectedLabels, expectedAnnotations)
						},
					}

					// Verify K8s resources only for adapters that have verifiers defined
					// This explicitly tests only adapters that create K8s resources
					for adapterName, verifier := range adapterResourceVerifiers {
						ginkgo.By("Verifying Kubernetes resource for adapter: " + adapterName)
						Eventually(func() error {
							return verifier()
						}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed(),
							"Kubernetes resource for adapter %s should be created and reach desired state", adapterName)
						ginkgo.GinkgoWriter.Printf("Successfully verified K8s resource for adapter: %s\n", adapterName)
					}

					ginkgo.By("Verify final cluster state to ensure Reconciled before cleanup")
					// Wait for cluster Reconciled condition to prevent namespace deletion conflicts
					// Without this, adapters may still be creating resources during cleanup
					Eventually(h.PollCluster(ctx, clusterID), h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).
						Should(helper.HaveResourceCondition(client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue))
				})
		})

		ginkgo.Describe("Adapter Dependency Relationships Workflow Validation", ginkgo.Label(labels.Tier0), func() {
			// This test validates that dependent adapters complete successfully.
			// cl-deployment depends on cl-job — the workflow engine enforces this ordering.
			// We validate the final converged state: both adapters reach Applied=True, Available=True, Health=True.
			ginkgo.It("should validate cl-deployment dependency on cl-job with comprehensive condition checks",
				func(ctx context.Context) {
					ginkgo.By("Verify cl-job and cl-deployment both reach final converged state")
					Eventually(func(g Gomega) {
						statuses, err := h.Client.GetClusterStatuses(ctx, clusterID)
						g.Expect(err).NotTo(HaveOccurred(), "failed to get cluster statuses")

						adapterMap := make(map[string]openapi.AdapterStatus)
						for _, adapter := range statuses.Items {
							adapterMap[adapter.Adapter] = adapter
						}

						for _, name := range []string{"cl-job", "cl-deployment"} {
							adapter, exists := adapterMap[name]
							g.Expect(exists).To(BeTrue(), "adapter %s should be present in statuses", name)

							g.Expect(adapter.ObservedGeneration).To(Equal(int32(1)),
								"adapter %s should have observed_generation=1 for new creation request", name)

							g.Expect(h.HasAdapterCondition(adapter.Conditions,
								client.ConditionTypeApplied, openapi.AdapterConditionStatusTrue)).To(BeTrue(),
								"adapter %s should have Applied=True", name)

							g.Expect(h.HasAdapterCondition(adapter.Conditions,
								client.ConditionTypeAvailable, openapi.AdapterConditionStatusTrue)).To(BeTrue(),
								"adapter %s should have Available=True", name)

							g.Expect(h.HasAdapterCondition(adapter.Conditions,
								client.ConditionTypeHealth, openapi.AdapterConditionStatusTrue)).To(BeTrue(),
								"adapter %s should have Health=True", name)

							for _, condition := range adapter.Conditions {
								g.Expect(condition.Reason).NotTo(BeNil(),
									"adapter %s condition %s should have non-nil reason", name, condition.Type)
								g.Expect(*condition.Reason).NotTo(BeEmpty(),
									"adapter %s condition %s should have non-empty reason", name, condition.Type)
								g.Expect(condition.Message).NotTo(BeNil(),
									"adapter %s condition %s should have non-nil message", name, condition.Type)
								g.Expect(*condition.Message).NotTo(BeEmpty(),
									"adapter %s condition %s should have non-empty message", name, condition.Type)
								g.Expect(condition.LastTransitionTime).NotTo(BeZero(),
									"adapter %s condition %s should have valid last_transition_time", name, condition.Type)
							}
						}
					}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())
				})
		})

	},
)
