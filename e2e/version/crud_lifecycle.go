package version

import (
	"context"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: version][crud] Version CRUD Lifecycle",
	ginkgo.Label(labels.Tier0),
	func() {
		var h *helper.Helper
		var channelID string
		var versionID string
		var version *client.Resource

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			ginkgo.By("creating parent channel")
			ch, err := h.Client.CreateChannelFromPayload(ctx, h.TestDataPath("payloads/channels/channel-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create channel")
			Expect(ch.Id).NotTo(BeNil(), "channel ID should not be nil")
			channelID = *ch.Id
			ginkgo.GinkgoWriter.Printf("Created parent channel ID: %s\n", channelID)

			ginkgo.By("creating version under channel")
			version, err = h.Client.CreateVersionFromPayload(ctx, channelID, h.TestDataPath("payloads/versions/version-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create version")
			Expect(version.Id).NotTo(BeNil(), "version ID should be generated")
			Expect(version.Name).NotTo(BeEmpty(), "version name should be present")
			versionID = *version.Id
			ginkgo.GinkgoWriter.Printf("Created version ID: %s, Name: %s\n", versionID, version.Name)

			ginkgo.DeferCleanup(func(ctx context.Context) {
				if err := h.CleanupTestChannel(ctx, channelID); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup channel %s: %v\n", channelID, err)
				}
			})
		})

		ginkgo.It("should create a version with correct fields", func(ctx context.Context) {
			Expect(version.Kind).To(HaveValue(Equal("Version")), "kind should be Version")
			Expect(version.Generation).To(Equal(int32(1)), "new version should have generation=1")
			Expect(version.DeletedTime).To(BeNil(), "new version should not have deleted_time")
		})

		ginkgo.It("should retrieve version by ID", func(ctx context.Context) {
			ginkgo.By("fetching version by ID")
			fetched, err := h.Client.GetVersion(ctx, channelID, versionID)
			Expect(err).NotTo(HaveOccurred(), "failed to get version")

			Expect(*fetched.Id).To(Equal(versionID))
			Expect(fetched.Name).To(Equal(version.Name))
			Expect(fetched.Kind).To(HaveValue(Equal("Version")))
			Expect(fetched.Generation).To(Equal(int32(1)))
		})

		ginkgo.It("should list versions under channel", func(ctx context.Context) {
			ginkgo.By("listing versions")
			list, err := h.Client.ListVersions(ctx, channelID, "")
			Expect(err).NotTo(HaveOccurred(), "failed to list versions")
			Expect(list.Items).NotTo(BeEmpty(), "version list should not be empty")

			ginkgo.By("verifying created version appears in list")
			found := false
			for _, v := range list.Items {
				if v.Id != nil && *v.Id == versionID {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "created version should appear in list")
		})

		ginkgo.It("should update version via PATCH", func(ctx context.Context) {
			ginkgo.By("patching version spec")
			patched, err := h.Client.PatchVersion(ctx, channelID, versionID, client.ResourcePatchRequest{
				Spec: map[string]any{
					"raw_version":   "4.18.0",
					"enabled":       true,
					"is_default":    true,
					"release_image": "quay.io/openshift-release-dev/ocp-release:4.18.0",
				},
			})
			Expect(err).NotTo(HaveOccurred(), "failed to patch version")
			Expect(patched.Generation).To(Equal(int32(2)), "generation should increment after PATCH")

			ginkgo.By("verifying patched spec via GET")
			fetched, err := h.Client.GetVersion(ctx, channelID, versionID)
			Expect(err).NotTo(HaveOccurred())
			Expect(fetched.Spec["is_default"]).To(Equal(true))
			Expect(fetched.Spec["raw_version"]).To(Equal("4.18.0"))
		})

		ginkgo.It("should update version labels via PATCH", func(ctx context.Context) {
			ginkgo.By("patching version labels")
			patched, err := h.Client.PatchVersion(ctx, channelID, versionID, client.ResourcePatchRequest{
				Labels: map[string]string{
					"crud": "version-lifecycle",
				},
			})
			Expect(err).NotTo(HaveOccurred(), "failed to patch version")
			Expect(patched.Generation).To(Equal(int32(2)), "generation should increment after PATCH")

			ginkgo.By("verifying patched labels via LIST")
			fetched, err := h.Client.ListVersions(ctx, channelID, "labels.crud='version-lifecycle'")
			Expect(err).NotTo(HaveOccurred())
			Expect(fetched.Items).To(HaveLen(1), "should have 1 version")
			Expect(fetched.Items[0].Id).To(HaveValue(Equal(versionID)))
		})

		ginkgo.It("should delete version", func(ctx context.Context) {
			ginkgo.By("deleting version")
			deleted, err := h.Client.DeleteVersion(ctx, channelID, versionID)
			Expect(err).NotTo(HaveOccurred(), "failed to delete version")
			Expect(deleted.DeletedTime).NotTo(BeNil(), "deleted version should have deleted_time set")
		})
	},
)
