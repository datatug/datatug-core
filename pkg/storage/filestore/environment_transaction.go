package filestore

// This transaction is intentionally limited to project environment metadata.
import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/dto"
	"github.com/gofrs/flock"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

// environmentOS holds filesystem boundaries so durability failures can be exercised
// deterministically on unprivileged hosts, as for the query transaction adapter.
type environmentFile interface {
	Name() string
	Chmod(os.FileMode) error
	Write([]byte) (int, error)
	WriteAt([]byte, int64) (int, error)
	Sync() error
	Close() error
}

var environmentOS = struct {
	ReadFile     func(string) ([]byte, error)
	Open         func(string) (environmentFile, error)
	CreateTemp   func(string, string) (environmentFile, error)
	Remove       func(string) error
	Rename       func(string, string) error
	Lstat        func(string) (os.FileInfo, error)
	Stat         func(string) (os.FileInfo, error)
	OpenFile     func(string, int, os.FileMode) (environmentFile, error)
	MkdirAll     func(string, os.FileMode) error
	UserCacheDir func() (string, error)
	MkdirTemp    func(string, string) (string, error)
	EvalSymlinks func(string) (string, error)
	Abs          func(string) (string, error)
	Random       func([]byte) (int, error)
}{
	os.ReadFile,
	func(p string) (environmentFile, error) { return os.Open(p) },
	func(d, p string) (environmentFile, error) { return os.CreateTemp(d, p) },
	os.Remove, os.Rename, os.Lstat, os.Stat,
	func(p string, f int, m os.FileMode) (environmentFile, error) { return os.OpenFile(p, f, m) },
	os.MkdirAll, os.UserCacheDir, os.MkdirTemp, filepath.EvalSymlinks, filepath.Abs, rand.Read,
}

var environmentTransactionFault func(string)

func environmentTransactionStep(step string) {
	if environmentTransactionFault != nil {
		environmentTransactionFault(step)
	}
}

var ErrEnvironmentRecoveryRequired = errors.New("environment recovery required")

type environmentLocator struct {
	Format      string `json:"format"`
	RootDigest  string `json:"rootDigest"`
	UID         uint32 `json:"uid"`
	NamespaceID string `json:"namespaceId"`
	Directory   string `json:"directory"`
}
type environmentReplacement struct {
	Path    string
	Stage   string
	OldHash string
	NewHash string
}
type environmentIntent struct {
	Files   []environmentReplacement
	Result  dto.EnvironmentOperationResult
	Binding string
}

// EnvironmentFiles is valid only during WithEnvironmentFiles' locked callback.
type EnvironmentFiles struct {
	Root      string
	namespace string
}

