package detective

import (
	"context"
	"time"
)

// Page is a bounded direct-resource listing. Cursors are opaque to Core.
type Page[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"nextCursor,omitempty"`
	HasMore    bool   `json:"hasMore"`
}

// OperationReceipt is an idempotency record committed with each mutation and
// audit event. It carries no parameter values, rows or free-text Case content.
type OperationReceipt struct {
	ID              string    `json:"id"`
	Project         string    `json:"project"`
	Actor           string    `json:"actor"`
	Action          string    `json:"action"`
	PayloadDigest   string    `json:"payloadDigest"` // server-computed canonical request binding
	ResourceID      string    `json:"resourceId,omitempty"`
	ResourceVersion int64     `json:"resourceVersion,omitempty"`
	At              time.Time `json:"at"`
}

// CaseRunLink is the authoritative direct lookup for a Case's execution
// evidence. Case.RunRefs is only a recent projection and may be paged.
type CaseRunLink struct {
	CaseID string    `json:"caseId"`
	RunID  string    `json:"runId"`
	At     time.Time `json:"at"`
	Actor  string    `json:"actor"`
}

// Repository uses direct typed records and bounded pages; a hosted adapter
// never has to fetch or write one whole-project State document. Authorization
// of the project, saved queries and linked evidence precedes these operations.
type Repository interface {
	CatalogReader
	GetCase(context.Context, string) (Case, error)
	GetRun(context.Context, string) (CheckRun, error)
	GetCaseRunLink(context.Context, string, string) (CaseRunLink, error)
	GetScenario(context.Context, string) (Scenario, error)
	ListChecks(context.Context, string, int) (Page[Check], error)
	ListCases(context.Context, string, int) (Page[Case], error)
	ListCaseEvents(context.Context, string, string, int) (Page[CaseEvent], error)
	ListRuns(context.Context, string, int) (Page[CheckRun], error)
	ListCaseRunLinks(context.Context, string, string, int) (Page[CaseRunLink], error)
	ListScenarios(context.Context, string, int) (Page[Scenario], error)
	ListPublishedValidators(context.Context, string, int) (Page[PublishedValidator], error)
	ListQueryValidators(context.Context, string, int) (Page[QueryValidators], error)
	Transact(context.Context, string, func(Transaction) error) error
}

// Transaction writes one operation receipt, its typed resources and its
// metadata-only audit event atomically. Put* methods compare expectedVersion;
// zero means the resource must be absent. The adapter must fail closed on a
// stale version, duplicate ID or an unavailable audit/receipt store. Before
// returning a stored response, GetOperation callers must call
// CheckOperationReplay with a fresh server-side BindOperation. PutOperation
// must reject receipts that fail Validate.
type Transaction interface {
	GetCase(context.Context, string) (Case, error)
	GetRun(context.Context, string) (CheckRun, error)
	GetCaseRunLink(context.Context, string, string) (CaseRunLink, error)
	GetScenario(context.Context, string) (Scenario, error)
	GetOperation(context.Context, string) (OperationReceipt, error)
	PutCheck(context.Context, Check, int64) error
	PutPublishedValidator(context.Context, PublishedValidator, int64) error
	PutQueryValidators(context.Context, QueryValidators, int64) error
	PutScenario(context.Context, Scenario, int64) error
	PutCase(context.Context, Case, int64) error
	PutRun(context.Context, CheckRun) error
	PutCaseRunLink(context.Context, CaseRunLink) error
	AppendCaseEvents(context.Context, string, []CaseEvent) error
	AppendAudit(context.Context, AuditEvent) error
	PutOperation(context.Context, OperationReceipt) error
}
