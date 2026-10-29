package sessioncontext

import (
	"context"
	"reflect"
	"sort"
)

// RetainedContextBinding is private input to the owning provider's reference
// resolver. Subject and Scope are complete semantic descriptors, not project
// object IDs. Namespace identifies storage; Source may name a different owner's
// session in that same repository. No source values or prose are exposed.
type RetainedContextBinding struct {
	Namespace                                    NativeNamespace
	LifecycleSessionID                           string
	Owner, SessionID, Collection, ID, WorkItemID string
	Active                                       bool
	Scopes                                       []Scope
	Subjects                                     []RetainedScopedSubject
	Source                                       *ContextSource
	LegacySourceID, SourceDigest                 string
}

type RetainedScopedSubject struct {
	Scope   Scope
	Subject Subject
}

// ProjectRetainedContextBindings validates all retained records in the five
// context collections, including heads removed by clearBag and old applied
// sets. Current references are flattened so an active session cannot hide a
// dependency behind a historical revision. It does not project execution pins,
// receipts, audits, messages, imports or redaction loss; those families require
// independent owning projectors. This result never certifies complete usage.
func ProjectRetainedContextBindings(ctx context.Context, inventory RetainedInventory) ([]RetainedContextBinding, error) {
	if ctx == nil {
		return nil, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	r, err := retainedReader(inventory)
	if err != nil {
		return nil, err
	}
	sessions := map[string]Session{}
	bags := map[string]Bag{}
	entries := map[string]Entry{}
	revisions := map[string]EntryRevision{}
	sets := map[string]AppliedContextSet{}
	keys := make([]string, 0, len(r))
	for key := range r {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	unknown := func() ([]RetainedContextBinding, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, ErrUnavailable
	}
	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		d := r[key]
		switch d.Collection {
		case Sessions:
			var v Session
			if decodeRetainedSource(d.Payload, &v) != nil || v.ID != d.ID || v.ID != d.SessionID || v.Owner != d.Owner || v.Version != d.Version || v.SchemaVersion != 1 || v.ProjectID != inventory.Namespace.ProjectID || v.DatabaseID != inventory.Namespace.DatabaseID || !safeID.MatchString(v.DefaultBagID) || !safeID.MatchString(v.ContextSetID) || len(v.BagIDs) == 0 || len(v.BagIDs) > 64 || v.WorkItemID != "" && !safeID.MatchString(v.WorkItemID) || v.PendingRunCount < 0 || v.CreatedAt.IsZero() || v.UpdatedAt.IsZero() {
				return unknown()
			}
			if v.State != "active" && v.State != "stopped" && v.State != "archived" && v.State != "deleted" {
				return unknown()
			}
			seen := map[string]bool{}
			for _, id := range v.BagIDs {
				if !safeID.MatchString(id) || seen[id] {
					return unknown()
				}
				seen[id] = true
			}
			if !seen[v.DefaultBagID] {
				return unknown()
			}
			sessions[d.ID] = v
		case Bags:
			var v Bag
			if decodeRetainedSource(d.Payload, &v) != nil || v.ID != d.ID || v.SessionID != d.SessionID || v.Version != d.Version || !validScope(&v.Scope) || v.ClearVersion < 0 || v.ClearVersion > v.Version || len(v.EntryIDs) > 1000 {
				return unknown()
			}
			seen := map[string]bool{}
			for _, id := range v.EntryIDs {
				if !safeID.MatchString(id) || seen[id] {
					return unknown()
				}
				seen[id] = true
			}
			bags[d.ID] = v
		case Entries:
			var v Entry
			if decodeRetainedSource(d.Payload, &v) != nil || v.ID != d.ID || v.SessionID != d.SessionID || v.Version != d.Version || !safeID.MatchString(v.BagID) || !safeID.MatchString(v.CurrentRevisionID) || !validAlias(v.Alias) || !retainedSubjectPresent(v.Subject) {
				return unknown()
			}
			entries[d.ID] = v
		case Revisions:
			var v EntryRevision
			if d.Version != 1 || decodeRetainedSource(d.Payload, &v) != nil || v.ID != d.ID || !safeID.MatchString(v.EntryID) || !safeID.MatchString(v.BagID) || v.ParentRevisionID != "" && !safeID.MatchString(v.ParentRevisionID) || !retainedSubjectPresent(v.Subject) || validateValue(v.Value) != nil {
				return unknown()
			}
			switch v.Provenance {
			case "":
				if v.Source != nil || v.LegacySourceID != "" || v.SourceDigest != "" || v.CreatedAt.IsZero() {
					return unknown()
				}
			case "work-context-copy":
				if v.Source == nil || v.LegacySourceID != "" || v.SourceDigest != "" || v.CreatedAt.IsZero() {
					return unknown()
				}
			case "legacy-import":
				// The import projector must separately verify source provenance.
				if v.Source != nil || v.LegacySourceID == "" || v.SourceDigest == "" {
					return unknown()
				}
			default:
				return unknown()
			}
			revisions[d.ID] = v
		case ContextSets:
			var v AppliedContextSet
			if d.Version != 1 || decodeRetainedSource(d.Payload, &v) != nil || v.ID != d.ID || v.SessionID != d.SessionID || len(v.Slots) > 64 || v.CreatedAt.IsZero() {
				return unknown()
			}
			sets[d.ID] = v
		}
	}
	owned := func(collection, id, session string) bool {
		d, ok := r[collection+"/"+id]
		s, found := sessions[session]
		return ok && found && d.SessionID == session && d.Owner == s.Owner
	}
	for id, s := range sessions {
		if !owned(ContextSets, s.ContextSetID, id) {
			return unknown()
		}
		for _, bag := range s.BagIDs {
			if !owned(Bags, bag, id) {
				return unknown()
			}
		}
	}
	for id, b := range bags {
		if !owned(Bags, id, b.SessionID) || !contains(sessions[b.SessionID].BagIDs, id) {
			return unknown()
		}
		for _, entry := range b.EntryIDs {
			if !owned(Entries, entry, b.SessionID) || entries[entry].BagID != id {
				return unknown()
			}
		}
	}
	for id, e := range entries {
		if !owned(Entries, id, e.SessionID) || !owned(Bags, e.BagID, e.SessionID) || !owned(Revisions, e.CurrentRevisionID, e.SessionID) {
			return unknown()
		}
		head := revisions[e.CurrentRevisionID]
		if head.EntryID != id || head.BagID != e.BagID {
			return unknown()
		}
	}
	dependencies := map[string][]string{}
	for id, v := range revisions {
		session := r[Revisions+"/"+id].SessionID
		if !owned(Revisions, id, session) || !owned(Entries, v.EntryID, session) || entries[v.EntryID].BagID != v.BagID || !owned(Bags, v.BagID, session) {
			return unknown()
		}
		if v.ParentRevisionID != "" {
			parent, ok := revisions[v.ParentRevisionID]
			if !ok || parent.EntryID != v.EntryID || parent.BagID != v.BagID || !owned(Revisions, parent.ID, session) {
				return unknown()
			}
			dependencies[id] = append(dependencies[id], parent.ID)
		}
		if source := v.Source; source != nil {
			original, ok := revisions[source.RevisionID]
			if !ok || !safeID.MatchString(source.SessionID) || !safeID.MatchString(source.BagID) || !safeID.MatchString(source.EntryID) || !safeID.MatchString(source.RevisionID) || !owned(Revisions, source.RevisionID, source.SessionID) || original.EntryID != source.EntryID || original.BagID != source.BagID || bags[source.BagID].Scope != bags[v.BagID].Scope || !reflect.DeepEqual(original.Subject, v.Subject) || !reflect.DeepEqual(original.Value, v.Value) {
				return unknown()
			}
			dependencies[id] = append(dependencies[id], original.ID)
		}
	}
	// Check destination chains and copy provenance together, in linear time.
	remaining := map[string]int{}
	children := map[string][]string{}
	queue := []string{}
	for id := range revisions {
		remaining[id] = len(dependencies[id])
		if remaining[id] == 0 {
			queue = append(queue, id)
		}
		for _, parent := range dependencies[id] {
			children[parent] = append(children[parent], id)
		}
	}
	visited := 0
	for len(queue) != 0 {
		id := queue[0]
		queue = queue[1:]
		visited++
		for _, child := range children[id] {
			remaining[child]--
			if remaining[child] == 0 {
				queue = append(queue, child)
			}
		}
	}
	if visited != len(revisions) {
		return unknown()
	}
	for id, set := range sets {
		if !owned(ContextSets, id, set.SessionID) {
			return unknown()
		}
		current := sessions[set.SessionID].ContextSetID == id
		seenScopes := map[Scope]bool{}
		count := 0
		for _, slot := range set.Slots {
			if !validScope(&slot.Scope) || seenScopes[slot.Scope] || slot.Version < 1 {
				return unknown()
			}
			seenScopes[slot.Scope] = true
			if slot.BagID == "" {
				if slot.BagVersion != 0 || len(slot.Entries) != 0 {
					return unknown()
				}
				continue
			}
			b, ok := bags[slot.BagID]
			if !ok || !owned(Bags, b.ID, set.SessionID) || b.Scope != slot.Scope || slot.BagVersion < 1 || slot.BagVersion > b.Version || current && slot.BagVersion < b.ClearVersion || len(slot.Entries) > MaxSnapshotEntries-count {
				return unknown()
			}
			count += len(slot.Entries)
			seen := map[string]bool{}
			for _, ref := range slot.Entries {
				e, exists := entries[ref.EntryID]
				v, present := revisions[ref.RevisionID]
				if !exists || !present || seen[ref.EntryID] || e.BagID != b.ID || !owned(Entries, e.ID, set.SessionID) || v.EntryID != e.ID || v.BagID != b.ID || !owned(Revisions, v.ID, set.SessionID) || current && !contains(b.EntryIDs, e.ID) {
					return unknown()
				}
				seen[e.ID] = true
			}
		}
	}
	out := make([]RetainedContextBinding, 0, len(sessions)+len(bags)+len(entries)+len(revisions)+len(sets))
	remainingBytes, remainingSubjects := 16<<20, 10000
	charge := func(value any) bool {
		encoded, err := marshalRetainedSource(value)
		if err != nil || len(encoded)+64 > remainingBytes {
			return false
		}
		remainingBytes -= len(encoded) + 64
		return true
	}
	addSubject := func(binding *RetainedContextBinding, scope Scope, subject Subject) bool {
		value := RetainedScopedSubject{scope, subject}
		if remainingSubjects == 0 || !charge(value) {
			return false
		}
		remainingSubjects--
		binding.Subjects = append(binding.Subjects, value)
		return true
	}
	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		d := r[key]
		s := sessions[d.SessionID]
		b := RetainedContextBinding{Namespace: inventory.Namespace, Owner: d.Owner, SessionID: d.SessionID, Collection: d.Collection, ID: d.ID, WorkItemID: s.WorkItemID, Active: s.State == "active" || s.State == "stopped"}
		b.LifecycleSessionID, err = retainedLifecycleIdentity(inventory.Namespace, d.SessionID)
		if err != nil {
			return unknown()
		}
		if d.Collection == Sessions || d.Collection == Bags || d.Collection == Entries || d.Collection == Revisions || d.Collection == ContextSets {
			if !charge(b) {
				return unknown()
			}
		}
		switch d.Collection {
		case Sessions:
		case Bags:
			b.Scopes = []Scope{bags[d.ID].Scope}
		case Entries:
			e := entries[d.ID]
			b.Active = b.Active && contains(bags[e.BagID].EntryIDs, e.ID)
			b.Scopes = []Scope{bags[e.BagID].Scope}
			if !addSubject(&b, bags[e.BagID].Scope, e.Subject) || !addSubject(&b, bags[e.BagID].Scope, revisions[e.CurrentRevisionID].Subject) {
				return unknown()
			}
		case Revisions:
			v := revisions[d.ID]
			b.Active = false
			b.Scopes = []Scope{bags[v.BagID].Scope}
			if !addSubject(&b, bags[v.BagID].Scope, v.Subject) {
				return unknown()
			}
			b.Source = v.Source
			b.LegacySourceID, b.SourceDigest = v.LegacySourceID, v.SourceDigest
			if !charge(struct {
				Source                       *ContextSource
				LegacySourceID, SourceDigest string
			}{b.Source, b.LegacySourceID, b.SourceDigest}) {
				return unknown()
			}
		case ContextSets:
			set := sets[d.ID]
			b.Active = b.Active && s.ContextSetID == set.ID
			for _, slot := range set.Slots {
				b.Scopes = append(b.Scopes, slot.Scope)
				for _, ref := range slot.Entries {
					if !addSubject(&b, slot.Scope, revisions[ref.RevisionID].Subject) {
						return unknown()
					}
				}
			}
		default:
			continue
		}
		if !charge(b.Scopes) {
			return unknown()
		}
		out = append(out, b)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func retainedSubjectPresent(subject Subject) bool {
	return subject.ConceptID != "" && subject.FieldID != "" && subject.FieldMappingRef != ""
}
