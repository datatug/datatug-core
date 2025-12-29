package comparator

import (
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/assert"
)

func TestCompareDatabases(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		_, err := CompareDatabases(DatabasesToCompare{})
		assert.Nil(t, err)
	})

	t.Run("complex", func(t *testing.T) {
		dbsToCompare := DatabasesToCompare{
			DbModel: datatug.DbModel{
				Schemas: datatug.SchemaModels{
					{
						ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "s1"}},
						Tables: datatug.TableModels{
							{
								DBCollectionKey: datatug.NewTableKey("t1", "s1", "c1", nil),
							},
						},
						Views: datatug.TableModels{
							{
								DBCollectionKey: datatug.NewViewKey("v1", "s1", "c1", nil),
							},
						},
					},
				},
			},
			Environments: []EnvToCompare{
				{
					ID: "e1",
					Databases: datatug.EnvDbCatalogs{
						{
							DbCatalogBase: datatug.DbCatalogBase{
								ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "c1"}},
							},
							Schemas: datatug.DbSchemas{
								nil,
								{
									ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "s1"}},
									Tables: []*datatug.CollectionInfo{
										nil,
										{
											DBCollectionKey: datatug.NewTableKey("t1", "s1", "c1", nil),
										},
									},
									Views: []*datatug.CollectionInfo{
										nil,
										{
											DBCollectionKey: datatug.NewViewKey("v1", "s1", "c1", nil),
										},
									},
								},
							},
						},
						{
							DbCatalogBase: datatug.DbCatalogBase{
								ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "c2"}},
							},
							Schemas: datatug.DbSchemas{
								{
									ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "s1"}},
									Tables: []*datatug.CollectionInfo{
										{
											DBCollectionKey: datatug.NewTableKey("t1", "s1", "c1", nil),
										},
									},
								},
							},
						},
					},
				},
			},
		}
		diff, err := CompareDatabases(dbsToCompare)
		assert.Nil(t, err)
		assert.NotNil(t, diff)
	})

	t.Run("error", func(t *testing.T) {
		dbsToCompare := DatabasesToCompare{
			Environments: []EnvToCompare{
				{
					ID: "e1",
					Databases: datatug.EnvDbCatalogs{
						{
							Schemas: datatug.DbSchemas{
								{
									ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "s1"}},
									Tables: []*datatug.CollectionInfo{
										{
											DBCollectionKey: datatug.NewTableKey("$error$", "s1", "c1", nil),
										},
									},
								},
							},
						},
					},
				},
			},
		}
		_, err := CompareDatabases(dbsToCompare)
		assert.NotNil(t, err)
	})
}
