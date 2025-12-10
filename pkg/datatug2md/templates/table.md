# Table: [{{.table.Schema}}](..).[{{.table.Name}}]
{{.recordsCount}}

```
USE {{.catalog}};
SELECT * FROM {{.table.Schema}}.{{.table.Name}};
```

{{.openInDatatugApp}}

## Primary key
{{.primaryKey}}

## Foreign keys
{{.foreignKeys}}

## Columns
{{.columns}}

## Indexes
{{.indexes}}

## Referenced by
{{.referencedBy}}

