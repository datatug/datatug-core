package filestore

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"

	"github.com/datatug/datatug-core/internal/plainfs"
	"github.com/datatug/datatug-core/pkg/datatug"
)

var _ datatug.GuardedBoardCreator = (*fsBoardsStore)(nil)

// CreateBoardGuarded publishes a new nested Board without ever updating an
// existing flat or nested definition. SaveBoard remains the explicit upsert.
func (s fsBoardsStore) CreateBoardGuarded(ctx context.Context, board *datatug.Board, authorize func() error) error {
	if board == nil {
		return fmt.Errorf("missing Board")
	}
	if err := board.Validate(); err != nil {
		return err
	}
	if reason, ok := validateQuerySegmentReason(board.ID); !ok {
		return fmt.Errorf("invalid Board ID: %s", reason)
	}
	data, err := json.MarshalIndent(board, "", "\t")
	if err != nil {
		return err
	}
	check := func() error {
		// Lstat refuses even unreadable content, dangling aliases and directory
		// targets; no parsing failure can turn an existing object into absence.
		for _, file := range []string{s.flatBoardFilePath(board.ID), s.nestedBoardFilePath(board.ID)} {
			if _, err := os.Lstat(file); err == nil {
				return fs.ErrExist
			} else if !os.IsNotExist(err) {
				return err
			}
		}
		if authorize != nil {
			return authorize()
		}
		return nil
	}
	if err := check(); err != nil {
		return err
	}
	return plainfs.PublishFile(ctx, s.projectDir, s.nestedBoardFilePath(board.ID), append(data, '\n'), check)
}
