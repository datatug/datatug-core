package licensing

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCommunityVerifier(t *testing.T) {
	verifier := NewCommunityVerifier()
	ctx := context.Background()

	status, err := verifier.VerifyEntitlement(ctx, "")
	require.NoError(t, err)
	assert.Equal(t, TierCommunity, status.Tier)
	assert.False(t, status.IsExpired(time.Now()))

	hasEnterprise, err := verifier.HasFeature(ctx, FeatureEnterpriseRBAC)
	require.NoError(t, err)
	assert.False(t, hasEnterprise)

	err = verifier.CheckCapacity(ctx, "datasources", 100)
	assert.NoError(t, err)

	cap := status.Capacity
	assert.Equal(t, 1, cap.MaxNodes())
	assert.Equal(t, -1, cap.MaxDatasources())
	assert.Equal(t, 1, cap.MaxTenants())
	_, ok := cap.MetricLimit("custom")
	assert.False(t, ok)
}

func TestEntitlementExpiration(t *testing.T) {
	now := time.Now()
	e := &EntitlementStatus{
		ExpiresAt: now.Add(-time.Hour),
	}
	assert.True(t, e.IsExpired(now))

	e2 := &EntitlementStatus{
		ExpiresAt: now.Add(time.Hour),
	}
	assert.False(t, e2.IsExpired(now))

	e3 := &EntitlementStatus{}
	assert.False(t, e3.IsExpired(now))
}
