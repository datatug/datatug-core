package perimeter

import (
	"context"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/dal-go/dalgo/access"
	"github.com/datatug/datatug-core/pkg/dataaccess"
	"github.com/stretchr/testify/require"
)

func explain(t *testing.T, s Snapshot, subject dataaccess.Subject, permission dataaccess.Permission, object dataaccess.ObjectRef) Entry {
	t.Helper()
	answer, err := Explain(context.Background(), s, subject, permission, resource(object), ExplainOptions{Detail: true})
	require.NoError(t, err)
	require.Len(t, answer.Entries, 1)
	return answer.Entries[0]
}

func codes(blockers []Blocker) []string {
	out := make([]string, len(blockers))
	for i, b := range blockers {
		out[i] = b.Code
	}
	return out
}

func sourceIDs(sources []Source) []string {
	out := make([]string, len(sources))
	for i, s := range sources {
		out[i] = s.Kind + ":" + s.ID
	}
	return out
}

func TestExplainGrantAllowWithSourcesAndRestrictions(t *testing.T) {
	g := grant("g1", blue, asOf.Add(30*time.Minute), invoices)
	entry := explain(t, snapshot(t, dataaccess.ReadNone, g), blue, dataaccess.DataRead, invoices)
	require.Equal(t, Allow, entry.Decision)
	require.Equal(t, &blue, entry.Subject)
	require.Equal(t, EnforcementServer, entry.Enforcement)
	require.Equal(t, []Source{{Kind: SourceGrant, ID: "g1", Target: "subject", ExpiresAt: &g.ExpiresAt}}, entry.Sources)
	require.Equal(t, []Restriction{{Kind: "protection", ID: connID, Effect: "read-only"}}, entry.Restrictions)
	require.Empty(t, entry.Blockers)
	require.Equal(t, asOf.Add(AnswerLease), *entry.AllowedUntil, "the lease never outlives the answer")

	entry = explain(t, snapshot(t, dataaccess.ReadNone, g), red, dataaccess.DataRead, invoices)
	require.Equal(t, Deny, entry.Decision, "a personal overlay never reaches another member")
	require.Equal(t, []string{"READ_DENIED", string(access.CodeNoMatch)}, codes(entry.Blockers))
	require.Equal(t, &Ref{Kind: "authority", ID: connID}, entry.Blockers[0].Source)
	require.Equal(t, &Ref{Kind: "policy", ID: "datatug-core-read-composition"}, entry.Blockers[1].Source)
	require.Empty(t, entry.Sources)
	require.Empty(t, entry.Candidates)

	s := snapshot(t, dataaccess.ReadNone, g)
	s.Connections[0].Protection.ReadOnly = false
	require.Empty(t, explain(t, s, blue, dataaccess.DataRead, invoices).Restrictions)
}

func TestExplanationChainOrderIsDeterministic(t *testing.T) {
	late := grant("g2", blue, asOf.Add(time.Hour), customers, invoices)
	early := grant("g1", blue, asOf.Add(30*time.Minute), invoices)
	tie := grant("g0", blue, asOf.Add(30*time.Minute), invoices)
	connWide := grant("g-conn", blue, asOf.Add(20*time.Minute))
	other := grant("g-red", red, asOf.Add(time.Hour), invoices)
	s := snapshot(t, dataaccess.ReadAll, late, early, other, tie, connWide)
	want := []string{"posture:prod-billing", "grant:g-conn", "grant:g0", "grant:g1", "grant:g2"}
	for range 3 {
		entry := explain(t, s, blue, dataaccess.DataRead, invoices)
		require.Equal(t, Allow, entry.Decision)
		require.Equal(t, want, sourceIDs(entry.Sources))
	}

	exact := snapshot(t, dataaccess.ReadExact, grant("g-c", green, asOf.Add(time.Hour), customers), grant("g-wide", green, asOf.Add(time.Hour)))
	entry := explain(t, exact, green, dataaccess.DataRead, customers)
	require.Equal(t, Allow, entry.Decision)
	require.Equal(t, []string{"baseline:prod-billing", "grant:g-c"}, sourceIDs(entry.Sources), "a connection-wide grant contributes nothing under exact-v1")
}

