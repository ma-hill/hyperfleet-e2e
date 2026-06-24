package channel

import (
	"context"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/labels"
)

var _ = ginkgo.Describe("[Suite: channel][crud] Channel CRUD Lifecycle",
	ginkgo.Label(labels.Tier0),
	func() {
		var h *helper.Helper
		var channelID string
		var channel *client.Resource

		ginkgo.BeforeEach(func(ctx context.Context) {
			h = helper.New()

			var err error
			channel, err = h.Client.CreateChannelFromPayload(ctx, h.TestDataPath("payloads/channels/channel-request.json"))
			Expect(err).NotTo(HaveOccurred(), "failed to create channel")
			Expect(channel.Id).NotTo(BeNil(), "channel ID should be generated")
			Expect(channel.Name).NotTo(BeEmpty(), "channel name should be present")
			channelID = *channel.Id
			ginkgo.GinkgoWriter.Printf("Created channel ID: %s, Name: %s\n", channelID, channel.Name)

			ginkgo.DeferCleanup(func(ctx context.Context) {
				if err := h.CleanupTestChannel(ctx, channelID); err != nil {
					ginkgo.GinkgoWriter.Printf("Warning: failed to cleanup channel %s: %v\n", channelID, err)
				}
			})
		})

		ginkgo.It("should create a channel with correct fields", func(ctx context.Context) {
			Expect(channel.Kind).To(Equal("Channel"), "kind should be Channel")
			Expect(channel.Generation).To(Equal(int32(1)), "new channel should have generation=1")
			Expect(channel.DeletedTime).To(BeNil(), "new channel should not have deleted_time")
		})

		ginkgo.It("should retrieve channel by ID", func(ctx context.Context) {
			ginkgo.By("fetching channel by ID")
			fetched, err := h.Client.GetChannel(ctx, channelID)
			Expect(err).NotTo(HaveOccurred(), "failed to get channel")

			Expect(*fetched.Id).To(Equal(channelID))
			Expect(fetched.Name).To(Equal(channel.Name))
			Expect(fetched.Kind).To(Equal("Channel"))
			Expect(fetched.Generation).To(Equal(int32(1)))
		})

		ginkgo.It("should list channels and find the created one", func(ctx context.Context) {
			ginkgo.By("listing all channels")
			list, err := h.Client.ListChannels(ctx, "")
			Expect(err).NotTo(HaveOccurred(), "failed to list channels")
			Expect(list.Items).NotTo(BeEmpty(), "channel list should not be empty")

			ginkgo.By("verifying created channel appears in list")
			found := false
			for _, ch := range list.Items {
				if ch.Id != nil && *ch.Id == channelID {
					found = true
					break
				}
			}
			Expect(found).To(BeTrue(), "created channel should appear in list")
		})

		ginkgo.It("should update channel via PATCH", func(ctx context.Context) {
			ginkgo.By("patching channel spec")
			patched, err := h.Client.PatchChannel(ctx, channelID, client.ResourcePatchRequest{
				Spec: map[string]any{
					"is_default":    true,
					"enabled_regex": ".*",
				},
			})
			Expect(err).NotTo(HaveOccurred(), "failed to patch channel")
			Expect(patched.Generation).To(Equal(int32(2)), "generation should increment after PATCH")

			ginkgo.By("verifying patched spec via GET")
			fetched, err := h.Client.GetChannel(ctx, channelID)
			Expect(err).NotTo(HaveOccurred())
			Expect(fetched.Spec["is_default"]).To(Equal(true))
		})

		ginkgo.It("should update channel labels via PATCH", func(ctx context.Context) {
			ginkgo.By("patching channel labels")
			patched, err := h.Client.PatchChannel(ctx, channelID, client.ResourcePatchRequest{
				Labels: map[string]string{
					"crud": channelID,
				},
			})
			Expect(err).NotTo(HaveOccurred(), "failed to patch channel")
			Expect(patched.Generation).To(Equal(int32(2)), "generation should increment after PATCH")

			ginkgo.By("verifying patched labels via GET")
			fetched, err := h.Client.ListChannels(ctx, "labels.crud='"+channelID+"'")
			Expect(err).NotTo(HaveOccurred())
			Expect(fetched.Items).To(HaveLen(1), "should have 1 channel")
			Expect(fetched.Items[0].Id).To(HaveValue(Equal(channelID)))
		})

		ginkgo.It("should delete channel", func(ctx context.Context) {
			ginkgo.By("deleting channel")
			deleted, err := h.Client.DeleteChannel(ctx, channelID)
			Expect(err).NotTo(HaveOccurred(), "failed to delete channel")
			Expect(deleted.DeletedTime).NotTo(BeNil(), "deleted channel should have deleted_time set")
		})
	},
)
