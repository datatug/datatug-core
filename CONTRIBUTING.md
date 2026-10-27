# Contributing to DataTug

Thank you for your interest in contributing to DataTug! We welcome contributions from the community to help make data modeling, exploration, and schema management open and accessible.

---

## Code of Conduct

All contributors and community participants are expected to adhere to our [Code of Conduct](CODE_OF_CONDUCT.md). Please read it to understand the behavioral standards expected across all communication channels.

---

## Licensing of Contributions

DataTug is an open-source project licensed under the [Apache License 2.0](LICENSES/Apache-2.0.txt) with commercial extensions.

By submitting a pull request, you agree that your contributions will be licensed under the Apache License 2.0 without additional restrictions, in accordance with Section 5 of the Apache 2.0 License.

We follow the **Developer Certificate of Origin (DCO)** standard. When creating commits, please sign off using `git commit -s`:

```
Signed-off-by: Jane Doe <jane.doe@example.com>
```

---

## Development Setup

### Prerequisites
- **Go**: Version 1.22 or newer (or latest stable)
- **Git**
- Optional: `golangci-lint` for static code analysis

### Repository Clones & Worktrees
Follow local development guidelines:
```bash
git clone https://github.com/datatug/datatug-core.git
cd datatug-core
```

### Building and Testing
Run all tests across all packages:
```bash
go test -v ./...
```

Run tests with race detection:
```bash
go test -race ./pkg/...
```

Run static analysis:
```bash
go vet ./...
```

---

## Architectural Guidelines

DataTug maintains a clean separation between its open-source core and enterprise extensions under our **Model A mixed licensing framework**:
1. **Community Packages**: `pkg/`, `cmd/`, `internal/`, `web/`, `scripts/`, `docs/`, and `spec/` are fully open-source under Apache 2.0.
2. **Commercial Interfaces**: `commercial/licensing/` provides entitlement contracts. Community modules must **never** import proprietary commercial implementations.
3. **No Direct OS/Symlink Bypasses**: Path resolution code (such as in `pkg/meaninggraph`) strictly validates boundaries against symlink escapes. Ensure any filesystem operations retain this security posture.

---

## Pull Request Process

1. **Open an Issue First**: For non-trivial features or architectural changes, please open an issue to discuss design before implementing.
2. **Branch Naming**: Use descriptive branch names (e.g., `feature/custom-comparator`, `fix/meaninggraph-path-resolution`).
3. **Tests Required**: Every bug fix or new feature must include comprehensive unit tests.
4. **Clean Commits**: Write clear, descriptive commit messages summarizing the intent and changes.
5. **Review**: Maintainers will review your PR. Be prepared to address feedback and iterate.

Thank you for helping build DataTug!
