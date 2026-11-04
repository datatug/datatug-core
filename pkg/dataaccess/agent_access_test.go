package dataaccess

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Perimeter part 1 (D23, D27): an agent holds its own approvals, and every
// authorization intersects its perimeter with its user's.

func agentSubject() Subject { return Subject{Realm: "sneat", Kind: Agent, ID: "a1"} }

func agentGrant(now time.Time, objects ...ObjectRef) Grant {
	g := exactReadGrant(now, objects...)
	g.ID = "agent-grant"
	g.Subject, g.RequestedBy = agentSubject(), agentSubject()
	user := fixtureSubject()
	g.AgentUser = &user
	g.Decision = &Decision{By: user, At: now, ForOwnAgent: true}
	return g
}

func requireDenialCode(t *testing.T, err error, code string) {
	t.Helper()
	var denial *Denial
	require.ErrorAs(t, err, &denial)
	require.Equal(t, code, denial.Code)
}

func exactAgentAuthority(t *testing.T, resolve func() Resolution) *Authority {
	t.Helper()
	a, err := NewAuthority(agentSubject(), fixtureConnection(), func(context.Context, ResolveRequest) (Resolution, error) { return resolve(), nil })
	require.NoError(t, err)
	return a
}

// AC:agent-asks-its-user (Lee's dormant path), AC:agent-capped-by-user (table
// part), S-42: the agent's own approval admits nothing beyond its user; once
// the user can read, the same approval takes effect; a revocation of the
// user's access closes the agent at its next authorization.
func TestAgentOwnApprovalCappedByUserPerTable(t *testing.T) {
	doc := exactFixtureDocument()
	now := time.Now()
	order := ObjectRef{"sales", "Order"}
	user := exactRawResolution()
	agentGrants := []Grant{agentGrant(now, order)}
	resolve := func() Resolution {
		r, err := ProjectAgentResolution(context.Background(), user, agentSubject(), agentGrants, exactSnapshot(t, doc))
		require.NoError(t, err)
		return r
	}
	a := exactAgentAuthority(t, resolve)
	ctx := withExactReadProfile(context.Background())
	readOrder := operationRequest(operationQuery("sales", "Order", ""))
	readCustomer := operationRequest(operationQuery("sales", "Customer", ""))

	_, err := a.authorizeRequest(ctx, readOrder)
	requireDenialCode(t, err, "AGENT_USER_LACKS_ACCESS")
	_, err = a.authorizeRequest(ctx, readCustomer)
	require.NoError(t, err, "the user's standing access is inherited and within the user")

	user.Grants = []Grant{exactReadGrant(now, order)}
	_, err = a.authorizeRequest(ctx, readOrder)
	require.NoError(t, err, "the dormant approval takes effect once the user can read")

	user.Grants[0].State = Revoked
	_, err = a.authorizeRequest(ctx, readOrder)
	requireDenialCode(t, err, "AGENT_USER_LACKS_ACCESS")

	user.Grants[0].State = Approved
	agentGrants = nil
	_, err = a.authorizeRequest(ctx, readOrder)
	requireDenialCode(t, err, "READ_DENIED")
}

func TestAgentConnectionGrantCappedByUser(t *testing.T) {
	now := time.Now()
	base := fixtureResolution()
	base.Protection.ReadPosture = ReadNone
	connectionGrant := agentGrant(now)
	connectionGrant.Scope = GrantScope{Connection: "conn", Requires: []string{}}
	userGrants := []Grant{}
	a := exactAgentAuthority(t, func() Resolution {
		r := base
		r.Subject = agentSubject()
		r.Grants = []Grant{connectionGrant}
		return agentOf(r, userGrants...)
	})
	_, err := a.authorizeRequest(context.Background(), nil)
	requireDenialCode(t, err, "AGENT_USER_LACKS_ACCESS")
	userGrants = []Grant{fixtureGrant(now)}
	authorization, err := a.authorizeRequest(context.Background(), nil)
	require.NoError(t, err)
	require.False(t, authorization.deadline.After(authorization.started.Add(time.Minute)))
	_, err = a.authorizeRequest(context.Background(), operationRequest(operationQuery("sales", "Customer", "")))
	require.NoError(t, err)
}

