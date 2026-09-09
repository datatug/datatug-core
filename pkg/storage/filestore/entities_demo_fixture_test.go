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
// loads datatug-demo-projects/demo-project-1 - the project whose per-item-
// directory layouts (entities, boards, dbmodels, environments) motivated
// these fixes - end to end: every entity loads, the mappings declared in
// datatug-demo-projects PR #9 are present, the demo's board1 board and
// chinook DB model load with their content, every environment loads and
// "local" resolves its "chinook-local" database catalog by id.
//
// Project.Validate() is deliberately NOT asserted to pass here (see S35's
// PR body/report): datatug-demo-projects' environments/*/*.env.json files
// all declare `"driver":"sqlite3","host":"localhost"`, and ServerRef.Validate
// correctly rejects any non-empty Host for the file-based sqlite3 driver
// (Host must be empty; use Path for the file location) - a real, pre-existing
// data issue in a different repository, not something this stream can or
// should paper over by weakening validation. Before the environments dual-
// layout fix, this went unnoticed because LoadEnvironments silently found
// zero environments (the exact bug S35 fixes) - Project.Validate() passed
// only because there was nothing to validate. This test asserts the specific
// expected error instead, so a real regression here would still be caught.
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

	wantEnvIDs := []string{"dev", "local", "prod", "QA", "UAT"}
	assert.ElementsMatch(t, wantEnvIDs, project.Environments.IDs())

	local := project.Environments.GetByID("local")
	require.NotNil(t, local)
	if assert.Len(t, local.DbServers, 1) {
		assert.Equal(t, "sqlite3", local.DbServers[0].Driver)
		assert.Contains(t, local.DbServers[0].Catalogs, "chinook-local",
			"the local environment must resolve its chinook-local database catalog")
	}

	catalogs, err := newFsEnvCatalogsStore(dst).LoadEnvDbCatalogs(ctx, "local")
	require.NoError(t, err)
	if assert.Len(t, catalogs, 1) {
		assert.Equal(t, "chinook-local", catalogs[0].ID)
	}

	err = project.Validate()
	if assert.Error(t, err, "see this test's doc comment: datatug-demo-projects' env files declare a non-empty sqlite3 host, which ServerRef.Validate correctly rejects") {
		assert.ErrorContains(t, err, "cannot be used with sqlite3")
		assert.ErrorContains(t, err, "localhost")
	}
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
