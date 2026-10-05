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

A model id that is empty, is `.` or `..`, or contains `/` or `\` (both are refused on every system) names no file; a writer and a reader refuse it. The file is written only through plain files and plain folders of the project: a link, or an entry that is not a plain file or folder, on the way is refused, and the refusal names the path inside the project.

A foreign keys file in the other of the two folders is not read: for a model stored in `dbmodels/<model>/`, a `dbmodels/<model>.refs.json` is ignored, and the other way round. A model moved by hand from one layout to the other reads as having no foreign keys until its file is moved too.

Deleting a model through the file store (`DeleteDbModel`) removes the model's own file and the foreign keys file, in each of the two folders; a file that is not there is not an error.

An older layout had a file `<catalog>.refs.json` in a catalog's folder, with another content (a JSON array of objects with `primaryKey`, `foreignKeys` and `referencedBy`). It is not this file and nothing reads one as the other: this file is only in the `dbmodels` folder and is a JSON object with a `version`.

### What the file holds

#### REQ: file-content

The file is one JSON object with two members:

- `version`: the number `1`, the format marker (see [REQ:version-marker](#req-version-marker));
- `foreignKeys`: an array with one object for every foreign key of every table of the model. It is an empty array, never absent and never `null`, when the model has none; a reader refuses a file without it, or with `null`.

Each foreign key is an object with these members, in this order:

| Member | Type | Meaning |
|---|---|---|
| `name` | string, not empty | the name of the foreign key, as its own definition spells it; for an engine that does not name a foreign key, see below |
| `table` | object | the table that holds the foreign key |
| `columns` | array of strings, not empty | the columns of `table` that make the key, in the order of the key |
| `refTable` | object | the table the key points to |
| `refColumns` | array of strings | the columns of `refTable` the key points to, in the same order and of the same count as `columns` |

For an engine that does not name a foreign key (SQLite does not), the writer gives it a name that is unique in its table and the same on every scan of an unchanged table; the name has no other meaning to a reader.

A table object has `schema` (a string: the name of the table's schema folder `dbmodels/<model>/<schema>/`, which is `main` for a table of a scanned SQLite file; it is left out only for a source that reports no schema at all, and a reader reads an empty string or `null` as no schema, and a writer writes neither) and `name` (a string, not empty). Member names are written exactly as shown, in this case; a reader need not accept another case, and a file that holds a member twice, or in two cases, is not a file of this format: a writer never writes one, and what a reader makes of it is not defined. All names are spelled as the object's own definition spells them (the table as it was created, a column as it was declared), never as another text that refers to it, such as the text of a key that names its columns in another case: no case folding, no quoting, no trimming. A writer whose source gives a column in another case than its declaration resolves it to the declared spelling before it writes; the file does not record the engine, so a reader can match names only byte for byte. A foreign key from a table to itself, to a table of another schema, and two foreign keys between the same two tables are each an ordinary entry. A composite key is one entry whose `columns` and `refColumns` have the same count, the column at each position of one pointing to the column at the same position of the other.

A key points to a table of the same model, and a table of a model is identified by its schema and its name, as its folder `dbmodels/<model>/<schema>/` is, whichever environment or catalog of the model has it. So the file does not store a catalog name. See [REQ:environments](#req-environments) for a model that more than one environment or catalog feeds.

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

- the entries are sorted by `table.schema`, then `table.name`, then `name`, each compared as a plain string of the bytes of its UTF-8 form; the order of the input does not matter;
- the order inside `columns` and `refColumns` is kept as given, never sorted;
- an absent `schema` sorts as the empty string, so before every other;
- the layout is, byte for byte: UTF-8 without a byte order mark; lines end in LF (U+000A); one tab for one level of indent; one member or element per line; a member written as `"name": value` (a colon and one space); an empty array written `[]`; the members in the order of [REQ:file-content](#req-file-content); and one trailing newline after the closing brace;
- in a string, `"` is written `\"` and `\` is written `\\`; U+0008, U+000C, U+000A, U+000D and U+0009 are written `\b`, `\f`, `\n`, `\r` and `\t`; every other character below U+0020, and U+2028 and U+2029, are written as `\u` and four lower-case hexadecimal digits; every other character, `<`, `>`, `&` and U+007F among them, is written as its UTF-8 bytes;
- two entries with the same `table.schema`, `table.name` and `name` are an error, not two entries.

