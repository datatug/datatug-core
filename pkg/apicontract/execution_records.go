package apicontract

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/datatug/datatug-core/pkg/incidents"
)

const executionHashBytes = sha256.Size

// ExecutionRecordScope is the immutable source store, project and environment
// under which one execution ran. The evidence store lives in ExecutionRef.
type ExecutionRecordScope struct {
	StoreID     string `json:"storeId"`
	Project     string `json:"project"`
	Environment string `json:"environment"`
}

func (s ExecutionRecordScope) Validate() error {
	if err := validateExecutionID("storeId", s.StoreID); err != nil {
		return err
	}
	if err := validateExecutionID("project", s.Project); err != nil {
		return err
	}
	return requireCanonicalString("environment", s.Environment)
}

type ExecutionPrincipal struct {
	ID     string   `json:"id"`
	Roles  []string `json:"roles"`
	Groups []string `json:"groups"`
}

func (p ExecutionPrincipal) Validate() error {
	if err := requireCanonicalString("id", p.ID); err != nil {
		return err
	}
	if err := validateNonEmptyStrings("roles", p.Roles); err != nil {
		return err
	}
	return validateNonEmptyStrings("groups", p.Groups)
}

// GrantRef qualifies an incident-local grant and the access.approved mutation
// that made it active. Event resolution remains a server operation.
type GrantRef struct {
	Incident           IncidentRef `json:"incident"`
	GrantID            string      `json:"grantId"`
	ApprovalMutationID string      `json:"approvalMutationId"`
}

func (r GrantRef) Validate() error {
	if err := r.Incident.Validate(); err != nil {
		return &ValidationError{Field: "incident", Message: err.Error()}
	}
	if err := validateExecutionID("grantId", r.GrantID); err != nil {
		return err
	}
	if err := incidents.ValidateMutationID(r.ApprovalMutationID); err != nil {
		return &ValidationError{Field: "approvalMutationId", Message: err.Error()}
	}
	return nil
}

// FieldAccessRef identifies one returned physical column and its optional
// semantic mapping without retaining a value.
type FieldAccessRef struct {
	StoreID     string `json:"storeId"`
	Project     string `json:"project"`
	Environment string `json:"environment"`
	Source      string `json:"source"`
	Collection  string `json:"collection,omitempty"`
	Column      string `json:"column"`
	Entity      string `json:"entity,omitempty"`
	Field       string `json:"field,omitempty"`
}

func (r FieldAccessRef) Validate() error {
	if err := (ExecutionRecordScope{StoreID: r.StoreID, Project: r.Project, Environment: r.Environment}).Validate(); err != nil {
		return err
	}
	if err := requireCanonicalString("source", r.Source); err != nil {
		return err
	}
	if err := requireCanonicalString("column", r.Column); err != nil {
		return err
	}
	if r.Collection != "" {
		if err := requireCanonicalString("collection", r.Collection); err != nil {
			return err
		}
	}
	if (r.Entity == "") != (r.Field == "") {
		return &ValidationError{Field: "entity/field", Message: "must be provided together"}
	}
	if r.Entity != "" {
		if err := requireCanonicalString("entity", r.Entity); err != nil {
			return err
		}
		if err := requireCanonicalString("field", r.Field); err != nil {
			return err
		}
	}
	return nil
}

const (
	MeasurementAggregateFirst    = "first"
	MeasurementAggregateSum      = "sum"
	MeasurementAggregateMin      = "min"
	MeasurementAggregateMax      = "max"
	MeasurementAggregateAvg      = "avg"
	MeasurementAggregateCount    = "count"
	MeasurementAggregateRowCount = "rowCount"
)

type MeasurementProjection struct {
	ID        string `json:"id"`
	Column    string `json:"column,omitempty"`
	Aggregate string `json:"aggregate"`
}

func (p MeasurementProjection) Validate() error {
	if err := requireCanonicalString("id", p.ID); err != nil {
		return err
	}
	if err := requireOneOf("aggregate", p.Aggregate,
		MeasurementAggregateFirst, MeasurementAggregateSum, MeasurementAggregateMin,
		MeasurementAggregateMax, MeasurementAggregateAvg, MeasurementAggregateCount,
		MeasurementAggregateRowCount); err != nil {
		return err
	}
	if p.Aggregate == MeasurementAggregateRowCount {
		if p.Column != "" {
			return &ValidationError{Field: "column", Message: "must be absent for rowCount"}
		}
		return nil
	}
	return requireCanonicalString("column", p.Column)
}

