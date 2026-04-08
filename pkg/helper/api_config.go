package helper

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/logger"
)

// UpgradeAPIRequiredAdapters upgrades the API Helm release to update the required adapters list.
func (h *Helper) UpgradeAPIRequiredAdapters(ctx context.Context, apiChartPath, namespace string, clusterAdapters []string) error {
	adapterList := "{" + strings.Join(clusterAdapters, ",") + "}"

	logger.Info("upgrading API required adapters",
		"namespace", namespace,
		"cluster_adapters", adapterList)

	cmdCtx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "helm", "upgrade", "api", apiChartPath, // #nosec G204 -- args from trusted test config
		"--namespace", namespace,
		"--reuse-values",
		"--wait",
		"--timeout", "3m",
		"--set", "config.adapters.required.cluster="+adapterList,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Error("helm upgrade API failed", "error", err, "output", string(output))
		return fmt.Errorf("helm upgrade API failed: %w (output: %s)", err, string(output))
	}

	logger.Info("API required adapters updated successfully",
		"cluster_adapters", adapterList,
		"output", string(output))
	return nil
}

// RestoreAPIRequiredAdaptersWithRetry restores the API required adapters with retry logic.
// This is designed for use in DeferCleanup to ensure API config is restored even on transient failures.
func (h *Helper) RestoreAPIRequiredAdaptersWithRetry(ctx context.Context, apiChartPath, namespace string, originalAdapters []string, maxRetries int) error {
	var lastErr error
	for attempt := 1; attempt <= maxRetries; attempt++ {
		err := h.UpgradeAPIRequiredAdapters(ctx, apiChartPath, namespace, originalAdapters)
		if err == nil {
			logger.Info("API config restored successfully", "attempt", attempt)
			return nil
		}
		lastErr = err
		logger.Error("failed to restore API config, retrying",
			"attempt", attempt,
			"max_retries", maxRetries,
			"error", err)
		if attempt < maxRetries {
			timer := time.NewTimer(10 * time.Second)
			select {
			case <-ctx.Done():
				timer.Stop()
				return fmt.Errorf("context cancelled during API config restore retry: %w", ctx.Err())
			case <-timer.C:
			}
		}
	}

	// All retries failed — log the manual fix command
	adapterList := strings.Join(originalAdapters, ",")
	logger.Error("CRITICAL: failed to restore API config after all retries. Manual fix required",
		"max_retries", maxRetries,
		"error", lastErr,
		"manual_fix", fmt.Sprintf(
			"helm upgrade api %s -n %s --reuse-values --set config.adapters.required.cluster={%s}",
			apiChartPath, namespace, adapterList))

	return fmt.Errorf("failed to restore API config after %d retries: %w", maxRetries, lastErr)
}

// GetAPIRequiredClusterAdapters returns the current list of required cluster adapters from config.
func (h *Helper) GetAPIRequiredClusterAdapters() []string {
	return h.Cfg.Adapters.Cluster
}
