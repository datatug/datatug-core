package sessioncontext

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"sort"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/detective"
)

// RetainedDetectiveExecution is a validated, private source projection. Native
// target identity remains distinct from the operational session namespace. The
// host must resolve that exact native project before joining its usage graph.
// Query pins preserve folder-qualified identities and the immutable definitions;
// the owning provider must also project their typed connection/meaning refs.
type RetainedDetectiveExecution struct {
	Namespace          NativeNamespace
	LifecycleSessionID string
	Owner              string
	Dispatch           DetectiveDispatchBinding
	Reference          apicontract.SessionInputReference
	SessionID          string
	RunID              string
	InputSnapshotID    string
	NativeRunID        string
	Scope              Scope
	Active             bool
	HandoffActive      bool
	Definition         DetectiveCheckDefinitionPin
	// Populated by the composed source reader after exact context joins.
	Subjects []RetainedScopedSubject
}

// Narrow owning codec/identity boundaries preserve fail-closed behavior when a
// dependency fails after an earlier family has already been validated.
var decodeRetainedSource = apicontract.DecodeStrict
var marshalRetainedSource = json.Marshal
var retainedLifecycleIdentity = SessionLifecycleID
var canonicalRetainedInputReference = apicontract.SessionInputReference.Canonical
var projectRetainedNativeSource = projectRetainedDetectiveExecution

type retainedInventoryReader map[string]RetainedDocument

func (r retainedInventoryReader) Load(ctx context.Context, collection, id string, out any) (int64, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	doc, exists := r[collection+"/"+id]
	if !exists {
		return 0, ErrNotFound
	}
	if decodeRetainedSource(doc.Payload, out) != nil {
		return 0, ErrUnavailable
	}
	return doc.Version, nil
}
func (retainedInventoryReader) List(context.Context, string, string, string, int) ([]json.RawMessage, string, error) {
	return nil, "", ErrUnavailable
}

func retainedReader(inventory RetainedInventory) (retainedInventoryReader, error) {
	n := inventory.Namespace
	if !validSessionLifecycleNamespace(n) || len(inventory.Documents) > 10000 {
		return nil, ErrUnavailable
	}
	known := map[string]bool{}
	for _, collection := range Collections() {
		known[collection] = true
	}
	manifest := map[string]bool{}
	for _, collection := range inventory.Collections {
		if !known[collection] || manifest[collection] {
			return nil, ErrUnavailable
		}
		manifest[collection] = true
	}
	if len(manifest) != len(known) {
		return nil, ErrUnavailable
	}
	reader := retainedInventoryReader{}
	bytes := 0
	for _, doc := range inventory.Documents {
		id := doc.Collection + "/" + doc.ID
		bytes += len(doc.Payload) + len(doc.ID) + len(doc.SessionID) + len(doc.Owner) + len(doc.Collection) + 64
		if !manifest[doc.Collection] || !safeID.MatchString(doc.ID) || !safeID.MatchString(doc.SessionID) || doc.Version < 1 || doc.Owner == "" || len(doc.Owner) > 256 || bytes > 16<<20 || !json.Valid(doc.Payload) {
			return nil, ErrUnavailable
		}
		if _, duplicate := reader[id]; duplicate {
			return nil, ErrUnavailable
		}
		doc.Payload = append(json.RawMessage(nil), doc.Payload...)
		reader[id] = doc
	}
	return reader, nil
}

