package sessioncontext

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

// These tests prove transaction reuse and parent binding, not real hosted
// Firestore race conformance. The hosted adapter must provide that receipt.
func TestDetectiveFenceUsesNativeOwnerTransactionAndNamespace(t *testing.T) {
	ctx := context.Background()
	db := dalgo2memory.New(dalgo2memory.SingleWriterProfile(), dalgo2memory.WithInterleavedReadsAndWritesInTransaction())
	parent := record.NewKeyWithID("privateOwners", "alice")
	repo, err := NewNativePinRepository(testMountedDatabase{"db1", db}, parent, "firestore", "space", "project1")
	if err != nil {
		t.Fatal(err)
	}
	s, err := NewNativePinService(repo, func(context.Context, Access) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	p := Principal{Actor: "alice", DatabaseID: "db1", ProjectID: "project1", Executor: true}
	c := pinCommand(t, s, p)
	if _, err = s.PinDetectiveInput(ctx, p, c); err != nil {
		t.Fatal(err)
	}
	d := c.Pin.Dispatch
	id := DetectiveIdentity{*s.nativeNamespace, p.Actor, c.SessionID, c.RunID, d.NativeStoreID, d.NativeSpaceID, d.NativeProjectID, d.NativeOperationID, d.NativeRequestDigest}
	owner, err := NewDetectiveOwner(s, func(context.Context, Principal, DetectiveIdentity) error { return nil }, func(context.Context, DetectiveIdentity) (DetectiveObservation, error) {
		return DetectiveObservation{}, ErrNotFound
	})
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := owner.ClaimDispatch(ctx, p, id)
	if err != nil {
		t.Fatal(err)
	}
	// Neither transaction-reader construction nor checking may create a nested
	// transaction. A nested transaction would also deadlock the single writer.
	s.repo = &interceptedRepository{Repository: repo, viewErr: ErrUnavailable, transactErr: ErrUnavailable}
	err = db.RunReadwriteTransaction(ctx, func(txCtx context.Context, tx dal.ReadwriteTransaction) error {
		reader, e := repo.ReaderInTransaction(tx)
		if e != nil {
			return e
		}
		if _, canWrite := reader.(Transaction); canWrite {
			t.Fatal("native reader exposes queued writes")
		}
		if e = owner.CheckFenceWithReader(txCtx, p, token, reader); e != nil {
			return e
		}
		// An identical raw key outside the repository parent must not be read.
		unscoped, e := NewDALRepository(testMountedDatabase{"db1", db}, nil)
		if e != nil {
			return e
		}
		wrong, e := unscoped.ReaderInTransaction(tx)
		if e != nil {
			return e
		}
		if e = owner.CheckFenceWithReader(txCtx, p, token, wrong); !errors.Is(e, ErrNotFound) {
			t.Fatalf("parent boundary bypass: %v", e)
		}
		var state detectiveOwnerRecord
		if _, e = reader.Load(txCtx, DetectiveOwners, id.RunID, &state); e != nil {
			return e
		}
		state.Generation++
		state.FenceReason = "contentrevoked"
		state.Version++
		encoded, e := json.Marshal(state)
		if e != nil {
			return e
		}
		value := map[string]any{"recordId": id.RunID, "version": state.Version, "owner": p.Actor, "sessionId": id.SessionID, "payload": string(encoded)}
		if e = tx.Set(txCtx, record.NewRecordWithData(record.NewKeyWithParentAndID(parent, DetectiveOwners, id.RunID), value)); e != nil {
			return e
		}
		// Construct a fresh view of the SAME transaction after its write. SQLite /
		// single-writer adapters expose their own uncommitted writes. Firestore's
		// adapter instead performs these fence reads before its first native write.
		current, e := repo.ReaderInTransaction(tx)
		if e != nil {
			return e
		}
		if e = owner.CheckFenceWithReader(txCtx, p, token, current); !errors.Is(e, ErrReadOnly) {
			t.Fatalf("uncommitted native fence invisible: %v", e)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err = owner.CheckFence(ctx, p, token); !errors.Is(err, ErrUnavailable) {
		t.Fatal("ordinary CheckFence bypassed repository View", err)
	}
}

type nilNativeReadTransaction struct{ dal.ReadTransaction }

func TestDetectiveTransactionPortsRejectMissingDependencies(t *testing.T) {
	s, p, owner, id, _, _ := ownerFixture(t, false)
	repo := s.repo.(*DALRepository)
	if _, err := repo.ReaderInTransaction(nil); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	var typedNil *nilNativeReadTransaction
	if _, err := repo.ReaderInTransaction(typedNil); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	var nilRepo *DALRepository
	if _, err := nilRepo.ReaderInTransaction(nil); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	token := DetectiveFence{Identity: id, Generation: 1}
	if err := owner.CheckFenceWithReader(context.Background(), p, token, nil); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	var nilReader *dalTransaction
	if err := owner.CheckFenceWithReader(context.Background(), p, token, nilReader); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	p.Executor = false
	if err := owner.CheckFenceWithReader(context.Background(), p, token, transactionReader{nilReader}); !errors.Is(err, ErrForbidden) {
		t.Fatal(err)
	}
}
