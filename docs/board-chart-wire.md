# Saved board chart wire

This note records the minimal board JSON contract introduced for the visual showcase. The complete board remains a project asset at `boards/<board-id>/board.json`; this change does not add an execution or authorization path.

- A newly authored tab may have an `id`. IDs are stable within one `tabs` widget and unique among its siblings. Legacy tabs without IDs, repeated display titles, and empty placeholder children remain valid. Code must not treat a title as identity for newly authored tabs.
- A `chart` widget's `data.sources` is a nonempty list of `{id, queryId, parameters?}` bindings. Each `queryId` refers to a saved query in the board's project. Its QueryDef and the viewer's environment determine the connection and execution target. Source IDs identify series within this widget; separate sources require separate authorized executions.
- `data.chart` has `type`, `categoryColumn`, `valueColumn`, and optional `periodColumn`. The supported wire types are `bar` and `bar-race`; a race requires `periodColumn`. These fields name columns in an admitted result or prepared chart recordset. The raw Northwind Orders race query needs the reviewed month/country/count transform before it can satisfy that mapping.
- `data.defaultView` may be `chart` (the omitted default) or `grid`. Switching views is presentation over one admitted snapshot; it does not change the saved query binding. A plain SQL widget remains the existing option for a query-only tab.
- Chart data accepts only declared fields. Runtime rows, access rights, query receipts, loading state, and chosen presentation state are not saved in the chart definition. Validators check wire shape, not whether a query exists, returns these columns, is complete, or is authorized. The app must check those facts when it reads and executes the saved board.

The matching TypeScript board model and renderer are separate Apps work. A board using `name: "chart"` requires that compatible client before it is displayed; this Go change alone does not make a chart render.
