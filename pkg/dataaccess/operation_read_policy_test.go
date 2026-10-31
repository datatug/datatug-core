package dataaccess

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/dal-go/dalgo/access"
	"github.com/dal-go/dalgo/dal"
	"github.com/stretchr/testify/require"
)

func operationQuery(schema, name, alias string) dal.StructuredQuery {
	return dal.From(dal.NewQualifiedRootCollectionRef(schema, name, alias)).NewQuery().SelectKeysOnly(reflect.String)
}
func operationRequest(query dal.Query) *access.Request {
	return &access.Request{Operation: access.Query, Resources: access.ResourcesForQuery(query)}
}
func TestOperationPolicyUsesDALgoResourcesAndIndependentGrantUnion(t *testing.T) {
	now := compositionTime()
	grant := objectGrant("customer", ObjectRef{Namespace: "sales", Name: "Customer"})
	for _, posture := range []ReadPosture{ReadAll, ReadNone} {
		p := newOperationReadPolicy(compositionResolution(posture, grant), now, func() time.Time { return now })
		for _, name := range []string{"Customer", "Order", "Customer"} {
			request := operationRequest(operationQuery("sales", name, ""))
			_, err := p.deadlineFor(request)
			if posture == ReadAll || name == "Customer" {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, access.ErrAccessDenied)
			}
		}
		// Actual recording/memory wrappers still execute ordinary DALgo policies.
		compositionReadVector(t, operationQuery("sales", "Customer", "c"), true, p)
		compositionReadVector(t, operationQuery("sales", "Order", "o"), posture == ReadAll, p)
	}
}
func TestOperationPolicyHasExactNamesAndNoOpaqueOrPointWidening(t *testing.T) {
	now := compositionTime()
	object := ObjectRef{Namespace: "a.b", Name: `"Customer".part`}
	resolution := compositionResolution(ReadNone, objectGrant("literal", object))
	p := newOperationReadPolicy(resolution, now, func() time.Time { return now })
	require.NoError(t, p.Authorize(context.Background(), *operationRequest(operationQuery(object.Namespace, object.Name, "alias"))))
	for _, query := range []dal.Query{operationQuery("a", `b."Customer".part`, ""), operationQuery("a.b", `"customer".part`, ""), keyQuery(), dal.NewTextQuery("SELECT private_values", nil)} {
		require.ErrorIs(t, p.Authorize(context.Background(), *operationRequest(query)), access.ErrAccessDenied)
	}
	require.Error(t, p.Authorize(context.Background(), access.Request{Operation: access.Get, Resources: []access.Resource{access.RecordResourceForKey(nil)}}))
	require.Error(t, p.Authorize(context.Background(), access.Request{Operation: access.Query}))
	require.Error(t, p.Authorize(context.Background(), *operationRequest(dal.From(dal.NewRootCollectionRef("Customer", "")).NewQuery().SelectKeysOnly(reflect.String))))
	require.Error(t, p.Authorize(context.Background(), *operationRequest(dal.From(dal.NewDatabaseCollectionRef("other", "a.b", object.Name, "")).NewQuery().SelectKeysOnly(reflect.String))))
	all := newOperationReadPolicy(compositionResolution(ReadAll), now, func() time.Time { return now })
	require.Error(t, all.Authorize(context.Background(), access.Request{Operation: access.Set, Resources: access.ResourcesForQuery(operationQuery("sales", "Customer", ""))}))
	require.Error(t, all.Authorize(context.Background(), access.Request{Operation: access.Query, Resources: []access.Resource{access.CollectionGroup("Customer")}}))
}
func TestOperationPolicySelectedResourceDeadlineAndImmutableInputs(t *testing.T) {
	now := compositionTime()
	a := objectGrant("a", ObjectRef{Namespace: "sales", Name: "Customer"})
	a.ExpiresAt = now.Add(5 * time.Second)
	b := objectGrant("b", ObjectRef{Namespace: "sales", Name: "Order"})
	b.ExpiresAt = now.Add(40 * time.Second)
	resolution := compositionResolution(ReadNone, a, b)
	p := newOperationReadPolicy(resolution, now, func() time.Time { return now })
	resolution.Grants[0].Scope.Objects[0].Name = "forged"
	request := operationRequest(operationQuery("sales", "Customer", ""))
	deadline, err := p.deadlineFor(request)
	require.NoError(t, err)
	require.Equal(t, compositionTime().Add(5*time.Second), deadline)
	_, err = p.deadlineFor(nil)
	require.Error(t, err, "object grants cannot substitute for connection-wide result authority")
	now = now.Add(5 * time.Second)
	require.Error(t, p.Authorize(context.Background(), *request))
	require.NoError(t, p.Authorize(context.Background(), *operationRequest(operationQuery("sales", "Order", ""))))
	// Longer duplicate grants widen the same object without making another
	// operation inherit that deadline or retaining caller-owned slices.
	later := objectGrant("later", ObjectRef{Namespace: "sales", Name: "Customer"})
	later.ExpiresAt = compositionTime().Add(20 * time.Second)
	p = newOperationReadPolicy(compositionResolution(ReadNone, a, later), compositionTime(), func() time.Time { return now })
	deadline, err = p.deadlineFor(request)
	require.NoError(t, err)
	require.Equal(t, later.ExpiresAt, deadline)
}
func TestOperationPolicyDoesNotNarrowBaselineAndMandatoryDeniesIntersect(t *testing.T) {
	now := compositionTime()
	connection := objectGrant("connection")
	connection.ExpiresAt = now.Add(10 * time.Second)
	for _, posture := range []ReadPosture{ReadAll, ReadNone} {
		p := newOperationReadPolicy(compositionResolution(posture, connection), now, func() time.Time { return now })
		deadline, err := p.deadlineFor(nil)
		require.NoError(t, err)
		if posture == ReadAll {
			require.Equal(t, now.Add(time.Minute), deadline)
		} else {
			require.Equal(t, connection.ExpiresAt, deadline)
		}
		mandatory := access.MustPolicy("mandatory", access.TableScope(access.TableName{Schema: "sales", Name: "Customer"}, access.Deny(access.Read)))
		compositionReadVector(t, operationQuery("sales", "Customer", ""), false, p, mandatory)
	}
	inactive := connection
	inactive.State = Revoked
	p := newOperationReadPolicy(compositionResolution(ReadNone, inactive), now, func() time.Time { return now })
	_, err := p.deadlineFor(nil)
	require.Error(t, err)
}
func TestCopyReadRequestDropsUntrustedMutableQueryAndColumns(t *testing.T) {
	original := operationRequest(operationQuery("sales", "Customer", ""))
	original.Query, original.Columns = keyQuery(), [][]string{{"secret"}}
	copy := copyReadRequest(original)
	original.Resources[0] = access.OpaqueQuery("changed")
	require.Nil(t, copy.Query)
	require.Nil(t, copy.Columns)
	_, ok := copy.Resources[0].Table()
	require.True(t, ok)
	require.Nil(t, copyReadRequest(nil))
}

