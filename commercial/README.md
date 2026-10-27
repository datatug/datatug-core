# DataTug Commercial Subsystem

This directory contains the commercial subsystem, enterprise entitlement definitions, and proprietary extensions for the DataTug platform.

---

## 1. Governance & Licensing Scope

Unlike the community core (`pkg/`, `cmd/`, `internal/`, `web/`, `docs/`, `scripts/`, `spec/`), which is licensed under the Apache License 2.0, all code and documentation under `commercial/` are governed by the **DataTug Commercial Terms**.

For full license terms, refer to:
- [`LICENSES/COMMERCIAL-TERMS.md`](../LICENSES/COMMERCIAL-TERMS.md)
- [`LICENSING.md`](../LICENSING.md)

---

## 2. Architectural Boundary & Separation of Concerns

DataTug enforces strict architectural boundaries to preserve the integrity of the open-source community edition:

1. **No Inward Dependencies**:
   Core community packages (`pkg/`, `internal/`, `cmd/`) must **never** import proprietary commercial implementations.
2. **Contract-First Entitlement**:
   The `commercial/licensing` package exposes clean Go interfaces (e.g., `EntitlementVerifier`, `CapacityDescriptor`, `CoordinatorContract`). This allows core subsystems to define optional hooks or runtime extension points without bundling proprietary enterprise logic into the open-source binaries.
3. **Standalone / Community Operation**:
   When running in community mode, DataTug operates fully standalone with built-in default/no-op verifiers, requiring zero proprietary runtime dependencies or network callouts to commercial license servers.

---

## 3. Package Summary

- **`commercial/licensing/`**:
  Defines enterprise entitlement contracts, capacity descriptors, license validation abstractions, and coordinator interfaces.
