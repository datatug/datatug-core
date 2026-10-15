package sessioncontext

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/datatug/datatug-core/pkg/apicontract"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func operationFixture(t *testing.T) (*Service, Principal, *DetectiveOwner, PinDetectiveInputCommand) {
	t.Helper()
	s, p := pinService(t)
	c := pinCommand(t, s, p)
	c.ClientRequestDigest = strings.Repeat("a", 64)
	if _, err := s.PinDetectiveInput(context.Background(), p, c); err != nil {
		t.Fatal(err)
	}
	o, err := NewDetectiveOwner(s, func(context.Context, Principal, DetectiveIdentity) error { return nil }, func(context.Context, DetectiveIdentity) (DetectiveObservation, error) {
		return DetectiveObservation{}, ErrNotFound
	})
	if err != nil {
		t.Fatal(err)
	}
	return s, p, o, c
}
func TestDetectiveOperationMappingSurvivesContentChangesAndRedaction(t *testing.T) {
	ctx := context.Background()
	s, p, o, c := operationFixture(t)
	accepted, err := o.ResolveOperation(ctx, p, c.OpID)
	if err != nil || accepted.ClientRequestDigest != c.ClientRequestDigest {
		t.Fatalf("accepted mapping %+v %v", accepted, err)
	}
	// Mutable caller/current definitions may change. Recovery resolves the old
	// acceptance without consulting them, and the claim still returns old bytes.
	c.Pin.Definition.Check.Title = "edited Check"
	c.Pin.Definition.Queries[0].Revision = "edited-query"
	bindPin(t, p, &c)
	retry, err := o.ResolveOperation(ctx, p, c.OpID)
	if err != nil || retry != accepted {
		t.Fatal("mutable definition changed accepted identity", err)
	}
	token, pin, first, err := o.ClaimDispatchOnce(ctx, p, accepted.Identity)
	if err != nil || !first || pin.Definition.Check.Title == "edited Check" {
		t.Fatal("first claim lost exact private pin", err)
	}
	same, _, first, err := o.ClaimDispatchOnce(ctx, p, accepted.Identity)
	if err != nil || first || same != token {
		t.Fatal("retry granted dispatch", err)
	}
	if _, err = o.Reconcile(ctx, p, accepted.Identity); !errors.Is(err, ErrNotFound) {
		t.Fatal("native absence treated as execution permission", err)
	}
	// A changed transport body must conflict before any dispatch, even if native
	// definition metadata otherwise names the same Check.
	c.ClientRequestDigest = strings.Repeat("b", 64)
	if _, err = s.PinDetectiveInput(ctx, p, c); !errors.Is(err, ErrConflict) {
		t.Fatal("changed body accepted under same client operation", err)
	}
	if err = o.Fence(ctx, p, accepted.Identity, "contentrevoked"); err != nil {
		t.Fatal(err)
	}
	s.authorize = func(context.Context, Access) error { return ErrForbidden }
	if again, err := o.ResolveOperation(ctx, p, c.OpID); err != nil || again != accepted {
		t.Fatal("content revocation hid safe accepted mapping", err)
	}
	s.authorize = func(context.Context, Access) error { return nil }
	if _, err = o.RedactPins(ctx, p, accepted.Identity, "delete", "", 10); err != nil {
		t.Fatal(err)
	}
	s.authorize = func(context.Context, Access) error { return ErrForbidden }
	s.repo = &interceptedRepository{Repository: s.repo, failLoad: func(collection, id string) error {
		if collection == Inputs || collection == Sessions {
			return ErrUnavailable
		}
		return nil
	}}
	if again, err := o.ResolveOperation(ctx, p, c.OpID); err != nil || again != accepted {
		t.Fatal("redacted metadata lookup read private content", err)
	}
	o.acceptedActorGate = func(context.Context, Principal, DetectiveIdentity) error { return ErrForbidden }
	if _, err = o.ResolveOperation(ctx, p, c.OpID); !errors.Is(err, ErrForbidden) {
		t.Fatal("inactive account resolved acceptance", err)
	}
}
func TestDetectiveClientRequestDigestValidationAndEmptyHashCompatibility(t *testing.T) {
	ctx := context.Background()
	s, p := pinService(t)
	c := pinCommand(t, s, p)
	for _, bad := range []string{"short", strings.Repeat("z", 64), strings.Repeat("A", 64)} {
		c.ClientRequestDigest = bad
		if _, err := s.PinDetectiveInput(ctx, p, c); !errors.Is(err, ErrInvalid) {
			t.Fatal("invalid client digest accepted", err)
		}
	}
	c.ClientRequestDigest = ""
	result, err := s.PinDetectiveInput(ctx, p, c)
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := s.prepareDetectivePin(ctx, p, c)
	if err != nil {
		t.Fatal(err)
	}
	// Preserve the exact pre-addition empty-client-digest acceptance hash.
	old, _ := json.Marshal(struct {
		Actor                                         string
		Namespace                                     NativeNamespace
		OpID, SessionID, InputID, RunID, ContextSetID string
		Version                                       int64
		Attachments                                   []EntryRef
		Pin                                           DetectiveInputPin
	}{p.Actor, *s.nativeNamespace, c.OpID, c.SessionID, c.InputSnapshotID, c.RunID, c.ExpectedContextSetID, c.ExpectedVersion, c.Attachments, canonical})
	hash := sha256.Sum256(old)
	if result.Receipt.PayloadDigest != hex.EncodeToString(hash[:]) {
		t.Fatal("legacy empty digest changed operation hash")
	}
	o, err := NewDetectiveOwner(s, func(context.Context, Principal, DetectiveIdentity) error { return nil }, func(context.Context, DetectiveIdentity) (DetectiveObservation, error) {
		return DetectiveObservation{}, ErrNotFound
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = o.ResolveOperation(ctx, p, c.OpID); !errors.Is(err, ErrUnavailable) {
		t.Fatal("unbound legacy acceptance entered transport", err)
	}
}
func TestDetectiveOperationLookupDeniesWrongIdentityAndCorruption(t *testing.T) {
	for _, kind := range []string{"actor", "database", "project", "namespace", "bad-op", "missing", "ordinary", "receipt-op", "receipt-actor", "receipt-run", "receipt-session", "work-session", "work-client", "work-acceptance", "work-digest", "work-version", "work-native"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			s, p, o, c := operationFixture(t)
			op := c.OpID
			switch kind {
			case "actor":
				p.Actor = "mallory"
			case "database":
				p.DatabaseID = "other"
			case "project":
				p.ProjectID = "other"
			case "namespace":
				s.nativeNamespace.NamespaceID = "other"
			case "bad-op":
				op = "/bad"
			case "missing":
				op = "missing"
			default:
				if err := s.repo.Transact(ctx, func(tx Transaction) error {
					if strings.HasPrefix(kind, "receipt-") || kind == "ordinary" {
						var v Receipt
						version, err := tx.Load(ctx, Operations, opKey(p, c.OpID), &v)
						if err != nil {
							return err
						}
						switch kind {
						case "ordinary":
							v.Action = "createSession"
						case "receipt-op":
							v.OpID = "other"
						case "receipt-actor":
							v.Actor = "other"
						case "receipt-run":
							v.ResourceID = "other"
						case "receipt-session":
							v.SessionID = "other"
						}
						return tx.Save(ctx, Operations, opKey(p, c.OpID), v, version, p.Actor, c.SessionID)
					}
					var v AcceptedDetectiveWork
					version, err := tx.Load(ctx, DetectiveWork, c.RunID, &v)
					if err != nil {
						return err
					}
					v.Version++
					switch kind {
					case "work-session":
						v.SessionID = "other"
					case "work-client":
						v.ClientOperationID = "other"
					case "work-acceptance":
						v.AcceptanceDigest = strings.Repeat("b", 64)
					case "work-digest":
						v.ClientRequestDigest = "not-a-digest"
					case "work-version":
						v.OwnerProtocolVersion = 0
					case "work-native":
						v.Dispatch.NativeOperationID = "/invalid"
					}
					return tx.Save(ctx, DetectiveWork, c.RunID, v, version, p.Actor, c.SessionID)
				}); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := o.ResolveOperation(ctx, p, op); err == nil {
				t.Fatal("wrong/corrupt accepted mapping disclosed")
			}
		})
	}
}
func TestDetectiveOperationLookupReadFailures(t *testing.T) {
	for failAt := 1; failAt <= 5; failAt++ {
		s, p, o, c := operationFixture(t)
		calls := 0
		s.repo = &interceptedRepository{Repository: s.repo, failLoad: func(string, string) error {
			calls++
			if calls == failAt {
				return ErrUnavailable
			}
			return nil
		}}
		if _, err := o.ResolveOperation(context.Background(), p, c.OpID); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("load failure %d: %v", failAt, err)
		}
	}
}

