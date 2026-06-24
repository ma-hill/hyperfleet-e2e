# Contributing to HyperFleet E2E

Thank you for contributing to HyperFleet E2E! This document provides guidelines for developing and contributing to the project.

## Development Setup

### Prerequisites

- **Go 1.25+** - [Install Go](https://go.dev/doc/install)
- **Make** - Build automation tool
- **Container tool** - Docker or Podman for building images
- **Git** - Version control

### Initial Setup

1. Fork and clone the repository:
   ```bash
   git clone https://github.com/YOUR_USERNAME/hyperfleet-e2e.git
   cd hyperfleet-e2e
   ```

2. Add upstream remote:
   ```bash
   git remote add upstream https://github.com/openshift-hyperfleet/hyperfleet-e2e.git
   ```

3. Install dependencies and generate code:
   ```bash
   make generate
   ```

4. Build the binary:
   ```bash
   make build
   ```

5. Verify your setup:
   ```bash
   make check
   ```

## Repository Structure

```
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

## Testing

### Unit Tests

Run unit tests for all packages:

```bash
make test
```

Generate coverage report:

```bash
make test-coverage
```

View coverage in browser:

```bash
open coverage.html
```

### E2E Tests

Run all E2E tests (requires HyperFleet API):

```bash
export HYPERFLEET_API_URL=https://api.hyperfleet.example.com
make e2e
```

Run specific test suites:

```bash
./bin/hyperfleet-e2e test --label-filter=tier0
./bin/hyperfleet-e2e test --focus "\[Suite: cluster\]"
```

### Writing Tests

See [Development Guide](docs/development.md) for detailed instructions on writing E2E tests.

Key points:
- Test files use `.go` extension (NOT `_test.go`)
- All tests must have labels (Tier0, Tier1, or Tier2)
- Use `ginkgo.By()` to mark major test steps
- Clean up resources with `DeferCleanup`
- Use helper functions from `pkg/helper`

## Common Tasks

### Update API Client

When the HyperFleet API OpenAPI schema changes:

```bash
# 1. Bump the spec module version in go.mod
go get github.com/openshift-hyperfleet/hyperfleet-api-spec@vX.Y.Z

# 2. Regenerate client code from the new spec
make generate
```

### Build and Test Workflow

Standard development workflow:

```bash
# 1. Make changes to code
# 2. Format code
make fmt

# 3. Run all checks
make check

# 4. Build binary
make build

# 5. Run E2E tests locally (optional)
./bin/hyperfleet-e2e test --label-filter=tier0
```

### Build Container Image

Build for local testing:

```bash
make image
```

Build and push to personal Quay registry:

```bash
QUAY_USER=myusername make image-dev
```

### Add a New Test

1. Create test file in appropriate suite directory:
   ```bash
   touch e2e/cluster/my-new-test.go
   ```

2. Follow the test template structure (see existing tests)

3. Add appropriate labels

4. Test your changes:
   ```bash
   make build
   ./bin/hyperfleet-e2e test --focus "My New Test"
   ```

## Code Quality

### Formatting

Format all Go code:

```bash
make fmt
```

Check if code is formatted:

```bash
make fmt-check
```

### Linting

Run golangci-lint:

```bash
make lint
```

Run all verification checks:

```bash
make verify
```

### Pre-commit Checklist

Before committing, ensure:

- [ ] Code is formatted (`make fmt`)
- [ ] All checks pass (`make check`)
- [ ] New tests added for new functionality
- [ ] Documentation updated if needed

## Commit Standards

We follow [Conventional Commits](https://www.conventionalcommits.org/) standards:

```
<type>(<scope>): <subject>

<body>

<footer>
```

### Commit Types

- `feat`: New feature
- `fix`: Bug fix
- `docs`: Documentation changes
- `refactor`: Code refactoring
- `test`: Test changes
- `chore`: Build/tooling changes

### Examples

```
feat(cluster): add validation for cluster creation payloads

Add payload validation to ensure cluster names follow naming conventions
and required fields are present before sending API requests.

Closes #123
```

```
fix(client): handle timeout errors gracefully

Previously, timeout errors would panic. Now they're caught and wrapped
with context about the operation that timed out.
```

## Pull Request Process

1. Create a feature branch from `main`:
   ```bash
   git checkout -b feature/my-feature
   ```

2. Make your changes and commit following commit standards

3. Push to your fork:
   ```bash
   git push origin feature/my-feature
   ```

4. Open a pull request against `main`

5. Ensure CI checks pass

6. Address review feedback

7. Once approved, a maintainer will merge your PR

### PR Requirements

- [ ] All CI checks passing
- [ ] Code reviewed and approved
- [ ] Commit messages follow standards
- [ ] Documentation updated if needed
- [ ] Tests added for new functionality

## Release Process

Releases are managed by maintainers following semantic versioning (MAJOR.MINOR.PATCH).

### Version Scheme

- **MAJOR**: Breaking changes
- **MINOR**: New features (backwards compatible)
- **PATCH**: Bug fixes (backwards compatible)

### Release Steps

1. Update `CHANGELOG.md` with release notes
2. Create release branch: `release-X.Y`
3. Tag release: `vX.Y.Z`
4. Build and push container images
5. Create GitHub release with changelog

## Getting Help

- Read the [documentation](docs/)
- Check existing [issues](https://github.com/openshift-hyperfleet/hyperfleet-e2e/issues)
- Ask questions in pull requests or issues

## License

By contributing, you agree that your contributions will be licensed under the Apache License 2.0.