func TestMandatoryDenyBeatsGrant(t *testing.T) {
	s := snapshot(t, dataaccess.ReadExact, grant("g-sec", blue, asOf.Add(time.Hour), secrets), grant("g-inv", blue, asOf.Add(time.Hour), invoices))
	entry := explain(t, s, blue, dataaccess.DataRead, secrets)
	require.Equal(t, Deny, entry.Decision)
	require.Empty(t, entry.Sources, "a deny never claims an allow source")
	require.Empty(t, entry.Candidates, "projection already removed the mandatory-denied object from the grant")
	require.Equal(t, []string{"READ_DENIED", string(access.CodeRuleDenied), string(access.CodeNoMatch)}, codes(entry.Blockers))
	require.Equal(t, &Ref{Kind: "policy", ID: "datatug-exact-mandatory"}, entry.Blockers[1].Source)
	require.Equal(t, entry, explain(t, s, blue, dataaccess.DataRead, secrets), "blocker order is deterministic")

	require.Equal(t, Allow, explain(t, s, blue, dataaccess.DataRead, invoices).Decision)
	require.Equal(t, Deny, explain(t, s, green, dataaccess.DataRead, invoices).Decision, "requestable is not readable")

	fine := snapshot(t, dataaccess.ReadAll, grant("g1", blue, asOf.Add(time.Hour), invoices))
	fine.Connections[0].Protection.MandatoryFineGrained = true
	entry = explain(t, fine, blue, dataaccess.DataRead, invoices)
	require.Equal(t, Indeterminate, entry.Decision, "a protection Core cannot enforce is unknown, never allowed")
	require.Equal(t, []Blocker{{Code: "AUTHORITY_UNSUPPORTED"}}, entry.Blockers)
	require.Empty(t, entry.Sources)
}

func TestExpiryIsMeasuredAgainstSnapshotTime(t *testing.T) {
	for name, tc := range map[string]struct {
		expiresAt time.Time
		want      Decision
	}{
		"expires exactly at AsOf":   {asOf, Deny},
		"expired one ns before":     {asOf.Add(-time.Nanosecond), Deny},
		"one ns left":               {asOf.Add(time.Nanosecond), Allow},
		"maximum production grant":  {asOf.Add(time.Hour), Allow},
		"long past (wall clock ok)": {time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC), Deny},
	} {
		t.Run(name, func(t *testing.T) {
			entry := explain(t, snapshot(t, dataaccess.ReadNone, grant("g", blue, tc.expiresAt, invoices)), blue, dataaccess.DataRead, invoices)
			require.Equal(t, tc.want, entry.Decision)
			if tc.want == Allow {
				want := asOf.Add(AnswerLease)
				if tc.expiresAt.Before(want) {
					want = tc.expiresAt
				}
				require.Equal(t, want, *entry.AllowedUntil)
			} else {
				require.Nil(t, entry.AllowedUntil)
				require.Empty(t, entry.Sources, "an expired grant is not counted")
			}
		})
	}
	s := snapshot(t, dataaccess.ReadNone, grant("g", blue, asOf.Add(time.Hour), invoices))
	require.Equal(t, Allow, explain(t, s, blue, dataaccess.DataRead, invoices).Decision)
	s.AsOf = asOf.Add(time.Hour)
	require.Equal(t, Deny, explain(t, s, blue, dataaccess.DataRead, invoices).Decision, "advancing the injected clock to expiry retracts access")
}

func TestUnsupportedPermissionsFailClosed(t *testing.T) {
	s := snapshot(t, dataaccess.ReadAll, grant("g", blue, asOf.Add(time.Hour)))
	for _, tc := range []struct {
		permission dataaccess.Permission
		decision   Decision
		blockers   []Blocker
	}{
		{dataaccess.DataInsert, Deny, []Blocker{{Code: CodeUnsupportedScope, Phase: "P3"}, {Code: CodeReadOnlyConnection, Source: &Ref{Kind: "protection", ID: connID}}}},
		{dataaccess.DataUpdate, Deny, []Blocker{{Code: CodeUnsupportedScope, Phase: "P3"}, {Code: CodeReadOnlyConnection, Source: &Ref{Kind: "protection", ID: connID}}}},
		{dataaccess.DataDelete, Deny, []Blocker{{Code: CodeUnsupportedScope, Phase: "P3"}, {Code: CodeReadOnlyConnection, Source: &Ref{Kind: "protection", ID: connID}}}},
		{dataaccess.DataWrite, Deny, []Blocker{{Code: CodeUnsupportedScope, Phase: "P3"}, {Code: CodeReadOnlyConnection, Source: &Ref{Kind: "protection", ID: connID}}}},
		{dataaccess.DataReveal, Deny, []Blocker{{Code: CodeUnsupportedScope, Phase: "L3"}}},
		{dataaccess.QueryRaw, Deny, []Blocker{{Code: CodeUnsupportedScope, Phase: "raw"}}},
		{dataaccess.QueryTugQL, Indeterminate, []Blocker{{Code: CodePermissionNotModelled}}},
		{"data.purge", Indeterminate, []Blocker{{Code: CodeUnknownPermission}}},
	} {
		entry := explain(t, s, blue, tc.permission, invoices)
		require.Equal(t, tc.decision, entry.Decision, tc.permission)
		require.Equal(t, tc.blockers, entry.Blockers, tc.permission)
		require.Empty(t, entry.Sources)
	}
	s.Connections[0].Protection.ReadOnly = false
	require.Equal(t, []Blocker{{Code: CodeUnsupportedScope, Phase: "P3"}}, explain(t, s, blue, dataaccess.DataUpdate, invoices).Blockers)
	s.Connections[0].Protection = nil
	require.Equal(t, Deny, explain(t, s, blue, dataaccess.DataDelete, invoices).Decision, "a write is refused even when protection is unreadable")
}

