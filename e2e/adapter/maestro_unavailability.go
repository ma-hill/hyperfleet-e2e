package adapter

import (
	"context"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // Gomega matchers are designed to be used with dot import

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/api/openapi"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client/maestro"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: adapter][maestro-transport][negative] Adapter can handle Maestro server unavailability gracefully",
	ginkgo.Serial,
	ginkgo.Label(labels.Tier2, labels.Negative, labels.Disruptive),
	func() {
		var h *helper.Helper

		ginkgo.BeforeEach(func() {
			h = helper.New()
		})

		ginkgo.It("should report failure status when Maestro is unavailable and recover when restored",
			func(ctx context.Context) {
				adapterName := "cl-maestro"

				ginkgo.By("Scale down Maestro deployment to simulate unavailability")
				err := h.ScaleDeployment(ctx, maestro.MaestroNamespace, maestro.MaestroServiceName, 0)
				Expect(err).NotTo(HaveOccurred(), "failed to scale down Maestro")

				// Registered first → runs last (LIFO). Hard assertion: Maestro must be
				// restored. Separate Ginkgo node so failure here cannot skip cluster cleanup.
				ginkgo.DeferCleanup(func(ctx context.Context) {
					ginkgo.By("Restore Maestro deployment")
					Expect(h.ScaleDeployment(ctx, maestro.MaestroNamespace, maestro.MaestroServiceName, 1)).
						To(Succeed(), "cleanup must restore Maestro deployment")
				})

				ginkgo.By("Create cluster while Maestro is unavailable")
				cluster, err := h.Client.CreateClusterFromPayload(ctx, h.TestDataPath("payloads/clusters/cluster-request.json"))
				Expect(err).NotTo(HaveOccurred(), "failed to create cluster")
				Expect(cluster.Id).NotTo(BeNil(), "cluster ID should be generated")
				clusterID := *cluster.Id
				ginkgo.GinkgoWriter.Printf("Created cluster ID: %s, Name: %s\n", clusterID, cluster.Name)

				// Registered second → runs first (LIFO). Soft failure: warn-log only.
				ginkgo.DeferCleanup(func(ctx context.Context) {
					ginkgo.By("Cleanup test cluster " + clusterID)
					if err := h.CleanupTestCluster(ctx, clusterID); err != nil {
						ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup cluster %s: %v\n", clusterID, err)
					}
				})

				ginkgo.By("Verify Maestro adapter reports failure status")
				verifyMaestroAdapterFailure(ctx, h, clusterID, adapterName)

				ginkgo.By("Verify cluster does not reach Reconciled while Maestro is unavailable")
				verifyClusterNotReconciledDuringOutage(ctx, h, clusterID)

				ginkgo.By("Restore Maestro deployment")
				if err := h.ScaleDeployment(ctx, maestro.MaestroNamespace, maestro.MaestroServiceName, 1); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: ScaleDeployment readiness wait timed out (scale command issued, Maestro may still be starting): %v\n", err)
				}

				ginkgo.By("Verify Maestro adapter recovers with all conditions True")
				verifyMaestroAdapterRecovery(ctx, h, clusterID, adapterName)

				ginkgo.By("Verify cluster reaches Reconciled=True after Maestro recovery")
				verifyClusterReconciledAfterRecovery(ctx, h, clusterID)
			})
	},
)

// verifyMaestroAdapterFailure verifies the Maestro adapter reports Applied=False, Available=False, Health=False.
func verifyMaestroAdapterFailure(ctx context.Context, h *helper.Helper, clusterID, adapterName string) {
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
			"Maestro adapter should be present in statuses")

		g.Expect(h.HasAdapterCondition(adapterStatus.Conditions,
			client.ConditionTypeApplied, openapi.AdapterConditionStatusFalse)).To(BeTrue(),
			"Maestro adapter Applied should be False")

		g.Expect(h.HasAdapterCondition(adapterStatus.Conditions,
			client.ConditionTypeAvailable, openapi.AdapterConditionStatusFalse)).To(BeTrue(),
			"Maestro adapter Available should be False")

		g.Expect(h.HasAdapterCondition(adapterStatus.Conditions,
			client.ConditionTypeHealth, openapi.AdapterConditionStatusFalse)).To(BeTrue(),
			"Maestro adapter Health should be False")
	}, h.Cfg.Timeouts.Adapter.Processing, h.Cfg.Polling.Interval).Should(Succeed())
}

// verifyClusterNotReconciledDuringOutage verifies the cluster Reconciled condition remains False.
func verifyClusterNotReconciledDuringOutage(ctx context.Context, h *helper.Helper, clusterID string) {
	Consistently(func(g Gomega) {
		cl, err := h.Client.GetCluster(ctx, clusterID)
		g.Expect(err).NotTo(HaveOccurred(), "failed to get cluster")
		g.Expect(cl.Status).NotTo(BeNil(), "cluster status should be present")

		g.Expect(h.HasResourceCondition(cl.Status.Conditions,
			client.ConditionTypeReconciled, openapi.ResourceConditionStatusFalse)).To(BeTrue(),
			"cluster Reconciled should remain False while Maestro is unavailable")
	}, h.Cfg.Polling.Interval*3, h.Cfg.Polling.Interval).Should(Succeed())
}

// verifyMaestroAdapterRecovery verifies the Maestro adapter recovers with Applied=True, Available=True, Health=True.
func verifyMaestroAdapterRecovery(ctx context.Context, h *helper.Helper, clusterID, adapterName string) {
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
			"Maestro adapter should be present in statuses after recovery")

		g.Expect(h.HasAdapterCondition(adapterStatus.Conditions,
			client.ConditionTypeApplied, openapi.AdapterConditionStatusTrue)).To(BeTrue(),
			"Maestro adapter Applied should be True after recovery")

		g.Expect(h.HasAdapterCondition(adapterStatus.Conditions,
			client.ConditionTypeAvailable, openapi.AdapterConditionStatusTrue)).To(BeTrue(),
			"Maestro adapter Available should be True after recovery")

		g.Expect(h.HasAdapterCondition(adapterStatus.Conditions,
			client.ConditionTypeHealth, openapi.AdapterConditionStatusTrue)).To(BeTrue(),
			"Maestro adapter Health should be True after recovery")
	}, h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).Should(Succeed())
}

// verifyClusterReconciledAfterRecovery verifies the cluster reaches Reconciled=True and LastKnownReconciled=True.
func verifyClusterReconciledAfterRecovery(ctx context.Context, h *helper.Helper, clusterID string) {
	Eventually(func(g Gomega) {
		cl, err := h.Client.GetCluster(ctx, clusterID)
		g.Expect(err).NotTo(HaveOccurred(), "failed to get cluster")
		g.Expect(cl.Status).NotTo(BeNil(), "cluster status should be present")

		g.Expect(h.HasResourceCondition(cl.Status.Conditions,
			client.ConditionTypeReconciled, openapi.ResourceConditionStatusTrue)).To(BeTrue(),
			"cluster Reconciled should transition to True")

		g.Expect(h.HasResourceCondition(cl.Status.Conditions,
			client.ConditionTypeLastKnownReconciled, openapi.ResourceConditionStatusTrue)).To(BeTrue(),
			"cluster LastKnownReconciled should transition to True")
	}, h.Cfg.Timeouts.Cluster.Reconciled, h.Cfg.Polling.Interval).Should(Succeed())
}