// ProjectRetainedDetectiveExecutions validates all retained input/run/acceptance
// rows in an all-owner inventory. It never reconstructs an old pin from today's
// Check. Ordinary client/draft execution, redacted dependency loss, unsealed
// legacy acceptances, unrecognized outcomes and broken joins return no partial result.
// An uncertain native run stays active until owner settlement proves completion.
//
// This covers the native execution subset only: context subjects, ordinary pins,
// operation/audit provenance and other project sources need their own projection.
// The result alone MUST NOT be treated as complete project or session usage.
func ProjectRetainedDetectiveExecutions(ctx context.Context, inventory RetainedInventory) ([]RetainedDetectiveExecution, error) {
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
	ids := []string{}
	inputs := map[string]bool{}
	for _, doc := range r {
		switch doc.Collection {
		case Runs:
			ids = append(ids, doc.ID)
		case Inputs:
			inputs[doc.ID] = true
		case DetectiveWork, DetectiveOwners, Outbox:
			if _, exists := r[Runs+"/"+doc.ID]; !exists {
				return nil, ErrUnavailable
			}
		}
	}
	sort.Strings(ids)
	out := make([]RetainedDetectiveExecution, 0, len(ids))
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		item, err := projectRetainedNativeSource(ctx, inventory.Namespace, r, id)
		if err != nil {
			return nil, err
		}
		if !inputs[item.InputSnapshotID] {
			return nil, ErrUnavailable
		}
		delete(inputs, item.InputSnapshotID)
		out = append(out, item)
	}
	if len(inputs) != 0 {
		return nil, ErrUnavailable
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func projectRetainedDetectiveExecution(ctx context.Context, namespace NativeNamespace, r retainedInventoryReader, id string) (RetainedDetectiveExecution, error) {
	unknown := func() (RetainedDetectiveExecution, error) {
		if err := ctx.Err(); err != nil {
			return RetainedDetectiveExecution{}, err
		}
		return RetainedDetectiveExecution{}, ErrUnavailable
	}
	var run Run
	version, err := r.Load(ctx, Runs, id, &run)
	if err != nil || run.ID != id || run.Version != version || run.ExecutionAuthority != "server" || run.Kind != "check" || run.InputReference == nil || run.CreatedAt.IsZero() || run.ExecutionDigest != "" || len(run.ExecutionDocument) != 0 && !bytes.Equal(bytes.TrimSpace(run.ExecutionDocument), []byte("null")) || len(run.ResolvedParameters) != 0 || len(run.Widgets) != 0 || len(run.ResultRefs) != 0 {
		return unknown()
	}
	runEnvelope := r[Runs+"/"+id]
	if run.SessionID != runEnvelope.SessionID {
		return unknown()
	}
	var input InputSnapshot
	if _, err = r.Load(ctx, Inputs, run.InputSnapshotID, &input); err != nil || input.Detective == nil || input.ID != run.InputSnapshotID || input.SessionID != run.SessionID || input.SessionID != r[Inputs+"/"+input.ID].SessionID || !validScope(&input.Scope) || input.CreatedAt.IsZero() || len(input.DefinitionDocument) != 0 && !bytes.Equal(bytes.TrimSpace(input.DefinitionDocument), []byte("null")) {
		return unknown()
	}
	pin := input.Detective
	ref := pin.Reference
	if ref.Validate() != nil || (NativeNamespace{ref.StorageID, ref.SpaceID, ref.DatabaseID, ref.ProjectID, ref.NamespaceID}) != namespace || ref.SessionID != input.SessionID || ref.InputSnapshotID != input.ID || ref.ContextSetID != input.ContextSetID || ref.EnvironmentID != input.Scope.EnvironmentID || ref.IdentityDomain != input.Scope.IdentityDomain || !reflect.DeepEqual(ref, *run.InputReference) {
		return unknown()
	}
	definition, digest, err := CanonicalDetectiveDefinition(ctx, pin.Definition)
	d := pin.Dispatch
	if err != nil || digest != ref.DefinitionDigest || digest != input.DefinitionDigest || digest != run.DefinitionRevision || digest != d.DefinitionDigest || run.DefinitionID != definition.Check.ID || d.CheckID != definition.Check.ID || d.CheckVersion != definition.Check.Version || run.DetectiveOperationID != d.NativeOperationID || !safeID.MatchString(d.NativeOperationID) || !safeID.MatchString(d.NativeStoreID) || !safeID.MatchString(d.NativeProjectID) || d.NativeStoreID == "local" && d.NativeSpaceID != "" || d.NativeStoreID != "local" && !safeID.MatchString(d.NativeSpaceID) {
		return unknown()
	}
	action := detective.ActionRequest{Type: d.Action, Project: d.NativeProjectID, CheckID: d.CheckID, CaseID: d.CaseID, Environment: input.Scope.EnvironmentID, Inputs: pin.EffectiveInputs, InputReference: &ref, ExpectedDefinitionDigest: d.DefinitionDigest, ExpectedCaseContextDigest: d.CaseContextDigest}
	payload, err := detective.ActionOperationPayload(action)
	if err != nil {
		return unknown()
	}
	var work AcceptedDetectiveWork
	if _, err = r.Load(ctx, DetectiveWork, id, &work); err != nil || work.Namespace != namespace || work.ID != id || work.RunID != id || work.SessionID != run.SessionID || work.InputSnapshotID != input.ID || work.State != "accepted" || work.Owner != r[DetectiveWork+"/"+id].Owner || work.Owner != runEnvelope.Owner || work.Owner != r[Inputs+"/"+input.ID].Owner || !reflect.DeepEqual(work.Reference, ref) || work.Dispatch != d || work.AcceptedAt.IsZero() {
		return unknown()
	}
	bound, err := detective.BindOperation(d.NativeProjectID, work.Owner, d.Action, d.CheckID, payload)
	if err != nil || bound.PayloadDigest != d.NativeRequestDigest {
		return unknown()
	}
	identity := DetectiveIdentity{namespace, work.Owner, run.SessionID, id, d.NativeStoreID, d.NativeSpaceID, d.NativeProjectID, d.NativeOperationID, d.NativeRequestDigest}
	_, owner, err := loadDetectiveOwner(ctx, r, identity)
	if err != nil {
		return unknown()
	}
	var session Session
	v, err := r.Load(ctx, Sessions, run.SessionID, &session)
	if err != nil || session.ID != run.SessionID || session.ID != r[Sessions+"/"+session.ID].SessionID || session.DatabaseID != namespace.DatabaseID || session.ProjectID != namespace.ProjectID || session.Version != v || session.SchemaVersion != 1 {
		return unknown()
	}
	if session.State != "active" && session.State != "stopped" && session.State != "archived" && session.State != "deleted" {
		return unknown()
	}
	operation := r[Operations+"/"+opKey(Principal{Actor: work.Owner, DatabaseID: namespace.DatabaseID, ProjectID: namespace.ProjectID}, work.ClientOperationID)]
	if operation.Owner != work.Owner || operation.SessionID != run.SessionID {
		return unknown()
	}
	active := true
	switch run.State {
	case "accepted", "outcome-unknown":
		if owner.Settled || run.CompletedAt != nil {
			return unknown()
		}
	case "completed", "failed", "canceled":
		if !owner.Settled || owner.Observation == nil || !owner.Observation.Produced || !owner.Observation.Done || owner.Observation.Identity != identity || owner.Observation.State != run.State || owner.Observation.FenceGeneration < 1 || owner.Observation.FenceGeneration > owner.Generation || owner.Observation.SessionGeneration != owner.SessionGeneration || !safeID.MatchString(owner.Observation.NativeRunID) || !safeID.MatchString(owner.Observation.NativeReceiptID) || run.OutcomeOrigin != "native-owner" || run.CompletedAt == nil || run.CompletedAt.IsZero() {
			return unknown()
		}
		active = false
	default:
		return unknown()
	}
	if doc, exists := r[DetectiveOwners+"/"+id]; exists && (doc.Owner != work.Owner || doc.SessionID != run.SessionID) {
		return unknown()
	}
	handoffActive := false
	if _, exists := r[Outbox+"/"+id]; owner.Settled && d.CaseID != "" && !exists {
		return unknown()
	}
	if doc, exists := r[Outbox+"/"+id]; exists {
		var handoff EvidenceHandoff
		if _, err = r.Load(ctx, Outbox, id, &handoff); err != nil || !owner.Settled || d.CaseID == "" || owner.Observation == nil || !owner.Observation.Done || handoff.DetectiveRunID != owner.Observation.NativeRunID || handoff.ID != id || handoff.RunID != id || handoff.SessionID != run.SessionID || handoff.InputSnapshotID != input.ID || handoff.CaseID != d.CaseID || doc.Owner != work.Owner || doc.SessionID != run.SessionID {
			return unknown()
		}
		switch handoff.State {
		case "pending", "blocked":
			if owner.HandoffReceiptID != "" {
				return unknown()
			}
			handoffActive = true
		case "delivered":
			if !safeID.MatchString(owner.HandoffReceiptID) || owner.Observation.CaseLinkStatus != "linked" && owner.Observation.CaseLinkStatus != "already-linked" {
				return unknown()
			}
		case "redacted":
			if owner.FenceReason != "deleted" {
				return unknown()
			}
		default:
			return unknown()
		}
	}
	ref, err = canonicalRetainedInputReference(ref)
	if err != nil {
		return unknown()
	}
	nativeRunID := ""
	if owner.Observation != nil && owner.Observation.Produced {
		if owner.Observation.Identity != identity || !safeID.MatchString(owner.Observation.NativeRunID) {
			return unknown()
		}
		nativeRunID = owner.Observation.NativeRunID
	}
	lifecycleID, err := retainedLifecycleIdentity(namespace, run.SessionID)
	if err != nil {
		return unknown()
	}
	return RetainedDetectiveExecution{Namespace: namespace, LifecycleSessionID: lifecycleID, Owner: work.Owner, Dispatch: d, Reference: ref, SessionID: run.SessionID, RunID: id, InputSnapshotID: input.ID, NativeRunID: nativeRunID, Scope: input.Scope, Active: active, HandoffActive: handoffActive, Definition: definition}, nil
}
