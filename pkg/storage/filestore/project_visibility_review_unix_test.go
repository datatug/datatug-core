//go:build unix

package filestore

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
)

func TestAstraArchiveReaderCompatibilityAndExactBounds(t *testing.T) {
	ref := datatug.ProjectObjectRef{Kind: "query", ID: "q"}
	for _, tc := range []struct {
		name, body    string
		valid, hidden bool
	}{
		{"empty-object", `{}`, true, false},
		{"missing-version", `{"archived":[{"kind":"query","id":"q"}]}`, true, true},
		{"null-version-existing-default", `{"version":null,"archived":[{"kind":"query","id":"q"}]}`, true, true},
		{"null-array-existing-empty", `{"version":1,"archived":null}`, true, false},
		{"unknown-field-existing-permitted", `{"version":1,"future":{"flag":true}}`, true, false},
		{"case-folded-schema", `{"VERSION":1,"Archived":[{"Kind":"query","Id":"q"}]}`, true, true},
		{"case-folded-duplicate-version", `{"VERSION":1,"version":1}`, false, false},
		{"nested-duplicate-identity", `{"version":1,"archived":[{"kind":"query","id":"foreign","ID":"q"}]}`, false, false},
		{"second-object", `{} {}`, false, false},
		{"trailing-null", `{"version":1} null`, false, false},
		{"json-float-version", `{"version":1.0}`, false, false},
		{"nul-tail", "{}\x00", false, false},
		{"null-top", "null", false, false},
		{"exact-byte-cap", "{}" + strings.Repeat(" ", (2<<20)-2), true, false},
		{"over-byte-cap", "{}" + strings.Repeat(" ", (2<<20)-1), false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeVisibilityFixture(t, dir, visibilityFile, tc.body)
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = root.Close() }()
			anchored, e1 := ReadProjectArchivedItemsFromRoot(root)
			legacy, e2 := ReadProjectArchivedItems(dir)
			if (e1 == nil) != tc.valid || (e2 == nil) != tc.valid || !reflect.DeepEqual(anchored, legacy) {
				t.Fatalf("root=%v/%v legacy=%v/%v wantValid=%v", anchored, e1, legacy, e2, tc.valid)
			}
			if tc.valid && (anchored == nil || anchored[ref] != tc.hidden) {
				t.Fatalf("wrong owned projection %v", anchored)
			}
			if !tc.valid && (anchored != nil || legacy != nil) {
				t.Fatal("partial output on error")
			}
			if _, err = root.Stat(visibilityFile); err != nil {
				t.Fatal("caller root closed", err)
			}
		})
	}
	for _, n := range []int{10000, 10001} {
		t.Run(fmt.Sprintf("reference-count-%d", n), func(t *testing.T) {
			refs := make([]string, n)
			for i := range refs {
				refs[i] = fmt.Sprintf(`{"kind":"query","id":"q%d"}`, i)
			}
			dir := t.TempDir()
			writeVisibilityFixture(t, dir, visibilityFile, `{"version":1,"archived":[`+strings.Join(refs, ",")+`]}`)
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = root.Close() }()
			rows, err := ReadProjectArchivedItemsFromRoot(root)
			if n == 10000 {
				if err != nil || len(rows) != n {
					t.Fatalf("exact ref cap %d %v", len(rows), err)
				}
			} else if err == nil || rows != nil {
				t.Fatal("over ref cap accepted", len(rows), err)
			}
		})
	}
}

