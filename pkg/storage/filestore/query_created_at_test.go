package filestore

import (
	"context"
	"encoding/json"
	"github.com/datatug/datatug-core/pkg/datatug"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestQueryCreatedAtServerOwnedPersistence(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		t.Run(map[bool]string{true: "legacy", false: "revisioned"}[legacy], func(t *testing.T) {
			s, dir := newTestQueriesStore(t)
			ctx := context.Background()
			q := dtqlQuery("q", "", "SELECT 1")
			q.CreatedAt = "client"
			before := time.Now().UTC()
			if legacy {
				if err := s.SaveQuery(ctx, &q); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := s.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfNoneMatch: true}); err != nil {
					t.Fatal(err)
				}
			}
			created, err := time.Parse(time.RFC3339Nano, q.CreatedAt)
			if err != nil || created.Before(before) || created.After(time.Now().UTC()) {
				t.Fatalf("invalid server creation %q", q.CreatedAt)
			}
			fresh := newFsQueriesStore(filepath.Dir(dir))
			loaded, err := fresh.LoadQueryRevision(ctx, "q")
			if err != nil {
				t.Fatal(err)
			}
			if loaded.Query.CreatedAt != q.CreatedAt {
				t.Fatal("creation lost after reload")
			}
			list, err := fresh.LoadQueries(ctx, "")
			if err != nil || len(list.Items) != 1 || list.Items[0].CreatedAt != q.CreatedAt {
				t.Fatalf("list missing creation: %+v %v", list, err)
			}
			q.CreatedAt = "tampered"
			q.Text = "SELECT 2"
			updated, err := fresh.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfMatch: loaded.Revision})
			if err != nil {
				t.Fatal(err)
			}
			if updated.Query.CreatedAt != loaded.Query.CreatedAt {
				t.Fatal("update changed creation")
			}

		})
	}
}

func TestQueryCreationTimeServerIntentProducesSameRevision(t *testing.T) {
	at := time.Date(2026, 10, 21, 12, 0, 0, 123, time.UTC)
	ctx := datatug.WithQueryCreationTime(context.Background(), at)
	var revision datatug.QueryRevision
	for i := 0; i < 2; i++ {
		s, _ := newTestQueriesStore(t)
		q := dtqlQuery("q", "", "SELECT 1")
		saved, err := s.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfNoneMatch: true})
		if err != nil {
			t.Fatal(err)
		}
		if q.CreatedAt != at.Format(time.RFC3339Nano) {
			t.Fatal("server operation time ignored")
		}
		if i == 1 && saved.Revision != revision {
			t.Fatal("retry preview revision changed")
		}
		revision = saved.Revision
	}
}

func TestQueryUndatedUpdateFailsClosed(t *testing.T) {
	s, dir := newTestQueriesStore(t)
	ctx := context.Background()
	q := dtqlQuery("q", "", "SELECT 1")
	if _, err := s.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfNoneMatch: true}); err != nil {
		t.Fatal(err)
	}
	metadata := filepath.Join(dir, "q.query.json")
	raw, err := os.ReadFile(metadata)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err = json.Unmarshal(raw, &fields); err != nil {
		t.Fatal(err)
	}
	delete(fields, "createdAt")
	raw, _ = json.Marshal(fields)
	if err = os.WriteFile(metadata, raw, 0600); err != nil {
		t.Fatal(err)
	}
	existing, err := readCurrentQueryPair(dir, "q")
	if err != nil {
		t.Fatal(err)
	}
	q.CreatedAt = "client"
	_, err = s.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfMatch: existing.revision})
	if err == nil {
		t.Fatal("update accepted undated persisted query")
	}

}

func TestQueryPersistedReadAndListRejectMissingCreation(t *testing.T) {
	s, dir := newTestQueriesStore(t)
	ctx := context.Background()
	q := dtqlQuery("q", "", "SELECT 1")
	if err := s.SaveQuery(ctx, &q); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "q.query.json")
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]any
	if err = json.Unmarshal(raw, &value); err != nil {
		t.Fatal(err)
	}
	delete(value, "createdAt")
	raw, _ = json.Marshal(value)
	if err = os.WriteFile(file, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = s.LoadQuery(ctx, "q"); err == nil {
		t.Fatal("read accepted missing mandatory metadata")
	}
	if _, err = s.LoadQueryRevision(ctx, "q"); err == nil {
		t.Fatal("revision read accepted missing mandatory metadata")
	}
	if _, err = s.LoadQueries(ctx, ""); err == nil {
		t.Fatal("list accepted missing mandatory metadata")
	}
}
