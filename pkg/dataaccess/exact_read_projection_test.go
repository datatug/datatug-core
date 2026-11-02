package dataaccess

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/dal-go/dalgo/access"

	"github.com/stretchr/testify/require"
)

func exactFixtureDocument() exactReadDocument {
	return exactReadDocument{Version: 1, Connection: fixtureConnection(), Generation: 1, NativeNamespace: "sales",
		Objects:     []ObjectRef{{"sales", "Customer"}, {"sales", "Order"}, {"sales", "Secret"}},
		Standing:    []exactReadStanding{{Subject: fixtureSubject(), Objects: []ObjectRef{{"sales", "Customer"}}}},
		Requestable: []ObjectRef{{"sales", "Order"}}, Denials: []exactReadDeny{{Object: &ObjectRef{"sales", "Secret"}}}}
}
func exactSnapshot(t *testing.T, doc exactReadDocument) *ExactReadSnapshot {
	t.Helper()
	data, err := json.Marshal(doc)
	require.NoError(t, err)
	s, err := DecodeExactReadSnapshot(data)
	require.NoError(t, err)
	return s
}
func exactRawResolution() Resolution {
	r := fixtureResolution()
	r.Protection.ReadPosture = ReadExact
	return r
}
func exactProjected(t *testing.T, doc exactReadDocument, grants ...Grant) Resolution {
	t.Helper()
	r := exactRawResolution()
	r.Grants = grants
	projected, err := ProjectExactRead(context.Background(), r, exactSnapshot(t, doc))
	require.NoError(t, err)
	return projected
}
func exactReadGrant(now time.Time, objects ...ObjectRef) Grant {
	g := fixtureGrant(now)
	g.Scope.Objects = objects
	if len(objects) > 0 {
		g.Scope.Requires = []string{ObjectsExact}
	}
	return g
}

