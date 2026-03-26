package helper

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/logger"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// generateRandomString generates a random alphanumeric string of the specified length
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			// Fallback: use current time nanoseconds for basic randomness
			b[i] = charset[(time.Now().UnixNano()+int64(i))%int64(len(charset))]
		} else {
			b[i] = charset[n.Int64()]
		}
	}
	return string(b)
}

// AdapterDeploymentOptions contains configuration for deploying an adapter via Helm
type AdapterDeploymentOptions struct {
	ReleaseName string
	Namespace   string
	ChartPath   string
	AdapterName string
	Timeout     time.Duration
	SetValues   map[string]string // Additional Helm --set values
}

// GenerateAdapterReleaseName generates a unique Helm release name for an adapter deployment
// The release name format is: adapter-<resource_type>-<adapter_name>-<random_suffix>
// The random suffix prevents conflicts when multiple tests run concurrently or when cleanup from previous runs is incomplete
// The name is truncated to 48 characters to leave room for Helm's deployment/pod suffixes (Kubernetes has a 63-char limit)
// If truncation is needed, the random suffix is always preserved to maintain uniqueness
func GenerateAdapterReleaseName(resourceType, adapterName string) string {
	randomSuffix := generateRandomString(5)

	// Kubernetes resource names have a 63-character limit
	// Reserve ~15 characters for Helm's deployment/pod suffixes
	maxReleaseNameLength := 48

	// Build the base name without the suffix first
	baseWithoutSuffix := fmt.Sprintf("adapter-%s-%s", resourceType, adapterName)

	// Calculate how much space we have for the base (reserve space for "-" + suffix)
	maxBaseLength := maxReleaseNameLength - len(randomSuffix) - 1

	// Truncate the base if necessary, but always keep the suffix
	if len(baseWithoutSuffix) > maxBaseLength {
		baseWithoutSuffix = baseWithoutSuffix[:maxBaseLength]
	}

	releaseName := fmt.Sprintf("%s-%s", baseWithoutSuffix, randomSuffix)
	return releaseName
}

