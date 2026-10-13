package sessioncontext

import (
	"context"
	"errors"
	"testing"

	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

type closeErrorReadSession struct {
	dal.ReadSession
	closeErr error
	nextErr  error
}

func (s closeErrorReadSession) ExecuteQueryToRecordsReader(ctx context.Context, query dal.Query) (dal.RecordsReader, error) {
	reader, err := s.ReadSession.ExecuteQueryToRecordsReader(ctx, query)
	if err != nil {
		return nil, err
	}
	return closeErrorRecordsReader{RecordsReader: reader, closeErr: s.closeErr, nextErr: s.nextErr}, nil
}

type closeErrorRecordsReader struct {
	dal.RecordsReader
	closeErr error
	nextErr  error
}

func (r closeErrorRecordsReader) Close() error {
	return errors.Join(r.RecordsReader.Close(), r.closeErr)
}

func (r closeErrorRecordsReader) Next() (record.Record, error) {
	if r.nextErr != nil {
		return nil, r.nextErr
	}
	return r.RecordsReader.Next()
}

func TestListReturnsReaderCloseFailureAndPreservesReadFailure(t *testing.T) {
	db := dalgo2memory.New(dalgo2memory.FirestoreProfile())
	t.Run("close failure after successful scan", func(t *testing.T) {
		wantCloseErr := errors.New("reader close failed")
		err := db.RunReadonlyTransaction(context.Background(), func(ctx context.Context, tx dal.ReadTransaction) error {
			unit := &dalTransaction{reader: closeErrorReadSession{ReadSession: tx, closeErr: wantCloseErr}}
			_, _, listErr := unit.List(ctx, Sessions, "alice", "", 10)
			if !errors.Is(listErr, ErrUnavailable) {
				t.Fatalf("List error = %v, want unavailable", listErr)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})

	t.Run("read failure takes precedence", func(t *testing.T) {
		readErr := context.Canceled
		closeErr := errors.New("secondary close failure")
		err := db.RunReadonlyTransaction(context.Background(), func(ctx context.Context, tx dal.ReadTransaction) error {
			unit := &dalTransaction{reader: closeErrorReadSession{ReadSession: tx, closeErr: closeErr, nextErr: readErr}}
			_, _, listErr := unit.List(ctx, Sessions, "alice", "", 10)
			if !errors.Is(listErr, context.Canceled) {
				t.Fatalf("List error = %v, want canceled read error", listErr)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	})
}