func TestRequestLeaseReauthorizesExactResourceAndOriginalGrantSet(t *testing.T) {
	for _, change := range []string{"expiry", "revoke", "downscope", "replacement", "resolver-error"} {
		t.Run(change, func(t *testing.T) {
			resolution := fixtureResolution()
			resolution.Protection.ReadPosture = ReadNone
			aGrant := fixtureGrant(resolution.ServerTime)
			aGrant.ID, aGrant.ExpiresAt = "a", resolution.ServerTime.Add(10*time.Second)
			aGrant.Scope.Objects, aGrant.Scope.Requires = []ObjectRef{{Namespace: "sales", Name: "Customer"}}, []string{ObjectsExact}
			bGrant := fixtureGrant(resolution.ServerTime)
			bGrant.ID = "b"
			bGrant.Scope.Objects, bGrant.Scope.Requires = []ObjectRef{{Namespace: "sales", Name: "Order"}}, []string{ObjectsExact}
			resolution.Grants = []Grant{aGrant, bGrant}
			fail := false
			a := testAuthority(t, func(context.Context, ResolveRequest) (Resolution, error) {
				if fail {
					return Resolution{}, context.DeadlineExceeded
				}
				return resolution, nil
			})
			request := operationRequest(operationQuery("sales", "Customer", ""))
			ctx, bound, cancel, err := a.prepareRequest(context.Background(), nil, request)
			require.NoError(t, err)
			defer cancel()
			deadline, ok := ctx.Deadline()
			require.True(t, ok)
			require.WithinDuration(t, time.Now().Add(10*time.Second), deadline, time.Second)
			request.Resources[0] = access.OpaqueQuery("caller mutation")
			_, refreshed, stop, err := a.prepare(ctx, bound)
			require.NoError(t, err)
			stop()
			require.Equal(t, bound.deadline, refreshed.deadline)
			switch change {
			case "expiry":
				resolution.Grants[0].ExpiresAt = resolution.ServerTime
			case "revoke":
				resolution.Grants[0].State = Revoked
			case "downscope":
				resolution.Grants[0].Scope.Objects[0].Name = "Other"
			case "replacement":
				resolution.Grants[0].ID = "replacement-a"
			case "resolver-error":
				fail = true
			}
			require.ErrorIs(t, a.recheck(ctx, bound), access.ErrAccessDenied)
			// An unrelated still-active grant must not validate A's held values.
			require.NoError(t, newOperationReadPolicy(resolution, time.Now(), time.Now).Authorize(context.Background(), *operationRequest(operationQuery("sales", "Order", ""))))
		})
	}
}

