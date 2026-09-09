package filestore

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFsBoardsStore_DualLayout mirrors TestFsEntitiesStore_DualLayout, but
// for boards' own nested convention: a FIXED filename ("board.json") inside
// each board's directory, not id-prefixed - see
// datatug-demo-projects/demo-project-1/boards/board1/board.json.
func TestFsBoardsStore_DualLayout(t *testing.T) {
	newStore := func(t *testing.T) (fsBoardsStore, string) {
		t.Helper()
		tmpDir, err := os.MkdirTemp("", "datatug_boards_layout_test")
		require.NoError(t, err)
		t.Cleanup(func() { _ = os.RemoveAll(tmpDir) })
		boardsDir := filepath.Join(tmpDir, storage.BoardsFolder)
		require.NoError(t, os.MkdirAll(boardsDir, 0777))
		return newFsBoardsStore(tmpDir), boardsDir
	}

	writeFlat := func(t *testing.T, boardsDir string, b *datatug.Board) {
		t.Helper()
		data, err := json.Marshal(b)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(boardsDir, b.ID+".board.json"), data, 0644))
	}
	writeNested := func(t *testing.T, boardsDir string, b *datatug.Board) {
		t.Helper()
		dir := filepath.Join(boardsDir, b.ID)
		require.NoError(t, os.MkdirAll(dir, 0777))
		data, err := json.Marshal(b)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(filepath.Join(dir, "board.json"), data, 0644))
	}

	t.Run("loads_flat_only", func(t *testing.T) {
		store, boardsDir := newStore(t)
		writeFlat(t, boardsDir, &datatug.Board{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "b1"}}})

		b, err := store.LoadBoard(context.Background(), "b1")
		assert.NoError(t, err)
		assert.Equal(t, "b1", b.ID)
	})

	t.Run("loads_nested_only", func(t *testing.T) {
		store, boardsDir := newStore(t)
		writeNested(t, boardsDir, &datatug.Board{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "b1", Title: "1st board"}}})

		b, err := store.LoadBoard(context.Background(), "b1")
		assert.NoError(t, err)
		assert.Equal(t, "b1", b.ID)
		assert.Equal(t, "1st board", b.Title)
	})

	t.Run("nested_wins_when_both_agree", func(t *testing.T) {
		store, boardsDir := newStore(t)
		b := &datatug.Board{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "b1"}}}
		writeFlat(t, boardsDir, b)
		writeNested(t, boardsDir, b)

		got, err := store.LoadBoard(context.Background(), "b1")
		assert.NoError(t, err)
		assert.Equal(t, "b1", got.ID)
	})

	t.Run("conflicting_duplicate_is_a_clear_error", func(t *testing.T) {
		store, boardsDir := newStore(t)
		writeFlat(t, boardsDir, &datatug.Board{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "b1", Title: "Flat"}}})
		writeNested(t, boardsDir, &datatug.Board{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "b1", Title: "Nested"}}})

		_, err := store.LoadBoard(context.Background(), "b1")
		assert.Error(t, err)
	})

	t.Run("not_found_in_either_layout", func(t *testing.T) {
		store, _ := newStore(t)
		_, err := store.LoadBoard(context.Background(), "missing")
		assert.Error(t, err)
	})

	t.Run("loadBoards_merges_both_layouts_deduped_and_sorted", func(t *testing.T) {
		store, boardsDir := newStore(t)
		writeFlat(t, boardsDir, &datatug.Board{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "zzz"}}})
		writeNested(t, boardsDir, &datatug.Board{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "aaa"}}})

		boards, err := store.LoadBoards(context.Background())
		require.NoError(t, err)
		require.Len(t, boards, 2)
		assert.Equal(t, "aaa", boards[0].ID)
		assert.Equal(t, "zzz", boards[1].ID)
	})

	t.Run("save_preserves_flat_layout_it_was_loaded_from", func(t *testing.T) {
		store, boardsDir := newStore(t)
		writeFlat(t, boardsDir, &datatug.Board{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "b1"}}})

		b, err := store.LoadBoard(context.Background(), "b1")
		require.NoError(t, err)
		b.Title = "Updated"
		require.NoError(t, store.SaveBoard(context.Background(), b))

		assert.FileExists(t, filepath.Join(boardsDir, "b1.board.json"))
		assert.NoFileExists(t, filepath.Join(boardsDir, "b1", "board.json"))
	})

	t.Run("save_preserves_nested_layout_it_was_loaded_from", func(t *testing.T) {
		store, boardsDir := newStore(t)
		writeNested(t, boardsDir, &datatug.Board{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "b1"}}})

		b, err := store.LoadBoard(context.Background(), "b1")
		require.NoError(t, err)
		b.Title = "Updated"
		require.NoError(t, store.SaveBoard(context.Background(), b))

		assert.FileExists(t, filepath.Join(boardsDir, "b1", "board.json"))
		assert.NoFileExists(t, filepath.Join(boardsDir, "b1.board.json"))
	})

	t.Run("save_new_defaults_to_nested", func(t *testing.T) {
		store, boardsDir := newStore(t)
		require.NoError(t, store.SaveBoard(context.Background(), &datatug.Board{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "b1", Title: "B1"}}}))

		assert.FileExists(t, filepath.Join(boardsDir, "b1", "board.json"))
	})

	t.Run("delete_removes_from_whichever_layout_exists", func(t *testing.T) {
		store, boardsDir := newStore(t)
		writeFlat(t, boardsDir, &datatug.Board{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "flat1"}}})
		writeNested(t, boardsDir, &datatug.Board{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "nested1"}}})

		require.NoError(t, store.DeleteBoard(context.Background(), "flat1"))
		require.NoError(t, store.DeleteBoard(context.Background(), "nested1"))

		assert.NoFileExists(t, filepath.Join(boardsDir, "flat1.board.json"))
		assert.NoFileExists(t, filepath.Join(boardsDir, "nested1", "board.json"))
	})
}