func TestAgentBindingRefusals(t *testing.T) {
	now := time.Now()
	valid := func() Resolution {
		r := fixtureResolution()
		r.Protection.ReadPosture = ReadNone
		r.Subject = agentSubject()
		r.Grants = []Grant{agentGrant(now)}
		r.Grants[0].Scope = GrantScope{Connection: "conn", Requires: []string{}}
		return agentOf(r, fixtureGrant(now))
	}
	other := Subject{Realm: "sneat", Kind: User, ID: "kim"}
	cases := map[string]struct {
		mutate func(*Resolution)
		code   string
	}{
		"missing agent user":          {func(r *Resolution) { r.AgentUser = nil }, "BINDING_MISMATCH"},
		"agent user is an agent":      {func(r *Resolution) { r.AgentUser.Subject.Kind = Agent }, "BINDING_MISMATCH"},
		"agent user invalid":          {func(r *Resolution) { r.AgentUser.Subject.ID = "" }, "BINDING_MISMATCH"},
		"agent user other realm":      {func(r *Resolution) { r.AgentUser.Subject.Realm = "other" }, "BINDING_MISMATCH"},
		"nested agent user":           {func(r *Resolution) { u := *r.AgentUser; r.AgentUser.AgentUser = &u }, "BINDING_MISMATCH"},
		"agent user other connection": {func(r *Resolution) { r.AgentUser.Connection.ID = "other" }, "BINDING_MISMATCH"},
		"agent user other protection": {func(r *Resolution) { r.AgentUser.Protection.Revision = 2 }, "BINDING_MISMATCH"},
		"agent user stale":            {func(r *Resolution) { r.AgentUser.ValidUntil = r.AgentUser.ServerTime }, "BINDING_MISMATCH"},
		"agent user no server time":   {func(r *Resolution) { r.AgentUser.ServerTime = time.Time{} }, "BINDING_MISMATCH"},
		"user grant invalid":          {func(r *Resolution) { r.AgentUser.Grants[0].Revision = 0 }, "UNSUPPORTED_SCOPE"},
		"user grant other subject":    {func(r *Resolution) { r.AgentUser.Grants[0].Subject = other; r.AgentUser.Grants[0].RequestedBy = other }, "BINDING_MISMATCH"},
		"user grant other connection": {func(r *Resolution) { r.AgentUser.Grants[0].Scope.Connection = "other" }, "BINDING_MISMATCH"},
		"human grant on agent": {func(r *Resolution) {
			r.Grants = []Grant{fixtureGrant(now)}
		}, "BINDING_MISMATCH"},
		"agent grant of another user": {func(r *Resolution) {
			r.Grants[0].AgentUser = &other
			r.Grants[0].Decision.By = other
		}, "BINDING_MISMATCH"},
		"agent grant without agent user": {func(r *Resolution) {
			r.Grants[0].AgentUser = nil
			r.Grants[0].Decision.ForOwnAgent = false
		}, "UNSUPPORTED_SCOPE"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			a := exactAgentAuthority(t, func() Resolution {
				r := valid()
				c.mutate(&r)
				return r
			})
			_, err := a.authorizeRequest(context.Background(), nil)
			requireDenialCode(t, err, c.code)
		})
	}
	t.Run("valid", func(t *testing.T) {
		_, err := exactAgentAuthority(t, valid).authorizeRequest(context.Background(), nil)
		require.NoError(t, err)
	})
	t.Run("human resolution carrying an agent user", func(t *testing.T) {
		r := agentOf(fixtureResolution())
		r.Subject = fixtureSubject()
		_, err := testAuthority(t, fixedResolver(r)).authorizeRequest(context.Background(), nil)
		requireDenialCode(t, err, "BINDING_MISMATCH")
	})
	t.Run("agent grant on a human resolution", func(t *testing.T) {
		r := fixtureResolution()
		g := agentGrant(now)
		g.Subject, g.RequestedBy = fixtureSubject(), fixtureSubject()
		g.Decision.ForOwnAgent = false
		r.Grants = []Grant{g}
		_, err := testAuthority(t, fixedResolver(r)).authorizeRequest(context.Background(), nil)
		requireDenialCode(t, err, "UNSUPPORTED_SCOPE")
	})
}

