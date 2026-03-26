package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/spf13/viper"
)

const (
	// TagMapstructure is the struct tag used by Viper for configuration mapping
	TagMapstructure = "mapstructure"

	// EnvPrefix is the prefix for all environment variables (without trailing underscore)
	// Viper automatically adds underscore when using SetEnvPrefix()
	EnvPrefix = "HYPERFLEET"

	// RedactedPlaceholder is used to mask sensitive information in logs
	RedactedPlaceholder = "**REDACTED**"

	// NotSetPlaceholder indicates a configuration value has not been set
	NotSetPlaceholder = "<not set>"
)

// EnvVar constructs an environment variable name with the HYPERFLEET prefix
// Example: EnvVar("LOG_LEVEL") returns "HYPERFLEET_LOG_LEVEL"
func EnvVar(name string) string {
	return EnvPrefix + "_" + name
}

// API config keys
var API = struct {
	// URL is the HyperFleet API base URL
	// Env: HYPERFLEET_API_URL
	URL string
}{
	URL: "api.url",
}

// Tests config keys for Ginkgo test execution
var Tests = struct {
	// GinkgoLabelFilter is the label filter for Ginkgo tests
	// Env: GINKGO_LABEL_FILTER
	GinkgoLabelFilter string

	// GinkgoFocus is a regex to focus on specific tests
	// Env: GINKGO_FOCUS
	GinkgoFocus string

	// GinkgoSkip is a regex to skip specific tests
	// Env: GINKGO_SKIP
	GinkgoSkip string

	// SuiteTimeout is the timeout for the entire test suite (Go duration format: "2h", "90m", etc.)
	// Env: SUITE_TIMEOUT
	SuiteTimeout string

	// JUnitReportPath is the path to write JUnit XML report
	// Env: JUNIT_REPORT_PATH
	JUnitReportPath string
}{
	GinkgoLabelFilter: "tests.ginkgoLabelFilter",
	GinkgoFocus:       "tests.focus",
	GinkgoSkip:        "tests.ginkgoSkip",
	SuiteTimeout:      "tests.suiteTimeout",
	JUnitReportPath:   "tests.junitReportPath",
}

// Log config keys
var Log = struct {
	// Level is the minimum log level
	// Env: HYPERFLEET_LOG_LEVEL
	Level string

	// Format is the log output format
	// Env: HYPERFLEET_LOG_FORMAT
	Format string

	// Output is the log destination
	// Env: HYPERFLEET_LOG_OUTPUT
	Output string
}{
	Level:  "log.level",
	Format: "log.format",
	Output: "log.output",
}

// AdaptersConfig contains required adapters for each resource type
type AdaptersConfig struct {
	Cluster  []string `yaml:"cluster" mapstructure:"cluster"`   // Required adapters for cluster resources
	NodePool []string `yaml:"nodepool" mapstructure:"nodepool"` // Required adapters for nodepool resources
}

// AdapterDeploymentConfig contains configuration for deploying adapters via Helm in tests.
type AdapterDeploymentConfig struct {
	ChartRepo     string `yaml:"chartRepo" mapstructure:"chartRepo"`
	ChartRef      string `yaml:"chartRef" mapstructure:"chartRef"`
	ChartPath     string `yaml:"chartPath" mapstructure:"chartPath"`
	ImageRegistry string `yaml:"imageRegistry" mapstructure:"imageRegistry"`
	ImageRepo     string `yaml:"imageRepo" mapstructure:"imageRepo"`
	ImageTag      string `yaml:"imageTag" mapstructure:"imageTag"`
}

// Config represents the e2e test configuration
type Config struct {
	Namespace         string                  `yaml:"namespace" mapstructure:"namespace"`
	GCPProjectID      string                  `yaml:"gcpProjectId" mapstructure:"gcpProjectId"`
	OutputDir         string                  `yaml:"outputDir" mapstructure:"outputDir"`
	TestDataDir       string                  `yaml:"testDataDir" mapstructure:"testDataDir"`
	API               APIConfig               `yaml:"api" mapstructure:"api"`
	Timeouts          TimeoutsConfig          `yaml:"timeouts" mapstructure:"timeouts"`
	Polling           PollingConfig           `yaml:"polling" mapstructure:"polling"`
	Log               LogConfig               `yaml:"log" mapstructure:"log"`
	Adapters          AdaptersConfig          `yaml:"adapters" mapstructure:"adapters"`
	AdapterDeployment AdapterDeploymentConfig `yaml:"adapterDeployment" mapstructure:"adapterDeployment"`
}

