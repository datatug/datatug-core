package sessioncontext

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/datatug/datatug-core/internal/jsonstrict"
	"github.com/datatug/datatug-core/pkg/apicontract"
)

const (
	Sessions      = "sessionContextSessions"
	Bags          = "sessionContextBags"
	Entries       = "sessionContextEntries"
	Revisions     = "sessionContextEntryRevisions"
	ContextSets   = "sessionContextSets"
	Operations    = "sessionContextOperations"
	Audit         = "sessionContextAudit"
	Inputs        = "sessionContextInputs"
	Runs          = "sessionContextRuns"
	Messages      = "sessionContextMessages"
	Outbox        = "sessionContextOutbox"
	LegacyImports = "sessionContextLegacyImports"
	// MaxSnapshotBytes caps the sum of serialized session, bag, context-set, current
	// entry and current/applied revision records returned by GetSnapshot.
	MaxSnapshotBytes = 4 * 1024 * 1024
	// MaxSnapshotEntries bounds current entries and applied references in one snapshot.
	MaxSnapshotEntries = 400
	// MaxSnapshotRevisions bounds the union of current and applied revisions.
	MaxSnapshotRevisions = 2 * MaxSnapshotEntries
)

var safeID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$`)
var errSnapshotLimit = errors.New("session context: snapshot limit exceeded")

func normalizedAlias(alias string) string { return strings.ToLower(strings.TrimSpace(alias)) }
func validAlias(alias string) bool {
	return len(alias) <= 128 && (alias == "" || normalizedAlias(alias) != "")
}

func legacyHash(values []string) string {
	// []string has no JSON marshaling failure mode; keep the JSON-array encoding
	// stable because this digest also determines imported resource IDs.
	encoded, _ := json.Marshal(values)
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:])
}

func prepareLegacyImport(p Principal, payload *LegacyImport) (LegacyImportManifest, []string, []string, error) {
	if payload == nil || !safeID.MatchString(payload.SourceID) || !validScope(&payload.Scope) || len(payload.Title) > 256 || len(payload.Entries) > MaxSnapshotEntries || len(payload.Messages) > 1000 {
		return LegacyImportManifest{}, nil, nil, ErrInvalid
	}
	// Firestore permits at most 500 writes in one transaction. Each entry
	// requires an immutable revision plus a current entry record; the remainder
	// accounts for session, bag, context set, manifest, audit, and receipt.
	if 2*len(payload.Entries)+len(payload.Messages)+6 > 500 {
		return LegacyImportManifest{}, nil, nil, ErrInvalid
	}
	if strings.TrimSpace(payload.Title) == "" {
		payload.Title = "Imported session"
	}
	entryIDs := map[string]bool{}
	aliases := map[string]bool{}
	for _, entry := range payload.Entries {
		if !safeID.MatchString(entry.ID) || entryIDs[entry.ID] || !validAlias(entry.Alias) || entry.Subject.ConceptID == "" || entry.Subject.FieldID == "" || entry.Subject.FieldMappingRef == "" || validateValue(entry.Value) != nil {
			return LegacyImportManifest{}, nil, nil, ErrInvalid
		}
		entryIDs[entry.ID] = true
		if alias := normalizedAlias(entry.Alias); alias != "" {
			if aliases[alias] {
				return LegacyImportManifest{}, nil, nil, ErrInvalid
			}
			aliases[alias] = true
		}
	}
	messageIDs := map[string]bool{}
	for _, message := range payload.Messages {
		if !safeID.MatchString(message.ID) || messageIDs[message.ID] || (message.Role != "user" && message.Role != "assistant") || len(message.Text) > 100000 {
			return LegacyImportManifest{}, nil, nil, ErrInvalid
		}
		messageIDs[message.ID] = true
	}
	encoded, err := json.Marshal(payload)
	if err != nil || len(encoded) > MaxSnapshotBytes {
		return LegacyImportManifest{}, nil, nil, ErrInvalid
	}
	importDigest := sha256.Sum256(encoded)
	identity := legacyHash([]string{p.Actor, p.DatabaseID, p.ProjectID, payload.Scope.EnvironmentID, payload.Scope.IdentityDomain, payload.SourceID})
	manifestID := "li_" + identity
	sessionID := "legacy_s_" + identity
	bagID := "legacy_b_" + identity
	entryTargetIDs := make([]string, len(payload.Entries))
	for i, entry := range payload.Entries {
		digest := sha256.Sum256([]byte(identity + "\x00" + entry.ID))
		entryTargetIDs[i] = "legacy_e_" + hex.EncodeToString(digest[:])
	}
	messageTargetIDs := make([]string, len(payload.Messages))
	for i, message := range payload.Messages {
		digest := sha256.Sum256([]byte(identity + "\x00" + message.ID))
		messageTargetIDs[i] = fmt.Sprintf("legacy_m_%04d_%s", i, hex.EncodeToString(digest[:]))
	}
	manifest := LegacyImportManifest{ID: manifestID, SessionID: sessionID, BagID: bagID, SourceID: payload.SourceID, SourceDigest: hex.EncodeToString(importDigest[:]), Scope: payload.Scope}
	return manifest, entryTargetIDs, messageTargetIDs, nil
}

func validateResolvedParameters(parameters map[string]apicontract.TypedValue) error {
	if len(parameters) > 100 {
		return ErrInvalid
	}
	for name, value := range parameters {
		if len(name) == 0 || len(name) > 128 || !safeID.MatchString(name) || value.Validate() != nil {
			return ErrInvalid
		}
	}
	return nil
}

func canonicalDefinitionDocument(expectedKind string, raw json.RawMessage) (json.RawMessage, string, error) {
	if len(raw) == 0 || len(raw) > 128*1024 || !json.Valid(raw) {
		return nil, "", ErrInvalid
	}
	var compact bytes.Buffer
	// json.Valid above makes Compact's syntax error impossible, and bytes.Buffer
	// cannot fail as a writer. Keep this single normalization pass as the compact
	// representation used for the immutable definition digest.
	_ = json.Compact(&compact, raw)
	type queryParameter struct {
		ID         string `json:"id"`
		Type       string `json:"type"`
		IsRequired bool   `json:"isRequired,omitempty"`
	}
	type queryDefinition struct {
		ID           string           `json:"id"`
		QueryType    string           `json:"queryType"`
		Text         string           `json:"text"`
		ConnectionID string           `json:"connectionId,omitempty"`
		Parameters   []queryParameter `json:"parameters,omitempty"`
	}
	type widgetDefinition struct {
		ID      string `json:"id"`
		QueryID string `json:"queryId"`
	}
	var document struct {
		SchemaVersion int                `json:"schemaVersion"`
		Kind          string             `json:"kind"`
		Queries       []queryDefinition  `json:"queries"`
		Widgets       []widgetDefinition `json:"widgets,omitempty"`
	}
	if err := jsonstrict.CheckNoDuplicateKeysFor(compact.Bytes(), reflect.TypeOf(document)); err != nil {
		return nil, "", ErrInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(compact.Bytes()))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&document); err != nil || document.SchemaVersion != 1 || document.Kind != expectedKind || len(document.Queries) > 100 || len(document.Widgets) > 100 {
		return nil, "", ErrInvalid
	}
	if expectedKind == "query" && len(document.Queries) != 1 || expectedKind == "dashboard" && len(document.Widgets) == 0 || expectedKind == "chat" && len(document.Widgets) != 0 {
		return nil, "", ErrInvalid
	}
	queryIDs := make(map[string]bool, len(document.Queries))
	for _, query := range document.Queries {
		if !safeID.MatchString(query.ID) || queryIDs[query.ID] || len(query.Text) == 0 || len(query.Text) > 100000 || query.ConnectionID != "" && !safeID.MatchString(query.ConnectionID) || len(query.Parameters) > 100 {
			return nil, "", ErrInvalid
		}
		if query.QueryType != "SQL" && query.QueryType != "TugQL" {
			return nil, "", ErrInvalid
		}
		queryIDs[query.ID] = true
		seenParameters := map[string]bool{}
		for _, parameter := range query.Parameters {
			if !safeID.MatchString(parameter.ID) || seenParameters[parameter.ID] || !validParameterType(parameter.Type) {
				return nil, "", ErrInvalid
			}
			seenParameters[parameter.ID] = true
		}
	}
	seenWidgets := map[string]bool{}
	for _, widget := range document.Widgets {
		if !safeID.MatchString(widget.ID) || seenWidgets[widget.ID] || !queryIDs[widget.QueryID] {
			return nil, "", ErrInvalid
		}
		seenWidgets[widget.ID] = true
	}
	digest := sha256.Sum256(compact.Bytes())
	return append(json.RawMessage(nil), compact.Bytes()...), hex.EncodeToString(digest[:]), nil
}

func validParameterType(value string) bool {
	switch value {
	case "string", "number", "integer", "decimal", "boolean", "date", "datetime":
		return true
	default:
		return false
	}
}

func settlePendingRun(session *Session) error {
	if session.PendingRunCount <= 0 {
		return ErrUnavailable
	}
	session.PendingRunCount--
	return nil
}

// Service requires a live authorization boundary. It never executes source queries on reads.
type Service struct {
	repo            Repository
	authorize       Authorizer
	now             func() time.Time
	nativeNamespace *NativeNamespace
}

func NewService(repo Repository, authorize Authorizer) (*Service, error) {
	if repo == nil || authorize == nil {
		return nil, ErrInvalid
	}
	return &Service{repo: repo, authorize: authorize, now: func() time.Time { return time.Now().UTC() }}, nil
}
func (s *Service) allow(ctx context.Context, p Principal, action, id string, scope *Scope) error {
	if identity, ok := s.repo.(interface{ DatabaseID() string }); ok && identity.DatabaseID() != p.DatabaseID {
		return ErrForbidden
	}
	if p.Actor == "" || !safeID.MatchString(p.DatabaseID) || p.ProjectID == "" {
		return ErrForbidden
	}
	if err := s.authorize(ctx, Access{Principal: p, Action: action, SessionID: id, Scope: scope}); err != nil {
		return fmt.Errorf("%w: authorization unavailable or denied", ErrForbidden)
	}
	return nil
}
func (s *Service) session(ctx context.Context, r Reader, p Principal, id, action string) (Session, error) {
	var v Session
	version, err := r.Load(ctx, Sessions, id, &v)
	if err != nil {
		return v, err
	}
	if v.ID != id || v.Owner != p.Actor || v.DatabaseID != p.DatabaseID || v.ProjectID != p.ProjectID {
		return Session{}, ErrForbidden
	}
	if version != v.Version || v.SchemaVersion != 1 {
		return Session{}, ErrUnavailable
	}
	if err = guardSessionRedaction(ctx, r, id); err != nil {
		return Session{}, err
	}
	return v, s.allow(ctx, p, action, id, nil)
}
func (s *Service) bag(ctx context.Context, r Reader, p Principal, session Session, id, action string) (Bag, error) {
	var b Bag
	version, err := r.Load(ctx, Bags, id, &b)
	if err != nil {
		return b, err
	}
	if b.ID != id || b.SessionID != session.ID || !contains(session.BagIDs, id) {
		return Bag{}, ErrForbidden
	}
	if version != b.Version {
		return Bag{}, ErrUnavailable
	}
	return b, s.allow(ctx, p, action, session.ID, &b.Scope)
}
func (s *Service) GetSnapshot(ctx context.Context, p Principal, id string) (Snapshot, error) {
	out := Snapshot{Bags: []Bag{}, Entries: []Entry{}, Revisions: []EntryRevision{}}
	err := s.repo.View(ctx, func(r Reader) error {
		v, e := s.session(ctx, r, p, id, "read")
		if e != nil {
			return e
		}
		out, e = s.loadSnapshot(ctx, r, p, v, "read")
		if errors.Is(e, errSnapshotLimit) {
			return ErrUnavailable
		}
		return e
	})
	if err != nil {
		return Snapshot{}, err
	}
	return out, nil
}

// loadSnapshot bounds the full read model that the CLI and dashboard fetch in one
// request. The same validation runs inside commands that can grow this model, so
// accepted writes cannot make an otherwise valid session unreadable.
func (s *Service) loadSnapshot(ctx context.Context, r Reader, p Principal, v Session, action string) (Snapshot, error) {
	out := Snapshot{Session: v, Bags: []Bag{}, Entries: []Entry{}, Revisions: []EntryRevision{}}
	if len(v.BagIDs) > 64 {
		return Snapshot{}, errSnapshotLimit
	}
	used, err := addSnapshotBytes(0, v)
	if err != nil {
		return Snapshot{}, errSnapshotLimit
	}
	_, err = r.Load(ctx, ContextSets, v.ContextSetID, &out.ContextSet)
	if err != nil {
		return Snapshot{}, err
	}
	if out.ContextSet.ID != v.ContextSetID || out.ContextSet.SessionID != v.ID {
		return Snapshot{}, ErrUnavailable
	}
	if len(out.ContextSet.Slots) > 64 {
		return Snapshot{}, errSnapshotLimit
	}
	used, err = addSnapshotBytes(used, out.ContextSet)
	if err != nil {
		return Snapshot{}, errSnapshotLimit
	}
	bags := map[string]Bag{}
	entries := map[string]Entry{}
	revisions := map[string]EntryRevision{}
	loadRevision := func(entry Entry, id string) error {
		if !safeID.MatchString(id) {
			return ErrUnavailable
		}
		revision, loaded := revisions[id]
		if !loaded {
			if _, err := r.Load(ctx, Revisions, id, &revision); err != nil {
				return err
			}
		}
		if revision.ID != id || revision.EntryID != entry.ID || revision.BagID != entry.BagID {
			return ErrUnavailable
		}
		if !loaded {
			var err error
			used, err = addSnapshotBytes(used, revision)
			if err != nil {
				return errSnapshotLimit
			}
			revisions[id] = revision
			out.Revisions = append(out.Revisions, revision)
		}
		return nil
	}
	for _, bid := range v.BagIDs {
		b, err := s.bag(ctx, r, p, v, bid, action)
		if err != nil {
			return Snapshot{}, err
		}
		used, err = addSnapshotBytes(used, b)
		if err != nil {
			return Snapshot{}, errSnapshotLimit
		}
		out.Bags = append(out.Bags, b)
		bags[b.ID] = b
		for _, eid := range b.EntryIDs {
			if len(out.Entries) >= MaxSnapshotEntries {
				return Snapshot{}, errSnapshotLimit
			}
			var entry Entry
			if _, err = r.Load(ctx, Entries, eid, &entry); err != nil {
				return Snapshot{}, err
			}
			if entry.ID != eid || entry.BagID != b.ID || entry.SessionID != v.ID {
				return Snapshot{}, ErrUnavailable
			}
			used, err = addSnapshotBytes(used, entry)
			if err != nil {
				return Snapshot{}, errSnapshotLimit
			}
			if err := loadRevision(entry, entry.CurrentRevisionID); err != nil {
				return Snapshot{}, err
			}
			out.Entries = append(out.Entries, entry)
			entries[eid] = entry
		}
	}
	seenScopes := map[Scope]bool{}
	appliedCount := 0
	for _, slot := range out.ContextSet.Slots {
		if !validScope(&slot.Scope) || seenScopes[slot.Scope] || slot.Version <= 0 {
			return Snapshot{}, ErrUnavailable
		}
		seenScopes[slot.Scope] = true
		if slot.BagID == "" {
			if len(slot.Entries) != 0 || slot.BagVersion != 0 {
				return Snapshot{}, ErrUnavailable
			}
			continue
		}
		bag, ok := bags[slot.BagID]
		if !ok || bag.Scope != slot.Scope || slot.BagVersion <= 0 || slot.BagVersion > bag.Version || slot.BagVersion < bag.ClearVersion {
			return Snapshot{}, ErrUnavailable
		}
		if len(slot.Entries) > MaxSnapshotEntries-appliedCount {
			return Snapshot{}, errSnapshotLimit
		}
		appliedCount += len(slot.Entries)
		seenEntries := map[string]bool{}
		for _, ref := range slot.Entries {
			entry, ok := entries[ref.EntryID]
			if !safeID.MatchString(ref.EntryID) || !ok || entry.BagID != bag.ID || seenEntries[ref.EntryID] {
				return Snapshot{}, ErrUnavailable
			}
			seenEntries[ref.EntryID] = true
			if err := loadRevision(entry, ref.RevisionID); err != nil {
				return Snapshot{}, err
			}
		}
	}
	return out, nil
}

func addSnapshotBytes(used int, value any) (int, error) {
	encoded, err := json.Marshal(value)
	if err != nil || len(encoded) > MaxSnapshotBytes-used {
		return used, ErrInvalid
	}
	return used + len(encoded), nil
}
func (s *Service) ListSessions(ctx context.Context, p Principal, cursor string, limit int) (Page[Session], error) {
	out := Page[Session]{Items: []Session{}}
	if err := s.allow(ctx, p, "list", "", nil); err != nil {
		return out, err
	}
	err := s.repo.View(ctx, func(r Reader) error {
		var e error
		out, e = listPage(ctx, r, Sessions, p.Actor, cursor, limit, func(v Session) (string, bool, error) {
			if v.Owner != p.Actor {
				return v.ID, false, ErrForbidden
			}
			if v.DatabaseID != p.DatabaseID || v.ProjectID != p.ProjectID {
				return v.ID, false, nil
			}
			err := guardSessionRedaction(ctx, r, v.ID)
			if errors.Is(err, ErrReadOnly) {
				return v.ID, false, nil
			}
			return v.ID, err == nil, err
		})
		return e
	})
	return out, err
}

// ListEntryRevisions exposes a stable, authorized history for one session entry.
func (s *Service) ListEntryRevisions(ctx context.Context, p Principal, sessionID, entryID, cursor string, limit int) (Page[EntryRevision], error) {
	out := Page[EntryRevision]{Items: []EntryRevision{}}
	err := s.repo.View(ctx, func(r Reader) error {
		v, e := s.session(ctx, r, p, sessionID, "listEntryRevisions")
		if e != nil {
			return e
		}
		if !safeID.MatchString(entryID) {
			return ErrInvalid
		}
		var entry Entry
		if _, e = r.Load(ctx, Entries, entryID, &entry); e != nil {
			return e
		}
		if entry.SessionID != sessionID || !contains(v.BagIDs, entry.BagID) {
			return ErrForbidden
		}
		if _, e = s.bag(ctx, r, p, v, entry.BagID, "listEntryRevisions"); e != nil {
			return e
		}
		out, e = listPage(ctx, r, Revisions, p.Actor, cursor, limit, func(revision EntryRevision) (string, bool, error) {
			if revision.EntryID != entryID {
				return revision.ID, false, nil
			}
			if revision.BagID != entry.BagID || revision.EntryID != entry.ID {
				return revision.ID, false, ErrUnavailable
			}
			return revision.ID, true, nil
		})
		return e
	})
	return out, err
}

// ReadEntryRevision returns one retained revision after checking its owning
// session and the bag's current authorization boundary.
func (s *Service) ReadEntryRevision(ctx context.Context, p Principal, sessionID, entryID, revisionID string) (EntryRevision, error) {
	var out EntryRevision
	err := s.repo.View(ctx, func(r Reader) error {
		v, e := s.session(ctx, r, p, sessionID, "readEntryRevision")
		if e != nil {
			return e
		}
		if !safeID.MatchString(entryID) || !safeID.MatchString(revisionID) {
			return ErrInvalid
		}
		var entry Entry
		if _, e = r.Load(ctx, Entries, entryID, &entry); e != nil {
			return e
		}
		if entry.SessionID != sessionID || !contains(v.BagIDs, entry.BagID) {
			return ErrForbidden
		}
		if _, e = s.bag(ctx, r, p, v, entry.BagID, "readEntryRevision"); e != nil {
			return e
		}
		if _, e = r.Load(ctx, Revisions, revisionID, &out); e != nil {
			return e
		}
		if out.ID != revisionID || out.EntryID != entry.ID || out.BagID != entry.BagID {
			return ErrForbidden
		}
		return nil
	})
	if err != nil {
		return EntryRevision{}, err
	}
	return out, nil
}

// ListRuns returns durable run receipts in stable ID order. Each page is scoped to
// the authorized session and reauthorizes every pinned source scope.
func (s *Service) ListRuns(ctx context.Context, p Principal, sessionID, cursor string, limit int) (Page[Run], error) {
	out := Page[Run]{Items: []Run{}}
	err := s.repo.View(ctx, func(r Reader) error {
		if _, e := s.session(ctx, r, p, sessionID, "listRuns"); e != nil {
			return e
		}
		var e error
		out, e = listPage(ctx, r, Runs, p.Actor, cursor, limit, func(run Run) (string, bool, error) {
			if run.SessionID != sessionID {
				return run.ID, false, nil
			}
			var input InputSnapshot
			if _, e := r.Load(ctx, Inputs, run.InputSnapshotID, &input); e != nil {
				return run.ID, false, e
			}
			if e := s.allow(ctx, p, "listRuns", sessionID, &input.Scope); e != nil {
				return run.ID, false, e
			}
			return run.ID, true, nil
		})
		return e
	})
	return out, err
}

// ListMessages returns durable chat history in stable ID order, rechecking the
// source scope pinned when each message was sent.
func (s *Service) ListMessages(ctx context.Context, p Principal, sessionID, cursor string, limit int) (Page[Message], error) {
	out := Page[Message]{Items: []Message{}}
	err := s.repo.View(ctx, func(r Reader) error {
		if _, e := s.session(ctx, r, p, sessionID, "listMessages"); e != nil {
			return e
		}
		var e error
		out, e = listPage(ctx, r, Messages, p.Actor, cursor, limit, func(message Message) (string, bool, error) {
			if message.SessionID != sessionID {
				return message.ID, false, nil
			}
			if message.Provenance == "legacy-import" {
				if e := s.allow(ctx, p, "listMessages", sessionID, &message.SourceScope); e != nil {
					return message.ID, false, e
				}
				return message.ID, true, nil
			}
			var input InputSnapshot
			if _, e := r.Load(ctx, Inputs, message.InputSnapshotID, &input); e != nil {
				return message.ID, false, e
			}
			if e := s.allow(ctx, p, "listMessages", sessionID, &input.Scope); e != nil {
				return message.ID, false, e
			}
			return message.ID, true, nil
		})
		return e
	})
	return out, err
}

// listPage scans owner-filtered storage rows until it has a complete page for
// the requested logical scope. The returned cursor is the last returned record,
// so no record is skipped when a page contains unrelated sessions.
func listPage[T any](ctx context.Context, r Reader, collection, owner, cursor string, limit int, match func(T) (string, bool, error)) (Page[T], error) {
	out := Page[T]{Items: []T{}}
	if limit < 1 || limit > 100 || cursor != "" && !safeID.MatchString(cursor) {
		return out, ErrInvalid
	}
	scanCursor := cursor
	lastScanned, scanned, pageBytes := cursor, 0, 0
	for {
		rows, next, err := r.List(ctx, collection, owner, scanCursor, min(100, 1000-scanned))
		if err != nil {
			return Page[T]{Items: []T{}}, err
		}
		if len(rows) == 0 {
			if next != "" {
				return Page[T]{Items: []T{}}, ErrUnavailable
			}
			return out, nil
		}
		for _, raw := range rows {
			if scanned >= 1000 {
				out.NextCursor, out.HasMore = lastScanned, true
				return out, nil
			}
			var item T
			var meta struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(raw, &item) != nil || json.Unmarshal(raw, &meta) != nil || !safeID.MatchString(meta.ID) {
				return Page[T]{Items: []T{}}, ErrUnavailable
			}
			id, keep, err := match(item)
			if err != nil {
				return Page[T]{Items: []T{}}, err
			}
			if id != meta.ID {
				return Page[T]{Items: []T{}}, ErrUnavailable
			}
			previousCursor := lastScanned
			scanCursor, lastScanned = meta.ID, meta.ID
			scanned++
			if keep {
				if len(raw) > MaxSnapshotBytes || pageBytes > MaxSnapshotBytes-len(raw) || len(out.Items) == limit {
					out.NextCursor, out.HasMore = previousCursor, true
					return out, nil
				}
				out.Items = append(out.Items, item)
				pageBytes += len(raw)
			}
		}
		if scanned >= 1000 && next != "" {
			out.NextCursor, out.HasMore = lastScanned, true
			return out, nil
		}
		if next == "" {
			return out, nil
		}
		if scanCursor == "" || scanCursor == cursor {
			return Page[T]{Items: []T{}}, ErrUnavailable
		}
	}
}
func opKey(p Principal, id string) string {
	b, _ := json.Marshal([]string{p.Actor, p.DatabaseID, p.ProjectID, id})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func assistantMessageID(runID string) string {
	digest := sha256.Sum256([]byte("assistant\x00" + runID))
	return "msg_" + hex.EncodeToString(digest[:])
}
func boundDigest(p Principal, c Command) (string, error) {
	b, e := json.Marshal(struct {
		Principal Principal
		Command   Command
	}{p, c})
	if e != nil {
		return "", ErrInvalid
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}
func (s *Service) ReadOperation(ctx context.Context, p Principal, id string) (Receipt, error) {
	var out Receipt
	if !safeID.MatchString(id) {
		return out, ErrInvalid
	}
	if err := s.allow(ctx, p, "readOperation", "", nil); err != nil {
		return out, err
	}
	err := s.repo.View(ctx, func(r Reader) error {
		if _, e := r.Load(ctx, Operations, opKey(p, id), &out); e != nil {
			return e
		}
		_, e := s.session(ctx, r, p, out.SessionID, "readOperation")
		if e != nil {
			return e
		}
		return s.allow(ctx, p, "readOperation", out.SessionID, out.Scope)
	})
	return out, err
}
func (s *Service) Execute(ctx context.Context, p Principal, c Command) (CommandResult, error) {
	var result CommandResult
	legacy := c.Action == "importLegacy"
	var legacyManifest LegacyImportManifest
	var legacyEntryIDs, legacyMessageIDs []string
	targetSessionID, targetScope := c.SessionID, c.Scope
	if legacy {
		if c.SessionID != "" || c.LegacyImport == nil || c.Scope != nil {
			return result, ErrInvalid
		}
		var err error
		legacyManifest, legacyEntryIDs, legacyMessageIDs, err = prepareLegacyImport(p, c.LegacyImport)
		if err != nil {
			return result, err
		}
		targetSessionID, targetScope = legacyManifest.SessionID, &legacyManifest.Scope
	} else if c.LegacyImport != nil {
		return result, ErrInvalid
	}
	if !safeID.MatchString(c.OpID) || (!legacy && !safeID.MatchString(c.SessionID)) {
		return result, ErrInvalid
	}
	if c.Action != "pinInput" && c.Action != "recordRunExecution" && len(c.ResolvedParameters) != 0 || c.Action != "pinInput" && len(c.DefinitionDocument) != 0 || c.Action != "recordRunExecution" && len(c.ExecutionDocument) != 0 {
		return result, ErrInvalid
	}
	if c.DatabaseID != "" && c.DatabaseID != p.DatabaseID || c.ProjectID != "" && c.ProjectID != p.ProjectID {
		return result, ErrForbidden
	}
	if err := s.allow(ctx, p, c.Action, targetSessionID, targetScope); err != nil {
		return result, err
	}
	digest, err := boundDigest(p, c)
	if err != nil {
		return result, err
	}
	err = s.repo.Transact(ctx, func(tx Transaction) error {
		key := opKey(p, c.OpID)
		var old Receipt
		if _, e := tx.Load(ctx, Operations, key, &old); e == nil {
			if old.PayloadDigest != digest || old.Actor != p.Actor || old.Action != c.Action || legacy && old.SessionID != targetSessionID {
				return ErrConflict
			}
			v, e := s.session(ctx, tx, p, old.SessionID, c.Action)
			if e != nil {
				return e
			}
			if c.BagID != "" {
				if _, e = s.bag(ctx, tx, p, v, c.BagID, c.Action); e != nil {
					return e
				}
			}
			if e = s.allow(ctx, p, c.Action, v.ID, old.Scope); e != nil {
				return e
			}
			result.Receipt = old
			return nil
		} else if !errors.Is(e, ErrNotFound) {
			return e
		}
		now := s.now()
		var v Session
		var resource string
		var resourceVersion int64
		count := 0
		receiptScope := c.Scope
		writeSession := true
		if legacy || c.Action == "createSession" {
			if e := guardSessionRedaction(ctx, tx, targetSessionID); e != nil {
				return e
			}
		}
		if legacy {
			var stored LegacyImportManifest
			if _, e := tx.Load(ctx, LegacyImports, legacyManifest.ID, &stored); e == nil {
				if stored.SourceID != legacyManifest.SourceID || stored.SourceDigest != legacyManifest.SourceDigest || stored.Scope != legacyManifest.Scope || stored.SessionID != targetSessionID || stored.BagID != legacyManifest.BagID {
					return ErrConflict
				}
				v, e = s.session(ctx, tx, p, targetSessionID, c.Action)
				if e != nil {
					return e
				}
				writeSession = false
				resource, resourceVersion = v.ID, v.Version
				count = 0
			} else if !errors.Is(e, ErrNotFound) {
				return e
			} else {
				now = s.now()
				legacyManifest.CreatedAt = now
				contextSetID := "legacy_cs_" + strings.TrimPrefix(legacyManifest.ID, "li_")
				v = Session{SchemaVersion: 1, ID: targetSessionID, DatabaseID: p.DatabaseID, ProjectID: p.ProjectID, Owner: p.Actor, Title: c.LegacyImport.Title, State: "active", Version: 1, DefaultBagID: legacyManifest.BagID, ContextSetID: contextSetID, BagIDs: []string{legacyManifest.BagID}, CreatedAt: now, UpdatedAt: now}
				bag := Bag{ID: legacyManifest.BagID, SessionID: v.ID, Title: "Imported context", Scope: legacyManifest.Scope, Version: 1, EntryIDs: []string{}}
				contextSet := AppliedContextSet{ID: contextSetID, SessionID: v.ID, Enabled: false, Slots: []AppliedSlot{}, CreatedAt: now}
				for i, source := range c.LegacyImport.Entries {
					revisionHash := sha256.Sum256([]byte(legacyManifest.ID + "\x00" + source.ID + "\x00" + legacyManifest.SourceDigest))
					revisionID := "legacy_r_" + hex.EncodeToString(revisionHash[:])
					createdAt := source.CreatedAt
					if createdAt.IsZero() {
						createdAt = now
					}
					revision := EntryRevision{ID: revisionID, EntryID: legacyEntryIDs[i], BagID: bag.ID, Subject: source.Subject, Value: source.Value, CreatedAt: createdAt, Provenance: "legacy-import", LegacySourceID: legacyManifest.SourceID, SourceDigest: legacyManifest.SourceDigest}
					entry := Entry{ID: legacyEntryIDs[i], SessionID: v.ID, BagID: bag.ID, Alias: source.Alias, Subject: source.Subject, CurrentRevisionID: revisionID, Version: 1}
					if e = tx.Save(ctx, Revisions, revision.ID, revision, 0, p.Actor, v.ID); e != nil {
						return e
					}
					if e = tx.Save(ctx, Entries, entry.ID, entry, 0, p.Actor, v.ID); e != nil {
						return e
					}
					bag.EntryIDs = append(bag.EntryIDs, entry.ID)
				}
				for i, source := range c.LegacyImport.Messages {
					historicalAt := source.CreatedAt
					message := Message{ID: legacyMessageIDs[i], SessionID: v.ID, Role: source.Role, Text: source.Text, CreatedAt: now, Provenance: "legacy-import", LegacySourceID: legacyManifest.SourceID, SourceDigest: legacyManifest.SourceDigest, HistoricalAt: historicalAt, SourceScope: legacyManifest.Scope}
					if e = tx.Save(ctx, Messages, message.ID, message, 0, p.Actor, v.ID); e != nil {
						return e
					}
				}
				if e = tx.Save(ctx, Bags, bag.ID, bag, 0, p.Actor, v.ID); e != nil {
					return e
				}
				if e = tx.Save(ctx, ContextSets, contextSet.ID, contextSet, 0, p.Actor, v.ID); e != nil {
					return e
				}
				if e = tx.Save(ctx, LegacyImports, legacyManifest.ID, legacyManifest, 0, p.Actor, v.ID); e != nil {
					return e
				}
				if _, e = s.loadSnapshot(ctx, tx, p, v, c.Action); errors.Is(e, errSnapshotLimit) {
					return ErrInvalid
				} else if e != nil {
					return e
				}
				resource, resourceVersion = v.ID, v.Version
				count = len(c.LegacyImport.Entries) + len(c.LegacyImport.Messages)
			}
			receiptScope = &legacyManifest.Scope
		} else if c.Action == "createSession" {
			if c.DatabaseID != p.DatabaseID || c.ProjectID != p.ProjectID || !safeID.MatchString(c.BagID) || !validScope(c.Scope) || len(c.Title) > 256 {
				return ErrInvalid
			}
			if _, e := tx.Load(ctx, Sessions, c.SessionID, &v); !errors.Is(e, ErrNotFound) {
				if e == nil {
					return ErrConflict
				}
				return e
			}
			v = Session{SchemaVersion: 1, ID: c.SessionID, DatabaseID: p.DatabaseID, ProjectID: p.ProjectID, Owner: p.Actor, Title: c.Title, State: "active", Version: 1, DefaultBagID: c.BagID, ContextSetID: "cs_" + key, BagIDs: []string{c.BagID}, CreatedAt: now, UpdatedAt: now}
			b := Bag{ID: c.BagID, SessionID: v.ID, Title: "Context Bag", Scope: *c.Scope, Version: 1, EntryIDs: []string{}}
			set := AppliedContextSet{ID: v.ContextSetID, SessionID: v.ID, Enabled: true, Slots: []AppliedSlot{}, CreatedAt: now}
			if e := tx.Save(ctx, Bags, b.ID, b, 0, p.Actor, v.ID); e != nil {
				return e
			}
			if e := tx.Save(ctx, ContextSets, set.ID, set, 0, p.Actor, v.ID); e != nil {
				return e
			}
			resource = b.ID
			resourceVersion = 1
		} else {
			var e error
			v, e = s.session(ctx, tx, p, c.SessionID, c.Action)
			if e != nil {
				return e
			}
			if v.State != "active" && c.Action != "transitionSession" && c.Action != "completeRun" && c.Action != "reportDashboard" && c.Action != "reportClientRun" && c.Action != "finishHandoff" {
				return ErrReadOnly
			}
			if c.ExpectedVersion != v.Version && c.Action != "applyContext" && c.Action != "completeRun" && c.Action != "reportDashboard" && c.Action != "reportClientRun" && c.Action != "finishHandoff" {
				return ErrConflict
			}
			switch c.Action {
			case "createBag":
				if !safeID.MatchString(c.BagID) || !validScope(c.Scope) || len(c.Title) > 256 || len(v.BagIDs) >= 64 {
					return ErrInvalid
				}
				b := Bag{ID: c.BagID, SessionID: v.ID, Title: c.Title, Scope: *c.Scope, Version: 1, EntryIDs: []string{}}
				if e = tx.Save(ctx, Bags, b.ID, b, 0, p.Actor, v.ID); e != nil {
					return e
				}
				v.BagIDs = append(v.BagIDs, b.ID)
				resource = b.ID
				resourceVersion = 1
			case "renameSession":
				if strings.TrimSpace(c.Title) == "" || len(c.Title) > 256 {
					return ErrInvalid
				}
				v.Title = c.Title
				resource = v.ID
				resourceVersion = v.Version + 1
			case "renameBag":
				if strings.TrimSpace(c.Title) == "" || len(c.Title) > 256 {
					return ErrInvalid
				}
				b, e := s.bag(ctx, tx, p, v, c.BagID, c.Action)
				if e != nil {
					return e
				}
				if b.Version != c.ExpectedBagVersion {
					return ErrConflict
				}
				b.Title = c.Title
				b.Version++
				if e = tx.Save(ctx, Bags, b.ID, b, c.ExpectedBagVersion, p.Actor, v.ID); e != nil {
					return e
				}
				resource = b.ID
				resourceVersion = b.Version
			case "putEntry":
				b, e := s.bag(ctx, tx, p, v, c.BagID, c.Action)
				if e != nil {
					return e
				}
				if b.Version != c.ExpectedBagVersion {
					return ErrConflict
				}
				if !safeID.MatchString(c.EntryID) || !safeID.MatchString(c.RevisionID) || c.Subject == nil || c.Subject.ConceptID == "" || c.Subject.FieldID == "" || c.Subject.FieldMappingRef == "" || !validAlias(c.Alias) || c.Value == nil {
					return ErrInvalid
				}
				if e = validateValue(*c.Value); e != nil {
					return e
				}
				var entry Entry
				actual, e := tx.Load(ctx, Entries, c.EntryID, &entry)
				if e != nil && !errors.Is(e, ErrNotFound) {
					return e
				}
				if errors.Is(e, ErrNotFound) {
					actual = 0
					entry = Entry{ID: c.EntryID, SessionID: v.ID, BagID: b.ID}
				} else if entry.BagID != b.ID || entry.SessionID != v.ID {
					return ErrForbidden
				}
				if actual != c.ExpectedEntryVersion {
					return ErrConflict
				}
				if normalizedAlias(c.Alias) != "" {
					for _, otherID := range b.EntryIDs {
						if otherID == entry.ID {
							continue
						}
						var other Entry
						if _, e = tx.Load(ctx, Entries, otherID, &other); e != nil {
							return e
						}
						if other.BagID != b.ID || other.SessionID != v.ID {
							return ErrUnavailable
						}
						if normalizedAlias(other.Alias) == normalizedAlias(c.Alias) {
							return ErrInvalid
						}
					}
				}
				rev := EntryRevision{ID: c.RevisionID, EntryID: entry.ID, BagID: b.ID, ParentRevisionID: entry.CurrentRevisionID, Subject: *c.Subject, Value: *c.Value, CreatedAt: now}
				if e = tx.Save(ctx, Revisions, rev.ID, rev, 0, p.Actor, v.ID); e != nil {
					return e
				}
				entry.Alias = c.Alias
				entry.Subject = *c.Subject
				entry.CurrentRevisionID = rev.ID
				entry.Version = actual + 1
				if e = tx.Save(ctx, Entries, entry.ID, entry, actual, p.Actor, v.ID); e != nil {
					return e
				}
				if !contains(b.EntryIDs, entry.ID) {
					if len(b.EntryIDs) >= 1000 {
						return ErrInvalid
					}
					b.EntryIDs = append(b.EntryIDs, entry.ID)
				}
				b.Version++
				if e = tx.Save(ctx, Bags, b.ID, b, c.ExpectedBagVersion, p.Actor, v.ID); e != nil {
					return e
				}
				resource = entry.ID
				resourceVersion = entry.Version
				count = valueCount(rev.Value)
			case "applyContext", "clearBag":
				b, e := s.bag(ctx, tx, p, v, c.BagID, c.Action)
				if e != nil {
					return e
				}
				if b.Version != c.ExpectedBagVersion {
					return ErrConflict
				}
				var set AppliedContextSet
				if _, e = tx.Load(ctx, ContextSets, v.ContextSetID, &set); e != nil {
					return e
				}
				at := -1
				slot := AppliedSlot{Scope: b.Scope, Entries: []EntryRef{}}
				for i, other := range set.Slots {
					if other.Scope == b.Scope {
						at = i
						slot = other
						break
					}
				}
				if slot.Version != c.ExpectedSlotVersion {
					return ErrConflict
				}
				if c.Action == "applyContext" {
					if c.Enabled == nil {
						return ErrInvalid
					}
					if c.ExpectedContextSetID != v.ContextSetID && (*c.Enabled != set.Enabled || c.ExpectedVersion == v.Version) {
						return ErrConflict
					}
					if c.ExpectedVersion != v.Version && c.ExpectedContextSetID == v.ContextSetID {
						return ErrConflict
					}
					refs, e := s.validateRefs(ctx, tx, p, v, b, c.Selection, true)
					if e != nil {
						return e
					}
					slot.BagID = b.ID
					slot.BagVersion = b.Version
					slot.Entries = refs
					set.Enabled = *c.Enabled
					count = len(refs)
				} else {
					b.EntryIDs = []string{}
					b.Version++
					b.ClearVersion = b.Version
					if e = tx.Save(ctx, Bags, b.ID, b, c.ExpectedBagVersion, p.Actor, v.ID); e != nil {
						return e
					}
					if slot.BagID == b.ID {
						slot.BagID = ""
						slot.BagVersion = 0
						slot.Entries = []EntryRef{}
						slot.Version++
						set.Slots[at] = slot
						set.ID = "cs_" + key
						set.CreatedAt = now
						if e = tx.Save(ctx, ContextSets, set.ID, set, 0, p.Actor, v.ID); e != nil {
							return e
						}
						v.ContextSetID = set.ID
					}
				}
				if c.Action == "applyContext" {
					slot.Version++
					if at < 0 {
						set.Slots = append(set.Slots, slot)
					} else {
						set.Slots[at] = slot
					}
					set.ID = "cs_" + key
					set.CreatedAt = now
					if e = tx.Save(ctx, ContextSets, set.ID, set, 0, p.Actor, v.ID); e != nil {
						return e
					}
					v.ContextSetID = set.ID
				}
				resource = b.ID
				resourceVersion = b.Version
			case "transitionSession":
				next := ""
				switch c.Transition {
				case "stop":
					if v.State == "active" {
						next = "stopped"
					}
				case "start":
					if v.State == "stopped" {
						next = "active"
					}
				case "archive":
					if v.State == "active" || v.State == "stopped" {
						next = "archived"
					}
				case "restore":
					if v.State == "archived" {
						next = "stopped"
					}
				case "delete":
					if v.State != "deleted" {
						if v.PendingRunCount != 0 {
							return ErrConflict
						}
						next = "deleted"
					}
				}
				if next == "" {
					return ErrInvalid
				}
				if v.State == "active" || next == "deleted" {
					v.NativeFenceGeneration++
				}
				v.State = next
				resource = v.ID
			case "pinInput":
				if !safeID.MatchString(c.InputSnapshotID) || !safeID.MatchString(c.RunID) || !validScope(c.Scope) || c.DefinitionID == "" || c.DefinitionRevision == "" || len(c.Text) > 100000 {
					return ErrInvalid
				}
				if c.Kind != "chat" && c.Kind != "query" && c.Kind != "dashboard" && c.Kind != "check" {
					return ErrInvalid
				}
				if c.Kind == "check" && !p.Executor {
					return ErrForbidden
				}
				if e = validateResolvedParameters(c.ResolvedParameters); e != nil {
					return e
				}
				definitionDocument, definitionDigest, documentErr := canonicalDefinitionDocument(c.Kind, c.DefinitionDocument)
				if documentErr != nil {
					return documentErr
				}
				if e = s.allow(ctx, p, c.Action, v.ID, c.Scope); e != nil {
					return e
				}
				var set AppliedContextSet
				if _, e = tx.Load(ctx, ContextSets, v.ContextSetID, &set); e != nil {
					return e
				}
				refs := []EntryRef{}
				if set.Enabled && c.Kind != "chat" {
					for _, slot := range set.Slots {
						if slot.Scope == *c.Scope && slot.BagID != "" {
							refs = append(refs, slot.Entries...)
						}
					}
				}
				refs = append(refs, c.Attachments...)
				refs, e = s.validateScopedRefs(ctx, tx, p, v, *c.Scope, refs)
				if e != nil {
					return e
				}
				parameterOrigin := "client-observed"
				if p.Executor {
					parameterOrigin = "server-executor"
				}
				input := InputSnapshot{ID: c.InputSnapshotID, SessionID: v.ID, ContextSetID: set.ID, Scope: *c.Scope, Entries: refs, ResolvedParameters: c.ResolvedParameters, ParametersOrigin: parameterOrigin, DefinitionDigest: definitionDigest, DefinitionDocument: definitionDocument, CreatedAt: now}
				authority := "client"
				if p.Executor {
					authority = "server"
				}
				run := Run{ID: c.RunID, SessionID: v.ID, InputSnapshotID: input.ID, Kind: c.Kind, DefinitionID: c.DefinitionID, DefinitionRevision: c.DefinitionRevision, ExecutionAuthority: authority, State: "accepted", Version: 1, CreatedAt: now}
				if e = tx.Save(ctx, Inputs, input.ID, input, 0, p.Actor, v.ID); e != nil {
					return e
				}
				if e = tx.Save(ctx, Runs, run.ID, run, 0, p.Actor, v.ID); e != nil {
					return e
				}
				v.PendingRunCount++
				if c.Kind == "chat" {
					m := Message{ID: run.ID, SessionID: v.ID, InputSnapshotID: input.ID, Role: "user", Text: c.Text, CreatedAt: now}
					if e = tx.Save(ctx, Messages, m.ID, m, 0, p.Actor, v.ID); e != nil {
						return e
					}
				}
				resource = run.ID
				resourceVersion = 1
				count = len(refs)
			case "completeRun":
				if !p.Executor {
					return ErrForbidden
				}
				var run Run
				version, e := tx.Load(ctx, Runs, c.RunID, &run)
				if e != nil {
					return e
				}
				if run.SessionID != v.ID || run.ExecutionAuthority != "server" || run.InputReference != nil || run.DetectiveOperationID != "" {
					return ErrForbidden
				}
				if version != c.ExpectedRunVersion || run.State != "accepted" {
					return ErrConflict
				}
				var input InputSnapshot
				if _, e = tx.Load(ctx, Inputs, run.InputSnapshotID, &input); e != nil {
					return e
				}
				receiptScope = &input.Scope
				if e = s.allow(ctx, p, c.Action, v.ID, &input.Scope); e != nil {
					return e
				}
				if e = validateRunOutcome(c.RunState, c.Widgets, c.ResultRefs, c.ErrorCode); e != nil {
					return e
				}
				run.State = c.RunState
				run.OutcomeOrigin = "server-executor"
				run.Widgets = c.Widgets
				run.ResultRefs = c.ResultRefs
				run.ErrorCode = c.ErrorCode
				run.Version++
				run.CompletedAt = &now
				if e = tx.Save(ctx, Runs, run.ID, run, version, p.Actor, v.ID); e != nil {
					return e
				}
				if e = settlePendingRun(&v); e != nil {
					return e
				}
				if c.CaseID != "" || c.DetectiveRunID != "" {
					if !safeID.MatchString(c.CaseID) || !safeID.MatchString(c.DetectiveRunID) {
						return ErrInvalid
					}
					h := EvidenceHandoff{ID: run.ID, SessionID: v.ID, RunID: run.ID, InputSnapshotID: run.InputSnapshotID, CaseID: c.CaseID, DetectiveRunID: c.DetectiveRunID, State: "pending"}
					if e = tx.Save(ctx, Outbox, h.ID, h, 0, p.Actor, v.ID); e != nil {
						return e
					}
				}
				resource = run.ID
				resourceVersion = run.Version
			case "recordRunExecution":
				if !safeID.MatchString(c.RunID) || c.CaseID != "" || c.DetectiveRunID != "" {
					return ErrInvalid
				}
				if e = validateResolvedParameters(c.ResolvedParameters); e != nil {
					return e
				}
				plan, planDigest, planErr := canonicalDefinitionDocument("query", c.ExecutionDocument)
				if planErr != nil {
					return planErr
				}
				var run Run
				version, e := tx.Load(ctx, Runs, c.RunID, &run)
				if e != nil {
					return e
				}
				authority := "client"
				if p.Executor {
					authority = "server"
				}
				if run.SessionID != v.ID || run.Kind != "chat" || run.ExecutionAuthority != authority || version != c.ExpectedRunVersion || run.State != "accepted" || run.ExecutionDigest != "" {
					return ErrConflict
				}
				var input InputSnapshot
				if _, e = tx.Load(ctx, Inputs, run.InputSnapshotID, &input); e != nil {
					return e
				}
				receiptScope = &input.Scope
				if e = s.allow(ctx, p, c.Action, v.ID, &input.Scope); e != nil {
					return e
				}
				run.ExecutionDocument = plan
				run.ExecutionDigest = planDigest
				run.ResolvedParameters = c.ResolvedParameters
				run.Version++
				if p.Executor {
					run.OutcomeOrigin = "server-executor"
				} else {
					run.OutcomeOrigin = "client-observed"
				}
				if e = tx.Save(ctx, Runs, run.ID, run, version, p.Actor, v.ID); e != nil {
					return e
				}
				resource = run.ID
				resourceVersion = run.Version
			case "reportDashboard":
				if p.Executor || c.CaseID != "" || c.DetectiveRunID != "" || c.Kind != "" || c.InputSnapshotID != "" || c.RunID == "" {
					return ErrInvalid
				}
				var run Run
				version, e := tx.Load(ctx, Runs, c.RunID, &run)
				if e != nil {
					return e
				}
				if run.SessionID != v.ID || run.Kind != "dashboard" || run.ExecutionAuthority != "client" || version != c.ExpectedRunVersion || run.State != "accepted" {
					return ErrConflict
				}
				var input InputSnapshot
				if _, e = tx.Load(ctx, Inputs, run.InputSnapshotID, &input); e != nil {
					return e
				}
				receiptScope = &input.Scope
				if e = s.allow(ctx, p, c.Action, v.ID, &input.Scope); e != nil {
					return e
				}
				if e = validateRunOutcome(c.RunState, c.Widgets, c.ResultRefs, c.ErrorCode); e != nil {
					return e
				}
				run.State = c.RunState
				run.OutcomeOrigin = "client-reported"
				run.Widgets = c.Widgets
				run.ResultRefs = c.ResultRefs
				run.ErrorCode = c.ErrorCode
				run.Version++
				run.CompletedAt = &now
				if e = tx.Save(ctx, Runs, run.ID, run, version, p.Actor, v.ID); e != nil {
					return e
				}
				if e = settlePendingRun(&v); e != nil {
					return e
				}
				resource = run.ID
				resourceVersion = run.Version
			case "reportClientRun":
				if p.Executor || c.CaseID != "" || c.DetectiveRunID != "" || c.InputSnapshotID != "" || c.RunID == "" || (c.Kind != "dashboard" && c.Kind != "chat" && c.Kind != "query") {
					return ErrInvalid
				}
				if c.Kind == "chat" {
					if len(c.Text) > 100000 || c.RunState == "completed" && strings.TrimSpace(c.Text) == "" {
						return ErrInvalid
					}
				} else if c.Text != "" {
					return ErrInvalid
				}
				var run Run
				version, e := tx.Load(ctx, Runs, c.RunID, &run)
				if e != nil {
					return e
				}
				if run.SessionID != v.ID || run.Kind != c.Kind || run.ExecutionAuthority != "client" || version != c.ExpectedRunVersion || run.State != "accepted" {
					return ErrConflict
				}
				outcomeUnknown := c.RunState == "outcome-unknown"
				if outcomeUnknown {
					if c.ErrorCode != "CLIENT_OUTCOME_UNKNOWN" || len(c.Widgets) != 0 || len(c.ResultRefs) != 0 || c.Text != "" {
						return ErrInvalid
					}
				} else if e = validateRunOutcome(c.RunState, c.Widgets, c.ResultRefs, c.ErrorCode); e != nil {
					return e
				}
				var input InputSnapshot
				if _, e = tx.Load(ctx, Inputs, run.InputSnapshotID, &input); e != nil {
					return e
				}
				receiptScope = &input.Scope
				if e = s.allow(ctx, p, c.Action, v.ID, &input.Scope); e != nil {
					return e
				}
				run.State = c.RunState
				run.OutcomeOrigin = "client-reported"
				if outcomeUnknown {
					run.OutcomeOrigin = "client-outcome-unknown"
				}
				run.Widgets = c.Widgets
				run.ResultRefs = c.ResultRefs
				run.ErrorCode = c.ErrorCode
				run.Version++
				run.CompletedAt = &now
				if e = tx.Save(ctx, Runs, run.ID, run, version, p.Actor, v.ID); e != nil {
					return e
				}
				if e = settlePendingRun(&v); e != nil {
					return e
				}
				if c.Kind == "chat" && c.Text != "" {
					assistant := Message{ID: assistantMessageID(run.ID), SessionID: v.ID, InputSnapshotID: run.InputSnapshotID, Role: "assistant", Text: c.Text, CreatedAt: now}
					if e = tx.Save(ctx, Messages, assistant.ID, assistant, 0, p.Actor, v.ID); e != nil {
						return e
					}
				}
				resource = run.ID
				resourceVersion = run.Version
			default:
				return ErrInvalid
			}
			v.Version++
			v.UpdatedAt = now
			if c.Action == "createBag" || c.Action == "renameSession" || c.Action == "renameBag" || c.Action == "putEntry" || c.Action == "applyContext" || c.Action == "clearBag" {
				if _, e := s.loadSnapshot(ctx, tx, p, v, c.Action); errors.Is(e, errSnapshotLimit) {
					return ErrInvalid
				} else if e != nil {
					return e
				}
			}
		}
		if writeSession {
			expected := v.Version - 1
			if c.Action == "createSession" || legacy {
				expected = 0
			}
			if e := tx.Save(ctx, Sessions, v.ID, v, expected, p.Actor, v.ID); e != nil {
				return e
			}
		}
		if c.BagID != "" {
			b, e := s.bag(ctx, tx, p, v, c.BagID, c.Action)
			if e != nil {
				return e
			}
			receiptScope = &b.Scope
		}
		receipt := Receipt{Scope: receiptScope, OpID: c.OpID, Actor: p.Actor, Action: c.Action, PayloadDigest: digest, SessionID: v.ID, SessionVersion: v.Version, ResourceID: resource, ResourceVersion: resourceVersion, At: now}
		event := AuditEvent{OpID: c.OpID, Actor: p.Actor, Action: c.Action, SessionID: v.ID, ResourceID: resource, Count: count, At: now}
		if e := tx.Save(ctx, Audit, key, event, 0, p.Actor, v.ID); e != nil {
			return e
		}
		if e := tx.Save(ctx, Operations, key, receipt, 0, p.Actor, v.ID); e != nil {
			return e
		}
		result.Receipt = receipt
		return nil
	})
	if err != nil {
		return CommandResult{}, err
	}
	return result, nil
}
func validScope(scope *Scope) bool {
	return scope != nil && safeID.MatchString(scope.EnvironmentID) && safeID.MatchString(scope.IdentityDomain)
}
func contains(values []string, v string) bool {
	for _, x := range values {
		if x == v {
			return true
		}
	}
	return false
}
func safeCode(code string) bool {
	return code != "" && len(code) <= 64 && regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`).MatchString(code)
}
func validateRunOutcome(state string, widgets []WidgetOutcome, resultRefs []string, errorCode string) error {
	if state != "completed" && state != "failed" && state != "canceled" || len(widgets) > 1000 || len(resultRefs) > 1000 {
		return ErrInvalid
	}
	if state == "failed" && !safeCode(errorCode) || state != "failed" && errorCode != "" {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, widget := range widgets {
		if !safeID.MatchString(widget.ID) || seen[widget.ID] || (widget.State != "completed" && widget.State != "failed") || (widget.State == "failed" && !safeCode(widget.ErrorCode)) || (widget.State == "completed" && !safeID.MatchString(widget.ResultRef)) {
			return ErrInvalid
		}
		seen[widget.ID] = true
	}
	for _, ref := range resultRefs {
		if !safeID.MatchString(ref) {
			return ErrInvalid
		}
	}
	return nil
}
func valueCount(v ValueEnvelope) int {
	if v.Kind == "scalar" {
		return 1
	}
	if len(v.Keys) > 0 {
		return len(v.Keys)
	}
	return len(v.Values)
}
func validateValue(v ValueEnvelope) error {
	if v.SchemaVersion != 1 {
		return ErrInvalid
	}
	if v.Kind == "scalar" {
		if v.Scalar == nil || len(v.Values) > 0 || len(v.Keys) > 0 || len(v.KeyFields) > 0 {
			return ErrInvalid
		}
		if e := v.Scalar.Validate(); e != nil {
			return ErrInvalid
		}
		return nil
	}
	if v.Kind != "set" || v.Scalar != nil || len(v.Values) > 0 && len(v.KeyFields) > 0 || len(v.Values) > 5300 || len(v.Keys) > 5300 {
		return ErrInvalid
	}
	for _, value := range v.Values {
		if e := value.Validate(); e != nil {
			return ErrInvalid
		}
	}
	seen := map[string]bool{}
	for _, field := range v.KeyFields {
		if !safeID.MatchString(field) || seen[field] {
			return ErrInvalid
		}
		seen[field] = true
	}
	for _, key := range v.Keys {
		if len(key.Components) != len(v.KeyFields) || len(v.KeyFields) == 0 {
			return ErrInvalid
		}
		for i, component := range key.Components {
			if component.Field != v.KeyFields[i] {
				return ErrInvalid
			}
			if e := component.Value.Validate(); e != nil {
				return ErrInvalid
			}
		}
	}
	b, e := json.Marshal(v)
	if e != nil || len(b) > 700000 {
		return ErrInvalid
	}
	return nil
}
func (s *Service) validateRefs(ctx context.Context, r Reader, p Principal, v Session, b Bag, refs []EntryRef, current bool) ([]EntryRef, error) {
	out := []EntryRef{}
	seen := map[string]string{}
	if len(refs) > 1000 {
		return nil, ErrInvalid
	}
	for _, ref := range refs {
		if !safeID.MatchString(ref.EntryID) || !safeID.MatchString(ref.RevisionID) {
			return nil, ErrInvalid
		}
		if previous, ok := seen[ref.EntryID]; ok {
			if previous != ref.RevisionID {
				return nil, ErrInvalid
			}
			continue
		}
		var entry Entry
		if _, e := r.Load(ctx, Entries, ref.EntryID, &entry); e != nil {
			return nil, e
		}
		if entry.BagID != b.ID || entry.SessionID != v.ID || !contains(b.EntryIDs, entry.ID) || current && entry.CurrentRevisionID != ref.RevisionID {
			return nil, ErrConflict
		}
		var rev EntryRevision
		if _, e := r.Load(ctx, Revisions, ref.RevisionID, &rev); e != nil {
			return nil, e
		}
		if rev.EntryID != ref.EntryID || rev.BagID != b.ID {
			return nil, ErrForbidden
		}
		seen[ref.EntryID] = ref.RevisionID
		out = append(out, ref)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EntryID < out[j].EntryID })
	return out, nil
}
func (s *Service) validateScopedRefs(ctx context.Context, r Reader, p Principal, v Session, scope Scope, refs []EntryRef) ([]EntryRef, error) {
	out := []EntryRef{}
	for _, ref := range refs {
		var entry Entry
		if _, e := r.Load(ctx, Entries, ref.EntryID, &entry); e != nil {
			return nil, e
		}
		b, e := s.bag(ctx, r, p, v, entry.BagID, "pinInput")
		if e != nil {
			return nil, e
		}
		if b.Scope != scope {
			return nil, ErrForbidden
		}
		validated, e := s.validateRefs(ctx, r, p, v, b, []EntryRef{ref}, false)
		if e != nil {
			return nil, e
		}
		out = append(out, validated...)
	}
	seen := map[string]string{}
	unique := []EntryRef{}
	for _, ref := range out {
		if prior, ok := seen[ref.EntryID]; ok {
			if prior != ref.RevisionID {
				return nil, ErrInvalid
			}
			continue
		}
		seen[ref.EntryID] = ref.RevisionID
		unique = append(unique, ref)
	}
	sort.Slice(unique, func(i, j int) bool { return strings.Compare(unique[i].EntryID, unique[j].EntryID) < 0 })
	return unique, nil
}

