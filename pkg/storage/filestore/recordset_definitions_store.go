package filestore

import (
	"context"
	"path"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/storage"
)

var _ datatug.RecordsetDefinitionsStore = (*fsRecordsetDefinitionsStore)(nil)

func newFsRecordsetDefinitionsStore(projectPath string) fsRecordsetDefinitionsStore {
	return fsRecordsetDefinitionsStore{
		fsProjectItemsStore: newFileProjectItemsStore[datatug.RecordsetDefinitions, *datatug.RecordsetDefinition, datatug.RecordsetDefinition](
			path.Join(projectPath, storage.RecordsetsFolder), storage.RecordsetFileSuffix,
		),
	}
}

type fsRecordsetDefinitionsStore struct {
	fsProjectItemsStore[datatug.RecordsetDefinitions, *datatug.RecordsetDefinition, datatug.RecordsetDefinition]
}

func (s fsRecordsetDefinitionsStore) LoadRecordsetDefinitions(ctx context.Context, o ...datatug.StoreOption) ([]*datatug.RecordsetDefinition, error) {
	return s.loadProjectItems(ctx, s.dirPath, o...)
}

func (s fsRecordsetDefinitionsStore) LoadRecordsetDefinition(ctx context.Context, id string, o ...datatug.StoreOption) (*datatug.RecordsetDefinition, error) {
	return s.loadProjectItem(ctx, s.dirPath, id, "", o...)
}

func (s fsRecordsetDefinitionsStore) LoadRecordsetData(ctx context.Context, id string) (datatug.Recordset, error) {
	_, _ = ctx, id
	panic("implement me") //TODO implement LoadRecordsetData
}
