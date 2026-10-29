package sessioncontext

import (
	"context"
	"errors"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

func TestRetainedAdmissionBindsBeforeAfterAndKeepsItsCopiesIndependent(t *testing.T) {
	repo, _ := retainedInventoryFixture(t)
	seedRetainedEnvelope(t, repo, Sessions, "session", "alice")
	calls, fences := 0, 0
	admitted, err := repo.WithRetainedWriteAdmission(func(ctx context.Context, tx dal.ReadwriteTransaction, namespace NativeNamespace, changes []RetainedWrite) (func() error, error) {
		calls++
		if namespace != repo.NativeNamespace() || len(changes) != 1 || changes[0].Before == nil || changes[0].Before.Version != 1 || changes[0].After.Version != 2 || changes[0].After.Owner != "alice" {
			t.Fatalf("wrong admission batch: %+v", changes)
		}
		// An adapter cannot accidentally modify the records it just admitted.
		changes[0].After.Payload[0] = 'x'
		changes[0].Before.Payload[0] = 'x'
		return func() error {
			fences++
			return tx.Set(ctx, record.NewRecordWithData(record.NewKeyWithParentAndID(repo.parent, "lifecycleFences", "generation"), map[string]any{"generation": 1}))
		}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	err = admitted.Transact(context.Background(), func(tx Transaction) error {
		return tx.Save(context.Background(), Sessions, "session", map[string]string{"id": "session", "title": "changed"}, 1, "alice", "session-a")
	})
	if err != nil || calls != 1 || fences != 1 {
		t.Fatalf("admission: %d/%d %v", calls, fences, err)
	}
	err = repo.View(context.Background(), func(reader Reader) error {
		var saved map[string]string
		version, err := reader.Load(context.Background(), Sessions, "session", &saved)
		if err != nil || version != 2 || saved["title"] != "changed" {
			t.Fatalf("adapter changed pending payload: %v %+v", err, saved)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestRetainedAdmissionRefusalAndFenceFailureRollBackSourceWrites(t *testing.T) {
	for _, failure := range []string{"admission", "fence", "missing closure"} {
		t.Run(failure, func(t *testing.T) {
			repo, db := retainedInventoryFixture(t)
			admitted, err := repo.WithRetainedWriteAdmission(func(ctx context.Context, tx dal.ReadwriteTransaction, _ NativeNamespace, changes []RetainedWrite) (func() error, error) {
				if len(changes) != 1 || changes[0].Before != nil || changes[0].After.Version != 1 {
					t.Fatalf("wrong insertion projection: %+v", changes)
				}
				if failure == "admission" {
					return nil, ErrReadOnly
				}
				if failure == "missing closure" {
					return nil, nil
				}
				return func() error {
					if err := tx.Set(ctx, record.NewRecordWithData(record.NewKeyWithParentAndID(repo.parent, "lifecycleFences", "generation"), map[string]any{"generation": 1})); err != nil {
						return err
					}
					return ErrReadOnly
				}, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			err = admitted.Transact(context.Background(), func(tx Transaction) error {
				return tx.Save(context.Background(), Sessions, "session", map[string]string{"id": "session"}, 0, "alice", "session-a")
			})
			if err == nil {
				t.Fatal("refused source write succeeded")
			}
			for _, key := range []*record.Key{record.NewKeyWithParentAndID(repo.parent, Sessions, "session"), record.NewKeyWithParentAndID(repo.parent, "lifecycleFences", "generation")} {
				if err := db.Get(context.Background(), record.NewRecordWithData(key, &map[string]any{})); !record.IsNotFound(err) {
					t.Fatalf("partial write retained: %s %v", key, err)
				}
			}
		})
	}
}

func TestRetainedBufferedAdmissionRequiresPreparationAndSealsQueuedWrites(t *testing.T) {
	for _, prepare := range []bool{false, true} {
		repo, db := retainedInventoryFixture(t)
		calls := 0
		admitted, err := repo.WithRetainedWriteAdmission(func(ctx context.Context, tx dal.ReadwriteTransaction, _ NativeNamespace, _ []RetainedWrite) (func() error, error) {
			calls++
			// This read must occur before the caller's first native write.
			_ = tx.Get(ctx, record.NewRecordWithData(record.NewKeyWithParentAndID(repo.parent, "lifecycleFences", "generation"), &map[string]any{}))
			return func() error { return nil }, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		err = db.RunReadwriteTransaction(context.Background(), func(ctx context.Context, tx dal.ReadwriteTransaction) error {
			buffer, err := admitted.BufferInTransaction(tx)
			if err != nil {
				return err
			}
			var escaped Transaction
			if err = buffer.Transact(ctx, func(unit Transaction) error {
				escaped = unit
				return unit.Save(ctx, Sessions, "session", map[string]string{"id": "session"}, 0, "alice", "session-a")
			}); err != nil {
				return err
			}
			if prepare {
				if err = buffer.PrepareFlush(ctx); err != nil {
					return err
				}
				if err = buffer.Transact(ctx, func(Transaction) error { t.Fatal("command accepted after preparation"); return nil }); !errors.Is(err, ErrUnavailable) {
					t.Fatal(err)
				}
				if err = escaped.Save(ctx, Sessions, "late", map[string]string{"id": "late"}, 0, "alice", "session-a"); !errors.Is(err, ErrUnavailable) {
					t.Fatalf("escaped transaction changed admitted batch: %v", err)
				}
			}
			return buffer.Flush(ctx)
		})
		if prepare && (err != nil || calls != 1) || !prepare && !errors.Is(err, ErrUnavailable) {
			t.Fatalf("prepare=%v calls=%d result=%v", prepare, calls, err)
		}
	}
}

func TestRetainedAdmissionCancellationRollsBackBeforeAnySourceCommit(t *testing.T) {
	for _, stage := range []string{"during admission", "after prepare", "during fence"} {
		t.Run(stage, func(t *testing.T) {
			repo, db := retainedInventoryFixture(t)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			fences := 0
			fence := record.NewKeyWithParentAndID(repo.parent, "lifecycleFences", "generation")
			admitted, err := repo.WithRetainedWriteAdmission(func(_ context.Context, tx dal.ReadwriteTransaction, _ NativeNamespace, _ []RetainedWrite) (func() error, error) {
				if stage == "during admission" {
					cancel()
				}
				return func() error {
					fences++
					if stage == "during fence" {
						cancel()
					}
					return tx.Set(context.Background(), record.NewRecordWithData(fence, map[string]any{"generation": 1}))
				}, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			save := func(unit Transaction) error {
				return unit.Save(ctx, Sessions, "session", map[string]string{"id": "session"}, 0, "alice", "session-a")
			}
			if stage == "after prepare" {
				err = db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
					buffer, err := admitted.BufferInTransaction(tx)
					if err != nil {
						return err
					}
					if err = buffer.Transact(ctx, save); err != nil {
						return err
					}
					if err = buffer.PrepareFlush(ctx); err != nil {
						return err
					}
					cancel()
					return buffer.Flush(ctx)
				})
			} else {
				err = admitted.Transact(ctx, save)
			}
			if err == nil || stage != "during fence" && fences != 0 {
				t.Fatalf("canceled stage %s reached persistence: calls=%d err=%v", stage, fences, err)
			}
			for _, key := range []*record.Key{fence, record.NewKeyWithParentAndID(repo.parent, Sessions, "session")} {
				if err := db.Get(context.Background(), record.NewRecordWithData(key, &map[string]any{})); !record.IsNotFound(err) {
					t.Fatalf("canceled transaction retained %s: %v", key, err)
				}
			}
		})
	}
}
