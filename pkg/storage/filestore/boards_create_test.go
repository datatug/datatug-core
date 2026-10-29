package filestore

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/require"
)

func createBoardDefinition() *datatug.Board {
	return &datatug.Board{ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "new-board", Title: "New Board", Folder: "~"}}}
}

func TestGuardedBoardCreatorPersistsAndReloads(t *testing.T) {
	dir := t.TempDir()
	store := newFsProjectStore("project", dir)
	creator, ok := any(store).(datatug.GuardedBoardCreator)
	require.True(t, ok)
	board := createBoardDefinition()
	checks := 0
	require.NoError(t, creator.CreateBoardGuarded(context.Background(), board, func() error { checks++; return nil }))
	require.Equal(t, 2, checks)
	loaded, err := newFsProjectStore("project", dir).LoadBoard(context.Background(), board.ID)
	require.NoError(t, err)
	require.Equal(t, board, loaded)
	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "boards", entries[0].Name())
	board.Title = "Update explicitly"
	require.NoError(t, store.SaveBoard(context.Background(), board))
	loaded, err = store.LoadBoard(context.Background(), board.ID)
	require.NoError(t, err)
	require.Equal(t, board.Title, loaded.Title)
}

func TestGuardedBoardCreatorPreservesExistingLayoutsAndAliases(t *testing.T) {
	for _, kind := range []string{"nested", "flat", "corrupt", "directory", "symlink", "dangling", "hardlink", "parent symlink", "parent file"} {
		t.Run(kind, func(t *testing.T) {
			dir, outside := t.TempDir(), t.TempDir()
			store, board := newFsBoardsStore(dir), createBoardDefinition()
			nested, flat := store.nestedBoardFilePath(board.ID), store.flatBoardFilePath(board.ID)
			require.NoError(t, os.MkdirAll(filepath.Dir(nested), 0o700))
			original := []byte("original retained content")
			external := filepath.Join(outside, "external")
			require.NoError(t, os.WriteFile(external, original, 0o600))
			switch kind {
			case "nested":
				require.NoError(t, store.SaveBoard(context.Background(), board))
			case "flat":
				data, err := json.Marshal(board)
				require.NoError(t, err)
				require.NoError(t, os.WriteFile(flat, data, 0o600))
			case "corrupt":
				require.NoError(t, os.WriteFile(nested, original, 0o600))
			case "directory":
				require.NoError(t, os.Mkdir(nested, 0o700))
			case "symlink", "dangling":
				target := external
				if kind == "dangling" {
					target += "-missing"
				}
				require.NoError(t, os.Symlink(target, nested))
			case "hardlink":
				require.NoError(t, os.Link(external, nested))
			case "parent symlink", "parent file":
				require.NoError(t, os.Remove(filepath.Dir(nested)))
				if kind == "parent symlink" {
					require.NoError(t, os.Symlink(outside, filepath.Dir(nested)))
				} else {
					require.NoError(t, os.WriteFile(filepath.Dir(nested), original, 0o600))
				}
			}
			before := treeHash(t, dir)
			require.Error(t, store.CreateBoardGuarded(context.Background(), board, nil))
			require.Equal(t, before, treeHash(t, dir))
			after, err := os.ReadFile(external)
			require.NoError(t, err)
			require.Equal(t, original, after)
		})
	}
}

func TestGuardedBoardCreatorRejectsInvalidContentBeforeWriting(t *testing.T) {
	for _, kind := range []string{"nil", "title", "path", "dot", "slash", "windows", "parameter", "marshal"} {
		t.Run(kind, func(t *testing.T) {
			dir, board := t.TempDir(), createBoardDefinition()
			switch kind {
			case "nil":
				board = nil
			case "title":
				board.Title = ""
			case "path":
				board.ID = "../escape"
			case "dot":
				board.ID = "."
			case "slash":
				board.ID = "a/b"
			case "windows":
				board.ID = "CON"
			case "parameter":
				board.Parameters = datatug.Parameters{{ID: "p"}}
			case "marshal":
				board.Parameters = datatug.Parameters{{ID: "p", Type: "number", DefaultValue: math.NaN()}}
			}
			require.Error(t, newFsBoardsStore(dir).CreateBoardGuarded(context.Background(), board, nil))
			entries, err := os.ReadDir(dir)
			require.NoError(t, err)
			require.Empty(t, entries)
		})
	}
}

