package sessioncontext

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/datatug/datatug-core/pkg/apicontract"
)

func copyFailureFixture(t *testing.T, refresh bool) (*Service, *Service, Principal, CopyContextCommand) {
	t.Helper()
	private, p := newTestService(t, nil)
	source := workTestService(t, private.repo, "source", p.Actor, nil)
	target := workTestService(t, private.repo, "target", p.Actor, nil)
	createTestSession(t, source, p, "source", "source-bag")
	createTestSession(t, target, p, "target", "target-bag")
	refs := []EntryRef{}
	for n := 1; n <= 2; n++ {
		entry, revision := fmt.Sprintf("entry-%d", n), fmt.Sprintf("revision-%d", n)
		executeOK(t, source, p, Command{OpID: entry, Action: "putEntry", SessionID: "source", ExpectedVersion: int64(n), BagID: "source-bag", ExpectedBagVersion: int64(n), EntryID: entry, RevisionID: revision, Alias: fmt.Sprintf("Order%d", n), Subject: &Subject{ConceptID: "Order", FieldID: fmt.Sprintf("id%d", n), FieldMappingRef: "mapping"}, Value: &ValueEnvelope{SchemaVersion: 1, Kind: "scalar", Scalar: valuePtr(apicontract.NewIntegerValue("11045"))}})
		refs = append(refs, EntryRef{entry, revision})
	}
	c := CopyContextCommand{OpID: "copy", SourceSessionID: "source", DestinationSessionID: "target", ExpectedSourceVersion: 3, ExpectedDestinationVersion: 1, Selection: refs}
	if refresh {
		if _, e := CopyContext(context.Background(), p, source, target, c); e != nil {
			t.Fatal(e)
		}
		c.OpID, c.ExpectedDestinationVersion = "refresh", 2
	}
	return source, target, p, c
}

func TestWorkCopyEveryStorageFailureRollsBack(t *testing.T) {
	for _, refresh := range []bool{false, true} {
		for _, write := range []bool{false, true} {
			t.Run(fmt.Sprintf("refresh=%v/write=%v", refresh, write), func(t *testing.T) {
				completed := false
				for failAt := 1; failAt < 80; failAt++ {
					source, target, p, c := copyFailureFixture(t, refresh)
					ctx := context.Background()
					raw := source.repo
					before, e := target.GetSnapshot(ctx, p, "target")
					if e != nil {
						t.Fatal(e)
					}
					count := 0
					fail := func(string, string) error {
						count++
						if count == failAt {
							return ErrUnavailable
						}
						return nil
					}
					repo := &interceptedRepository{Repository: raw}
					if write {
						repo.failSave = fail
					} else {
						repo.failLoad = fail
					}
					source.repo, target.repo = repo, repo
					_, e = CopyContext(ctx, p, source, target, c)
					if e == nil {
						completed = true
						break
					}
					if !errors.Is(e, ErrUnavailable) {
						t.Fatalf("failure %d: %v", failAt, e)
					}
					target.repo = raw
					after, e := target.GetSnapshot(ctx, p, "target")
					if e != nil {
						t.Fatal(e)
					}
					if !reflect.DeepEqual(before, after) {
						t.Fatalf("failure %d committed a partial copy", failAt)
					}
				}
				if !completed {
					t.Fatal("copy did not complete after exhausting failure positions")
				}
			})
		}
	}
}

func TestWorkCopyRejectsReusedOperationStaleSelectionAndScopedRevocation(t *testing.T) {
	ctx := context.Background()
	source, target, p, c := copyFailureFixture(t, true)
	c.OpID = "copy" // completed receipt bound to destination version 1
	if _, e := CopyContext(ctx, p, source, target, c); !errors.Is(e, ErrConflict) {
		t.Fatalf("receipt intent changed: %v", e)
	}
	c.OpID = "stale-entry"
	c.Selection[0].RevisionID = "old-revision"
	if _, e := CopyContext(ctx, p, source, target, c); !errors.Is(e, ErrConflict) {
		t.Fatalf("stale selection: %v", e)
	}
	source, target, p, c = copyFailureFixture(t, false)
	scopes := 0
	target.authorize = func(_ context.Context, a Access) error {
		if a.Scope != nil {
			scopes++
			if scopes == 2 {
				return ErrForbidden
			}
		}
		return nil
	}
	if _, e := CopyContext(ctx, p, source, target, c); !errors.Is(e, ErrForbidden) {
		t.Fatalf("new destination scope was not authorized: %v", e)
	}
}

