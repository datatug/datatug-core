package storage

import (
	"github.com/datatug/datatug-core/pkg/datatug"
)

type ProjectStoreRef interface {
	ProjectStore() datatug.ProjectStore
}