func TestMissingOrUnsupportedInputsAreIndeterminate(t *testing.T) {
	base := func() Snapshot { return snapshot(t, dataaccess.ReadAll) }
	for name, tc := range map[string]struct {
		mutate  func(*Snapshot)
		subject dataaccess.Subject
		object  dataaccess.ObjectRef
		code    string
	}{
		"snapshot incomplete":   {func(s *Snapshot) { s.Incomplete = []string{"teams"} }, blue, invoices, CodeInputIncomplete},
		"connection incomplete": {func(s *Snapshot) { s.Connections[0].Incomplete = []string{"grants"} }, blue, invoices, CodeInputIncomplete},
		"agent subject":         {func(*Snapshot) {}, dataaccess.Subject{Realm: "sneat", Kind: dataaccess.Agent, ID: "bot"}, invoices, CodeSubjectKindUnsupported},
		"membership unreadable": {func(s *Snapshot) { s.Members = nil }, blue, invoices, CodeMembershipUnavailable},
		"protection unreadable": {func(s *Snapshot) { s.Connections[0].Protection = nil }, blue, invoices, CodeProtectionUnavailable},
		"exact without policy": {func(s *Snapshot) {
			s.Connections[0].Protection.ReadPosture = dataaccess.ReadExact
		}, green, customers, "AUTHORITY_UNSUPPORTED"},
		"exact projection refused": {func(s *Snapshot) {
			*s = snapshot(t, dataaccess.ReadExact, withScope(grant("g", green, asOf.Add(time.Hour), customers)))
		}, green, customers, CodeExactPolicyUnavailable},
		"object not canonical": {func(*Snapshot) {}, blue, dataaccess.ObjectRef{Namespace: "billing", Name: "*"}, CodeEvaluationUnavailable},
		"local connection": {func(s *Snapshot) {
			s.Connections[0].Ref.Space, s.Connections[0].Ref.LocalAuthorityID = "", "local"
		}, blue, invoices, CodeLocalConnection},
		"invalid grant (Core refusal)": {func(s *Snapshot) {
			*s = snapshot(t, dataaccess.ReadAll, unsupported(grant("g", blue, asOf.Add(time.Hour), invoices)))
		}, blue, invoices, CodeUnsupportedScope},
	} {
		t.Run(name, func(t *testing.T) {
			s := base()
			tc.mutate(&s)
			entry := explain(t, s, tc.subject, dataaccess.DataRead, tc.object)
			require.Equal(t, Indeterminate, entry.Decision)
			require.Equal(t, []Blocker{{Code: tc.code}}, entry.Blockers)
			require.Nil(t, entry.AllowedUntil)
		})
	}

	entry := explain(t, base(), mallory, dataaccess.DataRead, invoices)
	require.Equal(t, Deny, entry.Decision)
	require.Equal(t, []Blocker{{Code: CodeNotProjectMember}}, entry.Blockers)

	s := base()
	s.Connections[0].Ref.Space, s.Connections[0].Ref.LocalAuthorityID = "", "local"
	require.Equal(t, EnforcementHonestClient, explain(t, s, blue, dataaccess.DataRead, invoices).Enforcement)

	answer, err := Explain(context.Background(), base(), blue, dataaccess.DataRead, Resource{Connection: "nope", Namespace: "billing", Name: "invoices"}, ExplainOptions{})
	require.NoError(t, err)
	require.False(t, answer.Complete)
	entry = answer.Entries[0]
	require.Equal(t, Indeterminate, entry.Decision)
	require.Empty(t, entry.Enforcement)
	require.Equal(t, []Blocker{{Code: CodeUnknownConnection}}, entry.Blockers)
}

