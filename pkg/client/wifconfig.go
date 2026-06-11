package client

import (
	"context"
	"fmt"

	"github.com/samber/lo"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/logger"
)

func (c *HyperFleetClient) CreateWifConfig(ctx context.Context, req ResourceCreateRequest) (*Resource, error) {
	logger.Info("creating wifconfig", "name", req.Name)
	wifConfig, err := c.CreateResource(ctx, "wifconfigs", req)
	if err != nil {
		return nil, fmt.Errorf("create wifconfig %q: %w", req.Name, err)
	}
	logger.Info("wifconfig created", "wifconfig_id", lo.FromPtr(wifConfig.Id), "name", req.Name)
	return wifConfig, nil
}

func (c *HyperFleetClient) GetWifConfig(ctx context.Context, wifConfigID string) (*Resource, error) {
	return c.GetResource(ctx, fmt.Sprintf("wifconfigs/%s", wifConfigID))
}

func (c *HyperFleetClient) ListWifConfigs(ctx context.Context, search string) (*ResourceList, error) {
	return c.ListResources(ctx, "wifconfigs", search)
}

func (c *HyperFleetClient) DeleteWifConfig(ctx context.Context, wifConfigID string) (*Resource, error) {
	logger.Info("deleting wifconfig", "wifconfig_id", wifConfigID)
	wifConfig, err := c.DeleteResource(ctx, fmt.Sprintf("wifconfigs/%s", wifConfigID))
	if err != nil {
		return nil, fmt.Errorf("delete wifconfig %q: %w", wifConfigID, err)
	}
	logger.Info("wifconfig deleted", "wifconfig_id", wifConfigID)
	return wifConfig, nil
}

func (c *HyperFleetClient) PatchWifConfig(ctx context.Context, wifConfigID string, req ResourcePatchRequest) (*Resource, error) {
	logger.Info("patching wifconfig", "wifconfig_id", wifConfigID)
	wifConfig, err := c.PatchResource(ctx, fmt.Sprintf("wifconfigs/%s", wifConfigID), req)
	if err != nil {
		return nil, fmt.Errorf("patch wifconfig %q: %w", wifConfigID, err)
	}
	logger.Info("wifconfig patched", "wifconfig_id", wifConfigID, "generation", wifConfig.Generation)
	return wifConfig, nil
}

func (c *HyperFleetClient) CreateWifConfigFromPayload(ctx context.Context, payloadPath string) (*Resource, error) {
	return c.CreateResourceFromPayload(ctx, "wifconfigs", payloadPath)
}
