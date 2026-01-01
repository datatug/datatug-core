package filestore

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
	"github.com/stretchr/testify/assert"
)

func TestFsBoardsStore(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "datatug_boards_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer func() {
		_ = os.RemoveAll(tmpDir)
	}()

	boardsDir := filepath.Join(tmpDir, storage.BoardsFolder)
	err = os.MkdirAll(boardsDir, 0777)
	assert.NoError(t, err)

	board1 := &datatug.Board{
		ProjBoardBrief: datatug.ProjBoardBrief{
			ProjItemBrief: datatug.ProjItemBrief{
				ID:    "b1",
				Title: "Board 1",
			},
		},
	}
	board2 := &datatug.Board{
		ProjBoardBrief: datatug.ProjBoardBrief{
			ProjItemBrief: datatug.ProjItemBrief{
				ID:    "b2",
				Title: "Board 2",
			},
		},
	}

	saveBoard := func(b *datatug.Board) {
		data, _ := json.Marshal(b)
		fileName := fmt.Sprintf("%v.%v.json", b.ID, storage.BoardFileSuffix)
		err = os.WriteFile(filepath.Join(boardsDir, fileName), data, 0644)
		assert.NoError(t, err)
	}

	saveBoard(board1)
	saveBoard(board2)

	store := newFsProjectStore("p1", tmpDir)
	ctx := context.Background()

	t.Run("LoadBoards", func(t *testing.T) {
		boards, err := store.LoadBoards(ctx)
		assert.NoError(t, err)
		assert.Len(t, boards, 2)
		ids := []string{boards[0].ID, boards[1].ID}
		assert.Contains(t, ids, "b1")
		assert.Contains(t, ids, "b2")
	})

	t.Run("LoadBoard", func(t *testing.T) {
		b, err := store.LoadBoard(ctx, "b1")
		assert.NoError(t, err)
		assert.Equal(t, "b1", b.ID)
		assert.Equal(t, "Board 1", b.Title)
	})

	t.Run("SaveBoard", func(t *testing.T) {
		board3 := &datatug.Board{
			ProjBoardBrief: datatug.ProjBoardBrief{
				ProjItemBrief: datatug.ProjItemBrief{
					ID:    "b3",
					Title: "Board 3",
				},
			},
		}
		err := store.SaveBoard(ctx, board3)
		assert.NoError(t, err)

		b, err := store.LoadBoard(ctx, "b3")
		assert.NoError(t, err)
		assert.Equal(t, "b3", b.ID)
	})

	t.Run("DeleteBoard", func(t *testing.T) {
		err := store.DeleteBoard(ctx, "b1")
		assert.NoError(t, err)

		_, err = store.LoadBoard(ctx, "b1")
		assert.Error(t, err)
	})

	t.Run("saveBoards", func(t *testing.T) {
		boards := datatug.Boards{
			{
				ProjBoardBrief: datatug.ProjBoardBrief{
					ProjItemBrief: datatug.ProjItemBrief{
						ID:    "b4",
						Title: "Board 4",
					},
				},
			},
		}
		err := store.saveBoards(ctx, boards)
		assert.NoError(t, err)

		b, err := store.LoadBoard(ctx, "b4")
		assert.NoError(t, err)
		assert.Equal(t, "b4", b.ID)
	})
}
