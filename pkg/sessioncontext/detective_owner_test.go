package sessioncontext

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/detective"
)

func ownerFixture(t *testing.T, withCase bool) (*Service, Principal, *DetectiveOwner, DetectiveIdentity, *DetectiveObservation, *bool) {
	t.Helper()
	s, p := pinService(t)
	c := pinCommand(t, s, p)
	if withCase {
		c.Case = &detective.Case{ID: "private-case", Version: 1, Status: "active"}
		bindPin(t, p, &c)
	}
	if _, err := s.PinDetectiveInput(context.Background(), p, c); err != nil {
		t.Fatal(err)
	}
	d := c.Pin.Dispatch
	id := DetectiveIdentity{*s.nativeNamespace, p.Actor, c.SessionID, c.RunID, d.NativeStoreID, d.NativeSpaceID, d.NativeProjectID, d.NativeOperationID, d.NativeRequestDigest}
	observation := &DetectiveObservation{Identity: id, FenceGeneration: 1}
	gate := new(bool)
	*gate = true
	owner, err := NewDetectiveOwner(s, func(context.Context, Principal, DetectiveIdentity) error {
		if !*gate {
			return ErrForbidden
		}
		return nil
	}, func(context.Context, DetectiveIdentity) (DetectiveObservation, error) { return *observation, nil })
	if err != nil {
		t.Fatal(err)
	}
	return s, p, owner, id, observation, gate
}
func ownerRecord(t *testing.T, s *Service, id DetectiveIdentity) detectiveOwnerRecord {
	t.Helper()
	var state detectiveOwnerRecord
	if err := s.repo.View(context.Background(), func(r Reader) error {
		_, err := r.Load(context.Background(), DetectiveOwners, id.RunID, &state)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return state
}
func ownerHandoff(t *testing.T, s *Service, id DetectiveIdentity) (EvidenceHandoff, int64) {
	t.Helper()
	var handoff EvidenceHandoff
	var version int64
	if err := s.repo.View(context.Background(), func(r Reader) error {
		var err error
		version, err = r.Load(context.Background(), Outbox, id.RunID, &handoff)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return handoff, version
}
func completeObservation(obs *DetectiveObservation) {
	obs.Produced = true
	obs.Done = true
	obs.NativeRunID = "native-run"
	obs.NativeReceiptID = "native-receipt"
	obs.State = "completed"
	obs.CaseLinkStatus = "linked"
}

func TestDetectiveOwnerClaimRecoveryAndExactBinding(t *testing.T) {
	ctx := context.Background()
	_, p, o, id, _, _ := ownerFixture(t, false)
	token, pin, err := o.ClaimDispatch(ctx, p, id)
	if err != nil {
		t.Fatal(err)
	}
	retry, _, err := o.ClaimDispatch(ctx, p, id)
	if err != nil || retry != token {
		t.Fatalf("claim generation changed: %+v %v", retry, err)
	}
	if !strings.Contains(string(pin.Definition.Queries[0].DefinitionJSON), "private_value") {
		t.Fatal("exact query pin lost")
	}
	if err = o.CheckFence(ctx, p, token); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*DetectiveIdentity){func(v *DetectiveIdentity) { v.Actor = "mallory" }, func(v *DetectiveIdentity) { v.Namespace.NamespaceID = "other" }, func(v *DetectiveIdentity) { v.NativeOperationID = "other" }, func(v *DetectiveIdentity) { v.Digest = strings.Repeat("a", 64) }, func(v *DetectiveIdentity) { v.NativeSpaceID = "other" }, func(v *DetectiveIdentity) { v.NativeProjectID = "other" }, func(v *DetectiveIdentity) { v.SessionID = "other" }, func(v *DetectiveIdentity) { v.NativeStoreID = "other" }} {
		changed := id
		mutate(&changed)
		if _, err = o.ReadStatus(ctx, p, changed); err == nil {
			t.Fatalf("foreign binding accepted: %+v", changed)
		}
	}
	p.Executor = false
	if _, _, err = o.ClaimDispatch(ctx, p, id); !errors.Is(err, ErrForbidden) {
		t.Fatalf("client claimed native work: %v", err)
	}
	if _, err = o.ReadStatus(ctx, p, id); err != nil {
		t.Fatalf("accepted actor status wrongly needs executor: %v", err)
	}
}
func TestDetectiveOwnerProducedRecoveryAndEvidenceReceipt(t *testing.T) {
	ctx := context.Background()
	s, p, o, id, obs, _ := ownerFixture(t, true)
	token, _, err := o.ClaimDispatch(ctx, p, id)
	if err != nil {
		t.Fatal(err)
	}
	obs.Produced = true
	obs.NativeRunID = "native-run"
	status, err := o.Reconcile(ctx, p, id)
	if err != nil || !status.Produced || status.Done {
		t.Fatalf("produced recovery lost: %+v %v", status, err)
	}
	snap, err := s.GetSnapshot(ctx, p, id.SessionID)
	if err != nil || snap.Session.PendingRunCount != 1 {
		t.Fatal("unfinished native work settled early")
	}
	completeObservation(obs)
	if _, err = o.Reconcile(ctx, p, id); err != nil {
		t.Fatal(err)
	}
	if _, err = o.Reconcile(ctx, p, id); err != nil {
		t.Fatal("lost acknowledgment replay", err)
	}
	snap, err = s.GetSnapshot(ctx, p, id.SessionID)
	if err != nil || snap.Session.PendingRunCount != 0 {
		t.Fatal("pending count not settled exactly once")
	}
	if err = o.CheckFence(ctx, p, token); !errors.Is(err, ErrReadOnly) {
		t.Fatal("settled run executable", err)
	}
	handoff, version := ownerHandoff(t, s, id)
	if handoff.State != "pending" || handoff.DetectiveRunID != "native-run" {
		t.Fatalf("handoff: %+v", handoff)
	}
	if err = o.SettleHandoff(ctx, p, id, version, true, "handoff-receipt"); err != nil {
		t.Fatal(err)
	}
	if err = o.SettleHandoff(ctx, p, id, version, true, "handoff-receipt"); err != nil {
		t.Fatal("idempotent delivery", err)
	}
	if err = o.SettleHandoff(ctx, p, id, version, true, "different-receipt"); !errors.Is(err, ErrConflict) {
		t.Fatal("changed receipt replay", err)
	}
	if ownerRecord(t, s, id).HandoffReceiptID != "handoff-receipt" {
		t.Fatal("delivery receipt not durable")
	}
	obs.NativeRunID = "other-run"
	if _, err = o.Reconcile(ctx, p, id); !errors.Is(err, ErrConflict) {
		t.Fatal("changed authoritative run accepted", err)
	}
}
func TestDetectiveOwnerRevokedContentStatusAndKnownResult(t *testing.T) {
	ctx := context.Background()
	s, p, o, id, obs, active := ownerFixture(t, true)
	token, _, err := o.ClaimDispatch(ctx, p, id)
	if err != nil {
		t.Fatal(err)
	}
	obs.Produced = true
	obs.NativeRunID = "native-run"
	if _, err = o.Reconcile(ctx, p, id); err != nil {
		t.Fatal(err)
	}
	if err = o.Fence(ctx, p, id, "contentrevoked"); err != nil {
		t.Fatal(err)
	}
	generation := ownerRecord(t, s, id).Generation
	if err = o.Fence(ctx, p, id, "contentrevoked"); err != nil || ownerRecord(t, s, id).Generation != generation {
		t.Fatal("fence replay advanced generation")
	}
	s.authorize = func(context.Context, Access) error { return ErrForbidden }
	if _, _, err = o.ClaimDispatch(ctx, p, id); !errors.Is(err, ErrReadOnly) {
		t.Fatal("fenced dispatch", err)
	}
	if err = o.CheckFence(ctx, p, token); !errors.Is(err, ErrReadOnly) {
		t.Fatal("fence did not revoke old token", err)
	}
	status, err := o.ReadStatus(ctx, p, id)
	if err != nil || !status.Produced || status.FenceReason != "contentrevoked" {
		t.Fatalf("safe status unavailable: %+v %v", status, err)
	}
	raw, _ := json.Marshal(status)
	for _, secret := range []string{"private-case", "private_value", "Private", "input", "definition", "native-project"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("status leaked %s: %s", secret, raw)
		}
	}
	// Native owner may recover its known result, without any query or Case write.
	completeObservation(obs)
	obs.CaseLinkStatus = "link-unavailable"
	if _, err = o.Reconcile(ctx, p, id); err != nil {
		t.Fatal(err)
	}
	handoff, version := ownerHandoff(t, s, id)
	if handoff.State != "blocked" {
		t.Fatal("fenced handoff not blocked")
	}
	if err = o.SettleHandoff(ctx, p, id, version, true, "receipt"); !errors.Is(err, ErrReadOnly) {
		t.Fatal("revoked evidence delivery", err)
	}
	*active = false
	if _, err = o.ReadStatus(ctx, p, id); !errors.Is(err, ErrForbidden) {
		t.Fatal("inactive account read accepted status", err)
	}
}
func TestDetectiveOwnerRedactionPagesAreBoundedAndIrreversible(t *testing.T) {
	ctx := context.Background()
	s, p, o, id, obs, _ := ownerFixture(t, true)
	token, _, err := o.ClaimDispatch(ctx, p, id)
	if err != nil {
		t.Fatal(err)
	}
	// Multiple independently owned pins plus an unrelated session input.
	err = s.repo.Transact(ctx, func(tx Transaction) error {
		var original InputSnapshot
		if _, e := tx.Load(ctx, Inputs, "input", &original); e != nil {
			return e
		}
		for _, key := range []string{"a-input", "z-input", "foreign-input"} {
			v := original
			v.ID = key
			if key == "foreign-input" {
				v.SessionID = "s2"
			}
			if e := tx.Save(ctx, Inputs, key, v, 0, p.Actor, v.SessionID); e != nil {
				return e
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	cursor, err := o.RedactPins(ctx, p, id, "delete-op", "", 1)
	if err != nil || cursor == "" {
		t.Fatal("bounded page missing continuation", err)
	}
	if err = o.CheckFence(ctx, p, token); !errors.Is(err, ErrReadOnly) {
		t.Fatal("redaction failed immediate fence", err)
	}
	if _, err = s.ReadDetectiveDispatch(ctx, p, id.SessionID, id.RunID); !errors.Is(err, ErrReadOnly) {
		t.Fatal("redacted pin readable", err)
	}
	if _, _, err = o.ClaimDispatch(ctx, p, id); !errors.Is(err, ErrReadOnly) {
		t.Fatal("redacted dispatch claim", err)
	}
	if _, err = o.RedactPins(ctx, p, id, "other-delete", cursor, 1); !errors.Is(err, ErrConflict) {
		t.Fatal("deletion operation changed", err)
	}
	for cursor != "" {
		cursor, err = o.RedactPins(ctx, p, id, "delete-op", cursor, 1)
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err = o.RedactPins(ctx, p, id, "delete-op", "", 1); err != nil {
		t.Fatal("page replay failed", err)
	}
	err = s.repo.View(ctx, func(r Reader) error {
		for _, key := range []string{"a-input", "input", "z-input", "foreign-input"} {
			var input InputSnapshot
			if _, e := r.Load(ctx, Inputs, key, &input); e != nil {
				return e
			}
			if key == "foreign-input" {
				if input.Detective == nil {
					t.Fatal("other session scrubbed")
				}
			} else if input.Detective != nil {
				t.Fatal("private pin retained", key)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	// No pin can be recreated by native completion or original acceptance replay.
	completeObservation(obs)
	if _, err = o.Reconcile(ctx, p, id); err != nil {
		t.Fatal(err)
	}
	status, err := o.ReadStatus(ctx, p, id)
	if err != nil || status.FenceReason != "deleted" || status.EvidenceState != "redacted" {
		t.Fatalf("deleted safe status %+v %v", status, err)
	}
	handoff, _ := ownerHandoff(t, s, id)
	if handoff.State != "redacted" {
		t.Fatal("redacted evidence reference not marked unavailable")
	}
}
func TestDetectiveOwnerRejectsUntrustedCompletionAndStop(t *testing.T) {
	ctx := context.Background()
	s, p, o, id, obs, _ := ownerFixture(t, false)
	if _, err := NewDetectiveOwner(s, nil, o.lookup); !errors.Is(err, ErrInvalid) {
		t.Fatal("nil gate enabled")
	}
	if _, err := NewDetectiveOwner(s, o.acceptedActorGate, nil); !errors.Is(err, ErrInvalid) {
		t.Fatal("nil lookup enabled")
	}
	if _, err := o.Reconcile(ctx, p, id); !errors.Is(err, ErrConflict) {
		t.Fatal("unclaimed completion", err)
	}
	token, _, err := o.ClaimDispatch(ctx, p, id)
	if err != nil {
		t.Fatal(err)
	}
	obs.Done = true
	if _, err = o.Reconcile(ctx, p, id); !errors.Is(err, ErrConflict) {
		t.Fatal("unproduced done accepted", err)
	}
	completeObservation(obs)
	obs.Identity.Digest = strings.Repeat("a", 64)
	if _, err = o.Reconcile(ctx, p, id); !errors.Is(err, ErrConflict) {
		t.Fatal("foreign native observation", err)
	}
	obs.Identity = id
	obs.FenceGeneration = token.Generation + 1
	if _, err = o.Reconcile(ctx, p, id); !errors.Is(err, ErrConflict) {
		t.Fatal("future native generation", err)
	}
	obs.FenceGeneration = token.Generation
	snap, err := s.GetSnapshot(ctx, p, id.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	executeOK(t, s, p, Command{Action: "transitionSession", Transition: "stop", OpID: "stop", SessionID: id.SessionID, ExpectedVersion: snap.Session.Version})
	if err = o.CheckFence(ctx, p, token); !errors.Is(err, ErrReadOnly) {
		t.Fatal("stopped session allowed dispatch/finalization", err)
	}
	snap = ownerSnapshot(t, s, p, id.SessionID)
	executeOK(t, s, p, Command{Action: "transitionSession", Transition: "start", OpID: "restart", SessionID: id.SessionID, ExpectedVersion: snap.Session.Version})
	if err = o.CheckFence(ctx, p, token); !errors.Is(err, ErrReadOnly) {
		t.Fatal("restart revived old native token", err)
	}
	if _, _, err = o.ClaimDispatch(ctx, p, id); !errors.Is(err, ErrReadOnly) {
		t.Fatal("restart reclaimed old native operation", err)
	}
}

// Exercise every transaction-owner I/O failure boundary. A failure must never
// publish a partial acceptance/settlement/fence/scrub mutation.
func TestDetectiveOwnerTransactionFailures(t *testing.T) {
	for _, action := range []string{"claim", "check", "status", "fence", "reconcile", "readHandoff", "settleHandoff", "redact"} {
		t.Run(action, func(t *testing.T) {
			for failAt := 1; failAt < 35; failAt++ {
				s, p, o, id, obs, _ := ownerFixture(t, true)
				ctx := context.Background()
				if action != "claim" {
					if _, _, err := o.ClaimDispatch(ctx, p, id); err != nil {
						t.Fatal(err)
					}
				}
				if action == "readHandoff" || action == "settleHandoff" || action == "fence" {
					completeObservation(obs)
					if _, err := o.Reconcile(ctx, p, id); err != nil {
						t.Fatal(err)
					}
				}
				if action == "reconcile" {
					completeObservation(obs)
				}
				calls := 0
				injected := false
				bump := func() error {
					calls++
					if calls == failAt {
						injected = true
						return ErrUnavailable
					}
					return nil
				}
				base := s.repo
				var rows []json.RawMessage
				var after string
				if action == "redact" {
					if err := base.View(ctx, func(r Reader) error { var e error; rows, after, e = r.List(ctx, Inputs, p.Actor, "", 10); return e }); err != nil {
						t.Fatal(err)
					}
				}
				s.repo = &interceptedRepository{Repository: base, failLoad: func(string, string) error { return bump() }, failSave: func(string, string) error { return bump() }, failList: func(string, string, string, int) ([]json.RawMessage, string, error) {
					if err := bump(); err != nil {
						return nil, "", err
					}
					return rows, after, nil
				}}
				var err error
				switch action {
				case "claim":
					_, _, err = o.ClaimDispatch(ctx, p, id)
				case "check":
					err = o.CheckFence(ctx, p, DetectiveFence{Identity: id, Generation: 1})
				case "status":
					_, err = o.ReadStatus(ctx, p, id)
				case "fence":
					err = o.Fence(ctx, p, id, "contentrevoked")
				case "reconcile":
					_, err = o.Reconcile(ctx, p, id)
				case "readHandoff":
					_, _, err = o.ReadHandoff(ctx, p, id)
				case "settleHandoff":
					err = o.SettleHandoff(ctx, p, id, 1, true, "delivery")
				case "redact":
					_, err = o.RedactPins(ctx, p, id, "delete", "", 10)
				}
				s.repo = base
				if !injected {
					if err != nil {
						t.Fatalf("baseline %s: %v", action, err)
					}
					break
				}
				if err == nil {
					t.Fatalf("%s accepted injected failure %d", action, failAt)
				}
				// The original safe status must remain queryable after the failed transaction.
				if _, err = o.ReadStatus(ctx, p, id); err != nil {
					t.Fatalf("failure left unreadable owner state: %v", err)
				}
			}
		})
	}
}
func TestDetectiveOwnerValidationAndBlockedHandoff(t *testing.T) {
	ctx := context.Background()
	s, p, o, id, obs, _ := ownerFixture(t, true)
	if _, err := NewDetectiveOwner(nil, o.acceptedActorGate, o.lookup); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	malformed := id
	malformed.Digest = strings.Repeat("x", 64)
	if _, err := o.ReadStatus(ctx, p, malformed); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	malformed = id
	malformed.RunID = "/bad"
	if _, err := o.ReadStatus(ctx, p, malformed); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if err := o.Fence(ctx, p, id, "unknown"); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := o.RedactPins(ctx, p, id, "delete", "", 101); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, _, err := o.ReadHandoff(ctx, p, id); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
	if err := o.SettleHandoff(ctx, p, id, 1, true, "/bad"); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if err := o.SettleHandoff(ctx, p, id, 1, false, "unexpected"); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if err := o.SettleHandoff(ctx, p, id, 1, true, "receipt"); !errors.Is(err, ErrReadOnly) {
		t.Fatal(err)
	}
	if _, _, err := o.ClaimDispatch(ctx, p, id); err != nil {
		t.Fatal(err)
	}
	old := o.lookup
	o.lookup = func(context.Context, DetectiveIdentity) (DetectiveObservation, error) {
		return DetectiveObservation{}, ErrUnavailable
	}
	if _, err := o.Reconcile(ctx, p, id); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
	o.lookup = old
	completeObservation(obs)
	obs.State = "invented"
	if _, err := o.Reconcile(ctx, p, id); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	obs.State = "completed"
	obs.CaseLinkStatus = "invented"
	if _, err := o.Reconcile(ctx, p, id); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	obs.CaseLinkStatus = "case-changed"
	if _, err := o.Reconcile(ctx, p, id); err != nil {
		t.Fatal(err)
	}
	handoff, version, err := o.ReadHandoff(ctx, p, id)
	if err != nil || handoff.State != "blocked" {
		t.Fatal("blocked handoff lost", err)
	}
	if err = o.SettleHandoff(ctx, p, id, version, true, "receipt"); !errors.Is(err, ErrReadOnly) {
		t.Fatal("native Case link bypass", err)
	}
	if err = o.Fence(ctx, p, id, "contentrevoked"); err != nil {
		t.Fatal(err)
	}
	if _, _, err = o.ReadHandoff(ctx, p, id); !errors.Is(err, ErrReadOnly) {
		t.Fatal("fenced content handoff", err)
	}
	if err = o.Fence(ctx, p, id, "deleted"); err != nil {
		t.Fatal(err)
	}
	if err = o.Fence(ctx, p, id, "canceled"); err != nil {
		t.Fatal(err)
	}
	if ownerRecord(t, s, id).FenceReason != "deleted" {
		t.Fatal("deletion fence downgraded")
	}
	p.Executor = false
	if err = o.CheckFence(ctx, p, DetectiveFence{Identity: id, Generation: 1}); !errors.Is(err, ErrForbidden) {
		t.Fatal(err)
	}
	if err = o.Fence(ctx, p, id, "canceled"); !errors.Is(err, ErrForbidden) {
		t.Fatal(err)
	}
	if _, err = o.Reconcile(ctx, p, id); !errors.Is(err, ErrForbidden) {
		t.Fatal(err)
	}
	if _, _, err = o.ReadHandoff(ctx, p, id); !errors.Is(err, ErrForbidden) {
		t.Fatal(err)
	}
	if err = o.SettleHandoff(ctx, p, id, version, true, "receipt"); !errors.Is(err, ErrForbidden) {
		t.Fatal(err)
	}
	if _, err = o.RedactPins(ctx, p, id, "delete", "", 1); !errors.Is(err, ErrForbidden) {
		t.Fatal(err)
	}
}
func TestDetectiveOwnerHandoffRetriesAndContentAuthorization(t *testing.T) {
	ctx := context.Background()
	s, p, o, id, obs, _ := ownerFixture(t, true)
	if _, _, err := o.ClaimDispatch(ctx, p, id); err != nil {
		t.Fatal(err)
	}
	completeObservation(obs)
	if _, err := o.Reconcile(ctx, p, id); err != nil {
		t.Fatal(err)
	}
	if err := o.SettleHandoff(ctx, p, id, 99, true, "receipt"); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if err := o.SettleHandoff(ctx, p, id, 1, false, ""); err != nil {
		t.Fatal(err)
	}
	if err := o.SettleHandoff(ctx, p, id, 1, false, ""); err != nil {
		t.Fatal("blocked delivery replay", err)
	}
	if err := o.SettleHandoff(ctx, p, id, 2, true, "receipt"); err != nil {
		t.Fatal(err)
	}
	if err := o.SettleHandoff(ctx, p, id, 3, false, ""); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	if err := o.Fence(ctx, p, id, "canceled"); err != nil {
		t.Fatal(err)
	}
	handoff, _ := ownerHandoff(t, s, id)
	if handoff.State != "delivered" {
		t.Fatal("separately delivered Case evidence modified")
	}
	// Content authorization is required for initial erasure acceptance and evidence reads.
	s, p, o, id, obs, _ = ownerFixture(t, true)
	if _, _, err := o.ClaimDispatch(ctx, p, id); err != nil {
		t.Fatal(err)
	}
	completeObservation(obs)
	if _, err := o.Reconcile(ctx, p, id); err != nil {
		t.Fatal(err)
	}
	s.authorize = func(context.Context, Access) error { return ErrForbidden }
	if _, _, err := o.ReadHandoff(ctx, p, id); !errors.Is(err, ErrForbidden) {
		t.Fatal(err)
	}
	if err := o.SettleHandoff(ctx, p, id, 1, true, "receipt"); !errors.Is(err, ErrForbidden) {
		t.Fatal(err)
	}
	if _, err := o.RedactPins(ctx, p, id, "delete", "", 1); !errors.Is(err, ErrForbidden) {
		t.Fatal(err)
	}
}

func TestDetectiveOwnerAcceptanceFencedBeforeFirstClaim(t *testing.T) {
	ctx := context.Background()
	s, p, o, id, _, _ := ownerFixture(t, false)
	snap := ownerSnapshot(t, s, p, id.SessionID)
	executeOK(t, s, p, Command{Action: "transitionSession", Transition: "stop", OpID: "stop-before-claim", SessionID: id.SessionID, ExpectedVersion: snap.Session.Version})
	snap = ownerSnapshot(t, s, p, id.SessionID)
	executeOK(t, s, p, Command{Action: "transitionSession", Transition: "start", OpID: "resume-before-claim", SessionID: id.SessionID, ExpectedVersion: snap.Session.Version})
	if _, _, err := o.ClaimDispatch(ctx, p, id); !errors.Is(err, ErrReadOnly) {
		t.Fatal("resume revived unclaimed acceptance", err)
	}
	status, err := o.ReadStatus(ctx, p, id)
	if err != nil || status.FenceReason != "canceled" {
		t.Fatalf("unclaimed stale status %+v %v", status, err)
	}
	// A genuinely new acceptance after resume uses the current lifecycle generation.
	var input InputSnapshot
	if err = s.repo.View(ctx, func(r Reader) error { _, err := r.Load(ctx, Inputs, "input", &input); return err }); err != nil {
		t.Fatal(err)
	}
	snap = ownerSnapshot(t, s, p, id.SessionID)
	c := PinDetectiveInputCommand{OpID: "new-pin", SessionID: id.SessionID, InputSnapshotID: "new-input", RunID: "new-run", ExpectedVersion: snap.Session.Version, ExpectedContextSetID: snap.Session.ContextSetID, Scope: input.Scope, Pin: *input.Detective}
	c.Pin.Reference.InputSnapshotID = c.InputSnapshotID
	c.Pin.Dispatch.NativeOperationID = "new-native"
	bindPin(t, p, &c)
	if _, err = s.PinDetectiveInput(ctx, p, c); err != nil {
		t.Fatal(err)
	}
	next := id
	next.RunID = c.RunID
	next.NativeOperationID = c.Pin.Dispatch.NativeOperationID
	next.Digest = c.Pin.Dispatch.NativeRequestDigest
	token, _, err := o.ClaimDispatch(ctx, p, next)
	if err != nil || token.SessionGeneration != 1 {
		t.Fatalf("fresh resumed acceptance unavailable: %+v %v", token, err)
	}
}

func TestDetectiveOwnerCorruptPersistedAuthoritiesFailClosed(t *testing.T) {
	for _, kind := range []string{"owner-generation", "input-identity", "session-owner", "run-input", "pending-count", "handoff-run"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			s, p, o, id, obs, _ := ownerFixture(t, true)
			if kind != "input-identity" {
				if _, _, err := o.ClaimDispatch(ctx, p, id); err != nil {
					t.Fatal(err)
				}
			}
			completeObservation(obs)
			if kind == "handoff-run" {
				if _, err := o.Reconcile(ctx, p, id); err != nil {
					t.Fatal(err)
				}
			}
			err := s.repo.Transact(ctx, func(tx Transaction) error {
				switch kind {
				case "owner-generation":
					var v detectiveOwnerRecord
					version, err := tx.Load(ctx, DetectiveOwners, id.RunID, &v)
					if err != nil {
						return err
					}
					v.Generation = 0
					v.Version++
					return tx.Save(ctx, DetectiveOwners, id.RunID, v, version, p.Actor, id.SessionID)
				case "input-identity":
					var v InputSnapshot
					version, err := tx.Load(ctx, Inputs, "input", &v)
					if err != nil {
						return err
					}
					v.SessionID = "foreign"
					return tx.Save(ctx, Inputs, "input", v, version, p.Actor, id.SessionID)
				case "session-owner", "pending-count":
					var v Session
					version, err := tx.Load(ctx, Sessions, id.SessionID, &v)
					if err != nil {
						return err
					}
					if kind == "session-owner" {
						v.Owner = "mallory"
					} else {
						v.PendingRunCount = 0
					}
					v.Version++
					return tx.Save(ctx, Sessions, id.SessionID, v, version, p.Actor, id.SessionID)
				case "run-input":
					var v Run
					version, err := tx.Load(ctx, Runs, id.RunID, &v)
					if err != nil {
						return err
					}
					v.InputSnapshotID = "foreign"
					v.Version++
					return tx.Save(ctx, Runs, id.RunID, v, version, p.Actor, id.SessionID)
				case "handoff-run":
					var v EvidenceHandoff
					version, err := tx.Load(ctx, Outbox, id.RunID, &v)
					if err != nil {
						return err
					}
					v.DetectiveRunID = "foreign"
					return tx.Save(ctx, Outbox, id.RunID, v, version, p.Actor, id.SessionID)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "owner-generation":
				_, err = o.ReadStatus(ctx, p, id)
			case "input-identity":
				_, _, err = o.ClaimDispatch(ctx, p, id)
			case "handoff-run":
				err = o.SettleHandoff(ctx, p, id, 2, true, "receipt")
			default:
				_, err = o.Reconcile(ctx, p, id)
			}
			if err == nil {
				t.Fatal("corrupt authority accepted")
			}
		})
	}
}
func TestDetectiveOwnerScrubRejectsCorruptPages(t *testing.T) {
	for _, kind := range []string{"malformed", "foreign-stored-session", "already-scrubbed"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			s, p, o, id, _, _ := ownerFixture(t, false)
			if kind == "already-scrubbed" {
				if _, err := o.RedactPins(ctx, p, id, "delete", "", 10); err != nil {
					t.Fatal(err)
				}
				var before, after int64
				if err := s.repo.View(ctx, func(r Reader) error {
					var v InputSnapshot
					var err error
					before, err = r.Load(ctx, Inputs, "input", &v)
					return err
				}); err != nil {
					t.Fatal(err)
				}
				if _, err := o.RedactPins(ctx, p, id, "delete", "", 10); err != nil {
					t.Fatal(err)
				}
				if err := s.repo.View(ctx, func(r Reader) error {
					var v InputSnapshot
					var err error
					after, err = r.Load(ctx, Inputs, "input", &v)
					return err
				}); err != nil {
					t.Fatal(err)
				}
				if before != after {
					t.Fatal("repeated scrub rewrote already redacted pin")
				}
				return
			}
			row := json.RawMessage(`{`)
			if kind == "foreign-stored-session" {
				if err := s.repo.Transact(ctx, func(tx Transaction) error {
					var v InputSnapshot
					version, err := tx.Load(ctx, Inputs, "input", &v)
					if err != nil {
						return err
					}
					v.SessionID = "foreign"
					return tx.Save(ctx, Inputs, "input", v, version, p.Actor, id.SessionID)
				}); err != nil {
					t.Fatal(err)
				}
				row = json.RawMessage(`{"id":"input","sessionId":"s1"}`)
			}
			s.repo = &interceptedRepository{Repository: s.repo, listOverride: func(string, string, string, int) ([]json.RawMessage, string, error) {
				return []json.RawMessage{row}, "", nil
			}}
			if _, err := o.RedactPins(ctx, p, id, "delete", "", 10); err == nil {
				t.Fatal("corrupt scrub page accepted")
			}
		})
	}
}
func TestDetectiveOwnerStoppedUnfinishedNativeResultIsPreserved(t *testing.T) {
	ctx := context.Background()
	s, p, o, id, obs, _ := ownerFixture(t, true)
	snap := ownerSnapshot(t, s, p, id.SessionID)
	executeOK(t, s, p, Command{Action: "transitionSession", Transition: "stop", OpID: "before-dispatch", SessionID: id.SessionID, ExpectedVersion: snap.Session.Version})
	if _, _, err := o.ClaimDispatch(ctx, p, id); !errors.Is(err, ErrReadOnly) {
		t.Fatal("stopped claim accepted", err)
	}
	snap = ownerSnapshot(t, s, p, id.SessionID)
	executeOK(t, s, p, Command{Action: "transitionSession", Transition: "start", OpID: "after-stop", SessionID: id.SessionID, ExpectedVersion: snap.Session.Version})
	// Use a fresh fixture for a source result produced before Stop.
	s, p, o, id, obs, _ = ownerFixture(t, true)
	if _, _, err := o.ClaimDispatch(ctx, p, id); err != nil {
		t.Fatal(err)
	}
	snap = ownerSnapshot(t, s, p, id.SessionID)
	executeOK(t, s, p, Command{Action: "transitionSession", Transition: "stop", OpID: "during-dispatch", SessionID: id.SessionID, ExpectedVersion: snap.Session.Version})
	completeObservation(obs)
	obs.CaseLinkStatus = "link-unavailable"
	status, err := o.Reconcile(ctx, p, id)
	if err != nil || status.FenceReason != "canceled" || !status.Produced {
		t.Fatalf("fenced known result lost: %+v %v", status, err)
	}
}

func ownerSnapshot(t *testing.T, s *Service, p Principal, id string) Snapshot {
	t.Helper()
	v, err := s.GetSnapshot(context.Background(), p, id)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestDetectiveOwnerLegacyUnboundAcceptanceCannotDispatch(t *testing.T) {
	ctx := context.Background()
	s, p, o, id, _, _ := ownerFixture(t, false)
	if err := s.repo.Transact(ctx, func(tx Transaction) error {
		var work AcceptedDetectiveWork
		version, err := tx.Load(ctx, DetectiveWork, id.RunID, &work)
		if err != nil {
			return err
		}
		work.OwnerProtocolVersion = 0
		work.Version++
		return tx.Save(ctx, DetectiveWork, id.RunID, work, version, p.Actor, id.SessionID)
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := o.ClaimDispatch(ctx, p, id); !errors.Is(err, ErrUnavailable) {
		t.Fatal("legacy acceptance acquired a new dispatch identity", err)
	}
	if _, err := o.ReadStatus(ctx, p, id); !errors.Is(err, ErrUnavailable) {
		t.Fatal("legacy acceptance treated as generation-bound", err)
	}
}
