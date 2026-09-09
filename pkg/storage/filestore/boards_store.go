package filestore

import (
	"context"
	"fmt"
	"os"
	"path"
	"reflect"
	"sort"
	"strings"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
)

var _ datatug.BoardsStore = (*fsBoardsStore)(nil)

func newFsBoardsStore(projectPath string) fsBoardsStore {
	return fsBoardsStore{
		fsProjectItemsStore: newFileProjectItemsStore[datatug.Boards, *datatug.Board, datatug.Board](
			path.Join(projectPath, storage.BoardsFolder), storage.BoardFileSuffix,
		),
	}
}

// fsBoardsStore loads and saves boards from two on-disk layouts, the same
// dual-layout rules store_entities.go applies to entities, but with a
// different nested filename: flat "<boardsDir>/<id>.board.json" and nested
// "<boardsDir>/<id>/<suffix>.json" - a FIXED filename per directory, not
// id-prefixed (e.g. datatug-demo-projects/demo-project-1's
// boards/board1/board.json - the board's identity comes from the directory
// name alone). Nested wins when both exist and agree; disagreement is a
// load error. A save keeps the layout a board was loaded from; a brand-new
// board defaults to nested.
type fsBoardsStore struct {
	fsProjectItemsStore[datatug.Boards, *datatug.Board, datatug.Board]
}

func (s fsBoardsStore) flatBoardFilePath(id string) string {
	return path.Join(s.dirPath, storage.JsonFileName(id, s.itemFileSuffix))
}

// nestedBoardFileName is the fixed filename used inside every board's
// directory, e.g. "board.json" - never id-prefixed.
func (s fsBoardsStore) nestedBoardFileName() string {
	return s.itemFileSuffix + ".json"
}

func (s fsBoardsStore) nestedBoardFilePath(id string) string {
	return path.Join(s.dirPath, id, s.nestedBoardFileName())
}

func readBoardFile(filePath string) (*datatug.Board, error) {
	board := new(datatug.Board)
	if err := readJSONFile(filePath, true, board); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return board, nil
}

// loadOneBoard reads id from both layouts and reconciles them: nested wins
// when both exist and their content matches; a content mismatch is a clear
// error. Returns (nil, nil) when the board exists in neither layout.
func (s fsBoardsStore) loadOneBoard(id string) (*datatug.Board, error) {
	nestedPath, flatPath := s.nestedBoardFilePath(id), s.flatBoardFilePath(id)
	nested, err := readBoardFile(nestedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load board[%s] from %s: %w", id, nestedPath, err)
	}
	flat, err := readBoardFile(flatPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load board[%s] from %s: %w", id, flatPath, err)
	}
	switch {
	case nested != nil && flat != nil:
		if !reflect.DeepEqual(nested, flat) {
			return nil, fmt.Errorf(
				"board[%s] is stored in both %s and %s with different content; remove one",
				id, nestedPath, flatPath)
		}
		nested.SetID(id)
		return nested, nil
	case nested != nil:
		nested.SetID(id)
		return nested, nil
	case flat != nil:
		flat.SetID(id)
		return flat, nil
	default:
		return nil, nil
	}
}

func (s fsBoardsStore) LoadBoard(_ context.Context, id string, o ...datatug.StoreOption) (*datatug.Board, error) {
	_ = datatug.GetStoreOptions(o...)
	board, err := s.loadOneBoard(id)
	if err != nil {
		return nil, err
	}
	if board == nil {
		return nil, fmt.Errorf("failed to load board[%s] from project: %w", id, os.ErrNotExist)
	}
	return board, nil
}

// listBoardIDs returns every board id found in either layout: flat
// "<boardsDir>/<id>.board.json" files and nested "<boardsDir>/<id>/<suffix>.json" directories.
func (s fsBoardsStore) listBoardIDs() ([]string, error) {
	dirEntries, err := os.ReadDir(s.dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	suffix := "." + s.itemFileSuffix + ".json"
	nestedFileName := s.nestedBoardFileName()
	seen := make(map[string]struct{}, len(dirEntries))
	var ids []string
	add := func(id string) {
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	for _, de := range dirEntries {
		name := de.Name()
		if de.IsDir() {
			if _, statErr := os.Stat(path.Join(s.dirPath, name, nestedFileName)); statErr == nil {
				add(name)
			}
			continue
		}
		if id, ok := strings.CutSuffix(name, suffix); ok {
			add(id)
		}
	}
	return ids, nil
}

func (s fsBoardsStore) LoadBoards(_ context.Context, o ...datatug.StoreOption) (datatug.Boards, error) {
	_ = datatug.GetStoreOptions(o...)
	ids, err := s.listBoardIDs()
	if err != nil {
		return nil, err
	}
	boards := make(datatug.Boards, 0, len(ids))
	for _, id := range ids {
		board, err := s.loadOneBoard(id)
		if err != nil {
			return nil, err
		}
		if board == nil {
			continue // named by the directory scan but vanished/unreadable in a benign race
		}
		boards = append(boards, board)
	}
	sort.Slice(boards, func(i, j int) bool {
		return boards[i].GetID() < boards[j].GetID()
	})
	return boards, nil
}

// saveTarget mirrors fsEntitiesStore.saveTarget, but the nested file has a
// fixed name (nestedBoardFileName), not an id-prefixed one.
func (s fsBoardsStore) saveTarget(id string) (dirPath, fileName string) {
	if _, err := os.Stat(s.nestedBoardFilePath(id)); err == nil {
		return path.Join(s.dirPath, id), s.nestedBoardFileName()
	}
	if _, err := os.Stat(s.flatBoardFilePath(id)); err == nil {
		return s.dirPath, storage.JsonFileName(id, s.itemFileSuffix)
	}
	return path.Join(s.dirPath, id), s.nestedBoardFileName()
}

func (s fsBoardsStore) SaveBoard(_ context.Context, board *datatug.Board) error {
	dirPath, fileName := s.saveTarget(board.ID)
	if err := saveJSONFile(dirPath, fileName, board); err != nil {
		return fmt.Errorf("failed to save board file: %w", err)
	}
	return nil
}

func (s fsBoardsStore) saveBoards(ctx context.Context, boards datatug.Boards) error {
	return saveItems(s.dirPath, len(boards), func(i int) func() error {
		return func() error {
			return s.SaveBoard(ctx, boards[i])
		}
	})
}

func (s fsBoardsStore) DeleteBoard(_ context.Context, id string) error {
	for _, filePath := range []string{s.nestedBoardFilePath(id), s.flatBoardFilePath(id)} {
		if _, err := os.Stat(filePath); err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if err := os.Remove(filePath); err != nil {
			return err
		}
	}
	return nil
}