func TestAgentGrantValidation(t *testing.T) {
	now := time.Now()
	user := fixtureSubject()
	other := Subject{Realm: "sneat", Kind: User, ID: "kim"}
	cases := map[string]func(*Grant){
		"agent subject without agent user": func(g *Grant) { g.AgentUser = nil; g.Decision.ForOwnAgent = false },
		"human grant forwarded":            func(g *Grant) { *g = fixtureGrant(now); g.Forwarded = &Forwarding{By: user, At: now} },
		"human grant effective":            func(g *Grant) { *g = fixtureGrant(now); g.EffectiveAt = &now },
		"human grant own-agent decision":   func(g *Grant) { *g = fixtureGrant(now); g.Decision.ForOwnAgent = true },
		"human grant dormant":              func(g *Grant) { *g = fixtureGrant(now); g.Decision.Dormant = true },
		"human grant declined by user":     func(g *Grant) { *g = fixtureGrant(now); g.Decision.ByAgentUser = true },
		"agent user on human subject":      func(g *Grant) { g.Subject, g.RequestedBy = user, user },
		"agent user invalid":               func(g *Grant) { g.AgentUser = &Subject{Realm: "sneat", Kind: User} },
		"agent user is a service":          func(g *Grant) { g.AgentUser = &Subject{Realm: "sneat", Kind: Service, ID: "svc"} },
		"agent user other realm":           func(g *Grant) { g.AgentUser = &Subject{Realm: "other", Kind: User, ID: "sam"} },
		"requested for someone else":       func(g *Grant) { g.RequestedBy = user },
		"forwarded by another":             func(g *Grant) { g.Forwarded = &Forwarding{By: other, At: now} },
		"forwarded without time":           func(g *Grant) { g.Forwarded = &Forwarding{By: user} },
		"forwarded with bad note":          func(g *Grant) { g.Forwarded = &Forwarding{By: user, At: now, Note: " x"} },
		"own decision by another":          func(g *Grant) { g.Decision.By = other },
		"own decision marked self":         func(g *Grant) { g.Decision.Self = true },
		"dormant without own":              func(g *Grant) { g.Decision.ForOwnAgent = false; g.Decision.Dormant = true },
		"own and declined":                 func(g *Grant) { g.Decision.ByAgentUser = true; g.State = Refused },
		"declined but approved":            func(g *Grant) { g.Decision.ForOwnAgent = false; g.Decision.ByAgentUser = true },
		"own approval refused":             func(g *Grant) { g.State = Refused },
		"effective zero":                   func(g *Grant) { g.Decision.Dormant = true; g.EffectiveAt = &time.Time{} },
		"effective without decision":       func(g *Grant) { g.Decision = nil; g.EffectiveAt = &now },
		"effective not dormant":            func(g *Grant) { g.EffectiveAt = &now },
		"decided by the agent itself":      func(g *Grant) { g.Decision = &Decision{By: agentSubject(), At: now} },
		"decided by another agent":         func(g *Grant) { g.Decision = &Decision{By: Subject{Realm: "sneat", Kind: Agent, ID: "a9"}, At: now} },
		"user decision unflagged":          func(g *Grant) { g.Decision = &Decision{By: user, At: now} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			g := agentGrant(now)
			mutate(&g)
			require.Error(t, g.ValidateMVP())
		})
	}
	valid := map[string]func(*Grant){
		"own approval": func(*Grant) {},
		"requested":    func(g *Grant) { g.Decision = nil; g.State = Requested },
		"forwarded with note": func(g *Grant) {
			g.Decision = nil
			g.State = Requested
			g.Forwarded = &Forwarding{By: user, At: now, Note: "please"}
		},
		"decided by an approver": func(g *Grant) { g.Decision = &Decision{By: other, At: now} },
		"declined by its user":   func(g *Grant) { g.State = Refused; g.Decision = &Decision{By: user, At: now, ByAgentUser: true} },
		"dormant then effective": func(g *Grant) { g.Decision.Dormant = true; g.EffectiveAt = &now },
		"human grant":            func(g *Grant) { *g = fixtureGrant(now) },
	}
	for name, mutate := range valid {
		t.Run(name, func(t *testing.T) {
			g := agentGrant(now)
			mutate(&g)
			require.NoError(t, g.ValidateMVP())
		})
	}
}

