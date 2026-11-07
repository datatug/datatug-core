package reconcile

import (
	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/recordsetcompare"
)

// Instance identifies one compared database instance along independent
// dimensions. Tenant and environment are labels, not a hierarchy: one tenant
// may have several instances, and one instance may hold several tenants
// (TenantScope then names the slice compared, for example a WHERE predicate
// already applied by the caller).
type Instance struct {
	ID          string            `json:"id"`
	Tenant      string            `json:"tenant,omitempty"`
	Environment string            `json:"environment,omitempty"`
	Family      string            `json:"family,omitempty"`
	TenantScope string            `json:"tenantScope,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// UnavailableReason says why a source contributed no rows.
type UnavailableReason string

const (
	UnavailablePermissionDenied UnavailableReason = "permission_denied"
	UnavailableExecutionFailed  UnavailableReason = "execution_failed"
	UnavailableNotExamined      UnavailableReason = "not_examined"
)

// Unavailable marks a source that could not be read. An unavailable source is
// never treated as matching: its instance is reported inconclusive.
type Unavailable struct {
	Reason  UnavailableReason `json:"reason"`
	Message string            `json:"message,omitempty"`
}

// Source is one instance's post-policy recordset. Rows must be strictly
// ordered by Rules.Key using apicontract.TypedKeySortKey (see
// recordsetcompare.OrderRecordset for callers holding a materialized recordset). Info.Limitations carries the
// access policy already applied by the caller: hidden columns are never
// compared, and filtered rows make absences inconclusive.
type Source struct {
	Instance    Instance
	Columns     []apicontract.Column
	Rows        recordsetcompare.OrderedRows
	Info        apicontract.CompareSideInfo
	Unavailable *Unavailable
	// ReadAt is when the source was read (RFC 3339), reported back verbatim.
	ReadAt string
	// Snapshot is true when every row came from one consistent snapshot of
	// this instance. Cross-instance reads are never one snapshot.
	Snapshot bool
}

// Mode selects how authority is assigned.
type Mode string

const (
	// ModeBaseline compares every instance to one explicitly named
	// authoritative instance. Deviations are attributed to the candidate.
	ModeBaseline Mode = "baseline"
	// ModeConsensus groups equal values and reports minority groups as
	// outliers. Consensus never declares the majority correct.
	ModeConsensus Mode = "consensus"
)

// FieldPolicy states the consistency expectation for one field.
type FieldPolicy string

const (
	// FieldIdentical requires byte-for-byte equal typed values.
	FieldIdentical FieldPolicy = "identical"
	// FieldNormalized compares strings after trimming, collapsing inner
	// whitespace and case-folding; other types compare as identical.
	FieldNormalized FieldPolicy = "normalized"
	// FieldInstanceSpecific expects values to differ: differences are
	// reported as expected, never as violations.
	FieldInstanceSpecific FieldPolicy = "instance-specific"
	// FieldIgnored excludes the field from comparison and from results.
	FieldIgnored FieldPolicy = "ignored"
)

// Presence states which instances must hold each record.
type Presence string

const (
	// PresenceRequired: every instance holds every record.
	PresenceRequired Presence = "required"
	// PresenceSubset (baseline mode): candidates may lack baseline records but
	// must not hold records the baseline lacks.
	PresenceSubset Presence = "subset"
	// PresenceSuperset (baseline mode): candidates must hold every baseline
	// record and may hold more.
	PresenceSuperset Presence = "superset"
	// PresenceAny: presence differences are expected.
	PresenceAny Presence = "any"
)

// ValueDisclosure controls whether observed values appear in results.
type ValueDisclosure string

const (
	// DiscloseValues includes post-policy visible values in value groups.
	DiscloseValues ValueDisclosure = "values"
	// DiscloseGroupsOnly reports only which instances agree, never values or
	// hashes of values. Use it when results leave the trust boundary (for
	// example to an AI service).
	DiscloseGroupsOnly ValueDisclosure = "groups-only"
)

// Rules is the reconciliation policy for one dataset.
type Rules struct {
	// Key is the semantic identity: column names whose values identify the
	// same logical record across instances. Never assume surrogate primary
	// keys match across databases.
	Key          []string               `json:"key"`
	Mode         Mode                   `json:"mode"`
	Baseline     string                 `json:"baseline,omitempty"`
	Presence     Presence               `json:"presence,omitempty"`
	DefaultField FieldPolicy            `json:"defaultField,omitempty"`
	Fields       map[string]FieldPolicy `json:"fields,omitempty"`
	Disclosure   ValueDisclosure        `json:"disclosure,omitempty"`
	// Limit bounds each detail list (unexpected, expected, inconclusive).
	// Counts in Summary are always complete.
	Limit int `json:"limit,omitempty"`
}

// Status is a reconciliation verdict.
type Status string

const (
	StatusConsistent    Status = "consistent"
	StatusDiscrepancies Status = "discrepancies"
	// StatusPartial: no unexpected differences in what was visible, but
	// policy hid rows or columns, so the result is not a full proof.
	StatusPartial Status = "partial"
	// StatusInconclusive: the instance (or run) could not be compared.
	StatusInconclusive Status = "inconclusive"
)

// Result is the structured outcome of one dataset reconciliation.
type Result struct {
	Mode       Mode             `json:"mode"`
	Baseline   string           `json:"baseline,omitempty"`
	Key        []string         `json:"key"`
	Presence   Presence         `json:"presence"`
	Disclosure ValueDisclosure  `json:"disclosure"`
	Status     Status           `json:"status"`
	Instances  []InstanceResult `json:"instances"`
	Columns    []ColumnStatus   `json:"columns"`
	Summary    Summary          `json:"summary"`
	// Unexpected, Expected and Inconclusive hold the first Limit records of
	// each class in key order.
	Unexpected   []RecordDiscrepancy `json:"unexpected"`
	Expected     []RecordDiscrepancy `json:"expected"`
	Inconclusive []RecordDiscrepancy `json:"inconclusive"`
	Truncated    bool                `json:"truncated"`
	Caveats      []string            `json:"caveats"`
}

// InstanceResult is one instance's share of the verdict.
type InstanceResult struct {
	Instance      Instance     `json:"instance"`
	Status        Status       `json:"status"`
	Authoritative bool         `json:"authoritative,omitempty"`
	Unavailable   *Unavailable `json:"unavailable,omitempty"`
	ReadAt        string       `json:"readAt,omitempty"`
	RowCount      int          `json:"rowCount"`
	RowsFiltered  bool         `json:"rowsFiltered,omitempty"`
	HiddenColumns []string     `json:"hiddenColumns,omitempty"`
	// Counts are per (record, instance) attributions.
	Missing      int `json:"missing"`
	Extra        int `json:"extra"`
	Conflicts    int `json:"conflicts"`
	Expected     int `json:"expected"`
	Inconclusive int `json:"inconclusive"`
}

// ColumnStatus reports schema drift for one non-key column: instances that
// lack it, hide it by policy, or declare a different type. Columns that are
// present with one type everywhere are not listed.
type ColumnStatus struct {
	Column    string            `json:"column"`
	Policy    FieldPolicy       `json:"policy"`
	MissingIn []string          `json:"missingIn,omitempty"`
	HiddenIn  []string          `json:"hiddenIn,omitempty"`
	Types     map[string]string `json:"types,omitempty"`
	// Compared is false when a type conflict excluded the column entirely.
	Compared bool `json:"compared"`
}

// Summary carries complete counts regardless of Limit.
type Summary struct {
	Instances           int `json:"instances"`
	ComparedInstances   int `json:"comparedInstances"`
	Records             int `json:"records"`
	Consistent          int `json:"consistent"`
	WithUnexpected      int `json:"withUnexpected"`
	WithExpectedOnly    int `json:"withExpectedOnly"`
	WithInconclusive    int `json:"withInconclusive"`
	MissingRecords      int `json:"missingRecords"`
	ExtraRecords        int `json:"extraRecords"`
	ConflictingRecords  int `json:"conflictingRecords"`
	ExpectedDifferences int `json:"expectedDifferences"`
}

// RecordDiscrepancy is one logical record that is not consistent everywhere.
type RecordDiscrepancy struct {
	Key []apicontract.TypedValue `json:"key"`
	// Present lists instances holding the record.
	Present []string `json:"present"`
	// Missing lists instances lacking the record; MissingExpected says
	// whether the presence rule allows it.
	Missing         []string `json:"missing,omitempty"`
	MissingExpected bool     `json:"missingExpected,omitempty"`
	// Extra lists instances holding a record the baseline lacks.
	Extra         []string `json:"extra,omitempty"`
	ExtraExpected bool     `json:"extraExpected,omitempty"`
	// Inconclusive lists instances whose absence cannot be judged because
	// policy filtered their rows.
	Inconclusive []string          `json:"inconclusive,omitempty"`
	Fields       []FieldDifference `json:"fields,omitempty"`
}

// FieldDifference is one field whose values disagree across instances.
type FieldDifference struct {
	Column   string       `json:"column"`
	Policy   FieldPolicy  `json:"policy"`
	Expected bool         `json:"expected"`
	Groups   []ValueGroup `json:"groups"`
	// Deviating lists instances attributed with the difference: in baseline
	// mode those outside the baseline's group; in consensus mode those
	// outside the unique largest group. Empty with NoConsensus when the
	// largest groups tie.
	Deviating   []string `json:"deviating,omitempty"`
	NoConsensus bool     `json:"noConsensus,omitempty"`
}

// ValueGroup is a set of instances holding the same (normalized) value.
type ValueGroup struct {
	Value         *apicontract.TypedValue `json:"value,omitempty"`
	Instances     []string                `json:"instances"`
	Authoritative bool                    `json:"authoritative,omitempty"`
}
