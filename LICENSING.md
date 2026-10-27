# DataTug Licensing Architecture

DataTug is released under a **Model A mixed licensing framework**, combining an open, permissive Apache 2.0 open-source core with commercial enterprise extensions.

This document serves as the authoritative specification of licensing scopes and directory boundaries across the DataTug codebase.

---

## 1. Directory License Scopes

| Directory / Path | Applicable License | Governing License File | Description |
| :--- | :--- | :--- | :--- |
| `pkg/` | **Apache License 2.0** | [`LICENSES/Apache-2.0.txt`](LICENSES/Apache-2.0.txt) | Core library modules, meaning graph resolvers, schemas, comparators, storage drivers. |
| `cmd/` | **Apache License 2.0** | [`LICENSES/Apache-2.0.txt`](LICENSES/Apache-2.0.txt) | Command-line utilities, developer tools, and WebAssembly builds. |
| `internal/` | **Apache License 2.0** | [`LICENSES/Apache-2.0.txt`](LICENSES/Apache-2.0.txt) | Internal utilities and shared helpers supporting community modules. |
| `web/` | **Apache License 2.0** | [`LICENSES/Apache-2.0.txt`](LICENSES/Apache-2.0.txt) | Web interfaces, browser components, and UI assets. |
| `scripts/` | **Apache License 2.0** | [`LICENSES/Apache-2.0.txt`](LICENSES/Apache-2.0.txt) | Build, deployment, and testing automation scripts. |
| `docs/` | **Apache License 2.0** | [`LICENSES/Apache-2.0.txt`](LICENSES/Apache-2.0.txt) | Architectural and user documentation. |
| `spec/` | **Apache License 2.0** | [`LICENSES/Apache-2.0.txt`](LICENSES/Apache-2.0.txt) | Format specifications, metadata contracts, and schema definitions. |
| `commercial/` | **DataTug Commercial Terms** | [`LICENSES/COMMERCIAL-TERMS.md`](LICENSES/COMMERCIAL-TERMS.md) | Enterprise entitlement interfaces, fleet coordinators, and commercial features. |

---

## 2. Community Open Source Core (Apache 2.0)

All foundational packages within `pkg/`, `cmd/`, `internal/`, `web/`, `scripts/`, `docs/`, and `spec/` are licensed under the [Apache License, Version 2.0](LICENSES/Apache-2.0.txt).

### Rights Granted Under Apache 2.0:
- **Commercial and Non-Commercial Use**: You may freely use, modify, integrate, and distribute DataTug open-source packages in personal, academic, or commercial projects.
- **Patent Grant**: Contributors provide an explicit grant of patent rights under Section 3 of the Apache 2.0 license.
- **Modification and Redistribution**: You may create derivative works and redistribute them under terms of your choosing, provided you comply with Section 4 notice and attribution requirements.

### Copyright Notice:
```
Copyright 2020-2026 DataTug Authors and Contributors (original work Copyright 2020 sneat-team)
```

---

## 3. Commercial Extensions (`commercial/`)

Directories and packages within `commercial/` are governed by the [DataTug Commercial Terms](LICENSES/COMMERCIAL-TERMS.md).

### Key Commercial Boundaries:
- **Cleanroom Interfaces**: Community code in `pkg/` or `cmd/` never depends on proprietary runtime implementations. The abstractions in `commercial/licensing/` define enterprise contracts (`EntitlementVerifier`, `CapacityDescriptor`, `CoordinatorContract`) enabling clean architectural separation.
- **Enterprise Capabilities**: Features such as multi-tenant cluster management, organizational identity federation (SAML/OIDC/RBAC), centralized audit trails, and hosted coordination engines are delivered under commercial subscriptions.
- **Self-Hosted & Managed Operations**: Usage of proprietary modules in organizational deployments or managed hosted environments requires an active commercial agreement or written entitlement.

---

## 4. Contributing

Contributions to Apache 2.0 directories are licensed under Apache 2.0 in accordance with Section 5 of the license. Please refer to [`CONTRIBUTING.md`](CONTRIBUTING.md) for contribution guidelines and Developer Certificate of Origin (DCO) requirements.