// APIConfig contains API-related configuration
type APIConfig struct {
	URL string `yaml:"url" mapstructure:"url"`
}

// TimeoutsConfig contains timeout configurations
type TimeoutsConfig struct {
	Cluster  ClusterTimeouts  `yaml:"cluster" mapstructure:"cluster"`
	NodePool NodePoolTimeouts `yaml:"nodepool" mapstructure:"nodepool"`
	Adapter  AdapterTimeouts  `yaml:"adapter" mapstructure:"adapter"`
}

// ClusterTimeouts contains cluster-related timeouts
type ClusterTimeouts struct {
	Ready time.Duration `yaml:"ready" mapstructure:"ready"`
}

// NodePoolTimeouts contains nodepool-related timeouts
type NodePoolTimeouts struct {
	Ready time.Duration `yaml:"ready" mapstructure:"ready"`
}

// AdapterTimeouts contains adapter-related timeouts
type AdapterTimeouts struct {
	Processing time.Duration `yaml:"processing" mapstructure:"processing"`
}

// PollingConfig contains polling configuration
type PollingConfig struct {
	Interval time.Duration `yaml:"interval" mapstructure:"interval"`
}

// LogConfig contains logging configuration
type LogConfig struct {
	Level  string `yaml:"level" mapstructure:"level"`   // debug, info, warn, error
	Format string `yaml:"format" mapstructure:"format"` // text, json
	Output string `yaml:"output" mapstructure:"output"` // stdout, stderr
}

// Load loads configuration from viper with improved validation
func Load() (*Config, error) {
	cfg := &Config{}

	// Use Unmarshal (not UnmarshalExact) to allow runtime test parameters (tests.*)
	// to coexist with persistent configuration. Test parameters (label-filter, focus, skip)
	// are set via flags/env vars and should not appear in config files.
	if err := viper.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("configuration error: %w\nPlease check your config file", err)
	}

	// WORKAROUND: viper.Unmarshal doesn't always respect env var bindings for nested structs
	// Use reflection to automatically apply all values from viper to the config struct
	applyViperValues(reflect.ValueOf(cfg).Elem(), "")

	// Apply defaults
	cfg.applyDefaults()

	// Validate with detailed errors
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	// Note: Display() is called after logger initialization in e2e.RunTests()
	// to ensure structured logging is properly configured

	return cfg, nil
}

// applyViperValues recursively applies values from viper to the config struct using reflection
// This ensures environment variables and flags properly override config file values
func applyViperValues(v reflect.Value, prefix string) {
	t := v.Type()

	for i := 0; i < v.NumField(); i++ {
		field := v.Field(i)
		fieldType := t.Field(i)

		tag := fieldType.Tag.Get(TagMapstructure)
		if tag == "" {
			continue
		}

		var configPath string
		if prefix == "" {
			configPath = tag
		} else {
			configPath = prefix + "." + tag
		}

		if field.Kind() == reflect.Struct && field.Type() != reflect.TypeOf(time.Duration(0)) {
			applyViperValues(field, configPath)
			continue
		}

		if !field.CanSet() {
			continue
		}

		// Apply value from viper based on field type
		switch field.Kind() {
		case reflect.String:
			if viperVal := viper.GetString(configPath); viperVal != "" {
				field.SetString(viperVal)
			}
		case reflect.Bool:
			// For bool, only apply if the key is explicitly set in viper
			// This preserves the priority order: flags > env > config > defaults
			if viper.IsSet(configPath) {
				field.SetBool(viper.GetBool(configPath))
			}
		case reflect.Slice:
			// Handle string slices
			if field.Type().Elem().Kind() == reflect.String {
				if viper.IsSet(configPath) {
					viperVal := viper.GetStringSlice(configPath)
					if len(viperVal) == 1 && strings.Contains(viperVal[0], ",") {
						viperVal = strings.Split(viperVal[0], ",")
						// Trim whitespace from each element
						for i := range viperVal {
							viperVal[i] = strings.TrimSpace(viperVal[i])
						}
					}
					if len(viperVal) > 0 {
						field.Set(reflect.ValueOf(viperVal))
					}
				}
			}
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			// Special handling for time.Duration (which is int64)
			if field.Type() == reflect.TypeOf(time.Duration(0)) {
				if viperVal := viper.GetDuration(configPath); viperVal != 0 {
					field.SetInt(int64(viperVal))
				}
			} else {
				if viperVal := viper.GetInt64(configPath); viperVal != 0 {
					field.SetInt(viperVal)
				}
			}
		case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
			if viperVal := viper.GetUint64(configPath); viperVal != 0 {
				field.SetUint(viperVal)
			}
		case reflect.Float32, reflect.Float64:
			if viperVal := viper.GetFloat64(configPath); viperVal != 0 {
				field.SetFloat(viperVal)
			}
		}
	}
}

