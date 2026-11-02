package dataaccess

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/dal-go/dalgo/access"
	"github.com/stretchr/testify/require"
	"log/slog"
	"testing"
	"time"
)

func TestExactPublicAdmissionMatrixAndLegacyDigest(t *testing.T) {
	good := exactProjected(t, exactFixtureDocument())
	for name, mutate := range map[string]func(*Resolution){
		"nil": func(r *Resolution) { r.ExactRead = nil }, "legacy": func(r *Resolution) { r.Protection.ReadPosture = ReadAll }, "version": func(r *Resolution) { r.ExactRead.Version++ }, "subject": func(r *Resolution) { r.ExactRead.Subject.ID = "other" }, "invalid subject": func(r *Resolution) { r.Subject.ID = ""; r.ExactRead.Subject = r.Subject }, "connection": func(r *Resolution) { r.ExactRead.Connection.ID = "other" }, "local selection": func(r *Resolution) { r.ExactRead.Connection.LocalAuthorityID = "local" }, "local response": func(r *Resolution) { r.Connection.LocalAuthorityID = "local" }, "baseline nil": func(r *Resolution) { r.ExactRead.Baseline = nil }, "baseline pattern": func(r *Resolution) { r.ExactRead.Baseline = []ObjectRef{{"sales", "*"}} }, "baseline duplicate": func(r *Resolution) { r.ExactRead.Baseline = append(r.ExactRead.Baseline, r.ExactRead.Baseline[0]) }, "baseline bound": func(r *Resolution) { r.ExactRead.Baseline = make([]ObjectRef, 1001) },
	} {
		t.Run(name, func(t *testing.T) {
			copy := exactProjected(t, exactFixtureDocument())
			mutate(&copy)
			_, err := canonicalExactResolution(copy)
			require.Error(t, err)
			_, err = resolutionDigest(copy)
			require.Error(t, err)
		})
	}
	for name, mutate := range map[string]func(*Resolution){"not readonly": func(r *Resolution) { r.Protection.ReadOnly = false }, "finegrain": func(r *Resolution) { r.Protection.MandatoryFineGrained = true }, "connection grant": func(r *Resolution) { r.Grants = []Grant{fixtureGrant(r.ServerTime)} }, "namespace absent": func(r *Resolution) {
		g := exactReadGrant(r.ServerTime, ObjectRef{Name: "Order"})
		g.Reason = "read authority"
		g.Decision = nil
		r.Grants = []Grant{g}
	}} {
		t.Run(name, func(t *testing.T) {
			r := exactProjected(t, exactFixtureDocument())
			mutate(&r)
			_, err := testAuthority(t, fixedResolver(r)).authorizeRequest(withExactReadProfile(context.Background()), operationRequest(operationQuery("sales", "Customer", "")))
			require.Error(t, err)
		})
	}
	a := testAuthority(t, fixedResolver(good))
	req := operationRequest(operationQuery("sales", "Customer", ""))
	_, err := a.authorizeRequest(context.Background(), req)
	require.ErrorContains(t, err, "EXACT_PROFILE_REQUIRED")
	_, err = a.authorizeRequest(withExactReadProfile(context.Background()), nil)
	require.ErrorContains(t, err, "EXACT_PROFILE_REQUIRED")
	_, bound, cancel, err := a.prepareRequest(withExactReadProfile(context.Background()), nil, req)
	require.NoError(t, err)
	cancel()
	_, again, stop, err := a.prepareWithCeiling(context.Background(), bound, nil, nil)
	require.NoError(t, err)
	stop()
	require.True(t, again.exactProfile)
	legacy := fixtureResolution()
	wire, err := json.Marshal(struct {
		Protection       ConnectionProtection
		Grants           []Grant
		LocalAuthorityID string `json:"localAuthorityId,omitempty"`
	}{legacy.Protection, nil, ""})
	require.NoError(t, err)
	digest, err := resolutionDigest(legacy)
	require.NoError(t, err)
	require.Equal(t, hashBytes(wire), digest)
	for _, data := range [][]byte{[]byte(`{"version":1,"VERSION":1}`), []byte{255}, []byte(`{} {}`), []byte(`{"unknown":1}`)} {
		var p ExactReadPolicy
		require.Error(t, p.UnmarshalJSON(data))
	}
	require.Error(t, (*ExactReadPolicy)(nil).UnmarshalJSON([]byte(`{}`)))
	wire, err = json.Marshal(good.ExactRead)
	require.NoError(t, err)
	var decoded ExactReadPolicy
	require.NoError(t, json.Unmarshal(wire, &decoded))
	require.Equal(t, *good.ExactRead, decoded)
	first, err := resolutionDigest(good)
	require.NoError(t, err)
	good.ExactRead.Baseline = append(good.ExactRead.Baseline, ObjectRef{"alpha", "A"}, ObjectRef{"sales", "AAA"})
	canonical, err := canonicalExactResolution(good)
	require.NoError(t, err)
	require.NotEqual(t, first, mustExactDigest(t, canonical))
	good.ExactRead.Baseline[0].Name = "changed"
	require.NotEqual(t, good.ExactRead.Baseline, canonical.ExactRead.Baseline)
	for _, req := range []*access.Request{operationRequest(keyQuery()), operationRequest(operationQuery("sales", "Order", ""))} {
		p := newOperationReadPolicy(good, good.ServerTime, func() time.Time { return good.ServerTime })
		require.Error(t, p.Authorize(context.Background(), *req))
	}
}
func mustExactDigest(t *testing.T, r Resolution) string {
	t.Helper()
	d, e := resolutionDigest(r)
	require.NoError(t, e)
	return d
}

