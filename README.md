# HyperFleet E2E

Black-box end-to-end testing for validating the HyperFleet cluster lifecycle management.

[![Go Version](https://img.shields.io/badge/Go-1.25-00ADD8?style=flat&logo=go)](https://go.dev/)
[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)

## What is it?

HyperFleet E2E is a Ginkgo-based testing framework that validates HyperFleet cluster lifecycle management through black-box tests. It creates ephemeral test clusters for each test, providing complete isolation and supporting parallel execution.

## Quick Start
- **[Setup Guide](docs/setup.md)** - Setup environment to run e2e tests
- **[Getting Started](docs/getting-started.md)** - Getting started guide

## Running Tests

### Filter by Labels

```bash
# Critical severity tests (Release gate)
./bin/hyperfleet-e2e test --label-filter=tier0

# Exclude slow tests
./bin/hyperfleet-e2e test --label-filter="!slow"
```

### Common Options

```bash
# Debug mode (API calls and framework internals)
./bin/hyperfleet-e2e test --log-level=debug

# Run specific test suite
./bin/hyperfleet-e2e test --focus "\[Suite: cluster\]"
```

Run `./bin/hyperfleet-e2e test --help` for all options.

### Performance Tests

Performance tests are labeled `perf` and measure baseline latencies for core operations. They run inside the cluster for production-representative numbers.

See [perf/README.md](perf/README.md).

## Configuration

Configuration priority (highest to lowest):
1. CLI flags (`--api-url`)
2. Environment variables (`HYPERFLEET_API_URL`)
3. Config file (`configs/config.yaml`)
4. Built-in defaults

**Example**:
```bash
export HYPERFLEET_API_URL=https://staging.api.example.com
./bin/hyperfleet-e2e test
```

See `configs/config.yaml` for all configuration options with detailed comments.

## Project Structure

```text
hyperfleet-e2e/
├── cmd/              - CLI entry point
│   └── hyperfleet-e2e/
├── pkg/              - Core packages
│   ├── api/          - OpenAPI generated client
│   ├── client/       - HyperFleet API client wrapper
│   ├── config/       - Configuration loading and validation
│   ├── e2e/          - Test execution engine (Ginkgo)
│   ├── helper/       - Test helper utilities
│   ├── labels/       - Test label definitions
│   └── logger/       - Structured logging (slog)
├── e2e/              - Test suites
│   ├── adapter/      - Adapter lifecycle tests
│   ├── channel/      - Channel management tests
│   ├── cluster/      - Cluster lifecycle tests
│   ├── nodepool/     - NodePool management tests
│   └── version/      - Version management tests
├── testdata/         - Test payloads and fixtures
│   ├── adapter-configs/ - Adapter configuration files
│   └── payloads/
│       ├── clusters/ - Cluster creation payloads
│       └── nodepools/ - NodePool creation payloads
├── test-design/      - Test design documentation
│   ├── templates/    - Test case templates
│   ├── testcases/    - Test case documents
│   └── user-journeys/ - User journey maps
├── configs/          - Configuration files
│   └── config.yaml   - Default configuration
├── docs/             - Documentation
├── env/              - Environment configuration files
├── hack/             - Build and development scripts
├── images/           - Container image definitions
├── openapi/          - OpenAPI spec and generation config
└── scripts/          - Utility scripts
```

## Key Features

- **Ephemeral Resources**: Each test creates and cleans up its own cluster for complete isolation
- **Payload Templates**: Dynamic resource naming with template variables to prevent naming conflicts
- **Flexible Filtering**: Run tests by labels, focus patterns, or skip patterns
- **Comprehensive Validation**: Verifies cluster phases, adapter conditions, and health status
- **Structured Logging**: JSON or text logging with configurable levels
- **Parallel Execution**: Full isolation enables safe parallel test runs

## Documentation

- **[System Architecture](https://github.com/openshift-hyperfleet/architecture)** - Single source of truth for all HyperFleet architectural documentation
- **[Setup Guide](docs/setup.md)** - Setup environment to run e2e tests
- **[Runbook](docs/runbook.md)** - E2E test runbook
- **[Getting Started](docs/getting-started.md)** - Run your first test in 10 minutes
- **[Framework Architecture](docs/architecture.md)** - Understand the framework design
- **[Development](docs/development.md)** - Write new tests
- **[Debugging](docs/debugging.md)** - Diagnose E2E test failures
- **[HyperFleet Architecture](https://github.com/openshift-hyperfleet/hyperfleet/tree/main/docs)** - Project-wide architecture and standards

## CI/CD Integration

```bash
# Set API URL
export HYPERFLEET_API_URL=$CI_API_URL

# Run critical severity tests with JUnit output
./bin/hyperfleet-e2e test \
  --label-filter=tier0 \
  --junit-report=results.xml \
  --log-format=json
```

### Container Usage
```bash
make image

podman run --rm \
  -e HYPERFLEET_API_URL=https://api.example.com \
  quay.io/openshift-hyperfleet/hyperfleet-e2e:latest \
  test --label-filter=tier0
```

## Development

```bash
make build      # Build binary
make test       # Run unit tests
make lint       # Run linter
make check      # Run all checks
```

See [Development Guide](docs/development.md) for adding new tests.

## Contributing

We welcome contributions! Please see [CONTRIBUTING.md](CONTRIBUTING.md) for development setup, testing guidelines, and contribution workflow.

Quick start:
1. Fork the repository
2. Create a feature branch
3. Make your changes and add tests
4. Run `make check`
5. Submit a pull request

See [OWNERS](OWNERS) for approval requirements.

## Support

- 📖 [Documentation](docs/)
- 🐛 [Report Issues](https://github.com/openshift-hyperfleet/hyperfleet-e2e/issues)

## License

Apache License 2.0 - See [LICENSE](LICENSE) for details.
