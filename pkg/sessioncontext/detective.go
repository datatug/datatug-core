package sessioncontext

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/detective"
)

const DetectiveWork = "sessionContextDetectiveWork"

var ErrEmptyCheckSet = fmt.Errorf("%w: unsupported empty Check set", ErrInvalid)

type DetectiveDispatchBinding struct {
	NativeOperationID   string `json:"nativeOperationId"`
	NativeStoreID       string `json:"nativeStoreId"`
	NativeSpaceID       string `json:"nativeSpaceId,omitempty"`
	NativeProjectID     string `json:"nativeProjectId"`
	Action              string `json:"action"`
	CheckID             string `json:"checkId"`
	CheckVersion        int64  `json:"checkVersion"`
	DefinitionDigest    string `json:"definitionDigest"`
	NativeRequestDigest string `json:"nativeRequestDigest"`
	CaseID              string `json:"caseId,omitempty"`
	CaseVersion         int64  `json:"caseVersion,omitempty"`
	CaseContextDigest   string `json:"caseContextDigest,omitempty"`
}
type DetectiveInputPin struct {
	Reference       apicontract.SessionInputReference `json:"reference"`
	Definition      DetectiveCheckDefinitionPin       `json:"definition"`
	EffectiveInputs map[string]detective.InputValue   `json:"effectiveInputs"`
	Dispatch        DetectiveDispatchBinding          `json:"dispatch"`
}

// AcceptedDetectiveWork records private acceptance, never native execution or
// native acceptance. Its metadata is reserved for later authenticated recovery.
type AcceptedDetectiveWork struct {
	ID              string                            `json:"id"`
	Owner           string                            `json:"owner"`
	Namespace       NativeNamespace                   `json:"namespace"`
	SessionID       string                            `json:"sessionId"`
	RunID           string                            `json:"runId"`
	InputSnapshotID string                            `json:"inputSnapshotId"`
	Reference       apicontract.SessionInputReference `json:"reference"`
	Dispatch        DetectiveDispatchBinding          `json:"dispatch"`
	State           string                            `json:"state"`
	Version         int64                             `json:"version"`
	AcceptedAt      time.Time                         `json:"acceptedAt"`

	// SessionGeneration is captured atomically with acceptance, before any dispatch claim.
	SessionGeneration int64 `json:"sessionGeneration"`
	// OwnerProtocolVersion binds lifecycle generation; legacy unbound acceptances cannot dispatch.
	OwnerProtocolVersion int64 `json:"ownerProtocolVersion"`
	// ClientOperationID and AcceptanceDigest bind the durable metadata lookup.
	ClientOperationID   string `json:"clientOperationId,omitempty"`
	AcceptanceDigest    string `json:"acceptanceDigest,omitempty"`
	ClientRequestDigest string `json:"clientRequestDigest,omitempty"`
}

// PinDetectiveInputCommand is a trusted Go boundary, never a generic HTTP command.
// The native owner supplies its existing scope-envelope OperationBinding and
// authoritative Case snapshot. This method does not authorize or execute a Case.
type PinDetectiveInputCommand struct {
	// ClientRequestDigest is host-computed from a canonical transport request.
	// Empty retains compatibility for internal callers, not transport acceptance.
	ClientRequestDigest                     string
	OpID, SessionID, InputSnapshotID, RunID string
	ExpectedVersion                         int64
	ExpectedContextSetID                    string
	Scope                                   Scope
	Attachments                             []EntryRef
	Pin                                     DetectiveInputPin
	Case                                    *detective.Case
	NativeBinding                           detective.OperationBinding
}
type PinnedDetectiveInput struct {
	Receipt Receipt
	Input   InputSnapshot
	Run     Run
}