func TestExactInspectionIsImmutableAndNeverCallsClock(t *testing.T) {
	resolution := exactProjected(t, exactFixtureDocument())
	now := resolution.ServerTime
	calls := 0
	p := newOperationReadPolicy(resolution, now, func() time.Time { calls++; return now })
	request := operationRequest(operationQuery("sales", "Customer", ""))
	before := calls
	bounded, stop := context.WithDeadline(context.Background(), p.ceiling)
	defer stop()
	require.False(t, p.Decide(context.Background(), *request).Allowed)
	require.True(t, p.Decide(bounded, *request).Allowed)
	tooLong, end := context.WithDeadline(context.Background(), p.ceiling.Add(time.Second))
	defer end()
	require.False(t, p.Decide(tooLong, *request).Allowed)
	require.False(t, p.Decide(context.WithoutCancel(bounded), *request).Allowed)
	require.Equal(t, before, calls)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	require.False(t, p.Decide(canceled, *request).Allowed)
	resolution.ExactRead.Baseline[0].Name = "forged"
	now = now.Add(time.Minute)
	require.True(t, p.Decide(bounded, *request).Allowed, "inspection is a captured immutable decision, not fresh authorization")
	require.Equal(t, before, calls)
	require.Error(t, p.Authorize(context.Background(), *request), "fresh deadline enforcement still refuses expired authority")
	require.Greater(t, calls, before)
	table := access.TableName{Schema: "sales", Name: "Customer"}
	require.True(t, canonicalReadIdentity(access.CollectionResourceFor(nil, "Customer"), table))
	require.False(t, canonicalReadIdentity(access.CollectionResourceFor(nil, "Order"), table))
}