func TestProjectAgentResolution(t *testing.T) {
	now := time.Now()
	ctx := context.Background()
	user := fixtureResolution()
	user.Grants = []Grant{fixtureGrant(now)}
	own := agentGrant(now)
	own.Scope = GrantScope{Connection: "conn", Requires: []string{}}

	r, err := ProjectAgentResolution(ctx, user, agentSubject(), []Grant{own}, nil)
	require.NoError(t, err)
	require.Equal(t, agentSubject(), r.Subject)
	require.Equal(t, []Grant{own}, r.Grants)
	require.Equal(t, user.Subject, r.AgentUser.Subject)
	require.Equal(t, user.Grants, r.AgentUser.Grants)

	exactUser := exactRawResolution()
	exactUser.Grants = []Grant{exactReadGrant(now, ObjectRef{"sales", "Order"})}
	ownExact := agentGrant(now, ObjectRef{"sales", "Order"}, ObjectRef{"sales", "Secret"})
	ownExact.Forwarded = &Forwarding{By: fixtureSubject(), At: now}
	ownExact.Decision.Dormant = true
	ownExact.EffectiveAt = &now
	pending := agentGrant(now, ObjectRef{"sales", "Order"})
	pending.State, pending.Decision = Requested, nil
	hidden := agentGrant(now, ObjectRef{"sales", "Secret"})
	r, err = ProjectAgentResolution(ctx, exactUser, agentSubject(), []Grant{ownExact, pending, hidden}, exactSnapshot(t, exactFixtureDocument()))
	require.NoError(t, err)
	require.Len(t, r.Grants, 1)
	require.Equal(t, []ObjectRef{{"sales", "Order"}}, r.Grants[0].Scope.Objects, "sealed denials and inactive grants are projected away")
	require.Nil(t, r.Grants[0].Forwarded)
	require.Nil(t, r.Grants[0].EffectiveAt)
	require.Nil(t, r.Grants[0].Decision)
	require.Equal(t, fixtureSubject(), *r.Grants[0].AgentUser)
	require.Len(t, r.AgentUser.Grants, 1)
	require.Equal(t, []ObjectRef{{"sales", "Customer"}}, r.ExactRead.Baseline)

	other := Subject{Realm: "sneat", Kind: User, ID: "kim"}
	refusals := map[string]func() (Resolution, error){
		"nil context":    func() (Resolution, error) { return ProjectAgentResolution(nil, user, agentSubject(), nil, nil) }, //nolint:staticcheck
		"human as agent": func() (Resolution, error) { return ProjectAgentResolution(ctx, user, fixtureSubject(), nil, nil) },
		"invalid agent": func() (Resolution, error) {
			return ProjectAgentResolution(ctx, user, Subject{Realm: "sneat", Kind: Agent}, nil, nil)
		},
		"agent as user": func() (Resolution, error) {
			u := user
			u.Subject = agentSubject()
			return ProjectAgentResolution(ctx, u, agentSubject(), nil, nil)
		},
		"invalid user": func() (Resolution, error) {
			u := user
			u.Subject.ID = ""
			return ProjectAgentResolution(ctx, u, agentSubject(), nil, nil)
		},
		"other realm": func() (Resolution, error) {
			return ProjectAgentResolution(ctx, user, Subject{Realm: "x", Kind: Agent, ID: "a1"}, nil, nil)
		},
		"nested": func() (Resolution, error) {
			u := user
			u.AgentUser = &user
			return ProjectAgentResolution(ctx, u, agentSubject(), nil, nil)
		},
		"invalid grant": func() (Resolution, error) {
			g := own
			g.Revision = 0
			return ProjectAgentResolution(ctx, user, agentSubject(), []Grant{g}, nil)
		},
		"human grant": func() (Resolution, error) {
			return ProjectAgentResolution(ctx, user, agentSubject(), []Grant{fixtureGrant(now)}, nil)
		},
		"another user's agent grant": func() (Resolution, error) {
			g := own
			g.AgentUser, g.Decision = &other, &Decision{By: other, At: now, ForOwnAgent: true}
			return ProjectAgentResolution(ctx, user, agentSubject(), []Grant{g}, nil)
		},
		"other connection": func() (Resolution, error) {
			g := own
			g.Scope.Connection = "other"
			return ProjectAgentResolution(ctx, user, agentSubject(), []Grant{g}, nil)
		},
		"exact posture without snapshot": func() (Resolution, error) {
			return ProjectAgentResolution(ctx, exactRawResolution(), agentSubject(), nil, nil)
		},
		"exact policy without snapshot": func() (Resolution, error) {
			u := user
			u.ExactRead = &ExactReadPolicy{}
			return ProjectAgentResolution(ctx, u, agentSubject(), nil, nil)
		},
		"user not exact": func() (Resolution, error) {
			return ProjectAgentResolution(ctx, user, agentSubject(), nil, exactSnapshot(t, exactFixtureDocument()))
		},
		"agent read none": func() (Resolution, error) {
			u := exactRawResolution()
			u.Protection.AgentRead = AgentNone
			return ProjectAgentResolution(ctx, u, agentSubject(), nil, exactSnapshot(t, exactFixtureDocument()))
		},
		"malformed exact agent object": func() (Resolution, error) {
			g := agentGrant(now, ObjectRef{"", "Order"})
			return ProjectAgentResolution(ctx, exactRawResolution(), agentSubject(), []Grant{g}, exactSnapshot(t, exactFixtureDocument()))
		},
		"invalid snapshot": func() (Resolution, error) {
			return ProjectAgentResolution(ctx, exactRawResolution(), agentSubject(), nil, &ExactReadSnapshot{})
		},
		"user projection refused": func() (Resolution, error) {
			u := exactRawResolution()
			u.ValidUntil = u.ServerTime
			return ProjectAgentResolution(ctx, u, agentSubject(), nil, exactSnapshot(t, exactFixtureDocument()))
		},
	}
	for name, call := range refusals {
		t.Run(name, func(t *testing.T) {
			_, err := call()
			require.Error(t, err)
		})
	}
	t.Run("canceled exact bind", func(t *testing.T) {
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		_, err := ProjectAgentResolution(canceled, exactRawResolution(), agentSubject(), nil, exactSnapshot(t, exactFixtureDocument()))
		require.Error(t, err)
	})
}

