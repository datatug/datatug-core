package sessioncontext

import (
	"context"
	"encoding/hex"
	"strings"
)

// RetainedSessionResource names an internal session resource, never an inferred
// saved query or project item. SessionID preserves cross-session copy targets.
type RetainedSessionResource struct{ Collection, ID, SessionID string }

// RetainedSessionProvenance exposes reference metadata only. Every row is
// historical; values, message text and command payloads are not returned.
// Resource identities preserve provenance joins. The owning resolver must
// distinguish actual retained content/use from identity-only audit metadata;
// these identities alone do not establish lifecycle-blocking references.
type RetainedSessionProvenance struct {
	Namespace                                NativeNamespace
	LifecycleSessionID                       string
	Owner, SessionID, Collection, ID, Action string
	Scopes                                   []Scope
	Resources                                []RetainedSessionResource
	LegacySourceID, SourceDigest             string
}

func retainedDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32 && strings.ToLower(value) == value
}

// Context and execution joins must have succeeded before this owning metadata
// reader runs. Redacted pins require an independently retained dependency index;
// without that index the entire source result remains unknown, even for a valid
// marker. No actor/expiry/presentation filter is used here.
func retainedSessionProvenance(ctx context.Context, r retainedInventoryReader, namespace NativeNamespace, keys []string, budget *retainedSessionSourceBudget) ([]RetainedSessionProvenance, error) {
	unknown := func() ([]RetainedSessionProvenance, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, ErrUnavailable
	}
	sessions := map[string]Session{}
	imports := map[string]LegacyImportManifest{}
	importsBySession := map[string]LegacyImportManifest{}
	receipts := map[string]Receipt{}
	runsByInput := map[string]Run{}
	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		d := r[key]
		switch d.Collection {
		case Redactions:
			return unknown()
		case Sessions:
			var value Session
			if decodeRetainedSource(d.Payload, &value) != nil {
				return unknown()
			}
			sessions[d.ID] = value
		case LegacyImports:
			var value LegacyImportManifest
			if d.Version != 1 || decodeRetainedSource(d.Payload, &value) != nil || value.ID != d.ID || value.SessionID != d.SessionID || !safeID.MatchString(value.SourceID) || !validScope(&value.Scope) || !retainedDigest(value.SourceDigest) || value.CreatedAt.IsZero() {
				return unknown()
			}
			identity := legacyHash([]string{d.Owner, namespace.DatabaseID, namespace.ProjectID, value.Scope.EnvironmentID, value.Scope.IdentityDomain, value.SourceID})
			if value.ID != "li_"+identity || value.SessionID != "legacy_s_"+identity || value.BagID != "legacy_b_"+identity {
				return unknown()
			}
			imports[d.ID] = value
			if _, duplicate := importsBySession[value.SessionID]; duplicate {
				return unknown()
			}
			importsBySession[value.SessionID] = value
		case Runs:
			var value Run
			if decodeRetainedSource(d.Payload, &value) != nil {
				return unknown()
			}
			if _, duplicate := runsByInput[value.InputSnapshotID]; duplicate {
				return unknown()
			}
			runsByInput[value.InputSnapshotID] = value
		case Operations:
			var value Receipt
			if d.Version != 1 || decodeRetainedSource(d.Payload, &value) != nil || value.Actor != d.Owner || value.SessionID != d.SessionID || !safeID.MatchString(value.OpID) || !retainedDigest(value.PayloadDigest) || value.At.IsZero() || value.SessionVersion < 1 || value.ResourceVersion < 0 || value.Scope != nil && !validScope(value.Scope) {
				return unknown()
			}
			if d.ID != opKey(Principal{Actor: value.Actor, DatabaseID: namespace.DatabaseID, ProjectID: namespace.ProjectID}, value.OpID) || value.RecoveryBindingDigest != "" && (value.Action != "pinDetectiveInput" || !retainedDigest(value.RecoveryBindingDigest)) {
				return unknown()
			}
			receipts[d.ID] = value
		}
	}
	resource := func(collection, id, session string, version int64) (RetainedSessionResource, bool) {
		d, exists := r[collection+"/"+id]
		s, found := sessions[session]
		if !exists || !found || d.SessionID != session || d.Owner != s.Owner || version > d.Version || version < 0 {
			return RetainedSessionResource{}, false
		}
		return RetainedSessionResource{collection, id, session}, true
	}
	for _, value := range imports {
		s, ok := sessions[value.SessionID]
		d := r[LegacyImports+"/"+value.ID]
		var bag Bag
		if !ok || s.WorkItemID != "" || s.Owner != d.Owner || s.DefaultBagID != value.BagID || !s.CreatedAt.Equal(value.CreatedAt) {
			return unknown()
		}
		if _, err := r.Load(ctx, Bags, value.BagID, &bag); err != nil || bag.Scope != value.Scope {
			return unknown()
		}
		if _, ok := resource(Bags, value.BagID, s.ID, 1); !ok {
			return unknown()
		}
	}
	// Every current ordinary/native pin command has a retained receipt and
	// ordinary audit; native settlement is the explicit audit-only exception.
	for id, value := range receipts {
		s, ok := sessions[value.SessionID]
		if !ok || value.SessionVersion > s.Version || s.WorkItemID == "" && value.Actor != s.Owner {
			return unknown()
		}
		a, exists := r[Audit+"/"+id]
		var event AuditEvent
		if !exists || a.Version != 1 || a.Owner != value.Actor || a.SessionID != s.ID || decodeRetainedSource(a.Payload, &event) != nil || event.OpID != value.OpID || event.Action != value.Action || event.Actor != value.Actor || event.SessionID != s.ID || !event.At.Equal(value.At) || event.Count < 0 {
			return unknown()
		}
		if value.Action != "copyContext" && event.ResourceID != value.ResourceID {
			return unknown()
		}
	}
	out := []RetainedSessionProvenance{}
	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		d := r[key]
		item := RetainedSessionProvenance{Namespace: namespace, Owner: d.Owner, SessionID: d.SessionID, Collection: d.Collection, ID: d.ID}
		var err error
		item.LifecycleSessionID, err = retainedLifecycleIdentity(namespace, d.SessionID)
		if err != nil {
			return unknown()
		}
		s, ok := sessions[d.SessionID]
		if !ok {
			return unknown()
		}
		switch d.Collection {
		case LegacyImports:
			v := imports[d.ID]
			item.Scopes = []Scope{v.Scope}
			item.Resources = []RetainedSessionResource{{Sessions, s.ID, s.ID}, {Bags, v.BagID, s.ID}}
			item.LegacySourceID, item.SourceDigest = v.SourceID, v.SourceDigest
		case Operations:
			v := receipts[d.ID]
			item.Action = v.Action
			if v.Scope != nil {
				item.Scopes = []Scope{*v.Scope}
			}
			collection := ""
			switch v.Action {
			case "createSession", "createBag", "renameBag", "applyContext", "clearBag":
				collection = Bags
			case "putEntry":
				collection = Entries
			case "renameSession", "transitionSession", "importLegacy", "copyContext":
				collection = Sessions
			case "pinInput", "pinDetectiveInput", "completeRun", "recordRunExecution", "reportDashboard", "reportClientRun":
				collection = Runs
			default:
				return unknown()
			}
			if v.Action == "transitionSession" {
				if v.ResourceVersion != 0 {
					return unknown()
				}
			} else if v.ResourceVersion < 1 {
				return unknown()
			}
			ref, ok := resource(collection, v.ResourceID, s.ID, v.ResourceVersion)
			if !ok {
				return unknown()
			}
			item.Resources = []RetainedSessionResource{{Sessions, s.ID, s.ID}, ref}
		case Audit:
			var event AuditEvent
			if d.Version != 1 || decodeRetainedSource(d.Payload, &event) != nil || event.SessionID != d.SessionID || event.Actor != d.Owner || event.Count < 0 || event.At.IsZero() {
				return unknown()
			}
			item.Action = event.Action
			if event.Action == "nativeComplete" {
				var run Run
				if _, err := r.Load(ctx, Runs, event.ResourceID, &run); err != nil || run.InputReference == nil || run.DetectiveOperationID != event.OpID || run.SessionID != s.ID || event.Actor != s.Owner || d.ID != opKey(Principal{Actor: event.Actor, DatabaseID: namespace.DatabaseID, ProjectID: namespace.ProjectID}, "nativeComplete-"+run.ID) || event.Count != 0 {
					return unknown()
				}
				if run.State != "completed" && run.State != "failed" && run.State != "canceled" || run.CompletedAt == nil || !event.At.Equal(*run.CompletedAt) {
					return unknown()
				}
				ref, ok := resource(Runs, run.ID, s.ID, 1)
				if !ok {
					return unknown()
				}
				item.Resources = []RetainedSessionResource{{Sessions, s.ID, s.ID}, ref}
			} else {
				v, exists := receipts[d.ID]
				if !exists || v.OpID != event.OpID || v.Actor != event.Actor || v.Action != event.Action || v.SessionID != s.ID {
					return unknown()
				}
				item.Resources = []RetainedSessionResource{{Operations, d.ID, s.ID}}
				if event.Action == "copyContext" {
					if event.ResourceID == s.ID {
						return unknown()
					}
					ref, ok := resource(Sessions, event.ResourceID, event.ResourceID, 1)
					if !ok {
						return unknown()
					}
					item.Resources = append(item.Resources, ref)
				}
			}
		case Messages:
			var message Message
			if d.Version != 1 || decodeRetainedSource(d.Payload, &message) != nil || message.ID != d.ID || message.SessionID != s.ID || d.Owner != s.Owner || message.Role != "user" && message.Role != "assistant" || len(message.Text) > 100000 || message.CreatedAt.IsZero() {
				return unknown()
			}
			if message.Provenance == "legacy-import" {
				manifest, exists := importsBySession[s.ID]
				if !exists || message.InputSnapshotID != "" || message.LegacySourceID != manifest.SourceID || message.SourceDigest != manifest.SourceDigest || message.SourceScope != manifest.Scope {
					return unknown()
				}
				item.Scopes = []Scope{message.SourceScope}
				item.Resources = []RetainedSessionResource{{LegacyImports, manifest.ID, s.ID}}
				item.LegacySourceID, item.SourceDigest = message.LegacySourceID, message.SourceDigest
			} else {
				if message.Provenance != "" || message.LegacySourceID != "" || message.SourceDigest != "" || !message.HistoricalAt.IsZero() || message.SourceScope != (Scope{}) {
					return unknown()
				}
				var input InputSnapshot
				if _, err := r.Load(ctx, Inputs, message.InputSnapshotID, &input); err != nil || input.SessionID != s.ID || input.Detective != nil {
					return unknown()
				}
				matched, exists := runsByInput[input.ID]
				if !exists || matched.SessionID != s.ID || matched.Kind != "chat" || message.Role == "user" && message.ID != matched.ID || message.Role == "assistant" && message.ID != assistantMessageID(matched.ID) {
					return unknown()
				}
				if _, ok := resource(Inputs, input.ID, s.ID, 1); !ok {
					return unknown()
				}
				item.Scopes = []Scope{input.Scope}
				item.Resources = []RetainedSessionResource{{Inputs, input.ID, s.ID}, {Runs, matched.ID, s.ID}}
			}
		case Revisions:
			var revision EntryRevision
			if decodeRetainedSource(d.Payload, &revision) != nil {
				return unknown()
			}
			if revision.Provenance != "legacy-import" {
				continue
			}
			manifest, exists := importsBySession[s.ID]
			if !exists || manifest.BagID != revision.BagID || manifest.SourceID != revision.LegacySourceID || manifest.SourceDigest != revision.SourceDigest {
				return unknown()
			}
			continue // its scoped subject was already projected by the context reader
		default:
			continue
		}
		if !budget.charge(item, 0) {
			return unknown()
		}
		out = append(out, item)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
