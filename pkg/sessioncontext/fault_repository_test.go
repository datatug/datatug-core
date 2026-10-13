package sessioncontext

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type interceptedRepository struct {
	Repository
	viewErr      error
	transactErr  error
	failLoad     func(string, string) error
	failList     func(string, string, string, int) ([]json.RawMessage, string, error)
	failSave     func(string, string) error
	listOverride func(string, string, string, int) ([]json.RawMessage, string, error)
}

func (r *interceptedRepository) DatabaseID() string {
	if identity, ok := r.Repository.(interface{ DatabaseID() string }); ok {
		return identity.DatabaseID()
	}
	return ""
}

func (r *interceptedRepository) View(ctx context.Context, fn func(Reader) error) error {
	if r.viewErr != nil {
		return r.viewErr
	}
	return r.Repository.View(ctx, func(reader Reader) error {
		return fn(interceptedReader{Reader: reader, failLoad: r.failLoad, failList: r.failList, listOverride: r.listOverride})
	})
}

func (r *interceptedRepository) Transact(ctx context.Context, fn func(Transaction) error) error {
	if r.transactErr != nil {
		return r.transactErr
	}
	return r.Repository.Transact(ctx, func(tx Transaction) error {
		return fn(interceptedTransaction{Transaction: tx, reader: interceptedReader{Reader: tx, failLoad: r.failLoad, failList: r.failList, listOverride: r.listOverride}, failSave: r.failSave})
	})
}

type interceptedReader struct {
	Reader
	failLoad     func(string, string) error
	failList     func(string, string, string, int) ([]json.RawMessage, string, error)
	listOverride func(string, string, string, int) ([]json.RawMessage, string, error)
}

func (r interceptedReader) Load(ctx context.Context, collection, id string, out any) (int64, error) {
	if r.failLoad != nil {
		if err := r.failLoad(collection, id); err != nil {
			return 0, err
		}
	}
	return r.Reader.Load(ctx, collection, id, out)
}

func (r interceptedReader) List(ctx context.Context, collection, owner, cursor string, limit int) ([]json.RawMessage, string, error) {
	if r.listOverride != nil {
		return r.listOverride(collection, owner, cursor, limit)
	}
	if r.failList != nil {
		return r.failList(collection, owner, cursor, limit)
	}
	return r.Reader.List(ctx, collection, owner, cursor, limit)
}

type interceptedTransaction struct {
	Transaction
	reader   interceptedReader
	failSave func(string, string) error
}

func (tx interceptedTransaction) Load(ctx context.Context, collection, id string, out any) (int64, error) {
	return tx.reader.Load(ctx, collection, id, out)
}

func (tx interceptedTransaction) List(ctx context.Context, collection, owner, cursor string, limit int) ([]json.RawMessage, string, error) {
	return tx.reader.List(ctx, collection, owner, cursor, limit)
}

func (tx interceptedTransaction) Save(ctx context.Context, collection, id string, value any, expected int64, owner, sessionID string) error {
	if tx.failSave != nil {
		if err := tx.failSave(collection, id); err != nil {
			return err
		}
	}
	return tx.Transaction.Save(ctx, collection, id, value, expected, owner, sessionID)
}

func interceptService(t *testing.T, base *Service, p Principal, repo *interceptedRepository) *Service {
	t.Helper()
	if repo == nil {
		repo = &interceptedRepository{Repository: base.repo}
	}
	svc, err := NewService(repo, base.authorize)
	if err != nil {
		t.Fatal(err)
	}
	svc.now = base.now
	return svc
}

func TestServiceConstructionAuthorizationAndOperationReads(t *testing.T) {
	if _, err := NewService(nil, func(context.Context, Access) error { return nil }); !errors.Is(err, ErrInvalid) {
		t.Fatalf("nil repository accepted: %v", err)
	}
	base, p := newTestService(t, nil)
	if _, err := NewService(base.repo, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("nil authorizer accepted: %v", err)
	}
	svc := interceptService(t, base, p, nil)
	for name, principal := range map[string]Principal{
		"missing actor":     {DatabaseID: p.DatabaseID, ProjectID: p.ProjectID},
		"unsafe database":   {Actor: p.Actor, DatabaseID: "bad database", ProjectID: p.ProjectID},
		"missing project":   {Actor: p.Actor, DatabaseID: p.DatabaseID},
		"database mismatch": {Actor: p.Actor, DatabaseID: "other-db", ProjectID: p.ProjectID},
	} {
		t.Run(name, func(t *testing.T) {
			if err := svc.allow(context.Background(), principal, "read", "s1", nil); !errors.Is(err, ErrForbidden) {
				t.Fatalf("allow error = %v", err)
			}
		})
	}
	denied := interceptService(t, base, p, &interceptedRepository{Repository: base.repo})
	denied.authorize = func(context.Context, Access) error { return errors.New("revoked") }
	if err := denied.allow(context.Background(), p, "read", "s1", nil); !errors.Is(err, ErrForbidden) {
		t.Fatalf("authorization denial was not normalized: %v", err)
	}
	createTestSession(t, svc, p, "s1", "b1")
	if _, err := svc.ReadOperation(context.Background(), p, "bad id"); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unsafe operation id = %v", err)
	}
	if _, err := svc.ReadOperation(context.Background(), p, "missing-op"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing operation = %v", err)
	}
	if _, err := svc.ReadOperation(context.Background(), p, "create-s1"); err != nil {
		t.Fatalf("authorized operation read = %v", err)
	}
	if _, err := svc.ReadOperation(context.Background(), Principal{Actor: "bob", DatabaseID: p.DatabaseID, ProjectID: p.ProjectID}, "create-s1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("another owner saw operation receipt: %v", err)
	}
}

