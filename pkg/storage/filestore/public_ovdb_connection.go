package filestore

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/datatug/datatug-core/internal/jsonstrict"
	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/dto"
)

// PublicOVDBConnectionSnapshot contains only safe metadata. It neither opens a
// source nor grants execution. Use only inside the owning EnvironmentFiles callback.
type PublicOVDBConnectionSnapshot struct {
	EnvironmentID, DatabaseID, ConnectionID, Revision string
	Exists                                            bool
}

type publicOVDBConnectionRead struct {
	snapshot    PublicOVDBConnectionSnapshot
	database    datatug.ProjectDatabase
	environment datatug.Environment
	files       map[string][]byte
	envFiles    []string
}

type connectionDirectoryReader interface {
	ReadDir(int) ([]os.DirEntry, error)
	Close() error
}

var openConnectionDirectory = func(name string) (connectionDirectoryReader, error) { return os.Open(name) }
var buildPublicConnectionChanges = publicOVDBConnectionChanges

func (f *EnvironmentFiles) PublicOVDBConnectionState(ctx context.Context, environmentID, databaseID string) (PublicOVDBConnectionSnapshot, error) {
	read, err := f.readPublicOVDBConnection(ctx, environmentID, databaseID)
	return read.snapshot, err
}

func (f *EnvironmentFiles) readPublicOVDBConnection(ctx context.Context, environmentID, databaseID string) (out publicOVDBConnectionRead, err error) {
	if ctx == nil || !datatug.ValidConnectionID(environmentID) || !datatug.ValidConnectionID(databaseID) {
		return out, datatug.ErrProjectLifecycleInvalid
	}
	if err = ctx.Err(); err != nil {
		return out, err
	}
	out.files = map[string][]byte{}
	remaining := 4 << 20
	readFile := func(rel string) ([]byte, bool, error) {
		if _, e := f.destination(rel); e != nil {
			return nil, false, e
		}
		limit := int64(1 << 20)
		if strings.HasPrefix(rel, "connections/") {
			limit = datatug.MaxPublicConnectionCatalogBytes
		}
		body, exists, e := readRegularFileCapped(filepath.Join(f.Root, rel), limit)
		if e != nil || !exists {
			return body, exists, e
		}
		remaining -= len(body)
		if remaining < 0 {
			return nil, false, datatug.ErrProjectUsageIncomplete
		}
		out.files[rel] = body
		return body, true, nil
	}
	body, exists, err := readFile("datatug-project.json")
	if err != nil {
		return out, err
	}
	if !exists {
		return out, os.ErrNotExist
	}
	var project datatug.ProjectFile
	if err = jsonstrict.CheckNoDuplicateKeysFor(body, reflect.TypeOf(project)); err != nil {
		return out, err
	}
	if err = json.Unmarshal(body, &project); err != nil {
		return out, err
	}
	if err = project.Databases.Validate(); err != nil {
		return out, err
	}
	for _, database := range project.Databases {
		if database.ID == databaseID {
			out.database = database
		}
	}
	if out.database.ID == "" {
		return out, os.ErrNotExist
	}
	out.snapshot = PublicOVDBConnectionSnapshot{EnvironmentID: environmentID, DatabaseID: databaseID, ConnectionID: out.database.ConnectionID}
	// This physical filename belongs permanently to the separate log contract.
	out.snapshot.Exists = strings.EqualFold(out.database.ConnectionID+".json", datatug.ProjectLogSourcesFile)
	for _, name := range []string{environmentID + ".env.json", "environment-summary.json"} {
		rel := "environments/" + environmentID + "/" + name
		body, exists, e := readFile(rel)
		if e != nil {
			return out, e
		}
		if !exists {
			continue
		}
		var env datatug.Environment
		var fields map[string]json.RawMessage
		if e = json.Unmarshal(body, &fields); e != nil || fields == nil {
			return out, ErrEnvironmentRecoveryRequired
		}
		if e = jsonstrict.CheckNoDuplicateKeysForSchema(body, reflect.TypeOf(connectionEnvironmentSchema{})); e != nil {
			return out, e
		}
		if e = json.Unmarshal(body, &env); e != nil {
			return out, e
		}
		if env.ID != "" && env.ID != environmentID {
			return out, datatug.ErrProjectLifecycleInvalid
		}
		env.SetID(environmentID)
		if e = env.Validate(); e != nil {
			return out, e
		}
		if len(out.envFiles) > 0 && !reflect.DeepEqual(out.environment, env) {
			return out, ErrEnvironmentRecoveryRequired
		}
		out.environment = env
		out.envFiles = append(out.envFiles, rel)
	}
	if len(out.envFiles) == 0 {
		return out, os.ErrNotExist
	}
	for _, id := range out.environment.EditionConnections {
		if strings.EqualFold(id, out.snapshot.ConnectionID) {
			out.snapshot.Exists = true
		}
	}
	dir := filepath.Join(f.Root, "connections")
	if info, e := dirLstat(dir); e == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return out, ErrEnvironmentRecoveryRequired
		}
		opened, e := openConnectionDirectory(dir)
		if e != nil {
			return out, e
		}
		entries, e := opened.ReadDir(1025)
		closeErr := opened.Close()
		if e != nil && !errors.Is(e, io.EOF) {
			return out, e
		}
		if closeErr != nil {
			return out, closeErr
		}
		if len(entries) > 1024 {
			return out, datatug.ErrProjectUsageIncomplete
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		count := 0
		ids := map[string]bool{}
		for _, entry := range entries {
			if err = ctx.Err(); err != nil {
				return out, err
			}
			name := entry.Name()
			if strings.EqualFold(name, out.snapshot.ConnectionID+".json") {
				out.snapshot.Exists = true
			}
			if !strings.HasSuffix(name, ".json") {
				continue
			}
			count++
			if count > maxConnectionCatalogs || !datatug.ValidConnectionID(strings.TrimSuffix(name, ".json")) {
				return out, datatug.ErrProjectUsageIncomplete
			}
			body, exists, e := readFile("connections/" + name)
			if e != nil {
				return out, e
			}
			if !exists {
				return out, ErrEnvironmentRecoveryRequired
			}
			catalog, isCatalog, e := datatug.ParsePublicProjectConnectionDirectoryJSON(name, body)
			if e != nil {
				return out, e
			}
			if !isCatalog {
				continue
			} // retained and charged; separate validated namespace
			all := []string{}
			for _, item := range catalog.Connections {
				all = append(all, item.ID)
			}
			for _, item := range catalog.BigQueryEditions {
				all = append(all, item.ID)
			}
			for _, item := range catalog.BigQueryPlans {
				all = append(all, item.ID)
			}
			for _, id := range all {
				key := strings.ToLower(id)
				if ids[key] {
					return out, datatug.ErrAmbiguousConnection
				}
				ids[key] = true
				if strings.EqualFold(id, out.snapshot.ConnectionID) {
					out.snapshot.Exists = true
				}
			}
			if strings.EqualFold(name, out.snapshot.ConnectionID+".json") {
				out.snapshot.Exists = true
			}
		}
		if count >= maxConnectionCatalogs && !out.snapshot.Exists {
			return out, datatug.ErrProjectUsageIncomplete
		}
		if len(entries) >= 1024 && !out.snapshot.Exists {
			return out, datatug.ErrProjectUsageIncomplete
		}
	} else if !os.IsNotExist(e) {
		return out, e
	}
	// Hash full bounded source bytes privately, including opaque metadata.
	out.snapshot.Revision = publicOVDBSnapshotRevision(environmentID, databaseID, out.files)
	if !out.snapshot.Exists {
		if _, _, err = buildPublicConnectionChanges(out); err != nil {
			return out, err
		}
	}
	return out, ctx.Err()
}

