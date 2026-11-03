package perimeter

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
	"github.com/datatug/datatug-core/pkg/dataaccess"
	"github.com/stretchr/testify/require"
)

// recordingSource is the fake adapter: it records whether a read passed every
// Core and DALgo check and reached native dispatch. It never returns a row.
type recordingSource struct{ dispatched bool }

func (s *recordingSource) OpenTableRead(_ context.Context, request dataaccess.TableIdentityRequest) (dataaccess.TableReadSpan, error) {
	return recordingSpan{source: s, request: request}, nil
}

type recordingSpan struct {
	source  *recordingSource
	request dataaccess.TableIdentityRequest
}

func (s recordingSpan) Session() dal.ReadSession { return recordingSession{source: s.source} }
func (s recordingSpan) Verify(context.Context, dataaccess.TableIdentityRequest) (dataaccess.TableIdentityFacts, error) {
	return dataaccess.TableIdentityFacts{Connection: s.request.Connection, Table: s.request.Table}, nil
}
func (recordingSpan) Close() error { return nil }

type recordingSession struct {
	dal.ReadSession
	source *recordingSource
}

func (s recordingSession) ExecuteQueryToRecordsetReader(context.Context, dal.Query, ...recordset.Option) (dal.RecordsetReader, error) {
	s.source.dispatched = true
	return nil, errors.New("recording adapter returns no rows")
}

// backendResolver is written from the backend, not from the evaluator. It
// mirrors datatug/backend at f9fc86835e3555555049e2eae05654b3a3aa6b98:
//   - facade4datatug/access_control.go:117-140 projectRoles: a non-member is refused.
//   - facade4datatug/access_control_reads.go:91-125 listAccessGrants(ownOnly):
//     query by subject realm and ID, refuse more than 100 rows, refuse a row
//     whose Subject differs from the actor.
//   - facade4datatug/access_control_reads.go:198-250 ResolveAccessWithExactSnapshot:
//     ValidUntil = min(at+1m, actor.ValidUntil); every listed grant's
//     fingerprint must equal the connection's (access_control.go:160); keep
//     approved grants with ExpiresAt after at; under posture none clamp
//     ValidUntil to the latest kept expiry; ProjectExactRead when an exact
//     configuration is stored.
func backendResolver(s Snapshot, conn Connection, actorValidUntil time.Time) dataaccess.Resolver {
	return func(ctx context.Context, request dataaccess.ResolveRequest) (dataaccess.Resolution, error) {
		subject := request.Subject
		if !slices.Contains(s.Members, subject) {
			return dataaccess.Resolution{}, errors.New("projectRoles: not a project member")
		}
		at := s.AsOf
		r := dataaccess.Resolution{Subject: subject, Connection: conn.Ref, Protection: *conn.Protection, Grants: []dataaccess.Grant{},
			ServerTime: at, ValidUntil: minTime(at.Add(time.Minute), actorValidUntil)}
		var listed []StoredGrant
		for _, g := range conn.Grants {
			if g.Subject.Realm == subject.Realm && g.Subject.ID == subject.ID {
				listed = append(listed, g)
			}
		}
		if len(listed) > 100 {
			return dataaccess.Resolution{}, errors.New("listAccessGrants: more than 100")
		}
		var latest time.Time
		for _, g := range listed {
			if g.Subject != subject {
				return dataaccess.Resolution{}, errors.New("listAccessGrants: subject mismatch")
			}
			f, c := g.Fingerprint, conn.Ref.Fingerprint
			if f.Engine != c.Engine || f.Database != c.Database || f.EngineIdentity != c.EngineIdentity || (f.EngineIdentity == "" && (f.Address == "" || f.Address != c.Address)) {
				return dataaccess.Resolution{}, errors.New("accessSameFingerprint")
			}
			if g.State == dataaccess.Approved && g.ExpiresAt.After(at) {
				r.Grants = append(r.Grants, g.Grant)
				if g.ExpiresAt.After(latest) {
					latest = g.ExpiresAt
				}
			}
		}
		if conn.Protection.ReadPosture == dataaccess.ReadNone && !latest.IsZero() {
			r.ValidUntil = minTime(r.ValidUntil, latest)
		}
		if conn.Exact != nil {
			return dataaccess.ProjectExactRead(ctx, r, conn.Exact)
		}
		return r, nil
	}
}

