package sessioncontext

import (
	"context"
	"encoding/json"
	"sort"
)

// RetainedOrdinaryExecution is a private, immutable pin projection, including
// executable SQL/TugQL. It must never be returned in a lifecycle response.
// DefinitionID/Revision are opaque caller provenance, NOT saved project query
// identity. Query/widget IDs inside the documents are document-local. A host
// needs an owning source binding before resolving project definitions.
type RetainedOrdinaryExecution struct {
	Namespace                                                    NativeNamespace
	LifecycleSessionID                                           string
	Owner, SessionID, RunID, InputSnapshotID, ContextSetID       string
	Kind, DefinitionID, DefinitionRevision                       string
	State, ExecutionAuthority, DefinitionDigest, ExecutionDigest string
	Scope                                                        Scope
	// Active conservatively includes client outcome-unknown: client settlement
	// clears operational pending bookkeeping but proves no source completion.
	Active                                bool
	DefinitionDocument, ExecutionDocument json.RawMessage
	Subjects                              []RetainedScopedSubject
	ResultRefs                            []string
	Widgets                               []WidgetOutcome
}

// RetainedSessionSources joins context and execution sources across all owners.
// Redaction loss and unsupported legacy handoffs remain unknown. Project source
// identity still needs the owning provider resolver. There is no Complete bit.
type RetainedSessionSources struct {
	Contexts   []RetainedContextBinding
	Native     []RetainedDetectiveExecution
	Ordinary   []RetainedOrdinaryExecution
	Provenance []RetainedSessionProvenance
}

type retainedSessionSourceBudget struct{ bytes, subjects int }

var readRetainedSourceInventory = retainedReader

func (b *retainedSessionSourceBudget) charge(value any, subjects int) bool {
	if subjects < 0 || subjects > b.subjects {
		return false
	}
	encoded, err := marshalRetainedSource(value)
	if err != nil || len(encoded)+64 > b.bytes {
		return false
	}
	b.bytes -= len(encoded) + 64
	b.subjects -= subjects
	return true
}