// DeployAdapter deploys an adapter using Helm upgrade --install
// This is a common function that can be reused across test cases
// The release name must be provided via opts.ReleaseName - use GenerateAdapterReleaseName() to create a unique name
func (h *Helper) DeployAdapter(ctx context.Context, opts AdapterDeploymentOptions) error {
	// Validate required fields
	if opts.Namespace == "" {
		return fmt.Errorf("AdapterDeploymentOptions.Namespace is required")
	}
	if opts.ChartPath == "" {
		return fmt.Errorf("AdapterDeploymentOptions.ChartPath is required")
	}
	if opts.AdapterName == "" {
		return fmt.Errorf("AdapterDeploymentOptions.AdapterName is required")
	}
	if opts.ReleaseName == "" {
		return fmt.Errorf("AdapterDeploymentOptions.ReleaseName is required - use GenerateAdapterReleaseName() to create a unique name")
	}

	// Set default timeout if not specified
	if opts.Timeout == 0 {
		opts.Timeout = 5 * time.Minute
	}

	releaseName := opts.ReleaseName

	logger.Info("deploying adapter via Helm",
		"adapter_name", opts.AdapterName,
		"release_name", releaseName,
		"namespace", opts.Namespace)

	// Copy adapter config folder to chart directory
	sourceAdapterDir := filepath.Join(h.Cfg.TestDataDir, AdapterConfigsDir, opts.AdapterName)
	destAdapterDir := filepath.Join(opts.ChartPath, opts.AdapterName)

	// Remove existing adapter config directory if it exists
	if _, err := os.Stat(destAdapterDir); err == nil {
		logger.Info("removing existing adapter config directory", "path", destAdapterDir)
		if err := os.RemoveAll(destAdapterDir); err != nil {
			return fmt.Errorf("failed to remove existing adapter config directory: %w", err)
		}
	}

	// Copy adapter config directory to chart
	logger.Info("copying adapter config", "from", sourceAdapterDir, "to", destAdapterDir)
	if err := copyDir(sourceAdapterDir, destAdapterDir); err != nil {
		return fmt.Errorf("failed to copy adapter config directory: %w", err)
	}

	// Determine the values.yaml file path in the copied adapter directory
	valuesFilePath := filepath.Join(destAdapterDir, "values.yaml")

	// Expand environment variables in values.yaml in-place using envsubst
	logger.Info("expanding environment variables in values.yaml in-place", "values_file", valuesFilePath)

	// Expand environment variables in values.yaml using envsubst
	expandedContent, err := expandEnvVarsInYAMLToBytes(ctx, valuesFilePath)
	if err != nil {
		return fmt.Errorf("failed to expand environment variables in values.yaml: %w", err)
	}

	// Overwrite values.yaml with expanded content
	if err := os.WriteFile(valuesFilePath, expandedContent, 0600); err != nil {
		return fmt.Errorf("failed to overwrite values.yaml with expanded content: %w", err)
	}

	logger.Info("successfully expanded environment variables in values.yaml")

	// Build Helm command with single values file
	helmArgs := []string{
		"upgrade", "--install",
		releaseName,
		opts.ChartPath,
		"--namespace", opts.Namespace,
		"--create-namespace",
		"--wait",
		"--timeout", opts.Timeout.String(),
		"-f", valuesFilePath,
	}

	// Add fullnameOverride to ensure consistent release naming
	helmArgs = append(helmArgs,
		"--set", fmt.Sprintf("fullnameOverride=%s", releaseName),
	)

	// Add additional --set values if provided
	for key, value := range opts.SetValues {
		helmArgs = append(helmArgs, "--set", fmt.Sprintf("%s=%s", key, value))
	}

	logger.Info("executing Helm command", "args", helmArgs)

	// Create context with timeout
	cmdCtx, cancel := context.WithTimeout(ctx, opts.Timeout+30*time.Second)
	defer cancel()

	// Execute Helm command
	cmd := exec.CommandContext(cmdCtx, "helm", helmArgs...) // #nosec G204 -- helmArgs is constructed from trusted config
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Error("helm upgrade failed", "error", err, "output", string(output))

		// Collect diagnostic information when deployment fails
		h.saveDiagnosticLogs(ctx, opts.AdapterName, releaseName, opts.Namespace)

		return fmt.Errorf("helm upgrade failed: %w (output: %s)", err, string(output))
	}

	logger.Info("adapter deployed successfully",
		"release_name", releaseName,
		"output", string(output))

	return nil
}

// UninstallAdapter uninstalls an adapter using Helm uninstall
// This is a common function that can be reused across test cases
func (h *Helper) UninstallAdapter(ctx context.Context, releaseName, namespace string) error {
	logger.Info("uninstalling adapter via Helm",
		"release_name", releaseName,
		"namespace", namespace)

	// Create context with timeout
	cmdCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()

	// Execute Helm uninstall command
	cmd := exec.CommandContext(cmdCtx, "helm", "uninstall", releaseName,
		"-n", namespace,
		"--wait",
		"--timeout", "5m")

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Check if the error is because the release doesn't exist
		if strings.Contains(string(output), "not found") {
			logger.Info("adapter release not found, skipping uninstall", "release_name", releaseName)
			// Clean up orphaned cluster-scoped resources even when release is not found
			// This handles cases like interrupted installs or manual deletions
			h.cleanupClusterScopedResources(ctx, releaseName)
			return nil
		}
		logger.Error("helm uninstall failed", "error", err, "output", string(output))
		return fmt.Errorf("helm uninstall failed: %w (output: %s)", err, string(output))
	}

	logger.Info("adapter uninstalled successfully",
		"release_name", releaseName,
		"output", string(output))

	// Clean up any orphaned cluster-scoped resources (ClusterRoles, ClusterRoleBindings)
	// These can be left behind if a previous test run failed or was interrupted
	h.cleanupClusterScopedResources(ctx, releaseName)

	return nil
}

