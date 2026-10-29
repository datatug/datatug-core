package plainfs

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPublishFileRefusesReplacedParentAtCallerDecision(t *testing.T) {
	for _, kind := range []string{"root", "ancestor", "parent", "parent symlink", "missing root", "missing parent"} {
		t.Run(kind, func(t *testing.T) {
			base := t.TempDir()
			root, moved := filepath.Join(base, "project"), filepath.Join(base, "moved")
			require.NoError(t, os.Mkdir(root, 0o700))
			target := filepath.Join(root, "boards", "new", "board.json")
			victim := filepath.Dir(target)
			if kind == "root" || kind == "missing root" {
				victim = root
			}
			if kind == "ancestor" {
				victim = filepath.Join(root, "boards")
			}
			err := PublishFile(context.Background(), root, target, []byte("must not publish in moved parent"), func() error {
				require.NoError(t, os.Rename(victim, moved))
				if kind == "missing root" || kind == "missing parent" {
					return nil
				}
				if kind == "parent symlink" {
					return os.Symlink(moved, victim)
				}
				return os.Mkdir(victim, 0o700)
			})
			require.Error(t, err, "a moved opened parent is not the requested logical path")
			_, statErr := os.Lstat(victim)
			if kind == "missing root" || kind == "missing parent" {
				require.ErrorIs(t, statErr, fs.ErrNotExist)
			} else {
				require.NoError(t, statErr, "cleanup must preserve the replacement, even when empty")
			}
			movedTarget := filepath.Join(moved, "board.json")
			if kind == "ancestor" {
				movedTarget = filepath.Join(moved, "new", "board.json")
			}
			if kind == "root" || kind == "missing root" {
				movedTarget = filepath.Join(moved, "boards", "new", "board.json")
			}
			require.NoFileExists(t, movedTarget, "caller-time replacement must be refused before publication")
			require.NoError(t, filepath.WalkDir(base, func(path string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				require.False(t, strings.HasPrefix(entry.Name(), ".datatug-create-"), "owned stage remains: %s", path)
				return nil
			}))
		})
	}
}

func TestPublishFileRefusesOpenedParentIdentityDrift(t *testing.T) {
	for _, kind := range []string{"different parent", "unreadable opened parent"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, "new", "board.json")
			open, lstat := publishOpenParent, publishLstat
			t.Cleanup(func() { publishOpenParent, publishLstat = open, lstat })
			failure := errors.New("opened parent cannot be verified")
			var opened *os.Root
			publishOpenParent = func(r *os.Root, name string) (*os.Root, error) {
				if kind == "different parent" {
					require.NoError(t, os.Rename(filepath.Dir(target), filepath.Join(root, "moved")))
					require.NoError(t, os.Mkdir(filepath.Dir(target), 0o700))
				}
				var err error
				opened, err = open(r, name)
				return opened, err
			}
			publishLstat = func(r *os.Root, name string) (fs.FileInfo, error) {
				if kind == "unreadable opened parent" && r == opened {
					return nil, failure
				}
				return lstat(r, name)
			}
			called := false
			err := PublishFile(context.Background(), root, target, []byte("content"), func() error { called = true; return nil })
			require.Error(t, err)
			require.False(t, called, "invalid preparation cannot reach the caller's final decision")
			if kind == "different parent" {
				require.DirExists(t, filepath.Dir(target))
			} else {
				require.ErrorIs(t, err, failure)
			}
			require.NoFileExists(t, target)
		})
	}
}

