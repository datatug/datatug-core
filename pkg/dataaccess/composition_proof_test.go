package dataaccess

import (
	"context"
	"reflect"
	"testing"
	"time"

	"github.com/dal-go/dalgo/access"
	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
	"github.com/stretchr/testify/require"
)

func compositionTime() time.Time { return time.Date(2026, 10, 30, 12, 0, 0, 0, time.UTC) }

func objectGrant(id string, objects ...ObjectRef) Grant {
	grant := fixtureGrant(compositionTime())
	grant.ID = id
	grant.Scope.Objects = objects
	if len(objects) > 0 {
		grant.Scope.Requires = []string{ObjectsExact}
	}
	return grant
}

func compositionResolution(posture ReadPosture, grants ...Grant) Resolution {
	resolution := fixtureResolution()
	resolution.ServerTime = compositionTime()
	resolution.ValidUntil = compositionTime().Add(time.Minute)
	resolution.Protection.ReadPosture = posture
	resolution.Grants = grants
	return resolution
}

func compositionPolicy(t *testing.T, posture ReadPosture, grants ...Grant) *readComposition {
	t.Helper()
	policy, err := newReadComposition(compositionResolution(posture, grants...), compositionTime(), compositionTime)
	require.NoError(t, err)
	return policy
}

func compositionQuery(source dal.RecordsetSource) dal.StructuredQuery {
	return dal.From(source).NewQuery().SelectKeysOnly(reflect.String)
}

func compositionJoin(left, right dal.RecordsetSource) dal.StructuredQuery {
	return dal.From(left).Join(dal.NewJoinedSource(right, dal.JoinInner,
		dal.NewComparison(dal.NewFieldRef(left.Alias(), "id"), dal.Equal, dal.NewFieldRef(right.Alias(), "id")),
	)).NewQuery().SelectKeysOnly(reflect.String)
}

type compositionMemory struct{ *memoryRecorder }

func (m *compositionMemory) ExecuteQueryToRecordsReader(ctx context.Context, query dal.Query) (dal.RecordsReader, error) {
	m.calls = append(m.calls, "Query")
	return m.DB.ExecuteQueryToRecordsReader(ctx, query)
}

// The memory adapter's query execution is real, but its schema/name behavior
// is no evidence about native engine resolution. Both record every dispatch.
func compositionReadVector(t *testing.T, query dal.Query, allowed bool, policies ...access.Policy) {
	t.Helper()
	a := &recordingAdapter{reader: &testReader{next: func() (record.Record, error) { return nil, dal.ErrNoMoreRecords }}}
	b := &compositionMemory{newMemoryRecorder(t)}
	for name, backend := range map[string]struct {
		read  dal.ReadSession
		calls *[]string
	}{"recording": {a, &a.calls}, "memory": {b, &b.calls}} {
		t.Run(name, func(t *testing.T) {
			secured := access.SecureReadSession(backend.read, policies...)
			reader, err := secured.ExecuteQueryToRecordsReader(context.Background(), query)
			if !allowed {
				require.ErrorIs(t, err, access.ErrAccessDenied)
				require.Empty(t, *backend.calls, "denial must precede every adapter call")
				require.Nil(t, reader)
				return
			}
			require.NoError(t, err)
			require.Equal(t, []string{"Query"}, *backend.calls)
			_, err = dal.ReadAllToRecords(context.Background(), reader)
			require.NoError(t, err)
		})
	}
}

func TestCOMP1PostureAndConnectionGrantOnTwoAdapters(t *testing.T) {
	plain := compositionQuery(dal.NewRootCollectionRef("docs", ""))
	qualified := compositionQuery(dal.NewQualifiedRootCollectionRef("sales", "Customer", ""))
	for _, query := range []dal.Query{plain, qualified} {
		compositionReadVector(t, query, true, compositionPolicy(t, ReadAll))
		compositionReadVector(t, query, false, compositionPolicy(t, ReadNone))
		compositionReadVector(t, query, true, compositionPolicy(t, ReadNone, objectGrant("connection")))
	}
	a := &recordingAdapter{}
	b := newMemoryRecorder(t)
	for _, raw := range []struct {
		backend dal.ReadwriteSession
		calls   *[]string
	}{{a, &a.calls}, {b, &b.calls}} {
		secured := access.SecureReadwriteSession(raw.backend, compositionPolicy(t, ReadAll, objectGrant("connection")))
		for _, write := range writeVectors(context.Background(), secured) {
			require.ErrorIs(t, write(), access.ErrAccessDenied)
			require.Empty(t, *raw.calls)
		}
	}
}

