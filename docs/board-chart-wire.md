# Saved board chart wire

This note records the minimal board JSON contract introduced for the visual showcase. The complete board remains a project asset at `boards/<board-id>/board.json`; this change does not add an execution or authorization path.

- A newly authored tab may have an `id`. IDs are stable within one `tabs` widget and unique among its siblings. Legacy tabs without IDs, repeated display titles, and empty placeholder children remain valid. Code must not treat a title as identity for newly authored tabs.
- A `chart` widget's `data.sources` is a nonempty list of `{id, queryId, parameters?}` bindings. Each `queryId` refers to a saved query in the board's project. Its QueryDef and the viewer's environment determine the connection and execution target. Source IDs identify series within this widget; separate sources require separate authorized executions.
- `data.chart` has `type`, `categoryColumn`, and either legacy `valueColumn` or, for a grouped `bar`, `series` of 2–8 distinct `{id,title,valueColumn}` measures from one saved query. The grouped form requires one common `unit`, exactly one source, no transform and no race settings; `valueColumn` is omitted. The supported wire types are `bar` and `bar-race`. A race still requires distinct `periodColumn`, `categoryColumn`, `valueColumn`, a declared `unit`, `grain: "month"`, `onDuplicate: "sum" | "reject"`, `mode: "period" | "cumulative"`, and `topN` from 1 to 100. At runtime these map to `BarRaceSpec` (`type` → `kind`, `valueColumn` → `measureColumn`); `grain` is the durable input-period contract, since the current race preparer admits YYYY-MM strings only. A race binds one saved query.
- A source may name the reusable transform `{name: "orders-monthly-country-count", orderIdColumn: "OrderID", dateColumn: "OrderDate", countryColumn: "ShipCountry"}`. It operates on the authorized, complete raw query result, rejects duplicate order IDs and invalid dates/countries, and emits canonical `period` (YYYY-MM), `category` (country), `value` (safe integer distinct-order count) columns. The chart must map those exact output names and use `onDuplicate: "reject"`. The transform is selected by the saved wire name and input column bindings, never by a query ID. The expected 830 orders and observed time range are evidence/runtime checks, not hidden transform constants. A chart without this transform must already receive compatible columns from its query.
- `data.defaultView` may be `chart` (the omitted default) or `grid`. Switching views uses the held admitted result from each bound source; it does not change a saved query binding. A plain SQL widget remains the existing option for a query-only tab.
- Chart data accepts only declared fields. Runtime rows, access rights, query receipts, loading state, and chosen presentation state are not saved in the chart definition. Validators check wire shape, not whether a query exists, returns these columns, is complete, or is authorized. The app must check those facts when it reads and executes the saved board.
- A `bar` may declare `selectionId`; a SQL widget may declare `data.sql.presentationFilter: {selectionId,column}`. The selected category remains viewer-local presentation state, never a saved value or query parameter. `Board.Validate` resolves each filter against exactly one chart producer across rows and nested tabs, while legacy SQL widget fields keep their permissive decoder and the new nested filter is closed to unknown fields. A viewer may filter only a complete admitted held result by exact string equality and must expose the full and visible row counts. This does not join records across saved queries.

The matching TypeScript board model and renderer are separate Apps work. A board using `name: "chart"` requires that compatible client before it is displayed; this Go change alone does not make a chart render.

The Apps mirror should use this exact JSON shape (optional tab IDs preserve old boards):

```ts
interface TabWidget { id?: string; title: string; widget?: BoardWidget }
interface ChartSourceTransform {
  name: 'orders-monthly-country-count';
  orderIdColumn: string; dateColumn: string; countryColumn: string;
}
interface ChartSourceBinding {
  id: string; queryId: string; parameters?: WidgetParameterBinding[];
  transform?: ChartSourceTransform;
}
type ChartSpec =
  | { type: 'bar'; categoryColumn: string; valueColumn: string; unit?: string; selectionId?: string }
  | { type: 'bar'; categoryColumn: string; valueColumn?: never; unit: string;
      series: { id: string; title: string; valueColumn: string }[]; selectionId?: string }
  | { type: 'bar-race'; periodColumn: string; categoryColumn: string;
      valueColumn: string; unit: string; grain: 'month';
      onDuplicate: 'sum' | 'reject'; mode: 'period' | 'cumulative'; topN: number };
interface ChartWidgetDef extends WidgetBase {
  sources: ChartSourceBinding[]; chart: ChartSpec; defaultView?: 'chart' | 'grid';
}
interface SQLWidgetSettings {
  queryId: string; parameters?: WidgetParameterBinding[];
  presentationFilter?: { selectionId: string; column: string };
}
// Add 'chart' to BoardWidgetName; BoardWidget.data remains unknown until narrowed.
```

The two round-trip fixtures in `pkg/datatug/boards_chart_test.go` are the cross-language acceptance input. Apps must parse and render both without dropping fields before authoring a new saved board. The runtime must prove saved-query admission, source rights, complete result, transform inputs/outputs, and chart field eligibility for each binding.
