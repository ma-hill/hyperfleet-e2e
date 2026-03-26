package adapter

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // Gomega matchers are designed to be used with dot import
	corev1 "k8s.io/api/core/v1"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client/maestro"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: adapter][maestro-transport] Adapter Framework - Maestro Transportation Layer",
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
		})

		ginkgo.Describe("Maestro Transport Happy Path", func() {
			// This test validates the complete Maestro transport happy path:
			// 1. Creating a cluster via HyperFleet API triggers adapter to create ManifestWork
			// 2. ManifestWork is created on Maestro server with correct metadata
			// 3. Maestro agent applies ManifestWork content to target cluster
			// 4. Adapter discovers ManifestWork via statusFeedback and reports status to API
			ginkgo.It("should create ManifestWork and report status via Maestro transport",
				func(ctx context.Context) {
					// Define variables for test adapter yaml
					adapterName := "cl-maestro"
					maestroConsumerName := "cluster1"
					namespaceName := fmt.Sprintf("%s-%s-namespace", clusterID, adapterName)
					configmapName := fmt.Sprintf("%s-%s-configmap", clusterID, adapterName)

					ginkgo.By("Step 1: Verify cluster was created with generation=1")
					cluster, err := h.Client.GetCluster(ctx, clusterID)
					Expect(err).NotTo(HaveOccurred(), "failed to get cluster")
					Expect(cluster.Generation).To(Equal(int32(1)), "cluster should have generation=1")

					var resourceBundle *maestro.ResourceBundle

					ginkgo.By("Step 2: Verify ManifestWork (resource bundle) was created on Maestro")
					// Query Maestro API via HTTP client
					Eventually(func(g Gomega) {
						rb, err := h.GetMaestroClient().FindResourceBundleByClusterID(ctx, clusterID)
						g.Expect(err).NotTo(HaveOccurred(), "failed to find resource bundle for cluster")
						resourceBundle = rb

						// Verify consumer name
						g.Expect(rb.ConsumerName).To(Equal(maestroConsumerName),
							"resource bundle should target correct consumer")

						// Verify version
						g.Expect(rb.Version).To(Equal(1),
							"resource bundle should have version=1")

						// Verify manifest names
						expectedManifests := []string{
							fmt.Sprintf("%s-%s-namespace", clusterID, adapterName),
							fmt.Sprintf("%s-%s-configmap", clusterID, adapterName),
						}
						g.Expect(rb.Manifests).To(HaveLen(2),
							"resource bundle should contain 2 manifests")

						manifestNames := make([]string, len(rb.Manifests))
						for i, m := range rb.Manifests {
							manifestNames[i] = m.Metadata.Name
						}
						g.Expect(manifestNames).To(ConsistOf(expectedManifests),
							"manifest names should match expected pattern")

						ginkgo.GinkgoWriter.Printf("Found resource bundle ID: %s\n", rb.ID)
					}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

					ginkgo.By("Step 3: Verify ManifestWork metadata (labels and annotations)")
					Expect(resourceBundle).NotTo(BeNil(), "resource bundle should be found")

					// Verify labels
					Expect(resourceBundle.Metadata.Labels).To(HaveKey(client.KeyClusterID))
					Expect(resourceBundle.Metadata.Labels[client.KeyClusterID]).To(Equal(clusterID))
					Expect(resourceBundle.Metadata.Labels).To(HaveKey(client.KeyGeneration))
					Expect(resourceBundle.Metadata.Labels[client.KeyGeneration]).To(Equal("1"))
					Expect(resourceBundle.Metadata.Labels).To(HaveKey(client.KeyAdapter))
					Expect(resourceBundle.Metadata.Labels[client.KeyAdapter]).To(Equal(adapterName))

					// Verify annotations
					Expect(resourceBundle.Metadata.Annotations).To(HaveKey(client.KeyGeneration))
					Expect(resourceBundle.Metadata.Annotations[client.KeyGeneration]).To(Equal("1"))
					Expect(resourceBundle.Metadata.Annotations).To(HaveKey(client.KeyManagedBy))
					Expect(resourceBundle.Metadata.Annotations[client.KeyManagedBy]).To(Equal(adapterName))

					ginkgo.By("Step 4: Verify feedbackRules configuration in Maestro resource bundle")
					// Verify manifestConfigs exist
					Expect(resourceBundle.ManifestConfigs).NotTo(BeEmpty(), "manifestConfigs should be present")
					Expect(resourceBundle.ManifestConfigs).To(HaveLen(2), "should have 2 manifest configs")

					// Verify namespace feedbackRules
					namespaceFeedback := maestro.FindManifestConfig(resourceBundle.ManifestConfigs,
						maestro.ResourceIdentifier{
							Name:     namespaceName,
							Resource: "namespaces",
						})
					Expect(namespaceFeedback).NotTo(BeNil(), "namespace manifest config should exist")
					Expect(namespaceFeedback.FeedbackRules).NotTo(BeEmpty())

					// Verify configmap feedbackRules
					configmapFeedback := maestro.FindManifestConfig(resourceBundle.ManifestConfigs,
						maestro.ResourceIdentifier{
							Name:      configmapName,
							Resource:  "configmaps",
							Namespace: namespaceName,
						})
					Expect(configmapFeedback).NotTo(BeNil(), "configmap manifest config should exist")
					Expect(configmapFeedback.FeedbackRules).NotTo(BeEmpty())

					ginkgo.By("Step 5: Verify K8s resources created by Maestro agent on target cluster")
					// Wait for Maestro agent to apply the ManifestWork content

					Eventually(func(g Gomega) {
						// Verify Namespace exists, is Active, and has correct labels/annotations
						ns, err := h.GetNamespace(ctx, namespaceName)
						g.Expect(err).NotTo(HaveOccurred(), "failed to get namespace")

						// Verify phase
						g.Expect(ns.Status.Phase).To(Equal(corev1.NamespaceActive), "namespace should be Active")

						// Verify labels
						g.Expect(ns.Labels).To(HaveKey("app.kubernetes.io/component"))
						g.Expect(ns.Labels["app.kubernetes.io/component"]).To(Equal("adapter-task-config"))
						g.Expect(ns.Labels).To(HaveKey("app.kubernetes.io/instance"))
						g.Expect(ns.Labels["app.kubernetes.io/instance"]).To(Equal(adapterName))
						g.Expect(ns.Labels).To(HaveKey("app.kubernetes.io/name"))
						g.Expect(ns.Labels["app.kubernetes.io/name"]).To(Equal("cl-maestro"))
						g.Expect(ns.Labels).To(HaveKey("app.kubernetes.io/transport"))
						g.Expect(ns.Labels["app.kubernetes.io/transport"]).To(Equal("maestro"))

						// Verify annotations
						g.Expect(ns.Annotations).To(HaveKey(client.KeyGeneration))
						g.Expect(ns.Annotations[client.KeyGeneration]).To(Equal("1"))

						// Verify ConfigMap exists with correct labels, annotations, and data
						cm, err := h.GetConfigMap(ctx, namespaceName, configmapName)
						g.Expect(err).NotTo(HaveOccurred(), "failed to get configmap")

						// Verify labels
						g.Expect(cm.Labels).To(HaveKey("app.kubernetes.io/component"))
						g.Expect(cm.Labels["app.kubernetes.io/component"]).To(Equal("adapter-task-config"))
						g.Expect(cm.Labels).To(HaveKey("app.kubernetes.io/instance"))
						g.Expect(cm.Labels["app.kubernetes.io/instance"]).To(Equal(adapterName))
						g.Expect(cm.Labels).To(HaveKey("app.kubernetes.io/name"))
						g.Expect(cm.Labels["app.kubernetes.io/name"]).To(Equal("cl-maestro"))
						g.Expect(cm.Labels).To(HaveKey("app.kubernetes.io/version"))
						g.Expect(cm.Labels["app.kubernetes.io/version"]).To(Equal("1.0.0"))
						g.Expect(cm.Labels).To(HaveKey("app.kubernetes.io/transport"))
						g.Expect(cm.Labels["app.kubernetes.io/transport"]).To(Equal("maestro"))

						// Verify annotations
						g.Expect(cm.Annotations).To(HaveKey(client.KeyGeneration))
						g.Expect(cm.Annotations[client.KeyGeneration]).To(Equal("1"))

						// Verify data
						g.Expect(cm.Data).To(HaveKey("cluster_id"))
						g.Expect(cm.Data["cluster_id"]).To(Equal(clusterID))
						g.Expect(cm.Data).To(HaveKey("cluster_name"))
						g.Expect(cm.Data["cluster_name"]).To(Equal(clusterName))

						ginkgo.GinkgoWriter.Printf("Verified K8s resources created: namespace=%s, configmap=%s\n",
							namespaceName, configmapName)
					}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

					ginkgo.By("Step 6: Verify adapter status report to HyperFleet API")
					Eventually(func(g Gomega) {
						statuses, err := h.Client.GetClusterStatuses(ctx, clusterID)
						g.Expect(err).NotTo(HaveOccurred(), "failed to get cluster statuses")

						// Find the adapter status
						var adapterStatus *openapi.AdapterStatus
						for _, status := range statuses.Items {
							if status.Adapter == adapterName {
								adapterStatus = &status
								break
							}
						}
						g.Expect(adapterStatus).NotTo(BeNil(),
							"adapter %s should report status", adapterName)

						// Verify observed_generation
						g.Expect(adapterStatus.ObservedGeneration).To(Equal(int32(1)),
							"adapter should have observed_generation=1")

						// Verify observed_time is present
						g.Expect(adapterStatus.LastReportTime).NotTo(BeZero(),
							"adapter should have valid observed_time")

						// Verify conditions
						hasApplied := h.HasAdapterCondition(
							adapterStatus.Conditions,
							client.ConditionTypeApplied,
							openapi.AdapterConditionStatusTrue,
						)
						g.Expect(hasApplied).To(BeTrue(),
							"adapter should have Applied=True")

						// Check Applied condition reason
						appliedCond := h.GetCondition(adapterStatus.Conditions, client.ConditionTypeApplied)
						g.Expect(appliedCond).NotTo(BeNil())
						g.Expect(appliedCond.Reason).NotTo(BeNil())
						g.Expect(*appliedCond.Reason).To(Equal("AppliedManifestWorkComplete"))

						hasAvailable := h.HasAdapterCondition(
							adapterStatus.Conditions,
							client.ConditionTypeAvailable,
							openapi.AdapterConditionStatusTrue,
						)
						g.Expect(hasAvailable).To(BeTrue(),
							"adapter should have Available=True")

						// Check Available condition reason
						availableCond := h.GetCondition(adapterStatus.Conditions, client.ConditionTypeAvailable)
						g.Expect(availableCond).NotTo(BeNil())
						g.Expect(availableCond.Reason).NotTo(BeNil())
						g.Expect(*availableCond.Reason).To(Equal("AllResourcesAvailable"))

						hasHealth := h.HasAdapterCondition(
							adapterStatus.Conditions,
							client.ConditionTypeHealth,
							openapi.AdapterConditionStatusTrue,
						)
						g.Expect(hasHealth).To(BeTrue(),
							"adapter should have Health=True")

						// Check Health condition reason
						healthCond := h.GetCondition(adapterStatus.Conditions, client.ConditionTypeHealth)
						g.Expect(healthCond).NotTo(BeNil())
						g.Expect(healthCond.Reason).NotTo(BeNil())
						g.Expect(*healthCond.Reason).To(Equal("Healthy"))

						// Verify data fields
						g.Expect(adapterStatus.Data).NotTo(BeNil(), "adapter data should be present")
						if adapterStatus.Data == nil {
							return // let Eventually retry with clean failure message
						}

						// Verify manifestwork data
						manifestworkData, ok := (*adapterStatus.Data)["manifestwork"].(map[string]interface{})
						g.Expect(ok).To(BeTrue(), "manifestwork data should be present")
						g.Expect(manifestworkData["name"]).To(Equal(fmt.Sprintf("%s-%s", clusterID, adapterName)))

						// Verify namespace data
						namespaceData, ok := (*adapterStatus.Data)["namespace"].(map[string]interface{})
						g.Expect(ok).To(BeTrue(), "namespace data should be present")
						g.Expect(namespaceData["phase"]).To(Equal("Active"))
						g.Expect(namespaceData["name"]).To(Equal(namespaceName))

						// Verify configmap data
						configmapData, ok := (*adapterStatus.Data)["configmap"].(map[string]interface{})
						g.Expect(ok).To(BeTrue(), "configmap data should be present")
						g.Expect(configmapData["clusterId"]).To(Equal(clusterID))
						g.Expect(configmapData["name"]).To(Equal(configmapName))

						if appliedCond != nil && appliedCond.Reason != nil &&
							availableCond != nil && availableCond.Reason != nil &&
							healthCond != nil && healthCond.Reason != nil {
							ginkgo.GinkgoWriter.Printf("Verified adapter status report: Applied=%s, Available=%s, Health=%s\n",
								*appliedCond.Reason, *availableCond.Reason, *healthCond.Reason)
						}
					}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())
				})
		})

		ginkgo.Describe("Maestro Generation-based Idempotency", func() {
			// This test validates the generation-based idempotency mechanism:
			// 1. When a ManifestWork does not exist, it should be created
			// 2. When the same event is reprocessed with the same generation, the operation should be skipped
			// 3. The Maestro resource version should remain unchanged across multiple Skip operations
			ginkgo.It("should skip ManifestWork operation when generation is unchanged",
				func(ctx context.Context) {
					adapterName := "cl-maestro"

					ginkgo.By("Step 1: Verify cluster was created with generation=1")
					cluster, err := h.Client.GetCluster(ctx, clusterID)
					Expect(err).NotTo(HaveOccurred(), "failed to get cluster")
					Expect(cluster.Generation).To(Equal(int32(1)), "cluster should have generation=1")

					var resourceBundle *maestro.ResourceBundle

					ginkgo.By("Step 2: Wait for initial ManifestWork creation and capture resource bundle ID")
					// Query Maestro API to find the resource bundle
					Eventually(func(g Gomega) {
						rb, err := h.GetMaestroClient().FindResourceBundleByClusterID(ctx, clusterID)
						g.Expect(err).NotTo(HaveOccurred(), "failed to find resource bundle for cluster")
						resourceBundle = rb

						// Verify version is 1 (initial creation)
						g.Expect(rb.Version).To(Equal(1),
							"resource bundle should have version=1 after initial creation")

						ginkgo.GinkgoWriter.Printf("Found resource bundle ID: %s with version: %d\n", rb.ID, rb.Version)
					}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

					Expect(resourceBundle).NotTo(BeNil(), "resource bundle should be found")
					initialVersion := resourceBundle.Version

					ginkgo.By("Step 3: Capture initial adapter status timestamp before skip period")
					// Capture the initial lastReportTime to verify adapter continues processing
					var initialReportTime time.Time
					Eventually(func(g Gomega) {
						statuses, err := h.Client.GetClusterStatuses(ctx, clusterID)
						g.Expect(err).NotTo(HaveOccurred(), "failed to get cluster statuses")

						var adapterStatus *openapi.AdapterStatus
						for _, status := range statuses.Items {
							if status.Adapter == adapterName {
								adapterStatus = &status
								break
							}
						}
						g.Expect(adapterStatus).NotTo(BeNil(), "adapter status should exist")
						g.Expect(adapterStatus.LastReportTime).NotTo(BeZero(), "lastReportTime should be set")

						initialReportTime = adapterStatus.LastReportTime
						ginkgo.GinkgoWriter.Printf("Captured initial lastReportTime: %v\n", initialReportTime)
					}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

					ginkgo.By("Step 4: Verify adapter continued processing during skip period")
					// We wait up to 3-4 polling cycles to ensure multiple processing cycles occur.
					Eventually(func(g Gomega) {
						statuses, err := h.Client.GetClusterStatuses(ctx, clusterID)
						g.Expect(err).NotTo(HaveOccurred(), "failed to get cluster statuses")

						var adapterStatus *openapi.AdapterStatus
						for _, status := range statuses.Items {
							if status.Adapter == adapterName {
								adapterStatus = &status
								break
							}
						}
						g.Expect(adapterStatus).NotTo(BeNil(), "adapter status should exist")

						// Verify lastReportTime was updated (adapter is still processing)
						g.Expect(adapterStatus.LastReportTime.After(initialReportTime)).To(BeTrue(),
							"adapter should have updated lastReportTime, indicating it processed events during skip period")

						// Verify observedGeneration is still 1 (adapter is processing the same generation)
						g.Expect(adapterStatus.ObservedGeneration).To(Equal(int32(1)),
							"adapter should still observe generation 1")

						ginkgo.GinkgoWriter.Printf("Verified adapter processed events: lastReportTime updated from %v to %v\n",
							initialReportTime, adapterStatus.LastReportTime)
					}, 3*h.Cfg.Polling.Interval, h.Cfg.Polling.Interval).Should(Succeed())

					ginkgo.By("Step 5: Verify Maestro resource version does not change on Skip")
					// Query the resource bundle again to verify version remains unchanged
					Eventually(func(g Gomega) {
						rb, err := h.GetMaestroClient().FindResourceBundleByClusterID(ctx, clusterID)
						g.Expect(err).NotTo(HaveOccurred(), "failed to find resource bundle")

						// Version should remain at initial version (1)
						g.Expect(rb.Version).To(Equal(initialVersion),
							"resource bundle version should remain unchanged across Skip operations")

						ginkgo.GinkgoWriter.Printf("Verified resource bundle version remains at: %d\n", rb.Version)
					}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())
				})
		})

		ginkgo.AfterEach(func(ctx context.Context) {
			// Skip cleanup if helper not initialized
			if h == nil {
				return
			}

			// Clean up cluster and all associated resources
			if clusterID != "" {
				ginkgo.By("Cleanup test cluster " + clusterID)
				if err := h.CleanupTestCluster(ctx, clusterID); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup cluster %s: %v\n", clusterID, err)
				}
			}
		})
	},
)

var _ = ginkgo.Describe("[Suite: adapter][maestro-transport][negative] Adapter Framework - Maestro Transport Negative Scenarios",
	ginkgo.Label(labels.Tier1),
	func() {
		var (
			h              *helper.Helper
			clusterID      string
			adapterRelease string // Track deployed adapter release name for cleanup
			chartPath      string
			baseDeployOpts helper.AdapterDeploymentOptions
		)

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			// Clone adapter Helm chart repository (shared across negative tests)
			ginkgo.By("Clone adapter Helm chart repository for negative tests")
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

		ginkgo.AfterEach(func(ctx context.Context) {
			// Clean up in reverse order: adapter first, then cluster
			// This ensures adapter is uninstalled before cluster cleanup
			if adapterRelease != "" {
				ginkgo.By("Uninstall adapter " + adapterRelease)
				if err := h.UninstallAdapter(ctx, adapterRelease, h.Cfg.Namespace); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: failed to uninstall adapter %s: %v\n", adapterRelease, err)
				} else {
					ginkgo.GinkgoWriter.Printf("Successfully uninstalled adapter: %s\n", adapterRelease)
				}
			}

			if clusterID != "" {
				ginkgo.By("Cleanup test cluster " + clusterID)
				if err := h.CleanupTestCluster(ctx, clusterID); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup cluster %s: %v\n", clusterID, err)
				}
			}
		})

		ginkgo.It("should fail when targeting unregistered Maestro consumer and report appropriate error",
			func(ctx context.Context) {
				// Test-specific adapter configuration
				adapterName := "cl-m-unreg-consumer"
				err := os.Setenv("ADAPTER_NAME", adapterName)
				Expect(err).NotTo(HaveOccurred(), "failed to set ADAPTER_NAME environment variable")
				ginkgo.DeferCleanup(func() {
					_ = os.Unsetenv("ADAPTER_NAME")
				})
				// Generate unique release name for this deployment
				releaseName := helper.GenerateAdapterReleaseName(helper.ResourceTypeClusters, adapterName)

				// Deploy the test adapter configured to target unregistered consumer
				ginkgo.By("Deploy test adapter with unregistered consumer configuration")

				// Create deployment options from base and add test-specific fields
				deployOpts := baseDeployOpts
				deployOpts.ReleaseName = releaseName
				deployOpts.AdapterName = adapterName

				err = h.DeployAdapter(ctx, deployOpts)
				Expect(err).NotTo(HaveOccurred(), "failed to deploy test adapter")
				adapterRelease = releaseName
				ginkgo.GinkgoWriter.Printf("Successfully deployed adapter: %s (release: %s)\n", adapterName, releaseName)

				// Create cluster after adapter is deployed
				ginkgo.By("Create test cluster")
				cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
				Expect(err).NotTo(HaveOccurred(), "failed to create cluster")
				Expect(cluster.Id).NotTo(BeNil(), "cluster ID should be generated")
				Expect(cluster.Name).NotTo(BeEmpty(), "cluster name should be present")
				clusterID = *cluster.Id
				ginkgo.GinkgoWriter.Printf("Created cluster ID: %s, Name: %s\n", clusterID, cluster.Name)

				ginkgo.By("Verify adapter reports failure for unregistered consumer")
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
					g.Expect(adapterStatus.ObservedGeneration).To(Equal(int32(1)),
						"adapter should have observed_generation=1")

					// Find Health condition
					var healthCondition *openapi.AdapterCondition
					for i, condition := range adapterStatus.Conditions {
						if condition.Type == client.ConditionTypeHealth {
							healthCondition = &adapterStatus.Conditions[i]
							break
						}
					}

					g.Expect(healthCondition).NotTo(BeNil(),
						"adapter should have Health condition")

					// Verify Health condition reports failure
					g.Expect(healthCondition.Status).To(Equal(openapi.AdapterConditionStatusFalse),
						"adapter Health condition should be False due to unregistered consumer")

					// Verify error details mention consumer not found/registered
					g.Expect(healthCondition.Message).NotTo(BeNil(),
						"adapter Health condition should have message")
					message := *healthCondition.Message
					g.Expect(message).To(SatisfyAny(
						ContainSubstring("unregistered-consumer"),
						ContainSubstring("not found"),
						ContainSubstring("not registered"),
					), "error message should mention unregistered consumer")

					// Find Applied condition - should be False
					var appliedCondition *openapi.AdapterCondition
					for i, condition := range adapterStatus.Conditions {
						if condition.Type == client.ConditionTypeApplied {
							appliedCondition = &adapterStatus.Conditions[i]
							break
						}
					}

					g.Expect(appliedCondition).NotTo(BeNil(),
						"adapter should have Applied condition")
					g.Expect(appliedCondition.Status).To(Equal(openapi.AdapterConditionStatusFalse),
						"adapter Applied condition should be False since ManifestWork was not created")

					ginkgo.GinkgoWriter.Printf("Verified adapter failure for unregistered consumer: Health=%s, Applied=%s\n",
						healthCondition.Status, appliedCondition.Status)
				}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

				ginkgo.By("Verify no ManifestWork was created by the test adapter on Maestro")
				Eventually(func(g Gomega) {
					// Query by cluster ID first to scope to current cluster
					rbs, err := h.GetMaestroClient().FindAllResourceBundlesByClusterID(ctx, clusterID)
					g.Expect(err).NotTo(HaveOccurred(), "should be able to query Maestro for resource bundles")

					// Filter by adapter name using source-id label
					var adapterBundles []maestro.ResourceBundle
					for _, rb := range rbs {
						if rb.Metadata.Labels != nil && rb.Metadata.Labels["maestro.io/source-id"] == adapterName {
							adapterBundles = append(adapterBundles, rb)
						}
					}

					g.Expect(adapterBundles).To(BeEmpty(),
						"no ManifestWork should be created by adapter %s for cluster %s with unregistered consumer", adapterName, clusterID)
					ginkgo.GinkgoWriter.Printf("Verified no ManifestWork exists from adapter %s for cluster %s\n",
						adapterName, clusterID)
				}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

				ginkgo.By("Verify no K8s resources were created by the test adapter")
				Eventually(func(g Gomega) {
					// Check specifically for namespace that would have been created by THIS adapter
					// Expected namespace name pattern: ${clusterID}-${adapterName}-namespace
					expectedNamespace := fmt.Sprintf("%s-%s-namespace", clusterID, adapterName)
					_, err := h.GetNamespace(ctx, expectedNamespace)

					// We expect the namespace to NOT exist (should get error)
					g.Expect(err).To(HaveOccurred(),
						"namespace %s should not exist when adapter %s fails to create ManifestWork",
						expectedNamespace, adapterName)

					ginkgo.GinkgoWriter.Printf("Verified namespace %s does not exist (adapter %s did not create resources)\n",
						expectedNamespace, adapterName)
				}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

				ginkgo.GinkgoWriter.Printf("Successfully validated adapter failure for unregistered consumer\n")
			})

		ginkgo.It("should fail to discover ManifestWork when discovery name does not match created resource",
			func(ctx context.Context) {
				// Test-specific adapter configuration
				adapterName := "cl-m-wrong-ds"
				// Set environment variable for envsubst expansion in values.yaml
				err := os.Setenv("ADAPTER_NAME", adapterName)
				Expect(err).NotTo(HaveOccurred(), "failed to set ADAPTER_NAME environment variable")
				ginkgo.DeferCleanup(func() {
					_ = os.Unsetenv("ADAPTER_NAME")
				})
				// Generate unique release name for this deployment
				releaseName := helper.GenerateAdapterReleaseName(helper.ResourceTypeClusters, adapterName)
				// Deploy the test adapter with wrong main discovery configuration
				ginkgo.By("Deploy test adapter with wrong ManifestWork discovery name")

				// Create deployment options from base and add test-specific fields
				deployOpts := baseDeployOpts
				deployOpts.ReleaseName = releaseName
				deployOpts.AdapterName = adapterName

				err = h.DeployAdapter(ctx, deployOpts)
				Expect(err).NotTo(HaveOccurred(), "failed to deploy test adapter")
				adapterRelease = releaseName
				ginkgo.GinkgoWriter.Printf("Successfully deployed adapter: %s (release: %s)\n", adapterName, releaseName)

				// Create cluster after adapter is deployed
				ginkgo.By("Create test cluster")
				cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
				Expect(err).NotTo(HaveOccurred(), "failed to create cluster")
				Expect(cluster.Id).NotTo(BeNil(), "cluster ID should be generated")
				Expect(cluster.Name).NotTo(BeEmpty(), "cluster name should be present")
				clusterID = *cluster.Id
				ginkgo.GinkgoWriter.Printf("Created cluster ID: %s, Name: %s\n", clusterID, cluster.Name)

				// Verify ManifestWork was created by the test adapter despite wrong discovery config
				ginkgo.By("Verify ManifestWork was created by the test adapter on Maestro")
				Eventually(func(g Gomega) {
					// Query by cluster ID first to scope to current cluster
					rbs, err := h.GetMaestroClient().FindAllResourceBundlesByClusterID(ctx, clusterID)
					g.Expect(err).NotTo(HaveOccurred(), "should be able to query Maestro for resource bundles")

					// Filter by adapter name using source-id label
					var adapterBundles []maestro.ResourceBundle
					for _, rb := range rbs {
						if rb.Metadata.Labels != nil && rb.Metadata.Labels["maestro.io/source-id"] == adapterName {
							adapterBundles = append(adapterBundles, rb)
						}
					}

					g.Expect(adapterBundles).NotTo(BeEmpty(), "ManifestWork should be created by adapter %s for cluster %s despite wrong discovery", adapterName, clusterID)
					ginkgo.GinkgoWriter.Printf("Found resource bundle created by adapter %s for cluster %s: ID=%s\n", adapterName, clusterID, adapterBundles[0].ID)
				}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

				// Verify K8s resources were created by Maestro agent
				ginkgo.By("Verify K8s resources were created by Maestro agent")
				namespaceName := fmt.Sprintf("%s-%s-namespace", clusterID, adapterName)
				configmapName := fmt.Sprintf("%s-%s-configmap", clusterID, adapterName)

				Eventually(func(g Gomega) {
					// Verify namespace exists
					_, err := h.GetNamespace(ctx, namespaceName)
					g.Expect(err).NotTo(HaveOccurred(), "namespace should be created by Maestro agent")

					// Verify configmap exists in the namespace
					_, err = h.GetConfigMap(ctx, namespaceName, configmapName)
					g.Expect(err).NotTo(HaveOccurred(), "configmap should be created by Maestro agent")

					ginkgo.GinkgoWriter.Printf("Verified resources exist: namespace=%s, configmap=%s\n", namespaceName, configmapName)
				}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

				ginkgo.By("Verify adapter reports discovery failure with appropriate error")
				// Wait for adapter to process the cluster and report discovery failure
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
					g.Expect(adapterStatus.ObservedGeneration).To(Equal(int32(1)),
						"adapter should have observed_generation=1")

					// Find Applied condition - should be False (ManifestWork not discovered)
					var appliedCondition *openapi.AdapterCondition
					for i, condition := range adapterStatus.Conditions {
						if condition.Type == client.ConditionTypeApplied {
							appliedCondition = &adapterStatus.Conditions[i]
							break
						}
					}

					g.Expect(appliedCondition).NotTo(BeNil(),
						"adapter should have Applied condition")
					g.Expect(appliedCondition.Status).To(Equal(openapi.AdapterConditionStatusFalse),
						"Applied should be False - ManifestWork not discovered")
					g.Expect(appliedCondition.Reason).NotTo(BeNil())
					g.Expect(*appliedCondition.Reason).To(Equal("ManifestWorkNotDiscovered"),
						"Applied reason should be ManifestWorkNotDiscovered")

					// Find Available condition - should be False
					var availableCondition *openapi.AdapterCondition
					for i, condition := range adapterStatus.Conditions {
						if condition.Type == client.ConditionTypeAvailable {
							availableCondition = &adapterStatus.Conditions[i]
							break
						}
					}

					g.Expect(availableCondition).NotTo(BeNil(),
						"adapter should have Available condition")
					g.Expect(availableCondition.Status).To(Equal(openapi.AdapterConditionStatusFalse),
						"Available should be False")
					g.Expect(availableCondition.Reason).NotTo(BeNil())
					g.Expect(*availableCondition.Reason).To(Equal("NamespaceNotDiscovered"),
						"Available reason should be NamespaceNotDiscovered")

					// Find Health condition - should be False (execution failed)
					var healthCondition *openapi.AdapterCondition
					for i, condition := range adapterStatus.Conditions {
						if condition.Type == client.ConditionTypeHealth {
							healthCondition = &adapterStatus.Conditions[i]
							break
						}
					}

					g.Expect(healthCondition).NotTo(BeNil(),
						"adapter should have Health condition")
					g.Expect(healthCondition.Status).To(Equal(openapi.AdapterConditionStatusFalse),
						"Health should be False - discovery failed")
					g.Expect(healthCondition.Reason).NotTo(BeNil())
					g.Expect(*healthCondition.Reason).To(Equal("ExecutionFailed:ResourceFailed"),
						"Health reason should be ExecutionFailed:ResourceFailed")
					g.Expect(healthCondition.Message).NotTo(BeNil())
					g.Expect(*healthCondition.Message).To(ContainSubstring("not found"),
						"Health message should mention ManifestWork not found")

					// Verify data section - all fields empty (main discovery failed)
					g.Expect(adapterStatus.Data).NotTo(BeNil(), "adapter status should have data")

					// ManifestWork name should be empty (main discovery failed)
					if manifestworkData, ok := (*adapterStatus.Data)["manifestwork"].(map[string]interface{}); ok {
						if nameVal, exists := manifestworkData["name"]; exists {
							g.Expect(nameVal).To(Or(BeNil(), Equal("")),
								"manifestwork name should be empty - main discovery failed")
						}
					}

					// Namespace name should be empty
					if namespaceData, ok := (*adapterStatus.Data)["namespace"].(map[string]interface{}); ok {
						if nameVal, exists := namespaceData["name"]; exists {
							g.Expect(nameVal).To(Or(BeNil(), Equal("")),
								"namespace name should be empty")
						}
					}

					// ConfigMap name should be empty
					if configmapData, ok := (*adapterStatus.Data)["configmap"].(map[string]interface{}); ok {
						if nameVal, exists := configmapData["name"]; exists {
							g.Expect(nameVal).To(Or(BeNil(), Equal("")),
								"configmap name should be empty")
						}
					}

					ginkgo.GinkgoWriter.Printf("Verified main discovery failure: Applied=%s, Health=%s, Available=%s\n",
						appliedCondition.Status, healthCondition.Status, availableCondition.Status)
				}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

				ginkgo.GinkgoWriter.Printf("Successfully validated adapter discovery failure reporting\n")
			})

		ginkgo.It("should fail nested discovery when resource names are wrong",
			func(ctx context.Context) {
				// Test-specific adapter configuration
				adapterName := "cl-m-wrong-nest"
				// Set environment variable for envsubst expansion in values.yaml
				err := os.Setenv("ADAPTER_NAME", adapterName)
				Expect(err).NotTo(HaveOccurred(), "failed to set ADAPTER_NAME environment variable")
				ginkgo.DeferCleanup(func() {
					_ = os.Unsetenv("ADAPTER_NAME")
				})

				// Generate unique release name for this deployment
				releaseName := helper.GenerateAdapterReleaseName(helper.ResourceTypeClusters, adapterName)

				// Deploy the test adapter with empty discovery configuration
				ginkgo.By("Deploy test adapter with empty nested discovery configuration")

				// Create deployment options from base and add test-specific fields
				deployOpts := baseDeployOpts
				deployOpts.ReleaseName = releaseName
				deployOpts.AdapterName = adapterName

				err = h.DeployAdapter(ctx, deployOpts)
				Expect(err).NotTo(HaveOccurred(), "failed to deploy test adapter")
				adapterRelease = releaseName
				ginkgo.GinkgoWriter.Printf("Successfully deployed adapter: %s (release: %s)\n", adapterName, releaseName)

				// Create cluster after adapter is deployed
				ginkgo.By("Create test cluster")
				cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
				Expect(err).NotTo(HaveOccurred(), "failed to create cluster")
				Expect(cluster.Id).NotTo(BeNil(), "cluster ID should be generated")
				Expect(cluster.Name).NotTo(BeEmpty(), "cluster name should be present")
				clusterID = *cluster.Id
				ginkgo.GinkgoWriter.Printf("Created cluster ID: %s, Name: %s\n", clusterID, cluster.Name)

				// Construct namespace name AFTER cluster is created
				namespaceName := fmt.Sprintf("%s-%s-namespace", clusterID, adapterName)

				// Verify ManifestWork was created by the test adapter
				ginkgo.By("Verify ManifestWork was created by the test adapter on Maestro")
				Eventually(func(g Gomega) {
					// Query by cluster ID first to scope to current cluster
					rbs, err := h.GetMaestroClient().FindAllResourceBundlesByClusterID(ctx, clusterID)
					g.Expect(err).NotTo(HaveOccurred(), "should be able to query Maestro for resource bundles")

					// Filter by adapter name using source-id label
					var adapterBundles []maestro.ResourceBundle
					for _, rb := range rbs {
						if rb.Metadata.Labels != nil && rb.Metadata.Labels["maestro.io/source-id"] == adapterName {
							adapterBundles = append(adapterBundles, rb)
						}
					}

					g.Expect(adapterBundles).NotTo(BeEmpty(), "ManifestWork should be created by adapter %s for cluster %s", adapterName, clusterID)
					ginkgo.GinkgoWriter.Printf("Found resource bundle created by adapter %s for cluster %s: ID=%s\n", adapterName, clusterID, adapterBundles[0].ID)
				}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

				// Verify K8s resources were created
				ginkgo.By("Verify K8s resources were created by Maestro agent")
				Eventually(func(g Gomega) {
					_, err := h.GetNamespace(ctx, namespaceName)
					g.Expect(err).NotTo(HaveOccurred(), "namespace should be created")
				}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

				ginkgo.By("Verify adapter handles empty discovery with fallback values")
				// Wait for adapter to process the cluster and report status with fallback values
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
					g.Expect(adapterStatus.ObservedGeneration).To(Equal(int32(1)),
						"adapter should have observed_generation=1")

					// Find Applied condition - should be True (ManifestWork created successfully)
					var appliedCondition *openapi.AdapterCondition
					for i, condition := range adapterStatus.Conditions {
						if condition.Type == client.ConditionTypeApplied {
							appliedCondition = &adapterStatus.Conditions[i]
							break
						}
					}

					g.Expect(appliedCondition).NotTo(BeNil(),
						"adapter should have Applied condition")
					g.Expect(appliedCondition.Status).To(Equal(openapi.AdapterConditionStatusTrue),
						"adapter Applied condition should be True since ManifestWork was created")

					// Find Available condition - should be False (nested resources not discovered)
					var availableCondition *openapi.AdapterCondition
					for i, condition := range adapterStatus.Conditions {
						if condition.Type == client.ConditionTypeAvailable {
							availableCondition = &adapterStatus.Conditions[i]
							break
						}
					}

					g.Expect(availableCondition).NotTo(BeNil(),
						"adapter should have Available condition")
					g.Expect(availableCondition.Status).To(Equal(openapi.AdapterConditionStatusFalse),
						"adapter Available condition should be False since nested discovery returned empty")

					// Find Health condition - should be True (adapter executed successfully)
					// Note: Nested discovery failure doesn't affect Health - the adapter ran successfully
					var healthCondition *openapi.AdapterCondition
					for i, condition := range adapterStatus.Conditions {
						if condition.Type == client.ConditionTypeHealth {
							healthCondition = &adapterStatus.Conditions[i]
							break
						}
					}

					g.Expect(healthCondition).NotTo(BeNil(),
						"adapter should have Health condition")
					g.Expect(healthCondition.Status).To(Equal(openapi.AdapterConditionStatusTrue),
						"adapter Health condition should be True - nested discovery failure doesn't affect health")
					g.Expect(healthCondition.Reason).NotTo(BeNil(),
						"Health condition should have a reason")
					g.Expect(*healthCondition.Reason).To(Equal("Healthy"),
						"Health reason should be Healthy")
					g.Expect(healthCondition.Message).NotTo(BeNil(),
						"Health condition should have a message")
					g.Expect(*healthCondition.Message).To(ContainSubstring("completed successfully"),
						"Health message should indicate successful execution")

					// Verify data field shows fallback/empty values for nested resources
					g.Expect(adapterStatus.Data).NotTo(BeNil(),
						"adapter status should have data field")

					// Check that namespace data is either empty or has fallback values
					if namespaceData, ok := (*adapterStatus.Data)["namespace"].(map[string]interface{}); ok {
						// Namespace name should be empty string or default value
						if name, exists := namespaceData["name"]; exists {
							g.Expect(name).To(Or(BeEmpty(), Equal("")),
								"namespace name should be empty due to empty discovery")
						}
					}

					ginkgo.GinkgoWriter.Printf("Verified empty discovery handling: Applied=%s, Available=%s, Health=%s\n",
						appliedCondition.Status, availableCondition.Status, healthCondition.Status)
				}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

				ginkgo.GinkgoWriter.Printf("Successfully validated adapter handling of empty nested discovery\n")
			})

		ginkgo.It("should fail post-action when status API is unreachable",
			func(ctx context.Context) {
				// Use cl-m-bad-api adapter with overridden API URL
				adapterName := "cl-m-bad-api"
				// Set environment variable for envsubst expansion in values.yaml
				err := os.Setenv("ADAPTER_NAME", adapterName)
				Expect(err).NotTo(HaveOccurred(), "failed to set ADAPTER_NAME environment variable")
				ginkgo.DeferCleanup(func() {
					_ = os.Unsetenv("ADAPTER_NAME")
				})

				// Generate unique release name for this deployment
				releaseName := helper.GenerateAdapterReleaseName(helper.ResourceTypeClusters, adapterName)

				// Deploy the test adapter with invalid API URL
				ginkgo.By("Deploy test adapter with unreachable API URL configuration")

				// Create deployment options with overridden API URL
				deployOpts := baseDeployOpts
				deployOpts.ReleaseName = releaseName
				deployOpts.AdapterName = adapterName
				// Override hyperfleetApi.baseUrl to make it unreachable
				deployOpts.SetValues = map[string]string{
					"adapterConfig.hyperfleetApi.baseUrl": "http://invalid-hyperfleet-api-endpoint.local:9999",
				}

				err = h.DeployAdapter(ctx, deployOpts)
				Expect(err).NotTo(HaveOccurred(), "failed to deploy test adapter")
				adapterRelease = releaseName
				ginkgo.GinkgoWriter.Printf("Successfully deployed adapter: %s (release: %s)\n", adapterName, releaseName)

				// Create cluster after adapter is deployed
				ginkgo.By("Create test cluster")
				cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
				Expect(err).NotTo(HaveOccurred(), "failed to create cluster")
				Expect(cluster.Id).NotTo(BeNil(), "cluster ID should be generated")
				Expect(cluster.Name).NotTo(BeEmpty(), "cluster name should be present")
				clusterID = *cluster.Id
				ginkgo.GinkgoWriter.Printf("Created cluster ID: %s, Name: %s\n", clusterID, cluster.Name)

				// Construct namespace name AFTER cluster is created
				namespaceName := fmt.Sprintf("%s-%s-namespace", clusterID, adapterName)

				ginkgo.By("Verify ManifestWork was applied successfully by the test adapter in Maestro")
				// Even though post-action failed, the ManifestWork should exist in Maestro
				Eventually(func(g Gomega) {
					// Query by cluster ID first to scope to current cluster
					rbs, err := h.GetMaestroClient().FindAllResourceBundlesByClusterID(ctx, clusterID)
					g.Expect(err).NotTo(HaveOccurred(), "should be able to query Maestro for resource bundles")

					// Filter by adapter name using source-id label
					var adapterBundles []maestro.ResourceBundle
					for _, rb := range rbs {
						if rb.Metadata.Labels != nil && rb.Metadata.Labels["maestro.io/source-id"] == adapterName {
							adapterBundles = append(adapterBundles, rb)
						}
					}

					g.Expect(adapterBundles).NotTo(BeEmpty(), "ManifestWork should exist in Maestro created by adapter %s for cluster %s", adapterName, clusterID)
					g.Expect(adapterBundles[0].ID).NotTo(BeEmpty(), "ManifestWork should have an ID")
					g.Expect(adapterBundles[0].ConsumerName).NotTo(BeEmpty(), "ManifestWork should have a consumer")
					ginkgo.GinkgoWriter.Printf("ManifestWork verified in Maestro for adapter %s and cluster %s: ID=%s, Consumer=%s\n",
						adapterName, clusterID, adapterBundles[0].ID, adapterBundles[0].ConsumerName)
				}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

				// Verify K8s resources were created
				ginkgo.By("Verify K8s resources were created by Maestro agent despite API being unreachable")
				Eventually(func(g Gomega) {
					_, err := h.GetNamespace(ctx, namespaceName)
					g.Expect(err).NotTo(HaveOccurred(), "namespace should be created")
				}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

				ginkgo.By("Verify adapter cannot report status when API is unreachable")
				// Since the adapter has invalid API URL, it should NOT successfully report status
				// The status should either not exist or remain empty
				Consistently(func(g Gomega) {
					statuses, err := h.Client.GetClusterStatuses(ctx, clusterID)
					g.Expect(err).NotTo(HaveOccurred(), "failed to get cluster statuses")

					// Find the test adapter status
					var adapterStatus *openapi.AdapterStatus
					for _, adapter := range statuses.Items {
						if adapter.Adapter == adapterName {
							adapterStatus = &adapter
							break
						}
					}

					// Adapter should NOT be able to post status
					g.Expect(adapterStatus).To(BeNil(), "adapter status should not exist when API is unreachable")
					ginkgo.GinkgoWriter.Printf("Verified: no status found for adapter %s (expected)\n", adapterName)
				}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())

				ginkgo.GinkgoWriter.Printf("Successfully validated adapter cannot report status with unreachable API\n")
			})
	},
)