func TestSessionAndBagReadersRejectCorruptOwnershipAndVersions(t *testing.T) {
	base, p := newTestService(t, nil)
	ctx := context.Background()
	validSession := Session{SchemaVersion: 1, ID: "s1", DatabaseID: p.DatabaseID, ProjectID: p.ProjectID, Owner: p.Actor, Version: 1, BagIDs: []string{"b1"}}
	validBag := Bag{ID: "b1", SessionID: "s1", Scope: Scope{EnvironmentID: "uat", IdentityDomain: "tenant-a"}, Version: 1}
	for name, test := range map[string]struct {
		value   Session
		version int64
		want    error
	}{
		"wrong session id":         {Session{SchemaVersion: 1, ID: "other", DatabaseID: p.DatabaseID, ProjectID: p.ProjectID, Owner: p.Actor, Version: 1}, 1, ErrForbidden},
		"wrong owner":              {Session{SchemaVersion: 1, ID: "s1", DatabaseID: p.DatabaseID, ProjectID: p.ProjectID, Owner: "bob", Version: 1}, 1, ErrForbidden},
		"wrong database":           {Session{SchemaVersion: 1, ID: "s1", DatabaseID: "other", ProjectID: p.ProjectID, Owner: p.Actor, Version: 1}, 1, ErrForbidden},
		"wrong project":            {Session{SchemaVersion: 1, ID: "s1", DatabaseID: p.DatabaseID, ProjectID: "other", Owner: p.Actor, Version: 1}, 1, ErrForbidden},
		"session version mismatch": {Session{SchemaVersion: 1, ID: "s1", DatabaseID: p.DatabaseID, ProjectID: p.ProjectID, Owner: p.Actor, Version: 2}, 1, ErrUnavailable},
		"session schema mismatch":  {Session{SchemaVersion: 2, ID: "s1", DatabaseID: p.DatabaseID, ProjectID: p.ProjectID, Owner: p.Actor, Version: 1}, 1, ErrUnavailable},
	} {
		t.Run(name, func(t *testing.T) {
			r := singleRecordReader{records: map[string]any{Sessions + "/s1": test.value}, versions: map[string]int64{Sessions + "/s1": test.version}}
			if _, err := base.session(ctx, r, p, "s1", "read"); !errors.Is(err, test.want) {
				t.Fatalf("session error = %v, want %v", err, test.want)
			}
		})
	}
	bagTests := []struct {
		name    string
		value   Bag
		session Session
		version int64
		want    error
	}{
		{name: "wrong bag session", value: Bag{ID: "b1", SessionID: "other", Scope: validBag.Scope, Version: 1}, session: validSession, version: 1, want: ErrForbidden},
		{name: "bag not attached", value: Bag{ID: "b1", SessionID: "s1", Scope: validBag.Scope, Version: 1}, session: Session{ID: "s1", BagIDs: []string{}}, version: 1, want: ErrForbidden},
		{name: "bag version mismatch", value: Bag{ID: "b1", SessionID: "s1", Scope: validBag.Scope, Version: 2}, session: validSession, version: 1, want: ErrUnavailable},
	}
	for _, test := range bagTests {
		t.Run(test.name, func(t *testing.T) {
			r := singleRecordReader{records: map[string]any{Bags + "/b1": test.value}, versions: map[string]int64{Bags + "/b1": test.version}}
			if _, err := base.bag(ctx, r, p, test.session, "b1", "read"); !errors.Is(err, test.want) {
				t.Fatalf("bag error = %v, want %v", err, test.want)
			}
		})
	}
	missing := singleRecordReader{loadErrors: map[string]error{Sessions + "/s1": ErrNotFound}}
	if _, err := base.session(ctx, missing, p, "s1", "read"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("session load failure = %v", err)
	}
	if _, err := base.bag(ctx, singleRecordReader{loadErrors: map[string]error{Bags + "/b1": ErrUnavailable}}, p, validSession, "b1", "read"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("bag load failure = %v", err)
	}
}

type singleRecordReader struct {
	records    map[string]any
	versions   map[string]int64
	loadErrors map[string]error
}

func (r singleRecordReader) Load(_ context.Context, collection, id string, out any) (int64, error) {
	key := collection + "/" + id
	if err := r.loadErrors[key]; err != nil {
		return 0, err
	}
	value, ok := r.records[key]
	if !ok {
		return 0, ErrNotFound
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return 0, ErrUnavailable
	}
	if err = json.Unmarshal(encoded, out); err != nil {
		return 0, ErrUnavailable
	}
	return r.versions[key], nil
}

func (singleRecordReader) List(context.Context, string, string, string, int) ([]json.RawMessage, string, error) {
	return nil, "", nil
}
