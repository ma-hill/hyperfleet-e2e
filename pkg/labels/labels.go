package labels

// Severity labels - Business Impact Dimension: Describes the severity of failure if the functionality is broken.
// See test-design/README.md for full classification criteria, decision flowchart, and examples.
const (
	Tier0 = "tier0" // Critical: Core user journey broken, fix immediately, blocks release
	Tier1 = "tier1" // Major: Important features affected, should be addressed
	Tier2 = "tier2" // Minor: Edge cases or low-frequency scenarios, can be deferred
)

// Stability labels - Test quality dimension: determines CI gate policy
// const (
// 	Stable    = "stable"    // Production-ready: stable and reliable, must pass to merge (Blocking)
// 	Informing = "informing" // Observation period: new test onboarding (Non-blocking)
// 	Flaky     = "flaky"     // Known unstable: quarantined for investigation
// )

// Scenario labels - Test path dimension: describes test design intent
const (
	// HappyPath = "happy-path" // Normal workflow: ideal path
	Negative    = "negative" // Error handling: edge cases and failure scenarios
	Performance = "perf"     // Performance: stress tests or large-scale resource scenarios
)

// Functionality labels - Feature category dimension: describes test coverage target
const (
	Upgrade = "upgrade" // Version compatibility: smooth upgrades
)

// Constraint labels - Execution constraint dimension: determines scheduling strategy
const (
	Disruptive = "disruptive" // Destructive testing: fault injection
	Slow       = "slow"       // Long-running: execution time exceeds 5-10 minutes
)
