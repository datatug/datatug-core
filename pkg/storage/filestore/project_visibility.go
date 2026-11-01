package filestore

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/datatug/datatug-core/internal/jsonstrict"
	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/dto"
)

// IO and encoding seams keep failure branches testable without privileged filesystem tricks.
var visibilityWalk = filepath.WalkDir
var visibilityRead = readRegularFileBudgeted
var visibilityMarshal = json.Marshal

const visibilityFile = ".datatug-visibility.json"

type projectVisibility struct {
	Version  int                        `json:"version"`
	Archived []datatug.ProjectObjectRef `json:"archived"`
}

var _ datatug.ProjectLifecycleStore = fsProjectStore{}

func readProjectVisibility(root string) (projectVisibility, error) {
	state := projectVisibility{Version: 1}
	b, exists, err := readRegularFileCapped(filepath.Join(root, visibilityFile), 2<<20)
	if err != nil || !exists {
		return state, err
	}
	return decodeProjectVisibility(b)
}

func decodeProjectVisibility(b []byte) (projectVisibility, error) {
	state := projectVisibility{Version: 1}
	trimmed := bytes.TrimSpace(b)
	// Syntax/depth admission precedes recursive duplicate-key traversal. Both
	// readers already require a complete valid JSON document below.
	if len(trimmed) == 0 || trimmed[0] != '{' || !json.Valid(b) {
		return state, datatug.ErrProjectUsageIncomplete
	}
	if jsonstrict.CheckNoDuplicateKeysFor(b, reflect.TypeOf(state)) != nil {
		return state, datatug.ErrProjectUsageIncomplete
	}
	if err := json.Unmarshal(b, &state); err != nil || state.Version != 1 || len(state.Archived) > 10000 {
		return state, datatug.ErrProjectUsageIncomplete
	}
	seen := map[datatug.ProjectObjectRef]bool{}
	for _, ref := range state.Archived {
		if ref.Validate() != nil || seen[ref] {
			return state, datatug.ErrProjectUsageIncomplete
		}
		seen[ref] = true
	}
	return state, nil
}

// ProjectItemArchived is visibility metadata only. Callers must not use it to
// deny direct object access, invalidate saved references, or stop executions.
func ProjectItemArchived(root string, ref datatug.ProjectObjectRef) (bool, error) {
	archived, err := ReadProjectArchivedItems(root)
	return archived[ref], err
}

// ReadProjectArchivedItems returns one bounded visibility snapshot for active
// list projection. It conveys no authorization or execution restriction.
func ReadProjectArchivedItems(root string) (map[datatug.ProjectObjectRef]bool, error) {
	return projectArchivedItems(readProjectVisibility(root))
}

func projectArchivedItems(state projectVisibility, err error) (map[datatug.ProjectObjectRef]bool, error) {
	if err != nil {
		return nil, err
	}
	out := make(map[datatug.ProjectObjectRef]bool, len(state.Archived))
	for _, ref := range state.Archived {
		out[ref] = true
	}
	return out, nil
}