func TestExactReviewAdmissionAndRedaction(t *testing.T) {
	for _, object := range []ObjectRef{{"sales", "a/b"}, {"sales", "{a}"}, {"s/a", "b"}} {
		require.False(t, validExactObject(object))
	}
	for _, mutate := range []func(*Grant){
		func(g *Grant) { g.Scope.Requires = append(g.Scope.Requires, "unknown") },
		func(g *Grant) { g.Scope.Requires = []string{} },
		func(g *Grant) { g.Permissions = append(g.Permissions, DataWrite) },
	} {
		raw := exactRawResolution()
		g := exactReadGrant(raw.ServerTime, ObjectRef{"sales", "Order"})
		mutate(&g)
		raw.Grants = []Grant{g}
		_, err := ProjectExactRead(context.Background(), raw, exactSnapshot(t, exactFixtureDocument()))
		require.Error(t, err)
		g.Reason = "read authority"
		g.Decision = nil
		raw.ExactRead = &ExactReadPolicy{Version: 1, Subject: raw.Subject, Connection: raw.Connection, Baseline: []ObjectRef{}}
		raw.Grants = []Grant{g}
		_, err = canonicalExactResolution(raw)
		require.Error(t, err)
	}
	raw := exactRawResolution()
	raw.Grants = []Grant{exactReadGrant(raw.ServerTime, ObjectRef{Name: "Order"})}
	_, err := ProjectExactRead(context.Background(), raw, exactSnapshot(t, exactFixtureDocument()))
	require.Error(t, err)
	snapshot := exactSnapshot(t, exactFixtureDocument())
	require.NotContains(t, fmt.Sprintf("%+v %#v %+v %#v", snapshot, snapshot, *snapshot, *snapshot), "Secret")
	require.NotContains(t, snapshot.String(), "Secret")
	require.NotContains(t, snapshot.GoString(), "Secret")
	require.NotContains(t, snapshot.LogValue().String(), "Secret")
	require.Equal(t, slog.KindString, snapshot.LogValue().Kind())
	ctx := withExactReadProfile(context.WithValue(context.Background(), exactBindingKey{}, exactReadBinding{Subject: fixtureSubject(), Connection: fixtureConnection()}))
	filtered := sourceReadContext{ctx}
	require.Nil(t, filtered.Value(exactReadProfileKey{}))
	require.Equal(t, exactReadBinding{Subject: fixtureSubject(), Connection: fixtureConnection()}, filtered.Value(exactBindingKey{}))
	require.NoError(t, checkExactBinding(ctx, fixtureSubject(), fixtureConnection()))
	wrong := fixtureSubject()
	wrong.ID = "other"
	require.Error(t, checkExactBinding(ctx, wrong, fixtureConnection()))
	p := newOperationReadPolicy(exactProjected(t, exactFixtureDocument()), time.Now(), time.Now)
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	require.ErrorIs(t, p.Authorize(canceled, *operationRequest(operationQuery("sales", "Customer", ""))), context.Canceled)
}

func TestExactBoundedPrincipalMismatchDeniesBeforeNativeDispatch(t *testing.T) {
	r := exactProjected(t, exactFixtureDocument())
	p := newOperationReadPolicy(r, r.ServerTime, time.Now)
	bounded, cancel := context.WithDeadline(context.Background(), p.ceiling)
	defer cancel()
	q := operationQuery("sales", "Customer", "")
	request := operationRequest(q)
	principal := access.PrincipalRef{Realm: r.Subject.Realm, Kind: access.PrincipalKind(r.Subject.Kind), ID: r.Subject.ID}
	matching := access.WithPrincipal(bounded, access.Principal{Subject: &principal})
	require.True(t, p.Decide(matching, *request).Allowed, "the bounded exact read must otherwise be allowed")
	for name, change := range map[string]func(*access.PrincipalRef){
		"realm": func(p *access.PrincipalRef) { p.Realm = "other" },
		"kind":  func(p *access.PrincipalRef) { p.Kind = access.PrincipalKind(Agent) },
		"id":    func(p *access.PrincipalRef) { p.ID = "other" },
	} {
		t.Run(name, func(t *testing.T) {
			wrong := principal
			change(&wrong)
			ctx := access.WithPrincipal(bounded, access.Principal{Subject: &wrong})
			decision := p.Decide(ctx, *request)
			require.False(t, decision.Allowed)
			require.Equal(t, "read subject mismatch", decision.Explanation)
			adapter := &recordingAdapter{}
			reader, err := access.SecureReadSession(adapter, p).ExecuteQueryToRecordsReader(ctx, q)
			require.ErrorIs(t, err, access.ErrAccessDenied)
			require.ErrorContains(t, err, "read subject mismatch")
			require.Nil(t, reader)
			require.Empty(t, adapter.calls)
		})
	}
}