func TestBufferedContextFailureAndLifetimeBoundaries(t *testing.T) {
	ctx := context.Background()
	s, p := newTestService(t, nil)
	repo := s.repo.(*DALRepository)
	var absent *BufferedRepository
	if e := absent.Transact(ctx, func(Transaction) error { return nil }); !errors.Is(e, ErrUnavailable) {
		t.Fatal(e)
	}
	if e := absent.Flush(ctx); !errors.Is(e, ErrUnavailable) {
		t.Fatal(e)
	}
	binding := WorkBinding{WorkItemID: "work", SessionID: "session", IndexOwner: p.Actor, DatabaseID: p.DatabaseID, ProjectID: p.ProjectID}
	if _, e := NewWorkService(nil, binding, func(context.Context, Access) error { return nil }); !errors.Is(e, ErrInvalid) {
		t.Fatal(e)
	}
	e := repo.db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		buffer, e := repo.BufferInTransaction(failingSetTransaction{ReadwriteTransaction: tx, setErr: errors.New("storage write lost")})
		if e != nil {
			return e
		}
		work := workTestService(t, buffer, "flush", p.Actor, nil)
		createTestSession(t, work, p, "flush", "flush-bag")
		if e = buffer.Flush(ctx); !errors.Is(e, ErrUnavailable) {
			t.Fatalf("flush error: %v", e)
		}
		if e = buffer.Transact(ctx, func(Transaction) error { return nil }); !errors.Is(e, ErrUnavailable) {
			t.Fatal(e)
		}
		if e = buffer.Flush(ctx); !errors.Is(e, ErrUnavailable) {
			t.Fatal(e)
		}
		return ErrUnavailable
	})
	if !errors.Is(e, ErrUnavailable) {
		t.Fatal(e)
	}
	if _, e = workTestService(t, repo, "flush", p.Actor, nil).GetSnapshot(ctx, p, "flush"); !errors.Is(e, ErrNotFound) {
		t.Fatalf("failed flush persisted: %v", e)
	}
}

func TestWorkCopyRejectsClosedCapacityOrConflictingBindings(t *testing.T) {
	for _, mode := range []string{"closed-source", "closed-target", "full-target", "orphan-bag", "foreign-entry", "alias-collision"} {
		t.Run(mode, func(t *testing.T) {
			ctx := context.Background()
			source, target, p, c := copyFailureFixture(t, false)
			bagID := copyID("cb_", "source", "source-bag", "target")
			entryID := copyID("ce_", "source", "entry-1", "target")
			want := ErrConflict
			err := target.repo.Transact(ctx, func(tx Transaction) error {
				save := func(collection, id string, value any, version int64) error {
					return tx.Save(ctx, collection, id, value, version, p.Actor, "target")
				}
				switch mode {
				case "closed-source", "closed-target":
					id := "target"
					if mode == "closed-source" {
						id = "source"
					}
					var session Session
					version, err := tx.Load(ctx, Sessions, id, &session)
					if err != nil {
						return err
					}
					session.State = "closed"
					session.Version++
					want = ErrReadOnly
					return save(Sessions, id, session, version)
				case "full-target":
					var session Session
					version, err := tx.Load(ctx, Sessions, "target", &session)
					if err != nil {
						return err
					}
					for n := 1; n < 64; n++ {
						id := fmt.Sprintf("empty-%d", n)
						if err := save(Bags, id, Bag{ID: id, SessionID: "target", Title: id, Scope: Scope{"uat", "tenant-a"}, Version: 1}, 0); err != nil {
							return err
						}
						session.BagIDs = append(session.BagIDs, id)
					}
					want = ErrInvalid
					session.Version++
					c.ExpectedDestinationVersion = session.Version
					return save(Sessions, "target", session, version)
				case "orphan-bag":
					return save(Bags, bagID, Bag{ID: bagID, SessionID: "other", Scope: Scope{"uat", "tenant-a"}, Version: 1}, 0)
				case "foreign-entry":
					return save(Entries, entryID, Entry{ID: entryID, SessionID: "other", BagID: "other-bag", Version: 1}, 0)
				case "alias-collision":
					var entry Entry
					version, err := tx.Load(ctx, Entries, "entry-2", &entry)
					if err != nil {
						return err
					}
					entry.Alias = "order1"
					return save(Entries, entry.ID, entry, version)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			before, err := target.GetSnapshot(ctx, p, "target")
			if err != nil {
				t.Fatal(err)
			}
			if _, err = CopyContext(ctx, p, source, target, c); !errors.Is(err, want) {
				t.Fatalf("copy = %v, want %v", err, want)
			}
			after, err := target.GetSnapshot(ctx, p, "target")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(before, after) {
				t.Fatal("rejected copy changed destination")
			}
		})
	}
}
