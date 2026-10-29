package datatug

import "context"

// QueryFolderDirectories is an optional native provider capability. These are
// directories under queries/, distinct from the legacy general FoldersStore.
// Deletion/archive require the independent project lifecycle capability.
type QueryFolderDirectories interface {
	CreateQueryFolder(context.Context, string, string) error
	DeclaredQueryFolders(context.Context) ([]string, error)
}

// GuardedQueryFolderDirectories lets request-serving callers recheck authority
// after the provider's locks, recovery and admission reads. authorize must run
// before acknowledging an existing declaration or writing the target directory,
// README or marker. A refusal leaves that target unchanged; prior transaction
// recovery remains the provider's responsibility.
type GuardedQueryFolderDirectories interface {
	QueryFolderDirectories
	CreateQueryFolderGuarded(context.Context, string, string, func() error) error
}
