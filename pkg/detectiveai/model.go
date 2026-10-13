// Package detectiveai contains provider-neutral, reviewable Detective AI
// proposals. Model output is never an authority to read, write, or execute.
package detectiveai

import (
	"context"
	"errors"
	"time"

	"github.com/datatug/datatug-core/pkg/detective"
)

var (
	ErrInvalid      = errors.New("invalid detective AI proposal")
	ErrStale        = errors.New("detective AI proposal is stale")
	ErrUnauthorized = errors.New("detective AI action is unauthorized")
	ErrUnsupported  = errors.New("unsupported detective AI action")
)

const (
	KindContext   = "extract-context"
	KindQuery     = "draft-query"
	KindCheck     = "draft-check"
	KindValidator = "draft-validator"
	KindScenario  = "draft-scenario"
	KindCase      = "edit-case"
	KindRun       = "run-check"
	KindExplain   = "explain-failure"
)

// Scope is supplied only by an authenticated host adapter. The model never
// receives an actor credential, and request JSON cannot override this scope.
type Scope struct {
	Key     string `json:"key"` // normalized store/Space/project/branch identity
	Project string `json:"project"`
	Actor   string `json:"actor"`
}

// Resource is a bounded, trusted catalog summary. Revision is the immutable
// query revision or the decimal version of a Detective resource.
type Resource struct {
	ID          string            `json:"id"`
	Title       string            `json:"title"`
	Revision    string            `json:"revision"`
	Inputs      []detective.Input `json:"inputs,omitempty"`
	Environment string            `json:"environment,omitempty"`
}

// Snapshot comes from the host's authorized project reads, never model text.
// A proposal binds to CatalogRevision and the referenced resource revisions.
type Snapshot struct {
	CatalogRevision string     `json:"catalogRevision"`
	Environments    []string   `json:"environments"`
	Queries         []Resource `json:"queries"`
	Checks          []Resource `json:"checks"`
	Validators      []Resource `json:"validators"`
	Scenarios       []Resource `json:"scenarios"`
	Cases           []Resource `json:"cases"`
	Runs            []Resource `json:"runs"`
}

// Source is a host-issued citation to pasted text or an authorized resource.
// Locator identifies a span or field without copying sensitive source text.
type Source struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Revision string `json:"revision"`
	Text     string `json:"text"` // bounded authorized excerpt; never persisted or logged by Core
}

type SourceEvidence struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Revision string `json:"revision"`
	Digest   string `json:"digest"`
	Length   int    `json:"length"`
}

type Provenance struct {
	SourceID   string  `json:"sourceId"`
	Start      int     `json:"start"` // byte offsets into Source.Text
	End        int     `json:"end"`
	Quote      string  `json:"quote,omitempty"`
	Confidence float64 `json:"confidence"`
	Ambiguous  bool    `json:"ambiguous,omitempty"`
}

type ExtractedValue struct {
	Value      detective.InputValue `json:"value"`
	Provenance []Provenance         `json:"provenance"`
}

// QueryDraft is deliberately inert. The host maps it to its existing saved
// query write contract after confirmation and its own source authorization.
type QueryDraft struct {
	ID               string            `json:"id"`
	Title            string            `json:"title"`
	Type             string            `json:"type"`
	Text             string            `json:"text"`
	ConnectionID     string            `json:"connectionId,omitempty"`
	Inputs           []detective.Input `json:"inputs,omitempty"`
	ExpectedRevision string            `json:"expectedRevision,omitempty"`
}

// CaseEdit contains only user-editable fields; server-owned timeline, run
// links, actor, and version are not present in model-generated payloads.
type CaseEdit struct {
	CaseID          string                          `json:"caseId"`
	ExpectedVersion int64                           `json:"expectedVersion"`
	Title           string                          `json:"title,omitempty"`
	Context         map[string]detective.InputValue `json:"context,omitempty"`
	Findings        []detective.Finding             `json:"findings,omitempty"`
	Status          string                          `json:"status,omitempty"`
	ClosureReason   string                          `json:"closureReason,omitempty"`
}

type RunDraft struct {
	CheckID     string                          `json:"checkId"`
	CaseID      string                          `json:"caseId,omitempty"`
	Environment string                          `json:"environment"`
	Inputs      map[string]detective.InputValue `json:"inputs"`
}

type FailureExplanation struct {
	RunID   string `json:"runId"`
	Summary string `json:"summary"`
	Basis   string `json:"basis"`
}

