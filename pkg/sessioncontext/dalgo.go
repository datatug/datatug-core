package sessioncontext

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"sort"
	"strings"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

// MountedDatabase is implemented by OVDB core.Database. The identity is captured
// with its mounted DALgo engine; clients cannot supply a raw storage connection.
type MountedDatabase interface {
	ID() string
	DB() dal.DB
}
type DALRepository struct {
	db         dal.DB
	adapter    string
	databaseID string
	parent     *record.Key
}

func NewDALRepository(mounted MountedDatabase, parent *record.Key) (*DALRepository, error) {
	if mounted == nil || !safeID.MatchString(mounted.ID()) || mounted.DB() == nil {
		return nil, ErrInvalid
	}
	return &DALRepository{db: mounted.DB(), adapter: mounted.DB().Adapter().Name(), databaseID: mounted.ID(), parent: parent}, nil
}
func (r *DALRepository) DatabaseID() string { return r.databaseID }
func (r *DALRepository) View(ctx context.Context, f func(Reader) error) error {
	return storageError(r.db.RunReadonlyTransaction(ctx, func(ctx context.Context, tx dal.ReadTransaction) error {
		return f(&dalTransaction{reader: tx, parent: r.parent, adapter: r.adapter, loaded: map[string]storedRecord{}, pending: map[string]storedRecord{}})
	}))
}

// Transact queues every write until the worker has completed all reads. Firestore
// transactions prohibit reads after writes, so this ordering is part of the contract.
func (r *DALRepository) Transact(ctx context.Context, f func(Transaction) error) error {
	return storageError(r.db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		unit := &dalTransaction{reader: tx, parent: r.parent, adapter: r.adapter, loaded: map[string]storedRecord{}, pending: map[string]storedRecord{}}
		if e := f(unit); e != nil {
			return e
		}
		ids := make([]string, 0, len(unit.pending))
		for id := range unit.pending {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		for _, id := range ids {
			doc := unit.pending[id]
			data := map[string]any{"recordId": doc.ID, "version": doc.Version, "owner": doc.Owner, "sessionId": doc.SessionID, "payload": string(doc.Payload)}
			if e := tx.Set(ctx, record.NewRecordWithData(unit.key(doc.Collection, doc.ID), data)); e != nil {
				return e
			}
		}
		return nil
	}, dal.TxWithMessage("session context command")))
}

type storedRecord struct {
	Collection string
	ID         string
	Version    int64
	Owner      string
	SessionID  string
	Payload    json.RawMessage
}
type dalTransaction struct {
	reader  dal.ReadSession
	parent  *record.Key
	adapter string
	loaded  map[string]storedRecord
	pending map[string]storedRecord
}

