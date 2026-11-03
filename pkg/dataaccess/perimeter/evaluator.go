package perimeter

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/dal-go/dalgo/access"
	"github.com/datatug/datatug-core/pkg/dataaccess"
)

// ForSubject answers "what can this subject access, and why" over every
// connection the filter keeps. See Filter.IncludeDenied for what a self-view
// discloses.
func ForSubject(ctx context.Context, s Snapshot, subject dataaccess.Subject, f Filter) (Answer, error) {
	e := newEvaluator(s)
	e.detail = f.IncludeDenied
	return e.forSubject(ctx, subject, f)
}

// Explain answers one (subject, permission, table) question. Without
// ExplainOptions.Detail a deny carries only READ_DENIED.
func Explain(ctx context.Context, s Snapshot, subject dataaccess.Subject, permission dataaccess.Permission, r Resource, o ExplainOptions) (Answer, error) {
	e := newEvaluator(s)
	e.detail = o.Detail
	return e.explain(ctx, subject, permission, r)
}

// ForResource answers "who can access this table, and why". It is an
// access.inspect-grade view with full detail. Candidates are the project
// members and every subject named by a grant on the connection. Under posture
// all it returns one "every project member" row per permission, plus a row
// only for a candidate whose decision differs from that row.
func ForResource(ctx context.Context, s Snapshot, r Resource, permissions []dataaccess.Permission) (Answer, error) {
	e := newEvaluator(s)
	e.detail = true
	return e.forResource(ctx, r, permissions)
}

// inspectFunc is dataaccess.InspectTableRead; tests may substitute it to
// prove the fail-closed handling of an inconsistent inspection.
type inspectFunc func(context.Context, dataaccess.Resolution, *dataaccess.ExactReadSnapshot, dataaccess.ObjectRef) (dataaccess.TableReadInspection, error)

type evaluator struct {
	snapshot Snapshot
	inspect  inspectFunc
	detail   bool
}

func newEvaluator(s Snapshot) evaluator {
	return evaluator{snapshot: s, inspect: dataaccess.InspectTableRead}
}

// projectMember stands for "a project member with no personal grant" in the
// single posture-all row. It never names a real subject in an answer.
var projectMember = dataaccess.Subject{Realm: "perimeter", Kind: dataaccess.User, ID: "project-member"}

