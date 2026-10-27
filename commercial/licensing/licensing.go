package licensing

import (
	"context"
	"errors"
	"time"
)

// ErrFeatureNotEntitled is returned when an enterprise feature is not licensed.
var ErrFeatureNotEntitled = errors.New("feature not included in active license entitlement")

// ErrCapacityExceeded is returned when usage exceeds authorized license limits.
var ErrCapacityExceeded = errors.New("licensed capacity limit exceeded")

// ErrLicenseExpired is returned when a commercial license has expired.
var ErrLicenseExpired = errors.New("commercial license has expired")

// ErrInvalidLicenseToken is returned when a license signature or format is invalid.
var ErrInvalidLicenseToken = errors.New("invalid or malformed license token")

// Feature identifies a commercial or enterprise feature capability.
type Feature string

const (
	FeatureFleetCoordination   Feature = "fleet_coordination"
	FeatureMultiTenant         Feature = "multi_tenant"
	FeatureEnterpriseRBAC      Feature = "enterprise_rbac"
	FeatureAuditLogging        Feature = "audit_logging"
	FeatureDistributedCatalog  Feature = "distributed_catalog"
	FeatureManagedMeaningGraph Feature = "managed_meaning_graph"
)

// LicenseTier represents the tier level of a DataTug installation.
type LicenseTier string

const (
	TierCommunity  LicenseTier = "community"
	TierEvaluation LicenseTier = "evaluation"
	TierEnterprise LicenseTier = "enterprise"
)

// EntitlementStatus describes the verified license state and granted capabilities.
type EntitlementStatus struct {
	Tier         LicenseTier
	CustomerName string
	IssuedAt     time.Time
	ExpiresAt    time.Time
	Features     []Feature
	Capacity     CapacityDescriptor
	Signature    string
}

// IsExpired checks if the entitlement is past its validity expiration.
func (e *EntitlementStatus) IsExpired(now time.Time) bool {
	if e.ExpiresAt.IsZero() {
		return false
	}
	return now.After(e.ExpiresAt)
}

// EntitlementVerifier defines the contract for validating enterprise licenses
// and verifying feature availability without tight coupling to proprietary license servers.
type EntitlementVerifier interface {
	// VerifyEntitlement inspects and validates a raw license token.
	VerifyEntitlement(ctx context.Context, token string) (*EntitlementStatus, error)

	// HasFeature verifies whether a specific enterprise capability is authorized.
	HasFeature(ctx context.Context, feature Feature) (bool, error)

	// CheckCapacity verifies that current usage of a metric does not violate limits.
	CheckCapacity(ctx context.Context, metric string, currentUsage int64) error
}

// CapacityDescriptor defines the metrics and boundaries governing licensed infrastructure.
type CapacityDescriptor interface {
	// MaxNodes returns the maximum allowable active nodes, or -1 if unlimited.
	MaxNodes() int

	// MaxDatasources returns the maximum number of connected databases, or -1 if unlimited.
	MaxDatasources() int

	// MaxTenants returns the maximum number of managed organizational tenants.
	MaxTenants() int

	// MetricLimit returns the licensed limit for a custom named metric.
	MetricLimit(metric string) (int64, bool)
}

// NodeInfo contains metadata describing an active DataTug node registering with a coordinator.
type NodeInfo struct {
	NodeID      string
	Hostname    string
	Version     string
	StartedAt   time.Time
	Environment string
	Labels      map[string]string
}

// NodeStats reports runtime metrics from an active node to the fleet coordinator.
type NodeStats struct {
	NodeID          string
	ReportedAt      time.Time
	ActiveQueries   int
	ConnectedStores int
	MemoryBytes     uint64
}

// CatalogSyncRequest contains parameters for synchronizing catalog state with a coordinator.
type CatalogSyncRequest struct {
	NodeID       string
	ProjectID    string
	LocalVersion string
	Checksum     string
}

// CatalogSyncResponse contains the coordinator's sync directive.
type CatalogSyncResponse struct {
	UpToDate     bool
	LatestCommit string
	DownloadURL  string
}

// CoordinatorContract defines the communication and synchronization boundary
// between DataTug instances and enterprise fleet coordinators.
type CoordinatorContract interface {
	// RegisterNode registers a running DataTug instance with the centralized coordinator.
	RegisterNode(ctx context.Context, info NodeInfo) error

	// Heartbeat sends periodic telemetry and health pings to the coordinator.
	Heartbeat(ctx context.Context, nodeID string, stats NodeStats) error

	// FetchEntitlement retrieves the latest authoritative entitlement status from the coordinator.
	FetchEntitlement(ctx context.Context) (*EntitlementStatus, error)

	// SyncCatalog coordinates catalog synchronization with the centralized enterprise registry.
	SyncCatalog(ctx context.Context, req CatalogSyncRequest) (*CatalogSyncResponse, error)
}
