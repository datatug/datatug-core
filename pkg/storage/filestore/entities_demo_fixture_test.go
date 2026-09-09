package filestore

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// demoProjectsRepoPath is where this session's sibling datatug-demo-projects
// checkout lives. It is a fixture, not a dependency: TestFsEntitiesStore_
// DemoProject1Fixture skips (never fails) when it is absent, since most
// environments running this module's tests (a different machine, a hosted
// CI runner) won't have it cloned at all.
const demoProjectsRepoPath = "/home/ai/projects/datatug/datatug-demo-projects"

// TestFsEntitiesStore_DemoProject1Fixture proves this package's filestore
// loads datatug-demo-projects/demo-project-1 - the project whose per-entity-
// directory layout (see store_entities.go) motivated this fix - end to end:
// every entity loads, the mappings declared in datatug-demo-projects PR #9
// are present, the demo's board1 board and chinook DB model load with their
// content, and Project.Validate() passes.
//
// This test only ever reads the sibling checkout - it must never mutate it
// (a "git pull" here previously did; a test is not the place to fetch
// changes into a repository other tooling/sessions may be using).
func TestFsEntitiesStore_DemoProject1Fixture(t *testing.T) {
	demoProjectDir := filepath.Join(demoProjectsRepoPath, "demo-project-1")
	if _, err := os.Stat(demoProjectDir); err != nil {
		t.Skipf("skipping: sibling checkout not found at %s: %v", demoProjectDir, err)
	}

	tmpDir, err := os.MkdirTemp("", "datatug_demo_project_1_fixture")
	require.NoError(t, err)
	t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })

	dst := filepath.Join(tmpDir, "demo-project-1")
	require.NoError(t, copyDir(demoProjectDir, dst))

	store := newFsProjectStore("demo-project-1", dst)
	ctx := context.Background()

	project, err := store.LoadProject(ctx)
	require.NoError(t, err)

	wantIDs := []string{"Album", "Artist", "Country", "Customer", "Invoice", "InvoiceLine", "Person", "Track"}
	assert.ElementsMatch(t, wantIDs, project.Entities.IDs())

	fieldByID := func(entityID, fieldID string) *datatug.EntityField {
		e := project.Entities.GetByID(entityID)
		require.NotNil(t, e, "entity %s not found", entityID)
		for _, f := range e.Fields {
			if f.ID == fieldID {
				return f
			}
		}
		t.Fatalf("field %s.%s not found", entityID, fieldID)
		return nil
	}

	assert.Contains(t, fieldByID("Customer", "ID").Mappings,
		datatug.PhysicalRef{Source: "chinook", Collection: "Customer", Column: "CustomerId"})
	assert.Contains(t, fieldByID("Customer", "ID").Mappings,
		datatug.PhysicalRef{Source: "support-notes", Collection: "support-notes", Column: "CustomerId"})
	assert.Contains(t, fieldByID("Customer", "Email").Mappings,
		datatug.PhysicalRef{Source: "chinook", Collection: "Customer", Column: "Email"})
	assert.Contains(t, fieldByID("Invoice", "ID").Mappings,
		datatug.PhysicalRef{Source: "chinook", Collection: "Invoice", Column: "InvoiceId"})
	assert.Contains(t, fieldByID("Country", "Name").Mappings,
		datatug.PhysicalRef{Source: "chinook", Collection: "Customer", Column: "Country"})

	if assert.Len(t, project.Boards, 1) {
		assert.Equal(t, "board1", project.Boards[0].ID)
		assert.Equal(t, "1st board", project.Boards[0].Title)
	}

	if assert.Len(t, project.DbModels, 1) {
		assert.Equal(t, "chinook", project.DbModels[0].ID)
	}

	assert.NoError(t, project.Validate())
}

// copyDir recursively copies src to dst, preserving the directory structure.
// Used only by tests to work on a disposable copy of a fixture checkout.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		if !info.Mode().IsRegular() {
			return nil // skip symlinks and other special files
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = in.Close() }()
		if err := os.MkdirAll(filepath.Dir(target), 0777); err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		defer func() { _ = out.Close() }()
		_, err = io.Copy(out, in)
		return err
	})
}
