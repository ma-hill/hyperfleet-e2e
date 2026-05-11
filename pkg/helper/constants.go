package helper

const (
	// AdapterConfigsDir is the directory for adapter configurations under testdata
	AdapterConfigsDir = "adapter-configs"
	// TestWorkDir is the working directory for test execution and temporary test files
	TestWorkDir = ".test-work"
)

const (
	ResourceTypeClusters  = "clusters"
	ResourceTypeNodepools = "nodepools"

	SentinelClustersRelease  = "sentinel-clusters"
	SentinelNodePoolsRelease = "sentinel-nodepools"

	defaultGCPProjectID = "hcm-hyperfleet"
)
