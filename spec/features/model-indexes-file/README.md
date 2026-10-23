---
format: https://specscore.md/feature-specification
status: Draft
---

# Feature: Indexes file of a database model

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/datatug/datatug-core/spec/features/model-indexes-file?op=explore) | [Edit](https://specscore.studio/app/github.com/datatug/datatug-core/spec/features/model-indexes-file?op=edit) | [Ask question](https://specscore.studio/app/github.com/datatug/datatug-core/spec/features/model-indexes-file?op=ask) | [Request change](https://specscore.studio/app/github.com/datatug/datatug-core/spec/features/model-indexes-file?op=request-change) |
**Status:** Draft
**Source Ideas:** —

## Summary

A project stores the indexes of a database model in one file beside the model, named `<model>.indexes.json`. Each index names the table it is on, its columns in the order of the index (each with a descending and an included flag), and whether it is unique and whether it is partial. The bytes are deterministic, so two scans of one database write one file. A project without the file is a project with no stored indexes, and no existing project file changes. The file is built as the foreign keys file `<model>.refs.json` is, and shares its location rule, its version marker and its reading rules.

## Problem

A scan reads the indexes of a database, and a scanned project then stores none of them. A consumer that wants to know which columns are indexed (a query advisor, a chat that suggests a filter, a check of a join) can know it only from a live connection. The file format is a public contract: projects outside this organisation will carry it, and a release that writes it cannot take it back. This page is that contract; the code follows it.

## Behavior

### Where the file is

#### REQ: file-location

The file of a model is named `<model>.indexes.json`, where `<model>` is the id of the model, and it sits in the same folder as the model's own file `<model>.dbmodel.json` and its foreign keys file `<model>.refs.json`. The folder is found as [the foreign keys file's](../model-foreign-keys-file/README.md#req-file-location) is:

1. if `dbmodels/<model>/<model>.dbmodel.json` exists, the file is `dbmodels/<model>/<model>.indexes.json`;
2. else, if `dbmodels/<model>.dbmodel.json` exists, the file is `dbmodels/<model>.indexes.json`;
3. else, when the model has no file of its own yet, the file is `dbmodels/<model>/<model>.indexes.json`, the folder a new model is given.

A model id that is empty, is `.` or `..`, or contains `/` or `\` names no file; a writer and a reader refuse it. The file is written only through plain files and plain folders of the project: a link, or an entry that is not a plain file or folder, on the way is refused, and the refusal names the path inside the project. An indexes file in the other of the two folders is not read.

Deleting a model through the file store (`DeleteDbModel`) removes the indexes file in each of the two folders, with the foreign keys file and before the model's own file; a file that is not there is not an error.

### What the file holds

#### REQ: file-content

The file is one JSON object with two members:

- `version`: the number `1`, the format marker (see [REQ:version-marker](#req-version-marker));
- `indexes`: an array with one object for every index of every table of the model. It is an empty array, never absent and never `null`, when the model has none; a reader refuses a file without it, or with `null`.

Each index is an object with these members, in this order:

| Member | Type | Meaning |
|---|---|---|
| `name` | string, not empty | the name of the index, as its own definition spells it |
| `table` | object | the table the index is on: `schema` and `name`, exactly as in the [foreign keys file](../model-foreign-keys-file/README.md#req-file-content) |
| `columns` | array of objects, not empty | the columns of the index, in the order of the index |
| `unique` | `true`, or absent | the index allows no two rows with the same key; written only when true, and absent or `null` reads as not unique |
| `partial` | `true`, or absent | the index covers only the rows its condition admits (a filtered index), so `unique` holds only among those rows; written only when true |

Each column is an object with these members, in this order:

| Member | Type | Meaning |
|---|---|---|
| `name` | string, not empty | the name of the column of `table`, as its declaration spells it |
| `descending` | `true`, or absent | the column is in descending order in the index; written only when true |
| `included` | `true`, or absent | the column is carried in the index but is not part of its key (`INCLUDE` in SQL Server and PostgreSQL); written only when true |

The key of the index is its columns that are not `included`, in the order of the array; an index has at least one. The `included` columns keep the order they were given in, which has no meaning for the key. A table object, names and member spelling follow the foreign keys file: `schema` is the name of the table's schema folder (`main` for a scanned SQLite file), left out only for a source that reports no schema, and a reader reads an empty string or `null` as no schema; names are spelled as the object's own definition spells them, with no case folding, quoting or trimming; a file that holds a member twice, or in two cases, is not a file of this format.

An index that backs a primary key or a unique constraint, when the source reports it as an index, is an ordinary entry of the file.

#### REQ: version-marker

The file has a top-level member `version` holding the whole number `1`. A reader refuses a file whose version it does not know, or that has none, with a sentence that names the file inside the project, the version found and the versions the reader reads. A reader ignores a member it does not know, at any depth, so that a later release can add a member without a new version. A new version is needed only when the meaning of an existing member changes.

#### Example

The file `pkg/storage/filestore/testdata/model.indexes.json` of this repository is the pinned, complete example that the tests compare byte for byte. Two of its entries:

```json
{
	"version": 1,
	"indexes": [
		{
			"name": "ix_invoice_customer_date",
			"table": {
				"schema": "public",
				"name": "invoice"
			},
			"columns": [
				{
					"name": "customer_id"
				},
				{
					"name": "issued_on",
					"descending": true
				}
			]
		},
		{
			"name": "ix_line_product",
			"table": {
				"schema": "public",
				"name": "line"
			},
			"columns": [
				{
					"name": "product_sku"
				},
				{
					"name": "quantity",
					"included": true
				},
				{
					"name": "price",
					"included": true
				}
			]
		}
	]
}
```

### Bytes

#### REQ: deterministic-bytes

A writer produces exactly one byte sequence for one set of indexes:

- the entries are sorted by `table.schema`, then `table.name`, then `name`, each compared as a plain string of the bytes of its UTF-8 form; the order of the input does not matter; an absent `schema` sorts as the empty string, so before every other;
- the order inside `columns` is kept as given, never sorted;
- `unique`, `partial`, `descending` and `included` are written only when true, and never as `false`;
- the layout and the writing of strings are those of the [foreign keys file](../model-foreign-keys-file/README.md#req-deterministic-bytes): UTF-8 without a byte order mark, LF line ends, one tab for one level of indent, one member or element per line, `"name": value`, an empty array written `[]`, the members in the order of [REQ:file-content](#req-file-content), one trailing newline, and the same escapes;
- two entries with the same `table.schema`, `table.name` and `name` are an error, not two entries.

A writer replaces the whole file with each write. A model with no index gets a file with an empty `indexes` array, so a later write that finds none replaces the indexes an earlier write found.

#### REQ: environments

As for the foreign keys file, the version 1 format does not record which environment or catalog of a model has an index. The file holds the indexes the last write found, and a reader takes every index of the file as an index of the model.

#### REQ: writer-checks

A writer refuses, before it touches the file, an index that has no name, a table without a name, no columns, a column with an empty name (an index on an expression, which has no column name, cannot be stored, and the refusal says so), only `included` columns and no key column, and a name that is not valid UTF-8 text. A writer that reads a source with indexes on expressions leaves those indexes out before it writes.

### Reading

#### REQ: reader-rules

A reader treats a missing file as a model with no stored indexes, and no error. These are errors, each naming the file inside the project and never the path on the machine:

- the file is a folder, a link or any other entry that is not a plain file (a link is refused and never followed), or is over 64 MiB;
- the content is not JSON, or not a JSON object of the shape above (a flag that is not a boolean or `null` among them), or has no `indexes` or has it as `null`;
- the file is not valid UTF-8 text;
- the version is unknown or absent ([REQ:version-marker](#req-version-marker));
- an entry breaks the checks of [REQ:writer-checks](#req-writer-checks), or two entries have the same table and name.

A reader returns the entries in the order of [REQ:deterministic-bytes](#req-deterministic-bytes), whatever order the file had, grouped by the table they are on.

#### REQ: no-change-to-existing-files

Writing the file changes no other file of the project, the model's own file and its foreign keys file among them, and a project with no such file reads exactly as before. The loader of the models, which finds a model by its `<model>.dbmodel.json` file, does not take the file for a model.

### What is not stored

#### REQ: not-stored

The file does not store: the type or method of an index (B-tree, hash, GIN, …) and the engine flags (clustered, column store, XML); the condition of a partial index (only that it has one); an index on an expression; whether an index backs a primary key or a unique constraint; the collation, the operator class and the null order of a column; storage options (fill factor, tablespace, compression); and the catalog of a table.

These are left out because engines report them in different shapes that this format should not fix before a need is known, and because a consumer that asks which columns are indexed, in which order and whether they are unique needs none of them. Each can be added later as an optional member of an index or a column without a new version, because a reader ignores what it does not know. An index on an expression is the exception: a version 1 reader refuses a column without a name, so it needs another top-level member, which a version 1 reader ignores, or a new version.

## Acceptance Criteria

### AC: round-trip

Given a model with a single-column index, a composite index with a descending column, a unique index, an index with included columns, a composite unique index, a partial unique index, a table `a.b` of schema `s` beside a table `b` of schema `s.a`, and names that hold a double quote and a backslash
When the indexes are saved and loaded again
Then the loaded indexes equal the saved ones, and the saved bytes equal the pinned example file byte for byte.

### AC: deterministic-bytes

Given the same indexes handed to the writer in two different orders
When both are saved
Then the two files are byte-identical, and a second save of the same indexes leaves the file unchanged.

### AC: not-stored-dropped

Given an index with a type, an origin, engine flags and a table in a catalog
When it is saved and loaded
Then it loads with its name, columns and flags only, and its table with no catalog.

### AC: location

Given a model stored nested, a model stored flat, and a model with no file of its own
When the indexes of each are saved
Then the file is `dbmodels/<model>/<model>.indexes.json`, `dbmodels/<model>.indexes.json` and `dbmodels/<model>/<model>.indexes.json`, beside the foreign keys file; the models are listed and loaded as before; a file in the other folder is not read; and deleting a model removes its indexes file with its own file.

### AC: no-file-no-indexes

Given a project with no indexes file
When the indexes of a model are loaded
Then there are none and no error, and no file of the project is created or changed by the load.

### AC: refusals

Given a file that is a folder, a link, not JSON, of an unknown version, without a version, without `indexes` or with it `null`, or with an entry that has only included columns
When it is loaded
Then the error names the file inside the project, and for an unknown version names the version found and the versions read; and given an index on an expression, a save is refused before the file is touched.

### AC: unknown-members-ignored

Given a file of version 1 with an extra member on the object, on an entry, on a table object and on a column, and a flag that is `null`
When it is loaded
Then the indexes load as if the extra members were absent and the `null` flag false.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