func withScope(g StoredGrant) StoredGrant {
	g.Scope.Fields = &dataaccess.FieldScope{Include: []string{"id"}}
	return g
}

func TestInconsistentInspectionIsIndeterminate(t *testing.T) {
	for name, tc := range map[string]struct {
		snapshot   Snapshot
		inspection dataaccess.TableReadInspection
	}{
		"admitted but assessed deny": {snapshot(t, dataaccess.ReadAll), dataaccess.TableReadInspection{Admitted: true, Assessment: dataaccess.InspectedAssessment{Outcome: access.AssessmentDeny}}},
		"admitted with no source":    {snapshot(t, dataaccess.ReadNone), dataaccess.TableReadInspection{Admitted: true, Assessment: dataaccess.InspectedAssessment{Outcome: access.AssessmentAllow}}},
	} {
		t.Run(name, func(t *testing.T) {
			e := newEvaluator(tc.snapshot)
			e.inspect = func(context.Context, dataaccess.Resolution, *dataaccess.ExactReadSnapshot, dataaccess.ObjectRef) (dataaccess.TableReadInspection, error) {
				return tc.inspection, nil
			}
			answer, err := e.explain(context.Background(), blue, dataaccess.DataRead, resource(invoices))
			require.NoError(t, err)
			entry := answer.Entries[0]
			require.Equal(t, Indeterminate, entry.Decision)
			require.Equal(t, []Blocker{{Code: CodeEvaluationInconsistent}}, entry.Blockers)
			require.Empty(t, entry.Sources)
		})
	}
}

func TestInvalidCallsAreRefused(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for name, tc := range map[string]struct {
		ctx context.Context
		s   Snapshot
	}{
		"nil context":      {nil, snapshot(t, dataaccess.ReadAll)},
		"canceled context": {canceled, snapshot(t, dataaccess.ReadAll)},
		"no snapshot time": {context.Background(), Snapshot{}},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ForSubject(tc.ctx, tc.s, blue, Filter{})
			require.Error(t, err)
			_, err = Explain(tc.ctx, tc.s, blue, dataaccess.DataRead, resource(invoices), ExplainOptions{})
			require.Error(t, err)
			_, err = ForResource(tc.ctx, tc.s, resource(invoices), nil)
			require.Error(t, err)
		})
	}
}