func TestPublishFileCancellationDuringFinalPathCheckRefusesPublication(t *testing.T) {
	root := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	stat := publishRootStat
	t.Cleanup(func() { publishRootStat = stat })
	publishRootStat = func(name string) (fs.FileInfo, error) { info, err := stat(name); cancel(); return info, err }
	err := PublishFile(ctx, root, filepath.Join(root, "new", "board.json"), []byte("content"), nil)
	require.ErrorIs(t, err, context.Canceled)
	entries, err := os.ReadDir(root)
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestPublishFileNeverAcknowledgesParentDriftDuringPublication(t *testing.T) {
	for _, boundary := range []string{"link", "directory sync"} {
		t.Run(boundary, func(t *testing.T) {
			root := t.TempDir()
			target, moved := filepath.Join(root, "new", "board.json"), filepath.Join(root, "moved")
			link, syncDir := publishLink, publishSyncDir
			t.Cleanup(func() { publishLink, publishSyncDir = link, syncDir })
			rotate := func() {
				require.NoError(t, os.Rename(filepath.Dir(target), moved))
				require.NoError(t, os.Mkdir(filepath.Dir(target), 0o700))
			}
			if boundary == "link" {
				publishLink = func(r *os.Root, old, next string) error { rotate(); return link(r, old, next) }
			} else {
				publishSyncDir = func(r *os.Root) error { err := syncDir(r); rotate(); return err }
			}
			err := PublishFile(context.Background(), root, target, []byte("complete content"), nil)
			require.Error(t, err, "changed logical path cannot be acknowledged")
			require.DirExists(t, filepath.Dir(target), "replacement remains")
			require.NoFileExists(t, target)
			body, err := os.ReadFile(filepath.Join(moved, "board.json"))
			require.NoError(t, err)
			require.Equal(t, "complete content", string(body), "post-link refusal does not claim rollback")
		})
	}
}

func TestPublishFileRefusalCleanupPreservesReplacementEmptyDirectory(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "new", "board.json")
	denied := errors.New("caller refused after parent replacement")
	err := PublishFile(context.Background(), root, target, []byte("content"), func() error {
		require.NoError(t, os.Rename(filepath.Dir(target), filepath.Join(root, "moved")))
		require.NoError(t, os.Mkdir(filepath.Dir(target), 0o700))
		return denied
	})
	require.ErrorIs(t, err, denied)
	require.DirExists(t, filepath.Dir(target), "replacement empty directory does not belong to this call")
	entries, err := os.ReadDir(filepath.Join(root, "moved"))
	require.NoError(t, err)
	require.Empty(t, entries, "owned stage should be removed from the pinned moved parent")
}

func TestPublishFilePreservesTheWinningFileAtTheFinalSystemCall(t *testing.T) {
	for _, kind := range []string{"valid file", "symlink", "directory", "hardlink"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			target, source := filepath.Join(root, "target"), filepath.Join(root, "winner")
			original := []byte("complete winning retained content")
			require.NoError(t, os.WriteFile(source, original, 0o600))
			err := PublishFile(context.Background(), root, target, []byte("must not replace"), func() error {
				switch kind {
				case "valid file":
					return os.WriteFile(target, original, 0o600)
				case "symlink":
					return os.Symlink(source, target)
				case "directory":
					return os.Mkdir(target, 0o700)
				case "hardlink":
					return os.Link(source, target)
				}
				return nil
			})
			require.ErrorIs(t, err, fs.ErrExist)
			if kind != "directory" {
				body, err := os.ReadFile(target)
				require.NoError(t, err)
				require.Equal(t, original, body)
			}
			entries, err := os.ReadDir(root)
			require.NoError(t, err)
			require.Len(t, entries, 2, "no abandoned staging file")
		})
	}
}

func TestPublishFileAdmissionAndPathRefusals(t *testing.T) {
	for _, kind := range []string{"outside", "root", "missing root", "canceled", "parent file", "parent symlink", "denied", "cancel at decision", "valid nested"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, "new", "nested", "board.json")
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			denied := errors.New("not authorized")
			var authorize func() error
			switch kind {
			case "outside":
				target = filepath.Join(root, "..", "outside")
			case "root":
				target = root
			case "missing root":
				root = filepath.Join(root, "missing")
				target = filepath.Join(root, "board.json")
			case "canceled":
				cancel()
			case "parent file":
				require.NoError(t, os.WriteFile(filepath.Join(root, "new"), []byte("existing"), 0o600))
			case "parent symlink":
				require.NoError(t, os.Symlink(t.TempDir(), filepath.Join(root, "new")))
			case "denied":
				authorize = func() error { return denied }
			case "cancel at decision":
				authorize = func() error { cancel(); return nil }
			}
			err := PublishFile(ctx, root, target, []byte("complete content"), authorize)
			if kind == "valid nested" {
				require.NoError(t, err)
				body, err := os.ReadFile(target)
				require.NoError(t, err)
				require.Equal(t, "complete content", string(body))
				return
			}
			require.Error(t, err)
			if kind == "denied" {
				require.ErrorIs(t, err, denied)
			}
			if kind == "canceled" || kind == "cancel at decision" {
				require.ErrorIs(t, err, context.Canceled)
			}
			if kind == "denied" || kind == "cancel at decision" {
				entries, err := os.ReadDir(root)
				require.NoError(t, err)
				require.Empty(t, entries, "refused preparation is removed")
			}
		})
	}
}

