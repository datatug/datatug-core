package dalgostore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"sort"
	"strings"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/datatug/datatug-core/pkg/datatug"
)

const MaxProjectAssets = 100
const MaxProjectAssetBytes = 256 << 10
const MaxProjectAssetListBytes = 2 << 20

var ErrAssetLimit = errors.New("project asset limit exceeded")
var ErrAssetInvalid = errors.New("invalid project asset")
var ErrAssetReadOnly = errors.New("project asset session is read-only")
var projectScopeIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
var assetIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,99}$`)

// assetDocument is a driver-neutral codec, not a second model. Content is the
// canonical Core DTO; its key is authoritative and no revision is duplicated.
type assetDocument struct {
	Content []byte `json:"content" firestore:"content"`
}

// NewProjectItemsStore scopes the real ProjectStore to caller-owned metadata.
// The caller derives root from authenticated authority; this constructor is
// deliberately incapable of reading or changing the parent project document.
// A DALGO transaction is used directly, without starting a nested transaction.
func NewProjectItemsStore(session dal.ReadSession, projectID string, root *record.Key, options ...ProjectItemsOption) (*ProjectStore, error) {
	if session == nil || root == nil || !projectScopeIDPattern.MatchString(projectID) || root.Collection() != "projects" || root.ID != projectID {
		return nil, ErrAssetInvalid
	}
	copied, err := copyAssetKey(root)
	if err != nil {
		return nil, err
	}
	store := &ProjectStore{projectID: projectID, itemRoot: copied, itemSession: session}
	for _, option := range options {
		option(store)
	}
	return store, nil
}

func copyAssetKey(key *record.Key) (*record.Key, error) {
	path := []*record.Key{}
	seen := map[*record.Key]bool{}
	for at := key; at != nil; at = at.Parent() {
		id, ok := at.ID.(string)
		if !ok || id == "" || strings.TrimSpace(at.Collection()) == "" || seen[at] || len(path) >= 32 {
			return nil, ErrAssetInvalid
		}
		seen[at] = true
		path = append(path, at)
	}
	var parent *record.Key
	for i := len(path) - 1; i >= 0; i-- {
		parent = record.NewKeyWithParentAndID(parent, path[i].Collection(), path[i].ID.(string))
	}
	return parent, nil
}
func (s *ProjectStore) assetRoot() *record.Key {
	if s.itemRoot != nil {
		return s.itemRoot
	}
	return projectKey(s.projectID)
}
func (s *ProjectStore) assetSession() dal.ReadSession {
	if s.itemSession != nil {
		return s.itemSession
	}
	return s.db
}
func assetKey(root *record.Key, kind, id string) (*record.Key, error) {
	if !assetIDPattern.MatchString(id) {
		return nil, ErrAssetInvalid
	}
	return record.NewKeyWithParentAndID(root, kind, id), nil
}
func encodeAsset(value any) ([]byte, error) {
	var err error
	switch v := value.(type) {
	case *datatug.Environment:
		if v == nil {
			return nil, ErrAssetInvalid
		}
		err = v.Validate()
	case *datatug.Board:
		if v == nil {
			return nil, ErrAssetInvalid
		}
		err = v.Validate()
	case *datatug.Entity:
		if v == nil {
			return nil, ErrAssetInvalid
		}
		err = v.Validate()
	case *datatug.Card:
		if v == nil {
			return nil, ErrAssetInvalid
		}
		err = v.Validate()
	default:
		return nil, ErrAssetInvalid
	}
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAssetInvalid, err)
	}
	body, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrAssetInvalid, err)
	}
	if len(body) > MaxProjectAssetBytes {
		return nil, ErrAssetLimit
	}
	return body, nil
}
func decodeAsset(body []byte, value any, id string) error {
	if len(body) == 0 || len(body) > MaxProjectAssetBytes {
		return ErrAssetLimit
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(value); err != nil {
		return fmt.Errorf("%w: %v", ErrAssetInvalid, err)
	}
	if decoder.Decode(new(any)) != io.EOF {
		return ErrAssetInvalid
	}
	var actual string
	switch v := value.(type) {
	case *datatug.Environment:
		actual = v.ID
	case *datatug.Board:
		actual = v.ID
	case *datatug.Entity:
		actual = v.ID
	case *datatug.Card:
		actual = v.ID
	}
	if actual != id {
		return ErrAssetInvalid
	}
	_, err := encodeAsset(value)
	return err
}
func (s *ProjectStore) loadAsset(ctx context.Context, kind, id string, value any) error {
	key, err := assetKey(s.assetRoot(), kind, id)
	if err != nil {
		return err
	}
	doc := new(assetDocument)
	if err = s.assetSession().Get(ctx, record.NewRecordWithData(key, doc)); err != nil {
		return err
	}
	return decodeAsset(doc.Content, value, id)
}
func (s *ProjectStore) saveAsset(ctx context.Context, kind, id string, value any) error {
	writer, ok := s.assetSession().(dal.ReadwriteSession)
	if !ok {
		return ErrAssetReadOnly
	}
	key, err := assetKey(s.assetRoot(), kind, id)
	if err != nil {
		return err
	}
	body, err := encodeAsset(value)
	if err != nil {
		return err
	}
	if s.itemSession == nil {
		return s.db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
			items, err := NewProjectItemsStore(tx, s.projectID, s.assetRoot())
			if err != nil {
				return err
			}
			return items.saveAsset(ctx, kind, id, value)
		})
	}
	// Capacity is checked before the only write, including callers that bypass
	// the backend facade. Query+condition reads remain in the caller transaction.
	var existing assetDocument
	r := record.NewRecordWithData(key, &existing)
	if err := writer.Get(ctx, r); err != nil && !record.IsNotFound(err) {
		return err
	}
	if !r.Exists() {
		items, err := s.listAssets(ctx, kind, func() any {
			if kind == "environments" {
				return new(datatug.Environment)
			}
			if kind == "boards" {
				return new(datatug.Board)
			}
			if kind == "cards" {
				return new(datatug.Card)
			}
			return new(datatug.Entity)
		})
		if err != nil {
			return err
		}
		if len(items) >= MaxProjectAssets {
			return ErrAssetLimit
		}
	}
	return writer.Set(ctx, record.NewRecordWithData(key, &assetDocument{Content: body}))
}
func (s *ProjectStore) deleteAsset(ctx context.Context, kind, id string) error {
	writer, ok := s.assetSession().(dal.ReadwriteSession)
	if !ok {
		return ErrAssetReadOnly
	}
	key, err := assetKey(s.assetRoot(), kind, id)
	if err != nil {
		return err
	}
	return writer.Delete(ctx, key)
}
func (s *ProjectStore) listAssets(ctx context.Context, kind string, newValue func() any) ([]any, error) {
	values, _, err := s.readAssets(ctx, kind, newValue)
	return values, err
}
func (s *ProjectStore) readAssets(ctx context.Context, kind string, newValue func() any) ([]any, int, error) {
	root := s.assetRoot()
	query := dal.From(dal.NewCollectionRef(kind, "", root)).NewQuery().Limit(MaxProjectAssets + 1).SelectIntoRecord(func() record.Record {
		return record.NewRecordWithData(record.NewIncompleteKey(kind, reflect.String, root), new(assetDocument))
	})
	records, err := dal.ExecuteQueryAndReadAllToRecords(ctx, query, s.assetSession())
	if err != nil {
		return nil, 0, err
	}
	if len(records) > MaxProjectAssets {
		return nil, 0, ErrAssetLimit
	}
	sort.Slice(records, func(i, j int) bool { return fmt.Sprint(records[i].Key().ID) < fmt.Sprint(records[j].Key().ID) })
	values := make([]any, 0, len(records))
	total := 0
	for _, r := range records {
		id, ok := r.Key().ID.(string)
		if !ok || !assetIDPattern.MatchString(id) {
			return nil, 0, ErrAssetInvalid
		}
		doc, ok := r.Data().(*assetDocument)
		if !ok {
			return nil, 0, ErrAssetInvalid
		}
		total += len(doc.Content)
		if total > MaxProjectAssetListBytes {
			return nil, 0, ErrAssetLimit
		}
		value := newValue()
		if err := decodeAsset(doc.Content, value, id); err != nil {
			return nil, 0, err
		}
		values = append(values, value)
	}
	return values, total, nil
}
func (s *ProjectStore) LoadBoard(ctx context.Context, id string, _ ...datatug.StoreOption) (*datatug.Board, error) {
	value := new(datatug.Board)
	if err := s.loadAsset(ctx, "boards", id, value); err != nil {
		return nil, err
	}
	return value, nil
}
func (s *ProjectStore) SaveBoard(ctx context.Context, value *datatug.Board) error {
	if value == nil {
		return ErrAssetInvalid
	}
	return s.saveAsset(ctx, "boards", value.ID, value)
}
func (s *ProjectStore) DeleteBoard(ctx context.Context, id string) error {
	return s.deleteAsset(ctx, "boards", id)
}
func (s *ProjectStore) LoadBoards(ctx context.Context, _ ...datatug.StoreOption) (datatug.Boards, error) {
	values, err := s.listAssets(ctx, "boards", func() any { return new(datatug.Board) })
	if err != nil {
		return nil, err
	}
	out := make(datatug.Boards, len(values))
	for i, v := range values {
		out[i] = v.(*datatug.Board)
	}
	return out, nil
}
func (s *ProjectStore) LoadCard(ctx context.Context, id string, _ ...datatug.StoreOption) (*datatug.Card, error) {
	value := new(datatug.Card)
	if err := s.loadAsset(ctx, "cards", id, value); err != nil {
		return nil, err
	}
	return value, nil
}
func (s *ProjectStore) SaveCard(ctx context.Context, value *datatug.Card) error {
	if value == nil {
		return ErrAssetInvalid
	}
	return s.saveAsset(ctx, "cards", value.ID, value)
}
func (s *ProjectStore) DeleteCard(ctx context.Context, id string) error {
	return s.deleteAsset(ctx, "cards", id)
}
func (s *ProjectStore) LoadCards(ctx context.Context, _ ...datatug.StoreOption) (datatug.Cards, error) {
	values, err := s.listAssets(ctx, "cards", func() any { return new(datatug.Card) })
	if err != nil {
		return nil, err
	}
	out := make(datatug.Cards, len(values))
	for i, v := range values {
		out[i] = v.(*datatug.Card)
	}
	return out, nil
}
func (s *ProjectStore) LoadEntity(ctx context.Context, id string, _ ...datatug.StoreOption) (*datatug.Entity, error) {
	value := new(datatug.Entity)
	if err := s.loadAsset(ctx, "entities", id, value); err != nil {
		return nil, err
	}
	return value, nil
}
func (s *ProjectStore) SaveEntity(ctx context.Context, value *datatug.Entity) error {
	if value == nil {
		return ErrAssetInvalid
	}
	return s.saveAsset(ctx, "entities", value.ID, value)
}
func (s *ProjectStore) DeleteEntity(ctx context.Context, id string) error {
	return s.deleteAsset(ctx, "entities", id)
}
func (s *ProjectStore) LoadEntities(ctx context.Context, _ ...datatug.StoreOption) (datatug.Entities, error) {
	values, err := s.listAssets(ctx, "entities", func() any { return new(datatug.Entity) })
	if err != nil {
		return nil, err
	}
	out := make(datatug.Entities, len(values))
	for i, v := range values {
		out[i] = v.(*datatug.Entity)
	}
	return out, nil
}

// ProjectItemsOption supplies metadata already authorized and read by the caller.
type ProjectItemsOption func(*ProjectStore)

// WithEnvironmentOrder binds the parent order read in the caller-owned session.
// It never grants the item store permission to access the parent document.
func WithEnvironmentOrder(order []string) ProjectItemsOption {
	copied := append([]string(nil), order...)
	return func(s *ProjectStore) { s.environmentOrder = append([]string(nil), copied...) }
}
