---
format: https://specscore.md/features-index-specification
---

# Features

Feature specifications for this project.

## Index

| Feature | Status | Description |
|---------|--------|-------------|
| [Foreign keys file of a database model](model-foreign-keys-file/README.md) | Draft | A project stores the foreign keys of a database model in one file beside the model, named `<model>.refs.json`. Each foreign key names the table that holds it, its columns in order, the table it points to and the columns it points to, in the same order. The bytes are deterministic, so two scans of one database write one file. A project without the file is a project with no stored foreign keys, and no existing project file changes. |

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/features-index-specification*