func minTime(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}

// admitted runs the real enforcement entry point, Authority.ReadTableResult,
// behind backendResolver, with the sealed mandatory policy bound into the
// execution context as hosts do. Admission means native dispatch was reached.
func admitted(t *testing.T, s Snapshot, conn Connection, subject dataaccess.Subject, object dataaccess.ObjectRef) bool {
	t.Helper()
	if conn.Protection == nil {
		return false // the backend cannot read the protection record
	}
	authority, err := dataaccess.NewAuthority(subject, conn.Ref, backendResolver(s, conn, s.AsOf.Add(time.Hour)))
	if err != nil {
		return false
	}
	ctx := context.Background()
	if conn.Exact != nil {
		if ctx, err = conn.Exact.BindContext(ctx, subject); err != nil {
			return false
		}
	}
	source := &recordingSource{}
	request := dataaccess.ResultRequest{Nonce: "perimeter-parity-nonce", RequestSHA256: strings.Repeat("a", 64)}
	result, err := authority.ReadTableResult(ctx, request, dataaccess.TableRead{Object: object, MaxRows: 1, MaxBytes: 4096}, source)
	require.Error(t, err, "the recording adapter never completes a read")
	require.Nil(t, result)
	return source.dispatched
}

// agree is the parity contract. Allow implies admission, always. Deny implies
// refusal, and an unknown never hides an admitted read, unless the case is
// one where the evaluator is documented as stricter than Core (stricter).
func agree(t *testing.T, label string, decision Decision, real, stricter bool) {
	t.Helper()
	switch decision {
	case Allow:
		require.Truef(t, real, "%s: FALSE ALLOW, enforcement refuses", label)
	default:
		require.Truef(t, !real || stricter, "%s: evaluator %s but enforcement admits", label, decision)
	}
}

var agent = dataaccess.Subject{Realm: "sneat", Kind: dataaccess.Agent, ID: "bot"}

type parityCase struct {
	snapshot Snapshot
	// stricter lists subjects for whom the evaluator may refuse what Core
	// admits: self-decided grants (R-1), agents, unreadable inputs.
	stricter func(dataaccess.Subject) bool
}

func onlyFor(subjects ...dataaccess.Subject) func(dataaccess.Subject) bool {
	return func(s dataaccess.Subject) bool { return slices.Contains(subjects, s) }
}

func everyone(dataaccess.Subject) bool { return true }

