package adapter

import (
	"context"
	"os"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: adapter-failures][negative] Adapter framework can detect and report failures to cluster API endpoints",
	ginkgo.Label(labels.Tier1),
	func() {
		var (
			h              *helper.Helper
			chartPath      string
			baseDeployOpts helper.AdapterDeploymentOptions
		)

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			// Clone adapter Helm chart repository (shared across all tests)
			ginkgo.By("Clone adapter Helm chart repository")
			var cleanupChart func() error
			var err error
			chartPath, cleanupChart, err = h.CloneHelmChart(ctx, helper.HelmChartCloneOptions{
				Component: "adapter",
				RepoURL:   h.Cfg.AdapterDeployment.ChartRepo,
				Ref:       h.Cfg.AdapterDeployment.ChartRef,
				ChartPath: h.Cfg.AdapterDeployment.ChartPath,
				WorkDir:   helper.TestWorkDir,
			})
			Expect(err).NotTo(HaveOccurred(), "failed to clone adapter Helm chart")
			ginkgo.GinkgoWriter.Printf("Cloned adapter chart to: %s\n", chartPath)

			// Ensure chart cleanup after test
			ginkgo.DeferCleanup(func(ctx context.Context) {
				ginkgo.By("Cleanup cloned Helm chart")
				if err := cleanupChart(); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup chart: %v\n", err)
				}
			})

			// Set up base deployment options with common fields
			baseDeployOpts = helper.AdapterDeploymentOptions{
				Namespace: h.Cfg.Namespace,
				ChartPath: chartPath,
			}
		})

		ginkgo.It("should detect invalid K8s resource and report failure with clear error message",
			func(ctx context.Context) {
				// Test-specific adapter configuration
				adapterName := "cl-invalid-resource"

				// Set environment variable for envsubst expansion in values.yaml
				err := os.Setenv("ADAPTER_NAME", adapterName)
				Expect(err).NotTo(HaveOccurred(), "failed to set ADAPTER_NAME environment variable")
				ginkgo.DeferCleanup(func() {
					_ = os.Unsetenv("ADAPTER_NAME")
				})

				// Generate unique release name for this deployment
				releaseName := helper.GenerateAdapterReleaseName(helper.ResourceTypeClusters, adapterName)

				// Deploy the test adapter with invalid K8s resource configuration
				ginkgo.By("Deploy test adapter with invalid K8s resource configuration")

				// Create deployment options from base and add test-specific fields
				deployOpts := baseDeployOpts
				deployOpts.ReleaseName = releaseName
				deployOpts.AdapterName = adapterName

				err = h.DeployAdapter(ctx, deployOpts)
				// Ensure adapter cleanup happens after this test
				ginkgo.DeferCleanup(func(ctx context.Context) {
					ginkgo.By("Uninstall test adapter")
					if err := h.UninstallAdapter(ctx, releaseName, h.Cfg.Namespace); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: failed to uninstall adapter %s: %v\n", releaseName, err)
					} else {
						ginkgo.GinkgoWriter.Printf("Successfully uninstalled adapter: %s\n", releaseName)
					}
				})
				Expect(err).NotTo(HaveOccurred(), "failed to deploy test adapter")
				ginkgo.GinkgoWriter.Printf("Successfully deployed adapter: %s (release: %s)\n", adapterName, releaseName)

				// Create cluster after adapter is deployed
				ginkgo.By("Create test cluster")
				cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
				Expect(err).NotTo(HaveOccurred(), "failed to create cluster")
				Expect(cluster.Id).NotTo(BeNil(), "cluster ID should be generated")
				Expect(cluster.Name).NotTo(BeEmpty(), "cluster name should be present")
				clusterID := *cluster.Id
				clusterName := cluster.Name
				ginkgo.GinkgoWriter.Printf("Created cluster ID: %s, Name: %s\n", clusterID, clusterName)

				// Ensure cluster cleanup happens after this test
				ginkgo.DeferCleanup(func(ctx context.Context) {
					ginkgo.By("Cleanup test cluster " + clusterID)
					if err := h.CleanupTestCluster(ctx, clusterID); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup cluster %s: %v\n", clusterID, err)
					}
				})

				ginkgo.By("Verify initial status of cluster")
				// Verify initial conditions are False
				// Use Eventually to handle async condition propagation
				Eventually(func(g Gomega) {
					cluster, err = h.Client.GetCluster(ctx, clusterID)
					g.Expect(err).NotTo(HaveOccurred(), "failed to get cluster")
					g.Expect(cluster.Status).NotTo(BeNil(), "cluster status should be present")

					hasReconciledFalse := h.HasResourceCondition(cluster.Status.Conditions,
						client.ConditionTypeReconciled, openapi.ResourceConditionStatusFalse)
					g.Expect(hasReconciledFalse).To(BeTrue(),
						"initial cluster conditions should have Reconciled=False")

					hasAvailableFalse := h.HasResourceCondition(cluster.Status.Conditions,
						client.ConditionTypeLastKnownReconciled, openapi.ResourceConditionStatusFalse)
					g.Expect(hasAvailableFalse).To(BeTrue(),
						"initial cluster conditions should have LastKnownReconciled=False")
				}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

				ginkgo.By("Verify adapter execution detects failure and reports error")
				// Wait for adapter to process the cluster and report failure status
				Eventually(func(g Gomega) {
					statuses, err := h.Client.GetClusterStatuses(ctx, clusterID)
					g.Expect(err).NotTo(HaveOccurred(), "failed to get cluster statuses")
					g.Expect(statuses.Items).NotTo(BeEmpty(), "adapter should have reported status")

					// Find the test adapter status
					var adapterStatus *openapi.AdapterStatus
					for i, adapter := range statuses.Items {
						if adapter.Adapter == adapterName {
							adapterStatus = &statuses.Items[i]
							break
						}
					}

					g.Expect(adapterStatus).NotTo(BeNil(),
						"adapter %s should be present in adapter statuses", adapterName)

					// Validate adapter metadata
					g.Expect(adapterStatus.CreatedTime).NotTo(BeZero(),
						"adapter should have valid created_time")
					g.Expect(adapterStatus.LastReportTime).NotTo(BeZero(),
						"adapter should have valid last_report_time")
					g.Expect(adapterStatus.ObservedGeneration).To(Equal(int32(1)),
						"adapter should have observed_generation=1")

					// Find Available condition
					var availableCondition *openapi.AdapterCondition
					for i, condition := range adapterStatus.Conditions {
						if condition.Type == client.ConditionTypeAvailable {
							availableCondition = &adapterStatus.Conditions[i]
							break
						}
					}

					g.Expect(availableCondition).NotTo(BeNil(),
						"adapter should have Available condition")

					// Verify Available condition reports failure
					g.Expect(availableCondition.Status).To(Equal(openapi.AdapterConditionStatusFalse),
						"adapter Available condition should be False due to invalid K8s resource")

					// Verify error details are present in reason and message
					g.Expect(availableCondition.Reason).NotTo(BeNil(),
						"adapter Available condition should have reason")
					g.Expect(*availableCondition.Reason).NotTo(BeEmpty(),
						"adapter Available condition reason should not be empty")

					g.Expect(availableCondition.Message).NotTo(BeNil(),
						"adapter Available condition should have message")
					g.Expect(*availableCondition.Message).NotTo(BeEmpty(),
						"adapter Available condition message should not be empty")

				}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

				ginkgo.GinkgoWriter.Printf("Successfully validated adapter failure detection and reporting\n")
			})
	},
)
