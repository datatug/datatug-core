package filestore

import (
	"context"
	"fmt"
	"os"
	"path"
	"sync"

	"github.com/datatug/datatug-core/pkg/datatug"
)

type IItem any

type IItemPtr[T IItem] interface {
	~*T
	GetID() string
	SetID(id string)
	Validate() error
}

type fsProjectItemsStore[TSlice ~[]TItemPtr, TItemPtr IItemPtr[TItem], TItem IItem] struct {
	dirPath        string
	itemFileSuffix string
}

func newFsProjectItemsStore[TSlice ~[]TItemPtr, TItemPtr IItemPtr[TItem], TItem IItem](
	projectPath, dirName, itemFileSuffix string,
) fsProjectItemsStore[TSlice, TItemPtr, TItem] {
	return fsProjectItemsStore[TSlice, TItemPtr, TItem]{
		dirPath:        path.Join(projectPath, dirName),
		itemFileSuffix: itemFileSuffix,
	}
}

func (s fsProjectItemsStore[TSlice, TItemPtr, TItem]) loadProjectItem(
	_ context.Context, id, fileName string, o ...datatug.StoreOption,
) (
	item TItemPtr, err error,
) {
	_ = datatug.GetStoreOptions(o...)
	filePath := path.Join(s.dirPath, fileName)
	item = new(TItem)
	if err = readJSONFile(filePath, true, &item); err != nil {
		return item, fmt.Errorf("failed to load %T[%s] from project: %w", item, id, err)
	}
	item.SetID(id)
	return
}

func (s fsProjectItemsStore[TSlice, TItemPtr, TItem]) loadProjectItems(
	ctx context.Context, o ...datatug.StoreOption,
) (
	items TSlice, err error,
) {
	_ = datatug.GetStoreOptions(o...)

	if err = loadDir(nil, s.dirPath, "*.json", processFiles,
		func(files []os.FileInfo) {
			items = make(TSlice, 0, len(files))
		},
		func(f os.FileInfo, i int, mutex *sync.Mutex) error {
			if f.IsDir() {
				return nil
			}
			id, suffix := getProjItemIDFromFileName(f.Name())
			if suffix != s.itemFileSuffix {
				return nil
			}
			item, err := s.loadProjectItem(ctx, id, f.Name())
			if err != nil {
				return err
			}
			mutex.Lock()
			items = append(items, item)
			mutex.Unlock()
			return nil
		}); err != nil {
		return nil, err
	}
	return
}

func (s fsProjectItemsStore[TSlice, TItemPtr, TItem]) saveProjectItem(_ context.Context, item TItemPtr) error {
	id := item.GetID()
	fileName := jsonFileName(id, s.itemFileSuffix)
	if err := saveJSONFile(s.dirPath, fileName, item); err != nil {
		return fmt.Errorf("failed to save %T file: %w", item, err)
	}
	return nil
}

func (s fsProjectItemsStore[TSlice, TItemPtr, TItem]) saveProjectItems(ctx context.Context, folderName string, items TSlice) error {
	return saveItems(folderName, len(items), func(i int) func() error {
		return func() error {
			return s.saveProjectItem(ctx, items[i])
		}
	})

}

func (s fsProjectItemsStore[TSlice, TItemPtr, TItem]) itemFileName(id string) string {
	return jsonFileName(id, s.itemFileSuffix)
}

func (s fsProjectItemsStore[TSlice, TItemPtr, TItem]) itemFilePath(id string) string {
	return path.Join(s.dirPath, s.itemFileName(id))
}

func (s fsProjectItemsStore[TSlice, TItemPtr, TItem]) deleteProjectItem(_ context.Context, id string) error {
	filePath := s.itemFilePath(id)
	if _, err := os.Stat(filePath); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.Remove(filePath)
}
