package config

import "time"

// Performance thresholds for API read operations.
// All API reads share the same threshold since payload size
// does not meaningfully impact latency at current scales.
const (
	ThresholdAPIRead = 50 * time.Millisecond
	ThresholdAPIList = 50 * time.Millisecond
)

// Performance thresholds for reconciliation operations.
// Calibrated from Prow tier1-nightly baselines (hyperfleet-dev-prow)
// with ~50% headroom to absorb run-to-run variance.
const (
	ThresholdClusterCreateReconciled  = 90 * time.Second // baseline ~60s
	ThresholdClusterUpdateReconciled  = 60 * time.Second // baseline ~40s
	ThresholdClusterDeleted           = 60 * time.Second // baseline ~40s
	ThresholdClusterCascadeDeleted    = 75 * time.Second // baseline ~50s
	ThresholdNodePoolCreateReconciled = 30 * time.Second // baseline ~20s
	ThresholdNodePoolDeleted          = 30 * time.Second // baseline ~20s
)
