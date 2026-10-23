# AGENTS.md — AI Agent Instructions for migtools/oadp-non-admin

## Project Overview
OADP Non-Admin Controller (NAC) enables non-admin users to perform backup and restore operations in OpenShift clusters through OADP. It provides a controller that translates non-admin user requests into Velero operations, enforcing namespace-scoped permissions and RBAC policies. Built with the Operator SDK using controller-runtime.

- **Primary Language**: Go
- **Module**: `github.com/migtools/oadp-non-admin`
- **Default Branch**: `oadp-dev`

## Build Instructions
```bash
# Build the controller binary (includes manifests, codegen, fmt, vet)
make build

# Build Docker image
make docker-build

# Push Docker image
make docker-push

# Build cross-platform image
make docker-buildx

# Run the controller locally against a cluster
make run
```

## Test Instructions
```bash
# Run all tests (includes manifests generation, codegen, fmt, vet, envtest)
make test

# Run specific tests
go test ./internal/controller/... -run TestName

# Format code
make fmt

# Vet code
make vet
```

## Linting
```bash
# Run golangci-lint
make lint

# Run golangci-lint with auto-fix
make lint-fix
```

Configuration: `.golangci.yml`

## Code Generation
```bash
# Generate CRD manifests (WebhookConfiguration, ClusterRole, CRDs)
make manifests

# Generate DeepCopy methods
make generate
```

## Code Conventions
- Operator SDK / controller-runtime patterns
- API types in `api/` with version directories
- Controllers in `internal/controller/`
- CRD and RBAC manifests in `config/`
- E2E tests in `test/`
- Documentation in `docs/`
- Use kubebuilder markers for RBAC and CRD generation
- Follow non-admin security patterns (namespace-scoped operations)

## Project Structure
```
api/           - NAC API types (NonAdminBackup, NonAdminRestore, etc.)
cmd/           - Controller entry point
config/        - Kubernetes manifests
  crd/         - CRD definitions
  rbac/        - RBAC rules
  manager/     - Deployment manifests
  samples/     - Example CR instances
docs/          - Documentation
hack/          - Development and CI scripts
internal/      - Private packages
  controller/  - Reconciler implementations
  handler/     - Event handlers
  predicate/   - Watch predicates
test/          - E2E and integration tests
```

## CI/CD
- GitHub Actions workflows in `.github/workflows/`:
  - `ci.yml` — CI pipeline (build, lint, test)
- Linter config: `.golangci.yml`
- Reproduce CI locally:
  ```bash
  make build
  make lint
  make test
  ```

## Common Tasks

### Adding a new API type
1. Define the type in `api/`
2. Run `make generate` to create DeepCopy methods
3. Run `make manifests` to generate CRD YAML
4. Create reconciler in `internal/controller/`
5. Add RBAC markers and regenerate: `make manifests`
6. Add tests

### Modifying RBAC permissions
1. Update kubebuilder RBAC markers in controller files
2. Run `make manifests` to regenerate RBAC YAML
3. Ensure namespace-scoped access patterns are maintained

### Building the installer manifest
```bash
make build-installer
```
This generates a consolidated YAML with CRDs and deployment.