// ReadInput resolves exact historical revisions after fresh source-scope authorization.
func (s *Service) ReadInput(ctx context.Context, p Principal, id string) (InputSnapshot, []EntryRevision, error) {
	var input InputSnapshot
	out := []EntryRevision{}
	err := s.repo.View(ctx, func(r Reader) error {
		if _, e := r.Load(ctx, Inputs, id, &input); e != nil {
			return e
		}
		v, e := s.session(ctx, r, p, input.SessionID, "readInput")
		if e != nil {
			return e
		}
		if input.Detective != nil && v.State == "deleted" {
			return ErrReadOnly
		}
		if e = s.allow(ctx, p, "readInput", v.ID, &input.Scope); e != nil {
			return e
		}
		for _, ref := range input.Entries {
			var rev EntryRevision
			if _, e = r.Load(ctx, Revisions, ref.RevisionID, &rev); e != nil {
				return e
			}
			b, e := s.bag(ctx, r, p, v, rev.BagID, "readInput")
			if e != nil {
				return e
			}
			if b.Scope != input.Scope || rev.EntryID != ref.EntryID {
				return ErrForbidden
			}
			out = append(out, rev)
		}
		return nil
	})
	if err != nil {
		return InputSnapshot{}, nil, err
	}
	return input, out, nil
}
func (s *Service) ReadRun(ctx context.Context, p Principal, id string) (Run, error) {
	var run Run
	err := s.repo.View(ctx, func(r Reader) error {
		if _, e := r.Load(ctx, Runs, id, &run); e != nil {
			return e
		}
		if _, e := s.session(ctx, r, p, run.SessionID, "readRun"); e != nil {
			return e
		}
		var input InputSnapshot
		if _, e := r.Load(ctx, Inputs, run.InputSnapshotID, &input); e != nil {
			return e
		}
		return s.allow(ctx, p, "readRun", run.SessionID, &input.Scope)
	})
	if err != nil {
		return Run{}, err
	}
	return run, nil
}
