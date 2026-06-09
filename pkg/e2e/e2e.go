package e2e

import (
	"context"
	"log"
	"testing"
	"time"

	"github.com/onsi/ginkgo/v2"
	"github.com/onsi/ginkgo/v2/types"
	"github.com/onsi/gomega"
	"github.com/spf13/viper"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
)

// RunTests is the main entry point for executing e2e tests from the CLI
// It runs Ginkgo tests directly without using testing.Main (which calls os.Exit)
func RunTests(ctx context.Context) int {
	select {
	case <-ctx.Done():
		log.Printf("Test execution cancelled: %v", ctx.Err())
		return 1
	default:
	}

	// Initialize testing framework without calling testing.Main
	testing.Init()

	// Register Gomega fail handler
	gomega.RegisterFailHandler(ginkgo.Fail)

	// Configure Ginkgo from viper
	suiteConfig, reporterConfig := ginkgo.GinkgoConfiguration()

	// Default the suite timeout to 2h. GinkgoConfiguration() returns Ginkgo's
	// own 1h default (never 0), so a post-hoc `if Timeout == 0` guard never
	// fired. Set the default here, before the viper override so SUITE_TIMEOUT
	// still wins when set.
	suiteConfig.Timeout = 2 * time.Hour

	configureGinkgoFromViper(&suiteConfig, &reporterConfig)

	// Run the test suite using Ginkgo's native GinkgoT
	// This avoids testing.Main and its os.Exit call
	passed := ginkgo.RunSpecs(ginkgo.GinkgoT(), "HyperFleet E2E Suite", suiteConfig, reporterConfig)

	if !passed {
		return 1
	}

	return 0
}

// configureGinkgoFromViper sets up Ginkgo configuration from viper
func configureGinkgoFromViper(suiteConfig *types.SuiteConfig, reporterConfig *types.ReporterConfig) {
	if timeout := viper.GetDuration(config.Tests.SuiteTimeout); timeout > 0 {
		suiteConfig.Timeout = timeout
	}

	if labelFilter := viper.GetString(config.Tests.GinkgoLabelFilter); labelFilter != "" {
		suiteConfig.LabelFilter = labelFilter
	}

	if focusTests := viper.GetString(config.Tests.GinkgoFocus); focusTests != "" {
		suiteConfig.FocusStrings = append(suiteConfig.FocusStrings, focusTests)
	}

	if skipTests := viper.GetString(config.Tests.GinkgoSkip); skipTests != "" {
		suiteConfig.SkipStrings = append(suiteConfig.SkipStrings, skipTests)
	}

	if junitReport := viper.GetString(config.Tests.JUnitReportPath); junitReport != "" {
		reporterConfig.JUnitReport = junitReport
	}

	if viper.GetBool(config.Tests.GinkgoDryRun) {
		suiteConfig.DryRun = true
	}

	reporterConfig.NoColor = true
	// Enable verbose test output for info and debug levels
	logLevel := viper.GetString(config.Log.Level)
	if logLevel == config.LogLevelDebug || logLevel == config.LogLevelInfo || logLevel == "" {
		reporterConfig.Verbose = true
	}
}
