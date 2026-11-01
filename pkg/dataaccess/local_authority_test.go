package dataaccess

import (
	"context"
	"testing"
	"time"

	"github.com/dal-go/dalgo/access"
	"github.com/stretchr/testify/require"
)

func TestLocalAuthorityIsExplicitAndCannotBorrowSpaceOrGrants(t *testing.T) {
	local := LocalConnectionRef{AuthorityID: "owned-local-registry", ID: "conn", Fingerprint: fixtureConnection().Fingerprint}
	resolution := fixtureResolution()
	resolution.Connection.Space = ""
	resolution.Connection.LocalAuthorityID = local.AuthorityID
	a, err := NewLocalAuthority(fixtureSubject(), local, fixedResolver(resolution))
	require.NoError(t, err)
	bound, err := a.authorize(context.Background())
	require.NoError(t, err)
	require.NotNil(t, bound)
	require.Empty(t, a.request.Connection.Space)
	_, err = NewAuthority(fixtureSubject(), resolution.Connection, fixedResolver(resolution))
	require.Error(t, err)
	mixed := resolution.Connection
	mixed.Space = "enrolled-space"
	_, err = NewAuthority(fixtureSubject(), mixed, fixedResolver(resolution))
	require.Error(t, err)
	for _, change := range []func(*Resolution){
		func(r *Resolution) { r.Connection.Space = "fake-space" },
		func(r *Resolution) { r.Connection.LocalAuthorityID = "other-host" },
		func(r *Resolution) { r.Connection.LocalAuthorityID = "" },
		func(r *Resolution) { r.Grants = []Grant{fixtureGrant(r.ServerTime)} },
	} {
		r := resolution
		change(&r)
		a, err := NewLocalAuthority(fixtureSubject(), local, fixedResolver(r))
		require.NoError(t, err)
		_, err = a.authorize(context.Background())
		require.Error(t, err)
	}
	_, err = NewLocalAuthority(Subject{}, local, fixedResolver(resolution))
	require.Error(t, err)
	for _, invalid := range []LocalConnectionRef{{}, {AuthorityID: "x", ID: "conn"}, {AuthorityID: "x", ID: "conn", Fingerprint: Fingerprint{Engine: "memory", Database: "fixture"}}} {
		_, err = NewLocalAuthority(fixtureSubject(), invalid, fixedResolver(resolution))
		require.Error(t, err)
	}
	_, err = NewLocalAuthority(fixtureSubject(), local, nil)
	require.Error(t, err)
	other := resolution
	other.Connection.LocalAuthorityID = "another-owned-registry"
	aDigest, err := resolutionDigest(resolution)
	require.NoError(t, err)
	bDigest, err := resolutionDigest(other)
	require.NoError(t, err)
	require.NotEqual(t, aDigest, bDigest)
}

func TestLocalStructuredBaselineUsesExactLocalSourceFacts(t *testing.T) {
	local := LocalConnectionRef{AuthorityID: "local-registry", ID: "conn", Fingerprint: fixtureConnection().Fingerprint}
	resolution := fixtureResolution()
	resolution.Connection.Space = ""
	resolution.Connection.LocalAuthorityID = local.AuthorityID
	a, err := NewLocalAuthority(fixtureSubject(), local, fixedResolver(resolution))
	require.NoError(t, err)
	op := structuredOperation()
	op.TugQL = "from: {name: Customer}\ncolumns: [{field: id}]"
	span := structuredSpan(&previewAdapter{reader: &previewReader{rs: previewRecordset(t, []string{"id"})}})
	span.facts.Connection = resolution.Connection
	span.facts.Resources = []StructuredResourceIdentity{{Path: "/Customer"}}
	r := structuredResult(t, a, context.Background(), op, span)
	require.Equal(t, resolution.Connection, r.metadata.Connection)
	span.facts.Connection.LocalAuthorityID = "changed"
	require.Error(t, r.WritePayload(context.Background(), &discardBuffer{}))
}

type discardBuffer struct{}

func (*discardBuffer) Write(value []byte) (int, error) { return len(value), nil }

func TestOperationReadPolicyRejectsDifferentExplicitPrincipal(t *testing.T) {
	resolution := fixtureResolution()
	p := newOperationReadPolicy(resolution, resolution.ServerTime, func() time.Time { return resolution.ServerTime })
	ctx := access.WithPrincipal(context.Background(), access.Principal{Subject: &access.PrincipalRef{Realm: "sneat", Kind: access.PrincipalKindAgent, ID: "other"}})
	require.False(t, p.Decide(ctx, access.Request{Operation: access.Query, Resources: []access.Resource{access.CollectionResourceFor(nil, "Customer")}}).Allowed)
}
