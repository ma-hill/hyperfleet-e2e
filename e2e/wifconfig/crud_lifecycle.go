package wifconfig

import (
	"context"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: wifconfig][crud] WifConfig CRUD Lifecycle",
	ginkgo.Label(labels.Tier0),
	func() {
		var h *helper.Helper
		var wifConfigID string
		var wifConfig *client.Resource

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			var err error
			wifConfig, err = h.Client.CreateWifConfigFromPayload(ctx, h.TestDataPath("payloads/wifconfigs/wifconfig-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create wifconfig")
			Expect(wifConfig.Id).NotTo(BeNil(), "wifconfig ID should be generated")
			Expect(wifConfig.Name).NotTo(BeEmpty(), "wifconfig name should be present")
			wifConfigID = *wifConfig.Id
			ginkgo.GinkgoWriter.Printf("Created wifconfig ID: %s, Name: %s\n", wifConfigID, wifConfig.Name)

			ginkgo.DeferCleanup(func(ctx context.Context) {
				if err := h.CleanupTestWifConfig(ctx, wifConfigID); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup wifconfig %s: %v\n", wifConfigID, err)
				}
			})
		})

		ginkgo.It("should create a wifconfig with correct fields", func(ctx context.Context) {
			Expect(wifConfig.Kind).To(Equal("WifConfig"), "kind should be WifConfig")
			Expect(wifConfig.Generation).To(Equal(int32(1)), "new wifconfig should have generation=1")
			Expect(wifConfig.DeletedTime).To(BeNil(), "new wifconfig should not have deleted_time")
		})

		ginkgo.It("should retrieve wifconfig by ID", func(ctx context.Context) {
			ginkgo.By("fetching wifconfig by ID")
			fetched, err := h.Client.GetWifConfig(ctx, wifConfigID)
			Expect(err).NotTo(HaveOccurred(), "failed to get wifconfig")

			Expect(*fetched.Id).To(Equal(wifConfigID))
			Expect(fetched.Name).To(Equal(wifConfig.Name))
			Expect(fetched.Kind).To(Equal("WifConfig"))
			Expect(fetched.Generation).To(Equal(int32(1)))
		})

		ginkgo.It("should list wifconfigs and find the created one", func(ctx context.Context) {
			ginkgo.By("listing all wifconfigs")
			list, err := h.Client.ListWifConfigs(ctx, "")
			Expect(err).NotTo(HaveOccurred(), "failed to list wifconfigs")
			Expect(list.Items).NotTo(BeEmpty(), "wifconfig list should not be empty")

			ginkgo.By("verifying created wifconfig appears in list")
			found := false
			for _, wc := range list.Items {
				if wc.Id != nil && *wc.Id == wifConfigID {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "created wifconfig should appear in list")
		})

		ginkgo.It("should update wifconfig via PATCH", func(ctx context.Context) {
			updatedProjectID := "updated-project"
			updatedVersion := "4.18"

			ginkgo.By("patching wifconfig spec")
			patched, err := h.Client.PatchWifConfig(ctx, wifConfigID, client.ResourcePatchRequest{
				Spec: map[string]any{
					"projectId": updatedProjectID,
					"version":   updatedVersion,
				},
			})
			Expect(err).NotTo(HaveOccurred(), "failed to patch wifconfig")
			Expect(patched.Generation).To(Equal(int32(2)), "generation should increment after PATCH")

			ginkgo.By("verifying patched spec via GET")
			fetched, err := h.Client.GetWifConfig(ctx, wifConfigID)
			Expect(err).NotTo(HaveOccurred())
			Expect(fetched.Spec["projectId"]).To(Equal(updatedProjectID))
			Expect(fetched.Spec["version"]).To(Equal(updatedVersion))
		})

		ginkgo.It("should delete wifconfig", func(ctx context.Context) {
			ginkgo.By("deleting wifconfig")
			deleted, err := h.Client.DeleteWifConfig(ctx, wifConfigID)
			Expect(err).NotTo(HaveOccurred(), "failed to delete wifconfig")
			Expect(deleted.DeletedTime).NotTo(BeNil(), "deleted wifconfig should have deleted_time set")
		})
	},
)
