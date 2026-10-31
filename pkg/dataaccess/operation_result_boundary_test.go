package dataaccess

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestObjectGrantsNeverSubstituteForConnectionResultAuthority(t *testing.T) {
	resolution := fixtureResolution()
	resolution.Protection.ReadPosture = ReadNone
	grant := fixtureGrant(resolution.ServerTime)
	grant.Scope.Objects = []ObjectRef{{Namespace: "sales", Name: "Customer"}}
	grant.Scope.Requires = []string{ObjectsExact}
	resolution.Grants = []Grant{grant}
	a := testAuthority(t, fixedResolver(resolution))
	called := false
	result, err := a.CaptureResult(context.Background(), resultRequest(), func(context.Context) ([]byte, error) {
		called = true
		return []byte(`{"record":{"private":"value"}}`), nil
	})
	require.Error(t, err)
	require.Nil(t, result)
	require.False(t, called)
	original := testAuthority(t, fixedResolver(fixtureResolution()))
	envelope, started := envelopeFixture(t, original)
	// Even trusted transport metadata matching this object grant set cannot
	// manufacture independent connection-wide result authority.
	envelope.Lease.GrantSetDigest, err = resolutionDigest(resolution)
	require.NoError(t, err)
	owned := envelope.Payload
	result, err = a.ImportResult(context.Background(), resultRequest(), started, envelope)
	require.Error(t, err)
	require.Nil(t, result)
	require.Equal(t, make([]byte, len(owned)), []byte(owned))
}
func TestMixedObjectAndConnectionResultCannotContinueAfterConnectionGrantEnds(t *testing.T) {
	for _, mode := range []string{"revoke", "expiry", "downscope"} {
		t.Run(mode, func(t *testing.T) {
			resolution := fixtureResolution()
			resolution.Protection.ReadPosture = ReadNone
			connection := fixtureGrant(resolution.ServerTime)
			connection.ID = "connection"
			object := connection
			object.ID = "other"
			object.Scope.Objects = []ObjectRef{{Namespace: "sales", Name: "Customer"}}
			object.Scope.Requires = []string{ObjectsExact}
			resolution.Grants = []Grant{connection, object}
			a := testAuthority(t, func(context.Context, ResolveRequest) (Resolution, error) { return resolution, nil })
			result := captureFixture(t, a, context.Background())
			envelope, started := envelopeFixture(t, a)
			imported, err := a.ImportResult(context.Background(), resultRequest(), started, envelope)
			require.NoError(t, err)
			defer imported.Close()
			switch mode {
			case "revoke":
				resolution.Grants[0].State = Revoked
			case "expiry":
				resolution.Grants[0].ExpiresAt = resolution.ServerTime.Add(-time.Second)
			case "downscope":
				resolution.Grants[0].Scope = object.Scope
			}
			var out bytes.Buffer
			require.Error(t, result.WriteEnvelope(context.Background(), &out))
			require.Empty(t, out.Bytes())
			assertResultPurged(t, result)
			require.Error(t, imported.WritePayload(context.Background(), &out))
			require.Empty(t, out.Bytes())
			assertResultPurged(t, imported)
		})
	}
}
func TestObjectGrantDoesNotNarrowUnrestrictedPointBaseline(t *testing.T) {
	resolution := fixtureResolution()
	grant := fixtureGrant(resolution.ServerTime)
	grant.Scope.Objects = []ObjectRef{{Namespace: "sales", Name: "Customer"}}
	grant.Scope.Requires = []string{ObjectsExact}
	resolution.Grants = []Grant{grant}
	a := testAuthority(t, fixedResolver(resolution))
	r := captureFixture(t, a, context.Background())
	var out bytes.Buffer
	require.NoError(t, r.WritePayload(context.Background(), &out))
	require.NotEmpty(t, out.Bytes())
}

// A cancellation between the network guard and the final serialized write must
// still purge and send zero bytes. Exercise the final boundary directly so the
// earlier network guard cannot mask this independent last check.
func TestResultFinalSerializedGuardRejectsLateCallerCancellation(t *testing.T) {
	a := testAuthority(t, fixedResolver(fixtureResolution()))
	r := captureFixture(t, a, context.Background())
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, r.guard(ctx))
	cancel()
	var out bytes.Buffer
	require.ErrorIs(t, r.writeAuthorized(ctx, &out, true), context.Canceled)
	require.Empty(t, out.Bytes())
	assertResultPurged(t, r)
}