// checkParity compares Explain, ForSubject and ForResource rows for every
// subject, connection and table against real admission.
func checkParity(t *testing.T, label string, c parityCase) (cases, allows int) {
	t.Helper()
	ctx := context.Background()
	stricter := func(s dataaccess.Subject) bool {
		return s.Kind != dataaccess.User || (c.stricter != nil && c.stricter(s))
	}
	subjects := []dataaccess.Subject{red, green, blue, mallory, agent}
	tables := append(slices.Clone(objects), dataaccess.ObjectRef{Namespace: "billing", Name: "ghost"})
	for _, conn := range c.snapshot.Connections {
		real := map[string]bool{}
		for _, subject := range subjects {
			for _, object := range tables {
				label := fmt.Sprintf("%s/%s/%s/%s.%s", label, conn.Ref.ID, subject.ID, object.Namespace, object.Name)
				r := Resource{Connection: conn.Ref.ID, Namespace: object.Namespace, Name: object.Name}
				admit := admitted(t, c.snapshot, conn, subject, object)
				real[subject.ID+"/"+object.Name] = admit
				answer, err := Explain(ctx, c.snapshot, subject, dataaccess.DataRead, r, ExplainOptions{Detail: true})
				require.NoError(t, err)
				agree(t, label, answer.Entries[0].Decision, admit, stricter(subject))
				cases++
				if admit {
					allows++
				}
			}
			view, err := ForSubject(ctx, c.snapshot, subject, Filter{Connections: []string{conn.Ref.ID}, IncludeDenied: true})
			require.NoError(t, err)
			for _, entry := range view.Entries {
				admit, known := real[subject.ID+"/"+entry.Resource.Name]
				if !known {
					admit = admitted(t, c.snapshot, conn, subject, dataaccess.ObjectRef{Namespace: entry.Resource.Namespace, Name: entry.Resource.Name})
				}
				agree(t, label+"/ForSubject/"+subject.ID+"/"+entry.Resource.Name, entry.Decision, admit, stricter(subject))
			}
		}
		for _, object := range objects {
			who, err := ForResource(ctx, c.snapshot, Resource{Connection: conn.Ref.ID, Namespace: object.Namespace, Name: object.Name}, nil)
			require.NoError(t, err)
			listed := map[dataaccess.Subject]Decision{}
			var group *Entry
			for i, entry := range who.Entries {
				if entry.Subject == nil {
					group = &who.Entries[i]
					continue
				}
				listed[*entry.Subject] = entry.Decision
			}
			for _, subject := range subjects {
				decision, ok := listed[subject]
				if !ok {
					if group == nil {
						continue // not a candidate under posture none or exact-v1
					}
					if !slices.Contains(c.snapshot.Members, subject) {
						continue // the group row speaks for members only
					}
					decision = group.Decision
				}
				agree(t, fmt.Sprintf("%s/ForResource/%s/%s", label, subject.ID, object.Name), decision, real[subject.ID+"/"+object.Name], stricter(subject))
			}
		}
	}
	return cases, allows
}

