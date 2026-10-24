package sessioncontext

import (
	"context"
	"reflect"
	"sort"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

// BufferedRepository belongs to the caller's DALGO transaction. Commands queue
// their writes; Flush issues them only after the caller finishes all reads.
// No method may be used after Flush. The caller commits or aborts the enclosing
// transaction and must return every command/Flush error to that transaction.
type BufferedRepository struct {
	unit       *dalTransaction
	tx         dal.ReadwriteTransaction
	databaseID string
	closed     bool
	failed     error
}

func (r *DALRepository) BufferInTransaction(tx dal.ReadwriteTransaction) (*BufferedRepository, error) {
	if r == nil || r.db == nil || tx == nil || reflect.ValueOf(tx).Kind() == reflect.Pointer && reflect.ValueOf(tx).IsNil() {
		return nil, ErrInvalid
	}
	return &BufferedRepository{unit: &dalTransaction{reader: tx, parent: r.parent, adapter: r.adapter, loaded: map[string]storedRecord{}, pending: map[string]storedRecord{}}, tx: tx, databaseID: r.databaseID}, nil
}
func (b *BufferedRepository) DatabaseID() string { return b.databaseID }
func (b *BufferedRepository) View(ctx context.Context, f func(Reader) error) error {
	if b == nil || b.closed || b.failed != nil {
		return ErrUnavailable
	}
	return f(transactionReader{b.unit})
}
func (b *BufferedRepository) Transact(ctx context.Context, f func(Transaction) error) error {
	if b == nil || b.closed || b.failed != nil {
		return ErrUnavailable
	}
	if err := f(b.unit); err != nil {
		b.failed = err
		return err
	}
	return nil
}
func (b *BufferedRepository) Flush(ctx context.Context) error {
	if b == nil || b.closed {
		return ErrUnavailable
	}
	b.closed = true
	if b.failed != nil {
		return b.failed
	}
	ids := make([]string, 0, len(b.unit.pending))
	for id := range b.unit.pending {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		doc := b.unit.pending[id]
		data := map[string]any{"recordId": doc.ID, "version": doc.Version, "owner": doc.Owner, "sessionId": doc.SessionID, "payload": string(doc.Payload)}
		if err := b.tx.Set(ctx, record.NewRecordWithData(b.unit.key(doc.Collection, doc.ID), data)); err != nil {
			return storageError(err)
		}
	}
	return nil
}
