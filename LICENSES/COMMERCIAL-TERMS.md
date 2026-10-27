# DataTug Commercial License Terms

> **DRAFT — PENDING QUALIFIED LEGAL REVIEW**
>
> *Notice: This document is a preliminary draft intended to outline commercial licensing terms and boundaries for enterprise, organizational self-hosted deployments, and managed services. It is subject to formal review, revision, and approval by legal counsel prior to final execution.*

---

## 1. Overview and Model A Architecture

DataTug operates under an open core licensing model ("Model A"):

1. **Community Scope (Open Source)**:
   The open-source core—comprising core data modeling, schema definition, local storage drivers, CLI tooling, meaning graph resolution, and developer utilities—is distributed under the permissive Apache License, Version 2.0. See [`LICENSING.md`](../LICENSING.md) and [`LICENSES/Apache-2.0.txt`](Apache-2.0.txt).

2. **Commercial Scope (Proprietary)**:
   Enterprise-grade coordination, multi-tenant fleet orchestration, centralized identity governance (RBAC/ABAC/SAML/OIDC), audit compliance logging, managed coordination servers, and proprietary integrations (located under `commercial/` and associated repositories) are governed exclusively by these DataTug Commercial Terms.

---

## 2. Definitions

- **"Company"** means DataTug and its authorized operating entities and affiliates.
- **"Customer"** means the individual, company, or legal entity entering into a commercial agreement, evaluation agreement, or purchasing a subscription for Commercial Modules.
- **"Commercial Software"** or **"Commercial Modules"** means any software, binary distributions, interfaces, source code, container images, or services located in or derived from the `commercial/` namespace, enterprise add-ons, or delivered under a Commercial License Key.
- **"Community Software"** means the codebases licensed under the Apache License 2.0 as enumerated in `LICENSING.md`.
- **"Coordinator"** means the centralized cluster management, synchronization server, or multi-user service governing distributed DataTug instances.
- **"Seat / Node / Capacity"** means the metered entitlement units specified in Customer’s Order Form or Entitlement Descriptor (e.g., active database instances, managed schemas, registered nodes, or user seats).

---

## 3. Commercial License Grants

### 3.1 Commercial Subscription Grant
Subject to timely payment of applicable subscription fees and continuous compliance with these terms, Company grants Customer a non-exclusive, non-transferable, non-sublicensable, worldwide license during the applicable subscription term to:
1. Install and execute Commercial Modules within Customer's private cloud, data centers, or self-managed infrastructure.
2. Utilize the Commercial Modules up to the authorized capacity metrics (e.g., nodes, coordinators, data source connections) set forth in Customer's Order Form.
3. Access and compile commercial interfaces (such as `commercial/licensing`) strictly for integrating authorized company tools and extensions.

### 3.2 Evaluation and Trial Licenses
If Customer is granted evaluation, trial, or proof-of-concept access to Commercial Modules, such license is strictly limited to non-production testing for an authorized evaluation period (not exceeding 30 calendar days unless extended in writing). Evaluation licenses are provided "AS-IS" with no warranties, indemnities, or SLA commitments.

---

## 4. Usage Restrictions and Compliance

Customer shall not, directly or indirectly:
1. **Circumvention**: Bypass, disable, modify, or attempt to defeat any license keys, entitlement verifiers (`EntitlementVerifier`), capacity limiters, or license validation mechanisms.
2. **Unauthorized Sublicensing / SaaS Offering**: Host, lease, sell, or offer Commercial Modules as a stand-alone hosted Managed Service, DBaaS, or Commercial SaaS to third parties without an express OEM or Commercial Distributor Agreement executed with Company. (Note: Utilizing open-source Community Software under Apache 2.0 remains free of this restriction).
3. **Decompilation**: Reverse engineer, decompile, decrypt, or disassemble any binary-only commercial distributions except to the extent permitted by mandatory statutory law.
4. **Proprietary Notices**: Remove or alter any trademark, copyright notice, or confidentiality markers from Commercial Modules.

---

## 5. Community vs. Commercial Boundary and Cleanroom Interfaces

1. **Independent Module Boundaries**:
   DataTug maintains a clean structural separation between open-source community code and commercial components. Community packages (`pkg/`, `cmd/`, `internal/`) must not import proprietary commercial implementations.
2. **Entitlement Interfaces**:
   Interfaces located under `commercial/licensing/` define neutral contractual abstractions (e.g., `EntitlementVerifier`, `CapacityDescriptor`, `CoordinatorContract`). Community editions utilize no-op or embedded single-node implementations of these contracts without requiring proprietary runtime code.

---

## 6. Support, Maintenance, and SLAs

1. Commercial subscriptions include enterprise maintenance, vulnerability hotfixes, and level-tier technical support in accordance with Company's published Enterprise Support Agreement.
2. Support services are delivered solely for supported releases and compliant deployments within the agreed capacity bounds.

---

## 7. Audit and Verification

Upon reasonable notice (not less than fifteen (15) business days), Company or an independent auditor may verify Customer’s deployment metrics and compliance with capacity limits. Any audit will occur during normal business hours without unreasonably disrupting Customer's operations.

---

## 8. Warranty and Limitation of Liability

1. **Limited Warranty**: Company warrants that Commercial Software will perform substantially in conformance with official documentation for ninety (90) days from deployment.
2. **Disclaimer**: EXCEPT AS EXPRESSLY PROVIDED, ALL COMMERCIAL MODULES ARE PROVIDED ON AN "AS-IS" BASIS. COMPANY DISCLAIMS ALL IMPLIED WARRANTIES OF MERCHANTABILITY, FITNESS FOR A PARTICULAR PURPOSE, AND SYSTEM INTEGRATION.
3. **Liability Cap**: TO THE MAXIMUM EXTENT PERMITTED BY APPLICABLE LAW, NEITHER PARTY SHALL BE LIABLE FOR INDIRECT, INCIDENTAL, CONSEQUENTIAL, OR PUNITIVE DAMAGES, AND COMPANY'S MAXIMUM AGGREGATE LIABILITY SHALL NOT EXCEED THE TOTAL FEES PAID BY CUSTOMER FOR THE COMMERCIAL MODULES IN THE TWELVE (12) MONTHS PRECEDING THE CLAIM.

---

## 9. Contact and Commercial Inquiries

For commercial licensing, enterprise quotes, custom OEM distributions, or legal queries regarding dual-licensing scopes, please contact:
- **Commercial Licensing Desk**: `licensing@datatug.org` (or commercial portal)
- **General Inquiries**: `legal@datatug.org`