func TestExactSnapshotClosedCodecAndCanonicalInventory(t *testing.T) {
	for name, mutate := range map[string]func(*exactReadDocument){
		"version": func(d *exactReadDocument) { d.Version++ }, "generation": func(d *exactReadDocument) { d.Generation = 0 },
		"local": func(d *exactReadDocument) { d.Connection.LocalAuthorityID = "local" }, "space": func(d *exactReadDocument) { d.Connection.Space = "" }, "connection": func(d *exactReadDocument) { d.Connection.ID = "" }, "engine": func(d *exactReadDocument) { d.Connection.Fingerprint.Engine = "" }, "database": func(d *exactReadDocument) { d.Connection.Fingerprint.Database = "" }, "nativeidentity": func(d *exactReadDocument) {
			d.Connection.Fingerprint.EngineIdentity = ""
			d.Connection.Fingerprint.Address = ""
		},
		"inventory absent": func(d *exactReadDocument) { d.Objects = nil }, "inventory duplicate": func(d *exactReadDocument) { d.Objects = append(d.Objects, d.Objects[0]) }, "fold collision": func(d *exactReadDocument) { d.Objects = append(d.Objects, ObjectRef{"SALES", "customer"}) }, "namespace invalid": func(d *exactReadDocument) { d.NativeNamespace = " bad" },
		"standing absent": func(d *exactReadDocument) { d.Standing = nil }, "denials absent": func(d *exactReadDocument) { d.Denials = nil }, "standing bound": func(d *exactReadDocument) { d.Standing = make([]exactReadStanding, 1001) }, "denials bound": func(d *exactReadDocument) { d.Denials = make([]exactReadDeny, 1001) },
		"standing subject": func(d *exactReadDocument) { d.Standing[0].Subject.ID = "" }, "standing Agent": func(d *exactReadDocument) { d.Standing[0].Subject.Kind = Agent }, "standing duplicate": func(d *exactReadDocument) { d.Standing = append(d.Standing, d.Standing[0]) }, "standing absentlist": func(d *exactReadDocument) { d.Standing[0].Objects = nil }, "standing unknown": func(d *exactReadDocument) { d.Standing[0].Objects = []ObjectRef{{"sales", "unknown"}} },
		"requestable nil": func(d *exactReadDocument) { d.Requestable = nil }, "requestable alias": func(d *exactReadDocument) { d.Requestable = []ObjectRef{{"sales", "order"}} },
		"deny subject": func(d *exactReadDocument) { d.Denials[0].Subject = &Subject{} }, "deny no selector": func(d *exactReadDocument) { d.Denials[0].Object = nil }, "deny both selectors": func(d *exactReadDocument) { d.Denials[0].Path = "/Secret" }, "deny unknown": func(d *exactReadDocument) { d.Denials[0].Object = &ObjectRef{"sales", "SECRET"} },
		"path namespace": func(d *exactReadDocument) { d.NativeNamespace = ""; d.Denials = []exactReadDeny{{Path: "/Secret"}} }, "path nested": func(d *exactReadDocument) { d.Denials = []exactReadDeny{{Path: "/Secret/id"}} }, "path pattern": func(d *exactReadDocument) { d.Denials = []exactReadDeny{{Path: "/*"}} }, "path missing slash": func(d *exactReadDocument) { d.Denials = []exactReadDeny{{Path: "Secret"}} }, "path unknown": func(d *exactReadDocument) { d.Denials = []exactReadDeny{{Path: "/unknown"}} }, "path multins": func(d *exactReadDocument) {
			d.Objects = append(d.Objects, ObjectRef{"other", "Secret"})
			d.Denials = []exactReadDeny{{Path: "/Secret"}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			d := exactFixtureDocument()
			mutate(&d)
			data, _ := json.Marshal(d)
			s, err := DecodeExactReadSnapshot(data)
			require.Error(t, err)
			require.Nil(t, s)
			require.NotContains(t, err.Error(), "Secret")
		})
	}
	data, _ := json.Marshal(exactFixtureDocument())
	for _, bad := range [][]byte{[]byte(strings.Repeat("x", 1048577)), []byte{255}, []byte(`{`), append(bytes.Clone(data), []byte(` {}`)...), bytes.Replace(data, []byte(`"version":1`), []byte(`"version":1,"VERSION":1`), 1), bytes.Replace(data, []byte(`"version":1`), []byte(`"customPolicy":{},"version":1`), 1)} {
		_, err := DecodeExactReadSnapshot(bad)
		require.Error(t, err)
	}
	s := exactSnapshot(t, exactFixtureDocument())
	var privateTransport any = s
	encoded, err := json.Marshal(privateTransport)
	require.NoError(t, err)
	require.JSONEq(t, `{}`, string(encoded))
	requestable := s.RequestableObjects()
	requestable[0].Name = "changed"
	require.Equal(t, "Order", s.RequestableObjects()[0].Name)
	require.Nil(t, (*ExactReadSnapshot)(nil).RequestableObjects())
	require.Nil(t, (&ExactReadSnapshot{}).RequestableObjects())
	empty := exactFixtureDocument()
	empty.Requestable = []ObjectRef{}
	empty.Standing = []exactReadStanding{}
	empty.Denials = []exactReadDeny{}
	require.NotNil(t, exactSnapshot(t, empty).RequestableObjects())
}