func TestAgentDigestFollowsUserAccess(t *testing.T) {
	now := time.Now()
	r := fixtureResolution()
	r.Subject = agentSubject()
	without, err := resolutionDigest(agentOf(r))
	require.NoError(t, err)
	with, err := resolutionDigest(agentOf(r, fixtureGrant(now)))
	require.NoError(t, err)
	require.NotEqual(t, without, with, "a change to the user's grants invalidates the agent's lease")
	human, err := resolutionDigest(fixtureResolution())
	require.NoError(t, err)
	plain := fixtureResolution()
	plain.Subject = agentSubject()
	unchanged, err := resolutionDigest(plain)
	require.NoError(t, err)
	require.NotEqual(t, human, without)
	require.NotEqual(t, unchanged, without)
	broken := agentOf(r)
	broken.AgentUser.Protection.ReadPosture = ReadExact
	_, err = resolutionDigest(broken)
	require.Error(t, err)
}

func TestExactAgentGrantBound(t *testing.T) {
	now := time.Now()
	human := fixtureResolution()
	require.True(t, exactAgentGrantBound(human, fixtureGrant(now)))
	require.False(t, exactAgentGrantBound(human, agentGrant(now)))
	agent := agentOf(fixtureResolution())
	agent.Subject = agentSubject()
	require.True(t, exactAgentGrantBound(agent, agentGrant(now)))
	require.False(t, exactAgentGrantBound(agent, fixtureGrant(now)))
	agent.AgentUser = nil
	require.False(t, exactAgentGrantBound(agent, agentGrant(now)))
}