func (s *Service) prepareDetectivePin(ctx context.Context, p Principal, c PinDetectiveInputCommand) (DetectiveInputPin, error) {
	var pin DetectiveInputPin
	if c.ClientRequestDigest != "" {
		decoded, err := hex.DecodeString(c.ClientRequestDigest)
		if err != nil || len(decoded) != 32 || strings.ToLower(c.ClientRequestDigest) != c.ClientRequestDigest {
			return pin, ErrInvalid
		}
	}
	if s.nativeNamespace == nil {
		return pin, ErrUnavailable
	}
	n := *s.nativeNamespace
	if !p.Executor || p.DatabaseID != n.DatabaseID || p.ProjectID != n.ProjectID {
		return pin, ErrForbidden
	}
	if !safeID.MatchString(c.OpID) || !safeID.MatchString(c.SessionID) || !safeID.MatchString(c.InputSnapshotID) || !safeID.MatchString(c.RunID) || !validScope(&c.Scope) {
		return pin, ErrInvalid
	}
	for _, input := range c.Pin.EffectiveInputs {
		if input.Set != nil && len(input.Set.Values) == 0 {
			return pin, ErrEmptyCheckSet
		}
	}
	raw, err := json.Marshal(c.Pin)
	if err != nil || len(raw) > 700000 || apicontract.DecodeStrict(raw, &pin) != nil {
		return pin, ErrInvalid
	}
	definition, digest, err := CanonicalDetectiveDefinition(ctx, pin.Definition)
	if err != nil {
		return pin, ErrInvalid
	}
	pin.Definition = definition
	r := pin.Reference
	if (NativeNamespace{r.StorageID, r.SpaceID, r.DatabaseID, r.ProjectID, r.NamespaceID}) != n || r.SessionID != c.SessionID || r.InputSnapshotID != c.InputSnapshotID || r.ContextSetID != c.ExpectedContextSetID || r.EnvironmentID != c.Scope.EnvironmentID || r.IdentityDomain != c.Scope.IdentityDomain || r.DefinitionDigest != digest {
		return pin, ErrInvalid
	}
	caseContext := map[string]detective.InputValue{}
	d := pin.Dispatch
	if c.Case != nil {
		if c.Case.ID != d.CaseID || c.Case.Version <= 0 || c.Case.Version != d.CaseVersion || c.Case.Status != "active" {
			return pin, ErrInvalid
		}
		if c.Case.Context != nil {
			caseContext = c.Case.Context
		}
		for _, input := range caseContext {
			if input.Set != nil && len(input.Set.Values) == 0 {
				return pin, ErrEmptyCheckSet
			}
		}
		encoded, encodeErr := json.Marshal(caseContext)
		if encodeErr != nil {
			return pin, ErrInvalid
		}
		hash := sha256.Sum256(encoded)
		if d.CaseContextDigest != hex.EncodeToString(hash[:]) {
			return pin, ErrInvalid
		}
	} else if d.CaseID != "" || d.CaseVersion != 0 || d.CaseContextDigest != "" {
		return pin, ErrInvalid
	}
	effective, err := detective.BindCaseInputs(definition.Check.Inputs, pin.EffectiveInputs, caseContext)
	if err != nil || !reflect.DeepEqual(effective, pin.EffectiveInputs) {
		return pin, ErrInvalid
	}
	for _, binding := range r.Bindings {
		if binding.Origin == "case" && (binding.CaseVersion != d.CaseVersion || !reflect.DeepEqual(effective[binding.InputName], caseContext[binding.InputName])) {
			return pin, ErrInvalid
		}
	}
	action := detective.ActionRequest{Type: d.Action, Project: d.NativeProjectID, CheckID: d.CheckID, CaseID: d.CaseID, Environment: c.Scope.EnvironmentID, Inputs: effective, InputReference: &r, ExpectedDefinitionDigest: d.DefinitionDigest, ExpectedCaseContextDigest: d.CaseContextDigest}
	if _, err = detective.ActionOperationPayload(action); err != nil || !safeID.MatchString(d.NativeOperationID) || !safeID.MatchString(d.NativeStoreID) || !safeID.MatchString(d.NativeProjectID) || d.NativeStoreID == "local" && d.NativeSpaceID != "" || d.NativeStoreID != "local" && !safeID.MatchString(d.NativeSpaceID) || d.CheckVersion != definition.Check.Version || d.CheckID != definition.Check.ID || d.DefinitionDigest != digest {
		return pin, ErrInvalid
	}
	if c.NativeBinding.Actor != p.Actor || c.NativeBinding.Project != d.NativeProjectID || c.NativeBinding.Action != d.Action || c.NativeBinding.ResourceID != d.CheckID || c.NativeBinding.PayloadDigest != d.NativeRequestDigest {
		return pin, ErrInvalid
	}
	if _, err := detective.NewOperationReceipt(d.NativeOperationID, c.NativeBinding, 0, s.now()); err != nil {
		return pin, ErrInvalid
	}
	pin.Reference, _ = r.Canonical() // ActionOperationPayload validated this reference
	pin.EffectiveInputs = effective
	return pin, nil
}