// applyDefaults applies default values for unset fields
func (c *Config) applyDefaults() {
	// Apply timeout defaults
	if c.Timeouts.Cluster.Ready == 0 {
		c.Timeouts.Cluster.Ready = DefaultClusterReadyTimeout
	}
	if c.Timeouts.NodePool.Ready == 0 {
		c.Timeouts.NodePool.Ready = DefaultNodePoolReadyTimeout
	}
	if c.Timeouts.Adapter.Processing == 0 {
		c.Timeouts.Adapter.Processing = DefaultAdapterProcessingTimeout
	}
	if c.Polling.Interval == 0 {
		c.Polling.Interval = DefaultPollInterval
	}

	// Apply log defaults
	if c.Log.Level == "" {
		c.Log.Level = DefaultLogLevel
	}
	if c.Log.Format == "" {
		c.Log.Format = DefaultLogFormat
	}
	if c.Log.Output == "" {
		c.Log.Output = DefaultLogOutput
	}

	// Apply adapter defaults
	if c.Adapters.Cluster == nil {
		c.Adapters.Cluster = DefaultClusterAdapters
	}
	if c.Adapters.NodePool == nil {
		c.Adapters.NodePool = DefaultNodePoolAdapters
	}

	// Apply general configuration defaults from environment variables or config file
	// Priority: config file values > environment variables > empty
	// If config file value is empty, fall back to environment variable

	// Namespace: from config file or NAMESPACE env var
	if c.Namespace == "" {
		c.Namespace = os.Getenv("NAMESPACE")
	}

	// GCPProjectID: from config file or GCP_PROJECT_ID env var
	if c.GCPProjectID == "" {
		c.GCPProjectID = os.Getenv("GCP_PROJECT_ID")
	}

	// OutputDir: from config file, OUTPUT_DIR env var, or default to "output"
	if c.OutputDir == "" {
		if envVal := os.Getenv("OUTPUT_DIR"); envVal != "" {
			c.OutputDir = envVal
		} else {
			c.OutputDir = "output"
		}
	}

	// TestDataDir: from config file, TESTDATA_DIR env var, or default to "testdata"
	if c.TestDataDir == "" {
		if envVal := os.Getenv("TESTDATA_DIR"); envVal != "" {
			c.TestDataDir = envVal
		} else {
			c.TestDataDir = "testdata"
		}
	}

	// Apply adapter deployment values from environment variables or config file

	// ChartRepo: from ADAPTER_CHART_REPO env var or config file
	if c.AdapterDeployment.ChartRepo == "" {
		c.AdapterDeployment.ChartRepo = os.Getenv("ADAPTER_CHART_REPO")
	}

	// ChartRef: from ADAPTER_CHART_REF env var or config file
	if c.AdapterDeployment.ChartRef == "" {
		c.AdapterDeployment.ChartRef = os.Getenv("ADAPTER_CHART_REF")
	}

	// ChartPath: from ADAPTER_CHART_PATH env var or config file
	if c.AdapterDeployment.ChartPath == "" {
		c.AdapterDeployment.ChartPath = os.Getenv("ADAPTER_CHART_PATH")
	}

	// ImageRegistry: from config file or IMAGE_REGISTRY env var
	// If not set, envsubst will use the env var directly (if set in shell)
	// If still not set, values.yaml defaults will be used
	if c.AdapterDeployment.ImageRegistry == "" {
		c.AdapterDeployment.ImageRegistry = os.Getenv("IMAGE_REGISTRY")
	}

	// ImageRepo: from config file or ADAPTER_IMAGE_REPO env var
	if c.AdapterDeployment.ImageRepo == "" {
		c.AdapterDeployment.ImageRepo = os.Getenv("ADAPTER_IMAGE_REPO")
	}

	// ImageTag: from config file or ADAPTER_IMAGE_TAG env var
	if c.AdapterDeployment.ImageTag == "" {
		c.AdapterDeployment.ImageTag = os.Getenv("ADAPTER_IMAGE_TAG")
	}
}

