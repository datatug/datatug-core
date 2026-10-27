package licensing

import (
	"context"
	"time"
)

// CommunityCapacity implements CapacityDescriptor for the standalone community edition.
type CommunityCapacity struct{}

func (c CommunityCapacity) MaxNodes() int                           { return 1 }
func (c CommunityCapacity) MaxDatasources() int                     { return -1 } // unlimited local datasources
func (c CommunityCapacity) MaxTenants() int                         { return 1 }
func (c CommunityCapacity) MetricLimit(metric string) (int64, bool) { return 0, false }

// CommunityVerifier provides a default verifier for community standalone deployments.
type CommunityVerifier struct{}

// NewCommunityVerifier returns an EntitlementVerifier configured for community mode.
func NewCommunityVerifier() EntitlementVerifier {
	return &CommunityVerifier{}
}

func (v *CommunityVerifier) VerifyEntitlement(_ context.Context, _ string) (*EntitlementStatus, error) {
	return &EntitlementStatus{
		Tier:         TierCommunity,
		CustomerName: "Community User",
		IssuedAt:     time.Now(),
		Capacity:     CommunityCapacity{},
		Features:     []Feature{},
	}, nil
}

func (v *CommunityVerifier) HasFeature(_ context.Context, feature Feature) (bool, error) {
	// Community tier does not provide enterprise features
	return false, nil
}

func (v *CommunityVerifier) CheckCapacity(_ context.Context, _ string, _ int64) error {
	// Community tier has no metered usage limits on local operations
	return nil
}