// ProjectFileVisibilityObject recognizes definition layouts, never execution-history
// records. IDs are derived from project-relative layouts, not client paths.
// Runtime records with hashed filenames (such as query validators) must come
// from the owning typed inventory callback, which supplies canonical identities.
func ProjectFileVisibilityObject(rel string) (datatug.ProjectObjectRef, bool) {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	if len(parts) < 2 {
		return datatug.ProjectObjectRef{}, false
	}
	if parts[0] == "dbs" && len(parts) == 3 && strings.HasSuffix(parts[2], ".dbserver.json") {
		ref := datatug.ProjectObjectRef{Kind: "server", ID: parts[1] + "/" + strings.TrimSuffix(parts[2], ".dbserver.json")}
		return ref, ref.Validate() == nil
	}
	if parts[0] == "dbs" && len(parts) == 4 && parts[3] == ".datatug-dbserver.json" {
		ref := datatug.ProjectObjectRef{Kind: "server", ID: parts[1] + "/" + parts[2]}
		return ref, ref.Validate() == nil
	}
	kinds := map[string]string{"queries": "query", "boards": "board", "cards": "card", "entities": "entity", "environments": "environment", "checks": "check", "validators": "validator", "scenarios": "scenario", "dbmodels": "data-model", "data-models": "data-model", "servers": "server", "dbservers": "server"}
	kind := kinds[parts[0]]
	if kind == "" {
		return datatug.ProjectObjectRef{}, false
	}
	name := parts[len(parts)-1]
	if kind == "environment" {
		if len(parts) != 3 || (name != "environment-summary.json" && name != parts[1]+".env.json") {
			return datatug.ProjectObjectRef{}, false
		}
		return datatug.ProjectObjectRef{Kind: kind, ID: parts[1]}, true
	}
	if kind == "query" && name == ".datatug-query-folder.json" {
		if len(parts) < 3 {
			return datatug.ProjectObjectRef{}, false
		}
		return datatug.ProjectObjectRef{Kind: "query-folder", ID: strings.Join(parts[1:len(parts)-1], "/")}, true
	}
	suffix := "." + kind + ".json"
	if kind == "data-model" {
		suffix = ".dbmodel.json"
	}
	if kind == "query" {
		for _, legacy := range []string{".sql.json", ".tugql.json", ".http.json"} {
			if strings.HasSuffix(name, legacy) {
				suffix = legacy
				break
			}
		}
	}
	if kind == "server" && strings.HasSuffix(name, ".dbserver.json") {
		suffix = ".dbserver.json"
	}
	var id string
	if strings.HasSuffix(name, suffix) {
		id = strings.TrimSuffix(name, suffix)
		if kind == "query" && len(parts) > 2 {
			id = strings.Join(append(append([]string{}, parts[1:len(parts)-1]...), id), "/")
		} else if len(parts) > 2 && (len(parts) != 3 || parts[len(parts)-2] != id) {
			return datatug.ProjectObjectRef{}, false
		}
	} else if len(parts) == 2 && (kind == "check" || kind == "scenario" || kind == "validator" || kind == "query-validator") && strings.HasSuffix(name, ".json") {
		id = strings.TrimSuffix(name, ".json")
	} else if len(parts) == 3 && name == kind+".json" {
		id = parts[1]
	} else {
		return datatug.ProjectObjectRef{}, false
	}
	ref := datatug.ProjectObjectRef{Kind: kind, ID: id}
	return ref, ref.Validate() == nil
}

