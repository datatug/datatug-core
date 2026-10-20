---
format: https://specscore.md/feature-specification
status: Draft
---

# Feature: Evidence-bound resolution and saved evidence charts

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/datatug/datatug-core/spec/features/evidence-bound-resolution-and-charts?op=explore) | [Edit](https://specscore.studio/app/github.com/datatug/datatug-core/spec/features/evidence-bound-resolution-and-charts?op=edit) | [Ask question](https://specscore.studio/app/github.com/datatug/datatug-core/spec/features/evidence-bound-resolution-and-charts?op=ask) | [Request change](https://specscore.studio/app/github.com/datatug/datatug-core/spec/features/evidence-bound-resolution-and-charts?op=request-change) |
**Status:** Draft
**Source Ideas:** —

## Summary

Required resolution criteria and generic held-result timeline and heatmap board contracts.

## Problem

An owned investigation needs a criterion that remains identical across failure and recovery, while ordinary incidents retain manual resolution semantics. Saved boards need generic processing and readiness views that survive Go-store round trips without persisting protected results or introducing another renderer.

## Behavior

### REQ: required resolution admission

The optional `ResolutionCriterion` uses `enforcement: required`. Existing incidents without it retain existing status/outcome semantics, including manual resolution while ordinary metrics are unmet. `incident.criterion-set` starts at version 1; draft revisions advance by one. A finalized criterion is immutable. `finalizedAt` is UTC execution wall time and matches its commit event. It pins the existing ProjectScope including environment, logical sourceID, workspace generation, typed affectedInputs, sessionRef, bagID, appliedManifestRef, checkID/version and maxAgeSeconds (1–86400). Attempts and configuration revisions are deliberately excluded: recovery changes those observations while the criterion stays fixed. Backend dispatch must finalize before the first failing run; Core cannot observe an unrecorded run.

`incident.resolved` commits `humanNote` and `ResolutionProof` atomically. A required criterion blocks generic resolved/watching/closed transitions until this verified resolution exists. The proof contains criterionVersion, exact scope, a dedicated CheckRunRef, UTC evaluatedAt, outcome pass and nonempty observationRefs/recoveryRefs. A CheckRunRef is `{storeId,projectId,environment,checkId,checkVersion,runId}`; a saved Check run is never represented as a query execution. ArtifactRef adds `{kind: check-run, checkRun: ...}`. All proof references must also appear in event refs for backend authorization. The fold rejects foreign scopes/generations/typed inputs/manifests, changed criterion/check revisions, unknown/error/fail, future/stale runs and runs predating finalization. Ordinary incident merge remains an explicit merge, not a successful resolution.

This is a trusted server receipt contract. The service must resolve actual saved runs under current rights, compare their scope, revision, completeness and result, and compare proof observation references against the current latest attempts/configuration for every affected input before emitting an event. Generic append must reject client-authored criterion/resolution events unless it performs that complete admission. Core does not fetch source records or make a claimed pass trustworthy. Synthetic business clocks are never execution wall time. Event-policy withholding removes the corresponding criterion/proof from views and detaches mutable data.

ProjectArtifactRef adds optional `folder` for saved queries such as folder `detective/shipping` and leaf id `receipt`. Folder paths contain only canonical relative forward-slash segments; reject absolute/trailing/double slash, dot/dotdot, backslash and percent encoding. Existing leaf-only refs remain unchanged. Resolvers compose folder plus leaf only after validation.

### REQ: shared evidence chart wire

Extend the existing query-backed chart widget with types `timeline`, `timing-bars` and `heatmap`. All require exactly one source with a saved queryId and ordinary parameter bindings, no transform, and a nonempty board-local selectionId. Legacy bar/bar-race behavior remains. Evidence charts reject bar/race fields and mismatched nested variants.

Timeline maps `laneColumn,eventColumn,sequenceColumn,timeColumn,clockDomainColumn,precisionColumn,statusColumn,evidenceRefColumn`. Heatmap maps `rowColumn,columnColumn,statusColumn,evidenceRefColumn`. Timeline and heatmap column names are canonical, nonempty and distinct. Dedicated `chart.timing` maps `rowKeyColumn,labelColumn,startTimeColumn,endTimeColumn,startClockDomainColumn,endClockDomainColumn,startPrecisionColumn,endPrecisionColumn,startEvidenceRefColumn,endEvidenceRefColumn,startIdentityColumn,endIdentityColumn`. Timing mappings are canonical and nonempty; start/end clock columns may share one column, as may precision columns. Other mapping roles differ. Timing fields and other nested variants are mutually exclusive. Unknown fields are refused by the existing strict chart decoder. The same board DTO round-trips through the actual Go file project store.

The shared Dashboardius leaf renderer consumes these definitions; DataTug continues to own persistence, query execution and ACL. This contract adds no renderer. The app admits a complete held result and validates actual column types before rendering: timeline sequence is integral per lane, timestamp is observed RFC3339 or null for unattempted, and clock domain/precision are explicit. Never invent timestamps, durations, percentages or zero-length unattempted spans. Heatmap statuses are exactly pass/fail/unknown with duplicate row/column cells rejected. Null/unavailable is unknown. Chart and accessible grid use that same held result. A selection carries the held execution/source/row identity and resolves the admitted sidecar slot under current rights; it must not manufacture a URL or silently rerun. No protected rows, SQL, selected state or execution receipts are saved in board definitions.

### REQ: exact retained row evidence

Optional `Result.rowEvidence` has `version:1`, exact `execution` matching `Result.execution`, canonical `recordsetFingerprint`, `scope:{storeId,spaceID?,projectId,environment}`, `source:{id,generation}`, `rowKeyColumn` and `rows:[{rowKey,slots}]`. Hosted Firestore requires Space identity. Row keys are typed strings or integer strings; type is preserved, and held/metadata keys must be unique and exact. Every slot key names a real held column. `state:available` requires valid nonempty ArtifactRefs including the exact retained ExecutionRef; `state:unavailable` carries no refs. Missing sidecar, row or slot means unavailable. Supplied malformed metadata, mismatched execution/fingerprint/scope, unknown/duplicate keys, invalid refs and unsupported versions fail structural validation. NULL business cells remain unchanged.

`RowEvidenceSelector` carries exact execution, fingerprint, scope, source/generation, row-key column, typed row key and slot column. `Result.ResolveRowEvidence` returns only a detached structurally available slot, refuses mismatched selector identity, and never reruns. These checks establish neither principal authority nor current rights. The provider must persist actual source observations with the execution and re-admit scope, source generation, retention and rights on open. Session, Context Bag, policy and applied-manifest facts come from the execution record, never client metadata.

Canonical cross-language handoff is `pkg/apicontract/testdata/recordset-fingerprints.json`: inputs, exact canonical JSON bytes and SHA-256 from unchanged `FingerprintRecordset`. Go/TypeScript agreement must cover nil/empty arrays, row/column order, HTML escaping, quotes, Unicode U+2028/U+2029, float formats, negative zero and large integer strings. Do not change canonicalization or substitute naive JSON.stringify.

Observed timing admission remains OPEN for provider/held-row preparation: endpoints must be observed/non-NULL, have nonempty distinct observation identity values within the actual source/generation, equal nonempty clock domains, positive equal precision, end >= start and actual admitted endpoint slots. One endpoint may recur across different timing rows. Genuine zero requires two distinct equal-time observations; unattempted/unavailable never becomes zero. DTO validation does not establish measured duration. Timeline point-only semantics, heatmap duplicate-cell refusal, actual retained provider enrichment, shared renderer/editor, TypeScript codec/fixtures, Dashboardius declarative profile and browser-save through the actual Go Project Store remain runtime/consumer gates.

## Acceptance Criteria

### AC: fixed-criterion-and-trusted-proof

Given a draft required criterion, when it is finalized and a passing receipt resolves the incident, then scope/check revisions remain fixed, the note and proof commit together, and generic status bypass, changed scope/revision, future/stale/unknown evidence and pre-finalization runs are rejected. Ordinary incidents still manually resolve. Withheld events hide corresponding view payloads.

### AC: reference-compatibility

Given a saved Check run or folder-qualified query, when its typed reference is validated and JSON round-tripped, then its real identity survives; mixed query-execution/Check-run identities and unsafe folders are rejected, and old folderless references remain valid.

### AC: chart-and-store-compatibility

Given generic timeline, timing-bars and heatmap definitions with one saved query, when validated, JSON round-tripped and saved/reloaded by the Go project store, then every supported mapping survives. Multiple sources, transforms, missing/duplicate columns, mixed variants, executable/unknown fields and bar/race options are refused. Existing bar and bar-race tests continue to pass.

### AC: retained-row-contract-and-fingerprint

Given an old Result without optional metadata or a Result carrying row evidence, when strict-decoded, validated and selected, then old Results remain compatible, unavailable metadata returns no reference, malformed metadata/selector mismatches are rejected, and available references retain exact execution and row-slot identity without aliasing the stored result. Frozen byte/digest fixtures agree with the unchanged Go canonical hash; TypeScript verification remains a consumer gate.

### AC: runtime-consumer-boundary

Given an incomplete, unauthorized or wrongly typed held result, when a consumer opens either chart or evidence, then it refuses rendering/drilldown honestly rather than fabricating a span, pass or cached row. Consumer deployment acceptance is separate from Core contract tests.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