// PinDetectiveInput atomically accepts private inputs only. It deliberately
// dispatches nothing and cannot settle native work without later attestation.
func (s *Service) PinDetectiveInput(ctx context.Context, p Principal, c PinDetectiveInputCommand) (PinnedDetectiveInput, error) {
	var result PinnedDetectiveInput
	if len(c.Attachments) > apicontract.MaxSessionInputEntryRefs {
		return result, ErrInvalid
	}
	// Attachments are a selection, not an execution order. Own and canonicalize
	// them before hashing so retry ordering cannot change an accepted operation.
	attachments := append([]EntryRef(nil), c.Attachments...)
	sort.Slice(attachments, func(i, j int) bool { return attachments[i].EntryID < attachments[j].EntryID })
	c.Attachments = nil
	for _, ref := range attachments {
		if !safeID.MatchString(ref.EntryID) || !safeID.MatchString(ref.RevisionID) {
			return result, ErrInvalid
		}
		if len(c.Attachments) > 0 && c.Attachments[len(c.Attachments)-1].EntryID == ref.EntryID {
			if c.Attachments[len(c.Attachments)-1].RevisionID != ref.RevisionID {
				return result, ErrInvalid
			}
			continue
		}
		c.Attachments = append(c.Attachments, ref)
	}
	pin, err := s.prepareDetectivePin(ctx, p, c)
	if err != nil {
		return result, err
	}
	if err = s.allow(ctx, p, "pinDetectiveInput", c.SessionID, &c.Scope); err != nil {
		return result, err
	}
	// Bind the owned canonical pin, not caller-owned Case prose or raw query JSON.
	bound, _ := json.Marshal(struct {
		Actor                                         string
		Namespace                                     NativeNamespace
		OpID, SessionID, InputID, RunID, ContextSetID string
		Version                                       int64
		Attachments                                   []EntryRef
		Pin                                           DetectiveInputPin
		ClientRequestDigest                           string `json:"ClientRequestDigest,omitempty"`
	}{p.Actor, *s.nativeNamespace, c.OpID, c.SessionID, c.InputSnapshotID, c.RunID, c.ExpectedContextSetID, c.ExpectedVersion, c.Attachments, pin, c.ClientRequestDigest})
	hash := sha256.Sum256(bound)
	digest, key := hex.EncodeToString(hash[:]), opKey(p, c.OpID)
	err = s.repo.Transact(ctx, func(tx Transaction) error {
		v, e := s.session(ctx, tx, p, c.SessionID, "pinDetectiveInput")
		if e != nil {
			return e
		}
		var receipt Receipt
		if _, e = tx.Load(ctx, Operations, key, &receipt); e == nil {
			if receipt.Action != "pinDetectiveInput" || receipt.PayloadDigest != digest || receipt.SessionID != v.ID || receipt.Actor != p.Actor {
				return ErrConflict
			}
			if _, e = tx.Load(ctx, Inputs, c.InputSnapshotID, &result.Input); e != nil {
				return e
			}
			if _, e = tx.Load(ctx, Runs, c.RunID, &result.Run); e != nil {
				return e
			}
			result.Receipt = receipt
			return nil
		} else if !errors.Is(e, ErrNotFound) {
			return e
		}
		if v.State != "active" {
			return ErrReadOnly
		}
		if v.Version != c.ExpectedVersion || v.ContextSetID != c.ExpectedContextSetID {
			return ErrConflict
		}
		var set AppliedContextSet
		if _, e = tx.Load(ctx, ContextSets, v.ContextSetID, &set); e != nil {
			return e
		}
		if set.ID != v.ContextSetID || set.SessionID != v.ID {
			return ErrUnavailable
		}
		refs := append([]EntryRef(nil), c.Attachments...)
		if set.Enabled {
			for _, slot := range set.Slots {
				if slot.Scope == c.Scope {
					refs = append(refs, slot.Entries...)
				}
			}
		}
		refs, e = s.validateScopedRefs(ctx, tx, p, v, c.Scope, refs)
		if e != nil {
			return e
		}
		selected := map[string]string{}
		for _, ref := range refs {
			selected[ref.EntryID] = ref.RevisionID
		}
		for _, binding := range pin.Reference.Bindings {
			if binding.Origin != "bag" {
				continue
			}
			// One selected bag envelope is one Check input; aggregation requires
			// an explicit semantic mapper, not implicit union/concatenation.
			if len(binding.Entries) != 1 {
				return ErrInvalid
			}
			ref := binding.Entries[0]
			if selected[ref.EntryID] != ref.RevisionID {
				return ErrInvalid
			}
			var rev EntryRevision
			if _, e = tx.Load(ctx, Revisions, ref.RevisionID, &rev); e != nil {
				return e
			}
			if rev.BagID != ref.BagID {
				return ErrInvalid
			}
			bag, e := s.bag(ctx, tx, p, v, ref.BagID, "pinDetectiveInput")
			if e != nil {
				return e
			}
			versionMatches := bag.Version == ref.BagVersion
			for _, slot := range set.Slots {
				if slot.BagID == bag.ID && slot.Scope == c.Scope && slot.BagVersion == ref.BagVersion && containsEntryRef(slot.Entries, ref.EntryID, ref.RevisionID) {
					versionMatches = true
				}
			}
			value, e := detectiveBagValue(rev.Value)
			if e != nil {
				return e
			}
			if !versionMatches || !reflect.DeepEqual(value, pin.EffectiveInputs[binding.InputName]) {
				return ErrInvalid
			}
		}
		now := s.now()
		input := InputSnapshot{ID: c.InputSnapshotID, SessionID: v.ID, ContextSetID: set.ID, Scope: c.Scope, Entries: refs, DefinitionDigest: pin.Reference.DefinitionDigest, ParametersOrigin: "server-executor", CreatedAt: now, Detective: &pin}
		run := Run{ID: c.RunID, SessionID: v.ID, InputSnapshotID: input.ID, Kind: "check", DefinitionID: pin.Dispatch.CheckID, DefinitionRevision: pin.Reference.DefinitionDigest, State: "accepted", ExecutionAuthority: "server", Version: 1, CreatedAt: now, InputReference: &pin.Reference, DetectiveOperationID: pin.Dispatch.NativeOperationID}
		work := AcceptedDetectiveWork{ClientOperationID: c.OpID, AcceptanceDigest: digest, ClientRequestDigest: c.ClientRequestDigest, OwnerProtocolVersion: 1, SessionGeneration: v.NativeFenceGeneration, ID: run.ID, Owner: p.Actor, Namespace: *s.nativeNamespace, SessionID: v.ID, RunID: run.ID, InputSnapshotID: input.ID, Reference: pin.Reference, Dispatch: pin.Dispatch, State: "accepted", Version: 1, AcceptedAt: now}
		v.PendingRunCount++
		v.Version++
		v.UpdatedAt = now
		scope := c.Scope
		receipt = Receipt{Scope: &scope, OpID: c.OpID, Actor: p.Actor, Action: "pinDetectiveInput", PayloadDigest: digest, SessionID: v.ID, SessionVersion: v.Version, ResourceID: run.ID, ResourceVersion: 1, At: now}
		for _, item := range []struct {
			collection, id string
			value          any
			expected       int64
		}{{Inputs, input.ID, input, 0}, {Runs, run.ID, run, 0}, {DetectiveWork, work.ID, work, 0}, {Sessions, v.ID, v, v.Version - 1}, {Operations, key, receipt, 0}, {Audit, key, AuditEvent{OpID: c.OpID, Actor: p.Actor, Action: "pinDetectiveInput", SessionID: v.ID, ResourceID: run.ID, Count: len(refs), At: now}, 0}} {
			if e = tx.Save(ctx, item.collection, item.id, item.value, item.expected, p.Actor, v.ID); e != nil {
				return e
			}
		}
		result = PinnedDetectiveInput{receipt, input, run}
		return nil
	})
	if err != nil {
		return PinnedDetectiveInput{}, err
	}
	return result, nil
}

