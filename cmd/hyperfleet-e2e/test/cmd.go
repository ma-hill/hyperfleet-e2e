package test

import (
	"log"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/openshift-hyperfleet/hyperfleet-e2e/cmd/hyperfleet-e2e/common"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/e2e"

	// Import test registry (which imports all test suites)
	_ "github.com/openshift-hyperfleet/hyperfleet-e2e/e2e"
)

var Cmd = &cobra.Command{
	Use:   "test",
	Short: "Run end to end tests",
	Long:  "Run end to end tests on HyperFleet System.",
	Args:  cobra.OnlyValidArgs,
	Run:   run,
}

var args struct {
	labelFilter   string
	focusTests    string
	skipTests     string
	junitReport   string
	dryRun        bool
	flakeAttempts int
}

func init() {
	pfs := Cmd.PersistentFlags()

	// Test control flags
	pfs.StringVar(&args.labelFilter, "label-filter", "",
		"Ginkgo label filter expression")
	pfs.StringVar(&args.focusTests, "focus", "",
		"Only run tests matching this regex")
	pfs.StringVar(&args.skipTests, "skip", "",
		"Skip tests matching this regex")
	pfs.StringVar(&args.junitReport, "junit-report", "",
		"Path to write JUnit XML report")
	pfs.BoolVar(&args.dryRun, "dry-run", false,
		"List matching specs without executing them")
	pfs.IntVar(&args.flakeAttempts, "flake-attempts", 1,
		"Number of attempts for flaky tests (1 = no retries, 3 = up to 2 retries)")
}

func run(cmd *cobra.Command, argv []string) {
	if err := common.LoadConfig(common.ConfigFile); err != nil {
		log.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Bind flags after config loading (osde2e pattern)
	pfs := cmd.Flags()
	_ = viper.BindPFlag(config.Tests.GinkgoLabelFilter, pfs.Lookup("label-filter"))
	_ = viper.BindPFlag(config.Tests.GinkgoFocus, pfs.Lookup("focus"))
	_ = viper.BindPFlag(config.Tests.GinkgoSkip, pfs.Lookup("skip"))
	_ = viper.BindPFlag(config.Tests.JUnitReportPath, pfs.Lookup("junit-report"))
	_ = viper.BindPFlag(config.Tests.GinkgoDryRun, pfs.Lookup("dry-run"))
	_ = viper.BindPFlag(config.Tests.FlakeAttempts, pfs.Lookup("flake-attempts"))

	// Bind parent command flags (api-url, logging flags)
	parentFlags := cmd.Parent().PersistentFlags()
	_ = viper.BindPFlag(config.API.URL, parentFlags.Lookup("api-url"))
	_ = viper.BindPFlag(config.Log.Level, parentFlags.Lookup("log-level"))
	_ = viper.BindPFlag(config.Log.Format, parentFlags.Lookup("log-format"))
	_ = viper.BindPFlag(config.Log.Output, parentFlags.Lookup("log-output"))

	// Bind test environment variables
	_ = viper.BindEnv(config.Tests.GinkgoLabelFilter, "GINKGO_LABEL_FILTER")
	_ = viper.BindEnv(config.Tests.GinkgoFocus, "GINKGO_FOCUS")
	_ = viper.BindEnv(config.Tests.GinkgoSkip, "GINKGO_SKIP")
	_ = viper.BindEnv(config.Tests.JUnitReportPath, "JUNIT_REPORT_PATH")
	_ = viper.BindEnv(config.Tests.SuiteTimeout, "SUITE_TIMEOUT")
	_ = viper.BindEnv(config.Tests.GinkgoDryRun, "GINKGO_DRY_RUN")
	_ = viper.BindEnv(config.Tests.FlakeAttempts, "FLAKE_ATTEMPTS")

	// Load and validate config (fast failure before entering Ginkgo)
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Configuration validation failed: %v\n", err)
		os.Exit(1)
	}

	e2e.SetSuiteConfig(cfg)

	exitCode := e2e.RunTests(cmd.Context())
	os.Exit(exitCode)
}
