package sessioncontext

import (
	"context"
	"sort"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

type RetainedWrite struct {
	Before *RetainedDocument
	After  RetainedDocument
}

// RetainedWriteAdmission belongs to the trusted project host. It projects typed
// before/after records and reads shared reference fences in the supplied SAME
// transaction. It returns a write-only closure for those fences. No source or
// fence write may happen during preparation. Both errors must abort the outer
// transaction. Admission is not source authorization; existing command gates
// still authorize owner/collaborator access and native accepted work.
type RetainedWriteAdmission func(context.Context, dal.ReadwriteTransaction, NativeNamespace, []RetainedWrite) (func() error, error)

// WithRetainedWriteAdmission creates an independently configured repository.
// It never mutates another service's repository or enables native pin commands.
// Only an explicitly bound native project repository may install this port.
func (r *DALRepository) WithRetainedWriteAdmission(admit RetainedWriteAdmission) (*DALRepository, error) {
	if r == nil || r.db == nil || r.parent == nil || r.nativeNamespace.NamespaceID == "" || admit == nil {
		return nil, ErrInvalid
	}
	copy := *r
	copy.admitRetained = admit
	return &copy, nil
}

func inventoryEnvelope(doc storedRecord) RetainedDocument {
	return RetainedDocument{Collection: doc.Collection, ID: doc.ID, Owner: doc.Owner, SessionID: doc.SessionID, Version: doc.Version, Payload: append([]byte(nil), doc.Payload...)}
}

func prepareRetainedAdmission(ctx context.Context, tx dal.ReadwriteTransaction, unit *dalTransaction, namespace NativeNamespace, admit RetainedWriteAdmission) (func() error, error) {
	if ctx == nil {
		return nil, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if admit == nil || len(unit.pending) == 0 {
		return nil, nil
	}
	if len(unit.pending) > 10000 {
		return nil, ErrUnavailable
	}
	ids := make([]string, 0, len(unit.pending))
	for id := range unit.pending {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	writes := make([]RetainedWrite, 0, len(ids))
	bytes := 0
	for _, id := range ids {
		after := unit.pending[id]
		before, loaded := unit.loaded[id]
		if !loaded {
			return nil, ErrUnavailable
		}
		bytes += len(after.Payload) + len(before.Payload)
		if bytes > 16<<20 {
			return nil, ErrUnavailable
		}
		change := RetainedWrite{After: inventoryEnvelope(after)}
		if before.Version > 0 {
			prior := inventoryEnvelope(before)
			change.Before = &prior
		}
		writes = append(writes, change)
	}
	persist, err := admit(ctx, tx, namespace, writes)
	if canceled := ctx.Err(); canceled != nil {
		return nil, canceled
	}
	if err == nil && persist == nil {
		return nil, ErrUnavailable
	}
	return persist, err
}

func flushSessionWrites(ctx context.Context, tx dal.ReadwriteTransaction, unit *dalTransaction) error {
	if ctx == nil {
		return ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	ids := make([]string, 0, len(unit.pending))
	for id := range unit.pending {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if err := ctx.Err(); err != nil {
			return err
		}
		doc := unit.pending[id]
		data := map[string]any{"recordId": doc.ID, "version": doc.Version, "owner": doc.Owner, "sessionId": doc.SessionID, "payload": string(doc.Payload)}
		if err := tx.Set(ctx, record.NewRecordWithData(unit.key(doc.Collection, doc.ID), data)); err != nil {
			return storageError(err)
		}
	}
	return ctx.Err()
}
