package sessioncontext

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
)

func TestRetainedAdmissionRejectsUnboundOrOversizeBatchesBeforeCallingProvider(t *testing.T) {
	repo, _ := retainedInventoryFixture(t)
	admit := func(context.Context, dal.ReadwriteTransaction, NativeNamespace, []RetainedWrite) (func() error, error) {
		t.Fatal("invalid batch reached authority")
		return nil, nil
	}
	var absent *DALRepository
	if _, err := absent.WithRetainedWriteAdmission(admit); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	if _, err := repo.WithRetainedWriteAdmission(nil); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	for _, mode := range []string{"nil context", "cancel", "count", "unloaded", "bytes"} {
		t.Run(mode, func(t *testing.T) {
			unit := &dalTransaction{pending: map[string]storedRecord{}, loaded: map[string]storedRecord{}}
			ctx := context.Background()
			var want = ErrUnavailable
			switch mode {
			case "nil context":
				ctx = nil
				want = ErrInvalid
			case "cancel":
				c, cancel := context.WithCancel(ctx)
				cancel()
				ctx = c
				want = context.Canceled
			case "count":
				for i := 0; i < 10001; i++ {
					unit.pending[fmt.Sprint(i)] = storedRecord{}
				}
			case "unloaded":
				unit.pending["row"] = storedRecord{}
			case "bytes":
				unit.pending["row"] = storedRecord{Payload: []byte(strings.Repeat("x", (16<<20)+1))}
				unit.loaded["row"] = storedRecord{}
			}
			persist, err := prepareRetainedAdmission(ctx, nil, unit, repo.NativeNamespace(), admit)
			if persist != nil || !errors.Is(err, want) {
				t.Fatal("invalid admission returned persistence closure", err)
			}
		})
	}
	if err := flushSessionWrites(nil, nil, &dalTransaction{}); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
	unit := &dalTransaction{pending: map[string]storedRecord{"row": {ID: "row"}}}
	ctx := checkpointContext(2)
	if err := flushSessionWrites(ctx, nil, unit); !errors.Is(err, context.Canceled) {
		t.Fatal("flush did not stop before first write", err)
	}
	ctx.cancel()
}

func TestBufferedAdmissionFailurePermanentlySealsBuffer(t *testing.T) {
	repo, db := retainedInventoryFixture(t)
	failure := errors.New("admission or fence failed")
	for _, mode := range []string{"prepare failure", "fence failure", "nil flush context"} {
		t.Run(mode, func(t *testing.T) {
			err := db.RunReadwriteTransaction(context.Background(), func(ctx context.Context, tx dal.ReadwriteTransaction) error {
				bound, err := repo.WithRetainedWriteAdmission(func(context.Context, dal.ReadwriteTransaction, NativeNamespace, []RetainedWrite) (func() error, error) {
					if mode == "prepare failure" {
						return nil, failure
					}
					return func() error { return failure }, nil
				})
				if err != nil {
					return err
				}
				buffer, err := bound.BufferInTransaction(tx)
				if err != nil {
					return err
				}
				if err = buffer.Transact(ctx, func(unit Transaction) error {
					return unit.Save(ctx, Sessions, "session", map[string]string{"id": "session"}, 0, "alice", "session")
				}); err != nil {
					return err
				}
				if mode == "nil flush context" {
					buffer.admit = nil
					if err = buffer.Flush(nil); !errors.Is(err, ErrInvalid) {
						t.Fatal(err)
					}
				} else {
					err = buffer.PrepareFlush(ctx)
					if mode == "prepare failure" {
						if !errors.Is(err, failure) {
							t.Fatal(err)
						}
						if err = buffer.PrepareFlush(ctx); !errors.Is(err, ErrUnavailable) {
							t.Fatal("failed prepare reopened", err)
						}
						if err = buffer.Flush(ctx); !errors.Is(err, failure) {
							t.Fatal("failure lost", err)
						}
					} else {
						if err != nil {
							return err
						}
						if err = buffer.Flush(ctx); !errors.Is(err, failure) {
							t.Fatal("failed fence persisted source", err)
						}
					}
				}
				if err = buffer.PrepareFlush(ctx); !errors.Is(err, ErrUnavailable) {
					t.Fatal("closed buffer reopened", err)
				}
				return failure
			})
			if !errors.Is(err, failure) {
				t.Fatal(err)
			}
		})
	}
	var nilBuffer *BufferedRepository
	if err := nilBuffer.PrepareFlush(context.Background()); !errors.Is(err, ErrUnavailable) {
		t.Fatal(err)
	}
}