// Draft is the strict, one-of model tool argument. Every proposal has a
// visible source trail; confidence is attached to each extracted value.
type Draft struct {
	Kind        string                        `json:"kind"`
	Title       string                        `json:"title"`
	Context     map[string]ExtractedValue     `json:"context,omitempty"`
	Query       *QueryDraft                   `json:"query,omitempty"`
	Check       *detective.Check              `json:"check,omitempty"`
	Validator   *detective.PublishedValidator `json:"validator,omitempty"`
	Scenario    *detective.Scenario           `json:"scenario,omitempty"`
	Case        *CaseEdit                     `json:"case,omitempty"`
	Run         *RunDraft                     `json:"run,omitempty"`
	Explanation *FailureExplanation           `json:"explanation,omitempty"`
	Sources     []Provenance                  `json:"sources"`
	Ambiguities []string                      `json:"ambiguities,omitempty"`
}

// Proposal is stored privately with the exact trusted scope and snapshot
// basis used for review. Digest binds its complete immutable payload.
type Proposal struct {
	ID               string           `json:"id"`
	ParentProposalID string           `json:"parentProposalId,omitempty"`
	Scope            Scope            `json:"scope"`
	Basis            Snapshot         `json:"basis"`
	Draft            Draft            `json:"draft"`
	Sources          []SourceEvidence `json:"sources"`
	ExpectedCounts   map[string]int   `json:"expectedCounts,omitempty"`
	CreatedAt        time.Time        `json:"createdAt"`
	ExpiresAt        time.Time        `json:"expiresAt"`
	Digest           string           `json:"digest"`
}

type Request struct {
	Scope          Scope          `json:"scope"`
	Snapshot       Snapshot       `json:"snapshot"`
	Text           string         `json:"text"`
	Sources        []Source       `json:"sources"`
	ExpectedCounts map[string]int `json:"expectedCounts,omitempty"` // explicit user-entered counts, never inferred by the model
}

type Match struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Title    string `json:"title"`
	Revision string `json:"revision"`
}

type Response struct {
	Proposals []Proposal `json:"proposals"`
	Matches   []Match    `json:"matches"`
	Text      string     `json:"text,omitempty"`
}

// ProposalStore must store proposals privately by ID and return immutable
// copies. An adapter enforces project/actor access on both operations.
type ProposalStore interface {
	Save(context.Context, Proposal) error
	Get(context.Context, string) (Proposal, error)
}

// ConfirmationPorts are host-owned deterministic authorization and mutation
// routes. Dispatch must persist its idempotency receipt and audit atomically.
type ConfirmationPorts interface {
	ProposalStore
	Current(context.Context, Scope) (Snapshot, error)
	// Authorize checks current principal/action/resource permissions. It must
	// not treat a version advanced by this same completed operation as lost
	// permission; Core checks freshness for new operations separately.
	Authorize(context.Context, Scope, Proposal) error
	AuthorizeSources(context.Context, Scope, []SourceEvidence) error
	// PriorReceipt returns an immutable, scoped operation receipt, if any.
	// The adapter must reauthorize the principal before revealing it and bind
	// lookup to the same operation scope used by Dispatch's atomic receipt.
	PriorReceipt(context.Context, Scope, string) (Receipt, bool, error)
	Dispatch(context.Context, Command) (Receipt, error)
}

type ConfirmRequest struct {
	ProposalID  string `json:"proposalId"`
	Digest      string `json:"digest"`
	OperationID string `json:"operationId"`
}

// ContextReviewRequest records the human-edited values to apply to an
// existing Case. Core derives a separate, digest-bound Case edit proposal;
// confirming that proposal uses the same audited deterministic Case route.
type ContextReviewRequest struct {
	ProposalID      string                          `json:"proposalId"`
	Digest          string                          `json:"digest"`
	CaseID          string                          `json:"caseId"`
	ExpectedVersion int64                           `json:"expectedVersion"`
	Context         map[string]detective.InputValue `json:"context"`
}

// Command is formed only by Confirm after it rechecks scope, freshness and
// authority. Hosts translate it to their existing saved-query/Detective route.
type Command struct {
	Scope       Scope                      `json:"scope"`
	OperationID string                     `json:"operationId"`
	Proposal    Proposal                   `json:"proposal"`
	Binding     detective.OperationBinding `json:"binding"`
}

type Receipt struct {
	OperationID   string    `json:"operationId"`
	ScopeKey      string    `json:"scopeKey"`
	Project       string    `json:"project"`
	Actor         string    `json:"actor"`
	ProposalID    string    `json:"proposalId"`
	PayloadDigest string    `json:"payloadDigest"`
	ResourceID    string    `json:"resourceId"`
	At            time.Time `json:"at"`
}
