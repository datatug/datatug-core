package dalgostore

import (
	"context"
	"errors"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/datatug/datatug-core/pkg/datatug"
	"reflect"
)

var ErrAssetConflict = errors.New("project asset already exists or is duplicated")
var ErrAssetTransactionRequired = errors.New("project asset batch requires a readwrite transaction or database")

// ProjectAssetBatch contains canonical definitions to create, never overwrite.
// Boards and entities have separate ID namespaces and per-kind capacity bounds.
type ProjectAssetBatch struct {
	Boards   datatug.Boards
	Entities datatug.Entities
}

type encodedAssetCreate struct {
	kind, id string
	body     []byte
}

// CreateAssets atomically creates a bounded batch in this store's existing scope.
// A DB-backed store owns the transaction. A caller-owned transaction is reused;
// the caller MUST propagate any error to its coordinator to roll back all writes.
// A bare readwrite session is refused because it cannot guarantee atomicity.
// All caller policy/receipt reads must precede this call. Inside the batch all
// validation, absence and capacity reads precede the first create-only Insert.
// Definitions are encoded before transaction callbacks and are not read again.
func (s *ProjectStore) CreateAssets(ctx context.Context, batch ProjectAssetBatch) error {
	if s == nil {
		return ErrAssetInvalid
	}
	session := s.assetSession()
	reflected := reflect.ValueOf(session)
	if !reflected.IsValid() || (reflected.Kind() == reflect.Pointer && reflected.IsNil()) {
		return ErrAssetReadOnly
	}
	tx, transactional := session.(dal.ReadwriteTransaction)
	db, database := session.(dal.DB)
	if !transactional && !database {
		if _, ok := session.(dal.ReadwriteSession); !ok {
			return ErrAssetReadOnly
		}
		return ErrAssetTransactionRequired
	}
	if len(batch.Boards)+len(batch.Entities) == 0 {
		return ErrAssetInvalid
	}
	if len(batch.Boards) > MaxProjectAssets || len(batch.Entities) > MaxProjectAssets {
		return ErrAssetLimit
	}
	creates := make([]encodedAssetCreate, 0, len(batch.Boards)+len(batch.Entities))
	seen := map[string]bool{}
	sizes := map[string]int{}
	add := func(kind, id string, value any) error {
		if _, err := assetKey(s.assetRoot(), kind, id); err != nil {
			return err
		}
		identity := kind + "/" + id
		if seen[identity] {
			return ErrAssetConflict
		}
		body, err := encodeAsset(value)
		if err != nil {
			return err
		}
		seen[identity] = true
		sizes[kind] += len(body)
		if sizes[kind] > MaxProjectAssetListBytes {
			return ErrAssetLimit
		}
		creates = append(creates, encodedAssetCreate{kind: kind, id: id, body: body})
		return nil
	}
	for _, value := range batch.Boards {
		if value == nil {
			return ErrAssetInvalid
		}
		if err := add("boards", value.ID, value); err != nil {
			return err
		}
	}
	for _, value := range batch.Entities {
		if value == nil {
			return ErrAssetInvalid
		}
		if err := add("entities", value.ID, value); err != nil {
			return err
		}
	}
	apply := func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		scoped, err := NewProjectItemsStore(tx, s.projectID, s.assetRoot())
		if err != nil {
			return err
		}
		for _, kind := range []string{"boards", "entities"} {
			if sizes[kind] == 0 {
				continue
			}
			factory := func() any {
				if kind == "boards" {
					return new(datatug.Board)
				}
				return new(datatug.Entity)
			}
			existing, total, err := scoped.readAssets(ctx, kind, factory)
			if err != nil {
				return err
			}
			count := 0
			for _, create := range creates {
				if create.kind == kind {
					count++
				}
			}
			if len(existing)+count > MaxProjectAssets || total+sizes[kind] > MaxProjectAssetListBytes {
				return ErrAssetLimit
			}
			for _, value := range existing {
				var id string
				if kind == "boards" {
					id = value.(*datatug.Board).ID
				} else {
					id = value.(*datatug.Entity).ID
				}
				if seen[kind+"/"+id] {
					return ErrAssetConflict
				}
			}
		}
		for _, create := range creates {
			// Keys were validated above and root was copied by NewProjectItemsStore.
			key := record.NewKeyWithParentAndID(scoped.assetRoot(), create.kind, create.id)
			if err := tx.Insert(ctx, record.NewRecordWithData(key, &assetDocument{Content: create.body})); err != nil {
				return err
			}
		}
		return nil
	}
	if transactional {
		return apply(ctx, tx)
	}
	return db.RunReadwriteTransaction(ctx, apply)
}
