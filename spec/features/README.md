---
format: https://specscore.md/features-index-specification
---

# Features

Feature specifications for this project.

## Index

| Feature | Status | Description |
|---------|--------|-------------|
| [Foreign keys file of a database model](model-foreign-keys-file/README.md) | Draft | A project stores the foreign keys of a database model in one file beside the model, named `<model>.refs.json`. Each foreign key names the table that holds it, its columns in order, the table it points to and the columns it points to, in the same order. The bytes are deterministic, so two scans of one database write one file. A project without the file is a project with no stored foreign keys, and no existing project file changes. |
| [Indexes file of a database model](model-indexes-file/README.md) | Draft | A project stores the indexes of a database model in one file beside the model, named `<model>.indexes.json`. Each index names the table it is on, its columns in the order of the index (each with a descending and an included flag), and whether it is unique and whether it is partial. The bytes are deterministic, so two scans of one database write one file. A project without the file is a project with no stored indexes, and no existing project file changes. The file is built as the foreign keys file `<model>.refs.json` is, and shares its location rule, its version marker and its reading rules. |
| [Multi-series saved board bar chart](board-multi-series-chart/README.md) | Draft | A saved bar chart displays two to eight measures from one admitted query while preserving legacy single-measure boards. |
| [Evidence-bound resolution and saved evidence charts](evidence-bound-resolution-and-charts/README.md) | Draft | Required resolution criteria and generic held-result timeline and heatmap board contracts. |
| [DALGO project assets](dalgo-project-assets/README.md) | Draft | A saved board or entity must survive a fresh read through the actual Go ProjectStore, including timeline/heatmap widgets and physical entity structure. Implement bounded DALGO asset members while preserving project ownership at the caller's admitted root. |

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/features-index-specification*