func (d *dalTransaction) key(collection, id string) *record.Key {
	return record.NewKeyWithParentAndID(d.parent, collection, id)
}
func (d *dalTransaction) Load(ctx context.Context, collection, id string, out any) (int64, error) {
	if !knownCollection(collection) || !safeID.MatchString(id) {
		return 0, ErrInvalid
	}
	key := collection + "/" + id
	doc, ok := d.pending[key]
	if !ok {
		doc, ok = d.loaded[key]
	}
	if !ok {
		data := map[string]any{}
		r := record.NewRecordWithData(d.key(collection, id), &data)
		if e := d.reader.Get(ctx, r); e != nil {
			if record.IsNotFound(e) {
				d.loaded[key] = storedRecord{}
				return 0, ErrNotFound
			}
			return 0, storageError(e)
		}
		if !r.Exists() {
			d.loaded[key] = storedRecord{}
			return 0, ErrNotFound
		}
		var e error
		doc, e = decodeRecord(collection, id, data)
		if e != nil {
			return 0, e
		}
		d.loaded[key] = doc
	}
	if doc.Version == 0 {
		return 0, ErrNotFound
	}
	if e := json.Unmarshal(doc.Payload, out); e != nil {
		return 0, ErrUnavailable
	}
	return doc.Version, nil
}
func (d *dalTransaction) Save(ctx context.Context, collection, id string, value any, expected int64, owner, sessionID string) error {
	if expected < 0 || owner == "" || !safeID.MatchString(sessionID) {
		return ErrInvalid
	}
	var previous json.RawMessage
	version, e := d.Load(ctx, collection, id, &previous)
	if e != nil && !errors.Is(e, ErrNotFound) {
		return e
	}
	if version != expected {
		return ErrConflict
	}
	payload, e := json.Marshal(value)
	if e != nil || len(payload) > 800000 {
		return ErrInvalid
	}
	d.pending[collection+"/"+id] = storedRecord{Collection: collection, ID: id, Version: expected + 1, Owner: owner, SessionID: sessionID, Payload: payload}
	return nil
}
func (d *dalTransaction) List(ctx context.Context, collection, owner, cursor string, limit int) ([]json.RawMessage, string, error) {
	if !knownCollection(collection) || owner == "" || limit < 1 || limit > 100 || cursor != "" && !safeID.MatchString(cursor) {
		return nil, "", ErrInvalid
	}
	builder := dal.NewQueryBuilder(dal.From(dal.NewCollectionRef(collection, "", d.parent))).WhereField("owner", dal.Equal, owner).OrderBy(dal.AscendingField("recordId")).Limit(limit + 1)
	if cursor != "" {
		builder = builder.WhereField("recordId", dal.GreaterThen, cursor)
	}
	reader, e := d.reader.ExecuteQueryToRecordsReader(ctx, builder.SelectIntoRecord(func() record.Record {
		data := any(map[string]any{})
		if d.adapter == "memory" {
			data = &map[string]any{}
		}
		return record.NewRecordWithIncompleteKey(collection, reflect.String, data)
	}))
	if e != nil {
		return nil, "", storageError(e)
	}
	defer reader.Close()
	out := []json.RawMessage{}
	last := ""
	for {
		r, e := reader.Next()
		if errors.Is(e, io.EOF) || errors.Is(e, dal.ErrNoMoreRecords) {
			break
		}
		if e != nil {
			return nil, "", storageError(e)
		}
		data, ok := r.Data().(map[string]any)
		if pointer, isPointer := r.Data().(*map[string]any); isPointer && pointer != nil {
			data, ok = *pointer, true
		}
		if !ok || data == nil {
			return nil, "", ErrUnavailable
		}
		doc, e := decodeRecord(collection, fmt.Sprint(r.Key().ID), data)
		if e != nil {
			return nil, "", e
		}
		if doc.Owner != owner {
			return nil, "", ErrForbidden
		}
		if len(out) == limit {
			return out, last, nil
		}
		out = append(out, append(json.RawMessage(nil), doc.Payload...))
		last = doc.ID
	}
	return out, "", nil
}
func decodeRecord(collection, id string, data any) (storedRecord, error) {
	m, ok := data.(map[string]any)
	if !ok {
		return storedRecord{}, ErrUnavailable
	}
	payload, ok := m["payload"].(string)
	if !ok || !json.Valid([]byte(payload)) {
		return storedRecord{}, ErrUnavailable
	}
	owner, ok := m["owner"].(string)
	if !ok || owner == "" {
		return storedRecord{}, ErrUnavailable
	}
	recordID, ok := m["recordId"].(string)
	if !ok || recordID != id {
		return storedRecord{}, ErrUnavailable
	}
	sessionID, ok := m["sessionId"].(string)
	if !ok {
		return storedRecord{}, ErrUnavailable
	}
	number, e := json.Marshal(m["version"])
	if e != nil {
		return storedRecord{}, ErrUnavailable
	}
	var version int64
	if e = json.Unmarshal(number, &version); e != nil || version < 1 {
		return storedRecord{}, ErrUnavailable
	}
	return storedRecord{Collection: collection, ID: id, Version: version, Owner: owner, SessionID: sessionID, Payload: json.RawMessage(payload)}, nil
}
func storageError(e error) error {
	if e == nil {
		return nil
	}
	for _, known := range []error{ErrConflict, ErrForbidden, ErrReadOnly, ErrInvalid, ErrNotFound, ErrUnavailable, context.Canceled, context.DeadlineExceeded} {
		if errors.Is(e, known) {
			return e
		}
	}
	if strings.Contains(e.Error(), "SQLITE_BUSY") {
		return ErrConflict
	}
	return ErrUnavailable
}
func Collections() []string {
	return []string{Sessions, Bags, Entries, Revisions, ContextSets, Operations, Audit, Inputs, Runs, Messages, Outbox}
}
func knownCollection(name string) bool {
	for _, v := range Collections() {
		if v == name {
			return true
		}
	}
	return false
}

// SQLiteSchemaStatements provision the private local OVDB mount before opening it.
// These identifiers are compile-time constants; no user SQL is interpolated.
func SQLiteSchemaStatements() []string {
	out := []string{}
	for _, collection := range Collections() {
		out = append(out, fmt.Sprintf(`CREATE TABLE IF NOT EXISTS "%s" (id TEXT PRIMARY KEY, recordId TEXT NOT NULL, version INTEGER NOT NULL, owner TEXT NOT NULL, sessionId TEXT NOT NULL, payload TEXT NOT NULL)`, collection), fmt.Sprintf(`CREATE INDEX IF NOT EXISTS "%s_owner_recordId" ON "%s" (owner, recordId)`, collection, collection))
	}
	return out
}

// ManifestCollectionsYAML is the collections block beneath schemas in an OVDB manifest.
func ManifestCollectionsYAML() string {
	var b strings.Builder
	b.WriteString("  collections:\n")
	for _, collection := range Collections() {
		fmt.Fprintf(&b, "    %s:\n      fields:\n        recordId: {type: string}\n        version: {type: integer}\n        owner: {type: string}\n        sessionId: {type: string}\n        payload: {type: string}\n", collection)
	}
	return b.String()
}