func TestCOMP2GrantUnionNeverNarrowsPostureOrAnotherGrant(t *testing.T) {
	customer := ObjectRef{Namespace: "sales", Name: "Customer"}
	orders := ObjectRef{Namespace: "sales", Name: "Order"}
	customerSource := dal.NewQualifiedRootCollectionRef(customer.Namespace, customer.Name, "c")
	orderSource := dal.NewQualifiedRootCollectionRef(orders.Namespace, orders.Name, "o")
	unrelated := objectGrant("unrelated", ObjectRef{Namespace: "other", Name: "Customer"})
	duplicate := objectGrant("duplicate", customer)
	for _, grants := range [][]Grant{nil, {unrelated}, {objectGrant("customer", customer), unrelated, duplicate, duplicate}} {
		policy := compositionPolicy(t, ReadAll, grants...)
		compositionReadVector(t, compositionQuery(orderSource), true, policy)
		compositionReadVector(t, compositionJoin(customerSource, orderSource), true, policy)
		compositionReadVector(t, compositionQuery(dal.NewRootCollectionRef("Order", "")), true, policy)
	}
	compositionReadVector(t, compositionJoin(customerSource, orderSource), false, compositionPolicy(t, ReadNone, objectGrant("customer", customer)))
	compositionReadVector(t, compositionJoin(customerSource, orderSource), true, compositionPolicy(t, ReadNone, objectGrant("customer", customer), objectGrant("order", orders), duplicate))
	compositionReadVector(t, compositionJoin(customerSource, dal.NewQualifiedRootCollectionRef("sales", "Customer", "c2")), true, compositionPolicy(t, ReadNone, objectGrant("customer", customer), duplicate))
	// One policy instance decides two successive requests. A compiled table
	// subset from the first is never retained for the second request.
	policy := compositionPolicy(t, ReadAll, unrelated)
	compositionReadVector(t, compositionQuery(customerSource), true, policy)
	compositionReadVector(t, compositionQuery(orderSource), true, policy)
}

func TestCOMP3MandatoryDeniesCannotBeWidened(t *testing.T) {
	customer := ObjectRef{Namespace: "sales", Name: "Customer"}
	orders := ObjectRef{Namespace: "sales", Name: "Order"}
	mandatory := access.MustPolicy("mandatory", access.Root(access.Allow(access.Read, "paths")),
		access.TableScope(access.TableName{Schema: customer.Namespace, Name: customer.Name}, access.Deny(access.Read, "excluded")),
		access.TableScope(access.TableName{Schema: orders.Namespace, Name: orders.Name}, access.Allow(access.Read, "orders")))
	for _, posture := range []ReadPosture{ReadAll, ReadNone} {
		policy := compositionPolicy(t, posture, objectGrant("both", customer, orders))
		compositionReadVector(t, compositionQuery(dal.NewQualifiedRootCollectionRef("sales", "Customer", "")), false, policy, mandatory)
		compositionReadVector(t, compositionQuery(dal.NewQualifiedRootCollectionRef("sales", "Order", "")), true, policy, mandatory)
		compositionReadVector(t, compositionJoin(dal.NewQualifiedRootCollectionRef("sales", "Order", "o"), dal.NewQualifiedRootCollectionRef("sales", "Customer", "c")), false, policy, mandatory)
	}
}

type compositionUnknown struct{}

func (compositionUnknown) String() string { return "unrecognized" }

type compositionUnknownSource struct{ dal.CollectionRef }