func TestAstraArchiveReaderPostLstatFIFOAndHandleOwnership(t *testing.T) {
	t.Run("fifo-swap", func(t *testing.T) {
		dir := t.TempDir()
		writeVisibilityFixture(t, dir, visibilityFile, `{"version":1}`)
		root, err := os.OpenRoot(dir)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = root.Close() }()
		var file *os.File
		fault := visibilityRootFault{Root: root, open: func(name string, flags int, mode fs.FileMode) (*os.File, error) {
			if err := os.Remove(filepath.Join(dir, name)); err != nil {
				t.Fatal(err)
			}
			if err := syscall.Mkfifo(filepath.Join(dir, name), 0600); err != nil {
				t.Fatal(err)
			}
			file, err = root.OpenFile(name, flags, mode)
			return file, err
		}}
		state, err := readProjectVisibilityRoot(fault)
		rows, err := projectArchivedItems(state, err)
		if err == nil || rows != nil {
			t.Fatal("FIFO admitted", rows, err)
		}
		if file != nil {
			if _, err := file.Stat(); !errors.Is(err, os.ErrClosed) {
				t.Fatal("opened FIFO leaked", err)
			}
		}
		if _, err := root.Stat("."); err != nil {
			t.Fatal("caller root closed", err)
		}
	})
	for _, body := range []string{`{"version":1}`, `null`} {
		t.Run(body, func(t *testing.T) {
			dir := t.TempDir()
			writeVisibilityFixture(t, dir, visibilityFile, body)
			root, err := os.OpenRoot(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = root.Close() }()
			var file *os.File
			fault := visibilityRootFault{Root: root, open: func(name string, flags int, mode fs.FileMode) (*os.File, error) {
				file, err = root.OpenFile(name, flags, mode)
				return file, err
			}}
			_, err = readProjectVisibilityRoot(fault)
			if (err == nil) != (body != `null`) {
				t.Fatal("unexpected decoding result", err)
			}
			if file == nil {
				t.Fatal("did not exercise open")
			}
			if _, err := file.Stat(); !errors.Is(err, os.ErrClosed) {
				t.Fatal("archive fd remains open", err)
			}
			if _, err := root.Stat(visibilityFile); err != nil {
				t.Fatal("caller root closed", err)
			}
		})
	}
}

func TestVisibilityCanonicalServerArchivePreservesDirectSource(t *testing.T) {
	store, scope := visibilityFixture(t)
	const name = "dbs/mysql/mysql:archive-host:3306/.datatug-dbserver.json"
	const body = `{"id":"mysql:archive-host:3306","server":{"driver":"mysql","host":"archive-host","port":3306},"catalogs":[]}`
	ref := datatug.ProjectObjectRef{Kind: "server", ID: "mysql/mysql:archive-host:3306"}
	writeVisibilityFixture(t, store.projectPath, name, body)
	review, err := store.ReviewProjectLifecycle(t.Context(), scope, ref)
	if err != nil || !review.CanArchive {
		t.Fatalf("canonical server has no owner archive target: %+v %v", review, err)
	}
	if _, err = store.ChangeProjectLifecycle(t.Context(), scope, datatug.ProjectLifecycleWrite{OperationID: "archive-canonical-server", ExpectedRevision: review.Revision, Change: datatug.ProjectLifecycleChange{Object: ref, Action: "archive"}}); err != nil {
		t.Fatal(err)
	}
	active, err := store.ListProjectLifecycle(t.Context(), scope, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range active.Items {
		if item.Ref == ref {
			t.Fatal("archived canonical server remains in active inventory")
		}
	}
	root, err := os.OpenRoot(store.projectPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	archived, err := ReadProjectArchivedItemsFromRoot(root)
	if err != nil || !archived[ref] {
		t.Fatal("canonical classifier and rooted archive disagree", archived, err)
	}
	retained, err := root.ReadFile(name)
	if err != nil || string(retained) != body {
		t.Fatal("archiving changed or blocked the directly addressed source", err)
	}
	all, err := store.ListProjectLifecycle(t.Context(), scope, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.ChangeProjectLifecycle(t.Context(), scope, datatug.ProjectLifecycleWrite{OperationID: "restore-canonical-server", ExpectedRevision: all.Revision, Change: datatug.ProjectLifecycleChange{Object: ref, Action: "restore"}}); err != nil {
		t.Fatal(err)
	}
	archived, err = ReadProjectArchivedItemsFromRoot(root)
	if err != nil || archived[ref] {
		t.Fatal("canonical server restore did not clear its display overlay", archived, err)
	}
}
