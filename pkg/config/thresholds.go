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
// Values include ~1.5-2x buffer over measured baselines
// to account for variance across runs and environments.
const (
	ThresholdClusterCreateReconciled  = 20 * time.Second
	ThresholdClusterUpdateReconciled  = 30 * time.Second
	ThresholdClusterDeleted           = 60 * time.Second
	ThresholdClusterCascadeDeleted    = 60 * time.Second
	ThresholdNodePoolCreateReconciled = 20 * time.Second
	ThresholdNodePoolDeleted          = 30 * time.Second
)
