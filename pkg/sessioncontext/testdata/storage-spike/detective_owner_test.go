package storage_spike

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/sessioncontext"
)

func TestSQLiteNativeOwnerRecoveryFenceAndScrub(t *testing.T) {
	ctx := context.Background()
	manifest := fixture(t, "sqlite")
	db := open(t, manifest)
	p := sessioncontext.Principal{Actor: "owner", DatabaseID: "session-store", ProjectID: "project", Executor: true}
	s, repo := nativePinService(t, db, p)
	pin := nativePin(t, s, repo, p)
	pin.ClientRequestDigest = strings.Repeat("a", 64)
	if _, err := s.PinDetectiveInput(ctx, p, pin); err != nil {
		t.Fatal(err)
	}
	d := pin.Pin.Dispatch
	id := sessioncontext.DetectiveIdentity{Namespace: repo.NativeNamespace(), Actor: p.Actor, SessionID: pin.SessionID, RunID: pin.RunID, NativeStoreID: d.NativeStoreID, NativeSpaceID: d.NativeSpaceID, NativeProjectID: d.NativeProjectID, NativeOperationID: d.NativeOperationID, Digest: d.NativeRequestDigest}
	observation := sessioncontext.DetectiveObservation{Identity: id, FenceGeneration: 1}
	owner := func(s *sessioncontext.Service) *sessioncontext.DetectiveOwner {
		t.Helper()
		o, err := sessioncontext.NewDetectiveOwner(s, func(context.Context, sessioncontext.Principal, sessioncontext.DetectiveIdentity) error { return nil }, func(context.Context, sessioncontext.DetectiveIdentity) (sessioncontext.DetectiveObservation, error) {
			return observation, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		return o
	}
	o := owner(s)
	token, _, first, err := o.ClaimDispatchOnce(ctx, p, id)
	if err != nil || !first {
		t.Fatal("first claim failed", err)
	}
	observation.Produced = true
	observation.NativeRunID = "native-result"
	if _, err = o.Reconcile(ctx, p, id); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	db = open(t, manifest)
	s, repo = nativePinService(t, db, p)
	o = owner(s)
	status, err := o.ReadStatus(ctx, p, id)
	if err != nil || !status.Produced || status.Done {
		t.Fatalf("durable produced result lost: %+v %v", status, err)
	}
	again, _, first, err := o.ClaimDispatchOnce(ctx, p, id)
	if err != nil || first || again != token {
		t.Fatal("restart replaced dispatch identity", err)
	}
	raw, err := sql.Open("sqlite", filepath.Join(filepath.Dir(manifest), "data.sqlite"))
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()
	if _, err = raw.Exec(`CREATE TRIGGER fail_owner_settlement BEFORE UPDATE ON "sessionContextDetectiveOwners" BEGIN SELECT RAISE(ABORT, 'injected owner write failure'); END`); err != nil {
		t.Fatal(err)
	}
	observation.Done = true
	observation.State = "completed"
	observation.NativeReceiptID = "native-receipt"
	if _, err = o.Reconcile(ctx, p, id); err == nil {
		t.Fatal("partial owner settlement committed")
	}
	snap, err := s.GetSnapshot(ctx, p, p.Actor)
	if err != nil || snap.Session.PendingRunCount != 1 {
		t.Fatal("atomic settlement rollback failed", err)
	}
	run, err := s.ReadRun(ctx, p, id.RunID)
	if err != nil || run.State != "accepted" {
		t.Fatal("partial run settlement", err)
	}
	if _, err = raw.Exec(`DROP TRIGGER fail_owner_settlement`); err != nil {
		t.Fatal(err)
	}
	if _, err = o.Reconcile(ctx, p, id); err != nil {
		t.Fatal(err)
	}
	if err = o.Fence(ctx, p, id, "contentrevoked"); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	db = open(t, manifest)
	s, repo = nativePinService(t, db, p)
	o = owner(s)
	if err = o.CheckFence(ctx, p, token); !errors.Is(err, sessioncontext.ErrReadOnly) {
		t.Fatal("durable fence lost on reopen", err)
	}
	if _, err = o.RedactPins(ctx, p, id, "delete", "", 1); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	db = open(t, manifest)
	s, repo = nativePinService(t, db, p)
	o = owner(s)
	if _, err = s.PinDetectiveInput(ctx, p, pin); !errors.Is(err, sessioncontext.ErrReadOnly) {
		t.Fatal("replay recreated redacted pin", err)
	}
	if _, err = s.ReadDetectiveDispatch(ctx, p, id.SessionID, id.RunID); !errors.Is(err, sessioncontext.ErrReadOnly) {
		t.Fatal("redacted dispatch readable", err)
	}
	status, err = o.ReadStatus(ctx, p, id)
	if err != nil || status.FenceReason != "deleted" || !status.Done {
		t.Fatalf("safe redacted status lost: %+v %v", status, err)
	}
	accepted, err := o.ResolveOperation(ctx, p, pin.OpID)
	if err != nil || accepted.Identity != id || accepted.ClientRequestDigest != pin.ClientRequestDigest {
		t.Fatalf("durable redacted operation mapping lost: %+v %v", accepted, err)
	}

}

// This is session transaction-owner conformance only. It does not claim native
// hosted acceptance/finalization integration or cross-store race protection.
func TestFirestoreEmulatorNativeOwnerFenceAndStatus(t *testing.T) {
	if os.Getenv("FIRESTORE_EMULATOR_HOST") == "" {
		t.Skip("hosted transaction-owner conformance requires existing FIRESTORE_EMULATOR_HOST")
	}
	ctx := context.Background()
	manifest := filepath.Join(t.TempDir(), "firestore.yaml")
	body := "database: {id: session-store, schema_mode: strict}\nstorage: {engine: firestore, firestore: {project: context-bag-emulator}}\nschemas:\n" + sessioncontext.ManifestCollectionsYAML()
	if err := os.WriteFile(manifest, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	p := sessioncontext.Principal{Actor: fmt.Sprintf("native-owner-fence-%d", time.Now().UnixNano()), DatabaseID: "session-store", ProjectID: "project", Executor: true}
	db := open(t, manifest)
	repo, err := sessioncontext.NewNativePinRepository(db, record.NewKeyWithID("privateOwners", p.Actor), "firestore", "space", p.ProjectID)
	if err != nil {
		t.Fatal(err)
	}
	s, err := sessioncontext.NewNativePinService(repo, func(context.Context, sessioncontext.Access) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	pin := nativePin(t, s, repo, p)
	if _, err := s.PinDetectiveInput(ctx, p, pin); err != nil {
		t.Fatal(err)
	}
	d := pin.Pin.Dispatch
	id := sessioncontext.DetectiveIdentity{Namespace: repo.NativeNamespace(), Actor: p.Actor, SessionID: pin.SessionID, RunID: pin.RunID, NativeStoreID: d.NativeStoreID, NativeProjectID: d.NativeProjectID, NativeOperationID: d.NativeOperationID, Digest: d.NativeRequestDigest}
	o, err := sessioncontext.NewDetectiveOwner(s, func(context.Context, sessioncontext.Principal, sessioncontext.DetectiveIdentity) error { return nil }, func(context.Context, sessioncontext.DetectiveIdentity) (sessioncontext.DetectiveObservation, error) {
		return sessioncontext.DetectiveObservation{Identity: id, FenceGeneration: 1}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := o.ClaimDispatch(ctx, p, id)
	if err != nil {
		t.Fatal(err)
	}
	if err = o.Fence(ctx, p, id, "contentrevoked"); err != nil {
		t.Fatal(err)
	}
	if err = o.CheckFence(ctx, p, token); !errors.Is(err, sessioncontext.ErrReadOnly) {
		t.Fatal("hosted transaction-owner fence bypass", err)
	}
	if status, err := o.ReadStatus(ctx, p, id); err != nil || status.FenceReason != "contentrevoked" {
		t.Fatalf("hosted accepted status: %+v %v", status, err)
	}
}

func TestSQLiteNativeTransactionFenceReadsUncommittedMarker(t *testing.T) {
	ctx := context.Background()
	db := open(t, fixture(t, "sqlite"))
	p := sessioncontext.Principal{Actor: "transaction-owner", DatabaseID: "session-store", ProjectID: "project", Executor: true}
	s, repo := nativePinService(t, db, p)
	pin := nativePin(t, s, repo, p)
	if _, err := s.PinDetectiveInput(ctx, p, pin); err != nil {
		t.Fatal(err)
	}
	d := pin.Pin.Dispatch
	id := sessioncontext.DetectiveIdentity{Namespace: repo.NativeNamespace(), Actor: p.Actor, SessionID: pin.SessionID, RunID: pin.RunID, NativeStoreID: d.NativeStoreID, NativeProjectID: d.NativeProjectID, NativeOperationID: d.NativeOperationID, Digest: d.NativeRequestDigest}
	o, err := sessioncontext.NewDetectiveOwner(s, func(context.Context, sessioncontext.Principal, sessioncontext.DetectiveIdentity) error { return nil }, func(context.Context, sessioncontext.DetectiveIdentity) (sessioncontext.DetectiveObservation, error) {
		return sessioncontext.DetectiveObservation{}, sessioncontext.ErrNotFound
	})
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := o.ClaimDispatch(ctx, p, id)
	if err != nil {
		t.Fatal(err)
	}
	rollback := errors.New("intentional native transaction rollback")
	err = db.DB().RunReadwriteTransaction(ctx, func(txCtx context.Context, tx dal.ReadwriteTransaction) error {
		marker := sessioncontext.SessionRedaction{SessionID: id.SessionID, Owner: p.Actor, Namespace: repo.NativeNamespace(), DeleteOpID: "delete", Version: 1}
		body, e := json.Marshal(marker)
		if e != nil {
			return e
		}
		data := map[string]any{"recordId": id.SessionID, "owner": p.Actor, "sessionId": id.SessionID, "version": int64(1), "payload": string(body)}
		if e = tx.Set(txCtx, record.NewRecordWithData(record.NewKeyWithID(sessioncontext.Redactions, id.SessionID), data)); e != nil {
			return e
		}
		reader, e := repo.ReaderInTransaction(tx)
		if e != nil {
			return e
		}
		if e = o.CheckFenceWithReader(txCtx, p, token, reader); !errors.Is(e, sessioncontext.ErrReadOnly) {
			t.Fatalf("native transaction missed own fence: %v", e)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatal(err)
	}
	if err = o.CheckFence(ctx, p, token); err != nil {
		t.Fatal("uncommitted marker escaped native rollback", err)
	}
}