func TestGuardedBoardCreatorFinalAdmissionAndConcurrentSave(t *testing.T) {
	for _, kind := range []string{"revoked", "canceled", "concurrent valid save"} {
		t.Run(kind, func(t *testing.T) {
			dir, board := t.TempDir(), createBoardDefinition()
			store := newFsBoardsStore(dir)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			denied, checks := errors.New("revoked authority"), 0
			file := store.nestedBoardFilePath(board.ID)
			var winning []byte
			err := store.CreateBoardGuarded(ctx, board, func() error {
				checks++
				if checks == 1 {
					return nil
				}
				entries, err := os.ReadDir(filepath.Dir(file))
				require.NoError(t, err)
				require.Len(t, entries, 1)
				require.Contains(t, entries[0].Name(), ".datatug-create-")
				switch kind {
				case "revoked":
					return denied
				case "canceled":
					cancel()
				case "concurrent valid save":
					command := exec.Command(os.Args[0], "-test.run=^TestGuardedBoardCreatorConcurrentSaveProcess$")
					command.Env = append(os.Environ(), "DATATUG_BOARD_CREATE_CHILD_ROOT="+dir)
					output, err := command.CombinedOutput()
					require.NoError(t, err, "%s", output)
					winning, err = os.ReadFile(file)
					require.NoError(t, err)
				}
				return nil
			})
			require.Error(t, err)
			if kind == "revoked" {
				require.ErrorIs(t, err, denied)
			}
			if kind == "canceled" {
				require.ErrorIs(t, err, context.Canceled)
			}
			if winning != nil {
				after, err := os.ReadFile(file)
				require.NoError(t, err)
				require.Equal(t, winning, after, "create must preserve the concurrently saved valid Board")
			} else {
				entries, err := os.ReadDir(dir)
				require.NoError(t, err)
				require.Empty(t, entries)
			}
		})
	}
}

func TestGuardedBoardCreatorConcurrentSaveProcess(t *testing.T) {
	dir := os.Getenv("DATATUG_BOARD_CREATE_CHILD_ROOT")
	if dir == "" {
		return
	}
	board := createBoardDefinition()
	board.Title = "Existing concurrently saved Board"
	require.NoError(t, newFsBoardsStore(dir).SaveBoard(context.Background(), board))
}

func TestGuardedBoardCreatorCollisionRetainsCause(t *testing.T) {
	dir, board := t.TempDir(), createBoardDefinition()
	store := newFsBoardsStore(dir)
	require.NoError(t, store.SaveBoard(context.Background(), board))
	require.ErrorIs(t, store.CreateBoardGuarded(context.Background(), board, nil), fs.ErrExist)
}

func TestGuardedBoardCreatorRefusesParentReplacementAfterPreparation(t *testing.T) {
	for _, kind := range []string{"root", "boards", "board parent", "parent symlink"} {
		t.Run(kind, func(t *testing.T) {
			base := t.TempDir()
			dir, moved := filepath.Join(base, "project"), filepath.Join(base, "moved")
			require.NoError(t, os.Mkdir(dir, 0o700))
			store, board := newFsBoardsStore(dir), createBoardDefinition()
			file := store.nestedBoardFilePath(board.ID)
			victim := filepath.Dir(file)
			if kind == "root" {
				victim = dir
			}
			if kind == "boards" {
				victim = filepath.Join(dir, "boards")
			}
			rel, err := filepath.Rel(victim, file)
			require.NoError(t, err)
			checks := 0
			err = store.CreateBoardGuarded(context.Background(), board, func() error {
				checks++
				if checks == 1 {
					return nil
				}
				require.NoError(t, os.Rename(victim, moved))
				if kind == "parent symlink" {
					return os.Symlink(moved, victim)
				}
				return os.Mkdir(victim, 0o700)
			})
			require.Error(t, err)
			require.Equal(t, 2, checks, "owning caller still makes the final decision after staging")
			_, err = os.Lstat(victim)
			require.NoError(t, err, "replacement must survive cleanup")
			require.NoFileExists(t, filepath.Join(moved, rel), "no publication into the moved original parent")
			entries, err := os.ReadDir(filepath.Dir(filepath.Join(moved, rel)))
			require.NoError(t, err)
			require.Empty(t, entries, "owned stage is removed without removing replacement directories")
		})
	}
}
