package filestore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// Review N-g: a sub-folder whose ID is "", "." or ".." used to save its
// items into the parent or grandparent, because path.Join cleaned the
// segment away. It is now refused before anything is written.
func TestSaveQueriesTree_RefusesASubFolderIDThatIsNotOneSegment(t *testing.T) {
	for _, id := range []string{"", ".", ".."} {
		t.Run(id, func(t *testing.T) {
			store, queriesDir := newTestQueriesStore(t)
			item := dtqlQuery("q", "", "BODY").QueryDef
			sub := &datatug.QueriesFolder{Items: datatug.QueryDefs{&item}}
			sub.ID = id
			tree := &datatug.QueriesFolder{Folders: datatug.QueryFolders{sub}}
			tree.ID = "root"
			err := store.saveQueriesTree(context.Background(), "a", tree)
			if err == nil || !strings.Contains(err.Error(), "sub-folder") {
				t.Fatalf("expected sub-folder %q to be refused, got: %v", id, err)
			}
			for _, dir := range []string{queriesDir, filepath.Join(queriesDir, "a")} {
				if _, statErr := os.Lstat(filepath.Join(dir, "q.query.json")); !os.IsNotExist(statErr) {
					t.Errorf("expected nothing saved into %s, got Lstat err=%v", dir, statErr)
				}
			}
		})
	}
}

// Review N-f: LoadQuery and LoadQueries refuse the reserved transaction
// namespace at any depth, so the tree listing skips it at any depth too,
// instead of listing queries no other API can reach.
func TestLoadQueriesTree_SkipsTheReservedNamespaceAtAnyDepth(t *testing.T) {
	store, queriesDir := newTestQueriesStore(t)
	ctx := context.Background()
	q := dtqlQuery("q", "sub", "BODY")
	if _, err := store.PutQuery(ctx, &q, datatug.QueryWriteCondition{IfNoneMatch: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	nested := filepath.Join(queriesDir, "sub", reservedQueryTxnDirName)
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for name, content := range map[string]string{"hidden.query.json": `{"title":"h","type":"DTQL"}`, "hidden.query.dtql": "H"} {
		if err := os.WriteFile(filepath.Join(nested, name), []byte(content), 0o644); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	tree, err := store.loadQueriesTree(ctx, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tree.Folders) != 1 || tree.Folders[0].ID != "sub" {
		t.Fatalf("expected one sub-folder, got %+v", tree.Folders)
	}
	if sub := tree.Folders[0]; len(sub.Folders) != 0 || len(sub.Items) != 1 {
		t.Errorf("expected sub to hold only q and no %s folder, got items %d, folders %+v", reservedQueryTxnDirName, len(sub.Items), sub.Folders)
	}
}