A writer replaces the whole file with each write, and writes only through plain files and plain folders of the project ([REQ:file-location](#req-file-location)). A model with no foreign key gets a file with an empty `foreignKeys` array, so a later write that finds none replaces the keys an earlier write found (see [REQ:environments](#req-environments) for what "found" means for a model of more than one environment).

#### REQ: environments

A model of a project can be fed by more than one environment, and each environment by more than one catalog. The version 1 format does not record which environment or catalog has a key. The file holds the keys the last write found: a write for one environment replaces the keys that a write for another environment found, as the other files a scan writes for a model hold the attributes of the last scan. A reader takes every key of the file as a key of the model, whichever environment has it.

A later release may add an optional `byEnv` member to a key, as a column has, without a new version; a reader that does not know it takes every key as a key of the model, as before.

#### REQ: writer-checks

A writer refuses, before it touches the file, a foreign key that has no name, a table without a name, no columns, an empty column name, referenced columns that differ in count from the columns, no referenced columns, an empty referenced column name, or a referenced table without a name, and a name that is not valid UTF-8 text. A foreign key whose table and referenced table are both in a catalog, and the two catalogs differ, is refused: the file cannot say it. A foreign key read from a source that does not report the referenced columns cannot be stored, and the refusal names it.

### Reading

#### REQ: reader-rules

A reader treats a missing file as a model with no stored foreign keys, and no error. These are errors, each naming the file inside the project and never the path on the machine:

- the file is a folder, a link or any other entry that is not a plain file (a link is refused and never followed), or is over 64 MiB;
- the content is not JSON, or not a JSON object of the shape above, or has no `foreignKeys` or has it as `null`;
- the file is not valid UTF-8 text, since a decoder would read it as other names;
- the version is unknown or absent ([REQ:version-marker](#req-version-marker));
- an entry breaks the checks of [REQ:writer-checks](#req-writer-checks), or two entries have the same table and name.

A reader returns the entries in the order of [REQ:deterministic-bytes](#req-deterministic-bytes), whatever order the file had, grouped by the table that holds them. Reading, as everywhere in the file store, does not walk the folders above the file looking for links; it is the file itself that is checked.

#### REQ: no-change-to-existing-files

Writing the file changes no other file of the project, and a project with no such file reads exactly as before: no existing file is changed by one byte, no existing reader looks at the file, and the loader of the models, which finds a model by its `<model>.dbmodel.json` file, does not take the file for a model.

### What is not stored

#### REQ: not-stored

The file does not store: indexes; check constraints; unique and primary keys (they are in the table's own files); the rule on delete or on update of a key; the match option; whether a key is deferrable, enforced or validated; the catalog of a table; and the inverse view of a key (the tables that reference a table), which is computed by reading every entry from the other side.

These are left out because the consumers of the file (a join suggestion, a related-records lookup, a chat that follows a relation) need only the pairs of columns that join two tables, and because engines report the rest in different shapes that this format should not fix before a need is known. Each of these can be added later as an optional member of a foreign key, or as another file beside this one, without a new version, because a reader ignores what it does not know. The one exception is a catalog that differs between the two tables of a key, which cannot be added that way (below).

Two things cannot be added to `foreignKeys` of a version 1 file: a key into another catalog, which a version 1 reader would read as a key into its own, and a key without referenced columns, for which a version 1 reader refuses the whole file. Either needs another top-level member, which a version 1 reader ignores, or a new version.

## Acceptance Criteria

### AC: round-trip

Given a model with a single-column key, a composite key, a key to a table of another schema, a key from a table to itself, two keys between the same two tables, a table `a.b` of schema `s` beside a table `b` of schema `s.a`, and names that hold a double quote and a backslash
When the foreign keys are saved and loaded again
Then the loaded keys equal the saved keys, and the saved bytes equal the pinned example file byte for byte.

### AC: deterministic-bytes

Given the same foreign keys handed to the writer in two different orders
When both are saved
Then the two files are byte-identical, and a second save of the same keys leaves the file unchanged.

### AC: escapes

Given names that hold a double quote, a backslash, a line feed, a tab, U+0008, U+000C, another control character, U+007F, U+2028 and `<`, `>` and `&`
When the foreign keys are saved
Then each is written as [REQ:deterministic-bytes](#req-deterministic-bytes) says, and loads back as the name it was.

### AC: environments

Given a model of two environments, the second of which has fewer foreign keys than the first
When the keys the first environment's scan found are saved and then the keys the second one's scan found are saved
Then the file holds only the keys of the second write, and loads as keys of the model.

### AC: location

Given a model stored nested, a model stored flat, and a model with no file of its own
When the foreign keys of each are saved
Then the file is `dbmodels/<model>/<model>.refs.json`, `dbmodels/<model>.refs.json` and `dbmodels/<model>/<model>.refs.json`, and the models are listed and loaded as before; a file in the other folder is not read; and deleting a model removes its foreign keys file with its own file.

### AC: no-file-no-keys

Given a project with no foreign keys file
When the foreign keys of a model are loaded
Then there are none and no error, and no file of the project is created or changed by the load.

### AC: refusals

Given a file that is a folder, a link, not JSON, of an unknown version, without a version, without `foreignKeys` or with it `null`, or with an entry whose column counts differ
When it is loaded
Then the error names the file inside the project, and for an unknown version names the version found and the versions read.

### AC: unknown-members-ignored

Given a file of version 1 with an extra member on the object, on an entry and on a table object
When it is loaded
Then the keys load as if the extra members were absent.

### AC: schema-reads-as-none

Given a file whose table has `"schema": ""` or `"schema": null`
When it is loaded
Then the table has no schema.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/feature-specification*
