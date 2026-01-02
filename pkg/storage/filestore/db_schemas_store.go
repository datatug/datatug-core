package filestore

//import (
//	"context"
//	"path"
//
//	"github.com/datatug/datatug-core/pkg/datatug"
//	"github.com/datatug/datatug-core/pkg/storage"
//)
//
//var _ datatug.DbSchemasStore = (*fsDbSchemasStore)(nil)
//
//func newFsDbSchemasStore(projectPath string) fsDbSchemasStore {
//	return fsDbSchemasStore{
//		fsProjectItemsStore: newFileProjectItemsStore[datatug.DbSchemas, *datatug.DbSchema, datatug.DbSchema](
//			path.Join(projectPath, storage.SchemasFolder), storage.DbSchemaFileSuffix,
//		),
//	}
//}
//
//type fsDbSchemasStore struct {
//	fsProjectItemsStore[datatug.DbSchemas, *datatug.DbSchema, datatug.DbSchema]
//}
//
//func (s fsDbSchemasStore) LoadDbSchemas(ctx context.Context, o ...datatug.StoreOption) (datatug.DbSchemas, error) {
//	items, err := s.loadProjectItems(ctx, s.dirPath, o...)
//	return items, err
//}
//
//func (s fsDbSchemasStore) LoadDbSchema(ctx context.Context, id string, o ...datatug.StoreOption) (*datatug.DbSchema, error) {
//	return s.loadProjectItem(ctx, s.dirPath, id, s.itemFileName(id), o...)
//}
//
//func (s fsDbSchemasStore) SaveDbSchema(ctx context.Context, DbSchema *datatug.DbSchema) error {
//	return s.saveProjectItem(ctx, s.dirPath, DbSchema)
//}
//
//func (s fsDbSchemasStore) saveDbSchemas(ctx context.Context, DbSchemas datatug.DbSchemas) error {
//	return s.saveProjectItems(ctx, s.dirPath, DbSchemas)
//}
//
//func (s fsDbSchemasStore) DeleteDbSchema(ctx context.Context, id string) error {
//	return s.deleteProjectItem(ctx, s.dirPath, id)
//}
