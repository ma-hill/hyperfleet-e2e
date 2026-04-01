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

var _ = ginkgo.Describe("[Suite: cluster][negative] Cluster Can Reach Correct Status After Adapter Crash and Recovery",
	ginkgo.Serial,
	ginkgo.Label(labels.Tier2, labels.Negative),
	func() {
		var (
			h                *helper.Helper
			adapterChartPath string
			apiChartPath     string
			baseDeployOpts   helper.AdapterDeploymentOptions
		)

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			// Clone adapter Helm chart
			ginkgo.By("Clone adapter Helm chart repository")
			var cleanupAdapterChart func() error
			var err error
			adapterChartPath, cleanupAdapterChart, err = h.CloneHelmChart(ctx, helper.HelmChartCloneOptions{
				Component: "adapter",
				RepoURL:   h.Cfg.AdapterDeployment.ChartRepo,
				Ref:       h.Cfg.AdapterDeployment.ChartRef,
				ChartPath: h.Cfg.AdapterDeployment.ChartPath,
				WorkDir:   helper.TestWorkDir,
			})
			Expect(err).NotTo(HaveOccurred(), "failed to clone adapter Helm chart")

			ginkgo.DeferCleanup(func(ctx context.Context) {
				ginkgo.By("Cleanup cloned adapter Helm chart")
				if err := cleanupAdapterChart(); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup adapter chart: %v\n", err)
				}
			})

			// Clone API Helm chart (needed to upgrade required adapters config)
			ginkgo.By("Clone API Helm chart repository")
			apiChartRepo := os.Getenv("API_CHART_REPO")
			if apiChartRepo == "" {
				apiChartRepo = "https://github.com/openshift-hyperfleet/hyperfleet-api.git"
			}
			apiChartRef := os.Getenv("API_CHART_REF")
			if apiChartRef == "" {
				apiChartRef = h.Cfg.AdapterDeployment.ChartRef
			}
			Expect(apiChartRef).NotTo(BeEmpty(), "API_CHART_REF must be set or ADAPTER_CHART_REF must be configured")
			apiChartPathCfg := os.Getenv("API_CHART_PATH")
			if apiChartPathCfg == "" {
				apiChartPathCfg = "charts"
			}

			var cleanupAPIChart func() error
			apiChartPath, cleanupAPIChart, err = h.CloneHelmChart(ctx, helper.HelmChartCloneOptions{
				Component: "api",
				RepoURL:   apiChartRepo,
				Ref:       apiChartRef,
				ChartPath: apiChartPathCfg,
				WorkDir:   helper.TestWorkDir,
			})
			Expect(err).NotTo(HaveOccurred(), "failed to clone API Helm chart")

			ginkgo.DeferCleanup(func(ctx context.Context) {
				ginkgo.By("Cleanup cloned API Helm chart")
				if err := cleanupAPIChart(); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup API chart: %v\n", err)
				}
			})

			baseDeployOpts = helper.AdapterDeploymentOptions{
				Namespace: h.Cfg.Namespace,
				ChartPath: adapterChartPath,
			}
		})

		ginkgo.It("should recover and report correct status after adapter crash",
			func(ctx context.Context) {
				adapterName := "cl-crash"

				err := os.Setenv("ADAPTER_NAME", adapterName)
				Expect(err).NotTo(HaveOccurred(), "failed to set ADAPTER_NAME environment variable")
				ginkgo.DeferCleanup(func() {
					_ = os.Unsetenv("ADAPTER_NAME")
				})

				releaseName := helper.GenerateAdapterReleaseName(helper.ResourceTypeClusters, adapterName)

				// Step 1a: Deploy dedicated crash-adapter
				ginkgo.By("Deploy dedicated crash-adapter")
				deployOpts := baseDeployOpts
				deployOpts.ReleaseName = releaseName
				deployOpts.AdapterName = adapterName

				err = h.DeployAdapter(ctx, deployOpts)
				// Register adapter cleanup (executed AFTER API config restore due to LIFO)
				ginkgo.DeferCleanup(func(ctx context.Context) {
					ginkgo.By("Uninstall crash-adapter")
					if err := h.UninstallAdapter(ctx, releaseName, h.Cfg.Namespace); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: failed to uninstall adapter %s: %v\n", releaseName, err)
					}

					ginkgo.By("Clean up Pub/Sub subscription")
					subscriptionID := h.Cfg.Namespace + "-" + helper.ResourceTypeClusters + "-" + adapterName
					if err := h.DeletePubSubSubscription(ctx, subscriptionID); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: failed to delete Pub/Sub subscription %s: %v\n", subscriptionID, err)
					}
				})
				Expect(err).NotTo(HaveOccurred(), "failed to deploy crash-adapter")
				ginkgo.GinkgoWriter.Printf("Deployed crash-adapter: release=%s\n", releaseName)

				// Step 1b: Upgrade API to add crash-adapter to required adapters
				ginkgo.By("Upgrade API to add crash-adapter to required adapters")
				originalAdapters := h.GetAPIRequiredClusterAdapters()
				updatedAdapters := append(append([]string{}, originalAdapters...), adapterName)

				// Register API config restore AFTER adapter cleanup registration (LIFO → executes FIRST)
				ginkgo.DeferCleanup(func(ctx context.Context) {
					ginkgo.By("Restore API required adapters to original config")
					if err := h.RestoreAPIRequiredAdaptersWithRetry(ctx, apiChartPath, h.Cfg.Namespace, originalAdapters, 3); err != nil {
						ginkgo.GinkgoWriter.Printf("CRITICAL: %v\n", err)
					}
				})

				err = h.UpgradeAPIRequiredAdapters(ctx, apiChartPath, h.Cfg.Namespace, updatedAdapters)
				Expect(err).NotTo(HaveOccurred(), "failed to upgrade API with crash-adapter in required adapters")

				// Step 1c: Find deployment name and scale down to simulate crash
				deploymentName, err := h.GetDeploymentName(ctx, h.Cfg.Namespace, releaseName)
				Expect(err).NotTo(HaveOccurred(), "failed to find crash-adapter deployment name")

				ginkgo.By("Scale down crash-adapter to simulate crash")
				err = h.ScaleDeployment(ctx, h.Cfg.Namespace, deploymentName, 0)
				Expect(err).NotTo(HaveOccurred(), "failed to scale down crash-adapter")

				// Step 2: Create cluster while crash-adapter is down
				ginkgo.By("Submit an API request to create a Cluster resource")
				cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
				Expect(err).NotTo(HaveOccurred(), "failed to create cluster")
				Expect(cluster.Id).NotTo(BeNil(), "cluster ID should be generated")
				clusterID := *cluster.Id
				ginkgo.GinkgoWriter.Printf("Created cluster ID: %s, Name: %s\n", clusterID, cluster.Name)

				ginkgo.DeferCleanup(func(ctx context.Context) {
					ginkgo.By("Cleanup test cluster " + clusterID)
					if err := h.CleanupTestCluster(ctx, clusterID); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup cluster %s: %v\n", clusterID, err)
					}
				})

				// Step 3: Verify crash-adapter has not reported status and cluster is not Ready
				ginkgo.By("Verify crash-adapter has not reported status")
				Eventually(func(g Gomega) {
					statuses, err := h.Client.GetClusterStatuses(ctx, clusterID)
					g.Expect(err).NotTo(HaveOccurred(), "failed to get cluster statuses")

					for _, status := range statuses.Items {
						g.Expect(status.Adapter).NotTo(Equal(adapterName),
							"crash-adapter should NOT be present in statuses (it is unavailable)")
					}

					// At least one other adapter should have reported
					g.Expect(len(statuses.Items)).To(BeNumerically(">", 0),
						"other adapters should have reported their statuses")
				}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

				ginkgo.By("Verify cluster Ready=False due to missing crash-adapter")
				Consistently(func(g Gomega) {
					cl, err := h.Client.GetCluster(ctx, clusterID)
					g.Expect(err).NotTo(HaveOccurred(), "failed to get cluster")
					g.Expect(cl.Status).NotTo(BeNil(), "cluster status should be present")

					g.Expect(h.HasResourceCondition(cl.Status.Conditions,
						client.ConditionTypeReady, openapi.ResourceConditionStatusFalse)).To(BeTrue(),
						"cluster Ready condition should remain False while crash-adapter is unavailable")
				}, h.Cfg.Polling.Interval*3, h.Cfg.Polling.Interval).Should(Succeed())

				ginkgo.GinkgoWriter.Printf("Verified: crash-adapter absent, cluster Ready=False\n")

				// Step 4: Restore crash-adapter and verify recovery
				ginkgo.By("Restore crash-adapter by scaling up")
				err = h.ScaleDeployment(ctx, h.Cfg.Namespace, deploymentName, 1)
				Expect(err).NotTo(HaveOccurred(), "failed to scale up crash-adapter")

				ginkgo.By("Verify crash-adapter reports correct status after recovery")
				Eventually(func(g Gomega) {
					statuses, err := h.Client.GetClusterStatuses(ctx, clusterID)
					g.Expect(err).NotTo(HaveOccurred(), "failed to get cluster statuses")

					var adapterStatus *openapi.AdapterStatus
					for i, status := range statuses.Items {
						if status.Adapter == adapterName {
							adapterStatus = &statuses.Items[i]
							break
						}
					}
					g.Expect(adapterStatus).NotTo(BeNil(),
						"crash-adapter should be present in statuses after recovery")

					g.Expect(h.HasAdapterCondition(adapterStatus.Conditions,
						client.ConditionTypeApplied, openapi.AdapterConditionStatusTrue)).To(BeTrue(),
						"crash-adapter should have Applied=True after recovery")

					g.Expect(h.HasAdapterCondition(adapterStatus.Conditions,
						client.ConditionTypeAvailable, openapi.AdapterConditionStatusTrue)).To(BeTrue(),
						"crash-adapter should have Available=True after recovery")

					g.Expect(h.HasAdapterCondition(adapterStatus.Conditions,
						client.ConditionTypeHealth, openapi.AdapterConditionStatusTrue)).To(BeTrue(),
						"crash-adapter should have Health=True after recovery")

					g.Expect(adapterStatus.ObservedGeneration).To(Equal(int32(1)),
						"observed_generation should be 1")
				}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

				ginkgo.By("Verify cluster reaches Ready=True after crash-adapter recovery")
				Eventually(func(g Gomega) {
					cl, err := h.Client.GetCluster(ctx, clusterID)
					g.Expect(err).NotTo(HaveOccurred(), "failed to get cluster")
					g.Expect(cl.Status).NotTo(BeNil(), "cluster status should be present")

					g.Expect(h.HasResourceCondition(cl.Status.Conditions,
						client.ConditionTypeReady, openapi.ResourceConditionStatusTrue)).To(BeTrue(),
						"cluster Ready condition should transition to True")

					g.Expect(h.HasResourceCondition(cl.Status.Conditions,
						client.ConditionTypeAvailable, openapi.ResourceConditionStatusTrue)).To(BeTrue(),
						"cluster Available condition should transition to True")
				}, h.Cfg.Timeouts.Cluster.Ready, h.Cfg.Polling.Interval).Should(Succeed())

				ginkgo.GinkgoWriter.Printf("Verified: crash-adapter recovered, cluster Ready=True\n")
			})
	},
)
