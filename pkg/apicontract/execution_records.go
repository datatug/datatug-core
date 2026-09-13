package apicontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const executionHashBytes = sha256.Size

// ExecutionRecordScope is the immutable project and environment identity of
// one recorded execution. A security context is deliberately absent: records
// retain the execution scope, while reads are authorized against the caller's
// current security context.
type ExecutionRecordScope struct {
	Project     string `json:"project"`
	Environment string `json:"environment"`
}

func (s ExecutionRecordScope) Validate() error {
	if err := requireNonEmpty("project", s.Project); err != nil {
		return err
	}
	return requireNonEmpty("environment", s.Environment)
}

// ExecutionPrincipal is the server-attested identity and memberships used for
// the recorded execution.
type ExecutionPrincipal struct {
	ID     string   `json:"id"`
	Roles  []string `json:"roles"`
	Groups []string `json:"groups"`
}

func (p ExecutionPrincipal) Validate() error {
	if err := requireNonEmpty("id", p.ID); err != nil {
		return err
	}
	if err := validateNonEmptyStrings("roles", p.Roles); err != nil {
		return err
	}
	return validateNonEmptyStrings("groups", p.Groups)
}

// ExecutionRecord is the immutable receipt of one server-side execution.
// Rows are never embedded here; snapshotRef addresses separately protected
// snapshot bytes, while snapshotExpiredAt records their later unavailability.
type ExecutionRecord struct {
	ID                string                `json:"id"`
	Scope             ExecutionRecordScope  `json:"scope"`
	QueryID           string                `json:"queryId,omitempty"`
	QueryRevision     string                `json:"queryRevision,omitempty"`
	DTQLHash          string                `json:"dtqlHash,omitempty"`
	Parameters        map[string]TypedValue `json:"parameters"`
	BindingsApplied   []Binding             `json:"bindingsApplied"`
	Principal         ExecutionPrincipal    `json:"principal"`
	ExecutedAt        string                `json:"executedAt"`
	DurationMS        int64                 `json:"durationMs"`
	Limitations       []Limitation          `json:"limitations"`
	Provenance        Provenance            `json:"provenance"`
	RowCount          int                   `json:"rowCount"`
	ResultFingerprint string                `json:"resultFingerprint"`
	SnapshotRef       string                `json:"snapshotRef,omitempty"`
	SnapshotExpiredAt string                `json:"snapshotExpiredAt,omitempty"`
}