func ProjectRetainedSessionSources(ctx context.Context, inventory RetainedInventory) (RetainedSessionSources, error) {
	unknown := func() (RetainedSessionSources, error) {
		if ctx != nil {
			if err := ctx.Err(); err != nil {
				return RetainedSessionSources{}, err
			}
		}
		return RetainedSessionSources{}, ErrUnavailable
	}
	if ctx == nil {
		return RetainedSessionSources{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return RetainedSessionSources{}, err
	}
	r, err := readRetainedSourceInventory(inventory)
	if err != nil {
		return RetainedSessionSources{}, err
	}
	covered := map[string]bool{Sessions: true, Bags: true, Entries: true, Revisions: true, ContextSets: true, Inputs: true, Runs: true, DetectiveWork: true, DetectiveOwners: true, Outbox: true, Operations: true, Audit: true, Messages: true, LegacyImports: true, Redactions: true}
	for _, collection := range inventory.Collections {
		if !covered[collection] {
			return unknown()
		}
	}
	contexts, err := ProjectRetainedContextBindings(ctx, inventory)
	if err != nil {
		return RetainedSessionSources{}, err
	}
	inputs := map[string]InputSnapshot{}
	runs := map[string]Run{}
	sessions := map[string]Session{}
	keys := make([]string, 0, len(r))
	for key := range r {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return RetainedSessionSources{}, err
		}
		d := r[key]
		switch d.Collection {
		case Sessions:
			var value Session
			if decodeRetainedSource(d.Payload, &value) != nil {
				return unknown()
			}
			sessions[d.ID] = value
		case Inputs:
			var value InputSnapshot
			if d.Version != 1 || decodeRetainedSource(d.Payload, &value) != nil || value.ID != d.ID || value.SessionID != d.SessionID || !safeID.MatchString(value.ContextSetID) || !validScope(&value.Scope) || value.CreatedAt.IsZero() || len(value.Entries) > 1000 || validateResolvedParameters(value.ResolvedParameters) != nil {
				return unknown()
			}
			inputs[d.ID] = value
		case Runs:
			var value Run
			if decodeRetainedSource(d.Payload, &value) != nil || value.ID != d.ID || value.SessionID != d.SessionID || value.Version != d.Version || !safeID.MatchString(value.InputSnapshotID) || value.DefinitionID == "" || value.DefinitionRevision == "" || value.CreatedAt.IsZero() {
				return unknown()
			}
			runs[d.ID] = value
		}
	}
	usedInputs := map[string]bool{}
	ordinaryRuns := map[string]bool{}
	for id, run := range runs {
		input, ok := inputs[run.InputSnapshotID]
		session, found := sessions[run.SessionID]
		if !ok || !found || usedInputs[input.ID] || input.SessionID != run.SessionID || r[Runs+"/"+id].Owner != session.Owner || r[Inputs+"/"+input.ID].Owner != session.Owner {
			return unknown()
		}
		set, exists := r[ContextSets+"/"+input.ContextSetID]
		if !exists || set.Owner != session.Owner || set.SessionID != session.ID {
			return unknown()
		}
		usedInputs[input.ID] = true
		ordinaryRuns[id] = input.Detective == nil
	}
	if len(usedInputs) != len(inputs) {
		return unknown()
	}
	// The existing native projector retains its own acceptance/owner/settlement
	// joins. Partition only the explicitly ordinary run/input pair; never drop
	// auxiliary native rows that could contain orphan handoffs or acceptances.
	nativeInventory := inventory
	nativeInventory.Documents = make([]RetainedDocument, 0, len(inventory.Documents))
	ordinaryInputs := map[string]bool{}
	for id, ordinary := range ordinaryRuns {
		if ordinary {
			ordinaryInputs[runs[id].InputSnapshotID] = true
		}
	}
	for _, d := range inventory.Documents {
		if d.Collection == Runs && ordinaryRuns[d.ID] || d.Collection == Inputs && ordinaryInputs[d.ID] {
			continue
		}
		nativeInventory.Documents = append(nativeInventory.Documents, d)
	}
	native, err := ProjectRetainedDetectiveExecutions(ctx, nativeInventory)
	if err != nil {
		return RetainedSessionSources{}, err
	}
	budget := &retainedSessionSourceBudget{bytes: 16 << 20, subjects: 10000}
	for _, item := range contexts {
		if !budget.charge(item, len(item.Subjects)) {
			return unknown()
		}
	}
	for i := range native {
		if !budget.charge(native[i], 0) {
			return unknown()
		}
		native[i].Subjects, err = retainedInputSubjects(ctx, r, inputs[native[i].InputSnapshotID], budget)
		if err != nil {
			return RetainedSessionSources{}, err
		}
	}
	out := RetainedSessionSources{Contexts: contexts, Native: native, Ordinary: []RetainedOrdinaryExecution{}}
	pending := map[string]int{}
	for _, run := range native {
		if run.Active {
			pending[run.SessionID]++
		}
	}
	for _, key := range keys {
		d := r[key]
		if d.Collection != Runs || !ordinaryRuns[d.ID] {
			continue
		}
		if err := ctx.Err(); err != nil {
			return RetainedSessionSources{}, err
		}
		run := runs[d.ID]
		input := inputs[run.InputSnapshotID]
		if run.Kind != "query" && run.Kind != "dashboard" && run.Kind != "chat" || run.InputReference != nil || run.DetectiveOperationID != "" || run.ExecutionAuthority != "client" && run.ExecutionAuthority != "server" {
			return unknown()
		}
		origin := "client-observed"
		if run.ExecutionAuthority == "server" {
			origin = "server-executor"
		}
		if input.ParametersOrigin != origin {
			return unknown()
		}
		definition, digest, err := canonicalDefinitionDocument(run.Kind, input.DefinitionDocument)
		if err != nil || digest != input.DefinitionDigest {
			return unknown()
		}
		var execution json.RawMessage
		if len(run.ExecutionDocument) != 0 || run.ExecutionDigest != "" {
			if run.Kind != "chat" {
				return unknown()
			}
			execution, digest, err = canonicalDefinitionDocument("query", run.ExecutionDocument)
			if err != nil || digest != run.ExecutionDigest || validateResolvedParameters(run.ResolvedParameters) != nil {
				return unknown()
			}
		} else if len(run.ResolvedParameters) != 0 {
			return unknown()
		}
		pendingRun := run.State == "accepted"
		if pendingRun {
			if run.CompletedAt != nil || len(run.Widgets) != 0 || len(run.ResultRefs) != 0 || run.ErrorCode != "" || len(execution) == 0 && run.OutcomeOrigin != "" || len(execution) != 0 && run.OutcomeOrigin != origin {
				return unknown()
			}
			pending[run.SessionID]++
		} else {
			if run.Version < 2 || run.CompletedAt == nil || run.CompletedAt.IsZero() {
				return unknown()
			}
			if run.State == "outcome-unknown" {
				if run.ExecutionAuthority != "client" || run.OutcomeOrigin != "client-outcome-unknown" || run.ErrorCode != "CLIENT_OUTCOME_UNKNOWN" || len(run.Widgets) != 0 || len(run.ResultRefs) != 0 {
					return unknown()
				}
			} else {
				if validateRunOutcome(run.State, run.Widgets, run.ResultRefs, run.ErrorCode) != nil {
					return unknown()
				}
				expected := "client-reported"
				if run.ExecutionAuthority == "server" {
					expected = "server-executor"
				}
				if run.OutcomeOrigin != expected {
					return unknown()
				}
			}
		}
		active := pendingRun || run.State == "outcome-unknown"
		item := RetainedOrdinaryExecution{Namespace: inventory.Namespace, Owner: d.Owner, SessionID: run.SessionID, RunID: run.ID, InputSnapshotID: input.ID, ContextSetID: input.ContextSetID, Kind: run.Kind, DefinitionID: run.DefinitionID, DefinitionRevision: run.DefinitionRevision, State: run.State, ExecutionAuthority: run.ExecutionAuthority, DefinitionDigest: input.DefinitionDigest, ExecutionDigest: run.ExecutionDigest, Scope: input.Scope, Active: active, DefinitionDocument: definition, ExecutionDocument: execution, ResultRefs: run.ResultRefs, Widgets: run.Widgets}
		item.LifecycleSessionID, err = retainedLifecycleIdentity(inventory.Namespace, run.SessionID)
		if err != nil {
			return unknown()
		}
		if !budget.charge(item, 0) {
			return unknown()
		}
		item.Subjects, err = retainedInputSubjects(ctx, r, input, budget)
		if err != nil {
			return RetainedSessionSources{}, err
		}
		out.Ordinary = append(out.Ordinary, item)
	}
	for id, session := range sessions {
		if session.PendingRunCount != pending[id] {
			return unknown()
		}
	}
	out.Provenance, err = retainedSessionProvenance(ctx, r, inventory.Namespace, keys, budget)
	if err != nil {
		return RetainedSessionSources{}, err
	}
	if err := ctx.Err(); err != nil {
		return RetainedSessionSources{}, err
	}
	return out, nil
}