func publicOVDBSnapshotRevision(environmentID, databaseID string, files map[string][]byte) string {
	fingerprints := map[string]string{}
	for name, body := range files {
		fingerprints[name] = environmentHash(body)
	}
	encoded, _ := json.Marshal(struct {
		Environment, Database string
		Files                 map[string]string
	}{environmentID, databaseID, fingerprints})
	return environmentHash(encoded)
}

// CommitPublicOVDBConnection creates a fresh declaration and both sides of its
// exact environment association in the existing durable multi-file journal.
// Existing catalogs and opaque JSON members are never replaced.
func (f *EnvironmentFiles) CommitPublicOVDBConnection(ctx context.Context, observed PublicOVDBConnectionSnapshot, binding string, result *dto.EnvironmentOperationResult) error {
	return f.CommitPublicOVDBConnectionGuarded(ctx, observed, binding, result, nil)
}

// CommitPublicOVDBConnectionGuarded rechecks source bytes and serving authority
// after staging, immediately before the durable commit decision. A failed guard
// rolls the undecided intent back without changing canonical project files.
func (f *EnvironmentFiles) CommitPublicOVDBConnectionGuarded(ctx context.Context, observed PublicOVDBConnectionSnapshot, binding string, result *dto.EnvironmentOperationResult, authorize func() error) error {
	if result == nil {
		return datatug.ErrProjectLifecycleInvalid
	}
	current, err := f.readPublicOVDBConnection(ctx, observed.EnvironmentID, observed.DatabaseID)
	if err != nil {
		return err
	}
	if current.snapshot != observed {
		return datatug.ErrProjectLifecycleConflict
	}
	if observed.Exists {
		return datatug.ErrAmbiguousConnection
	}
	changes, expected, err := buildPublicConnectionChanges(current)
	if err != nil {
		return err
	}
	for path, body := range changes {
		current.files[path] = body
	}
	result.AfterStateDigest = publicOVDBSnapshotRevision(observed.EnvironmentID, observed.DatabaseID, current.files)
	if err = ctx.Err(); err != nil {
		return err
	}
	return f.commitChecked(changes, expected, binding, *result, func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		latest, err := f.readPublicOVDBConnection(ctx, observed.EnvironmentID, observed.DatabaseID)
		if err != nil {
			return err
		}
		if latest.snapshot != observed {
			return datatug.ErrProjectLifecycleConflict
		}
		if authorize != nil {
			return authorize()
		}
		return nil
	})
}

