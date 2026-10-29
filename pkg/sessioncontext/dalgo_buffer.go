package sessioncontext

import (
	"context"
	"reflect"

	"github.com/dal-go/dalgo/dal"
)

// BufferedRepository belongs to the caller's DALGO transaction. Commands queue
// their writes; Flush issues them only after the caller finishes all reads.
// No method may be used after Flush. The caller commits or aborts the enclosing
// transaction and must return every command/Flush error to that transaction.
type BufferedRepository struct {
	unit             *dalTransaction
	tx               dal.ReadwriteTransaction
	databaseID       string
	closed           bool
	failed           error
	namespace        NativeNamespace
	admit            RetainedWriteAdmission
	prepared         bool
	persistAdmission func() error
}

func (r *DALRepository) BufferInTransaction(tx dal.ReadwriteTransaction) (*BufferedRepository, error) {
	if r == nil || r.db == nil || tx == nil || reflect.ValueOf(tx).Kind() == reflect.Pointer && reflect.ValueOf(tx).IsNil() {
		return nil, ErrInvalid
	}
	return &BufferedRepository{unit: &dalTransaction{reader: tx, parent: r.parent, adapter: r.adapter, loaded: map[string]storedRecord{}, pending: map[string]storedRecord{}}, tx: tx, databaseID: r.databaseID, namespace: r.nativeNamespace, admit: r.admitRetained}, nil
}
func (b *BufferedRepository) DatabaseID() string { return b.databaseID }
func (b *BufferedRepository) View(ctx context.Context, f func(Reader) error) error {
	if b == nil || b.closed || b.failed != nil || b.prepared {
		return ErrUnavailable
	}
	return f(transactionReader{b.unit})
}
func (b *BufferedRepository) Transact(ctx context.Context, f func(Transaction) error) error {
	if b == nil || b.closed || b.failed != nil || b.prepared {
		return ErrUnavailable
	}
	if err := f(b.unit); err != nil {
		b.failed = err
		return err
	}
	return nil
}

// PrepareFlush performs reference-admission reads before a caller's first native
// write. It seals the pending session writes; subsequent commands are refused.
// Flush later writes the admitted fences and records without further reads.
// A repository with admission installed cannot skip this step in a coupled tx.
func (b *BufferedRepository) PrepareFlush(ctx context.Context) error {
	if b == nil || b.closed || b.failed != nil || b.prepared {
		return ErrUnavailable
	}
	b.unit.sealed = true
	persist, err := prepareRetainedAdmission(ctx, b.tx, b.unit, b.namespace, b.admit)
	if err != nil {
		b.failed = err
		return err
	}
	b.persistAdmission, b.prepared = persist, true
	return nil
}
func (b *BufferedRepository) Flush(ctx context.Context) error {
	if b == nil || b.closed {
		return ErrUnavailable
	}
	b.closed = true
	b.unit.sealed = true
	if b.failed != nil {
		return b.failed
	}
	if b.admit != nil && !b.prepared {
		return ErrUnavailable
	}
	if ctx == nil {
		return ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if b.persistAdmission != nil {
		if err := b.persistAdmission(); err != nil {
			return err
		}
	}
	return flushSessionWrites(ctx, b.tx, b.unit)
}