func (e evaluator) check(ctx context.Context) error {
	if ctx == nil {
		return fmt.Errorf("perimeter: context required")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if e.snapshot.AsOf.IsZero() {
		return fmt.Errorf("perimeter: snapshot time required")
	}
	return nil
}

func (e evaluator) forSubject(ctx context.Context, subject dataaccess.Subject, f Filter) (Answer, error) {
	if err := e.check(ctx); err != nil {
		return Answer{}, err
	}
	permissions := defaultPermissions(f.Permissions)
	entries := []Entry{}
	complete := true
	for _, conn := range e.connections() {
		if !f.keeps(conn) {
			continue
		}
		complete = complete && connectionComplete(conn)
		named := ownObjects(conn, subject)
		folded := false
		for _, object := range objectsFor(conn, named) {
			for _, permission := range permissions {
				r := Resource{Connection: conn.Ref.ID, Namespace: object.Namespace, Name: object.Name}
				entry := e.evaluate(ctx, &conn, subject, false, permission, r)
				switch {
				case e.detail, entry.Decision == Allow, slices.Contains(named, object):
					entries = append(entries, entry)
				case entry.Decision == Indeterminate:
					// Denied names are not wire data (R-10): an unknown on an
					// object the subject neither reaches nor named is folded.
					folded = true
				}
			}
		}
		if folded {
			entries = append(entries, Entry{Resource: Resource{Connection: conn.Ref.ID}, Decision: Indeterminate,
				Blockers: []Blocker{{Code: CodeResourcesNotEnumerated}}, Enforcement: EnforcementServer})
		}
	}
	return e.answer(entries, complete), nil
}

func (e evaluator) explain(ctx context.Context, subject dataaccess.Subject, permission dataaccess.Permission, r Resource) (Answer, error) {
	if err := e.check(ctx); err != nil {
		return Answer{}, err
	}
	conn := e.connection(r.Connection)
	return e.answer([]Entry{e.evaluate(ctx, conn, subject, false, permission, r)}, conn != nil && connectionComplete(*conn)), nil
}

func (e evaluator) forResource(ctx context.Context, r Resource, permissions []dataaccess.Permission) (Answer, error) {
	if err := e.check(ctx); err != nil {
		return Answer{}, err
	}
	conn := e.connection(r.Connection)
	entries := []Entry{}
	for _, permission := range defaultPermissions(permissions) {
		if conn == nil || (conn.Protection != nil && conn.Protection.ReadPosture == dataaccess.ReadAll) {
			group := e.evaluate(ctx, conn, projectMember, true, permission, r)
			group.Subject, group.Group = nil, GroupProjectMembers
			entries = append(entries, group)
			if conn == nil {
				continue
			}
			for _, candidate := range e.candidates(conn) {
				if entry := e.evaluate(ctx, conn, candidate, false, permission, r); entry.Decision != group.Decision {
					entries = append(entries, entry)
				}
			}
			continue
		}
		for _, candidate := range e.candidates(conn) {
			entries = append(entries, e.evaluate(ctx, conn, candidate, false, permission, r))
		}
	}
	return e.answer(entries, conn != nil && connectionComplete(*conn)), nil
}

func (e evaluator) answer(entries []Entry, complete bool) Answer {
	complete = complete && e.snapshot.Members != nil && len(e.snapshot.Incomplete) == 0
	for _, entry := range entries {
		complete = complete && entry.Decision != Indeterminate
	}
	return Answer{AsOf: e.snapshot.AsOf, ValidUntil: e.snapshot.AsOf.Add(AnswerLease), Complete: complete,
		Entries: entries, Limitations: slices.Clone(Limitations)}
}

// evaluate answers one question. Read decisions come only from the Core
// inspection of the backend-equal resolution; this function refuses what it
// cannot evaluate and may only narrow an allow.
func (e evaluator) evaluate(ctx context.Context, conn *Connection, subject dataaccess.Subject, member bool, permission dataaccess.Permission, r Resource) Entry {
	entry := Entry{Subject: &subject, Resource: r, Permission: permission}
	if conn == nil {
		return indeterminate(entry, CodeUnknownConnection)
	}
	entry.Enforcement = EnforcementServer
	if conn.Ref.Space == "" {
		entry.Enforcement = EnforcementHonestClient
		return indeterminate(entry, CodeLocalConnection)
	}
	if len(e.snapshot.Incomplete) > 0 || len(conn.Incomplete) > 0 {
		return indeterminate(entry, CodeInputIncomplete)
	}
	// Subject kind and membership come before the permission, so no
	// permission choice can make a non-member's answer list a name.
	if subject.Kind != dataaccess.User {
		return indeterminate(entry, CodeSubjectKindUnsupported)
	}
	if e.snapshot.Members == nil {
		return indeterminate(entry, CodeMembershipUnavailable)
	}
	if !member && !slices.Contains(e.snapshot.Members, subject) {
		entry.Blockers, entry.Decision = []Blocker{{Code: CodeNotProjectMember}}, Deny
		return entry
	}
	switch permission {
	case dataaccess.DataRead:
	case dataaccess.DataInsert, dataaccess.DataUpdate, dataaccess.DataDelete, dataaccess.DataWrite:
		// Core has no write rule: every write is refused before dispatch.
		entry.Blockers = []Blocker{{Code: CodeUnsupportedScope, Phase: "P3"}}
		if conn.Protection != nil && conn.Protection.ReadOnly {
			entry.Blockers = append(entry.Blockers, Blocker{Code: CodeReadOnlyConnection, Source: &Ref{Kind: "protection", ID: conn.Protection.ID}})
		}
		entry.Decision = Deny
		return entry
	case dataaccess.DataReveal:
		entry.Blockers, entry.Decision = []Blocker{{Code: CodeUnsupportedScope, Phase: "L3"}}, Deny
		return entry
	case dataaccess.QueryRaw:
		entry.Blockers, entry.Decision = []Blocker{{Code: CodeUnsupportedScope, Phase: "raw"}}, Deny
		return entry
	case dataaccess.QueryTugQL:
		return indeterminate(entry, CodePermissionNotModelled)
	default:
		return indeterminate(entry, CodeUnknownPermission)
	}
	if conn.Protection == nil {
		return indeterminate(entry, CodeProtectionUnavailable)
	}
	resolution, code := e.resolution(conn, subject)
	if code != "" {
		return indeterminate(entry, code)
	}
	object := dataaccess.ObjectRef{Namespace: r.Namespace, Name: r.Name}
	entry = e.judge(ctx, conn, entry, resolution, object, nil)
	if entry.Decision != Allow || conn.AllowSelfApproval {
		return entry
	}
	// The evaluator's own stricter rule (R-1) may only narrow an allow: the
	// read is also judged without grants their beneficiary decided.
	filtered := resolution
	filtered.Grants = []dataaccess.Grant{}
	var self []string
	for _, g := range resolution.Grants {
		if g.Decision != nil && g.Decision.By == g.Subject {
			if covers(conn, g, object) {
				self = append(self, g.ID)
			}
			continue
		}
		filtered.Grants = append(filtered.Grants, g)
	}
	if len(filtered.Grants) == len(resolution.Grants) {
		return entry
	}
	slices.Sort(self)
	return e.judge(ctx, conn, Entry{Subject: entry.Subject, Resource: r, Permission: permission, Enforcement: entry.Enforcement}, filtered, object, self)
}

// judge projects and inspects one resolution. Only a plain Core read denial
// is a deny; every other Core refusal is indeterminate.
func (e evaluator) judge(ctx context.Context, conn *Connection, entry Entry, resolution dataaccess.Resolution, object dataaccess.ObjectRef, self []string) Entry {
	var mandatory *dataaccess.ExactReadSnapshot
	// As the backend does, a stored exact configuration is always projected;
	// projection refuses it under any other posture. Posture exact-v1 with no
	// configuration is refused by Core.
	if conn.Exact != nil {
		projected, err := dataaccess.ProjectExactRead(ctx, resolution, conn.Exact)
		if err != nil {
			return indeterminate(entry, CodeExactPolicyUnavailable)
		}
		resolution, mandatory = projected, conn.Exact
	}
	inspection, err := e.inspect(ctx, resolution, mandatory, object)
	if err != nil {
		return indeterminate(entry, CodeEvaluationUnavailable)
	}
	sources := sourcesFor(conn, resolution, object)
	if !inspection.Admitted {
		if inspection.Code != CodeReadDenied {
			return indeterminate(entry, inspection.Code)
		}
		entry.Decision = Deny
		entry.Blockers = []Blocker{{Code: CodeReadDenied}}
		if !e.detail {
			return entry
		}
		entry.Candidates = sources
		entry.Blockers[0].Source = &Ref{Kind: "authority", ID: conn.Ref.ID}
		for _, assessed := range inspection.Assessment.Policies {
			// DALgo may report one policy again through a canonical
			// native-table delegate; a blocker is listed once.
			blocker := Blocker{Code: string(assessed.Code), Source: &Ref{Kind: "policy", ID: assessed.Policy}}
			if !assessed.Allowed && !slices.ContainsFunc(entry.Blockers, func(b Blocker) bool {
				return b.Code == blocker.Code && *b.Source == *blocker.Source
			}) {
				entry.Blockers = append(entry.Blockers, blocker)
			}
		}
		for _, id := range self {
			entry.Blockers = append(entry.Blockers, Blocker{Code: CodeSelfDecided, Source: &Ref{Kind: SourceGrant, ID: id}})
		}
		return entry
	}
	if inspection.Assessment.Outcome != access.AssessmentAllow || len(sources) == 0 {
		return indeterminate(entry, CodeEvaluationInconsistent)
	}
	until := inspection.Until
	entry.Decision, entry.Sources, entry.AllowedUntil = Allow, sources, &until
	if conn.Protection.ReadOnly {
		entry.Restrictions = []Restriction{{Kind: "protection", ID: conn.Protection.ID, Effect: "read-only"}}
	}
	return entry
}

func indeterminate(entry Entry, code string) Entry {
	entry.Decision = Indeterminate
	entry.Blockers = []Blocker{{Code: code}}
	return entry
}

// resolution builds exactly what the backend resolver builds for the subject,
// before exact projection. Mirrors datatug/backend
// facade4datatug/access_control_reads.go at f9fc86835e3555555049e2eae05654b3a3aa6b98:
// listAccessGrants own-only query, cap and subject check (lines 91-125), then
// ResolveAccessWithExactSnapshot (lines 198-250): fingerprint check on every
// listed grant, approved grants unexpired at server time, ValidUntil at most
// one minute and, under posture none, at most the latest grant expiry. The
// actor's own token deadline is a host fact outside the snapshot.
func (e evaluator) resolution(conn *Connection, subject dataaccess.Subject) (dataaccess.Resolution, string) {
	at := e.snapshot.AsOf
	r := dataaccess.Resolution{Subject: subject, Connection: conn.Ref, Protection: *conn.Protection, Grants: []dataaccess.Grant{},
		ServerTime: at, ValidUntil: at.Add(AnswerLease)}
	var own []StoredGrant
	for _, g := range conn.Grants {
		if g.Subject.Realm == subject.Realm && g.Subject.ID == subject.ID {
			own = append(own, g)
		}
	}
	if len(own) > MaxGrantsPerSubject {
		return r, CodeGrantListTruncated
	}
	latest := at
	for _, g := range own {
		if g.Subject != subject || !sameFingerprint(g.Fingerprint, conn.Ref.Fingerprint) {
			return r, CodeGrantBindingMismatch
		}
		if g.State == dataaccess.Approved && g.ExpiresAt.After(at) {
			r.Grants = append(r.Grants, g.Grant)
			if g.ExpiresAt.After(latest) {
				latest = g.ExpiresAt
			}
		}
	}
	if conn.Protection.ReadPosture == dataaccess.ReadNone && latest.After(at) && latest.Before(r.ValidUntil) {
		r.ValidUntil = latest
	}
	return r, ""
}

// sameFingerprint mirrors the backend's accessSameFingerprint
// (facade4datatug/access_control.go:160 at the SHA above).
func sameFingerprint(a, b dataaccess.Fingerprint) bool {
	return a.Engine == b.Engine && a.Database == b.Database && a.EngineIdentity == b.EngineIdentity &&
		(a.EngineIdentity != "" || (a.Address != "" && a.Address == b.Address))
}

// covers mirrors the compiler: under exact-v1 a connection-wide grant
// contributes nothing.
func covers(conn *Connection, g dataaccess.Grant, object dataaccess.ObjectRef) bool {
	return slices.Contains(g.Scope.Objects, object) || (len(g.Scope.Objects) == 0 && conn.Protection.ReadPosture != dataaccess.ReadExact)
}

// sourcesFor attributes the inputs the compiler reads for an admitted (or
// candidate) read: posture all, the projected baseline, and the resolution's
// grants that cover the object.
func sourcesFor(conn *Connection, resolution dataaccess.Resolution, object dataaccess.ObjectRef) []Source {
	var sources []Source
	if conn.Protection.ReadPosture == dataaccess.ReadAll {
		sources = append(sources, Source{Kind: SourcePosture, ID: conn.Ref.ID, Target: string(dataaccess.ReadAll)})
	}
	if resolution.ExactRead != nil && slices.Contains(resolution.ExactRead.Baseline, object) {
		sources = append(sources, Source{Kind: SourceBaseline, ID: conn.Ref.ID, Target: "subject"})
	}
	var grants []Source
	for _, g := range resolution.Grants {
		if covers(conn, g, object) {
			expires := g.ExpiresAt
			grants = append(grants, Source{Kind: SourceGrant, ID: g.ID, Target: "subject", ExpiresAt: &expires})
		}
	}
	slices.SortFunc(grants, func(a, b Source) int {
		return cmp.Or(a.ExpiresAt.Compare(*b.ExpiresAt), cmp.Compare(a.ID, b.ID))
	})
	return append(sources, grants...)
}

func (e evaluator) connections() []Connection {
	conns := slices.Clone(e.snapshot.Connections)
	slices.SortStableFunc(conns, func(a, b Connection) int { return cmp.Compare(a.Ref.ID, b.Ref.ID) })
	return conns
}

func (e evaluator) connection(id string) *Connection {
	for i := range e.snapshot.Connections {
		if e.snapshot.Connections[i].Ref.ID == id {
			return &e.snapshot.Connections[i]
		}
	}
	return nil
}

func (e evaluator) candidates(conn *Connection) []dataaccess.Subject {
	candidates := slices.Clone(e.snapshot.Members)
	for _, g := range conn.Grants {
		candidates = append(candidates, g.Subject)
	}
	slices.SortFunc(candidates, compareSubjects)
	return slices.Compact(candidates)
}

func compareSubjects(a, b dataaccess.Subject) int {
	return cmp.Or(cmp.Compare(a.Realm, b.Realm), cmp.Compare(a.Kind, b.Kind), cmp.Compare(a.ID, b.ID))
}

func sortObjects(objects []dataaccess.ObjectRef) []dataaccess.ObjectRef {
	slices.SortFunc(objects, func(a, b dataaccess.ObjectRef) int {
		return cmp.Or(cmp.Compare(a.Namespace, b.Namespace), cmp.Compare(a.Name, b.Name))
	})
	return slices.Compact(objects)
}

// ownObjects are the objects named by the subject's own grants on conn.
func ownObjects(conn Connection, subject dataaccess.Subject) []dataaccess.ObjectRef {
	var objects []dataaccess.ObjectRef
	for _, g := range conn.Grants {
		if g.Subject == subject && g.Scope.Connection == conn.Ref.ID {
			objects = append(objects, g.Scope.Objects...)
		}
	}
	return sortObjects(objects)
}

func objectsFor(conn Connection, named []dataaccess.ObjectRef) []dataaccess.ObjectRef {
	return sortObjects(append(slices.Clone(conn.Inventory), named...))
}

func connectionComplete(conn Connection) bool {
	return conn.Protection != nil && len(conn.Incomplete) == 0
}

func (f Filter) keeps(conn Connection) bool {
	if len(f.Connections) > 0 && !slices.Contains(f.Connections, conn.Ref.ID) {
		return false
	}
	// An unreadable protection has no known environment; keep it so the
	// answer shows it as unknown rather than silently dropping it.
	return f.Environment == "" || conn.Protection == nil || conn.Protection.Environment == f.Environment
}

func defaultPermissions(permissions []dataaccess.Permission) []dataaccess.Permission {
	if len(permissions) == 0 {
		return []dataaccess.Permission{dataaccess.DataRead}
	}
	return permissions
}
