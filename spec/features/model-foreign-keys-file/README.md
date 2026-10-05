---
format: https://specscore.md/feature-specification
status: Draft
---

# Feature: Foreign keys file of a database model

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/datatug/datatug-core/spec/features/model-foreign-keys-file?op=explore) | [Edit](https://specscore.studio/app/github.com/datatug/datatug-core/spec/features/model-foreign-keys-file?op=edit) | [Ask question](https://specscore.studio/app/github.com/datatug/datatug-core/spec/features/model-foreign-keys-file?op=ask) | [Request change](https://specscore.studio/app/github.com/datatug/datatug-core/spec/features/model-foreign-keys-file?op=request-change) |
**Status:** Draft
**Source Ideas:** —

## Summary

A project stores the foreign keys of a database model in one file beside the model, named `<model>.refs.json`. Each foreign key names the table that holds it, its columns in order, the table it points to and the columns it points to, in the same order. The bytes are deterministic, so two scans of one database write one file. A project without the file is a project with no stored foreign keys, and no existing project file changes.

## Problem

A scan reads the foreign keys of a database, and a scanned project then stores none of them. Every consumer that needs a relation (chat, the related-records lookup, a join suggestion) can know it only from a live connection to a database that reports it, and knows nothing from a project on its own. The file format is a public contract: projects outside this organisation will carry it, and a release that writes it cannot take it back. This page is that contract; the code follows it.

## Behavior

### Where the file is

#### REQ: file-location

The file of a model is named `<model>.refs.json`, where `<model>` is the id of the model, and it sits in the same folder as the model's own file `<model>.dbmodel.json` in the `dbmodels` folder of the project. The folder is found the way the model's own file is found:

1. if `dbmodels/<model>/<model>.dbmodel.json` exists, the file is `dbmodels/<model>/<model>.refs.json`;
2. else, if `dbmodels/<model>.dbmodel.json` exists, the file is `dbmodels/<model>.refs.json`;
3. else, when the model has no file of its own yet, the file is `dbmodels/<model>/<model>.refs.json`, the folder a new model is given.

A model id that is empty, is `.` or `..`, or contains a path separator names no file; a writer and a reader refuse it. The file is written only through plain files and plain folders of the project: a link, or an entry that is not a plain file or folder, on the way is refused, and the refusal names the path inside the project.

### What the file holds

#### REQ: file-content

The file is one JSON object with two members:

- `version`: the number `1`, the format marker (see [REQ:version-marker](#req-version-marker));
- `foreignKeys`: an array with one object for every foreign key of every table of the model. It is an empty array, never absent, when the model has none.

Each foreign key is an object with these members, in this order:

| Member | Type | Meaning |
|---|---|---|
| `name` | string, not empty | the name of the foreign key, as the database spells it |
| `table` | object | the table that holds the foreign key |
| `columns` | array of strings, not empty | the columns of `table` that make the key, in the order of the key |
| `refTable` | object | the table the key points to |
| `refColumns` | array of strings | the columns of `refTable` the key points to, in the same order and of the same count as `columns` |

A table object has `schema` (a string, left out when the table has no schema, as in SQLite) and `name` (a string, not empty). All names are spelled as the database spells them: no case folding, no quoting, no trimming. A foreign key from a table to itself, to a table of another schema, and two foreign keys between the same two tables are each an ordinary entry. A composite key is one entry whose `columns` and `refColumns` have the same length at every position.

A foreign key cannot point to another database, so the file does not store a catalog name: the file belongs to one model of one database.

#### REQ: version-marker

No project file of this repository carries a format marker today, so this file uses a top-level member `version` holding the whole number `1`. A reader refuses a file whose version it does not know, or that has none, with a sentence that names the file inside the project, the version found and the versions the reader reads. A reader ignores a member it does not know, at any depth, so that a later release can add a member without a new version. A new version is needed only when the meaning of an existing member changes.

#### Example

A model with a composite key and a key to a table of another schema. The file `pkg/storage/filestore/testdata/model.refs.json` of this repository is the pinned, complete example that the tests compare byte for byte.

```json
{
	"version": 1,
	"foreignKeys": [
		{
			"name": "fk_invoice_ledger",
			"table": {
				"schema": "public",
				"name": "invoice"
			},
			"columns": [
				"ledger_id"
			],
			"refTable": {
				"schema": "accounting",
				"name": "ledger"
			},
			"refColumns": [
				"id"
			]
		},
		{
			"name": "fk_line_product",
			"table": {
				"schema": "public",
				"name": "line"
			},
			"columns": [
				"product_vendor",
				"product_sku"
			],
			"refTable": {
				"schema": "public",
				"name": "product"
			},
			"refColumns": [
				"vendor",
				"sku"
			]
		}
	]
}
```

### Bytes

#### REQ: deterministic-bytes

A writer produces exactly one byte sequence for one set of foreign keys:

- the entries are sorted by `table.schema`, then `table.name`, then `name`, each compared as a plain string of bytes; the order of the input does not matter;
- the order inside `columns` and `refColumns` is kept as given, never sorted;
- the layout is that of the JSON encoder with a tab for one level of indent and one member or element per line, members in the order of [REQ:file-content](#req-file-content), no escaping of `<`, `>` or `&`, UTF-8 without a byte order mark, and one trailing newline;
- two entries with the same `table.schema`, `table.name` and `name` are an error, not two entries.

A writer replaces the whole file with each write, and writes only through plain files and plain folders of the project ([REQ:file-location](#req-file-location)). A model with no foreign key gets a file with an empty `foreignKeys` array, so a later write that finds none replaces the keys an earlier write found.

#### REQ: writer-checks

A writer refuses, before it touches the file, a foreign key that has no name, a table without a name, no columns, an empty column name, referenced columns that differ in count from the columns, no referenced columns, or a referenced table without a name. A foreign key whose table and referenced table are both in a catalog, and the two catalogs differ, is refused: the file cannot say it. A foreign key read from a source that does not report the referenced columns cannot be stored, and the refusal names it.

### Reading

#### REQ: reader-rules

A reader treats a missing file as a model with no stored foreign keys, and no error. These are errors, each naming the file inside the project and never the path on the machine:

- the file is a folder, a link or any other entry that is not a plain file (a link is refused and never followed), or is over 64 MiB;
- the content is not JSON, or not a JSON object of the shape above;
- the version is unknown or absent ([REQ:version-marker](#req-version-marker));
- an entry breaks the checks of [REQ:writer-checks](#req-writer-checks), or two entries have the same table and name.

A reader returns the entries in the order of [REQ:deterministic-bytes](#req-deterministic-bytes), whatever order the file had, grouped by the table that holds them. Reading, as everywhere in the file store, does not walk the folders above the file looking for links; it is the file itself that is checked.

#### REQ: no-change-to-existing-files

Writing the file changes no other file of the project, and a project with no such file reads exactly as before: no existing file is changed by one byte, no existing reader looks at the file, and the loader of the models, which finds a model by its `<model>.dbmodel.json` file, does not take the file for a model.

### What is not stored

#### REQ: not-stored

The file does not store: indexes; check constraints; unique and primary keys (they are in the table's own files); the rule on delete or on update of a key; the match option; whether a key is deferrable, enforced or validated; the catalog of a table; and the inverse view of a key (the tables that reference a table), which is computed by reading every entry from the other side.

These are left out because the consumers of the file (a join suggestion, a related-records lookup, a chat that follows a relation) need only the pairs of columns that join two tables, and because engines report the rest in different shapes that this format should not fix before a need is known. Each can be added later as an optional member of a foreign key, or as another file beside this one, without a new version, because a reader ignores what it does not know.

## Acceptance Criteria

### AC: round-trip

Given a model with a single-column key, a composite key, a key to a table of another schema, a key from a table to itself, and two keys between the same two tables
When the foreign keys are saved and loaded again
Then the loaded keys equal the saved keys, and the saved bytes equal the pinned example file byte for byte.

### AC: deterministic-bytes

Given the same foreign keys handed to the writer in two different orders
When both are saved
Then the two files are byte-identical, and a second save of the same keys leaves the file unchanged.

### AC: location

Given a model stored nested, a model stored flat, and a model with no file of its own
When the foreign keys of each are saved
Then the file is `dbmodels/<model>/<model>.refs.json`, `dbmodels/<model>.refs.json` and `dbmodels/<model>/<model>.refs.json`, and the models are listed and loaded as before.

### AC: no-file-no-keys

Given a project with no foreign keys file
When the foreign keys of a model are loaded
Then there are none and no error, and no file of the project is created or changed by the load.

### AC: refusals

Given a file that is a folder, a link, not JSON, of an unknown version, without a version, or with an entry whose column counts differ
When it is loaded
Then the error names the file inside the project, and for an unknown version names the version found and the versions read.

### AC: unknown-members-ignored

Given a file of version 1 with an extra member on the object, on an entry and on a table object
When it is loaded
Then the keys load as if the extra members were absent.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
