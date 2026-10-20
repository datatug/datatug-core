---
format: https://specscore.md/feature-specification
status: Draft
---

# Feature: DALGO project assets

> [SpecScore.**Studio**](https://specscore.studio): | [Explore](https://specscore.studio/app/github.com/datatug/datatug-core/spec/features/dalgo-project-assets?op=explore) | [Edit](https://specscore.studio/app/github.com/datatug/datatug-core/spec/features/dalgo-project-assets?op=edit) | [Ask question](https://specscore.studio/app/github.com/datatug/datatug-core/spec/features/dalgo-project-assets?op=ask) | [Request change](https://specscore.studio/app/github.com/datatug/datatug-core/spec/features/dalgo-project-assets?op=request-change) |
**Status:** Draft
**Source Ideas:** —

## Summary

A saved board or entity must survive a fresh read through the actual Go ProjectStore, including timeline/heatmap widgets and physical entity structure. Implement bounded DALGO asset members while preserving project ownership at the caller's admitted root.

## Problem

The existing DALGO ProjectStore exposes Board/Entity interfaces but returns ErrNotImplemented. Its global parent belongs to native stores; hosted Space project metadata has a different owning model and must never be overwritten by a generic store.

## Behavior

### REQ: scoped-items

The actual dalgostore.ProjectStore implements Board/Entity load/list/save/delete. Native NewProjectStore retains its global ext/datatug/projects identity. A separate item-only constructor accepts a server-selected validated project key and a DALGO ReadSession; writes require ReadwriteSession. It copies the parent path and never derives an authority boundary from a client-supplied path. Hosted parent LoadProjectFile/LoadProject/SaveProject refuse before database access; no parent delete capability is exposed. Native Store.DeleteProject refuses before deleting anything when boards/entities are nonempty.

### REQ: validated-bounded-codec

Store the actual Core Board/Entity wire through a lossless driver-neutral codec under the admitted parent boards/id and entities/id. Validate the DTO and key identity on both write and read; malformed or oversized stored data fails closed. Limit100items,256KiB per asset,2MiB aggregate lists, deterministic ordering, bounded101-row overflow detection. Reject invalid IDs and read-only mutations. Other unimplemented project collections and full-project graph operations keep explicit refusal.

### REQ: atomic-caller

A transaction-scoped store uses the supplied session directly without opening a nested transaction. The caller owns authority, expected revision, capacity admission, receipts and current reference checks. Shared DTO write conditions mirror SaveQuery: exactly one of ifNoneMatch or ifMatch; content revisions are kind-qualified deterministic hashes of the canonical asset. No provider credentials, inline query execution state or cached protected rows are introduced.

## Acceptance Criteria

### AC: roundtrip

Given a native or server-scoped project and valid boards/entities, when saved and loaded by a fresh actual ProjectStore, then Core validation and canonical JSON survive unchanged and sibling projects/spaces remain isolated. Timeline/heatmap and physical entity structure must not be silently dropped.

### AC: transaction-and-parent

Given a caller transaction and item-only store, when an asset plus receipt are written and the caller aborts, then neither remains. Parent metadata operations refuse without database access. Native project deletion with assets refuses and preserves parent/items.

### AC: limits-and-failures

Given invalid IDs, malformed stored content,100/101items, payload/list overflow, read-only sessions or failing DALGO reads/writes, then operations return explicit errors without partial/truncated success. List reads are deterministic and bounded.

## Review

[review r1 entity-table-codec] Independent Sol review found a blocker and two major issues in a proposed global DBCollectionKey codec. Accepted/fixed: methods live only on named TableKeys, so embedding owners keep columns/dbType/FKs; schema/refSchema and catalog/database remain distinct; typed integer parent IDs preserve exact decimal text/runtime integer kind. Name-only public fixture compatibility is preserved; nonempty legacy Ref and unsupported runtime aliases/scans fail explicitly. No findings declined. No global DBCollectionKey JSON methods are added.

## Open Questions

Backend current query/source authorization and operation receipts are owned by shared-project-assets. MeaningGraph publication, shipping provider/recovery and evidence-backed resolution are outside this capability.

---
*This document follows the https://specscore.md/feature-specification*
