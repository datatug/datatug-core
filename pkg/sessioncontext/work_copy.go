package sessioncontext

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
)

// CopyContextCommand selects exact current revisions. Both session heads are
// compared; refresh never replaces an entry subsequently edited in the child.
// Selection must be nonempty. Copied entries remain unapplied until applyContext.
type CopyContextCommand struct {
	OpID, SourceSessionID, DestinationSessionID       string
	ExpectedSourceVersion, ExpectedDestinationVersion int64
	Selection                                         []EntryRef
}

func copyID(kind string, parts ...string) string {
	encoded, _ := json.Marshal(parts)
	sum := sha256.Sum256(encoded)
	return kind + hex.EncodeToString(sum[:])
}

// CopyContext copies typed values and named bags within one transaction and
// project. source and destination must use the same Repository instance. A
// private destination must belong to the real actor; this is the execution
// snapshot path, not delegation of native execution ownership.
func CopyContext(ctx context.Context, p Principal, source, destination *Service, c CopyContextCommand) (CommandResult, error) {
	var result CommandResult
	if source == nil || destination == nil || !sameRepository(source.repo, destination.repo) || !safeID.MatchString(c.OpID) || !safeID.MatchString(c.SourceSessionID) || !safeID.MatchString(c.DestinationSessionID) || c.SourceSessionID == c.DestinationSessionID || c.ExpectedSourceVersion < 1 || c.ExpectedDestinationVersion < 1 || len(c.Selection) == 0 || len(c.Selection) > MaxSnapshotEntries {
		return result, ErrInvalid
	}
	selection := append([]EntryRef(nil), c.Selection...)
	sort.Slice(selection, func(i, j int) bool { return selection[i].EntryID < selection[j].EntryID })
	for i, ref := range selection {
		if !safeID.MatchString(ref.EntryID) || !safeID.MatchString(ref.RevisionID) || i > 0 && selection[i-1].EntryID == ref.EntryID {
			return result, ErrInvalid
		}
	}
	c.Selection = selection
	digest := copyID("", p.Actor, p.DatabaseID, p.ProjectID, c.OpID, c.SourceSessionID, c.DestinationSessionID)
	encoded, _ := json.Marshal(c)
	digest = copyID("", digest, string(encoded))
	err := destination.repo.Transact(ctx, func(tx Transaction) error {
		src, err := source.session(ctx, tx, p, c.SourceSessionID, "copyContext")
		if err != nil {
			return err
		}
		dst, err := destination.session(ctx, tx, p, c.DestinationSessionID, "copyContext")
		if err != nil {
			return err
		}
		if src.State != "active" || dst.State != "active" {
			return ErrReadOnly
		}
		// Reauthorize all source and destination scopes before operation replay.
		snap, err := source.loadSnapshot(ctx, tx, p, src, "copyContext")
		if err != nil {
			return err
		}
		if _, err = destination.loadSnapshot(ctx, tx, p, dst, "copyContext"); err != nil {
			return err
		}
		key := opKey(p, c.OpID)
		var old Receipt
		if _, err = tx.Load(ctx, Operations, key, &old); err == nil {
			if old.Actor != p.Actor || old.Action != "copyContext" || old.PayloadDigest != digest || old.SessionID != dst.ID {
				return ErrConflict
			}
			result.Receipt = old
			return nil
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
		if src.Version != c.ExpectedSourceVersion || dst.Version != c.ExpectedDestinationVersion {
			return ErrConflict
		}
		entries := map[string]Entry{}
		revisions := map[string]EntryRevision{}
		bags := map[string]Bag{}
		for _, entry := range snap.Entries {
			entries[entry.ID] = entry
		}
		for _, revision := range snap.Revisions {
			revisions[revision.ID] = revision
		}
		for _, bag := range snap.Bags {
			bags[bag.ID] = bag
		}
		targetBags := map[string]Bag{}
		originalVersions := map[string]int64{}
		now := destination.now()
		for _, ref := range selection {
			entry, ok := entries[ref.EntryID]
			if !ok || entry.CurrentRevisionID != ref.RevisionID {
				return ErrConflict
			}
			revision, ok := revisions[ref.RevisionID]
			if !ok {
				return ErrUnavailable
			}
			fromBag := bags[entry.BagID]
			if err = destination.authorizeCopy(ctx, p, dst.ID, &fromBag.Scope); err != nil {
				return err
			}
			bagID := copyID("cb_", src.ID, fromBag.ID, dst.ID)
			bag, loaded := targetBags[bagID]
			if !loaded {
				version, e := tx.Load(ctx, Bags, bagID, &bag)
				if errors.Is(e, ErrNotFound) {
					if len(dst.BagIDs) >= 64 {
						return ErrInvalid
					}
					bag = Bag{ID: bagID, SessionID: dst.ID, Title: fromBag.Title, Scope: fromBag.Scope, Version: 1, EntryIDs: []string{}}
					dst.BagIDs = append(dst.BagIDs, bagID)
				} else if e != nil {
					return e
				} else {
					if bag.SessionID != dst.ID || bag.Scope != fromBag.Scope || !contains(dst.BagIDs, bagID) || bag.Version != version || bag.ClearVersion != 0 {
						return ErrConflict
					}
					bag.Version++
				}
				originalVersions[bagID] = version
			}
			entryID := copyID("ce_", src.ID, entry.ID, dst.ID)
			var previous Entry
			entryVersion, e := tx.Load(ctx, Entries, entryID, &previous)
			if e != nil && !errors.Is(e, ErrNotFound) {
				return e
			}
			parentRevision := ""
			if entryVersion > 0 {
				if previous.BagID != bagID || previous.SessionID != dst.ID || !contains(bag.EntryIDs, entryID) {
					return ErrConflict
				}
				var prior EntryRevision
				if _, e = tx.Load(ctx, Revisions, previous.CurrentRevisionID, &prior); e != nil {
					return e
				}
				if prior.Source == nil || prior.Source.SessionID != src.ID || prior.Source.BagID != fromBag.ID || prior.Source.EntryID != entry.ID || prior.Provenance != "work-context-copy" {
					return ErrConflict
				}
				parentRevision = prior.ID
			} else {
				bag.EntryIDs = append(bag.EntryIDs, entryID)
			}
			// A new source alias must not collide with a locally added entry.
			for _, otherID := range bag.EntryIDs {
				if otherID == entryID || normalizedAlias(entry.Alias) == "" {
					continue
				}
				var other Entry
				if _, err = tx.Load(ctx, Entries, otherID, &other); err != nil {
					return err
				}
				if normalizedAlias(other.Alias) == normalizedAlias(entry.Alias) {
					return ErrConflict
				}
			}
			revisionID := copyID("cr_", dst.ID, c.OpID, entry.ID, ref.RevisionID)
			target := Entry{ID: entryID, SessionID: dst.ID, BagID: bagID, Alias: entry.Alias, Subject: entry.Subject, CurrentRevisionID: revisionID, Version: entryVersion + 1}
			copied := EntryRevision{ID: revisionID, EntryID: entryID, BagID: bagID, ParentRevisionID: parentRevision, Subject: revision.Subject, Value: revision.Value, CreatedAt: now, Provenance: "work-context-copy", Source: &ContextSource{SessionID: src.ID, BagID: fromBag.ID, EntryID: entry.ID, RevisionID: revision.ID}}
			if err = tx.Save(ctx, Revisions, revisionID, copied, 0, destination.indexOwner(p), dst.ID); err != nil {
				return err
			}
			if err = tx.Save(ctx, Entries, entryID, target, entryVersion, destination.indexOwner(p), dst.ID); err != nil {
				return err
			}
			targetBags[bagID] = bag
		}
		for id, bag := range targetBags {
			if err = tx.Save(ctx, Bags, id, bag, originalVersions[id], destination.indexOwner(p), dst.ID); err != nil {
				return err
			}
		}
		dst.Version++
		dst.UpdatedAt = now
		if _, err = destination.loadSnapshot(ctx, tx, p, dst, "copyContext"); err != nil {
			return err
		}
		if err = tx.Save(ctx, Sessions, dst.ID, dst, c.ExpectedDestinationVersion, destination.indexOwner(p), dst.ID); err != nil {
			return err
		}
		receipt := Receipt{OpID: c.OpID, Actor: p.Actor, Action: "copyContext", PayloadDigest: digest, SessionID: dst.ID, SessionVersion: dst.Version, ResourceID: dst.ID, ResourceVersion: dst.Version, At: now}
		event := AuditEvent{OpID: c.OpID, Actor: p.Actor, Action: "copyContext", SessionID: dst.ID, ResourceID: src.ID, Count: len(selection), At: now}
		if err = tx.Save(ctx, Audit, key, event, 0, p.Actor, dst.ID); err != nil {
			return err
		}
		if err = tx.Save(ctx, Operations, key, receipt, 0, p.Actor, dst.ID); err != nil {
			return err
		}
		result.Receipt = receipt
		return nil
	})
	if err != nil {
		return CommandResult{}, err
	}
	return result, nil
}
func sameRepository(a, b Repository) bool {
	av, bv := reflect.ValueOf(a), reflect.ValueOf(b)
	return av.IsValid() && bv.IsValid() && av.Kind() == reflect.Pointer && bv.Kind() == reflect.Pointer && av.Type() == bv.Type() && av.Pointer() == bv.Pointer()
}