func TestExactCanonicalAgentUserRefused(t *testing.T) {
	r, err := ProjectAgentResolution(context.Background(), exactRawResolution(), agentSubject(), nil, exactSnapshot(t, exactFixtureDocument()))
	require.NoError(t, err)
	r.AgentUser.ExactRead = nil
	_, err = canonicalExactResolution(r)
	require.Error(t, err)
}

func TestSameProtectionComparesEveryFact(t *testing.T) {
	base := fixtureResolution().Protection
	base.MaxDuration = map[Permission]int{DataRead: 60}
	require.True(t, sameProtection(base, base))
	for _, mutate := range []func(*ConnectionProtection){
		func(p *ConnectionProtection) { p.ID = "x" },
		func(p *ConnectionProtection) { p.Fingerprint.Database = "x" },
		func(p *ConnectionProtection) { p.Name = "x" },
		func(p *ConnectionProtection) { p.Environment = "x" },
		func(p *ConnectionProtection) { p.ReadOnly = false },
		func(p *ConnectionProtection) { p.ReadPosture = ReadNone },
		func(p *ConnectionProtection) { p.AgentRead = AgentNone },
		func(p *ConnectionProtection) { p.MaxDuration = map[Permission]int{DataRead: 30} },
		func(p *ConnectionProtection) { p.MandatoryFineGrained = true },
		func(p *ConnectionProtection) { p.Revision = 9 },
	} {
		changed := base
		changed.MaxDuration = map[Permission]int{DataRead: 60}
		mutate(&changed)
		require.False(t, sameProtection(base, changed))
	}
}

func TestPolicyIntersectionPerTable(t *testing.T) {
	start := time.Now()
	t1, t2, t3 := start.Add(10*time.Second), start.Add(20*time.Second), start.Add(30*time.Second)
	customer, order := ObjectRef{"sales", "Customer"}, ObjectRef{"sales", "Order"}
	agent := &operationReadPolicy{connection: t1, objects: map[ObjectRef]time.Time{customer: t3, order: t2}, ceiling: t3, now: time.Now}
	user := &operationReadPolicy{connection: t2, objects: map[ObjectRef]time.Time{order: t3}, ceiling: t2, exact: true, now: time.Now}
	out := agent.intersect(user)
	require.Equal(t, t1, out.connection)
	require.Equal(t, t2, out.ceiling)
	require.True(t, out.exact)
	require.Equal(t, t2, out.objects[customer], "agent's table grant capped by the user's connection read")
	require.Equal(t, t2, out.objects[order])
	none := &operationReadPolicy{objects: map[ObjectRef]time.Time{}, now: time.Now}
	require.Empty(t, agent.intersect(none).objects)
	require.True(t, agent.intersect(none).connection.IsZero())
}