func TestForSubject(t *testing.T) {
	ctx := context.Background()
	s := snapshot(t, dataaccess.ReadNone, grant("g1", blue, asOf.Add(time.Hour), invoices), grant("g-red", red, asOf.Add(time.Hour), customers))
	answer, err := ForSubject(ctx, s, blue, Filter{})
	require.NoError(t, err)
	require.Equal(t, []Resource{resource(invoices)}, resources(answer), "denied names are not listed by default")
	require.Equal(t, Allow, answer.Entries[0].Decision)
	require.True(t, answer.Complete)

	answer, err = ForSubject(ctx, s, blue, Filter{IncludeDenied: true})
	require.NoError(t, err)
	require.Equal(t, asOf, answer.AsOf)
	require.Equal(t, asOf.Add(AnswerLease), answer.ValidUntil)
	require.True(t, answer.Complete)
	require.Equal(t, Limitations, answer.Limitations)
	require.Len(t, answer.Entries, 3)
	for i, want := range []struct {
		object   dataaccess.ObjectRef
		decision Decision
	}{{customers, Deny}, {invoices, Allow}, {secrets, Deny}} {
		require.Equal(t, resource(want.object), answer.Entries[i].Resource)
		require.Equal(t, want.decision, answer.Entries[i].Decision)
		require.Equal(t, dataaccess.DataRead, answer.Entries[i].Permission)
	}

	t.Run("permissions and filters", func(t *testing.T) {
		answer, err := ForSubject(ctx, s, blue, Filter{Connections: []string{connID}, Environment: "production",
			Permissions: []dataaccess.Permission{dataaccess.DataRead, dataaccess.DataUpdate}, IncludeDenied: true})
		require.NoError(t, err)
		require.Len(t, answer.Entries, 6)
		require.Equal(t, dataaccess.DataUpdate, answer.Entries[1].Permission)
		require.Equal(t, Deny, answer.Entries[1].Decision)
		for _, f := range []Filter{{Connections: []string{"other"}, IncludeDenied: true}, {Environment: "uat", IncludeDenied: true}} {
			answer, err := ForSubject(ctx, s, blue, f)
			require.NoError(t, err)
			require.Empty(t, answer.Entries)
			require.True(t, answer.Complete)
		}
	})

	t.Run("unreadable protection stays visible as unknown", func(t *testing.T) {
		broken := snapshot(t, dataaccess.ReadNone)
		broken.Connections[0].Protection = nil
		answer, err := ForSubject(ctx, broken, blue, Filter{Environment: "production"})
		require.NoError(t, err)
		require.False(t, answer.Complete)
		require.Equal(t, []Entry{{Resource: Resource{Connection: connID}, Decision: Indeterminate,
			Blockers: []Blocker{{Code: CodeResourcesNotEnumerated}}, Enforcement: EnforcementServer}}, answer.Entries, "unknowns fold into one row with no table name")
		answer, err = ForSubject(ctx, broken, blue, Filter{IncludeDenied: true})
		require.NoError(t, err)
		require.Len(t, answer.Entries, 3)
	})

	t.Run("objects come from inventory and the subject's own grants", func(t *testing.T) {
		other := dataaccess.ObjectRef{Namespace: "billing", Name: "archive"}
		s := snapshot(t, dataaccess.ReadNone, grant("g1", blue, asOf.Add(time.Hour), other, invoices), grant("g-red", red, asOf.Add(time.Hour), dataaccess.ObjectRef{Namespace: "billing", Name: "payroll"}))
		s.Connections[0].Inventory = []dataaccess.ObjectRef{invoices}
		answer, err := ForSubject(ctx, s, blue, Filter{})
		require.NoError(t, err)
		require.Equal(t, []Resource{resource(other), resource(invoices)}, resources(answer))
		require.Equal(t, Allow, answer.Entries[0].Decision)
	})

	t.Run("connections in deterministic order", func(t *testing.T) {
		second := s.Connections[0]
		second.Ref.ID, second.Protection = "a-first", nil
		multi := s
		multi.Connections = []Connection{s.Connections[0], second}
		answer, err := ForSubject(ctx, multi, blue, Filter{})
		require.NoError(t, err)
		require.Equal(t, "a-first", answer.Entries[0].Resource.Connection)
		require.Equal(t, connID, answer.Entries[len(answer.Entries)-1].Resource.Connection)
		require.False(t, answer.Complete)
	})

	t.Run("empty snapshot", func(t *testing.T) {
		answer, err := ForSubject(ctx, Snapshot{AsOf: asOf}, blue, Filter{})
		require.NoError(t, err)
		require.Empty(t, answer.Entries)
		require.False(t, answer.Complete, "unreadable membership is never a complete empty answer")
		answer, err = ForSubject(ctx, Snapshot{AsOf: asOf, Members: []dataaccess.Subject{}}, blue, Filter{})
		require.NoError(t, err)
		require.Empty(t, answer.Entries)
		require.True(t, answer.Complete)
	})
}

func resources(a Answer) []Resource {
	out := make([]Resource, len(a.Entries))
	for i, e := range a.Entries {
		out[i] = e.Resource
	}
	return out
}

func subjects(a Answer) []string {
	out := make([]string, len(a.Entries))
	for i, e := range a.Entries {
		if e.Subject == nil {
			out[i] = e.Group
		} else {
			out[i] = e.Subject.ID
		}
	}
	return out
}

