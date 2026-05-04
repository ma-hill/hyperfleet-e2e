package cluster

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

var _ = ginkgo.Describe("[Suite: cluster][negative] Cluster Can Reflect Adapter Failure in Top-Level Status",
	ginkgo.Label(labels.Tier1, labels.Negative),
	func() {
		var (
			h              *helper.Helper
			chartPath      string
			baseDeployOpts helper.AdapterDeploymentOptions
		)

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			// Clone adapter Helm chart repository (shared across all tests in this Describe)
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

		ginkgo.It("should reflect adapter precondition failure in cluster top-level status",
			func(ctx context.Context) {
				adapterName := "cl-precondition-error"

				// Set environment variable for envsubst expansion in values.yaml
				err := os.Setenv("ADAPTER_NAME", adapterName)
				Expect(err).NotTo(HaveOccurred(), "failed to set ADAPTER_NAME environment variable")
				ginkgo.DeferCleanup(func() {
					_ = os.Unsetenv("ADAPTER_NAME")
				})

				// Generate unique release name for this deployment
				releaseName := helper.GenerateAdapterReleaseName(helper.ResourceTypeClusters, adapterName)

				// Deploy the precondition-error-adapter with invalid precondition URL
				ginkgo.By("Deploy dedicated precondition-error-adapter with invalid precondition URL")

				deployOpts := baseDeployOpts
				deployOpts.ReleaseName = releaseName
				deployOpts.AdapterName = adapterName

				err = h.DeployAdapter(ctx, deployOpts)
				// Ensure adapter cleanup happens after this test
				ginkgo.DeferCleanup(func(ctx context.Context) {
					ginkgo.By("Uninstall precondition-error-adapter")
					if err := h.UninstallAdapter(ctx, releaseName, h.Cfg.Namespace); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: failed to uninstall adapter %s: %v\n", releaseName, err)
					} else {
						ginkgo.GinkgoWriter.Printf("Successfully uninstalled adapter: %s\n", releaseName)
					}

					// Clean up Pub/Sub subscription created by the adapter
					ginkgo.By("Clean up Pub/Sub subscription")
					subscriptionID := h.Cfg.Namespace + "-" + helper.ResourceTypeClusters + "-" + adapterName
					if err := h.DeletePubSubSubscription(ctx, subscriptionID); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: failed to delete Pub/Sub subscription %s: %v\n", subscriptionID, err)
					}
				})
				Expect(err).NotTo(HaveOccurred(), "failed to deploy precondition-error-adapter")
				ginkgo.GinkgoWriter.Printf("Deployed precondition-error-adapter: release=%s\n", releaseName)

				// Create cluster after adapter is deployed
				ginkgo.By("Submit an API request to create a Cluster resource")
				cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
				Expect(err).NotTo(HaveOccurred(), "failed to create cluster")
				Expect(cluster.Id).NotTo(BeNil(), "cluster ID should be generated")
				Expect(cluster.Name).NotTo(BeEmpty(), "cluster name should be present")
				clusterID := *cluster.Id
				ginkgo.GinkgoWriter.Printf("Created cluster ID: %s, Name: %s\n", clusterID, cluster.Name)

				// Ensure cluster cleanup happens after this test
				ginkgo.DeferCleanup(func(ctx context.Context) {
					ginkgo.By("Cleanup test cluster " + clusterID)
					if err := h.CleanupTestCluster(ctx, clusterID); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup cluster %s: %v\n", clusterID, err)
					}
				})

				// Step 3: Verify adapter failure is reported via status API
				ginkgo.By("Verify adapter failure is reported via status API")
				Eventually(func(g Gomega) {
					statuses, err := h.Client.GetClusterStatuses(ctx, clusterID)
					g.Expect(err).NotTo(HaveOccurred(), "failed to get cluster statuses")

					// Find the precondition-error-adapter in statuses
					var adapterStatus *openapi.AdapterStatus
					for i, status := range statuses.Items {
						if status.Adapter == adapterName {
							adapterStatus = &statuses.Items[i]
							break
						}
					}
					g.Expect(adapterStatus).NotTo(BeNil(),
						"precondition-error-adapter should be present in statuses response")

					// Verify Applied=False
					g.Expect(h.HasAdapterCondition(adapterStatus.Conditions,
						client.ConditionTypeApplied, openapi.AdapterConditionStatusFalse)).To(BeTrue(),
						"precondition-error-adapter should have Applied=False")

					// Verify Available=False
					g.Expect(h.HasAdapterCondition(adapterStatus.Conditions,
						client.ConditionTypeAvailable, openapi.AdapterConditionStatusFalse)).To(BeTrue(),
						"precondition-error-adapter should have Available=False")

					// Verify Health=False with reason/message indicating precondition failure
					g.Expect(h.HasAdapterCondition(adapterStatus.Conditions,
						client.ConditionTypeHealth, openapi.AdapterConditionStatusFalse)).To(BeTrue(),
						"precondition-error-adapter should have Health=False")

					healthCond := h.GetCondition(adapterStatus.Conditions, client.ConditionTypeHealth)
					g.Expect(healthCond).NotTo(BeNil(), "Health condition should exist")
					g.Expect(healthCond.Reason).NotTo(BeNil(), "Health condition reason should not be nil")
					g.Expect(*healthCond.Reason).NotTo(BeEmpty(), "Health condition reason should not be empty")
					g.Expect(healthCond.Message).NotTo(BeNil(), "Health condition message should not be nil")
					g.Expect(*healthCond.Message).NotTo(BeEmpty(), "Health condition message should not be empty")

					ginkgo.GinkgoWriter.Printf("Verified adapter failure: Applied=False, Available=False, Health=False (reason=%s)\n",
						*healthCond.Reason)
				}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

				// Step 4: Verify cluster top-level status reflects adapter failure
				ginkgo.By("Verify cluster top-level status reflects adapter failure")
				// The cluster should remain Reconciled=False and Available=False while an adapter reports failure.
				// Use Consistently to verify the conditions remain False over multiple polling cycles.
				Consistently(func(g Gomega) {
					cl, err := h.Client.GetCluster(ctx, clusterID)
					g.Expect(err).NotTo(HaveOccurred(), "failed to get cluster")
					g.Expect(cl.Status).NotTo(BeNil(), "cluster status should be present")

					g.Expect(h.HasResourceCondition(cl.Status.Conditions,
						client.ConditionTypeReconciled, openapi.ResourceConditionStatusFalse)).To(BeTrue(),
						"cluster Reconciled condition should remain False")

					g.Expect(h.HasResourceCondition(cl.Status.Conditions,
						client.ConditionTypeAvailable, openapi.ResourceConditionStatusFalse)).To(BeTrue(),
						"cluster Available condition should remain False")
				}, h.Cfg.Polling.Interval*3, h.Cfg.Polling.Interval).Should(Succeed())

				ginkgo.GinkgoWriter.Printf("Verified cluster top-level status: Reconciled=False, Available=False (reflecting adapter failure)\n")
			})
	},
)