func TestCoversUserAccess(t *testing.T) {
	now := time.Now()
	order := ObjectRef{"sales", "Order"}
	all := fixtureResolution()
	ok, err := Covers(all, GrantScope{Connection: "conn"})
	require.NoError(t, err)
	require.True(t, ok)
	none := fixtureResolution()
	none.Protection.ReadPosture = ReadNone
	ok, err = Covers(none, GrantScope{Connection: "conn", Objects: []ObjectRef{order}})
	require.NoError(t, err)
	require.False(t, ok)
	ok, err = Covers(none, GrantScope{Connection: "conn"})
	require.NoError(t, err)
	require.False(t, ok)
	none.Grants = []Grant{exactReadGrant(now, order)}
	ok, err = Covers(none, GrantScope{Connection: "conn", Objects: []ObjectRef{order}})
	require.NoError(t, err)
	require.True(t, ok)
	ok, err = Covers(none, GrantScope{Connection: "conn", Objects: []ObjectRef{order, {"sales", "Customer"}}})
	require.NoError(t, err)
	require.False(t, ok)
	exact := exactProjected(t, exactFixtureDocument())
	ok, err = Covers(exact, GrantScope{Connection: "conn", Objects: []ObjectRef{{"sales", "Customer"}}})
	require.NoError(t, err)
	require.True(t, ok)
	for name, r := range map[string]func() (Resolution, GrantScope){
		"invalid subject": func() (Resolution, GrantScope) {
			r := fixtureResolution()
			r.Subject.ID = ""
			return r, GrantScope{Connection: "conn"}
		},
		"stale": func() (Resolution, GrantScope) {
			r := fixtureResolution()
			r.ValidUntil = r.ServerTime
			return r, GrantScope{Connection: "conn"}
		},
		"no server time": func() (Resolution, GrantScope) {
			r := fixtureResolution()
			r.ServerTime = time.Time{}
			return r, GrantScope{Connection: "conn"}
		},
		"other connection": func() (Resolution, GrantScope) { return fixtureResolution(), GrantScope{Connection: "x"} },
		"invalid grant": func() (Resolution, GrantScope) {
			r := fixtureResolution()
			g := fixtureGrant(now)
			g.Revision = 0
			r.Grants = []Grant{g}
			return r, GrantScope{Connection: "conn"}
		},
		"foreign grant": func() (Resolution, GrantScope) {
			r := fixtureResolution()
			r.Grants = []Grant{agentGrant(now)}
			return r, GrantScope{Connection: "conn"}
		},
		"grant on other connection": func() (Resolution, GrantScope) {
			r := fixtureResolution()
			g := fixtureGrant(now)
			g.Scope.Connection = "x"
			r.Grants = []Grant{g}
			return r, GrantScope{Connection: "conn"}
		},
		"unprojected exact": func() (Resolution, GrantScope) { return exactRawResolution(), GrantScope{Connection: "conn"} },
	} {
		t.Run(name, func(t *testing.T) {
			res, scope := r()
			_, err := Covers(res, scope)
			require.Error(t, err)
		})
	}
}