// Validate validates configuration with detailed error messages
func (c *Config) Validate() error {
	// Validate API URL requirement
	if c.API.URL == "" {
		return fmt.Errorf(`configuration validation failed:
  - Field 'Config.API.URL' is required
    Please provide API URL (in order of priority):
      • Flag: --api-url
      • Environment variable: HYPERFLEET_API_URL
      • Config file: api.url: <url>`)
	}

	return nil
}

// Display logs the merged configuration using structured logging
func (c *Config) Display() {
	slog.Info("Loaded configuration",
		"api_url", redactURL(c.API.URL),
		"namespace", c.Namespace,
		"gcp_project_id", c.GCPProjectID,
		"output_dir", c.OutputDir,
		"testdata_dir", c.TestDataDir,
		"timeout_cluster_ready", c.Timeouts.Cluster.Ready,
		"timeout_nodepool_ready", c.Timeouts.NodePool.Ready,
		"timeout_adapter_processing", c.Timeouts.Adapter.Processing,
		"polling_interval", c.Polling.Interval,
		"log_level", c.Log.Level,
		"log_format", c.Log.Format,
		"log_output", c.Log.Output,
		"adapters_cluster", c.Adapters.Cluster,
		"adapters_nodepool", c.Adapters.NodePool,
		"adapter_chart_repo", redactURL(c.AdapterDeployment.ChartRepo),
		"adapter_chart_ref", valueOrNotSet(c.AdapterDeployment.ChartRef),
		"adapter_chart_path", valueOrNotSet(c.AdapterDeployment.ChartPath),
		"adapter_image_registry", valueOrNotSet(c.AdapterDeployment.ImageRegistry),
		"adapter_image_repo", valueOrNotSet(c.AdapterDeployment.ImageRepo),
		"adapter_image_tag", valueOrNotSet(c.AdapterDeployment.ImageTag),
	)
}

// valueOrNotSet returns the value if non-empty, otherwise returns NotSetPlaceholder
func valueOrNotSet(value string) string {
	if value == "" {
		return NotSetPlaceholder
	}
	return value
}

// redactURL redacts credentials from URLs
func redactURL(rawURL string) string {
	if rawURL == "" {
		return NotSetPlaceholder
	}

	// Parse the URL to safely handle credentials
	u, err := url.Parse(rawURL)
	if err != nil {
		// If parsing fails, redact entirely for safety
		return RedactedPlaceholder
	}

	// If URL contains user credentials, redact them
	if u.User != nil {
		// Clear the User field and manually build the redacted URL
		u.User = nil
		redactedURL := u.String()

		// Insert RedactedPlaceholder after the scheme://
		if u.Scheme != "" {
			redactedURL = u.Scheme + "://" + RedactedPlaceholder + "@" + u.Host
			if u.Path != "" {
				redactedURL += u.Path
			}
			if u.RawQuery != "" {
				redactedURL += "?" + u.RawQuery
			}
			if u.Fragment != "" {
				redactedURL += "#" + u.Fragment
			}
		}
		return redactedURL
	}

	// Return the URL as-is if no credentials present
	return u.String()
}
