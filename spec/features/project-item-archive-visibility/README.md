---
format: https://specscore.md/feature-specification
status: Draft
---

# Feature: Project item archive visibility

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/datatug/datatug-core/spec/features/project-item-archive-visibility?op=explore) | [Edit](https://specscore.studio/app/github.com/datatug/datatug-core/spec/features/project-item-archive-visibility?op=edit) | [Ask question](https://specscore.studio/app/github.com/datatug/datatug-core/spec/features/project-item-archive-visibility?op=ask) | [Request change](https://specscore.studio/app/github.com/datatug/datatug-core/spec/features/project-item-archive-visibility?op=request-change) |
**Status:** Draft
**Source Ideas:** —

## Summary

Archive and restore change discovery visibility while preserving definitions, references, direct access and execution. Destructive deletion remains a separate capability requiring a complete reference inventory.

## Problem

Treating archive as deletion blocks routine cleanup of referenced items. Filtering internal resolver reads instead breaks existing boards and saved execution inputs.

## Behavior

Journey: open a project's item catalog → review an authorized current item → Archive → item disappears from active lists → reload → find it in Archived → Restore → item returns to active lists. Existing boards, queries and sessions continue resolving the definition throughout.

Providers authorize the current actor and project scope. A visibility catalog can advertise `archiveSupported` without claiming a complete reference graph. The visibility planner never grants Delete. Complete-graph deletion retains its existing reference checks and concurrent writer boundary.

The filesystem provider stores a bounded project visibility sidecar atomically with a durable operation receipt. Archive/restore require a current catalog revision; replay binds the actor, complete provider scope and request digest. Replaying an old Archive after a later Restore returns its original receipt without changing restored visibility. Current runtime targets can be supplied through an actor-aware callback, invoked outside filesystem locks so normal project readers remain safe. The callback does not establish a deletion proof.

User-facing APIs opt into `ActiveItemsOnly`. Internal/default loaders and direct object reads retain archived definitions for reference and execution resolution. Source files and retained execution history are not rewritten or removed by archive.

## Acceptance Criteria

- Given an item referenced by a board or session, when archived, then the definition and references remain intact and direct reads still succeed.
- Given active-only query, board, card, entity, environment or model discovery, when an item is archived, then it is hidden; default internal loads continue returning it.
- Given a full active-only project summary, then hidden query folders and queries are excluded and environment order contains only returned environments.
- Given a restored item and a replay of an older archive operation, then the original acknowledgement is returned and the item remains restored.
- Given a stale revision, changed operation payload, invalid scope, unsafe filesystem metadata or malformed inventory, then the mutation is refused.
- Given a visibility-only catalog, then archive/restore may be supported but Delete remains unavailable.
- Given a complete deletion inventory with retained references, then destructive deletion remains blocked.
- Given runtime inventory callbacks using normal project readers, then review, archive and discovery finish without lock reentry deadlock.
- Given legacy query definitions or supported current definition layouts, then the visibility catalog uses the same project object identity across reloads.
- Given concurrent archive commands with the same expected revision, then one transition succeeds and the stale command conflicts.

## Open Questions

None at this time.

---

*This document follows the https://specscore.md/feature-specification*