type repeatedClaimRepository struct{ Repository }

func (r repeatedClaimRepository) Transact(ctx context.Context, fn func(Transaction) error) error {
	if err := r.Repository.Transact(ctx, fn); err != nil {
		return err
	}
	return r.Repository.Transact(ctx, fn)
}
func TestDetectiveFirstClaimOnlyOneWinnerAndRetryFlagReset(t *testing.T) {
	ctx := context.Background()
	_, p, o, c := operationFixture(t)
	accepted, err := o.ResolveOperation(ctx, p, c.OpID)
	if err != nil {
		t.Fatal(err)
	}
	var winners atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, first, err := o.ClaimDispatchOnce(ctx, p, accepted.Identity)
			if err != nil {
				if first || (!errors.Is(err, ErrUnavailable) && !errors.Is(err, ErrConflict)) {
					t.Error("failed concurrent claim granted dispatch or unexpected error", err)
				}
			}
			if first {
				winners.Add(1)
			}
		}()
	}
	wg.Wait()
	if winners.Load() != 1 {
		t.Fatalf("first claims=%d", winners.Load())
	}
	var s *Service
	s, p, o, c = operationFixture(t)
	accepted, err = o.ResolveOperation(ctx, p, c.OpID)
	if err != nil {
		t.Fatal(err)
	}
	s.repo = repeatedClaimRepository{s.repo}
	if _, _, first, err := o.ClaimDispatchOnce(ctx, p, accepted.Identity); err != nil || first {
		t.Fatal("retried callback leaked previous first-claim flag", err)
	}
	s, p, o, c = operationFixture(t)
	accepted, err = o.ResolveOperation(ctx, p, c.OpID)
	if err != nil {
		t.Fatal(err)
	}
	s.repo = &interceptedRepository{Repository: s.repo, failSave: func(string, string) error { return ErrUnavailable }}
	if _, _, first, err := o.ClaimDispatchOnce(ctx, p, accepted.Identity); !errors.Is(err, ErrUnavailable) || first {
		t.Fatal("failed claim granted dispatch", err)
	}
}