func TestCOMP4RawUnknownAndUnresolvedSourcesNeverDispatch(t *testing.T) {
	base := dal.NewRootCollectionRef("docs", "d")
	cycle := dal.From(base)
	cycle.Join(dal.NewJoinedFrom(cycle, dal.JoinInner, dal.NewComparison(dal.Field("id"), dal.Equal, dal.Field("id"))))
	expression := &dal.BinaryExpression{Operator: dal.Add, Right: dal.Constant{Value: 1}}
	expression.Left = expression
	var deep dal.Expression = dal.Field("id")
	for range 66 {
		deep = dal.Binary(deep, dal.Add, dal.Constant{Value: 1})
	}
	queries := map[string]dal.Query{
		"raw":                dal.NewTextQuery("SELECT 1", nil),
		"unknown source":     compositionQuery(compositionUnknownSource{base}),
		"unknown condition":  dal.From(base).NewQuery().Where(compositionUnknown{}).SelectKeysOnly(reflect.String),
		"unknown expression": dal.From(base).NewQuery().SelectColumns(dal.Column{Expression: compositionUnknown{}}),
		"unknown join":       compositionJoin(base, compositionUnknownSource{dal.NewRootCollectionRef("other", "o")}),
		"cycle":              cycle.NewQuery().SelectKeysOnly(reflect.String),
		"expression cycle":   dal.From(base).NewQuery().SelectColumns(dal.Column{Expression: expression}),
		"too deep":           dal.From(base).NewQuery().SelectColumns(dal.Column{Expression: deep}),
		"database schema":    compositionQuery(dal.NewDatabaseCollectionRef("app", "sales", "Customer", "")),
		"database only":      compositionQuery(dal.NewDatabaseCollectionRef("app", "", "Customer", "")),
		"collection group":   compositionQuery(dal.NewCollectionGroupRef("docs", "")),
	}
	for name, query := range queries {
		t.Run(name, func(t *testing.T) {
			compositionReadVector(t, query, false, compositionPolicy(t, ReadAll, objectGrant("connection")))
		})
	}
}

func TestCOMP5CanonicalIdentitiesAreExactOrRefused(t *testing.T) {
	grant := objectGrant("customer", ObjectRef{Namespace: "sales", Name: "Customer"})
	qualified := dal.NewQualifiedRootCollectionRef("sales", "Customer", "alias")
	for _, source := range []dal.RecordsetSource{qualified, &qualified, dal.NewQualifiedRootCollectionRef("sales", "Customer", "other")} {
		compositionReadVector(t, compositionQuery(source), true, compositionPolicy(t, ReadNone, grant))
	}
	for _, source := range []dal.RecordsetSource{
		dal.NewQualifiedRootCollectionRef("Sales", "Customer", ""), dal.NewQualifiedRootCollectionRef("sales", "customer", ""),
		dal.NewQualifiedRootCollectionRef("public", "Customer", ""), dal.NewQualifiedRootCollectionRef("sales", `"Customer"`, ""),
		dal.NewRootCollectionRef("Customer", ""), dal.NewRootCollectionRef("sales.Customer", ""),
		dal.NewCollectionRef("Customer", "", record.NewKeyWithID("parent", "one")),
	} {
		compositionReadVector(t, compositionQuery(source), false, compositionPolicy(t, ReadNone, grant))
	}
	for _, object := range []ObjectRef{{Namespace: "a", Name: "b.c"}, {Namespace: "a.b", Name: "c"}, {Namespace: `"sales"`, Name: `"Customer"`}} {
		compositionReadVector(t, compositionQuery(dal.NewQualifiedRootCollectionRef(object.Namespace, object.Name, "")), true, compositionPolicy(t, ReadNone, objectGrant("literal", object)))
	}
	compositionReadVector(t, compositionQuery(dal.NewQualifiedRootCollectionRef("a.b", "c", "")), false, compositionPolicy(t, ReadNone, objectGrant("parts", ObjectRef{Namespace: "a", Name: "b.c"})))
	// With no verified mapping for bare names, DALgo refuses a case-fold
	// basename collision. No guessed default schema or SQL unquoting is used.
	for _, bare := range []string{"Customer", "CUSTOMER"} {
		compositionReadVector(t, compositionJoin(qualified, dal.NewRootCollectionRef(bare, "bare")), false, compositionPolicy(t, ReadAll))
	}
	otherSchema := dal.NewQualifiedRootCollectionRef("public", "Customer", "p")
	compositionReadVector(t, compositionJoin(qualified, otherSchema), true, compositionPolicy(t, ReadAll))
	compositionReadVector(t, compositionJoin(qualified, otherSchema), false, compositionPolicy(t, ReadNone, grant))
	compositionReadVector(t, compositionJoin(qualified, otherSchema), true, compositionPolicy(t, ReadNone, grant, objectGrant("public", ObjectRef{Namespace: "public", Name: "Customer"})))
	compositionReadVector(t, compositionQuery(dal.NewRootCollectionRef("Customer", "")), false, compositionPolicy(t, ReadNone, objectGrant("unresolved", ObjectRef{Name: "Customer"})))
	// Point keys carry no Table. Even an identically named object grant cannot
	// become a Collection-prefix permission on root or nested records.
	for _, key := range []*record.Key{record.NewKeyWithID("Customer", "one"), record.NewKeyWithParentAndID(record.NewKeyWithID("parent", "one"), "Customer", "two")} {
		policy := compositionPolicy(t, ReadNone, grant, objectGrant("bare", ObjectRef{Name: "Customer"}))
		a := &recordingAdapter{}
		_, err := access.SecureReadSession(a, policy).Exists(context.Background(), key)
		require.ErrorIs(t, err, access.ErrAccessDenied)
		require.Empty(t, a.calls)
	}
}

