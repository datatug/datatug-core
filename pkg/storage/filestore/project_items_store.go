package filestore

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"
	"sync"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
)

type IItem any

type IItemPtr[T IItem] interface {
	~*T
	GetID() string
	SetID(id string)
	Validate() error
}

type ProjItemStoredAs int

const (
	ProjItemStoredAsFile ProjItemStoredAs = iota
	ProjItemStoredAsDir
)

type fsProjectItemsStore[TSlice ~[]TItemPtr, TItemPtr IItemPtr[TItem], TItem IItem] struct {
	storedAs        ProjItemStoredAs
	dirPath         string
	itemFileSuffix  string
	summaryFileName string
}

func newFileProjectItemsStore[TSlice ~[]TItemPtr, TItemPtr IItemPtr[TItem], TItem IItem](
	dirPath, itemFileSuffix string,
) fsProjectItemsStore[TSlice, TItemPtr, TItem] {
	return fsProjectItemsStore[TSlice, TItemPtr, TItem]{
		storedAs:       ProjItemStoredAsFile,
		dirPath:        dirPath,
		itemFileSuffix: itemFileSuffix,
	}
}

func newDirProjectItemsStore[TSlice ~[]TItemPtr, TItemPtr IItemPtr[TItem], TItem IItem](
	dirPath, summaryFileName string,
) fsProjectItemsStore[TSlice, TItemPtr, TItem] {
	return fsProjectItemsStore[TSlice, TItemPtr, TItem]{
		storedAs:        ProjItemStoredAsDir,
		dirPath:         dirPath,
		summaryFileName: summaryFileName,
	}
}

func (s fsProjectItemsStore[TSlice, TItemPtr, TItem]) loadProjectItem(
	_ context.Context, dirPath, id, fileName string, o ...datatug.StoreOption,
) (
	item TItemPtr, err error,
) {
	_ = datatug.GetStoreOptions(o...)
	if fileName == "" {
		switch s.storedAs {
		case ProjItemStoredAsFile:
			if s.itemFileSuffix == "" {
				fileName = id + ".json"
			} else {
				fileName = id + "." + s.itemFileSuffix + ".json"
			}
		case ProjItemStoredAsDir:
			dirPath = path.Join(dirPath, id)
			fileName = s.summaryFileName
		}

	}
	filePath := path.Join(dirPath, fileName)
	item = new(TItem)
	if err = readJSONFile(filePath, true, &item); err != nil {
		return item, fmt.Errorf("failed to load %T[%s] from project: %w", item, id, err)
	}
	item.SetID(id)
	return
}

func (s fsProjectItemsStore[TSlice, TItemPtr, TItem]) loadProjectItems(
	ctx context.Context, dirPath string, o ...datatug.StoreOption,
) (
	items TSlice, err error,
) {
	_ = datatug.GetStoreOptions(o...)

	var filesMask string
	var fsObjectType process
	var loader func(f os.FileInfo, i int, mutex *sync.Mutex) (err error)

	switch s.storedAs {
	case ProjItemStoredAsFile:
		if s.itemFileSuffix == "" {
			filesMask = "*.json"
		} else {
			filesMask = fmt.Sprintf("*.%s.json", s.itemFileSuffix)
		}
		fsObjectType = processFiles
		loader = func(f os.FileInfo, i int, mutex *sync.Mutex) error {
			if f.IsDir() {
				return nil
			}
			id, suffix := storage.GetProjItemIDFromFileName(f.Name())
			if suffix != s.itemFileSuffix {
				return nil
			}
			item, err := s.loadProjectItem(ctx, dirPath, id, f.Name())
			if err != nil {
				return err
			}
			mutex.Lock()
			items = append(items, item)
			mutex.Unlock()
			return nil
		}
	case ProjItemStoredAsDir:
		fsObjectType = processDirs
		loader = func(f os.FileInfo, i int, mutex *sync.Mutex) error {
			if !f.IsDir() {
				return nil
			}
			id := f.Name()
			itemDir := path.Join(dirPath, id)
			var item TItemPtr
			item, err = s.loadProjectItem(ctx, itemDir, id, fmt.Sprintf(".datatug-%s.json", s.itemFileSuffix))
			if err != nil && !errors.Is(err, fs.ErrNotExist) {
				return err
			}
			item.SetID(id)
			mutex.Lock()
			items = append(items, item)
			mutex.Unlock()
			return nil
		}
	}

	err = loadDir(nil, dirPath, filesMask, fsObjectType,
		func(files []os.FileInfo) {
			items = make(TSlice, 0, len(files))
		},
		loader)

	slices.SortFunc(items, func(a, b TItemPtr) int {
		return strings.Compare(a.GetID(), b.GetID())
	})

	if err != nil {
		return nil, err
	}
	return
}

func (s fsProjectItemsStore[TSlice, TItemPtr, TItem]) saveProjectItem(_ context.Context, dirPath string, item TItemPtr, o ...datatug.StoreOption) error {
	_ = datatug.GetStoreOptions(o...)
	if item == nil {
		return fmt.Errorf("an attempt to save a nil %T to %s", item, dirPath)
	}
	id := item.GetID()
	var fileName string
	switch s.storedAs {
	case ProjItemStoredAsFile:
		fileName = storage.JsonFileName(id, s.itemFileSuffix)
	case ProjItemStoredAsDir:
		dirPath = path.Join(dirPath, id)
		fileName = s.summaryFileName
	}

	if err := saveJSONFile(dirPath, fileName, item); err != nil {
		return fmt.Errorf("failed to save %T file: %w", item, err)
	}
	return nil
}

func (s fsProjectItemsStore[TSlice, TItemPtr, TItem]) saveProjectItems(ctx context.Context, dirPath string, items TSlice) error {
	return saveItems(dirPath, len(items), func(i int) func() error {
		return func() error {
			return s.saveProjectItem(ctx, dirPath, items[i])
		}
	})

}

func (s fsProjectItemsStore[TSlice, TItemPtr, TItem]) itemFileName(id string) string {
	return storage.JsonFileName(id, s.itemFileSuffix)
}

func (s fsProjectItemsStore[TSlice, TItemPtr, TItem]) itemFilePath(dirPath, id string) string {
	return path.Join(dirPath, s.itemFileName(id))
}

func (s fsProjectItemsStore[TSlice, TItemPtr, TItem]) deleteProjectItem(_ context.Context, dirPath, id string) error {
	filePath := s.itemFilePath(dirPath, id)
	if _, err := os.Stat(filePath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.Remove(filePath)
}
