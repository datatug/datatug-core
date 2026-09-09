# semantic

A pure resolver that maps physical columns (source + collection + scanned
column list) to the entity fields a project's model says they mean, per
`REQ:field-mapping-model` in `datatug/datatug`'s `core-investigation-loop`
feature spec: declared `EntityField.Mappings` win; absent a declared mapping,
`EntityField.NamePatterns` are tried and the result is labelled `inferred`; a
column matching neither is simply absent from the result.

No I/O, no mutation of its inputs — safe to call from `datatug-cli`'s HTTP
resolver, the TUI, or a future `datatug context` verb.

## Example

```go
entities := []*datatug.Entity{
	{
		ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "customer"}},
		Fields: datatug.EntityFields{
			{
				ID: "id", Type: "string",
				Mappings: datatug.PhysicalRefs{
					{Source: "chinook", Collection: "Customer", Column: "CustomerId"},
				},
			},
			{
				ID: "email", Type: "string",
				NamePatterns: datatug.StringPatterns{{Type: "exact", Value: "Email"}},
			},
		},
	},
}

results := semantic.Resolve(entities, "chinook", "Customer", []semantic.Column{
	{Name: "CustomerId", Type: "int"},
	{Name: "Email", Type: "string"},
	{Name: "FirstName", Type: "string"},
})
// results == []semantic.Resolution{
//   {Column: "CustomerId", Entity: "customer", Field: "id", Provenance: semantic.Declared},
//   {Column: "Email", Entity: "customer", Field: "email", Provenance: semantic.Inferred, Rule: "namePattern:exact:Email"},
// }
// "FirstName" matches neither a declared mapping nor a name pattern, so it
// has no entry at all.
```