func TestCOMPGrantDeadlinesAndImmutableSnapshot(t *testing.T) {
	now := compositionTime()
	grant := objectGrant("customer", ObjectRef{Namespace: "sales", Name: "Customer"})
	grant.ExpiresAt = now.Add(5 * time.Second)
	resolution := compositionResolution(ReadNone, grant)
	policy, err := newReadComposition(resolution, now, func() time.Time { return now })
	require.NoError(t, err)
	resolution.Grants[0].Scope.Objects[0].Name = "Other"
	resolution.Protection.ReadPosture = ReadAll
	resolution.Grants[0].ExpiresAt = now.Add(time.Hour)
	query := compositionQuery(dal.NewQualifiedRootCollectionRef("sales", "Customer", ""))
	compositionReadVector(t, query, true, policy)
	compositionReadVector(t, compositionQuery(dal.NewQualifiedRootCollectionRef("sales", "Other", "")), false, policy)
	now = now.Add(5 * time.Second)
	compositionReadVector(t, query, false, policy)
	all, err := newReadComposition(compositionResolution(ReadAll, grant), compositionTime(), func() time.Time { return now })
	require.NoError(t, err)
	compositionReadVector(t, query, true, all)
	now = compositionTime().Add(time.Minute)
	compositionReadVector(t, query, false, all)
	for _, state := range []GrantState{Requested, Refused, Withdrawn, Revoked} {
		inactive := objectGrant("inactive", ObjectRef{Namespace: "sales", Name: "Customer"})
		inactive.State = state
		compositionReadVector(t, query, false, compositionPolicy(t, ReadNone, inactive))
	}
	expired := objectGrant("expired", ObjectRef{Namespace: "sales", Name: "Customer"})
	expired.ExpiresAt = compositionTime()
	compositionReadVector(t, query, false, compositionPolicy(t, ReadNone, expired))
}