func environmentHash(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func environmentFileHash(p string) (string, error) {
	b, e := environmentOS.ReadFile(p)
	if os.IsNotExist(e) {
		return "absent", nil
	}
	if e != nil {
		return "", e
	}
	return environmentHash(b), nil
}
func syncEnvironmentDir(p string) (err error) {
	f, e := environmentOS.Open(p)
	if e != nil {
		return e
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	return f.Sync()
}
func durableEnvironmentFile(p string, b []byte, mode os.FileMode) (err error) {
	f, e := environmentOS.CreateTemp(filepath.Dir(p), ".environment-write-")
	if e != nil {
		return e
	}
	tmp := f.Name()
	defer func() {
		if cleanupErr := environmentOS.Remove(tmp); cleanupErr != nil && !os.IsNotExist(cleanupErr) {
			err = errors.Join(err, cleanupErr)
		}
	}()
	if e = f.Chmod(mode); e == nil {
		_, e = f.Write(b)
	}
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	if e = environmentOS.Rename(tmp, p); e != nil {
		return e
	}
	return syncEnvironmentDir(filepath.Dir(p))
}
func environmentPlainPath(p string) error {
	p = filepath.Clean(p)
	for at := p; ; at = filepath.Dir(at) {
		s, e := environmentOS.Lstat(at)
		if e != nil {
			return e
		}
		if s.Mode()&os.ModeSymlink != 0 {
			return ErrEnvironmentRecoveryRequired
		}
		if at == filepath.Dir(at) {
			break
		}
	}
	return nil
}
func environmentPrivateDir(p string) error {
	if e := environmentPlainPath(p); e != nil {
		return e
	}
	s, e := environmentOS.Stat(p)
	if e != nil {
		return e
	}
	st, ok := s.Sys().(*syscall.Stat_t)
	if !ok || st.Uid != uint32(os.Getuid()) || s.Mode().Perm() != 0700 || !s.IsDir() {
		return ErrEnvironmentRecoveryRequired
	}
	return nil
}
func readEnvironmentPrivateFile(p string) ([]byte, error) {
	st, e := environmentOS.Lstat(p)
	if e != nil {
		return nil, e
	}
	owner, ok := st.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != uint32(os.Getuid()) || !st.Mode().IsRegular() || st.Mode().Perm() != 0600 || st.Size() > 8<<20 {
		return nil, ErrEnvironmentRecoveryRequired
	}
	return environmentOS.ReadFile(p)
}
func registerEnvironmentNamespace(root, digest, id string) error {
	lockPath := filepath.Join(root, ".datatug-environments.lock")
	want := []byte("environment-owner/1:" + digest + ":" + id)
	old, e := environmentOS.ReadFile(lockPath)
	if e != nil {
		return e
	}
	if len(old) > 0 {
		if string(old) != string(want) {
			return ErrEnvironmentRecoveryRequired
		}
		return nil
	}
	marker, e := environmentOS.OpenFile(lockPath, os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	_, e = marker.WriteAt(want, 0)
	if e == nil {
		e = marker.Sync()
	}
	ce := marker.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	return syncEnvironmentDir(root)
}
func environmentNamespace(root, stateRoot string) (string, error) {
	digest := environmentHash([]byte(root))
	git := exec.Command("git", "-C", root, "rev-parse", "--absolute-git-dir")
	if body, e := git.Output(); e == nil {
		dir := filepath.Join(strings.TrimSpace(string(body)), "datatug-environment-operations", digest)
		if _, e = environmentOS.Stat(dir); os.IsNotExist(e) {
			if marker, err := environmentOS.ReadFile(filepath.Join(root, ".datatug-environments.lock")); err != nil || len(marker) > 0 {
				return "", ErrEnvironmentRecoveryRequired
			}
		}
		if e = environmentOS.MkdirAll(dir, 0700); e != nil {
			return "", e
		}
		if e = environmentPrivateDir(dir); e != nil {
			return "", e
		}
		if e = registerEnvironmentNamespace(root, digest, "git:"+digest); e != nil {
			return "", e
		}
		return dir, nil
	}
	locatorPath := filepath.Join(root, ".datatug-environments-owner.json")
	if st, err := environmentOS.Lstat(locatorPath); err == nil && st.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("%w: %s is a symlink", ErrEnvironmentRecoveryRequired, filepath.Base(locatorPath))
	}
	body, e := environmentOS.ReadFile(locatorPath)
	if e == nil {
		var locator environmentLocator
		if json.Unmarshal(body, &locator) != nil || locator.Format != "datatug-environment-owner/1" || locator.RootDigest != digest || locator.UID != uint32(os.Getuid()) || locator.NamespaceID == "" || !filepath.IsAbs(locator.Directory) {
			return "", ErrEnvironmentRecoveryRequired
		}
		st, e := environmentOS.Lstat(locatorPath)
		if e != nil || st.Mode().Perm() != 0600 || st.Mode()&os.ModeSymlink != 0 {
			return "", ErrEnvironmentRecoveryRequired
		}
		owner, ok := st.Sys().(*syscall.Stat_t)
		if !ok || owner.Uid != uint32(os.Getuid()) {
			return "", ErrEnvironmentRecoveryRequired
		}
		if e = environmentPrivateDir(locator.Directory); e != nil {
			return "", fmt.Errorf("%w: %v", ErrEnvironmentRecoveryRequired, e)
		}
		identity, e := readEnvironmentPrivateFile(filepath.Join(locator.Directory, "namespace"))
		if e != nil || string(identity) != locator.NamespaceID {
			return "", ErrEnvironmentRecoveryRequired
		}
		if e = registerEnvironmentNamespace(root, digest, locator.NamespaceID); e != nil {
			return "", e
		}
		return locator.Directory, nil
	}
	if !os.IsNotExist(e) {
		return "", fmt.Errorf("%w: %v", ErrEnvironmentRecoveryRequired, e)
	}
	if marker, err := environmentOS.ReadFile(filepath.Join(root, ".datatug-environments.lock")); err != nil || len(marker) > 0 {
		return "", ErrEnvironmentRecoveryRequired
	}
	if stateRoot == "" {
		stateRoot, e = environmentOS.UserCacheDir()
		if e != nil {
			return "", e
		}
		stateRoot = filepath.Join(stateRoot, "datatug", "environment-operations")
	}
	if e = environmentOS.MkdirAll(stateRoot, 0700); e != nil {
		return "", e
	}
	stateRoot, e = environmentOS.EvalSymlinks(stateRoot)
	if e != nil {
		return "", e
	}
	dir, e := environmentOS.MkdirTemp(stateRoot, digest+"-")
	if e != nil {
		return "", e
	}
	if e = environmentPrivateDir(dir); e != nil {
		return "", e
	}
	var random [16]byte
	if _, e = environmentOS.Random(random[:]); e != nil {
		return "", e
	}
	id := hex.EncodeToString(random[:])
	if e = durableEnvironmentFile(filepath.Join(dir, "namespace"), []byte(id), 0600); e != nil {
		return "", e
	}
	locator := environmentLocator{"datatug-environment-owner/1", digest, uint32(os.Getuid()), id, dir}
	// Locator fields are primitive strings and integers only.
	body, _ = json.Marshal(locator)
	if e = durableEnvironmentFile(locatorPath, body, 0600); e != nil {
		return "", e
	}
	if e = registerEnvironmentNamespace(root, digest, id); e != nil {
		return "", e
	}
	return dir, nil
}

// WithEnvironmentFiles serializes cooperating processes and recovers before any callback.
func WithEnvironmentFiles(ctx context.Context, projectRoot, stateRoot string, fn func(*EnvironmentFiles) error) error {
	return withEnvironmentFiles(ctx, projectRoot, stateRoot, true, fn)
}

// ReadEnvironmentFiles serializes reads without creating project or private state.
// Existing operation ownership is still validated and recovered before reading.
func ReadEnvironmentFiles(ctx context.Context, projectRoot string, fn func(*EnvironmentFiles) error) error {
	return withEnvironmentFiles(ctx, projectRoot, "", false, fn)
}
func withEnvironmentFiles(ctx context.Context, projectRoot, stateRoot string, initialize bool, fn func(*EnvironmentFiles) error) (err error) {
	if projectRoot == "" {
		return datatug.ErrProjectDoesNotExist
	}
	root, e := environmentOS.EvalSymlinks(projectRoot)
	if e != nil {
		return e
	}
	root, e = environmentOS.Abs(root)
	if e != nil {
		return e
	}
	lockPath := filepath.Join(root, ".datatug-environments.lock")
	if st, e := environmentOS.Lstat(lockPath); e == nil && (!st.Mode().IsRegular() || st.Mode()&os.ModeSymlink != 0) {
		return fmt.Errorf("%w: %s is not a plain file", ErrEnvironmentRecoveryRequired, filepath.Base(lockPath))
	}
	// The directory inode exists even for read-only projects. All readers and
	// writers use it, so first-time initialization cannot race a read.
	lock := flock.New(root, flock.SetFlag(os.O_RDONLY))
	// flock.TryLockContext returns an error whenever acquisition is false.
	_, e = lock.TryLockContext(ctx, 10_000_000)
	if e != nil {
		return e
	}
	defer func() { err = errors.Join(err, lock.Unlock()) }()
	if !initialize {
		existing := false
		for _, name := range []string{".datatug-environments.lock", ".datatug-environments-owner.json"} {
			if _, err := environmentOS.Lstat(filepath.Join(root, name)); err == nil {
				existing = true
			} else if !os.IsNotExist(err) {
				return err
			}
		}
		git := exec.Command("git", "-C", root, "rev-parse", "--absolute-git-dir")
		if body, err := git.Output(); err == nil {
			_, err = environmentOS.Lstat(filepath.Join(strings.TrimSpace(string(body)), "datatug-environment-operations", environmentHash([]byte(root))))
			if err == nil {
				existing = true
			} else if !os.IsNotExist(err) {
				return err
			}
		}
		if !existing {
			return fn(&EnvironmentFiles{Root: root})
		}
	}
	if initialize {
		marker, err := environmentOS.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
		if err != nil {
			return err
		}
		if err = marker.Close(); err != nil {
			return err
		}
	}
	namespace, e := environmentNamespace(root, stateRoot)
	if e != nil {
		return e
	}
	files := &EnvironmentFiles{root, namespace}
	if e = files.recover(); e != nil {
		return fmt.Errorf("%w: %v", ErrEnvironmentRecoveryRequired, e)
	}
	return fn(files)
}
func (f *EnvironmentFiles) destination(rel string) (string, error) {
	if filepath.IsAbs(rel) || filepath.Clean(rel) != rel || strings.Contains(rel, "\\") {
		return "", ErrEnvironmentRecoveryRequired
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if rel != "datatug-project.json" && (len(parts) != 3 || parts[0] != "environments" || parts[1] == "" || parts[1] == "." || parts[1] == ".." || (parts[2] != parts[1]+".env.json" && parts[2] != "environment-summary.json")) {
		return "", ErrEnvironmentRecoveryRequired
	}
	p := filepath.Join(f.Root, rel)
	for at := p; at != filepath.Dir(f.Root); at = filepath.Dir(at) {
		s, e := environmentOS.Lstat(at)
		if os.IsNotExist(e) {
			continue
		}
		if e != nil || s.Mode()&os.ModeSymlink != 0 {
			return "", ErrEnvironmentRecoveryRequired
		}
		if at == f.Root {
			break
		}
	}
	return p, nil
}
func (f *EnvironmentFiles) recover() error {
	intentPath := filepath.Join(f.namespace, "intent.json")
	body, e := readEnvironmentPrivateFile(intentPath)
	if os.IsNotExist(e) {
		return nil
	}
	if e != nil {
		return e
	}
	var intent environmentIntent
	if e = json.Unmarshal(body, &intent); e != nil {
		return e
	}
	decision, e := readEnvironmentPrivateFile(filepath.Join(f.namespace, "commit"))
	decided := e == nil && string(decision) == environmentHash(body)
	if e != nil && !os.IsNotExist(e) {
		return e
	}
	for i, v := range intent.Files {
		dest, e := f.destination(v.Path)
		if e != nil {
			return e
		}
		if filepath.Dir(v.Stage) != filepath.Dir(dest) || !strings.HasPrefix(filepath.Base(v.Stage), ".environment-stage-") {
			return ErrEnvironmentRecoveryRequired
		}
		if !decided {
			_ = environmentOS.Remove(v.Stage)
			continue
		}
		current, e := environmentFileHash(dest)
		if e != nil {
			return e
		}
		if current == v.NewHash {
			continue
		}
		if current != v.OldHash {
			return fmt.Errorf("destination changed outside environment lock")
		}
		stage, e := environmentOS.ReadFile(v.Stage)
		if e != nil || environmentHash(stage) != v.NewHash {
			return ErrEnvironmentRecoveryRequired
		}
		if e = environmentPlainPath(v.Stage); e != nil {
			return e
		}
		if e = environmentOS.Rename(v.Stage, dest); e != nil {
			return e
		}
		if e = syncEnvironmentDir(filepath.Dir(dest)); e != nil {
			return e
		}
		environmentTransactionStep(fmt.Sprintf("rename-%d", i))
	}
	if decided {
		environmentTransactionStep("before-receipt")
		intent.Result.Status = "committed"
		receipt, _ := json.Marshal(intent)
		if e = durableEnvironmentFile(filepath.Join(f.namespace, environmentHash([]byte(intent.Result.OperationID))+".receipt.json"), receipt, 0600); e != nil {
			return e
		}
	}
	if e = environmentOS.Remove(intentPath); e != nil {
		return e
	}
	if e = syncEnvironmentDir(f.namespace); e != nil {
		return e
	}
	if decided {
		if e = environmentOS.Remove(filepath.Join(f.namespace, "commit")); e != nil {
			return e
		}
		return syncEnvironmentDir(f.namespace)
	}
	return nil
}

// Commit writes an immutable intent and commit decision before any canonical replacement.
func (f *EnvironmentFiles) Commit(changes map[string][]byte, binding string, result dto.EnvironmentOperationResult) error {
	return f.CommitChecked(changes, nil, binding, result)
}

// CommitChecked fences exact bytes captured by the mutation's authoritative read.
func (f *EnvironmentFiles) CommitChecked(changes map[string][]byte, expected map[string]string, binding string, result dto.EnvironmentOperationResult) error {
	if f.namespace == "" {
		return os.ErrPermission
	}
	intent := environmentIntent{Binding: binding, Result: result}
	keys := make([]string, 0, len(changes))
	for rel := range changes {
		keys = append(keys, rel)
	}
	sort.Strings(keys)
	for _, rel := range keys {
		body := changes[rel]
		dest, e := f.destination(rel)
		if e != nil {
			return e
		}
		if e = environmentOS.MkdirAll(filepath.Dir(dest), 0755); e != nil {
			return e
		}
		old, e := environmentFileHash(dest)
		if e != nil {
			return e
		}
		if expected != nil {
			want, ok := expected[rel]
			if !ok || want != old {
				return fmt.Errorf("destination changed since environment snapshot")
			}
		}
		stage, e := environmentOS.CreateTemp(filepath.Dir(dest), ".environment-stage-")
		if e != nil {
			return e
		}
		if e = stage.Chmod(0600); e == nil {
			_, e = stage.Write(body)
		}
		if e == nil {
			e = stage.Sync()
		}
		ce := stage.Close()
		if e == nil {
			e = ce
		}
		if e != nil {
			return e
		}
		if e = syncEnvironmentDir(filepath.Dir(dest)); e != nil {
			return e
		}
		intent.Files = append(intent.Files, environmentReplacement{rel, stage.Name(), old, environmentHash(body)})
	}
	environmentTransactionStep("stages")
	// Intent and its receipt codec contain only strings, integers and slices
	// of those primitives, with no caller-supplied raw JSON.
	body, _ := json.Marshal(intent)
	var e error
	if e = durableEnvironmentFile(filepath.Join(f.namespace, "intent.json"), body, 0600); e != nil {
		return e
	}
	environmentTransactionStep("intent")
	if e = durableEnvironmentFile(filepath.Join(f.namespace, "commit"), []byte(environmentHash(body)), 0600); e != nil {
		return e
	}
	environmentTransactionStep("decision")
	return f.recover()
}

// Operation returns private evidence only for the matching verified actor scope.
func (f *EnvironmentFiles) Operation(id, binding string) (*dto.EnvironmentOperationResult, error) {
	if f.namespace == "" {
		return nil, os.ErrNotExist
	}
	b, e := readEnvironmentPrivateFile(filepath.Join(f.namespace, environmentHash([]byte(id))+".receipt.json"))
	if e != nil {
		return nil, e
	}
	var intent environmentIntent
	if e = json.Unmarshal(b, &intent); e != nil {
		return nil, e
	}
	if intent.Binding != binding {
		return nil, os.ErrPermission
	}
	until, err := time.Parse(time.RFC3339Nano, intent.Result.ReplayUntil)
	if err == nil && !time.Now().Before(until) && intent.Result.Status != "expired" {
		intent.Result.Status = "expired"
		intent.Files = nil
		// The same closed primitive-only intent schema is used for receipts.
		body, _ := json.Marshal(intent)
		if err = durableEnvironmentFile(filepath.Join(f.namespace, environmentHash([]byte(id))+".receipt.json"), body, 0600); err != nil {
			return nil, err
		}
	}
	return &intent.Result, nil
}

type environmentLockContextKey struct{}

func withEnvironmentStoreLock(ctx context.Context, root string, fn func(context.Context) error) error {
	if root == "" {
		return datatug.ErrProjectDoesNotExist
	}
	canonical, e := environmentOS.EvalSymlinks(root)
	if e != nil {
		return e
	}
	if ctx.Value(environmentLockContextKey{}) == canonical {
		return fn(ctx)
	}
	return WithEnvironmentFiles(ctx, canonical, "", func(_ *EnvironmentFiles) error {
		return fn(context.WithValue(ctx, environmentLockContextKey{}, canonical))
	})
}
func (f *EnvironmentFiles) LoadEnvironments(ctx context.Context) (datatug.Environments, error) {
	return newFsEnvironmentsStore(f.Root).loadEnvironments(ctx)
}
func (f *EnvironmentFiles) LoadProjectFile() (datatug.ProjectFile, error) {
	return loadProjectFileUnlocked(f.Root)
}

func withEnvironmentStoreReadLock(ctx context.Context, root string, fn func(context.Context) error) error {
	canonical, err := environmentOS.EvalSymlinks(root)
	if err != nil {
		return err
	}
	if ctx.Value(environmentLockContextKey{}) == canonical {
		return fn(ctx)
	}
	return ReadEnvironmentFiles(ctx, canonical, func(_ *EnvironmentFiles) error {
		return fn(context.WithValue(ctx, environmentLockContextKey{}, canonical))
	})
}