func containsEntryRef(refs []EntryRef, entry, revision string) bool {
	for _, ref := range refs {
		if ref.EntryID == entry && ref.RevisionID == revision {
			return true
		}
	}
	return false
}
func detectiveBagValue(value ValueEnvelope) (detective.InputValue, error) {
	var result detective.InputValue
	if value.Kind == "scalar" && value.Scalar != nil {
		result.Scalar = value.Scalar
	} else if value.Kind == "set" && len(value.Values) == 0 && len(value.Keys) == 0 {
		return result, ErrEmptyCheckSet
	} else if value.Kind == "set" && len(value.Values) > 0 && len(value.Keys) == 0 {
		set, err := apicontract.NewTypedValueSet(value.Values...)
		if err != nil {
			return result, ErrInvalid
		}
		result.Set = &set
	} else {
		return result, ErrInvalid
	}
	if result.Validate() != nil {
		return result, ErrInvalid
	}
	return result, nil
}

func (s *Service) ReadDetectiveDispatch(ctx context.Context, p Principal, sessionID, runID string) (DetectiveInputPin, error) {
	run, err := s.ReadRun(ctx, p, runID)
	if err != nil {
		return DetectiveInputPin{}, err
	}
	if run.SessionID != sessionID {
		return DetectiveInputPin{}, ErrForbidden
	}
	input, _, err := s.ReadInput(ctx, p, run.InputSnapshotID)
	if err != nil {
		return DetectiveInputPin{}, err
	}
	if input.Detective == nil {
		return DetectiveInputPin{}, ErrNotFound
	}
	return *input.Detective, nil
}
