---
format: https://specscore.md/feature-specification
status: Draft
---

# Feature: Multi-series saved board bar chart

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/datatug/datatug-core/spec/features/board-multi-series-chart?op=explore) | [Edit](https://specscore.studio/app/github.com/datatug/datatug-core/spec/features/board-multi-series-chart?op=edit) | [Ask question](https://specscore.studio/app/github.com/datatug/datatug-core/spec/features/board-multi-series-chart?op=ask) | [Request change](https://specscore.studio/app/github.com/datatug/datatug-core/spec/features/board-multi-series-chart?op=request-change) |
**Status:** Draft
**Source Ideas:** —

## Summary

A saved bar chart displays two to eight measures from one admitted query while preserving legacy single-measure boards.

## Problem

The existing saved `bar` chart maps one `valueColumn`. The native OVDB root query already returns one complete 17-row Northwind/Chinook country comparison with two distinct customer-count columns, but a saved board cannot show those values side by side. A chart field added only to the browser would be rejected by Core's strict chart decoder and by server consumers still pinned to an older Core release. The linked Orders detail also needs a declarative saved contract; a viewer's selected country and filtered rows must remain browser-only.

## Behavior

### REQ: compatible grouped bar wire

A `chart` widget retains its existing `sources`, `chart` and `defaultView` structure. For `type:"bar"`, exactly one of the following is present:

- Legacy `valueColumn`: one measure, with the existing optional `unit`. Existing saved boards, validation and rendering continue to work.
- New `series`: an array of 2–8 objects `{id,title,valueColumn}`. Every id and value column is nonempty and distinct, ids have no surrounding whitespace, titles are nonempty display text, and no value column equals `categoryColumn`. A single nonempty `unit` on the chart applies to every series. `valueColumn` is omitted, not empty. The chart binds exactly one saved query source; the series are columns of that one admitted result, not separate executions or a cross-source join.

`bar-race` keeps its existing `valueColumn` and race settings and rejects `series`. Multi-series `bar` rejects race settings and source transforms. The JSON decoder rejects unknown chart fields as it does today. New `series` and `selectionId` field names require exact canonical spelling; case variants and case-variant duplicates are rejected. In the new branch every spelling of legacy `valueColumn` must be absent: explicit `null` or `""` is invalid, while legacy charts retain case-insensitive decoding. `series:null`, an empty array, and null/empty/unknown members of a series object are invalid. Structural validation does not assert that a saved query exists, that column names or types match its result, or that rights were granted; the app validates those facts after query admission. No result rows, execution receipts or selected state are stored in board JSON.

Example:

```json
{
  "type": "bar",
  "categoryColumn": "country",
  "unit": "customer records",
  "series": [
    {"id": "northwind", "title": "Northwind", "valueColumn": "northwindCustomers"},
    {"id": "chinook", "title": "Chinook", "valueColumn": "chinookCustomers"}
  ],
  "selectionId": "customer-country"
}
```

### REQ: declared presentation link

An optional `selectionId` on a `bar` chart declares that a user may select one category value in that chart. It is a stable board-local identifier, nonempty without surrounding whitespace. At most one chart in a board may produce a given selection id. It is not supported by `bar-race`. A SQL widget may declare `data.sql.presentationFilter: {selectionId, column}`, where `column` is the exact result column compared with the selected chart category. Both fields are nonempty literal names. A filter references exactly one declared chart selection in the same board; it contains no executable expression, query parameter, SQL fragment or default value. The new nested filter object is closed: unknown fields, explicit `null` and empty members are rejected before a permissive legacy SQL decoder can discard them. The outer `presentationFilter` member requires exact canonical spelling, including when another case variant is also present; other legacy SQL widget fields stay permissive. Existing SQL widgets without this member behave unchanged. Core validates the saved structure and reference; the app checks the admitted result's column and type before applying it.

Resolve saved references in a second `Board.Validate` pass after per-widget validation: walk every row/card and nested `tabs` child, active or inactive, through the existing maximum nesting depth of 32; skip valid empty placeholders. Handle both directly typed widget definitions and raw JSON/map definitions without mutating their saved data. Collect chart producer ids and SQL filter references, then reject duplicate producers and any reference with no producer. Standalone widget validation checks local shape; only a complete board resolves cross-widget references. Legacy boards with neither declaration remain valid.

The selected value lives only in separate board-view presentation state in browser memory. It is not serialized, included in a query request, added to existing `boardContext`, used in board execution/cache identity, or propagated as a `boardParameterId`. A detail view filters a demonstrably complete, already admitted held result by exact string equality on `column`; it never reruns the query or treats a partial page as complete. A truncated, paged or unverified result displays an honest unsupported state instead of a misleading subset count. A complete result shows the matching count and full held count, including a truthful zero-match state. Clearing selection restores the same held result and its original order. Selection changes affect only declared detail presentations; all other widgets retain their executions, playback, focus and results. A tutorial step change clears a selection that is no longer present, and tutorial restart/Show Full clears the selection. Tutorial state remains session-only.

For the showcase, the selected chart value is a *customer country* in the cross-database query; the linked `ShipCountry` is where a Northwind order was shipped. Equal labels relate country categories, not customers, people or orders across businesses. The comparison query itself produces 17 shared exact-label rows; no client computation pretends to join source records.

The concrete completeness proof for the existing showcase Orders path is `executeSavedPublicChartQuery` admitting the publisher-pinned 830 rows and canonical digest before returning a `browser-public-api` result. The presentation guard requires `complete:true`, `hasMore:false`, `truncated:false`, `publicApiReceipt.rowsReturned === recordset.rows.length === 830`, the saved Orders query identity and valid chart source evidence. The 830 count is a demo pin, never a generic filtering limit. A future unpinned result requires its own equally demonstrable complete-result admission.

## Acceptance Criteria

### AC: legacy-round-trip

Given the existing single-series bar and bar-race fixtures, when parsed, validated and serialized by Core, then they retain their supported behavior and wire shape; a new `series` member is never required.

### AC: multi-series-wire

Given one chart source and a two-series bar with one unit, when a board is validated and round-tripped, then the distinct ids, titles, columns, unit and optional selection id survive exactly. Zero, one or nine series; duplicate ids or columns; mixed `valueColumn` and `series` including explicit null or empty value; null/unknown series fields; absent unit; race settings; transforms; multiple sources; and unknown fields are rejected with field-specific errors.

### AC: presentation-link-wire

Given a chart selection and a SQL widget's `presentationFilter`, when a board is validated and round-tripped, then the reference and exact column survive in typed and raw JSON/map forms. An unresolved or duplicate selection id, blank or null column, null filter object, or executable/unknown filter field is rejected, including in inactive nested tabs; existing SQL widgets and valid empty tab placeholders remain valid.

### AC: admitted-presentation

Given one complete, authorized held result with declared category and numeric series columns, when the board displays it, then grouped bars and the grid show the same two per-row measures and common unit without another query or fabricated sum. Toggling chart/grid adds no query. Invalid, missing or incomplete runtime rows fail visibly rather than silently rendering invented values.

### AC: linked-exploration

Given a demonstrably complete 830-row held Orders result, when a user selects or rapidly changes a chart country, then only the declared Orders detail presentation filters by exact `ShipCountry`, labels its visible count out of 830 and source meaning, and the latest choice wins without a request. Zero matches is valid; Clear restores all 830 in original order. Selection and Clear add no query request. With an incomplete or paged Orders result, the view reports that linked filtering is unavailable. Other widget results, chart/grid view, playback and focus stay stable; tutorial restart and Show Full clear ephemeral selection.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