// TestViewsMatchEnforcement is contract test T-C1 over a hand-built matrix.
func TestViewsMatchEnforcement(t *testing.T) {
	active, hour := asOf.Add(30*time.Minute), asOf.Add(time.Hour)
	kind := grant("g-kind", blue, active, invoices)
	kind.Subject.Kind = dataaccess.Service
	moved := grant("g-fp", blue, active, invoices)
	moved.Fingerprint.EngineIdentity = "cluster-b"
	elsewhere := grant("g-else", blue, active, invoices)
	elsewhere.Scope.Connection = "uat-orders"
	var many []StoredGrant
	for i := range 101 {
		many = append(many, grant(fmt.Sprintf("g-%03d", i), blue, asOf.Add(-time.Hour), invoices))
	}
	type variant struct {
		grants   []StoredGrant
		mutate   func(*Snapshot)
		stricter func(dataaccess.Subject) bool
	}
	variants := map[string]variant{
		"no grants":                          {},
		"object grant":                       {grants: []StoredGrant{grant("g1", blue, active, invoices)}},
		"connection grant":                   {grants: []StoredGrant{grant("g-conn", blue, active)}},
		"two grants":                         {grants: []StoredGrant{grant("g1", blue, active, invoices), grant("g2", blue, hour, customers, invoices)}},
		"expires exactly at AsOf":            {grants: []StoredGrant{grant("g-edge", blue, asOf, invoices)}},
		"expired 1ns before":                 {grants: []StoredGrant{grant("g-old", blue, asOf.Add(-time.Nanosecond), invoices)}},
		"revoked":                            {grants: []StoredGrant{withState(grant("g-rev", blue, active, invoices), dataaccess.Revoked)}},
		"refused":                            {grants: []StoredGrant{withState(grant("g-ref", blue, active, invoices), dataaccess.Refused)}},
		"requested":                          {grants: []StoredGrant{withState(grant("g-req", blue, active, invoices), dataaccess.Requested)}},
		"withdrawn":                          {grants: []StoredGrant{withState(grant("g-wd", blue, active, invoices), dataaccess.Withdrawn)}},
		"grant under mandatory":              {grants: []StoredGrant{grant("g-sec", blue, active, secrets)}},
		"non-member grant":                   {grants: []StoredGrant{grant("g-mal", mallory, active, invoices)}},
		"unsupported grant scope":            {grants: []StoredGrant{unsupported(grant("g-bad", blue, active, invoices))}},
		"revoked unsupported grant":          {grants: []StoredGrant{withState(unsupported(grant("g-bad", green, active, invoices)), dataaccess.Revoked)}},
		"expired unsupported grant":          {grants: []StoredGrant{unsupported(grant("g-bad", green, asOf, invoices))}},
		"self-decided grant":                 {grants: []StoredGrant{selfDecided(grant("g-self", blue, active, invoices))}, stricter: onlyFor(blue)},
		"self-decided unsupported (H-1)":     {grants: []StoredGrant{selfDecided(unsupported(grant("g-sx", green, active, invoices)))}},
		"self-decided unsupported plus good": {grants: []StoredGrant{selfDecided(unsupported(grant("g-sx", green, active, invoices))), grant("g-ok", green, active, invoices)}},
		"self-approval allowed": {grants: []StoredGrant{selfDecided(grant("g-self", blue, active, invoices))},
			mutate: func(s *Snapshot) { s.Connections[0].AllowSelfApproval = true }},
		"grant for another connection": {grants: []StoredGrant{elsewhere}},
		"grant fingerprint mismatch":   {grants: []StoredGrant{moved}},
		"grant subject kind mismatch":  {grants: []StoredGrant{kind}},
		"more than 100 grants":         {grants: many},
		"members unreadable":           {mutate: func(s *Snapshot) { s.Members = nil }},
		"snapshot incomplete":          {mutate: func(s *Snapshot) { s.Incomplete = []string{"teams"} }, stricter: everyone},
		"connection incomplete":        {mutate: func(s *Snapshot) { s.Connections[0].Incomplete = []string{"grants"} }, stricter: everyone},
		"protection unreadable":        {mutate: func(s *Snapshot) { s.Connections[0].Protection = nil }},
		"unknown posture":              {mutate: func(s *Snapshot) { s.Connections[0].Protection.ReadPosture = "maybe" }},
		"unknown agent read":           {mutate: func(s *Snapshot) { s.Connections[0].Protection.AgentRead = "maybe" }},
		"agent read none":              {mutate: func(s *Snapshot) { s.Connections[0].Protection.AgentRead = dataaccess.AgentNone }},
		"writable connection":          {mutate: func(s *Snapshot) { s.Connections[0].Protection.ReadOnly = false }},
		"mandatory fine-grained":       {grants: []StoredGrant{grant("g1", blue, active, invoices)}, mutate: func(s *Snapshot) { s.Connections[0].Protection.MandatoryFineGrained = true }},
		"stale exact configuration": {mutate: func(s *Snapshot) {
			if s.Connections[0].Exact == nil {
				s.Connections[0].Exact = exactSnapshot(t)
			}
		}},
		"exact configuration missing": {mutate: func(s *Snapshot) { s.Connections[0].Exact = nil }},
		"two connections": {grants: []StoredGrant{grant("g1", blue, active, invoices)}, mutate: func(s *Snapshot) {
			uat := s.Connections[0]
			uat.Ref.ID = "uat-orders"
			uat.Ref.Fingerprint.EngineIdentity = "cluster-uat"
			protection := *uat.Protection
			protection.ID, protection.Fingerprint, protection.ReadPosture = uat.Ref.ID, uat.Ref.Fingerprint, dataaccess.ReadNone
			uat.Protection, uat.Exact = &protection, nil
			g := grant("g-uat", red, active, customers)
			g.Scope.Connection, g.Fingerprint = uat.Ref.ID, uat.Ref.Fingerprint
			uat.Grants = []StoredGrant{g}
			s.Connections = append(s.Connections, uat)
		}},
	}
	cases, allows := 0, 0
	for name, v := range variants {
		for _, posture := range []dataaccess.ReadPosture{dataaccess.ReadAll, dataaccess.ReadNone, dataaccess.ReadExact} {
			s := snapshot(t, posture, v.grants...)
			if v.mutate != nil {
				v.mutate(&s)
			}
			c, a := checkParity(t, name+"/"+string(posture), parityCase{snapshot: s, stricter: v.stricter})
			cases, allows = cases+c, allows+a
		}
	}
	require.Positive(t, allows, "the matrix must exercise admitted reads")
	require.Less(t, allows, cases, "the matrix must exercise refused reads")
}