func TestFrozenQualifiedProfileDoesNotGuessNamesOrBroadenShape(t *testing.T) {
	query := dal.From(dal.NewQualifiedRootCollectionRef(`a.b`, `"Customer".part`, "alias")).NewQuery().Limit(7).Offset(3).StartAfter("cursor").SelectKeysOnly(reflect.String)
	_, ok := freezeKeysQuery(query)
	require.False(t, ok, "default opener has no native naming proof")
	frozen, ok := freezeNamedKeysQuery(query, true)
	require.True(t, ok)
	got := frozen.(dal.StructuredQuery)
	resource, mapped := access.ResourcesForQuery(got)[0].Table()
	require.True(t, mapped)
	require.Equal(t, access.TableName{Schema: `a.b`, Name: `"Customer".part`}, resource)
	require.Equal(t, 7, got.Limit())
	require.Equal(t, 3, got.Offset())
	require.Equal(t, dal.Cursor("cursor"), got.StartAfter())
	for _, bad := range []dal.Query{
		dal.From(dal.NewQualifiedRootCollectionRef("sales", "Customer", "")).NewQuery().Where(dal.NewComparison(dal.Field("id"), dal.Equal, dal.Constant{Value: 1})).SelectKeysOnly(reflect.String),
		dal.From(dal.NewQualifiedRootCollectionRef("sales", "Customer", "")).NewQuery().SelectColumns(dal.Column{Expression: dal.NewAggregate("UNKNOWN", false, dal.Field("id"))}),
		compositionJoin(dal.NewQualifiedRootCollectionRef("sales", "Customer", "c"), dal.NewQualifiedRootCollectionRef("sales", "Order", "o")),
		dal.From(dal.NewQualifiedRootCollectionRef("*", "Customer", "")).NewQuery().SelectKeysOnly(reflect.String),
		dal.NewTextQuery("SELECT private_values", nil),
	} {
		_, ok := freezeNamedKeysQuery(bad, true)
		require.False(t, ok)
	}
}

func TestOperationPolicyNeverExceedsResolutionCeiling(t *testing.T) {
	now := compositionTime()
	grant := objectGrant("long")
	grant.ExpiresAt = now.Add(2 * time.Hour)
	p := newOperationReadPolicy(compositionResolution(ReadNone, grant), now, func() time.Time { return now })
	deadline, err := p.deadlineFor(nil)
	require.NoError(t, err)
	require.Equal(t, now.Add(time.Minute), deadline)
}