// Review 8888f1c M1: inspection reports what the agent can actually read.
func TestInspectTableReadCapsAgentByUser(t *testing.T) {
	ctx := context.Background()
	now := time.Now()
	order := ObjectRef{"sales", "Order"}
	doc := exactFixtureDocument()
	user := exactRawResolution()
	r, err := ProjectAgentResolution(ctx, user, agentSubject(), []Grant{agentGrant(now, order)}, exactSnapshot(t, doc))
	require.NoError(t, err)
	got, err := InspectTableRead(ctx, r, exactSnapshot(t, doc), order)
	require.NoError(t, err)
	require.False(t, got.Admitted)
	require.Equal(t, "AGENT_USER_LACKS_ACCESS", got.Code)
	require.NotEqual(t, "allow", string(got.Assessment.Outcome))
	user.Grants = []Grant{exactReadGrant(now, order)}
	r, err = ProjectAgentResolution(ctx, user, agentSubject(), []Grant{agentGrant(now, order)}, exactSnapshot(t, doc))
	require.NoError(t, err)
	got, err = InspectTableRead(ctx, r, exactSnapshot(t, doc), order)
	require.NoError(t, err)
	require.True(t, got.Admitted)
	require.Equal(t, "allow", string(got.Assessment.Outcome))
	r, err = ProjectAgentResolution(ctx, user, agentSubject(), nil, exactSnapshot(t, doc))
	require.NoError(t, err)
	r.AgentUser = nil
	got, err = InspectTableRead(ctx, r, exactSnapshot(t, doc), ObjectRef{"sales", "Customer"})
	require.NoError(t, err)
	require.False(t, got.Admitted)
	require.Equal(t, "indeterminate", string(got.Assessment.Outcome))
}

// Review 8888f1c M2, L4, L5, L6.
func TestAgentUserResolutionFreshAndBound(t *testing.T) {
	now := time.Now()
	r := fixtureResolution()
	r.Subject = agentSubject()
	for name, mutate := range map[string]func(*Resolution){
		"older user answer":  func(r *Resolution) { r.AgentUser.ServerTime = r.ServerTime.Add(-time.Minute) },
		"longer user answer": func(r *Resolution) { r.AgentUser.ValidUntil = r.ValidUntil.Add(time.Second) },
	} {
		t.Run(name, func(t *testing.T) {
			a := exactAgentAuthority(t, func() Resolution { c := agentOf(r); c.AgentUser.Grants = []Grant{}; mutate(&c); return c })
			_, err := a.authorizeRequest(context.Background(), nil)
			requireDenialCode(t, err, "BINDING_MISMATCH")
		})
	}
	t.Run("local authority refuses user grants", func(t *testing.T) {
		local := LocalConnectionRef{AuthorityID: "owned-local-registry", ID: "conn", Fingerprint: fixtureConnection().Fingerprint}
		lr := agentOf(r, fixtureGrant(now))
		lr.Connection.Space, lr.Connection.LocalAuthorityID = "", local.AuthorityID
		lr.AgentUser.Connection = lr.Connection
		a, err := NewLocalAuthority(agentSubject(), local, fixedResolver(lr))
		require.NoError(t, err)
		_, err = a.authorizeRequest(context.Background(), nil)
		requireDenialCode(t, err, "LOCAL_GRANTS_UNSUPPORTED")
	})
	t.Run("digest binds the user's identity", func(t *testing.T) {
		dana, err := resolutionDigest(agentOf(r))
		require.NoError(t, err)
		other := agentOf(r)
		other.AgentUser.Subject.ID = "kim"
		kim, err := resolutionDigest(other)
		require.NoError(t, err)
		require.NotEqual(t, dana, kim)
	})
	t.Run("projection does not alias the caller's grants", func(t *testing.T) {
		user := fixtureResolution()
		user.Grants = []Grant{fixtureGrant(now)}
		own := agentGrant(now)
		own.Scope = GrantScope{Connection: "conn", Requires: []string{}}
		own.CaseRef = &CaseRef{Kind: "incident", ID: "1"}
		grants := []Grant{own}
		out, err := ProjectAgentResolution(context.Background(), user, agentSubject(), grants, nil)
		require.NoError(t, err)
		grants[0].Decision.By.ID = "changed"
		grants[0].AgentUser.ID = "changed"
		grants[0].CaseRef.ID = "changed"
		user.Grants[0].Decision.By.ID = "changed"
		require.Equal(t, "sam", out.Grants[0].Decision.By.ID)
		require.Equal(t, "sam", out.Grants[0].AgentUser.ID)
		require.Equal(t, "1", out.Grants[0].CaseRef.ID)
		require.Equal(t, "priya", out.AgentUser.Grants[0].Decision.By.ID)
	})
}