// TestReviewerProbeSelfDecidedInvalidGrant is the H-1 probe: green holds a
// self-decided approved grant with an unsupported scope, alone and next to a
// valid grant. Core refuses the whole resolution, so nothing may be allowed.
func TestReviewerProbeSelfDecidedInvalidGrant(t *testing.T) {
	bad := selfDecided(unsupported(grant("g-sx", green, asOf.Add(30*time.Minute), invoices)))
	good := grant("g-ok", green, asOf.Add(30*time.Minute), invoices)
	for _, grants := range [][]StoredGrant{{bad}, {bad, good}} {
		for _, posture := range []dataaccess.ReadPosture{dataaccess.ReadAll, dataaccess.ReadNone, dataaccess.ReadExact} {
			s := snapshot(t, posture, grants...)
			for _, object := range objects {
				answer, err := Explain(context.Background(), s, green, dataaccess.DataRead, resource(object), ExplainOptions{})
				require.NoError(t, err)
				require.False(t, admitted(t, s, s.Connections[0], green, object))
				require.NotEqual(t, Allow, answer.Entries[0].Decision, "%s/%s", posture, object.Name)
			}
		}
	}
}

// TestGeneratedGrantSetsMatchEnforcement is a seeded property test over grant
// states, expiry at AsOf and ±1 ns, self- or other-decided, and supported or
// unsupported scope, for two subjects under every posture.
//
// The wall-clock Authority cannot hold a 1 ns admission window open, so the
// enforcement side sees a grant expiring at AsOf+1ns as AsOf+30m: both are
// "active at AsOf". The evaluator always sees the true value.
func TestGeneratedGrantSetsMatchEnforcement(t *testing.T) {
	rng := rand.New(rand.NewPCG(20261103, 1))
	states := []dataaccess.GrantState{dataaccess.Approved, dataaccess.Approved, dataaccess.Revoked, dataaccess.Refused, dataaccess.Requested, dataaccess.Withdrawn}
	offsets := []time.Duration{-time.Nanosecond, 0, time.Nanosecond, 30 * time.Minute}
	postures := []dataaccess.ReadPosture{dataaccess.ReadAll, dataaccess.ReadNone, dataaccess.ReadExact}
	for i := range 150 {
		var evaluated, enforced []StoredGrant
		strict := map[dataaccess.Subject]bool{}
		selfApproval := rng.IntN(4) == 0
		for j := range 1 + rng.IntN(3) {
			subject := []dataaccess.Subject{blue, green}[rng.IntN(2)]
			var scope []dataaccess.ObjectRef
			if rng.IntN(3) > 0 {
				scope = []dataaccess.ObjectRef{objects[rng.IntN(len(objects))]}
			}
			g := withState(grant(fmt.Sprintf("g%d", j), subject, asOf.Add(offsets[rng.IntN(len(offsets))]), scope...), states[rng.IntN(len(states))])
			if rng.IntN(4) == 0 {
				g = unsupported(g)
			}
			if rng.IntN(3) == 0 {
				g = selfDecided(g)
				strict[subject] = strict[subject] || !selfApproval
			}
			evaluated = append(evaluated, g)
			if g.ExpiresAt.Equal(asOf.Add(time.Nanosecond)) {
				g.ExpiresAt = asOf.Add(30 * time.Minute)
			}
			enforced = append(enforced, g)
		}
		posture := postures[rng.IntN(len(postures))]
		s := snapshot(t, posture, evaluated...)
		s.Connections[0].AllowSelfApproval = selfApproval
		e := snapshot(t, posture, enforced...)
		e.Connections[0].AllowSelfApproval = selfApproval
		for _, subject := range []dataaccess.Subject{blue, green} {
			for _, object := range objects {
				answer, err := Explain(context.Background(), s, subject, dataaccess.DataRead, resource(object), ExplainOptions{})
				require.NoError(t, err)
				agree(t, fmt.Sprintf("case %d/%s/%s/%s %+v", i, posture, subject.ID, object.Name, evaluated), answer.Entries[0].Decision,
					admitted(t, e, e.Connections[0], subject, object), strict[subject])
			}
		}
	}
}