// cleanupClusterScopedResources removes orphaned cluster-scoped resources that may be left
// after Helm uninstall. This is a best-effort cleanup and logs errors without failing.
func (h *Helper) cleanupClusterScopedResources(ctx context.Context, releaseName string) {
	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// Try to delete ClusterRole
	clusterRoleCmd := exec.CommandContext(cmdCtx, "kubectl", "delete", "clusterrole", releaseName,
		"--ignore-not-found=true")
	if output, err := clusterRoleCmd.CombinedOutput(); err != nil {
		logger.Info("could not delete ClusterRole (may not exist)",
			"release_name", releaseName,
			"output", string(output))
	} else {
		logger.Info("cleaned up ClusterRole", "release_name", releaseName)
	}

	// Try to delete ClusterRoleBinding
	clusterRoleBindingCmd := exec.CommandContext(cmdCtx, "kubectl", "delete", "clusterrolebinding", releaseName,
		"--ignore-not-found=true")
	if output, err := clusterRoleBindingCmd.CombinedOutput(); err != nil {
		logger.Info("could not delete ClusterRoleBinding (may not exist)",
			"release_name", releaseName,
			"output", string(output))
	} else {
		logger.Info("cleaned up ClusterRoleBinding", "release_name", releaseName)
	}
}

// saveDiagnosticLogs saves diagnostic information when adapter deployment fails
// Saves to <outputDir>/<adapter-name>-<random-4chars>/ directory
// outputDir is configured via OUTPUT_DIR env var or config file (defaults to "output")
func (h *Helper) saveDiagnosticLogs(ctx context.Context, adapterName, releaseName, namespace string) {
	// Generate output directory with adapter name and random suffix
	randomSuffix := generateRandomString(4)
	outputDir := filepath.Join(h.Cfg.OutputDir, fmt.Sprintf("%s-%s", adapterName, randomSuffix))

	// Create output directory
	if err := os.MkdirAll(outputDir, 0750); err != nil {
		logger.Error("failed to create diagnostic output directory",
			"error", err,
			"output_dir", outputDir)
		return
	}

	logger.Info("saving diagnostic logs",
		"adapter_name", adapterName,
		"release_name", releaseName,
		"namespace", namespace,
		"output_dir", outputDir)

	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// 1. Get pods using client-go
	pods, err := h.K8sClient.CoreV1().Pods(namespace).List(cmdCtx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/instance=%s", releaseName),
	})
	if err != nil {
		logger.Error("failed to list pods", "error", err)
		return
	}

	if len(pods.Items) == 0 {
		logger.Info("no pods found for release", "release_name", releaseName)
		return
	}

	logger.Info("found pods for release",
		"total_pods", len(pods.Items),
		"release_name", releaseName)

	// Save logs and description for unhealthy pods only
	for _, pod := range pods.Items {
		// Check if pod is healthy (Running and all containers ready)
		isHealthy := pod.Status.Phase == "Running"
		if isHealthy && len(pod.Status.ContainerStatuses) > 0 {
			for _, cs := range pod.Status.ContainerStatuses {
				if !cs.Ready {
					isHealthy = false
					break
				}
			}
		}

		// Skip healthy pods
		if isHealthy {
			logger.Info("skipping healthy pod", "pod", pod.Name)
			continue
		}

		podName := pod.Name
		logger.Info("saving logs for unhealthy pod",
			"pod", podName,
			"phase", pod.Status.Phase)

		// Save pod logs using kubectl command
		podLogFile := filepath.Join(outputDir, fmt.Sprintf("%s.log", podName))
		podLogCmd := exec.CommandContext(cmdCtx, "kubectl", "logs", // #nosec G204 -- podName and namespace are from trusted k8s API
			podName,
			"-n", namespace,
			"--tail=200")

		var logContent string
		logContent += fmt.Sprintf("$ %s\n\n", podLogCmd.String())
		logOutput, err := podLogCmd.CombinedOutput()
		if err != nil {
			logContent += fmt.Sprintf("Error: %v\n", err)
			logContent += string(logOutput)
		} else {
			logContent += string(logOutput)
		}

		if err := os.WriteFile(podLogFile, []byte(logContent), 0600); err != nil {
			logger.Error("failed to write pod log file",
				"pod", podName,
				"error", err)
		} else {
			logger.Info("saved pod logs",
				"pod", podName,
				"file", podLogFile)
		}

		// Save pod description using kubectl describe command
		podDescFile := filepath.Join(outputDir, fmt.Sprintf("%s-describe.txt", podName))
		podDescCmd := exec.CommandContext(cmdCtx, "kubectl", "describe", "pod", // #nosec G204 -- podName and namespace are from trusted k8s API
			podName,
			"-n", namespace)

		var descContent string
		descContent += fmt.Sprintf("$ %s\n\n", podDescCmd.String())
		descOutput, err := podDescCmd.CombinedOutput()
		if err != nil {
			descContent += fmt.Sprintf("Error: %v\n", err)
			descContent += string(descOutput)
		} else {
			descContent += string(descOutput)
		}

		if err := os.WriteFile(podDescFile, []byte(descContent), 0600); err != nil {
			logger.Error("failed to write pod description file",
				"pod", podName,
				"error", err)
		} else {
			logger.Info("saved pod description",
				"pod", podName,
				"file", podDescFile)
		}
	}

	logger.Info("diagnostic logs saved successfully", "output_dir", outputDir)
}

