package dataaccess

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// Perimeter D29 (REQ:approval-lifetimes, REQ:agent-sessions): a "This session"
// approval counts only in its own live agent session and never beyond it.

func sessionGrant(now time.Time, session string) Grant {
	g := agentGrant(now)
	g.Scope = GrantScope{Connection: "conn", Requires: []string{}}
	g.Lifetime, g.AgentSessionID = LifetimeSession, session
	return g
}

func sessionResolution(now time.Time, grant Grant, session *AgentSession) Resolution {
	r := fixtureResolution()
	r.Protection.ReadPosture = ReadNone
	r.Subject = agentSubject()
	r.Grants = []Grant{grant}
	r = agentOf(r, fixtureGrant(now))
	r.AgentSession = session
	return r
}

// AC:session-bound-approval-ends-with-session, AC:other-agent-or-invented-session-refused (core half).
func TestSessionGrantCountsOnlyInItsLiveSession(t *testing.T) {
	now := time.Now()
	grant := sessionGrant(now, "s1")
	session := &AgentSession{ID: "s1", EndsAt: now.Add(30 * time.Minute)}
	var current Resolution
	a := exactAgentAuthority(t, func() Resolution { return current })

	current = sessionResolution(now, grant, session)
	authorization, err := a.authorizeRequest(context.Background(), nil)
	require.NoError(t, err)
	digest := authorization.digest

	// A session ending soon caps the approval at the session's end.
	current = sessionResolution(now, grant, &AgentSession{ID: "s1", EndsAt: now.Add(20 * time.Second)})
	authorization, err = a.authorizeRequest(context.Background(), nil)
	require.NoError(t, err)
	require.True(t, authorization.deadline.Before(authorization.started.Add(21*time.Second)))
	require.Equal(t, digest, authorization.digest, "the idle end moving does not change the digest")

	// An ended session: the approval no longer counts.
	current = sessionResolution(now, grant, &AgentSession{ID: "s1", EndsAt: now.Add(-time.Second)})
	_, err = a.authorizeRequest(context.Background(), nil)
	requireDenialCode(t, err, "READ_DENIED")

	// Another session, or no session, never carries the approval.
	current = sessionResolution(now, grant, &AgentSession{ID: "s2", EndsAt: now.Add(time.Hour)})
	_, err = a.authorizeRequest(context.Background(), nil)
	requireDenialCode(t, err, "READ_DENIED")
	current = sessionResolution(now, grant, nil)
	_, err = a.authorizeRequest(context.Background(), nil)
	requireDenialCode(t, err, "READ_DENIED")
	// A stale session grant never costs the agent its other access.
	stale := sessionResolution(now, grant, &AgentSession{ID: "s2", EndsAt: now.Add(time.Hour)})
	standing := grant
	standing.ID, standing.Lifetime, standing.AgentSessionID = "standing", "", ""
	stale.Grants = append(stale.Grants, standing)
	current = stale
	rolled, err := a.authorizeRequest(context.Background(), nil)
	require.NoError(t, err)
	stale.AgentSession = &AgentSession{ID: "s3", EndsAt: now.Add(time.Hour)}
	current = stale
	rolledAgain, err := a.authorizeRequest(context.Background(), nil)
	require.NoError(t, err)
	require.Equal(t, rolled.digest, rolledAgain.digest, "a session no grant uses is not bound")

	// A deadline approval keeps working without a session (D29 SQ-5).
	deadline := grant
	deadline.Lifetime, deadline.AgentSessionID = "", ""
	current = sessionResolution(now, deadline, nil)
	_, err = a.authorizeRequest(context.Background(), nil)
	require.NoError(t, err)
	current = sessionResolution(now, grant, &AgentSession{ID: "s9", EndsAt: now.Add(time.Hour)})
	current.Grants[0].AgentSessionID = "s9"
	other, err := a.authorizeRequest(context.Background(), nil)
	require.NoError(t, err)
	require.NotEqual(t, digest, other.digest, "a used session is bound")
}