func TestExactClosedScopeAdmissionDoesNotDropAnyUnsupportedField(t *testing.T) {
	for name, mutate := range map[string]func(*GrantScope){
		"schemas": func(s *GrantScope) { s.Schemas = []Selector{{Kind: "exact", Value: "sales"}} },
		"exclude": func(s *GrantScope) { s.Exclude = []Selector{{Kind: "exact", Value: "Secret"}} },
		"records": func(s *GrantScope) { s.Records = &RecordScope{Language: "sql"} },
		"fields":  func(s *GrantScope) { s.Fields = &FieldScope{Include: []string{"id"}} },
		"request": func(s *GrantScope) { s.Request = &RequestBinding{Fingerprint: "bound"} },
	} {
		t.Run(name, func(t *testing.T) {
			raw := exactRawResolution()
			g := exactReadGrant(raw.ServerTime, ObjectRef{"sales", "Order"})
			g.Reason = "read authority"
			g.Decision = nil
			mutate(&g.Scope)
			require.False(t, exactGrantFeatures(g), "closed exact admission must not depend on future MVP validator behavior")
			raw.Grants = []Grant{g}
			_, err := ProjectExactRead(context.Background(), raw, exactSnapshot(t, exactFixtureDocument()))
			require.Error(t, err)
			raw.ExactRead = &ExactReadPolicy{Version: 1, Subject: raw.Subject, Connection: raw.Connection, Baseline: []ObjectRef{}}
			_, err = canonicalExactResolution(raw)
			require.Error(t, err)
		})
	}
}
func TestExactAgentNoneEmptyResourcesAndFormattingRefuse(t *testing.T) {
	r := exactProjected(t, exactFixtureDocument())
	agent := Subject{Realm: r.Subject.Realm, Kind: Agent, ID: "worker"}
	r.Subject = agent
	r.ExactRead.Subject = agent
	r.Protection.AgentRead = AgentNone
	_, err := canonicalExactResolution(r)
	require.Error(t, err)
	r.Protection.AgentRead = AgentInherit
	_, err = canonicalExactResolution(r)
	require.NoError(t, err)
	p := newOperationReadPolicy(r, r.ServerTime, time.Now)
	ctx, cancel := context.WithDeadline(context.Background(), p.ceiling)
	defer cancel()
	_, _, err = p.planAt(access.Request{Operation: access.Query}, r.ServerTime)
	require.ErrorContains(t, err, "operation resources required")
	require.False(t, p.Decide(ctx, access.Request{Operation: access.Query}).Allowed)
	snapshot := exactSnapshot(t, exactFixtureDocument())
	for _, format := range []string{"%v", "%+v", "%#v", "%d", "%s", "%q", "%x", "%#x", "%f", "%t", "%p"} {
		for _, value := range []any{snapshot, *snapshot} {
			text := fmt.Sprintf(format, value)
			require.NotContains(t, text, "Secret")
			require.NotContains(t, text, "sales")
			require.NotContains(t, text, "generation")
		}
	}
}

func TestExactNotBeforeRemainsClosedIndependentlyOfGenericAdmission(t *testing.T) {
	raw := exactRawResolution()
	g := exactReadGrant(raw.ServerTime, ObjectRef{"sales", "Order"})
	later := raw.ServerTime.Add(time.Hour)
	g.NotBefore = &later
	require.False(t, exactGrantFeatures(g))
	raw.Grants = []Grant{g}
	_, err := ProjectExactRead(context.Background(), raw, exactSnapshot(t, exactFixtureDocument()))
	require.Error(t, err)
	g.Reason = "read authority"
	g.Decision = nil
	raw.Grants = []Grant{g}
	raw.ExactRead = &ExactReadPolicy{Version: 1, Subject: raw.Subject, Connection: raw.Connection, Baseline: []ObjectRef{}}
	_, err = canonicalExactResolution(raw)
	require.Error(t, err)
}
func TestExactInvalidInternalRulesReturnUnsupportedDenial(t *testing.T) {
	r := exactProjected(t, exactFixtureDocument())
	p := newOperationReadPolicy(r, r.ServerTime, time.Now)
	ctx := access.WithPrincipal(context.Background(), access.Principal{Subject: &access.PrincipalRef{Realm: r.Subject.Realm, Kind: access.PrincipalKind(r.Subject.Kind), ID: r.Subject.ID}})
	request := *operationRequest(operationQuery("sales", "Customer", ""))
	// planAt emits unique names and valid plain scopes today. A future internal
	// caller passing an invalid compilation must get an explicit safe denial.
	invalid := []access.Rule{access.Root(access.Allow(access.Read, "duplicate")), access.Root(access.Allow(access.Read, "duplicate"))}
	_, err := access.NewPolicyForRealm(r.Subject.Realm, p.Name(), invalid...)
	require.Error(t, err)
	decision := p.decideRules(ctx, request, invalid)
	require.False(t, decision.Allowed)
	require.Equal(t, access.CodeEnforcementUnsupported, decision.Code)
	require.Equal(t, "read composition unavailable", decision.Explanation)
}