func TestDetectiveRecoveryReceiptIndependentlyBindsValidMetadata(t *testing.T) {
	for _, kind := range []string{"client-digest", "native-operation", "native-digest", "native-project", "namespace", "input-map", "binding-refs", "generation", "protocol", "state", "missing-receipt-binding", "changed-receipt-binding"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			s, p, o, c := operationFixture(t)
			if err := s.repo.Transact(ctx, func(tx Transaction) error {
				if strings.Contains(kind, "receipt-binding") {
					var receipt Receipt
					version, err := tx.Load(ctx, Operations, opKey(p, c.OpID), &receipt)
					if err != nil {
						return err
					}
					receipt.RecoveryBindingDigest = ""
					if kind == "changed-receipt-binding" {
						receipt.RecoveryBindingDigest = strings.Repeat("b", 64)
					}
					return tx.Save(ctx, Operations, opKey(p, c.OpID), receipt, version, p.Actor, c.SessionID)
				}
				var work AcceptedDetectiveWork
				version, err := tx.Load(ctx, DetectiveWork, c.RunID, &work)
				if err != nil {
					return err
				}
				work.Version++
				switch kind {
				case "client-digest":
					work.ClientRequestDigest = strings.Repeat("b", 64)
				case "native-operation":
					work.Dispatch.NativeOperationID = "different-safe-operation"
				case "native-digest":
					work.Dispatch.NativeRequestDigest = strings.Repeat("b", 64)
				case "native-project":
					work.Dispatch.NativeProjectID = "different-safe-project"
				case "namespace":
					work.Namespace.NamespaceID = "different-safe-namespace"
				case "input-map":
					work.InputSnapshotID = "different-input"
					work.Reference.InputSnapshotID = work.InputSnapshotID
				case "generation":
					work.SessionGeneration++
				case "protocol":
					work.OwnerProtocolVersion++
				case "state":
					work.State = "canceled"
				case "binding-refs":
					work.Reference.Bindings = []apicontract.SessionInputBindingRef{{InputName: "safe-input", Origin: "manual"}}
				}
				return tx.Save(ctx, DetectiveWork, c.RunID, work, version, p.Actor, c.SessionID)
			}); err != nil {
				t.Fatal(err)
			}
			out, err := o.ResolveOperation(ctx, p, c.OpID)
			if !errors.Is(err, ErrConflict) || out != (AcceptedDetectiveOperation{}) {
				t.Fatalf("valid metadata tampering escaped independent receipt binding: %+v %v", out, err)
			}
		})
	}
}

