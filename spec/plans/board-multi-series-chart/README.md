---
format: https://specscore.md/plan-specification
status: Draft
---

# Plan: Deliver compatible multi-series board chart wire

**Status:** Draft
**Source Feature:** board-multi-series-chart
**Date:** 2026-10-19
**Owner:** backend_final_review
**Supersedes:** —

## Summary

Publish one backward-compatible Core board-wire release for grouped bar series and a declared presentation-only country link. The visitor then sees two customer-record measures in one chart and can select a country to narrow only the already held Orders detail, with Clear restoring the full result.

## Approach

Extend the existing `ChartSpec`, `ChartWidgetDef` and `SQLWidgetSettings` validator path; preserve the current strict chart decoder and legacy fixtures. The new `series` and `selectionId` chart fields plus SQL `presentationFilter` ship in the same Core version because backend board reads and project-copy validation use Core. The outer SQL decoder stays legacy-compatible, while the new nested filter is closed to unknown, null and executable members; new series requires `valueColumn` absent, not null or empty. A second `Board.Validate` pass resolves unique producers and filter references across rows, cards and active/inactive nested tabs, with depth32, raw map and typed data handling and valid empty placeholders. Standalone widgets validate local shape. Release Core first; update and verify backend, Cloud, CLI and Demo Project validation pins and Apps source against that exact tag. Deploy a compatible Apps parser, tutorial validator and renderer before publishing a board that uses the new fields. Genuine DTQL parameter binding, multiple independently executed chart sources, new chart libraries, browser source scans and client-side cross-business joins are outside this slice. The amended and independently reviewed Apps `native-server-federation` feature/plan owns the separate presentation acceptance.

## Tasks

### Task 1: Freeze and test the complete saved wire

**Verifies:** board-multi-series-chart#ac:legacy-round-trip, board-multi-series-chart#ac:multi-series-wire, board-multi-series-chart#ac:presentation-link-wire
**Status:** planning

Add Go fields and validators for two to eight distinct bar series, one unit and one source, while retaining legacy `valueColumn` and bar-race. Include the chart selection producer and SQL presentation-filter reference in the same release. Keep the outer SQL widget permissive but decode the new filter object strictly before unknown fields disappear. Reject series with explicit empty/null `valueColumn`, null/unknown/empty series and filter members. The Board-level second pass walks every row/card and active/inactive nested tab to depth32; accept directly typed and raw JSON/map forms, skip valid placeholders, reject duplicate producers in separate cards/tabs and unresolved references. Assert all cases with focused raw/typed round trips.

### Task 2: Release Core and verify consumers

**Verifies:** board-multi-series-chart#ac:legacy-round-trip, board-multi-series-chart#ac:multi-series-wire, board-multi-series-chart#ac:presentation-link-wire
**Status:** planning

Independently review the exact Core head, run focused Go tests and repository gates, publish a Core tag, then bump and validate backend, Cloud, CLI, Demo Project test consumers and Apps against that tag. Backend project-copy and board reads must accept the new board without dropping either declaration; old demo boards must still pass. Keep each consumer in the coordinated isolated task and land through the single root landing owner. The public Demo board remains unchanged until the compatible Apps model/tutorial/parser/renderer is deployed with an exact-build receipt, after provider and Go consumer checks; only then publish the new Demo HEAD.

### Task 3: Prove admitted presentation with the Apps owner

**Verifies:** board-multi-series-chart#ac:admitted-presentation, board-multi-series-chart#ac:linked-exploration
**Status:** planning

Apps mirrors the wire and checks the complete raw GitHub board with a bounded nested traversal before rendering; Core validation is not executed on this public read path. One native root POST provides the existing complete 17-row result. A narrow ordinary-root chart admission arm validates exact planned two-source identities, complete/totalRows/materialized-row equality and typed rows without inventing a protected execution profile, collection or rights; a partial result fails visibly. Grouped bars support all declared two to eight series on one zero baseline/common numeric scale, with stable colors/labels and no normalization or sum. Keyboard selection and chart/grid toggle reuse held rows. A declared SQL filter narrows the publisher-admitted `demodb/northwind-orders-full-sqlite` result in memory by exact `ShipCountry` string only after its verified830-row digest, rights and complete public receipt; incomplete or paged results show an honest unsupported state. Chart/grid toggle, selection and Clear add no request or SQLite download. Keep selection outside `boardContext` so unrelated query inputs, held results, race playback and focus remain stable. Show the SQL runtime picker only when an alternate working copy exists. Use focused component/model tests and a real browser journey on the published Demo board.

### Task 4: Publish and inspect the board

**Verifies:** board-multi-series-chart#ac:admitted-presentation, board-multi-series-chart#ac:linked-exploration
**Status:** planning

After the compatible Apps build is deployed, add the paired chart beside the existing Northwind country chart; the inspector remains on its own full-width row in Demo Project main `eaba7b052fff26ad16a21649cf536c59b68d61a5`. Bind the native comparison query with 17 exact shared country rows and two customer-record columns, and bind the separate detail widget explicitly to `demodb/northwind-orders-full-sqlite` (the name denotes the saved query; default execution is the public OVDB server). Preserve the original held830 rows and receipt while filtering only a presentation projection, so Clear restores row order. Run backend `TestCloneCurrentProjectCheckout` against unchanged 500 KiB source-proof and 768 KiB operation limits, plus focused model and browser gates. Check exact source receipts and row counts without broad public DB load. Validate deployed browser behavior and source semantics before calling the combined chart acceptance complete.

## Open Questions

None at this time. The wire names remain a reviewed pre-implementation proposal until this Feature and Plan receive independent foundation approval.

---
*This document follows the https://specscore.md/plan-specification*
