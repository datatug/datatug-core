# semantic

Pure logic — no I/O, no mutation of its inputs — that turns a project's model
(`Entity`, `EntityField.Mappings`/`NamePatterns`, `QueryDef.Parameters[].Meta`)
plus a scanned schema into the three things the `core-investigation-loop`
feature's investigation loop needs: which column means which entity field
(`Resolve`), what else is reachable from a selected value (`RelatedLookups`),
and which library queries can already be run with the values on hand
(`Applicable`). Safe to call from `datatug-cli`'s HTTP resolver, the TUI, or a
future `datatug context` verb — this package never talks to a database or a
project store itself.

## Resolve

Maps physical columns (source + collection + scanned column list) to the
entity fields a project's model says they mean, per `REQ:field-mapping-model`:
declared `EntityField.Mappings` win; absent a declared mapping,
`EntityField.NamePatterns` are tried and the result is labelled `inferred`; a
column matching neither is simply absent from the result.

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

## RelatedLookups

Given a selected `SemanticValue` (a resolved cell — build one from a
`Resolution` plus the value the cell held), returns every other collection
reachable from it: through declared foreign keys in both directions within
its own source, and through mappings of the same entity field in other
sources/collections, per `REQ:related-lookup-model`. It only says *where* to
look — the caller (`datatug-cli`, per `REQ:related-lookup-execution`) builds
and runs the actual filtered query through the access-policy path.

```go
schema := map[semantic.SchemaKey]semantic.TableSchema{
	{Source: "chinook", Collection: "Customer"}: {
		PrimaryKey: &datatug.UniqueKey{Name: "PK_Customer", Columns: []string{"CustomerId"}},
		ReferencedBy: datatug.ReferencedBys{
			{
				DBCollectionKey: datatug.NewTableKey("Invoice", "", "", nil),
				ForeignKeys:     []*datatug.RefByForeignKey{{Name: "FK_Invoice_Customer", Columns: []string{"CustomerId"}}},
			},
		},
	},
}

selected := semantic.SemanticValue{
	Entity: "customer", Field: "id", Value: 5,
	Source: "chinook", Collection: "Customer", Column: "CustomerId",
	Provenance: semantic.Declared,
}

lookups := semantic.RelatedLookups(entities, schema, selected)
// lookups == []semantic.Lookup{
//   {Source: "chinook", Collection: "Invoice", Column: "CustomerId", Kind: semantic.LookupReferencedBy, Via: "FK_Invoice_Customer"},
//   {Source: "support-notes", Collection: "Customer", Column: "CustomerId", Kind: semantic.LookupSameField, Via: "field:customer.id"},
// }
// (the second entry assumes entities also declares a mapping of customer.id
// to source "support-notes", collection "Customer")
```

## Applicable

Given the library's queries and the semantic values on hand (the current
selection plus the Investigation Context), splits them into queries every
required, `Meta`-tagged parameter of which can be bound, and queries still
missing at least one — per `REQ:applicable-queries` and
`REQ:parameter-auto-binding`. A parameter with no `Meta`, or an unsatisfied
optional one, never blocks applicability.

```go
query := &datatug.QueryDef{
	ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "customer-invoices", Title: "Customer invoices"}},
	Type: datatug.QueryTypeSQL,
	Parameters: datatug.Parameters{
		{ID: "customerId", Type: "integer", IsRequired: true, Meta: &datatug.EntityFieldRef{Entity: "Customer", Field: "ID"}},
	},
}

available := []semantic.SemanticValue{
	{Entity: "Customer", Field: "ID", Value: 5, Source: "chinook", Collection: "Customer", Column: "CustomerId", Provenance: semantic.Declared},
}

applicable, notYet := semantic.Applicable([]*datatug.QueryDef{query}, available)
// applicable[0].Bindings  == []semantic.Binding{{Parameter: "customerId", Value: 5, From: available[0]}}
// applicable[0].Chain     == []string{"CustomerId → Customer.ID (declared) → parameter customerId"}
// notYet                  == nil (this query had everything it needed)
```