type failedPublicationFile struct {
	publicationFile
	failure string
	err     error
}

func (f failedPublicationFile) Write(b []byte) (int, error) {
	if f.failure == "write" {
		return 0, f.err
	}
	return f.publicationFile.Write(b)
}
func (f failedPublicationFile) Sync() error {
	if f.failure == "sync" {
		return f.err
	}
	return f.publicationFile.Sync()
}
func (f failedPublicationFile) Close() error {
	err := f.publicationFile.Close()
	if f.failure == "close" {
		return errors.Join(err, f.err)
	}
	return err
}

func TestPublishFileIOFailuresNeverAcknowledgeOrReplace(t *testing.T) {
	for _, kind := range []string{"open", "write", "sync", "close", "link", "directory sync", "cleanup", "lstat", "parent stat", "mkdir", "mkdir race", "parent open"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			target := filepath.Join(root, "new", "board.json")
			failure := errors.New("injected owning I/O failure")
			open, link, remove, syncDir, lstat, mkdir, parent := publishOpenFile, publishLink, publishRemove, publishSyncDir, publishLstat, publishMkdir, publishOpenParent
			t.Cleanup(func() {
				publishOpenFile, publishLink, publishRemove, publishSyncDir, publishLstat, publishMkdir, publishOpenParent = open, link, remove, syncDir, lstat, mkdir, parent
			})
			switch kind {
			case "open":
				publishOpenFile = func(*os.Root, string) (publicationFile, error) { return nil, failure }
			case "write", "sync", "close":
				publishOpenFile = func(r *os.Root, name string) (publicationFile, error) {
					f, err := open(r, name)
					return failedPublicationFile{publicationFile: f, failure: kind, err: failure}, err
				}
			case "link":
				publishLink = func(*os.Root, string, string) error { return failure }
			case "directory sync":
				publishSyncDir = func(*os.Root) error { return failure }
			case "cleanup":
				publishRemove = func(*os.Root, string) error { return failure }
			case "lstat":
				publishLstat = func(*os.Root, string) (fs.FileInfo, error) { return nil, failure }
			case "parent stat":
				publishLstat = func(r *os.Root, name string) (fs.FileInfo, error) {
					if name != "." {
						return nil, failure
					}
					return lstat(r, name)
				}
			case "mkdir":
				publishMkdir = func(*os.Root, string, fs.FileMode) error { return failure }
			case "mkdir race":
				publishMkdir = func(r *os.Root, name string, mode fs.FileMode) error {
					require.NoError(t, mkdir(r, name, mode))
					return fs.ErrExist
				}
			case "parent open":
				publishOpenParent = func(*os.Root, string) (*os.Root, error) { return nil, failure }
			}
			err := PublishFile(context.Background(), root, target, []byte("complete content"), nil)
			if kind == "mkdir race" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, failure)
			if kind == "directory sync" || kind == "cleanup" {
				body, err := os.ReadFile(target)
				require.NoError(t, err)
				require.Equal(t, "complete content", string(body), "post-publication failure must not pretend the write was rolled back")
			} else {
				_, err := os.Lstat(target)
				require.ErrorIs(t, err, fs.ErrNotExist)
				entries, err := os.ReadDir(root)
				require.NoError(t, err)
				require.Empty(t, entries)
			}
		})
	}
}

func TestPublicationDirectorySyncRefusesAClosedRoot(t *testing.T) {
	r, err := os.OpenRoot(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, r.Close())
	require.Error(t, publishSyncDir(r))
}