const (
	MeasurementComplete    = "complete"
	MeasurementUnavailable = "unavailable"

	MeasurementReasonNoRows        = "no-rows"
	MeasurementReasonNonNumeric    = "non-numeric"
	MeasurementReasonPolicyLimited = "policy-limited"
	MeasurementReasonTruncated     = "truncated"
	MeasurementReasonSourceRefused = "source-refused"
)

type ScalarMeasurement struct {
	Projection   MeasurementProjection `json:"projection"`
	Completeness string                `json:"completeness"`
	Value        *TypedValue           `json:"value,omitempty"`
	Reason       string                `json:"reason,omitempty"`
}

func (m ScalarMeasurement) Validate() error {
	if err := m.Projection.Validate(); err != nil {
		return &ValidationError{Field: "projection", Message: err.Error()}
	}
	return validateMeasurementOutcome(m.Completeness, m.Value, m.Reason)
}

func validateMeasurementOutcome(completeness string, value *TypedValue, reason string) error {
	switch completeness {
	case MeasurementComplete:
		if value == nil || reason != "" {
			return &ValidationError{Field: "completeness", Message: "complete requires value and forbids reason"}
		}
		if err := value.Validate(); err != nil {
			return &ValidationError{Field: "value", Message: err.Error()}
		}
		if value.Type != ValueTypeNumber && value.Type != ValueTypeInteger && value.Type != ValueTypeDecimal {
			return &ValidationError{Field: "value", Message: "complete measurement must be numeric"}
		}
		return nil
	case MeasurementUnavailable:
		if value != nil {
			return &ValidationError{Field: "value", Message: "must be absent when measurement is unavailable"}
		}
		return requireOneOf("reason", reason,
			MeasurementReasonNoRows, MeasurementReasonNonNumeric, MeasurementReasonPolicyLimited,
			MeasurementReasonTruncated, MeasurementReasonSourceRefused)
	default:
		return &ValidationError{Field: "completeness", Message: "must be complete or unavailable"}
	}
}

// ExecutionRecord is an immutable receipt. Snapshot lifecycle state is stored
// and transported separately from this value.
type ExecutionRecord struct {
	Ref               ExecutionRef               `json:"ref"`
	Scope             ExecutionRecordScope       `json:"scope"`
	QueryID           string                     `json:"queryId,omitempty"`
	QueryRevision     string                     `json:"queryRevision,omitempty"`
	DTQLHash          string                     `json:"dtqlHash,omitempty"`
	Parameters        map[string]TypedValueOrSet `json:"parameters"`
	BindingsApplied   []Binding                  `json:"bindingsApplied"`
	Principal         ExecutionPrincipal         `json:"principal"`
	PolicyFingerprint string                     `json:"policyFingerprint"`
	ExecutedAt        string                     `json:"executedAt"`
	DurationMS        int64                      `json:"durationMs"`
	Limitations       []Limitation               `json:"limitations"`
	Provenance        Provenance                 `json:"provenance"`
	AuthorizedFields  []FieldAccessRef           `json:"authorizedFields"`
	RowCount          int                        `json:"rowCount"`
	ResultFingerprint string                     `json:"resultFingerprint"`
	SnapshotRef       string                     `json:"snapshotRef,omitempty"`
	Incident          *IncidentRef               `json:"incident,omitempty"`
	GrantUses         []GrantRef                 `json:"grantUses,omitempty"`
	Measurements      []ScalarMeasurement        `json:"measurements"`
}

