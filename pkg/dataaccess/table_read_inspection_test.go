package dataaccess

import (
	"context"
	"testing"
	"time"

	"github.com/dal-go/dalgo/access"
	"github.com/stretchr/testify/require"
)

func TestInspectTableReadMatchesAdmissionWithoutReadingRows(t *testing.T) {
	ctx := context.Background()
	customer := ObjectRef{Namespace: "sales", Name: "Customer"}

	t.Run("posture all", func(t *testing.T) {
		r := fixtureResolution()
		got, err := InspectTableRead(ctx, r, nil, customer)
		require.NoError(t, err)
		require.True(t, got.Admitted)
		require.Empty(t, got.Code)
		require.Equal(t, r.ServerTime.Add(time.Minute), got.Until)
		require.Equal(t, access.AssessmentAllow, got.Assessment.Outcome)
	})

	t.Run("posture none without grant", func(t *testing.T) {
		r := fixtureResolution()
		r.Protection.ReadPosture = ReadNone
		got, err := InspectTableRead(ctx, r, nil, customer)
		require.NoError(t, err)
		require.False(t, got.Admitted)
		require.Equal(t, "READ_DENIED", got.Code)
		require.True(t, got.Until.IsZero())
		require.Equal(t, access.AssessmentDeny, got.Assessment.Outcome)
		require.Equal(t, access.CodeNoMatch, got.Assessment.Policies[0].Code)
	})

	t.Run("posture none with object grant", func(t *testing.T) {
		r := fixtureResolution()
		r.Protection.ReadPosture = ReadNone
		g := exactReadGrant(r.ServerTime, customer)
		g.ExpiresAt = r.ServerTime.Add(10 * time.Second)
		r.Grants = []Grant{g}
		got, err := InspectTableRead(ctx, r, nil, customer)
		require.NoError(t, err)
		require.True(t, got.Admitted)
		require.Equal(t, g.ExpiresAt, got.Until)
		require.Equal(t, access.AssessmentAllow, got.Assessment.Outcome)
	})

	t.Run("exact baseline and mandatory deny", func(t *testing.T) {
		doc := exactFixtureDocument()
		snapshot := exactSnapshot(t, doc)
		r := exactProjected(t, doc)
		got, err := InspectTableRead(ctx, r, snapshot, customer)
		require.NoError(t, err)
		require.True(t, got.Admitted)
		require.Equal(t, access.AssessmentAllow, got.Assessment.Outcome)

		got, err = InspectTableRead(ctx, r, snapshot, ObjectRef{Namespace: "sales", Name: "Secret"})
		require.NoError(t, err)
		require.False(t, got.Admitted)
		require.Equal(t, "READ_DENIED", got.Code)
		require.Equal(t, access.AssessmentDeny, got.Assessment.Outcome)
		var mandatory bool
		for _, p := range got.Assessment.Policies {
			if p.Policy == "datatug-exact-mandatory" && p.Code == access.CodeRuleDenied && !p.Allowed {
				mandatory = true
			}
		}
		require.True(t, mandatory, "the sealed mandatory deny is part of the assessment")
	})

	t.Run("exact posture without projection", func(t *testing.T) {
		r := exactRawResolution()
		got, err := InspectTableRead(ctx, r, nil, customer)
		require.NoError(t, err)
		require.False(t, got.Admitted)
		require.Equal(t, "AUTHORITY_UNSUPPORTED", got.Code)
		require.Equal(t, access.AssessmentIndeterminate, got.Assessment.Outcome)
	})

	t.Run("unsealed mandatory snapshot", func(t *testing.T) {
		got, err := InspectTableRead(ctx, fixtureResolution(), &ExactReadSnapshot{}, customer)
		require.NoError(t, err)
		require.True(t, got.Admitted)
		require.Equal(t, access.AssessmentIndeterminate, got.Assessment.Outcome)
	})

	t.Run("inputs refused", func(t *testing.T) {
		//nolint:staticcheck // a nil context is the refused input under test
		_, err := InspectTableRead(nil, fixtureResolution(), nil, customer)
		require.Error(t, err)
		_, err = InspectTableRead(ctx, fixtureResolution(), nil, ObjectRef{Namespace: "sales", Name: "*"})
		require.Error(t, err)
		local := fixtureResolution()
		local.Connection.LocalAuthorityID = "local"
		_, err = InspectTableRead(ctx, local, nil, customer)
		require.Error(t, err)
		canceled, cancel := context.WithCancel(ctx)
		cancel()
		_, err = InspectTableRead(canceled, fixtureResolution(), nil, customer)
		require.Error(t, err)
	})
}

func TestInspectedReadRulesAuthorizeUsesDecideRules(t *testing.T) {
	r := fixtureResolution()
	p := newOperationReadPolicy(r, r.ServerTime, func() time.Time { return r.ServerTime })
	_, req, err := TableRead{Object: ObjectRef{Namespace: "sales", Name: "Customer"}, MaxRows: 1, MaxBytes: 1}.query()
	require.NoError(t, err)
	request := *req
	allow := inspectedReadRules{policy: p, rules: []access.Rule{access.TableScope(access.TableName{Schema: "sales", Name: "Customer"}, access.Allow(access.Read, "table-0"))}}
	require.NoError(t, allow.Authorize(context.Background(), request))
	require.Error(t, inspectedReadRules{policy: p}.Authorize(context.Background(), request))
}