func TestCOMPValidationAndAgentCeiling(t *testing.T) {
	for _, change := range []func(*Resolution){
		func(r *Resolution) { r.Protection.ReadPosture = "unknown" },
		func(r *Resolution) { r.Protection.AgentRead = "unknown" },
		func(r *Resolution) { r.Protection.ID = "other" },
		func(r *Resolution) { r.Connection.Fingerprint.Database = "other" },
		func(r *Resolution) { r.Subject.ID = "" },
		func(r *Resolution) { r.ValidUntil = r.ServerTime },
		func(r *Resolution) { r.Grants = []Grant{objectGrant("bad")}; r.Grants[0].Scope.Connection = "other" },
		func(r *Resolution) { r.Grants = []Grant{objectGrant("bad")}; r.Grants[0].Subject.ID = "other" },
		func(r *Resolution) {
			r.Grants = []Grant{objectGrant("bad")}
			r.Grants[0].Permissions = []Permission{DataWrite}
		},
		func(r *Resolution) {
			r.Grants = []Grant{objectGrant("bad")}
			r.Grants[0].Scope.Requires = []string{"unknown"}
		},
		func(r *Resolution) {
			r.Grants = []Grant{objectGrant("bad")}
			r.Grants[0].Scope.Exclude = []Selector{{Kind: "exact", Value: "secret"}}
		},
	} {
		resolution := compositionResolution(ReadAll)
		change(&resolution)
		_, err := newReadComposition(resolution, compositionTime(), compositionTime)
		require.ErrorIs(t, err, access.ErrAccessDenied)
	}
	_, err := newReadComposition(compositionResolution(ReadAll), compositionTime(), nil)
	require.ErrorIs(t, err, access.ErrAccessDenied)
	_, err = newReadComposition(compositionResolution(ReadAll), time.Time{}, compositionTime)
	require.ErrorIs(t, err, access.ErrAccessDenied)
	agent := compositionResolution(ReadAll)
	agent.Subject.Kind = Agent
	agent.Protection.AgentRead = AgentNone
	policy, err := newReadComposition(agent, compositionTime(), compositionTime)
	require.NoError(t, err)
	compositionReadVector(t, keyQuery(), false, policy)
	agent.Protection.AgentRead = AgentInherit
	policy, err = newReadComposition(agent, compositionTime(), compositionTime)
	require.NoError(t, err)
	compositionReadVector(t, keyQuery(), true, policy)
	grant := objectGrant("agent")
	grant.Subject = agent.Subject
	agent.Grants = []Grant{grant}
	_, err = newReadComposition(agent, compositionTime(), compositionTime)
	require.ErrorIs(t, err, access.ErrAccessDenied)
	// Empty resources and the public Policy.Authorize contract fail closed.
	policy = compositionPolicy(t, ReadAll)
	require.ErrorIs(t, policy.Authorize(context.Background(), access.Request{Operation: access.Query}), access.ErrAccessDenied)
	require.NoError(t, policy.Authorize(context.Background(), access.Request{Operation: access.Get, Resources: []access.Resource{access.RecordResourceForKey(record.NewKeyWithID("docs", "one"))}}))
}

func TestCOMP6ProductionProfileRemainsUnchangedWhenProofIsAbsent(t *testing.T) {
	backend := &recordingAdapter{}
	resolution := compositionResolution(ReadNone, objectGrant("customer", ObjectRef{Namespace: "sales", Name: "Customer"}))
	boundary := testDatabase(t, testAuthority(t, fixedResolver(resolution)), dal.NewDB(backend))
	_, err := boundary.Exists(context.Background(), record.NewKeyWithID("docs", "one"))
	require.ErrorIs(t, err, access.ErrAccessDenied, "production object execution is still gated")
	require.Empty(t, backend.calls)
	resolution = compositionResolution(ReadAll)
	boundary = testDatabase(t, testAuthority(t, fixedResolver(resolution)), dal.NewDB(backend))
	_, err = boundary.ExecuteQueryToRecordsReader(context.Background(), compositionQuery(dal.NewQualifiedRootCollectionRef("sales", "Customer", "")))
	require.ErrorIs(t, err, access.ErrAccessDenied, "production broader query shapes remain unsupported")
	require.Empty(t, backend.calls)
	// Existing ordinary policies still run without the prototype: no rule or
	// generic scope was added to DALgo, and no production wiring was changed.
	compositionReadVector(t, keyQuery(), true, readPolicy)
	compositionReadVector(t, compositionQuery(dal.NewQualifiedRootCollectionRef("sales", "Customer", "")), false, readPolicy)
}

func TestCOMPConnectionGrantDeadlineDoesNotExtendBaseline(t *testing.T) {
	now := compositionTime()
	grant := objectGrant("connection")
	grant.ExpiresAt = now.Add(5 * time.Second)
	none, err := newReadComposition(compositionResolution(ReadNone, grant), now, func() time.Time { return now })
	require.NoError(t, err)
	all, err := newReadComposition(compositionResolution(ReadAll, grant), now, func() time.Time { return now })
	require.NoError(t, err)
	compositionReadVector(t, keyQuery(), true, none)
	now = now.Add(5 * time.Second)
	compositionReadVector(t, keyQuery(), false, none)
	compositionReadVector(t, keyQuery(), true, all)
	// A slow resolve consumes the original lease; later Decide calls never
	// rebase the interval onto their current local time.
	now = compositionTime().Add(time.Minute)
	compositionReadVector(t, keyQuery(), false, all)
}