func (r ExecutionRecord) Validate() error {
	if err := validateExecutionID("id", r.ID); err != nil {
		return err
	}
	if err := r.Scope.Validate(); err != nil {
		return &ValidationError{Field: "scope", Message: err.Error()}
	}
	if (r.QueryID == "") == (r.DTQLHash == "") {
		return &ValidationError{Field: "queryId/dtqlHash", Message: "exactly one of queryId or dtqlHash is required"}
	}
	if r.QueryID != "" && strings.TrimSpace(r.QueryID) != r.QueryID {
		return &ValidationError{Field: "queryId", Message: "must be canonical"}
	}
	if r.QueryRevision != "" && r.QueryID == "" {
		return &ValidationError{Field: "queryRevision", Message: "requires queryId"}
	}
	if r.QueryRevision != "" && strings.TrimSpace(r.QueryRevision) != r.QueryRevision {
		return &ValidationError{Field: "queryRevision", Message: "must be canonical"}
	}
	if r.DTQLHash != "" {
		if err := validateSHA256("dtqlHash", r.DTQLHash); err != nil {
			return err
		}
	}
	for key, value := range r.Parameters {
		if strings.TrimSpace(key) == "" {
			return &ValidationError{Field: "parameters", Message: "parameter id is required"}
		}
		if err := value.Validate(); err != nil {
			return &ValidationError{Field: "parameters", Message: fmt.Sprintf("%s: %s", key, err)}
		}
	}
	for i, binding := range r.BindingsApplied {
		if err := binding.Validate(); err != nil {
			return &ValidationError{Field: "bindingsApplied", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
	}
	if err := r.Principal.Validate(); err != nil {
		return &ValidationError{Field: "principal", Message: err.Error()}
	}
	executedAt, err := validateExecutionTime("executedAt", r.ExecutedAt)
	if err != nil {
		return err
	}
	if r.DurationMS < 0 {
		return &ValidationError{Field: "durationMs", Message: "must not be negative"}
	}
	for i, limitation := range r.Limitations {
		if err := limitation.Validate(); err != nil {
			return &ValidationError{Field: "limitations", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
	}
	if err := r.Provenance.Validate(); err != nil {
		return &ValidationError{Field: "provenance", Message: err.Error()}
	}
	if r.RowCount < 0 {
		return &ValidationError{Field: "rowCount", Message: "must not be negative"}
	}
	if err := validateSHA256("resultFingerprint", r.ResultFingerprint); err != nil {
		return err
	}
	if r.SnapshotRef != "" {
		if strings.TrimSpace(r.SnapshotRef) != r.SnapshotRef {
			return &ValidationError{Field: "snapshotRef", Message: "must be canonical"}
		}
	}
	if r.SnapshotRef != "" && r.SnapshotExpiredAt != "" {
		return &ValidationError{Field: "snapshotExpiredAt", Message: "must be absent while snapshotRef is present"}
	}
	if r.SnapshotExpiredAt != "" {
		expiredAt, err := validateExecutionTime("snapshotExpiredAt", r.SnapshotExpiredAt)
		if err != nil {
			return err
		}
		if !expiredAt.After(executedAt) {
			return &ValidationError{Field: "snapshotExpiredAt", Message: "must be after executedAt"}
		}
	}
	return nil
}

// ExecutionRecordBrief is the list-safe subset of an execution receipt. It
// intentionally omits parameters, bindings, principal and detailed provenance.
type ExecutionRecordBrief struct {
	ID                string               `json:"id"`
	Scope             ExecutionRecordScope `json:"scope"`
	QueryID           string               `json:"queryId,omitempty"`
	QueryRevision     string               `json:"queryRevision,omitempty"`
	DTQLHash          string               `json:"dtqlHash,omitempty"`
	ExecutedAt        string               `json:"executedAt"`
	DurationMS        int64                `json:"durationMs"`
	RowCount          int                  `json:"rowCount"`
	ResultFingerprint string               `json:"resultFingerprint"`
	SnapshotRef       string               `json:"snapshotRef,omitempty"`
	SnapshotExpiredAt string               `json:"snapshotExpiredAt,omitempty"`
	Incident          *IncidentRef         `json:"incident,omitempty"`
}

func (b ExecutionRecordBrief) Validate() error {
	record := ExecutionRecord{
		ID: b.ID, Scope: b.Scope, QueryID: b.QueryID, QueryRevision: b.QueryRevision, DTQLHash: b.DTQLHash,
		Parameters: map[string]TypedValue{}, Principal: ExecutionPrincipal{ID: "brief", Roles: []string{}, Groups: []string{}},
		ExecutedAt: b.ExecutedAt, DurationMS: b.DurationMS, Provenance: Provenance{
			Source: "brief", Mode: ProvenanceModeLive, ObservedAt: b.ExecutedAt, ExecutionProfile: ExecutionProfileProtected,
		},
		RowCount: b.RowCount, ResultFingerprint: b.ResultFingerprint, SnapshotRef: b.SnapshotRef,
		SnapshotExpiredAt: b.SnapshotExpiredAt,
	}
	if err := record.Validate(); err != nil {
		return err
	}
	if b.Incident != nil {
		if err := b.Incident.Validate(); err != nil {
			return &ValidationError{Field: "incident", Message: err.Error()}
		}
	}
	return nil
}

// ExecutionListRequest is GET /datatug/executions.
type ExecutionListRequest struct {
	Scope
	QueryID    string `json:"queryId,omitempty"`
	IncidentID string `json:"incidentId,omitempty"`
	Since      string `json:"since,omitempty"`
	Until      string `json:"until,omitempty"`
	Limit      *int   `json:"limit,omitempty"`
}

func (r ExecutionListRequest) Validate() error {
	if err := r.Scope.Validate(); err != nil {
		return err
	}
	if r.IncidentID != "" {
		if err := validateExecutionID("incidentId", r.IncidentID); err != nil {
			return err
		}
	}
	var since, until time.Time
	var err error
	if r.Since != "" {
		if since, err = validateExecutionTime("since", r.Since); err != nil {
			return err
		}
	}
	if r.Until != "" {
		if until, err = validateExecutionTime("until", r.Until); err != nil {
			return err
		}
	}
	if !since.IsZero() && !until.IsZero() && until.Before(since) {
		return &ValidationError{Field: "until", Message: "must not be before since"}
	}
	if r.Limit != nil && (*r.Limit <= 0 || *r.Limit > executionMaxLimit) {
		return &ValidationError{Field: "limit", Message: fmt.Sprintf("must be between 1 and %d, got %d", executionMaxLimit, *r.Limit)}
	}
	return nil
}

type ExecutionListResponse struct {
	Executions []ExecutionRecordBrief `json:"executions"`
	Truncated  bool                   `json:"truncated"`
}

func (r ExecutionListResponse) Validate() error {
	for i, execution := range r.Executions {
		if err := execution.Validate(); err != nil {
			return &ValidationError{Field: "executions", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
	}
	return nil
}

// SnapshotReadResponse returns recorded evidence. It deliberately is not a
// Result: reading evidence does not execute a query and creates no provenance.
type SnapshotReadResponse struct {
	RecordID          string       `json:"recordId"`
	Recordset         *Recordset   `json:"recordset,omitempty"`
	Limitations       []Limitation `json:"limitations,omitempty"`
	Truncated         bool         `json:"truncated,omitempty"`
	SnapshotExpiredAt string       `json:"snapshotExpiredAt,omitempty"`
}

func (r SnapshotReadResponse) Validate() error {
	if err := validateExecutionID("recordId", r.RecordID); err != nil {
		return err
	}
	if r.SnapshotExpiredAt != "" {
		if _, err := validateExecutionTime("snapshotExpiredAt", r.SnapshotExpiredAt); err != nil {
			return err
		}
		if r.Recordset != nil || len(r.Limitations) != 0 || r.Truncated {
			return &ValidationError{Field: "snapshotExpiredAt", Message: "expired evidence must not include recordset data"}
		}
		return nil
	}
	if r.Recordset == nil {
		return &ValidationError{Field: "recordset", Message: "is required when snapshot evidence has not expired"}
	}
	if err := r.Recordset.Validate(); err != nil {
		return err
	}
	for i, limitation := range r.Limitations {
		if err := limitation.Validate(); err != nil {
			return &ValidationError{Field: "limitations", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
	}
	return nil
}

const (
	SeriesAggregateFirst = "first"
	SeriesAggregateSum   = "sum"
	SeriesAggregateMin   = "min"
	SeriesAggregateMax   = "max"
	SeriesAggregateAvg   = "avg"
	SeriesAggregateCount = "count"
)

// ExecutionSeriesProjection selects either implicit rowCount (both fields
// absent) or one numeric column aggregate.
type ExecutionSeriesProjection struct {
	Column    string `json:"column,omitempty"`
	Aggregate string `json:"aggregate,omitempty"`
}

func (p ExecutionSeriesProjection) Validate() error {
	if (p.Column == "") != (p.Aggregate == "") {
		return &ValidationError{Field: "projection", Message: "column and aggregate must be provided together"}
	}
	if p.Aggregate != "" {
		return requireOneOf("aggregate", p.Aggregate, SeriesAggregateFirst, SeriesAggregateSum, SeriesAggregateMin, SeriesAggregateMax, SeriesAggregateAvg, SeriesAggregateCount)
	}
	return nil
}

type ExecutionSeriesRequest struct {
	Scope
	QueryID         string                    `json:"queryId"`
	BindingsApplied []Binding                 `json:"bindingsApplied"`
	Projection      ExecutionSeriesProjection `json:"projection"`
}

func (r ExecutionSeriesRequest) Validate() error {
	if err := r.Scope.Validate(); err != nil {
		return err
	}
	if err := requireNonEmpty("queryId", r.QueryID); err != nil {
		return err
	}
	for i, binding := range r.BindingsApplied {
		if err := binding.Validate(); err != nil {
			return &ValidationError{Field: "bindingsApplied", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
	}
	return r.Projection.Validate()
}

type ExecutionSeriesPoint struct {
	ExecutedAt string     `json:"executedAt"`
	Value      TypedValue `json:"value"`
	RecordID   string     `json:"recordId"`
}

func (p ExecutionSeriesPoint) Validate() error {
	if _, err := validateExecutionTime("executedAt", p.ExecutedAt); err != nil {
		return err
	}
	if err := p.Value.Validate(); err != nil {
		return &ValidationError{Field: "value", Message: err.Error()}
	}
	return validateExecutionID("recordId", p.RecordID)
}

type ExecutionSeriesResponse struct {
	Points       []ExecutionSeriesPoint `json:"points"`
	OmittedCount int                    `json:"omittedCount"`
}

func (r ExecutionSeriesResponse) Validate() error {
	if r.OmittedCount < 0 {
		return &ValidationError{Field: "omittedCount", Message: "must not be negative"}
	}
	var previous time.Time
	for i, point := range r.Points {
		if err := point.Validate(); err != nil {
			return &ValidationError{Field: "points", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
		at, _ := time.Parse(time.RFC3339, point.ExecutedAt)
		if !previous.IsZero() && at.Before(previous) {
			return &ValidationError{Field: "points", Message: "must be ordered by executedAt"}
		}
		previous = at
	}
	return nil
}

// FingerprintRecordset returns a lowercase SHA-256 digest over the approved
// canonical JSON shape. Validation occurs before encoding, so invalid typed
// rows can never be fingerprinted as evidence.
func FingerprintRecordset(recordset Recordset) (string, error) {
	if err := recordset.Validate(); err != nil {
		return "", err
	}
	canonical := struct {
		Columns []Column       `json:"columns"`
		Rows    [][]TypedValue `json:"rows"`
	}{Columns: recordset.Columns, Rows: recordset.Rows}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("apicontract: fingerprint recordset: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func validateExecutionID(field, value string) error {
	if err := requireNonEmpty(field, value); err != nil {
		return err
	}
	if strings.TrimSpace(value) != value || strings.Contains(value, "/") {
		return &ValidationError{Field: field, Message: "must be canonical and must not contain slash"}
	}
	return nil
}

func validateSHA256(field, value string) error {
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != executionHashBytes || strings.ToLower(value) != value {
		return &ValidationError{Field: field, Message: "must be a lowercase SHA-256 hex digest"}
	}
	return nil
}

func validateExecutionTime(field, value string) (time.Time, error) {
	if err := NewDatetimeValue(value).Validate(); err != nil {
		return time.Time{}, &ValidationError{Field: field, Message: err.Error()}
	}
	parsed, _ := time.Parse(time.RFC3339, value)
	return parsed, nil
}

func validateNonEmptyStrings(field string, values []string) error {
	for i, value := range values {
		if strings.TrimSpace(value) == "" {
			return &ValidationError{Field: field, Message: fmt.Sprintf("index %d: is required", i)}
		}
	}
	return nil
}
