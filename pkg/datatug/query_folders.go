package datatug

import "context"

// QueryFolderDirectories is an optional native provider capability. These are
// directories under queries/, distinct from the legacy general FoldersStore.
// Deletion/archive require the independent project lifecycle capability.
type QueryFolderDirectories interface {
	CreateQueryFolder(context.Context, string, string) error
	DeclaredQueryFolders(context.Context) ([]string, error)
}
