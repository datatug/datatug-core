package sessioncontext

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
)

const DetectiveOwners = "sessionContextDetectiveOwners"

// DetectiveIdentity binds both operational authorities. It is not a bearer grant.
// NativeOperationID and Digest must come from the native owner's accepted request.
type DetectiveIdentity struct {
	Namespace         NativeNamespace `json:"namespace"`
	Actor             string          `json:"actor"`
	SessionID         string          `json:"sessionId"`
	RunID             string          `json:"runId"`
	NativeStoreID     string          `json:"nativeStoreId"`
	NativeSpaceID     string          `json:"nativeSpaceId,omitempty"`
	NativeProjectID   string          `json:"nativeProjectId"`
	NativeOperationID string          `json:"nativeOperationId"`
	Digest            string          `json:"digest"`
}

type DetectiveFence struct {
	SessionGeneration int64             `json:"sessionGeneration"`
	Identity          DetectiveIdentity `json:"identity"`
	Generation        int64             `json:"generation"`
}

// DetectiveObservation is supplied only by an authenticated native owner lookup,
// never by command JSON or a browser. Produced means the result is durable, Done
// means native finalization has completed. A fence must never discard Produced.
type DetectiveObservation struct {
	// FenceGeneration copies DetectiveFence.Generation (initially 1).
	// SessionGeneration copies DetectiveFence.SessionGeneration (initially 0).
	// The native owner persists both with its acceptance and checks them at finalization.
	FenceGeneration   int64             `json:"fenceGeneration"`
	SessionGeneration int64             `json:"sessionGeneration"`
	Identity          DetectiveIdentity `json:"identity"`
	Produced          bool              `json:"produced"`
	Done              bool              `json:"done"`
	NativeRunID       string            `json:"nativeRunId,omitempty"`
	NativeReceiptID   string            `json:"nativeReceiptId,omitempty"`
	State             string            `json:"state,omitempty"` // completed, failed, canceled
	CaseLinkStatus    string            `json:"caseLinkStatus,omitempty"`
}

// AcceptedDetectiveStatus contains no content, Case identity, labels, or input refs.
type AcceptedDetectiveStatus struct {
	RunID         string `json:"runId"`
	State         string `json:"state"`
	FenceReason   string `json:"fenceReason,omitempty"`
	Produced      bool   `json:"produced"`
	Done          bool   `json:"done"`
	EvidenceState string `json:"evidenceState,omitempty"`
}

type detectiveOwnerRecord struct {
	SessionGeneration int64                 `json:"sessionGeneration"`
	Identity          DetectiveIdentity     `json:"identity"`
	Version           int64                 `json:"version"`
	Generation        int64                 `json:"generation"`
	Claimed           bool                  `json:"claimed"`
	FenceReason       string                `json:"fenceReason,omitempty"`
	Observation       *DetectiveObservation `json:"observation,omitempty"`
	Settled           bool                  `json:"settled"`
	HandoffReceiptID  string                `json:"handoffReceiptId,omitempty"`
}

// DetectiveOwner is deliberately separate from Service.Command. Its host must
// authenticate an active account for every gate call, even after content access
// is revoked, and supply a native lookup bound to the exact operation identity.
// Native execution must call CheckFence before source execution AND native
// finalization. A claim is recoverable ownership, never permission to reexecute.
// Cross-store adapters must serialize/check these fences at their native owner;
// Core cannot turn a remote check followed by a write into an atomic transaction.
type DetectiveOwner struct {
	service           *Service
	acceptedActorGate func(context.Context, Principal, DetectiveIdentity) error
	lookup            func(context.Context, DetectiveIdentity) (DetectiveObservation, error)
}

func NewDetectiveOwner(s *Service, gate func(context.Context, Principal, DetectiveIdentity) error, lookup func(context.Context, DetectiveIdentity) (DetectiveObservation, error)) (*DetectiveOwner, error) {
	if s == nil || s.nativeNamespace == nil || gate == nil || lookup == nil {
		return nil, ErrInvalid
	}
	return &DetectiveOwner{s, gate, lookup}, nil
}

