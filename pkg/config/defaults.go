package config

import "time"

// Log level constants
const (
	// LogLevelDebug enables detailed test steps and all framework internal logs
	LogLevelDebug = "debug"

	// LogLevelInfo enables detailed test steps and high-level framework logs (default)
	LogLevelInfo = "info"

	// LogLevelWarn shows only warnings and errors (minimal output for CI/CD)
	LogLevelWarn = "warn"

	// LogLevelError shows only errors (absolute minimal output)
	LogLevelError = "error"
)

// Log format constants
const (
	LogFormatJSON = "json"
	LogFormatText = "text"
)

// Log output constants
const (
	LogOutputStdout = "stdout"
	LogOutputStderr = "stderr"
)

// Default timeout values
const (
	// DefaultClusterReconciledTimeout is the default timeout for waiting for a cluster to become reconciled
	DefaultClusterReconciledTimeout = 2 * time.Minute

	// DefaultClusterDeletedTimeout is the default timeout for waiting for a cluster to be hard-deleted (404)
	DefaultClusterDeletedTimeout = 2 * time.Minute

	// DefaultNodePoolReconciledTimeout is the default timeout for waiting for a nodepool to become reconciled
	DefaultNodePoolReconciledTimeout = 5 * time.Minute

	// DefaultNodePoolDeletedTimeout is the default timeout for waiting for a nodepool to be hard-deleted (404)
	DefaultNodePoolDeletedTimeout = 2 * time.Minute

	// DefaultAdapterProcessingTimeout is the default timeout for waiting for adapter conditions
	DefaultAdapterProcessingTimeout = 5 * time.Minute

	// DefaultPollInterval is the default interval for polling operations
	DefaultPollInterval = 10 * time.Second

	// DefaultLogLevel is the default log level
	DefaultLogLevel = LogLevelInfo

	// DefaultLogFormat is the default log format
	DefaultLogFormat = LogFormatText

	// DefaultLogOutput is the default log output
	DefaultLogOutput = LogOutputStdout
)

// Default required adapters for resource types
var (
	// DefaultClusterAdapters is the default list of required adapters for cluster resources
	DefaultClusterAdapters = []string{
		"clusters-namespace",
		"clusters-job",
		"clusters-deployment",
	}

	// DefaultNodePoolAdapters is the default list of required adapters for nodepool resources
	DefaultNodePoolAdapters = []string{
		"nodepools-configmap",
	}
)