func publicOVDBConnectionChanges(current publicOVDBConnectionRead) (map[string][]byte, map[string]string, error) {
	observed := current.snapshot
	connection := struct {
		datatug.EditionConnection
		Source string                     `json:"source"`
		OVDB   datatug.PublicOVDBDatabase `json:"ovdb"`
	}{
		EditionConnection: datatug.EditionConnection{ID: observed.ConnectionID, Dataset: current.database.OVDB.DatabaseID, Storage: "ovdb", Environments: []string{observed.EnvironmentID}, Readiness: "public-api"},
		Source:            strings.TrimRight(current.database.OVDB.BaseURL, "/") + "/v1/databases/" + current.database.OVDB.DatabaseID,
		OVDB:              current.database.OVDB,
	}
	if err := connection.Validate(); err != nil {
		return nil, nil, err
	}
	body, _ := json.MarshalIndent(struct {
		Format      string `json:"format"`
		Connections []any  `json:"connections"`
	}{datatug.ConnectionCatalogFormat, []any{connection}}, "", "\t")
	if _, err := datatug.ParsePublicProjectConnectionsJSON(body); err != nil {
		return nil, nil, err
	}
	rel := "connections/" + observed.ConnectionID + ".json"
	changes := map[string][]byte{rel: body}
	expected := map[string]string{rel: "absent"}
	for _, path := range current.envFiles {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(current.files[path], &fields); err != nil || fields == nil {
			return nil, nil, ErrEnvironmentRecoveryRequired
		}
		fields["editionConnections"], _ = json.Marshal(append(append([]string{}, current.environment.EditionConnections...), observed.ConnectionID))
		changes[path], _ = json.MarshalIndent(fields, "", "\t")
		expected[path] = environmentHash(current.files[path])
	}
	// Reserve prospective output capacity before advertising the write. Opaque
	// JSON may expand during indentation; never commit an unreadable new state.
	if err := validatePublicConnectionChangeCapacity(current.files, changes); err != nil {
		return nil, nil, err
	}
	return changes, expected, nil
}

func validatePublicConnectionChangeCapacity(files, changes map[string][]byte) error {
	bytes := 0
	for path, body := range files {
		if replacement, ok := changes[path]; ok {
			body = replacement
		}
		if len(body) > 1<<20 {
			return datatug.ErrProjectUsageIncomplete
		}
		bytes += len(body)
	}
	for path, body := range changes {
		if len(body) > 1<<20 {
			return datatug.ErrProjectUsageIncomplete
		}
		if _, exists := files[path]; !exists {
			bytes += len(body)
		}
	}
	if bytes > 4<<20 {
		return datatug.ErrProjectUsageIncomplete
	}
	return nil
}