func TestDetectiveClaimRechecksReceiptBindingAfterResolution(t *testing.T) {
	for _, kind := range []string{"lifecycle-generation", "cleared-client-binding", "cleared-digest", "invalid-client-operation", "missing-receipt"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			s, p, o, c := operationFixture(t)
			accepted, err := o.ResolveOperation(ctx, p, c.OpID)
			if err != nil {
				t.Fatal(err)
			}
			if kind == "lifecycle-generation" || kind == "cleared-client-binding" {
				snap := ownerSnapshot(t, s, p, c.SessionID)
				executeOK(t, s, p, Command{Action: "transitionSession", Transition: "stop", OpID: "stop", SessionID: c.SessionID, ExpectedVersion: snap.Session.Version})
				snap = ownerSnapshot(t, s, p, c.SessionID)
				executeOK(t, s, p, Command{Action: "transitionSession", Transition: "start", OpID: "resume", SessionID: c.SessionID, ExpectedVersion: snap.Session.Version})
			}
			if err = s.repo.Transact(ctx, func(tx Transaction) error {
				var work AcceptedDetectiveWork
				version, err := tx.Load(ctx, DetectiveWork, c.RunID, &work)
				if err != nil {
					return err
				}
				work.Version++
				if kind == "cleared-client-binding" {
					work.SessionGeneration = 1
					work.ClientOperationID = ""
					work.ClientRequestDigest = ""
				} else if kind == "cleared-digest" {
					work.ClientRequestDigest = ""
				} else if kind == "lifecycle-generation" {
					work.SessionGeneration = 1
				} else if kind == "invalid-client-operation" {
					work.ClientOperationID = "/invalid"
				} else {
					work.ClientOperationID = "missing-receipt"
				}
				return tx.Save(ctx, DetectiveWork, c.RunID, work, version, p.Actor, c.SessionID)
			}); err != nil {
				t.Fatal(err)
			}
			_, _, first, err := o.ClaimDispatchOnce(ctx, p, accepted.Identity)
			if err == nil || first {
				t.Fatal("changed authority revived resolved acceptance", err)
			}
		})
	}
}
