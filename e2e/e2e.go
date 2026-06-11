package e2e

// Import test suites for auto-registration
import (
	_ "github.com/openshift-hyperfleet/hyperfleet-e2e/e2e/adapter"
	_ "github.com/openshift-hyperfleet/hyperfleet-e2e/e2e/channel"
	_ "github.com/openshift-hyperfleet/hyperfleet-e2e/e2e/cluster"
	_ "github.com/openshift-hyperfleet/hyperfleet-e2e/e2e/nodepool"
	_ "github.com/openshift-hyperfleet/hyperfleet-e2e/e2e/version"
	_ "github.com/openshift-hyperfleet/hyperfleet-e2e/e2e/wifconfig"
)
