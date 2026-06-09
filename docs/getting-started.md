# Getting Started

New to HyperFleet E2E? This guide will help you run your first test in 10 minutes.

## Prerequisites

- **Go 1.25+** - Required for building the framework
- **HyperFleet API access** - API endpoint URL
- **10 minutes** - Time to complete this guide

## Installation

### Clone and Build

```bash
git clone https://github.com/openshift-hyperfleet/hyperfleet-e2e.git
cd hyperfleet-e2e
make build
```

### Verify Installation

```bash
./bin/hyperfleet-e2e --help
```

You should see the command help output.

## Your First Test

**Step 1**: Set API URL

```bash
export HYPERFLEET_API_URL=https://api.hyperfleet.example.com
```

**Step 2**: Run tests

```bash
./bin/hyperfleet-e2e test --label-filter=tier0
```

**What happens**:
1. Framework creates a new cluster via API
2. Waits for cluster to reach Reconciled state
3. Validates adapter conditions
4. Deletes cluster after test completes

## What Just Happened?

The framework:

1. **Loaded configuration** - Merged config file, environment variables, and CLI flags
2. **Executed tests** - Ran all tests matching your filter
3. **Managed resources** - Created and deleted temporary test clusters
4. **Generated results** - Displayed test outcomes

## Running Specific Tests

```bash
# Run critical tests only
./bin/hyperfleet-e2e test --label-filter=tier0

# Run all cluster suite tests
./bin/hyperfleet-e2e test --focus "\[Suite: cluster\]"

# Run cluster tier0 tests only
./bin/hyperfleet-e2e test --label-filter="tier0" --focus "\[Suite: cluster\]"

# Deep debug mode (add API calls and framework internals)
./bin/hyperfleet-e2e test --log-level=debug
```

## Listing Tests Without Execution

Use `--dry-run` to discover which specs match your filters without connecting to the API or creating resources. No `--api-url` is required in dry-run mode.

```bash
# List all tier0 tests
./bin/hyperfleet-e2e test --dry-run --label-filter=tier0

# List tier1 cluster tests
./bin/hyperfleet-e2e test --dry-run --label-filter=tier1 --focus "\[Suite: cluster\]"

# List tests for each tier via Makefile
make list-tests
```

**Note**: The default output already shows detailed test execution steps. If a test fails, you can usually diagnose the issue from the logs without re-running in debug mode. Use `--log-level=debug` when you need to see API calls and framework internals. See [Debugging Guide](debugging.md) for more debugging techniques.

## Common Commands

```bash
make build       # Build binary
make test        # Run unit tests
make e2e         # Run E2E tests
make list-tests  # List tests by tier (dry-run, no API required)
make lint        # Run linter
make generate    # Regenerate OpenAPI client
```

## Troubleshooting

### Common Issues

**API connection errors**:
```bash
# Verify API URL
echo $HYPERFLEET_API_URL
curl -I $HYPERFLEET_API_URL
```

**Test timeouts**: Increase timeouts via environment variables:
```bash
HYPERFLEET_TIMEOUTS_CLUSTER_RECONCILED=45m make e2e
```

**Configuration not taking effect**:

Priority order (highest to lowest):
1. CLI flags (`--api-url`)
2. Environment variables (`HYPERFLEET_API_URL`)
3. Config file (`configs/config.yaml`)
4. Built-in defaults

**Need detailed logs**:
```bash
# Default (info) shows test execution steps
./bin/hyperfleet-e2e test

# Debug mode shows API calls and framework internals
./bin/hyperfleet-e2e test --log-level=debug
```

## Next Steps

- **[Architecture](architecture.md)** - Understand how the framework works
- **[Development](development.md)** - Write your own tests
- **CLI Reference** - Run `./bin/hyperfleet-e2e --help`
- **Configuration** - See detailed comments in `configs/config.yaml`