// Exact posture: a session grant on an object is projected, bound and capped.
func TestExactSessionGrant(t *testing.T) {
	doc := exactFixtureDocument()
	now := time.Now()
	order := ObjectRef{"sales", "Order"}
	user := exactRawResolution()
	user.Grants = []Grant{exactReadGrant(now, order)}
	g := agentGrant(now, order)
	g.Lifetime, g.AgentSessionID = LifetimeSession, "s1"
	var session *AgentSession
	a := exactAgentAuthority(t, func() Resolution {
		r, err := ProjectAgentResolution(context.Background(), user, agentSubject(), []Grant{g}, exactSnapshot(t, doc))
		require.NoError(t, err)
		r.AgentSession = session
		return r
	})
	ctx := withExactReadProfile(context.Background())
	readOrder := operationRequest(operationQuery("sales", "Order", ""))
	_, err := a.authorizeRequest(ctx, readOrder)
	requireDenialCode(t, err, "READ_DENIED")
	session = &AgentSession{ID: "s1", EndsAt: now.Add(time.Hour)}
	_, err = a.authorizeRequest(ctx, readOrder)
	require.NoError(t, err)
}

func TestAgentSessionBindingRefusals(t *testing.T) {
	now := time.Now()
	session := &AgentSession{ID: "s1", EndsAt: now.Add(time.Hour)}
	for name, mutate := range map[string]func(*Resolution){
		"session on the user's resolution": func(r *Resolution) { r.AgentUser.AgentSession = session },
		"malformed session ID":             func(r *Resolution) { r.AgentSession = &AgentSession{ID: " s1", EndsAt: now.Add(time.Hour)} },
		"session without an end":           func(r *Resolution) { r.AgentSession = &AgentSession{ID: "s1"} },
	} {
		r := sessionResolution(now, sessionGrant(now, "s1"), session)
		mutate(&r)
		a := exactAgentAuthority(t, func() Resolution { return r })
		_, err := a.authorizeRequest(context.Background(), nil)
		requireDenialCode(t, err, "BINDING_MISMATCH")
		_ = name
	}
	// A person's resolution never carries an agent session.
	person := fixtureResolution()
	person.Grants = []Grant{fixtureGrant(now)}
	person.AgentSession = session
	a, err := NewAuthority(person.Subject, fixtureConnection(), func(context.Context, ResolveRequest) (Resolution, error) { return person, nil })
	require.NoError(t, err)
	_, err = a.authorizeRequest(context.Background(), nil)
	requireDenialCode(t, err, "BINDING_MISMATCH")
}

func TestLifetimeValidation(t *testing.T) {
	now := time.Now()
	require.NoError(t, sessionGrant(now, "s1").ValidateMVP())
	request := AccessRequest{Permissions: []Permission{DataRead}, Scope: GrantScope{Connection: "conn", Requires: []string{}}, Reason: "why", ExpiresAt: now}
	for _, lifetime := range []Lifetime{"", LifetimeDeadline, LifetimeSession} {
		request.Lifetime = lifetime
		require.NoError(t, request.ValidateMVP(), lifetime)
	}
	for _, lifetime := range []Lifetime{LifetimeOnce, "always", "forever"} {
		request.Lifetime = lifetime
		require.Error(t, request.ValidateMVP(), lifetime)
	}
	for name, mutate := range map[string]func(*Grant){
		"session without ID":       func(g *Grant) { g.AgentSessionID = "" },
		"ID without session":       func(g *Grant) { g.Lifetime = LifetimeDeadline },
		"malformed ID":             func(g *Grant) { g.AgentSessionID = "s 1 " },
		"once refused in the MVP":  func(g *Grant) { g.Lifetime = LifetimeOnce },
		"session on a human grant": func(g *Grant) { *g = fixtureGrant(now); g.Lifetime, g.AgentSessionID = LifetimeSession, "s1" },
	} {
		g := sessionGrant(now, "s1")
		mutate(&g)
		require.Error(t, g.ValidateMVP(), name)
	}
	user := fixtureResolution()
	user.AgentSession = &AgentSession{ID: "s1", EndsAt: now.Add(time.Hour)}
	_, err := ProjectAgentResolution(context.Background(), user, agentSubject(), nil, nil)
	require.Error(t, err, "a user's resolution never carries a session into projection")
}

// Inspection evaluates without binding: an unbound session grant reports nothing.
func TestSessionGrantSkippedWithoutBinding(t *testing.T) {
	now := time.Now()
	r := sessionResolution(now, sessionGrant(now, "s1"), &AgentSession{ID: "s2", EndsAt: now.Add(time.Hour)})
	p := newOperationReadPolicy(r, now, func() time.Time { return now })
	require.True(t, p.connection.IsZero())
	r.AgentSession.ID = "s1"
	p = newOperationReadPolicy(r, now, func() time.Time { return now })
	require.False(t, p.connection.IsZero())
}