func (r ExecutionRecord) Validate() error {
	if err := r.Ref.Validate(); err != nil {
		return &ValidationError{Field: "ref", Message: err.Error()}
	}
	if err := r.Scope.Validate(); err != nil {
		return &ValidationError{Field: "scope", Message: err.Error()}
	}
	if r.Ref.ProjectID != r.Scope.Project {
		return &ValidationError{Field: "ref", Message: "projectId must match scope project"}
	}
	if err := validateExecutionQueryIdentity(r.QueryID, r.QueryRevision, r.DTQLHash); err != nil {
		return err
	}
	for key, value := range r.Parameters {
		if err := requireCanonicalString("parameters", key); err != nil {
			return err
		}
		if err := value.Validate(); err != nil {
			return &ValidationError{Field: "parameters", Message: fmt.Sprintf("%s: %s", key, err)}
		}
	}
	if err := validateBindings("bindingsApplied", r.BindingsApplied); err != nil {
		return err
	}
	if err := r.Principal.Validate(); err != nil {
		return &ValidationError{Field: "principal", Message: err.Error()}
	}
	if err := validateSHA256("policyFingerprint", r.PolicyFingerprint); err != nil {
		return err
	}
	if _, err := validateExecutionTime("executedAt", r.ExecutedAt); err != nil {
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
	if r.Provenance.QueryID != r.QueryID {
		return &ValidationError{Field: "provenance.queryId", Message: "must match the recorded queryId"}
	}
	seenFields := make(map[FieldAccessRef]struct{}, len(r.AuthorizedFields))
	for i, field := range r.AuthorizedFields {
		if err := field.Validate(); err != nil {
			return &ValidationError{Field: "authorizedFields", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
		if _, ok := seenFields[field]; ok {
			return &ValidationError{Field: "authorizedFields", Message: fmt.Sprintf("duplicate index %d", i)}
		}
		if field.StoreID != r.Scope.StoreID || field.Project != r.Scope.Project || field.Environment != r.Scope.Environment || field.Source != r.Provenance.Source {
			return &ValidationError{Field: "authorizedFields", Message: fmt.Sprintf("index %d must match the recorded source scope", i)}
		}
		seenFields[field] = struct{}{}
	}
	if r.RowCount < 0 {
		return &ValidationError{Field: "rowCount", Message: "must not be negative"}
	}
	if err := validateSHA256("resultFingerprint", r.ResultFingerprint); err != nil {
		return err
	}
	if r.SnapshotRef != "" && strings.TrimSpace(r.SnapshotRef) != r.SnapshotRef {
		return &ValidationError{Field: "snapshotRef", Message: "must be canonical"}
	}
	if r.Incident != nil {
		if err := r.Incident.Validate(); err != nil {
			return &ValidationError{Field: "incident", Message: err.Error()}
		}
	}
	if r.GrantUses != nil && len(r.GrantUses) == 0 {
		return &ValidationError{Field: "grantUses", Message: "must be absent or contain at least one grant"}
	}
	seenGrants := make(map[string]struct{}, len(r.GrantUses))
	for i, grant := range r.GrantUses {
		if err := grant.Validate(); err != nil {
			return &ValidationError{Field: "grantUses", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
		if r.Incident == nil || grant.Incident != *r.Incident {
			return &ValidationError{Field: "grantUses", Message: fmt.Sprintf("index %d incident must equal record incident", i)}
		}
		key := grant.Incident.String() + "/" + grant.GrantID + "/" + grant.ApprovalMutationID
		if _, ok := seenGrants[key]; ok {
			return &ValidationError{Field: "grantUses", Message: fmt.Sprintf("duplicate index %d", i)}
		}
		seenGrants[key] = struct{}{}
	}
	seenMeasurements := make(map[string]struct{}, len(r.Measurements))
	for i, measurement := range r.Measurements {
		if err := measurement.Validate(); err != nil {
			return &ValidationError{Field: "measurements", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
		if _, ok := seenMeasurements[measurement.Projection.ID]; ok {
			return &ValidationError{Field: "measurements", Message: fmt.Sprintf("duplicate projection id %q", measurement.Projection.ID)}
		}
		seenMeasurements[measurement.Projection.ID] = struct{}{}
		if len(r.Limitations) > 0 && measurement.Completeness == MeasurementComplete && measurement.Projection.Aggregate != MeasurementAggregateRowCount {
			return &ValidationError{Field: "measurements", Message: fmt.Sprintf("index %d: policy-limited results may complete only rowCount", i)}
		}
	}
	return nil
}

// SnapshotState is mutable lifecycle state keyed separately by an immutable
// snapshotRef. It is never embedded in ExecutionRecord.
type SnapshotState struct {
	Availability string `json:"availability"`
	ChangedAt    string `json:"changedAt"`
	Reason       string `json:"reason,omitempty"`
}

const (
	SnapshotAvailable = "available"
	SnapshotExpired   = "expired"
	SnapshotDeleted   = "deleted"
)

func (s SnapshotState) Validate() error {
	if err := requireOneOf("availability", s.Availability, SnapshotAvailable, SnapshotExpired, SnapshotDeleted); err != nil {
		return err
	}
	if _, err := validateExecutionTime("changedAt", s.ChangedAt); err != nil {
		return err
	}
	if s.Availability == SnapshotAvailable && s.Reason != "" {
		return &ValidationError{Field: "reason", Message: "must be absent while snapshot is available"}
	}
	if s.Reason != "" && strings.TrimSpace(s.Reason) != s.Reason {
		return &ValidationError{Field: "reason", Message: "must be canonical"}
	}
	return nil
}

// ValidateTransition accepts idempotent retries and the only state advance
// allowed by the lifecycle: available to one terminal state at a later time.
func (s SnapshotState) ValidateTransition(next SnapshotState) error {
	if err := s.Validate(); err != nil {
		return &ValidationError{Field: "current", Message: err.Error()}
	}
	if err := next.Validate(); err != nil {
		return &ValidationError{Field: "next", Message: err.Error()}
	}
	if s == next {
		return nil
	}
	if s.Availability != SnapshotAvailable || next.Availability == SnapshotAvailable {
		return &ValidationError{Field: "availability", Message: "may advance only from available to expired or deleted"}
	}
	currentAt, _ := time.Parse(time.RFC3339, s.ChangedAt)
	nextAt, _ := time.Parse(time.RFC3339, next.ChangedAt)
	if !nextAt.After(currentAt) {
		return &ValidationError{Field: "changedAt", Message: "must advance when availability changes"}
	}
	return nil
}

// ExecutionRecordBrief is the list-safe receipt subset. SnapshotState is the
// current separately stored lifecycle value, not a mutation of the receipt.
type ExecutionRecordBrief struct {
	Ref               ExecutionRef         `json:"ref"`
	Scope             ExecutionRecordScope `json:"scope"`
	QueryID           string               `json:"queryId,omitempty"`
	QueryRevision     string               `json:"queryRevision,omitempty"`
	DTQLHash          string               `json:"dtqlHash,omitempty"`
	ExecutedAt        string               `json:"executedAt"`
	DurationMS        int64                `json:"durationMs"`
	RowCount          int                  `json:"rowCount"`
	ResultFingerprint string               `json:"resultFingerprint"`
	SnapshotRef       string               `json:"snapshotRef,omitempty"`
	SnapshotState     *SnapshotState       `json:"snapshotState,omitempty"`
	Incident          *IncidentRef         `json:"incident,omitempty"`
}

func (b ExecutionRecordBrief) Validate() error {
	if err := b.Ref.Validate(); err != nil {
		return &ValidationError{Field: "ref", Message: err.Error()}
	}
	if err := b.Scope.Validate(); err != nil {
		return &ValidationError{Field: "scope", Message: err.Error()}
	}
	if b.Ref.ProjectID != b.Scope.Project {
		return &ValidationError{Field: "ref", Message: "projectId must match scope project"}
	}
	if err := validateExecutionQueryIdentity(b.QueryID, b.QueryRevision, b.DTQLHash); err != nil {
		return err
	}
	executedAt, err := validateExecutionTime("executedAt", b.ExecutedAt)
	if err != nil {
		return err
	}
	if b.DurationMS < 0 || b.RowCount < 0 {
		return &ValidationError{Field: "durationMs/rowCount", Message: "must not be negative"}
	}
	if err := validateSHA256("resultFingerprint", b.ResultFingerprint); err != nil {
		return err
	}
	if (b.SnapshotRef == "") != (b.SnapshotState == nil) {
		return &ValidationError{Field: "snapshotRef/snapshotState", Message: "must be present together"}
	}
	if b.SnapshotRef != "" {
		if strings.TrimSpace(b.SnapshotRef) != b.SnapshotRef {
			return &ValidationError{Field: "snapshotRef", Message: "must be canonical"}
		}
		if err := b.SnapshotState.Validate(); err != nil {
			return &ValidationError{Field: "snapshotState", Message: err.Error()}
		}
		changedAt, _ := time.Parse(time.RFC3339, b.SnapshotState.ChangedAt)
		if changedAt.Before(executedAt) || (b.SnapshotState.Availability != SnapshotAvailable && !changedAt.After(executedAt)) {
			return &ValidationError{Field: "snapshotState.changedAt", Message: "available state must not predate execution and terminal state must be after execution"}
		}
	}
	if b.Incident != nil {
		if err := b.Incident.Validate(); err != nil {
			return &ValidationError{Field: "incident", Message: err.Error()}
		}
	}
	return nil
}

type ExecutionListRequest struct {
	Scope
	QueryID  string       `json:"queryId,omitempty"`
	Incident *IncidentRef `json:"incident,omitempty"`
	Since    string       `json:"since,omitempty"`
	Until    string       `json:"until,omitempty"`
	Limit    *int         `json:"limit,omitempty"`
}

func (r ExecutionListRequest) Validate() error {
	if err := r.Scope.Validate(); err != nil {
		return err
	}
	if r.QueryID != "" {
		if err := requireCanonicalString("queryId", r.QueryID); err != nil {
			return err
		}
	}
	if r.Incident != nil {
		if err := r.Incident.Validate(); err != nil {
			return &ValidationError{Field: "incident", Message: err.Error()}
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
	Execution     ExecutionRef  `json:"execution"`
	SnapshotRef   string        `json:"snapshotRef"`
	SnapshotState SnapshotState `json:"snapshotState"`
	Recordset     *Recordset    `json:"recordset,omitempty"`
	Limitations   []Limitation  `json:"limitations,omitempty"`
	Truncated     bool          `json:"truncated,omitempty"`
}

func (r SnapshotReadResponse) Validate() error {
	if err := r.Execution.Validate(); err != nil {
		return &ValidationError{Field: "execution", Message: err.Error()}
	}
	if err := requireCanonicalString("snapshotRef", r.SnapshotRef); err != nil {
		return err
	}
	if err := r.SnapshotState.Validate(); err != nil {
		return &ValidationError{Field: "snapshotState", Message: err.Error()}
	}
	if r.SnapshotState.Availability != SnapshotAvailable {
		if r.Recordset != nil || len(r.Limitations) != 0 || r.Truncated {
			return &ValidationError{Field: "snapshotState", Message: "unavailable evidence must not include recordset data"}
		}
		return nil
	}
	if r.Recordset == nil {
		return &ValidationError{Field: "recordset", Message: "is required while snapshot evidence is available"}
	}
	if err := r.Recordset.Validate(); err != nil {
		return &ValidationError{Field: "recordset", Message: err.Error()}
	}
	for i, limitation := range r.Limitations {
		if err := limitation.Validate(); err != nil {
			return &ValidationError{Field: "limitations", Message: fmt.Sprintf("index %d: %s", i, err)}
		}
	}
	return nil
}

// ExecutionSeriesPartition contains every immutable identity that must match
// before records may contribute points to one derived series.
type ExecutionSeriesPartition struct {
	EvidenceStoreID   string                `json:"evidenceStoreId"`
	SourceScope       ExecutionRecordScope  `json:"sourceScope"`
	Source            string                `json:"source"`
	PolicyFingerprint string                `json:"policyFingerprint"`
	QueryID           string                `json:"queryId,omitempty"`
	QueryRevision     string                `json:"queryRevision,omitempty"`
	DTQLHash          string                `json:"dtqlHash,omitempty"`
	BindingsApplied   []Binding             `json:"bindingsApplied"`
	Projection        MeasurementProjection `json:"projection"`
}

func (p ExecutionSeriesPartition) Validate() error {
	if err := validateExecutionID("evidenceStoreId", p.EvidenceStoreID); err != nil {
		return err
	}
	if err := p.SourceScope.Validate(); err != nil {
		return &ValidationError{Field: "sourceScope", Message: err.Error()}
	}
	if err := requireCanonicalString("source", p.Source); err != nil {
		return err
	}
	if err := validateSHA256("policyFingerprint", p.PolicyFingerprint); err != nil {
		return err
	}
	if err := validateExecutionQueryIdentity(p.QueryID, p.QueryRevision, p.DTQLHash); err != nil {
		return err
	}
	if p.QueryID != "" && p.QueryRevision == "" {
		return &ValidationError{Field: "queryRevision", Message: "is required to partition a saved-query series"}
	}
	if err := validateBindings("bindingsApplied", p.BindingsApplied); err != nil {
		return err
	}
	if err := p.Projection.Validate(); err != nil {
		return &ValidationError{Field: "projection", Message: err.Error()}
	}
	return nil
}

type ExecutionSeriesRequest struct {
	Scope
	Partition ExecutionSeriesPartition `json:"partition"`
}

func (r ExecutionSeriesRequest) Validate() error {
	if err := r.Scope.Validate(); err != nil {
		return err
	}
	if err := r.Partition.Validate(); err != nil {
		return &ValidationError{Field: "partition", Message: err.Error()}
	}
	if r.StoreID != r.Partition.EvidenceStoreID || r.Project != r.Partition.SourceScope.Project || r.Environment != r.Partition.SourceScope.Environment {
		return &ValidationError{Field: "partition", Message: "evidence store, project and environment must match request scope"}
	}
	return nil
}

type ExecutionSeriesPoint struct {
	ExecutedAt   string       `json:"executedAt"`
	Execution    ExecutionRef `json:"execution"`
	Completeness string       `json:"completeness"`
	Value        *TypedValue  `json:"value,omitempty"`
	Reason       string       `json:"reason,omitempty"`
}

func (p ExecutionSeriesPoint) Validate() error {
	if _, err := validateExecutionTime("executedAt", p.ExecutedAt); err != nil {
		return err
	}
	if err := p.Execution.Validate(); err != nil {
		return &ValidationError{Field: "execution", Message: err.Error()}
	}
	return validateMeasurementOutcome(p.Completeness, p.Value, p.Reason)
}

type ExecutionSeriesResponse struct {
	Points  []ExecutionSeriesPoint `json:"points"`
	Omitted bool                   `json:"omitted"`
}

func (r ExecutionSeriesResponse) Validate() error {
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
// canonical JSON shape. Validation occurs before encoding. Nil and empty
// arrays normalize identically without changing column or row order.
func FingerprintRecordset(recordset Recordset) (string, error) {
	if err := recordset.Validate(); err != nil {
		return "", err
	}
	columns := recordset.Columns
	if columns == nil {
		columns = []Column{}
	}
	rows := recordset.Rows
	if rows == nil {
		rows = [][]TypedValue{}
	} else if len(recordset.Columns) == 0 {
		rows = make([][]TypedValue, len(recordset.Rows))
		copy(rows, recordset.Rows)
		for i, row := range rows {
			if row == nil {
				rows[i] = []TypedValue{}
			}
		}
	}
	canonical := struct {
		Columns []Column       `json:"columns"`
		Rows    [][]TypedValue `json:"rows"`
	}{Columns: columns, Rows: rows}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("apicontract: fingerprint recordset: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func validateExecutionQueryIdentity(queryID, queryRevision, dtqlHash string) error {
	if (queryID == "") == (dtqlHash == "") {
		return &ValidationError{Field: "queryId/dtqlHash", Message: "exactly one of queryId or dtqlHash is required"}
	}
	if queryID != "" {
		if err := requireCanonicalString("queryId", queryID); err != nil {
			return err
		}
	}
	if queryRevision != "" {
		if queryID == "" {
			return &ValidationError{Field: "queryRevision", Message: "requires queryId"}
		}
		if err := requireCanonicalString("queryRevision", queryRevision); err != nil {
			return err
		}
	}
	if dtqlHash != "" {
		return validateSHA256("dtqlHash", dtqlHash)
	}
	return nil
}

func validateBindings(field string, bindings []Binding) error {
	seen := make(map[string]struct{}, len(bindings))
	for i, binding := range bindings {
		if err := binding.Validate(); err != nil {
			return &ValidationError{Field: field, Message: fmt.Sprintf("index %d: %s", i, err)}
		}
		if _, ok := seen[binding.ParameterID]; ok {
			return &ValidationError{Field: field, Message: fmt.Sprintf("duplicate parameter %q", binding.ParameterID)}
		}
		seen[binding.ParameterID] = struct{}{}
	}
	return nil
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

func requireCanonicalString(field, value string) error {
	if err := requireNonEmpty(field, value); err != nil {
		return err
	}
	if strings.TrimSpace(value) != value {
		return &ValidationError{Field: field, Message: "must be canonical"}
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