// expandEnvVarsInYAMLToBytes expands environment variables in a YAML file using envsubst
// Returns the expanded content as bytes
func expandEnvVarsInYAMLToBytes(ctx context.Context, yamlPath string) ([]byte, error) {
	// Read the YAML file
	content, err := os.ReadFile(yamlPath) // #nosec G304 -- yamlPath is constructed from trusted config
	if err != nil {
		return nil, fmt.Errorf("failed to read YAML file: %w", err)
	}

	// Use envsubst command to expand environment variables
	cmd := exec.CommandContext(ctx, "envsubst")
	cmd.Stdin = bytes.NewReader(content)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("envsubst failed: %w (stderr: %s)", err, stderr.String())
	}

	return stdout.Bytes(), nil
}

// DeletePubSubSubscription deletes a Google Pub/Sub subscription.
// If the subscription does not exist, it is treated as a no-op.
func (h *Helper) DeletePubSubSubscription(ctx context.Context, subscriptionID string) error {
	projectID := h.Cfg.GCPProjectID
	if projectID == "" {
		projectID = defaultGCPProjectID
	}

	logger.Info("deleting Pub/Sub subscription",
		"subscription", subscriptionID,
		"project", projectID)

	cmdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(cmdCtx, "gcloud", "pubsub", "subscriptions", "delete", // #nosec G204 -- subscriptionID and projectID are from trusted test config
		subscriptionID,
		"--project="+projectID,
		"--quiet")

	output, err := cmd.CombinedOutput()
	if err != nil {
		outputStr := string(output)
		if strings.Contains(outputStr, "NOT_FOUND") || strings.Contains(outputStr, "not found") {
			logger.Info("Pub/Sub subscription not found, skipping deletion", "subscription", subscriptionID)
			return nil
		}
		return fmt.Errorf("failed to delete Pub/Sub subscription %s: %w (output: %s)", subscriptionID, err, outputStr)
	}

	logger.Info("Pub/Sub subscription deleted successfully", "subscription", subscriptionID)
	return nil
}

// copyDir recursively copies a directory tree
func copyDir(src, dst string) error {
	// Get source directory info
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	// Create destination directory
	if err := os.MkdirAll(dst, srcInfo.Mode()); err != nil {
		return err
	}

	// Read source directory contents
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}

	// Copy each entry
	for _, entry := range entries {
		srcPath := filepath.Join(src, entry.Name())
		dstPath := filepath.Join(dst, entry.Name())

		if entry.IsDir() {
			// Recursively copy subdirectory
			if err := copyDir(srcPath, dstPath); err != nil {
				return err
			}
		} else {
			// Copy file
			if err := copyFile(srcPath, dstPath); err != nil {
				return err
			}
		}
	}

	return nil
}

// copyFile copies a single file
func copyFile(src, dst string) error {
	srcData, err := os.ReadFile(src) // #nosec G304 -- src is constructed from trusted config
	if err != nil {
		return err
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	return os.WriteFile(dst, srcData, srcInfo.Mode())
}