func (o *DetectiveOwner) identity(ctx context.Context, p Principal, id DetectiveIdentity, executor bool) error {
	s := o.service
	if s.nativeNamespace == nil || id.Namespace != *s.nativeNamespace || id.Actor != p.Actor || p.Actor == "" || p.DatabaseID != id.Namespace.DatabaseID || p.ProjectID != id.Namespace.ProjectID || executor && !p.Executor {
		return ErrForbidden
	}
	if !safeID.MatchString(id.SessionID) || !safeID.MatchString(id.RunID) || !safeID.MatchString(id.NativeOperationID) || !safeID.MatchString(id.NativeStoreID) || !safeID.MatchString(id.NativeProjectID) || len(id.Digest) != 64 {
		return ErrInvalid
	}
	if _, err := hex.DecodeString(id.Digest); err != nil {
		return ErrInvalid
	}
	if err := o.acceptedActorGate(ctx, p, id); err != nil {
		return ErrForbidden
	}
	return nil
}

func (o *DetectiveOwner) load(ctx context.Context, r Reader, id DetectiveIdentity) (AcceptedDetectiveWork, detectiveOwnerRecord, error) {
	var work AcceptedDetectiveWork
	version, err := r.Load(ctx, DetectiveWork, id.RunID, &work)
	if err != nil {
		return work, detectiveOwnerRecord{}, err
	}
	if work.OwnerProtocolVersion != 1 {
		return work, detectiveOwnerRecord{}, ErrUnavailable
	}
	d := work.Dispatch
	expected := DetectiveIdentity{work.Namespace, work.Owner, work.SessionID, work.RunID, d.NativeStoreID, d.NativeSpaceID, d.NativeProjectID, d.NativeOperationID, d.NativeRequestDigest}
	if expected != id || work.ID != id.RunID || version != work.Version {
		return work, detectiveOwnerRecord{}, ErrConflict
	}
	// Every current pin has an independent receipt seal, including internal pins
	// without a client digest. Unsealed legacy work fails closed; mutable work
	// fields cannot select a weaker authorization path.
	if err = verifyDetectiveRecoveryReceipt(ctx, r, work); err != nil {
		return work, detectiveOwnerRecord{}, err
	}
	state := detectiveOwnerRecord{Identity: id, Generation: 1, SessionGeneration: work.SessionGeneration}
	v, err := r.Load(ctx, DetectiveOwners, id.RunID, &state)
	if errors.Is(err, ErrNotFound) {
		return work, state, nil
	}
	if err != nil {
		return work, state, err
	}
	if state.Identity != id || state.Version != v || state.Generation < 1 || state.SessionGeneration != work.SessionGeneration {
		return work, state, ErrUnavailable
	}
	return work, state, nil
}

func (o *DetectiveOwner) save(ctx context.Context, tx Transaction, state *detectiveOwnerRecord) error {
	old := state.Version
	state.Version++
	return tx.Save(ctx, DetectiveOwners, state.Identity.RunID, state, old, state.Identity.Actor, state.Identity.SessionID)
}

func ownerStatus(work AcceptedDetectiveWork, state detectiveOwnerRecord) AcceptedDetectiveStatus {
	out := AcceptedDetectiveStatus{RunID: work.RunID, State: "accepted", FenceReason: state.FenceReason}
	if state.Claimed {
		out.State = "dispatching"
	}
	if state.Observation != nil {
		out.Produced, out.Done = state.Observation.Produced, state.Observation.Done
		if out.Done {
			out.State = state.Observation.State
		}
	}
	if state.FenceReason != "" {
		out.State = "fenced"
	}
	return out
}