func TestExactProjectionFiltersBaselineAndEveryGrantWithoutRewritingAudit(t *testing.T) {
	raw := exactRawResolution()
	g := exactReadGrant(raw.ServerTime, ObjectRef{"sales", "Customer"}, ObjectRef{"sales", "Order"}, ObjectRef{"sales", "Secret"})
	g.Reason = "private Secret reason"
	g.PriorGrant = "prior-private"
	g.CaseRef = &CaseRef{Kind: "issue", ID: "Secret-case"}
	g.Decision.Note = "Secret note"
	original, _ := json.Marshal(g)
	raw.Grants = []Grant{g}
	s := exactSnapshot(t, exactFixtureDocument())
	projected, err := ProjectExactRead(context.Background(), raw, s)
	require.NoError(t, err)
	require.Len(t, projected.ExactRead.Baseline, 1)
	require.Len(t, projected.Grants, 1)
	require.Len(t, projected.Grants[0].Scope.Objects, 2)
	require.Equal(t, g.ExpiresAt, projected.Grants[0].ExpiresAt)
	require.Equal(t, "read authority", projected.Grants[0].Reason)
	require.Nil(t, projected.Grants[0].CaseRef)
	require.Nil(t, projected.Grants[0].Decision)
	require.Empty(t, projected.Grants[0].PriorGrant)
	require.NoError(t, projected.Grants[0].ValidateMVP())
	after, _ := json.Marshal(g)
	require.Equal(t, original, after)
	wire, _ := json.Marshal(projected)
	require.NotContains(t, string(wire), "Secret")
	require.NotContains(t, string(wire), "generation")
	normalized := exactReadGrant(raw.ServerTime, ObjectRef{"sales", "Order"}, ObjectRef{"sales", "unknown"})
	raw.Grants = []Grant{normalized}
	projected, err = ProjectExactRead(context.Background(), raw, s)
	require.NoError(t, err)
	require.Equal(t, []ObjectRef{{"sales", "Order"}}, projected.Grants[0].Scope.Objects)
	for _, state := range []GrantState{Revoked, Refused, Requested} {
		copy := g
		copy.State = state
		raw.Grants = []Grant{copy}
		p, e := ProjectExactRead(context.Background(), raw, s)
		require.NoError(t, e)
		require.Empty(t, p.Grants)
	}
	expired := g
	expired.ExpiresAt = raw.ServerTime
	whole := fixtureGrant(raw.ServerTime)
	denied := exactReadGrant(raw.ServerTime, ObjectRef{"sales", "Secret"})
	raw.Grants = []Grant{expired, whole, denied}
	p, e := ProjectExactRead(context.Background(), raw, s)
	require.NoError(t, e)
	require.Empty(t, p.Grants)
	for name, mutate := range map[string]func(*Grant){"reason": func(g *Grant) { g.Reason = "" }, "permissions": func(g *Grant) { g.Permissions = append(g.Permissions, DataWrite) }, "subject": func(g *Grant) { g.Subject.ID = "other" }, "connection": func(g *Grant) { g.Scope.Connection = "other" }, "pattern": func(g *Grant) { g.Scope.Objects[0].Name = "*" }} {
		t.Run(name, func(t *testing.T) {
			bad := exactReadGrant(raw.ServerTime, ObjectRef{"sales", "Order"})
			mutate(&bad)
			copy := raw
			copy.Grants = []Grant{bad}
			_, err := ProjectExactRead(context.Background(), copy, s)
			require.Error(t, err)
		})
	}
	noNamespace := exactFixtureDocument()
	noNamespace.NativeNamespace = ""
	raw.Grants = []Grant{exactReadGrant(raw.ServerTime, ObjectRef{Name: "Order"})}
	_, err = ProjectExactRead(context.Background(), raw, exactSnapshot(t, noNamespace))
	require.Error(t, err)
}

