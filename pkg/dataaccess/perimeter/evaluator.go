package perimeter

import (
	"cmp"
	"context"
	"fmt"
	"slices"

	"github.com/dal-go/dalgo/access"
	"github.com/datatug/datatug-core/pkg/dataaccess"
)

// ForSubject answers "what can this subject access, and why": one entry per
// (connection, object, permission) for every connection the filter keeps and
// every object in its inventory or named by one of the subject's grants. Denied
// entries are omitted unless Filter.IncludeDenied is set.
func ForSubject(ctx context.Context, s Snapshot, subject dataaccess.Subject, f Filter) (Answer, error) {
	return newEvaluator(s).forSubject(ctx, subject, f)
}

// Explain answers one (subject, permission, table) question with its sources,
// restrictions and blockers.
func Explain(ctx context.Context, s Snapshot, subject dataaccess.Subject, permission dataaccess.Permission, r Resource) (Entry, error) {
	return newEvaluator(s).explain(ctx, subject, permission, r)
}

// ForResource answers "who can access this table, and why". Candidates are the
// project members and every subject named by a grant on the connection. Under
// posture all it returns one "every project member" row per permission, plus a
// row only for a candidate whose decision differs from that row.
func ForResource(ctx context.Context, s Snapshot, r Resource, permissions []dataaccess.Permission) (Answer, error) {
	return newEvaluator(s).forResource(ctx, r, permissions)
}

// inspectFunc is dataaccess.InspectTableRead; tests may substitute it to
// prove the fail-closed handling of an inconsistent inspection.
type inspectFunc func(context.Context, dataaccess.Resolution, *dataaccess.ExactReadSnapshot, dataaccess.ObjectRef) (dataaccess.TableReadInspection, error)

type evaluator struct {
	snapshot Snapshot
	inspect  inspectFunc
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
		for _, object := range objectsFor(conn, subject) {
			for _, permission := range permissions {
				r := Resource{Connection: conn.Ref.ID, Namespace: object.Namespace, Name: object.Name}
				// Denied names are not wire data: by default only reachable
				// and unknown resources are listed (R-10).
				if entry := e.evaluate(ctx, &conn, subject, false, permission, r); f.IncludeDenied || entry.Decision != Deny {
					entries = append(entries, entry)
				}
			}
		}
	}
	return e.answer(entries, complete), nil
}

