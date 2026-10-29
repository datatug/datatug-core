package sessioncontext

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

// RetainedDocument is a server-internal envelope. Payload may contain private
// inputs and must never be returned from a user-facing inventory endpoint.
// Source adapters decode the collection's actual type, validate its schema and
// resolve its references; this envelope alone is not a complete reference graph.
type RetainedDocument struct {
	Collection string
	ID         string
	Owner      string
	SessionID  string
	Version    int64
	Payload    json.RawMessage
}

type RetainedInventory struct {
	Namespace NativeNamespace
	// Collections includes empty collections too. Consumers must reject any
	// collection they cannot project; silently skipping a new source is unsafe.
	Collections []string
	Documents   []RetainedDocument
}

type RetainedInventoryLimits struct {
	MaxDocuments int
	MaxBytes     int
}

// RetainedInventoryInTransaction scans all owners and all retained rows under
// this repository's explicitly bound native project parent. It does not filter
// by current session state, expiration, redaction or execution outcome. Retained
// tombstones and expired-but-unpurged snapshots still carry references.
//
// The owning server must authorize this internal operation and prove that tx is
// on the SAME mounted database as this repository. Like ReaderInTransaction,
// this method cannot prove a transaction's database identity from DAL's interface.
// It starts no transaction and performs no writes. Call before the first write;
// the provider must share admission with every concurrent reference writer.
//
// Every known source collection is scanned with a budget+1 limit. Any overflow,
// malformed envelope, query/read/close failure discards the entire result. This
// is a storage inventory, not a completeness certificate for other authorities.
func (r *DALRepository) RetainedInventoryInTransaction(ctx context.Context, tx dal.ReadTransaction, limits RetainedInventoryLimits) (RetainedInventory, error) {
	if r == nil || r.db == nil || r.parent == nil || r.nativeNamespace.NamespaceID == "" || ctx == nil || tx == nil ||
		reflect.ValueOf(tx).Kind() == reflect.Pointer && reflect.ValueOf(tx).IsNil() ||
		limits.MaxDocuments < 1 || limits.MaxDocuments > 10000 || limits.MaxBytes < 1 || limits.MaxBytes > 16<<20 {
		return RetainedInventory{}, ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return RetainedInventory{}, err
	}
	out := RetainedInventory{Namespace: r.nativeNamespace, Collections: Collections(), Documents: []RetainedDocument{}}
	remaining := limits.MaxBytes
	for _, collection := range out.Collections {
		if err := ctx.Err(); err != nil {
			return RetainedInventory{}, err
		}
		documents, err := r.readRetainedCollection(ctx, tx, collection, limits.MaxDocuments-len(out.Documents), &remaining)
		if err != nil {
			return RetainedInventory{}, err
		}
		out.Documents = append(out.Documents, documents...)
	}
	if err := ctx.Err(); err != nil {
		return RetainedInventory{}, err
	}
	return out, nil
}

func (r *DALRepository) readRetainedCollection(ctx context.Context, tx dal.ReadTransaction, collection string, capacity int, remaining *int) (out []RetainedDocument, retErr error) {
	query := dal.From(dal.NewCollectionRef(collection, "", r.parent)).NewQuery().
		OrderBy(dal.Ascending(dal.DocumentID())).Limit(capacity + 1).
		SelectIntoRecord(func() record.Record {
			var data any = map[string]any{}
			if r.adapter == "memory" {
				data = &map[string]any{}
			}
			return record.NewRecordWithData(record.NewIncompleteKey(collection, reflect.String, r.parent), data)
		})
	reader, err := tx.ExecuteQueryToRecordsReader(ctx, query)
	if err != nil {
		return nil, storageError(err)
	}
	defer func() {
		if err := reader.Close(); retErr == nil && err != nil {
			out, retErr = nil, storageError(err)
		}
	}()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		row, err := reader.Next()
		if canceled := ctx.Err(); canceled != nil {
			return nil, canceled
		}
		if errors.Is(err, io.EOF) || errors.Is(err, dal.ErrNoMoreRecords) {
			return out, nil
		}
		if err != nil {
			return nil, storageError(err)
		}
		if len(out) >= capacity || row == nil || row.Key() == nil || row.Key().Validate() != nil || row.Key().Collection() != collection || row.Key().Parent() == nil || row.Key().Parent().String() != r.parent.String() {
			return nil, ErrUnavailable
		}
		data, ok := row.Data().(map[string]any)
		if pointer, yes := row.Data().(*map[string]any); yes && pointer != nil {
			data, ok = *pointer, true
		}
		if !ok {
			return nil, ErrUnavailable
		}
		id, idOK := row.Key().ID.(string)
		if !idOK {
			return nil, ErrUnavailable
		}
		payload, ok := data["payload"].(string)
		if !ok || len(payload) > *remaining {
			return nil, ErrUnavailable
		}
		doc, err := decodeRecord(collection, id, data)
		if err != nil || !safeID.MatchString(doc.ID) || !safeID.MatchString(doc.SessionID) || len(doc.Owner) > 256 || strings.TrimSpace(doc.Owner) != doc.Owner || strings.ContainsAny(doc.Owner, "\x00\r\n") {
			return nil, ErrUnavailable
		}
		cost := len(doc.Payload) + len(doc.ID) + len(doc.Owner) + len(doc.SessionID) + len(collection) + 64
		if cost > *remaining {
			return nil, ErrUnavailable
		}
		*remaining -= cost
		out = append(out, RetainedDocument{Collection: collection, ID: doc.ID, Owner: doc.Owner, SessionID: doc.SessionID, Version: doc.Version, Payload: append(json.RawMessage(nil), doc.Payload...)})
	}
}