func TestForResource(t *testing.T) {
	ctx := context.Background()

	t.Run("posture all is one every-project-member row", func(t *testing.T) {
		s := snapshot(t, dataaccess.ReadAll, grant("g1", blue, asOf.Add(time.Hour), invoices))
		answer, err := ForResource(ctx, s, resource(invoices), nil)
		require.NoError(t, err)
		require.True(t, answer.Complete)
		require.Equal(t, []string{GroupProjectMembers}, subjects(answer))
		row := answer.Entries[0]
		require.Nil(t, row.Subject)
		require.Equal(t, Allow, row.Decision)
		require.Equal(t, []Source{{Kind: SourcePosture, ID: connID, Target: "all"}}, row.Sources)
	})

	t.Run("posture all lists only exceptions", func(t *testing.T) {
		s := snapshot(t, dataaccess.ReadAll, grant("g-mal", mallory, asOf.Add(time.Hour), invoices))
		answer, err := ForResource(ctx, s, resource(invoices), []dataaccess.Permission{dataaccess.DataRead, dataaccess.DataUpdate})
		require.NoError(t, err)
		require.Equal(t, []string{GroupProjectMembers, "mallory", GroupProjectMembers}, subjects(answer))
		require.Equal(t, Deny, answer.Entries[1].Decision)
		require.Equal(t, dataaccess.DataUpdate, answer.Entries[2].Permission)
		require.Equal(t, Deny, answer.Entries[2].Decision)

		s.Connections[0].Protection.MandatoryFineGrained = true
		answer, err = ForResource(ctx, s, resource(invoices), nil)
		require.NoError(t, err)
		require.Equal(t, []string{GroupProjectMembers, "mallory"}, subjects(answer), "every member is unknown alike")
		require.Equal(t, []Decision{Indeterminate, Deny}, decisions(answer))

		s.Members = nil
		s.Connections[0].Protection.MandatoryFineGrained = false
		answer, err = ForResource(ctx, s, resource(invoices), nil)
		require.NoError(t, err)
		require.False(t, answer.Complete)
		require.Equal(t, []string{GroupProjectMembers}, subjects(answer), "no every-member allow during a membership outage")
		require.Equal(t, []Blocker{{Code: CodeMembershipUnavailable}}, answer.Entries[0].Blockers)
	})

	t.Run("posture none enumerates members and grant subjects", func(t *testing.T) {
		s := snapshot(t, dataaccess.ReadNone, grant("g1", blue, asOf.Add(time.Hour), invoices), grant("g2", blue, asOf.Add(50*time.Minute), invoices),
			grant("g-mal", mallory, asOf.Add(time.Hour), invoices))
		answer, err := ForResource(ctx, s, resource(invoices), nil)
		require.NoError(t, err)
		require.Equal(t, []string{"blue", "green", "mallory", "red"}, subjects(answer))
		require.Equal(t, []Decision{Allow, Deny, Deny, Deny}, decisions(answer))
		require.Equal(t, []string{"grant:g2", "grant:g1"}, sourceIDs(answer.Entries[0].Sources), "earliest expiry first")
		require.True(t, answer.Complete)
	})

	t.Run("exact posture", func(t *testing.T) {
		s := snapshot(t, dataaccess.ReadExact, grant("g1", blue, asOf.Add(time.Hour), customers))
		answer, err := ForResource(ctx, s, resource(customers), nil)
		require.NoError(t, err)
		require.Equal(t, []string{"blue", "green", "red"}, subjects(answer))
		require.Equal(t, []Decision{Allow, Allow, Deny}, decisions(answer))
		require.Equal(t, []string{"baseline:prod-billing"}, sourceIDs(answer.Entries[1].Sources))
	})

	t.Run("unknown connection and empty snapshot", func(t *testing.T) {
		for _, s := range []Snapshot{snapshot(t, dataaccess.ReadNone), {AsOf: asOf}} {
			answer, err := ForResource(ctx, s, Resource{Connection: "nope", Namespace: "billing", Name: "invoices"}, nil)
			require.NoError(t, err)
			require.False(t, answer.Complete)
			require.Equal(t, []string{GroupProjectMembers}, subjects(answer))
			require.Equal(t, Indeterminate, answer.Entries[0].Decision)
			require.Equal(t, []Blocker{{Code: CodeUnknownConnection}}, answer.Entries[0].Blockers)
		}
	})

	t.Run("incomplete connection", func(t *testing.T) {
		s := snapshot(t, dataaccess.ReadNone)
		s.Connections[0].Incomplete = []string{"grants"}
		answer, err := ForResource(ctx, s, resource(invoices), nil)
		require.NoError(t, err)
		require.False(t, answer.Complete)
		require.Equal(t, []Decision{Indeterminate, Indeterminate, Indeterminate}, decisions(answer))
	})
}

func decisions(a Answer) []Decision {
	out := make([]Decision, len(a.Entries))
	for i, e := range a.Entries {
		out[i] = e.Decision
	}
	return out
}

