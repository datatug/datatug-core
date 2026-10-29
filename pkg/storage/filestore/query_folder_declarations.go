package filestore

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/datatug/datatug-core/internal/jsonstrict"
	"github.com/datatug/datatug-core/pkg/datatug"
	"io"
	"os"
	"path"
	"reflect"
	"sort"
	"strings"
)

const queryFolderDeclarationFile = ".datatug-query-folder.json"

var _ datatug.QueryFolderDirectories = (*fsQueriesStore)(nil)

var openQueryFolderDirectory = func(name string) (connectionDirectoryReader, error) { return os.Open(name) }
var queryDeclarationListingBudget = fsQueriesStore.newListingBudget

type queryFolderDeclaration struct {
	Version int    `json:"version"`
	Name    string `json:"name"`
}

func readQueryFolderDeclaration(directory, name string, budget *queryReadBudget) (bool, error) {
	body, exists, err := readRegularFileBudgeted(path.Join(directory, queryFolderDeclarationFile), 4096, budget)
	if err != nil || !exists {
		return false, err
	}
	var value queryFolderDeclaration
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if jsonstrict.CheckNoDuplicateKeysForSchema(body, reflect.TypeOf(value)) != nil || decoder.Decode(&value) != nil || decoder.Decode(new(any)) != io.EOF || value.Version != 1 || value.Name != name {
		return false, fmt.Errorf("invalid query folder declaration")
	}
	return true, nil
}

func writeQueryFolderDeclaration(directory, name string) error {
	if exists, err := readQueryFolderDeclaration(directory, name, nil); err != nil || exists {
		return err
	}
	body, _ := json.Marshal(queryFolderDeclaration{Version: 1, Name: name})
	// Publish complete metadata through the existing durable temp-file/rename
	// helper. Failed writes/syncs leave no malformed final declaration.
	return durableEnvironmentFile(path.Join(directory, queryFolderDeclarationFile), body, 0o644)
}

// DeclaredQueryFolders retains explicitly created empty folders across reloads.
// Unrelated directories/README files are not declarations. This is read-only,
// follows no symlink directories, and charges a shared bounded traversal budget.
func (s fsQueriesStore) DeclaredQueryFolders(ctx context.Context) (out []string, retErr error) {
	retErr = s.withQueryReadLock(ctx, func(g queryLockGuard) error {
		var err error
		out, err = s.declaredQueryFoldersLocked(ctx, g, s.newListingBudget(), 1000, 10000)
		return err
	})
	if retErr != nil {
		return nil, retErr
	}
	return out, ctx.Err()
}

func (s fsQueriesStore) declaredQueryFoldersLocked(ctx context.Context, g queryLockGuard, budget *queryReadBudget, maximumDirectories, maximumEntries int) (out []string, retErr error) {
	retErr = func() error {
		visited := 0
		remainingEntries := maximumEntries
		var walk func(string, int) error
		walk = func(relative string, depth int) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			visited++
			if visited > maximumDirectories || depth > 32 {
				return fmt.Errorf("query folder declaration traversal exceeds capacity")
			}
			directory, err := walkQueryDir(s.dirPath, relative, "", false)
			if err != nil {
				return err
			}
			if err = g.refuseStuckFolder(relative, directory); err != nil {
				return err
			}
			if relative != "" {
				exists, err := readQueryFolderDeclaration(directory, path.Base(relative), budget)
				if err != nil {
					return err
				}
				if exists {
					out = append(out, relative)
				}
			}
			entries, err := readQueryFolderEntries(ctx, directory, budget, &remainingEntries)
			if err != nil {
				return err
			}
			for _, entry := range entries {
				if entry.IsDir() && entry.Name() != reservedQueryTxnDirName && !strings.HasPrefix(entry.Name(), ".") {
					if err = walk(path.Join(relative, entry.Name()), depth+1); err != nil {
						return err
					}
				}
			}
			return nil
		}
		_, err := dirLstat(s.dirPath)
		if os.IsNotExist(err) {
			out = []string{}
			return nil
		}
		if err != nil {
			return err
		}
		return walk("", 0)
	}()
	if retErr != nil {
		return nil, retErr
	}
	sort.Strings(out)
	return out, ctx.Err()
}

// Admit the prospective directory, README and declaration within the same lock
// and bounds used by readers. An accepted create cannot strand a project beyond
// the directory/depth/entry/byte limits its next listing must enforce.
func (s fsQueriesStore) admitQueryFolderDeclaration(ctx context.Context, g queryLockGuard, parent, name string) (already bool, err error) {
	relative := path.Join(parent, name)
	if len(strings.Split(relative, "/")) > 32 {
		return false, fmt.Errorf("query folder declaration depth exceeds capacity")
	}
	// Even a repeat create must prove that the complete target is confined.
	// Reading an existing declaration first would follow a symlinked target.
	directory, err := walkQueryDir(s.dirPath, relative, "", false)
	if err != nil {
		return false, err
	}
	if err := g.refuseStuckFolder(relative, directory); err != nil {
		return false, err
	}
	if exists, err := readQueryFolderDeclaration(directory, name, nil); err != nil || exists {
		return exists, err
	}
	newDirectories, newEntries, reservedBytes := 0, 1, int64(len(queryFolderDeclarationFile)+32)
	body, _ := json.Marshal(queryFolderDeclaration{Version: 1, Name: name})
	reservedBytes += int64(len(body))
	if _, err := dirLstat(directory); os.IsNotExist(err) {
		newDirectories++
		newEntries++
		reservedBytes += int64(len(name) + 32)
	} else if err != nil {
		return false, err
	}
	if _, err := dirLstat(path.Join(directory, "README.md")); os.IsNotExist(err) {
		newEntries++
		reservedBytes += int64(len("README.md") + 32)
	} else if err != nil {
		return false, err
	}
	budget := queryDeclarationListingBudget(s)
	if err := budget.take(directory, reservedBytes); err != nil {
		return false, err
	}
	_, err = s.declaredQueryFoldersLocked(ctx, g, budget, 1000-newDirectories, 10000-newEntries)
	if err != nil {
		return false, err
	}
	// project_full shares its byte budget with query definitions and bodies.
	// Reserve the additional marker there too, before changing the directory.
	treeBudget := queryDeclarationListingBudget(s)
	if err = treeBudget.take(directory, int64(len(body))); err != nil {
		return false, err
	}
	_, err = s.loadQueriesTreeLocked(ctx, g, "", treeBudget)
	return false, err
}

func readQueryFolderEntries(ctx context.Context, directory string, budget *queryReadBudget, remaining *int) (out []os.DirEntry, retErr error) {
	f, err := openQueryFolderDirectory(directory)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := f.Close(); retErr == nil && err != nil {
			out, retErr = nil, err
		}
	}()
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		entries, err := f.ReadDir(64)
		if err != nil && err != io.EOF {
			return nil, err
		}
		for _, entry := range entries {
			if *remaining == 0 {
				return nil, fmt.Errorf("query folder declaration traversal exceeds entry capacity")
			}
			*remaining--
			if err := budget.take(directory, int64(len(entry.Name())+32)); err != nil {
				return nil, err
			}
			out = append(out, entry)
		}
		if err == io.EOF {
			return out, ctx.Err()
		}
	}
}