func (e evaluator) explain(ctx context.Context, subject dataaccess.Subject, permission dataaccess.Permission, r Resource) (Entry, error) {
	if err := e.check(ctx); err != nil {
		return Entry{}, err
	}
	return e.evaluate(ctx, e.connection(r.Connection), subject, false, permission, r), nil
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

// evaluate answers one question. Every read decision comes from the Core
// inspection; this function only selects inputs, refuses what it cannot
// evaluate, and attributes sources for the explanation.
func (e evaluator) evaluate(ctx context.Context, conn *Connection, subject dataaccess.Subject, member bool, permission dataaccess.Permission, r Resource) Entry {
	entry := Entry{Subject: &subject, Resource: r, Permission: permission, Enforcement: EnforcementServer}
	if conn == nil {
		entry.Enforcement = EnforcementNone
		return indeterminate(entry, CodeUnknownConnection)
	}
	if conn.Ref.Space == "" {
		entry.Enforcement = EnforcementHonestClient
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
	if len(e.snapshot.Incomplete) > 0 || len(conn.Incomplete) > 0 {
		return indeterminate(entry, CodeInputIncomplete)
	}
	if subject.Kind != dataaccess.User {
		return indeterminate(entry, CodeSubjectKindUnsupported)
	}
	if !member {
		if e.snapshot.Members == nil {
			return indeterminate(entry, CodeMembershipUnavailable)
		}
		if !slices.Contains(e.snapshot.Members, subject) {
			entry.Blockers, entry.Decision = []Blocker{{Code: CodeNotProjectMember}}, Deny
			return entry
		}
	}
	if conn.Protection == nil {
		return indeterminate(entry, CodeProtectionUnavailable)
	}
	resolution := e.resolution(conn, subject)
	var mandatory *dataaccess.ExactReadSnapshot
	if conn.Protection.ReadPosture == dataaccess.ReadExact {
		if conn.Exact == nil {
			return indeterminate(entry, CodeExactPolicyUnavailable)
		}
		projected, err := dataaccess.ProjectExactRead(ctx, resolution, conn.Exact)
		if err != nil {
			return indeterminate(entry, CodeExactPolicyUnavailable)
		}
		resolution, mandatory = projected, conn.Exact
	}
	object := dataaccess.ObjectRef{Namespace: r.Namespace, Name: r.Name}
	inspection, err := e.inspect(ctx, resolution, mandatory, object)
	if err != nil {
		return indeterminate(entry, CodeEvaluationUnavailable)
	}
	var selfDecided []string
	entry.Sources, selfDecided = e.sources(conn, resolution, subject, object)
	if inspection.Admitted {
		if inspection.Assessment.Outcome != access.AssessmentAllow || len(entry.Sources) == 0 {
			entry.Sources = nil
			return indeterminate(entry, CodeEvaluationInconsistent)
		}
		until := inspection.Until
		entry.Decision, entry.AllowedUntil = Allow, &until
		if conn.Protection.ReadOnly {
			entry.Restrictions = []Restriction{{Kind: "protection", ID: conn.Protection.ID, Effect: "read-only"}}
		}
		return entry
	}
	entry.Decision = Deny
	entry.Blockers = []Blocker{{Code: inspection.Code, Source: &Ref{Kind: "authority", ID: conn.Ref.ID}}}
	for _, assessed := range inspection.Assessment.Policies {
		// DALgo may report one policy again through a canonical native-table
		// delegate; a blocker is listed once, at its first position.
		blocker := Blocker{Code: string(assessed.Decision.Code), Source: &Ref{Kind: "policy", ID: assessed.Decision.Policy}}
		if !assessed.Decision.Allowed && !slices.ContainsFunc(entry.Blockers, func(b Blocker) bool {
			return b.Code == blocker.Code && *b.Source == *blocker.Source
		}) {
			entry.Blockers = append(entry.Blockers, blocker)
		}
	}
	for _, id := range selfDecided {
		entry.Blockers = append(entry.Blockers, Blocker{Code: CodeSelfDecided, Source: &Ref{Kind: SourceGrant, ID: id}})
	}
	return entry
}

func indeterminate(entry Entry, code string) Entry {
	entry.Decision = Indeterminate
	entry.Blockers = []Blocker{{Code: code}}
	return entry
}

// resolution is what the trusted resolver returns for this subject and
// connection before exact projection: approved grants unexpired at AsOf.
func (e evaluator) resolution(conn *Connection, subject dataaccess.Subject) dataaccess.Resolution {
	r := dataaccess.Resolution{Subject: subject, Connection: conn.Ref, Protection: *conn.Protection, Grants: []dataaccess.Grant{},
		ServerTime: e.snapshot.AsOf, ValidUntil: e.snapshot.AsOf.Add(AnswerLease)}
	for _, g := range conn.Grants {
		if e.active(g, subject) && !selfDecided(conn, g) {
			r.Grants = append(r.Grants, g)
		}
	}
	return r
}

// active reports an approved grant for subject that is unexpired at AsOf.
// A grant expiring exactly at AsOf is expired.
func (e evaluator) active(g dataaccess.Grant, subject dataaccess.Subject) bool {
	return g.Subject == subject && g.State == dataaccess.Approved && g.ExpiresAt.After(e.snapshot.AsOf)
}

// selfDecided reports a grant decided by its own beneficiary on a connection
// whose policy does not allow self-approval. It confers nothing (R-1).
func selfDecided(conn *Connection, g dataaccess.Grant) bool {
	return g.Decision != nil && g.Decision.By == g.Subject && !conn.AllowSelfApproval
}

// sources attributes the contributing allows the compiler reads: posture all,
// the projected exact baseline, and active grants covering the object. Under
// exact-v1 a connection-wide grant contributes nothing, as in the compiler.
// It also returns, sorted, the IDs of covering grants excluded as self-decided.
func (e evaluator) sources(conn *Connection, resolution dataaccess.Resolution, subject dataaccess.Subject, object dataaccess.ObjectRef) ([]Source, []string) {
	var sources []Source
	posture := conn.Protection.ReadPosture
	if posture == dataaccess.ReadAll {
		sources = append(sources, Source{Kind: SourcePosture, ID: conn.Ref.ID, Target: string(dataaccess.ReadAll)})
	}
	if resolution.ExactRead != nil && slices.Contains(resolution.ExactRead.Baseline, object) {
		sources = append(sources, Source{Kind: SourceBaseline, ID: conn.Ref.ID, Target: "subject"})
	}
	var grants []Source
	var excluded []string
	for _, g := range conn.Grants {
		covers := slices.Contains(g.Scope.Objects, object) || (len(g.Scope.Objects) == 0 && posture != dataaccess.ReadExact)
		if !e.active(g, subject) || g.Scope.Connection != conn.Ref.ID || !covers {
			continue
		}
		if selfDecided(conn, g) {
			excluded = append(excluded, g.ID)
		} else {
			expires := g.ExpiresAt
			grants = append(grants, Source{Kind: SourceGrant, ID: g.ID, Target: "subject", ExpiresAt: &expires})
		}
	}
	slices.SortFunc(grants, func(a, b Source) int {
		return cmp.Or(a.ExpiresAt.Compare(*b.ExpiresAt), cmp.Compare(a.ID, b.ID))
	})
	slices.Sort(excluded)
	return append(sources, grants...), excluded
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

func objectsFor(conn Connection, subject dataaccess.Subject) []dataaccess.ObjectRef {
	objects := slices.Clone(conn.Inventory)
	for _, g := range conn.Grants {
		if g.Subject == subject && g.Scope.Connection == conn.Ref.ID {
			objects = append(objects, g.Scope.Objects...)
		}
	}
	slices.SortFunc(objects, func(a, b dataaccess.ObjectRef) int {
		return cmp.Or(cmp.Compare(a.Namespace, b.Namespace), cmp.Compare(a.Name, b.Name))
	})
	return slices.Compact(objects)
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