func TestSelfDecidedGrantConfersNothing(t *testing.T) {
	self := grant("g-self", blue, asOf.Add(time.Hour), invoices)
	self.Decision = &dataaccess.Decision{By: blue, At: asOf.Add(-time.Minute), Self: true}
	other := grant("g-ok", blue, asOf.Add(30*time.Minute), invoices)
	for _, posture := range []dataaccess.ReadPosture{dataaccess.ReadNone, dataaccess.ReadExact} {
		s := snapshot(t, posture, self)
		entry := explain(t, s, blue, dataaccess.DataRead, invoices)
		require.Equal(t, Deny, entry.Decision, posture)
		require.Empty(t, entry.Sources)
		require.Equal(t, Blocker{Code: CodeSelfDecided, Source: &Ref{Kind: SourceGrant, ID: "g-self"}}, entry.Blockers[len(entry.Blockers)-1])

		s.Connections[0].AllowSelfApproval = true
		entry = explain(t, s, blue, dataaccess.DataRead, invoices)
		require.Equal(t, Allow, entry.Decision, "an explicit self-approval policy lets the grant count")
		require.Equal(t, []string{"grant:g-self"}, sourceIDs(entry.Sources))

		s = snapshot(t, posture, self, other)
		entry = explain(t, s, blue, dataaccess.DataRead, invoices)
		require.Equal(t, Allow, entry.Decision, "another grant still allows")
		require.Equal(t, []string{"grant:g-ok"}, sourceIDs(entry.Sources))
	}
}

func TestExplainWithoutDetailDisclosesNoPolicy(t *testing.T) {
	ctx := context.Background()
	s := snapshot(t, dataaccess.ReadExact, grant("g-sec", blue, asOf.Add(time.Hour), secrets))
	var seen []Entry
	for _, object := range []dataaccess.ObjectRef{secrets, {Namespace: "billing", Name: "ghost"}, invoices} {
		answer, err := Explain(ctx, s, red, dataaccess.DataRead, resource(object), ExplainOptions{})
		require.NoError(t, err)
		require.Equal(t, asOf.Add(AnswerLease), answer.ValidUntil)
		entry := answer.Entries[0]
		require.Equal(t, Deny, entry.Decision)
		require.Equal(t, []Blocker{{Code: CodeReadDenied}}, entry.Blockers)
		require.Empty(t, entry.Candidates)
		entry.Resource = Resource{}
		seen = append(seen, entry)
	}
	require.Equal(t, seen[0], seen[1], "a mandatory-denied table and a missing name look the same")
	require.Equal(t, seen[0], seen[2])
	detailed := explain(t, s, red, dataaccess.DataRead, secrets)
	require.Greater(t, len(detailed.Blockers), 1, "detail shows the policy chain")
}

func TestSelfViewNeverListsUnreachableNames(t *testing.T) {
	ctx := context.Background()
	s := snapshot(t, dataaccess.ReadExact, grant("g1", blue, asOf.Add(time.Hour), invoices))
	for name, tc := range map[string]struct {
		subject     dataaccess.Subject
		permissions []dataaccess.Permission
		mutate      func(*Snapshot)
	}{
		"non-modelled permission":     {blue, []dataaccess.Permission{dataaccess.QueryTugQL}, nil},
		"unknown permission":          {red, []dataaccess.Permission{"data.purge"}, nil},
		"non-member tugql":            {mallory, []dataaccess.Permission{dataaccess.QueryTugQL}, nil},
		"agent":                       {agent, nil, nil},
		"membership unreadable":       {red, nil, func(s *Snapshot) { s.Members = nil }},
		"connection incomplete":       {red, nil, func(s *Snapshot) { s.Connections[0].Incomplete = []string{"grants"} }},
		"non-member every permission": {mallory, []dataaccess.Permission{dataaccess.DataRead, dataaccess.DataUpdate, dataaccess.QueryRaw}, nil},
	} {
		t.Run(name, func(t *testing.T) {
			copy := s
			copy.Members = slices.Clone(s.Members)
			copy.Connections = slices.Clone(s.Connections)
			if tc.mutate != nil {
				tc.mutate(&copy)
			}
			answer, err := ForSubject(ctx, copy, tc.subject, Filter{Permissions: tc.permissions})
			require.NoError(t, err)
			for _, entry := range answer.Entries {
				if entry.Subject != nil {
					require.Equal(t, invoices.Name, entry.Resource.Name, "only the subject's own grant object may be named")
				}
				require.NotEqual(t, secrets.Name, entry.Resource.Name)
				require.NotEqual(t, customers.Name, entry.Resource.Name)
			}
		})
	}
}