func (s fsProjectStore) visibilityCatalog(ctx context.Context, extra []datatug.ProjectUsageItem) (datatug.ProjectLifecycleCatalog, error) {
	state, err := readProjectVisibility(s.projectPath)
	if err != nil {
		return datatug.ProjectLifecycleCatalog{}, err
	}
	archived := map[datatug.ProjectObjectRef]bool{}
	for _, ref := range state.Archived {
		archived[ref] = true
	}
	items := map[datatug.ProjectObjectRef]datatug.ProjectUsageItem{}
	budget := &queryReadBudget{remaining: 32 << 20, limit: 32 << 20}
	entries := 0
	err = visibilityWalk(s.projectPath, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if p == s.projectPath {
			return nil
		}
		entries++
		if entries > 20000 {
			return datatug.ErrProjectUsageIncomplete
		}
		rel, e := filepath.Rel(s.projectPath, p)
		if e != nil {
			return e
		}
		first := strings.Split(filepath.ToSlash(rel), "/")[0]
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") || d.Name() == "node_modules" {
				return filepath.SkipDir
			}
			if len(strings.Split(filepath.ToSlash(rel), "/")) == 1 {
				switch first {
				case "queries", "boards", "cards", "entities", "environments", "checks", "validators", "scenarios", "dbmodels", "data-models", "servers", "dbservers", "dbs", "connections":
				default:
					return filepath.SkipDir
				}
			}
			return nil
		}
		ref, recognized := ProjectFileVisibilityObject(rel)
		connection := first == "connections" && strings.HasSuffix(d.Name(), ".json")
		if !recognized && !connection {
			return nil
		}
		b, exists, e := visibilityRead(p, 2<<20, budget)
		if e != nil {
			return e
		}
		if !exists {
			return datatug.ErrProjectLifecycleConflict
		}
		var raw map[string]json.RawMessage
		if e = json.Unmarshal(b, &raw); e != nil {
			return datatug.ErrProjectUsageIncomplete
		}
		add := func(ref datatug.ProjectObjectRef, raw map[string]json.RawMessage) error {
			if ref.Validate() != nil {
				return datatug.ErrProjectUsageIncomplete
			}
			var title string
			_ = json.Unmarshal(raw["title"], &title)
			if title == "" {
				_ = json.Unmarshal(raw["name"], &title)
			}
			if len(title) > 1024 {
				return datatug.ErrProjectUsageIncomplete
			}
			if title == "" {
				title = ref.ID
			}
			if prior, ok := items[ref]; ok && prior.Title != title {
				return datatug.ErrProjectUsageIncomplete
			}
			items[ref] = datatug.ProjectUsageItem{Ref: ref, Title: title, Archived: archived[ref]}
			if len(items) > 10000 {
				return datatug.ErrProjectUsageIncomplete
			}
			return nil
		}
		if recognized {
			if ref.Kind == "query" || ref.Kind == "query-folder" {
				for at := strings.LastIndex(ref.ID, "/"); at >= 0; at = strings.LastIndex(ref.ID[:at], "/") {
					id := ref.ID[:at]
					folder := datatug.ProjectObjectRef{Kind: "query-folder", ID: id}
					if _, exists := items[folder]; !exists {
						if e = add(folder, map[string]json.RawMessage{}); e != nil {
							return e
						}
					}
				}
			}
			return add(ref, raw)
		}
		for _, key := range []string{"connections", "bigQueryEditions", "bigQueryPlans"} {
			var connections []map[string]json.RawMessage
			if len(raw[key]) > 0 {
				if e = json.Unmarshal(raw[key], &connections); e != nil {
					return datatug.ErrProjectUsageIncomplete
				}
				for _, v := range connections {
					var id string
					_ = json.Unmarshal(v["id"], &id)
					if e = add(datatug.ProjectObjectRef{Kind: "connection", ID: id}, v); e != nil {
						return e
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		return datatug.ProjectLifecycleCatalog{}, err
	}
	{
		for _, item := range extra {
			if item.Ref.Validate() != nil || item.Restricted || item.Historical || len(item.Title) > 1024 {
				return datatug.ProjectLifecycleCatalog{}, datatug.ErrProjectUsageIncomplete
			}
			if item.Title == "" {
				item.Title = item.Ref.ID
			}
			if prior, exists := items[item.Ref]; exists && prior.Title != item.Title {
				return datatug.ProjectLifecycleCatalog{}, datatug.ErrProjectUsageIncomplete
			}
			item.Archived = archived[item.Ref]
			items[item.Ref] = item
			if len(items) > 10000 {
				return datatug.ProjectLifecycleCatalog{}, datatug.ErrProjectUsageIncomplete
			}
		}
	}
	out := datatug.ProjectLifecycleCatalog{ArchiveSupported: true, Items: make([]datatug.ProjectUsageItem, 0, len(items))}
	for _, item := range items {
		out.Items = append(out.Items, item)
	}
	sort.Slice(out.Items, func(i, j int) bool {
		a, b := out.Items[i].Ref, out.Items[j].Ref
		return a.Kind < b.Kind || (a.Kind == b.Kind && a.ID < b.ID)
	})
	body, err := visibilityMarshal(out.Items)
	if err != nil {
		return out, err
	}
	h := sha256.Sum256(body)
	out.Revision = hex.EncodeToString(h[:])
	return out, nil
}

func (s fsProjectStore) ListProjectLifecycle(ctx context.Context, scope datatug.ProjectLifecycleScope, includeArchived bool) (catalog datatug.ProjectLifecycleCatalog, err error) {
	if err = s.visibilityScope(scope); err != nil {
		return catalog, err
	}
	extra, err := s.runtimeVisibilityItems(ctx, scope)
	if err != nil {
		return catalog, err
	}
	err = ReadEnvironmentFiles(ctx, s.projectPath, func(_ *EnvironmentFiles) error { catalog, err = s.visibilityCatalog(ctx, extra); return err })
	if err == nil && !includeArchived {
		visible := catalog.Items[:0]
		for _, item := range catalog.Items {
			if !item.Archived {
				visible = append(visible, item)
			}
		}
		catalog.Items = visible
	}
	return catalog, err
}

func (s fsProjectStore) ReviewProjectLifecycle(ctx context.Context, scope datatug.ProjectLifecycleScope, target datatug.ProjectObjectRef) (review datatug.ProjectLifecycleReview, err error) {
	if err = s.visibilityScope(scope); err != nil {
		return review, err
	}
	extra, err := s.runtimeVisibilityItems(ctx, scope)
	if err != nil {
		return review, err
	}
	err = ReadEnvironmentFiles(ctx, s.projectPath, func(_ *EnvironmentFiles) error {
		catalog, e := s.visibilityCatalog(ctx, extra)
		if e != nil {
			return e
		}
		review, e = datatug.ReviewProjectVisibility(catalog, target)
		return e
	})
	return review, err
}

func (s fsProjectStore) visibilityScope(scope datatug.ProjectLifecycleScope) error {
	if strings.TrimSpace(scope.ActorID) == "" || scope.Project != s.projectID || scope.SpaceID != "" || scope.Branch != "" || scope.Storage == "" || s.projectPath == "" {
		return datatug.ErrProjectLifecycleInvalid
	}
	return nil
}

func (s fsProjectStore) ChangeProjectLifecycle(ctx context.Context, scope datatug.ProjectLifecycleScope, write datatug.ProjectLifecycleWrite) (ack datatug.ProjectLifecycleAck, err error) {
	if err = s.visibilityScope(scope); err != nil {
		return ack, err
	}
	if err = write.Validate(); err != nil {
		return ack, err
	}
	if write.Change.Action == "delete" {
		return ack, datatug.ErrProjectUsageIncomplete
	}
	extra, err := s.runtimeVisibilityItems(ctx, scope)
	if err != nil {
		return ack, err
	}
	err = WithEnvironmentFiles(ctx, s.projectPath, "", func(files *EnvironmentFiles) error {
		bindingBytes, _ := visibilityMarshal(scope)
		binding := environmentHash(bindingBytes)
		requestBytes, _ := visibilityMarshal(write)
		digest := environmentHash(requestBytes)
		opID := "visibility-" + environmentHash([]byte(binding+"/"+write.OperationID))
		prior, e := files.Operation(opID, binding)
		if e == nil {
			if prior.PayloadDigest != digest || prior.Status == "expired" {
				return datatug.ErrProjectLifecycleConflict
			}
			ack = datatug.ProjectLifecycleAck{OperationID: write.OperationID, Revision: prior.AfterStateDigest, Change: write.Change}
			return nil
		}
		if !errors.Is(e, os.ErrNotExist) {
			return e
		}
		catalog, e := s.visibilityCatalog(ctx, extra)
		if e != nil {
			return e
		}
		if _, e = datatug.PlanProjectVisibility(catalog, write); e != nil {
			return e
		}
		state, e := readProjectVisibility(s.projectPath)
		if e != nil {
			return e
		}
		refs := state.Archived[:0]
		for _, ref := range state.Archived {
			if ref != write.Change.Object {
				refs = append(refs, ref)
			}
		}
		if write.Change.Action == "archive" {
			refs = append(refs, write.Change.Object)
		}
		state.Archived = refs
		sort.Slice(state.Archived, func(i, j int) bool {
			a, b := state.Archived[i], state.Archived[j]
			return a.Kind < b.Kind || (a.Kind == b.Kind && a.ID < b.ID)
		})
		body, e := visibilityMarshal(state)
		if e != nil {
			return e
		}
		for i := range catalog.Items {
			if catalog.Items[i].Ref == write.Change.Object {
				catalog.Items[i].Archived = write.Change.Action == "archive"
			}
		}
		// Visibility revisions bind the same sorted identity/title/state catalog
		// returned by subsequent reads. Source bodies and references stay intact.
		catalogBody, e := visibilityMarshal(catalog.Items)
		if e != nil {
			return e
		}
		revision := environmentHash(catalogBody)
		before, e := environmentFileHash(filepath.Join(s.projectPath, visibilityFile))
		if e != nil {
			return e
		}
		now := time.Now().UTC()
		result := dto.EnvironmentOperationResult{OperationID: opID, Kind: write.Change.Action, PayloadDigest: digest, AcceptedAt: now.Format(time.RFC3339Nano), ReplayUntil: now.Add(30 * 24 * time.Hour).Format(time.RFC3339Nano), AfterStateDigest: revision, Status: "succeeded"}
		if e = files.CommitChecked(map[string][]byte{visibilityFile: body}, map[string]string{visibilityFile: before}, binding, result); e != nil {
			return e
		}
		ack = datatug.ProjectLifecycleAck{OperationID: write.OperationID, Revision: revision, Change: write.Change}
		return nil
	})
	return ack, err
}

// Enumerate before taking the filesystem lock: host callbacks can use public
// project/environment readers. These are current visibility targets, never a
// deletion/reference proof. The host reauthorizes the actor on every request.
func (s fsProjectStore) runtimeVisibilityItems(ctx context.Context, scope datatug.ProjectLifecycleScope) ([]datatug.ProjectUsageItem, error) {
	if s.visibilityItems == nil {
		return nil, nil
	}
	return s.visibilityItems(ctx, scope)
}