func retainedInputSubjects(ctx context.Context, r retainedInventoryReader, input InputSnapshot, budget *retainedSessionSourceBudget) ([]RetainedScopedSubject, error) {
	seen := map[string]bool{}
	out := make([]RetainedScopedSubject, 0, len(input.Entries))
	owner := r[Inputs+"/"+input.ID].Owner
	for _, ref := range input.Entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var entry Entry
		var revision EntryRevision
		var bag Bag
		if !safeID.MatchString(ref.EntryID) || !safeID.MatchString(ref.RevisionID) || seen[ref.EntryID] {
			return nil, ErrUnavailable
		}
		if _, err := r.Load(ctx, Entries, ref.EntryID, &entry); err != nil || entry.ID != ref.EntryID || entry.SessionID != input.SessionID {
			return nil, ErrUnavailable
		}
		if _, err := r.Load(ctx, Revisions, ref.RevisionID, &revision); err != nil || revision.ID != ref.RevisionID || revision.EntryID != entry.ID || revision.BagID != entry.BagID {
			return nil, ErrUnavailable
		}
		if _, err := r.Load(ctx, Bags, entry.BagID, &bag); err != nil || bag.ID != entry.BagID || bag.SessionID != input.SessionID || bag.Scope != input.Scope {
			return nil, ErrUnavailable
		}
		for _, doc := range []RetainedDocument{r[Entries+"/"+entry.ID], r[Revisions+"/"+revision.ID], r[Bags+"/"+bag.ID]} {
			if owner == "" || doc.Owner != owner || doc.SessionID != input.SessionID {
				return nil, ErrUnavailable
			}
		}
		// Immutable pin membership survives clearing today's bag. Keep its own
		// revision's Subject, including for native pins and cross-owner copies.
		subject := RetainedScopedSubject{input.Scope, revision.Subject}
		if !budget.charge(subject, 1) {
			return nil, ErrUnavailable
		}
		out = append(out, subject)
		seen[ref.EntryID] = true
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