func TestResolutionMirrorsBackend(t *testing.T) {
	approved := grant("g-ok", blue, asOf.Add(30*time.Second), invoices)
	other := grant("g-red", red, asOf.Add(30*time.Minute), invoices)
	grants := []StoredGrant{approved, other,
		withState(grant("g-rev", blue, asOf.Add(time.Hour), invoices), dataaccess.Revoked),
		withState(grant("g-req", blue, asOf.Add(time.Hour), invoices), dataaccess.Requested),
		grant("g-edge", blue, asOf, invoices), grant("g-old", blue, asOf.Add(-time.Second), invoices)}
	s := snapshot(t, dataaccess.ReadNone, grants...)
	e := newEvaluator(s)
	r, code := e.resolution(&s.Connections[0], blue)
	require.Empty(t, code)
	require.Equal(t, []dataaccess.Grant{approved.Grant}, r.Grants, "only approved grants unexpired at AsOf, own only")
	require.Equal(t, asOf, r.ServerTime)
	require.Equal(t, asOf.Add(30*time.Second), r.ValidUntil, "posture none clamps to the latest grant expiry")

	s = snapshot(t, dataaccess.ReadAll, approved)
	r, _ = newEvaluator(s).resolution(&s.Connections[0], blue)
	require.Equal(t, asOf.Add(AnswerLease), r.ValidUntil)
	s = snapshot(t, dataaccess.ReadNone, grant("g-long", blue, asOf.Add(time.Hour), invoices))
	r, _ = newEvaluator(s).resolution(&s.Connections[0], blue)
	require.Equal(t, asOf.Add(AnswerLease), r.ValidUntil)
	s = snapshot(t, dataaccess.ReadNone)
	r, _ = newEvaluator(s).resolution(&s.Connections[0], blue)
	require.Equal(t, asOf.Add(AnswerLease), r.ValidUntil)

	moved := grant("g-fp", blue, asOf.Add(-time.Hour), invoices)
	moved.Fingerprint.EngineIdentity = ""
	moved.Fingerprint.Address = "db.example"
	kind := grant("g-kind", blue, asOf.Add(time.Hour), invoices)
	kind.Subject.Kind = dataaccess.Service
	var many []StoredGrant
	for i := range MaxGrantsPerSubject + 1 {
		many = append(many, grant(fmt.Sprintf("g-%03d", i), blue, asOf.Add(-time.Hour), invoices))
	}
	for name, tc := range map[string]struct {
		grants []StoredGrant
		code   string
	}{
		"fingerprint of an inactive grant": {[]StoredGrant{approved, moved}, CodeGrantBindingMismatch},
		"subject kind":                     {[]StoredGrant{kind}, CodeGrantBindingMismatch},
		"over the page cap":                {many, CodeGrantListTruncated},
		"at the page cap":                  {many[:MaxGrantsPerSubject], ""},
	} {
		t.Run(name, func(t *testing.T) {
			s := snapshot(t, dataaccess.ReadAll, tc.grants...)
			_, code := newEvaluator(s).resolution(&s.Connections[0], blue)
			require.Equal(t, tc.code, code)
			entry := explain(t, s, blue, dataaccess.DataRead, invoices)
			if tc.code != "" {
				require.Equal(t, []Blocker{{Code: tc.code}}, entry.Blockers)
				require.Equal(t, Allow, explain(t, s, red, dataaccess.DataRead, invoices).Decision, "another subject is unaffected, as in the backend")
			}
		})
	}
	require.True(t, sameFingerprint(dataaccess.Fingerprint{Engine: "e", Database: "d", Address: "a"}, dataaccess.Fingerprint{Engine: "e", Database: "d", Address: "a"}))
	require.False(t, sameFingerprint(dataaccess.Fingerprint{Engine: "e", Database: "d"}, dataaccess.Fingerprint{Engine: "e", Database: "d"}))
}

func TestSubjectChecksPrecedeThePermission(t *testing.T) {
	s := snapshot(t, dataaccess.ReadAll)
	for _, permission := range []dataaccess.Permission{dataaccess.DataRead, dataaccess.DataUpdate, dataaccess.QueryRaw, dataaccess.QueryTugQL, "data.purge"} {
		require.Equal(t, []Blocker{{Code: CodeNotProjectMember}}, explain(t, s, mallory, permission, invoices).Blockers, permission)
		require.Equal(t, []Blocker{{Code: CodeSubjectKindUnsupported}}, explain(t, s, agent, permission, invoices).Blockers, permission)
		unread := s
		unread.Members = nil
		require.Equal(t, []Blocker{{Code: CodeMembershipUnavailable}}, explain(t, unread, red, permission, invoices).Blockers, permission)
	}
}