func TestExactProjectionCancellationBindingAndUnknownAmbientRefuse(t *testing.T) {
	raw := exactRawResolution()
	s := exactSnapshot(t, exactFixtureDocument())
	for _, ctx := range []context.Context{nil} {
		_, err := ProjectExactRead(ctx, raw, s)
		require.Error(t, err)
	}
	for _, bad := range []*ExactReadSnapshot{nil, {}} {
		_, err := ProjectExactRead(context.Background(), raw, bad)
		require.Error(t, err)
		_, err = bad.BindContext(context.Background(), raw.Subject)
		require.Error(t, err)
	}
	var missingContext context.Context
	_, err := s.BindContext(missingContext, raw.Subject)
	require.Error(t, err)
	_, err = s.BindContext(context.Background(), Subject{})
	require.Error(t, err)
	conflicted, err := access.WithNativeTableNamespace(context.Background(), "other")
	require.NoError(t, err)
	_, err = s.BindContext(conflicted, raw.Subject)
	require.Error(t, err)
	for name, mutate := range map[string]func(*Resolution){"source": func(r *Resolution) { r.Connection.ID = "other" },
		"protection id":     func(r *Resolution) { r.Protection.ID = "other" },
		"protection source": func(r *Resolution) { r.Protection.Fingerprint.EngineIdentity = "other" },
		"server time":       func(r *Resolution) { r.ServerTime = time.Time{} },
		"ceiling":           func(r *Resolution) { r.ValidUntil = r.ServerTime },
		"agent mode":        func(r *Resolution) { r.Protection.AgentRead = "unknown" }, "subject": func(r *Resolution) { r.Subject.ID = "" }, "Agent": func(r *Resolution) { r.Subject.Kind = Agent }, "posture": func(r *Resolution) { r.Protection.ReadPosture = ReadAll }, "write": func(r *Resolution) { r.Protection.ReadOnly = false }, "fine": func(r *Resolution) { r.Protection.MandatoryFineGrained = true }} {
		t.Run(name, func(t *testing.T) {
			copy := raw
			mutate(&copy)
			_, err := ProjectExactRead(context.Background(), copy, s)
			require.Error(t, err)
		})
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = ProjectExactRead(canceled, raw, s)
	require.ErrorIs(t, err, context.Canceled)

	bound, err := s.BindContext(context.Background(), raw.Subject)
	require.NoError(t, err)
	_, err = ProjectExactRead(bound, raw, s)
	require.NoError(t, err)
	multi := exactFixtureDocument()
	multi.NativeNamespace = ""
	multi.Objects = append(multi.Objects, ObjectRef{"other", "Known"})
	multi.Denials = []exactReadDeny{}
	_, err = ProjectExactRead(context.Background(), raw, exactSnapshot(t, multi))
	require.NoError(t, err)
}

func TestExactPrivateChangeRebindsPublicScopeButUnrelatedChangeDoesNot(t *testing.T) {
	raw := exactRawResolution()
	raw.Grants = []Grant{exactReadGrant(raw.ServerTime, ObjectRef{"sales", "Order"})}
	doc := exactFixtureDocument()
	first, err := ProjectExactRead(context.Background(), raw, exactSnapshot(t, doc))
	require.NoError(t, err)
	digest, err := resolutionDigest(first)
	require.NoError(t, err)
	doc.Generation++
	doc.Denials = []exactReadDeny{}
	same, err := ProjectExactRead(context.Background(), raw, exactSnapshot(t, doc))
	require.NoError(t, err)
	sameDigest, err := resolutionDigest(same)
	require.NoError(t, err)
	require.Equal(t, digest, sameDigest)
	doc.Denials = []exactReadDeny{{Path: "/Order"}}
	after, err := ProjectExactRead(context.Background(), raw, exactSnapshot(t, doc))
	require.NoError(t, err)
	require.Empty(t, after.Grants)
	newDigest, err := resolutionDigest(after)
	require.NoError(t, err)
	require.NotEqual(t, digest, newDigest)
	// A legacy collection denial constrains the qualified native probe and actual execution.
	ctx, err := exactSnapshot(t, doc).BindContext(context.Background(), raw.Subject)
	require.NoError(t, err)
	q := operationQuery("sales", "Order", "")
	assessment, err := access.AssessContextPlan(ctx, *operationRequest(q))
	require.NoError(t, err)
	require.Equal(t, access.AssessmentDeny, assessment.Outcome)
	doc.Denials = []exactReadDeny{{Path: "/"}}
	none, err := ProjectExactRead(context.Background(), raw, exactSnapshot(t, doc))
	require.NoError(t, err)
	require.Empty(t, none.ExactRead.Baseline)
	require.Empty(t, none.Grants)
}

func TestExactAgentInheritIntersectsUserAndAgentPrincipals(t *testing.T) {
	user := exactRawResolution()
	user.Grants = []Grant{exactReadGrant(user.ServerTime, ObjectRef{"sales", "Order"})}
	agent := Subject{Realm: user.Subject.Realm, Kind: Agent, ID: "worker"}
	for _, deny := range []*Subject{nil, &user.Subject, &agent} {
		doc := exactFixtureDocument()
		if deny != nil {
			doc.Denials = []exactReadDeny{{Subject: deny, Object: &ObjectRef{"sales", "Customer"}}}
		}
		p, err := ProjectAgentExactRead(context.Background(), user, agent, exactSnapshot(t, doc))
		require.NoError(t, err)
		require.Equal(t, agent, p.Subject)
		require.Empty(t, p.Grants)
		require.Equal(t, deny == nil, len(p.ExactRead.Baseline) == 1)
	}
	for _, bad := range []Subject{{}, {Realm: "other", Kind: Agent, ID: "worker"}, {Realm: user.Subject.Realm, Kind: User, ID: "worker"}} {
		_, err := ProjectAgentExactRead(context.Background(), user, bad, exactSnapshot(t, exactFixtureDocument()))
		require.Error(t, err)
	}
	user.Protection.AgentRead = AgentNone
	_, err := ProjectAgentExactRead(context.Background(), user, agent, exactSnapshot(t, exactFixtureDocument()))
	require.Error(t, err)
}

type exactCancelOnCheck struct {
	context.Context
	cancel        context.CancelFunc
	checks, limit int
}

func (c *exactCancelOnCheck) Err() error {
	c.checks++
	if c.checks >= c.limit {
		c.cancel()
	}
	return c.Context.Err()
}
func TestExactProjectionCancellationAtEveryStageReleasesNoPartialAuthority(t *testing.T) {
	s := exactSnapshot(t, exactFixtureDocument())
	raw := exactRawResolution()
	raw.Grants = []Grant{exactReadGrant(raw.ServerTime, ObjectRef{"sales", "Order"})}
	agent := Subject{Realm: raw.Subject.Realm, Kind: Agent, ID: "worker"}
	for limit := 1; limit < 45; limit++ {
		for _, isAgent := range []bool{false, true} {
			ctx, cancel := context.WithCancel(context.Background())
			checking := &exactCancelOnCheck{Context: ctx, cancel: cancel, limit: limit}
			var r Resolution
			var err error
			if isAgent {
				r, err = ProjectAgentExactRead(checking, raw, agent, s)
			} else {
				r, err = ProjectExactRead(checking, raw, s)
			}
			if err != nil {
				require.ErrorIs(t, err, context.Canceled)
				require.Zero(t, r.Subject)
			}
			cancel()
		}
	}
	// Invalid internal state must fail closed even though ordinary callers can
	// construct snapshots only through the strict decoder.
	bad := *s
	document := *s.document
	bad.document = &document
	bad.document.Objects = []ObjectRef{{Namespace: "sales"}}
	_, err := bad.BindContext(context.Background(), raw.Subject)
	require.Error(t, err)
	_, err = ProjectExactRead(context.Background(), raw, &bad)
	require.Error(t, err)
	_, err = ProjectAgentExactRead(context.Background(), raw, agent, &bad)
	require.Error(t, err)
	_, err = s.allowed(context.Background(), ObjectRef{"sales", "Customer"})
	require.Error(t, err)
	_, err = s.allowed(context.Background(), ObjectRef{Name: "*"})
	require.NoError(t, err)
	doc := exactFixtureDocument()
	doc.Denials[0].Object = &ObjectRef{Name: "*"}
	data, _ := json.Marshal(doc)
	_, err = DecodeExactReadSnapshot(data)
	require.Error(t, err)
	_, err = ProjectAgentExactRead(context.Background(), raw, agent, nil)
	require.Error(t, err)
}

func TestExactAssessmentNeverTreatsIncompleteOrResidualAsReadAuthority(t *testing.T) {
	for _, assessment := range []access.Assessment{{}, {Complete: true, Restrictions: []access.AssessmentRestriction{{}}}} {
		allowed, err := exactAssessmentAllowed(assessment, nil)
		require.Error(t, err)
		require.False(t, allowed)
	}
	allowed, err := exactAssessmentAllowed(access.Assessment{Complete: true, Outcome: access.AssessmentAllow}, context.Canceled)
	require.Error(t, err)
	require.False(t, allowed)
}