func (o *DetectiveOwner) ReadStatus(ctx context.Context, p Principal, id DetectiveIdentity) (AcceptedDetectiveStatus, error) {
	var out AcceptedDetectiveStatus
	if err := o.identity(ctx, p, id, false); err != nil {
		return out, err
	}
	err := o.service.repo.View(ctx, func(r Reader) error {
		work, state, err := o.load(ctx, r, id)
		if err != nil {
			return err
		}
		out = ownerStatus(work, state)
		var lifecycle Session
		if _, err = r.Load(ctx, Sessions, id.SessionID, &lifecycle); err != nil {
			return err
		}
		if !state.Settled && lifecycle.NativeFenceGeneration != state.SessionGeneration && out.FenceReason == "" {
			out.State = "fenced"
			out.FenceReason = "canceled"
		}
		var marker SessionRedaction
		if _, err = r.Load(ctx, Redactions, id.SessionID, &marker); err == nil {
			out.FenceReason = "deleted"
			out.State = "fenced"
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
		var handoff EvidenceHandoff
		if _, err = r.Load(ctx, Outbox, id.RunID, &handoff); err == nil {
			out.EvidenceState = handoff.State
			if out.FenceReason == "deleted" {
				out.EvidenceState = "redacted"
			}
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
		return nil
	})
	return out, err
}

// ClaimDispatch returns the SAME generation on retry. It has no clock or lease.
// Recovery must reconcile the native operation identity before any execution.
func (o *DetectiveOwner) ClaimDispatch(ctx context.Context, p Principal, id DetectiveIdentity) (DetectiveFence, DetectiveInputPin, error) {
	token, pin, _, err := o.ClaimDispatchOnce(ctx, p, id)
	return token, pin, err
}

// ClaimDispatchOnce distinguishes the successful first ownership claim from
// replay. Only first=true may begin original native acceptance; it is still
// subject to native admission/fencing and durable journaling before dispatch.
// first=false never authorizes execution merely because native work is absent.
// A native accepted-but-not-started continuation needs its own durable proof.
func (o *DetectiveOwner) ClaimDispatchOnce(ctx context.Context, p Principal, id DetectiveIdentity) (DetectiveFence, DetectiveInputPin, bool, error) {
	first := false
	var token DetectiveFence
	var pin DetectiveInputPin
	if err := o.identity(ctx, p, id, true); err != nil {
		return token, pin, false, err
	}
	err := o.service.repo.Transact(ctx, func(tx Transaction) error {
		first = false
		work, state, err := o.load(ctx, tx, id)
		if err != nil {
			return err
		}
		if state.FenceReason != "" || state.Settled {
			return ErrReadOnly
		}
		session, err := o.service.session(ctx, tx, p, id.SessionID, "claimDetectiveDispatch")
		if err != nil {
			return err
		}
		if session.State != "active" {
			return ErrReadOnly
		}
		var input InputSnapshot
		if _, err = tx.Load(ctx, Inputs, work.InputSnapshotID, &input); err != nil {
			return err
		}
		if input.ID != work.InputSnapshotID || input.SessionID != id.SessionID || input.Detective == nil || !reflect.DeepEqual(input.Detective.Dispatch, work.Dispatch) {
			return ErrUnavailable
		}
		pin = *input.Detective
		if state.SessionGeneration != session.NativeFenceGeneration {
			return ErrReadOnly
		}
		state.SessionGeneration = session.NativeFenceGeneration
		token = DetectiveFence{Identity: id, Generation: state.Generation, SessionGeneration: state.SessionGeneration}
		if state.Claimed {
			return nil
		}
		state.Claimed = true
		first = true
		return o.save(ctx, tx, &state)
	})
	if err != nil {
		return DetectiveFence{}, DetectiveInputPin{}, false, err
	}
	return token, pin, first, nil
}

// CheckFence observes the fence in a fresh session repository read.
// Native owners sharing the authority should use CheckFenceWithReader inside
// their existing transaction so concurrent fences cause a transaction conflict.
func (o *DetectiveOwner) CheckFence(ctx context.Context, p Principal, token DetectiveFence) error {
	if err := o.identity(ctx, p, token.Identity, true); err != nil {
		return err
	}
	return o.service.repo.View(ctx, func(r Reader) error { return o.checkFenceReader(ctx, p, token, r) })
}

// CheckFenceWithReader performs the same fence and authorization validation
// using the caller's transaction snapshot. It starts or commits no transaction.
// The trusted native owner must supply a namespace-bound Reader and perform all
// reads before writes where its storage engine requires that ordering.
func (o *DetectiveOwner) CheckFenceWithReader(ctx context.Context, p Principal, token DetectiveFence, r Reader) error {
	if r == nil || reflect.ValueOf(r).Kind() == reflect.Pointer && reflect.ValueOf(r).IsNil() {
		return ErrInvalid
	}
	if err := o.identity(ctx, p, token.Identity, true); err != nil {
		return err
	}
	return o.checkFenceReader(ctx, p, token, r)
}

func (o *DetectiveOwner) checkFenceReader(ctx context.Context, p Principal, token DetectiveFence, r Reader) error {
	_, state, err := o.load(ctx, r, token.Identity)
	if err != nil {
		return err
	}
	if !state.Claimed || state.Generation != token.Generation || state.FenceReason != "" || state.Settled {
		return ErrReadOnly
	}
	session, err := o.service.session(ctx, r, p, token.Identity.SessionID, "checkDetectiveFence")
	if err != nil {
		return err
	}
	if session.State != "active" || session.NativeFenceGeneration != token.SessionGeneration || state.SessionGeneration != token.SessionGeneration {
		return ErrReadOnly
	}
	return nil
}

// Fence durably invalidates all prior dispatch tokens. Repeating a reason does
// not advance generation. No method clears fences or recreates private pins.
func (o *DetectiveOwner) Fence(ctx context.Context, p Principal, id DetectiveIdentity, reason string) error {
	if reason != "canceled" && reason != "contentrevoked" && reason != "deleted" {
		return ErrInvalid
	}
	if err := o.identity(ctx, p, id, true); err != nil {
		return err
	}
	return o.service.repo.Transact(ctx, func(tx Transaction) error {
		_, state, err := o.load(ctx, tx, id)
		if err != nil {
			return err
		}
		if state.FenceReason == reason || state.FenceReason == "deleted" {
			return nil
		}
		state.Generation++
		state.FenceReason = reason
		var handoff EvidenceHandoff
		version, err := tx.Load(ctx, Outbox, id.RunID, &handoff)
		if err == nil {
			if handoff.State != "delivered" {
				handoff.State = "blocked"
				handoff.ErrorCode = "content_unavailable"
				if reason == "deleted" {
					handoff.State = "redacted"
				}
				if err = tx.Save(ctx, Outbox, id.RunID, handoff, version, p.Actor, id.SessionID); err != nil {
					return err
				}
			}
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
		return o.save(ctx, tx, &state)
	})
}

// Reconcile reads the native owner's authoritative operation. It cannot run a
// query or create a native Case link. Produced-but-unfinished results survive
// fences; only native Done observations can settle an accepted session run.
func (o *DetectiveOwner) Reconcile(ctx context.Context, p Principal, id DetectiveIdentity) (AcceptedDetectiveStatus, error) {
	var out AcceptedDetectiveStatus
	if err := o.identity(ctx, p, id, true); err != nil {
		return out, err
	}
	observation, err := o.lookup(ctx, id)
	if err != nil {
		return out, err
	}
	if observation.Identity != id || observation.FenceGeneration < 1 || observation.Done && (!observation.Produced || !safeID.MatchString(observation.NativeReceiptID)) || observation.Produced && !safeID.MatchString(observation.NativeRunID) {
		return out, ErrConflict
	}
	if observation.Done && observation.State != "completed" && observation.State != "failed" && observation.State != "canceled" {
		return out, ErrInvalid
	}
	switch observation.CaseLinkStatus {
	case "", "linked", "already-linked", "case-changed", "case-archived", "case-unavailable", "link-unavailable":
	default:
		return out, ErrInvalid
	}
	err = o.service.repo.Transact(ctx, func(tx Transaction) error {
		work, state, err := o.load(ctx, tx, id)
		if err != nil {
			return err
		}
		if !state.Claimed || observation.FenceGeneration > state.Generation || observation.SessionGeneration != state.SessionGeneration {
			return ErrConflict
		}
		if state.Observation != nil {
			prior := state.Observation
			if prior.Produced && (!observation.Produced || prior.NativeRunID != observation.NativeRunID) || prior.Done && !reflect.DeepEqual(prior, &observation) {
				return ErrConflict
			}
		}
		state.Observation = &observation
		var lifecycle Session
		if _, err = tx.Load(ctx, Sessions, id.SessionID, &lifecycle); err != nil {
			return err
		}
		if lifecycle.Owner != p.Actor || lifecycle.DatabaseID != p.DatabaseID || lifecycle.ProjectID != p.ProjectID {
			return ErrForbidden
		}
		if !state.Settled && lifecycle.NativeFenceGeneration != state.SessionGeneration && state.FenceReason == "" {
			state.Generation++
			state.FenceReason = "canceled"
		}
		var marker SessionRedaction
		if _, err = tx.Load(ctx, Redactions, id.SessionID, &marker); err == nil {
			if state.FenceReason != "deleted" {
				state.Generation++
				state.FenceReason = "deleted"
			}
		} else if !errors.Is(err, ErrNotFound) {
			return err
		}
		if observation.Done && !state.Settled {
			// Deliberately use identity ownership, not content reads, so accepted status
			// can settle safely after a source/project permission has been revoked.
			var session Session
			var run Run
			sv, err := tx.Load(ctx, Sessions, id.SessionID, &session)
			if err != nil {
				return err
			}
			rv, err := tx.Load(ctx, Runs, id.RunID, &run)
			if err != nil {
				return err
			}
			if session.Owner != p.Actor || session.DatabaseID != p.DatabaseID || session.ProjectID != p.ProjectID || session.Version != sv || run.SessionID != id.SessionID || run.InputSnapshotID != work.InputSnapshotID || run.DetectiveOperationID != id.NativeOperationID || run.Version != rv || run.State != "accepted" {
				return ErrConflict
			}
			if err = settlePendingRun(&session); err != nil {
				return err
			}
			now := o.service.now()
			session.Version++
			session.UpdatedAt = now
			run.Version++
			run.State = observation.State
			run.CompletedAt = &now
			run.OutcomeOrigin = "native-owner"
			if err = tx.Save(ctx, Sessions, session.ID, session, sv, p.Actor, id.SessionID); err != nil {
				return err
			}
			if err = tx.Save(ctx, Runs, run.ID, run, rv, p.Actor, id.SessionID); err != nil {
				return err
			}
			if work.Dispatch.CaseID != "" {
				handoff := EvidenceHandoff{ID: id.RunID, SessionID: id.SessionID, RunID: id.RunID, InputSnapshotID: work.InputSnapshotID, CaseID: work.Dispatch.CaseID, DetectiveRunID: observation.NativeRunID, State: "pending"}
				if observation.CaseLinkStatus != "linked" && observation.CaseLinkStatus != "already-linked" {
					handoff.State = "blocked"
					handoff.ErrorCode = "native_link_unavailable"
				}
				if state.FenceReason != "" {
					handoff.State = "blocked"
					handoff.ErrorCode = "content_unavailable"
				}
				if state.FenceReason == "deleted" {
					handoff.State = "redacted"
				}
				if err = tx.Save(ctx, Outbox, id.RunID, handoff, 0, p.Actor, id.SessionID); err != nil {
					return err
				}
			}
			if err = tx.Save(ctx, Audit, opKey(p, "nativeComplete-"+id.RunID), AuditEvent{OpID: id.NativeOperationID, Actor: p.Actor, Action: "nativeComplete", SessionID: id.SessionID, ResourceID: id.RunID, At: now}, 0, p.Actor, id.SessionID); err != nil {
				return err
			}
			state.Settled = true
		}
		if err = o.save(ctx, tx, &state); err != nil {
			return err
		}
		out = ownerStatus(work, state)
		return nil
	})
	return out, err
}

// ReadHandoff is a trusted worker read, with ordinary content authorization.
// Reauthorization of the destination Case remains the native adapter's duty.
func (o *DetectiveOwner) ReadHandoff(ctx context.Context, p Principal, id DetectiveIdentity) (EvidenceHandoff, int64, error) {
	var handoff EvidenceHandoff
	var version int64
	if err := o.identity(ctx, p, id, true); err != nil {
		return handoff, 0, err
	}
	err := o.service.repo.View(ctx, func(r Reader) error {
		_, state, err := o.load(ctx, r, id)
		if err != nil {
			return err
		}
		if state.FenceReason != "" {
			return ErrReadOnly
		}
		if _, err = o.service.session(ctx, r, p, id.SessionID, "readDetectiveHandoff"); err != nil {
			return err
		}
		version, err = r.Load(ctx, Outbox, id.RunID, &handoff)
		return err
	})
	if err != nil {
		return EvidenceHandoff{}, 0, err
	}
	return handoff, version, nil
}

// SettleHandoff records only an external receipt, after the adapter reauthorizes
// both resources and delivers selected input evidence idempotently. It never
// changes native CaseRunLink. The version is the outbox storage version.
func (o *DetectiveOwner) SettleHandoff(ctx context.Context, p Principal, id DetectiveIdentity, expected int64, delivered bool, receiptID string) error {
	if delivered && !safeID.MatchString(receiptID) || !delivered && receiptID != "" {
		return ErrInvalid
	}
	if err := o.identity(ctx, p, id, true); err != nil {
		return err
	}
	return o.service.repo.Transact(ctx, func(tx Transaction) error {
		_, state, err := o.load(ctx, tx, id)
		if err != nil {
			return err
		}
		if !state.Settled || state.FenceReason != "" {
			return ErrReadOnly
		}
		if err = guardSessionRedaction(ctx, tx, id.SessionID); err != nil {
			return err
		}
		if err = o.service.allow(ctx, p, "settleDetectiveHandoff", id.SessionID, nil); err != nil {
			return err
		}
		var handoff EvidenceHandoff
		version, err := tx.Load(ctx, Outbox, id.RunID, &handoff)
		if err != nil {
			return err
		}
		if handoff.ID != id.RunID || handoff.SessionID != id.SessionID || handoff.DetectiveRunID != state.Observation.NativeRunID {
			return ErrConflict
		}
		target := "blocked"
		if delivered {
			target = "delivered"
		}
		if handoff.State == target {
			if state.HandoffReceiptID != receiptID {
				return ErrConflict
			}
			return nil
		}
		if version != expected || handoff.State == "delivered" {
			return ErrConflict
		}
		if state.Observation.CaseLinkStatus != "linked" && state.Observation.CaseLinkStatus != "already-linked" {
			return ErrReadOnly
		}
		handoff.State = target
		handoff.ErrorCode = ""
		if !delivered {
			handoff.ErrorCode = "handoff_denied"
		}
		if err = tx.Save(ctx, Outbox, id.RunID, handoff, version, p.Actor, id.SessionID); err != nil {
			return err
		}
		state.HandoffReceiptID = receiptID
		return o.save(ctx, tx, &state)
	})
}

// RedactPins installs an irreversible content marker before bounded scrubbing.
// Cursor advances across owner-filtered input records, including other sessions;
// a caller resumes until NextCursor is empty. Repeating pages is safe. This
// removes session-side executable pins only; separately owned Case evidence,
// operational records, legacy stores, backups and source data are untouched.
func (o *DetectiveOwner) RedactPins(ctx context.Context, p Principal, id DetectiveIdentity, deleteOpID, cursor string, limit int) (string, error) {
	if err := o.identity(ctx, p, id, true); err != nil {
		return "", err
	}
	if !safeID.MatchString(deleteOpID) || limit < 1 || limit > 100 || cursor != "" && !safeID.MatchString(cursor) {
		return "", ErrInvalid
	}
	var next string
	err := o.service.repo.Transact(ctx, func(tx Transaction) error {
		if _, _, err := o.load(ctx, tx, id); err != nil {
			return err
		}
		var marker SessionRedaction
		_, err := tx.Load(ctx, Redactions, id.SessionID, &marker)
		if errors.Is(err, ErrNotFound) {
			// An active-account gate alone grants status, not deletion. The first page
			// additionally needs current owner/content deletion authorization.
			if _, err = o.service.session(ctx, tx, p, id.SessionID, "deleteDetectivePins"); err != nil {
				return err
			}
			marker = SessionRedaction{SessionID: id.SessionID, Owner: p.Actor, Namespace: id.Namespace, DeleteOpID: deleteOpID, Version: 1, RedactedAt: o.service.now()}
			if err = tx.Save(ctx, Redactions, id.SessionID, marker, 0, p.Actor, id.SessionID); err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else if marker.Owner != p.Actor || marker.Namespace != id.Namespace || marker.DeleteOpID != deleteOpID || marker.Version < 1 {
			return ErrConflict
		}
		rows, after, err := tx.List(ctx, Inputs, p.Actor, cursor, limit)
		if err != nil {
			return err
		}
		next = after
		for _, raw := range rows {
			var input InputSnapshot
			if json.Unmarshal(raw, &input) != nil {
				return ErrUnavailable
			}
			if input.SessionID != id.SessionID {
				continue
			}
			var stored InputSnapshot
			version, err := tx.Load(ctx, Inputs, input.ID, &stored)
			if err != nil {
				return err
			}
			if stored.SessionID != id.SessionID {
				return ErrConflict
			}
			if stored.Detective == nil && len(stored.ResolvedParameters) == 0 && (len(stored.DefinitionDocument) == 0 || string(stored.DefinitionDocument) == "null") {
				continue
			}
			stored.Detective = nil
			stored.ResolvedParameters = nil
			stored.DefinitionDocument = nil
			stored.ParametersOrigin = "redacted"
			if err = tx.Save(ctx, Inputs, input.ID, stored, version, p.Actor, id.SessionID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return next, nil
}
