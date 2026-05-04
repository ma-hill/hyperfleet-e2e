package client

// Hyperfleet keys used for resource identification and filtering in labels/annotations
const (
	KeyClusterID  = "hyperfleet.io/cluster-id"
	KeyAdapter    = "hyperfleet.io/adapter"
	KeyManagedBy  = "hyperfleet.io/managed-by"
	KeyGeneration = "hyperfleet.io/generation"
)

// Condition types used by adapters
const (
	ConditionTypeApplied   = "Applied"   // Resources created successfully
	ConditionTypeAvailable = "Available" // Work completed successfully
	ConditionTypeHealth    = "Health"    // No unexpected errors
)

// Condition types used by cluster-level resources (clusters, nodepools)
const (
	ConditionTypeReconciled = "Reconciled" // Resource is reconciled
)
