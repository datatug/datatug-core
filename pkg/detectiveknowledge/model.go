// Package detectiveknowledge owns the private, reviewable knowledge proposal
// contract shared by local and hosted Detective stores. Preparing a handoff
// never writes to MeaningGraph or ModelSpec.
package detectiveknowledge

import (
	"errors"
	"time"
)

var (
	ErrInvalid  = errors.New("invalid detective knowledge proposal")
	ErrConflict = errors.New("detective knowledge version conflict")
	ErrCapacity = errors.New("detective knowledge handoff capacity reached")
)

const MaxHandoffs = 20

type Proposal struct {
	ID           string    `json:"id"`
	CaseID       string    `json:"caseId"`
	Kind         string    `json:"kind"`
	Title        string    `json:"title"`
	Statement    string    `json:"statement"`
	EvidenceRefs []string  `json:"evidenceRefs"`
	FindingRefs  []string  `json:"findingRefs,omitempty"`
	ClaimStatus  string    `json:"claimStatus"`
	Status       string    `json:"status"`
	Version      int64     `json:"version"`
	CreatedAt    time.Time `json:"createdAt"`
	CreatedBy    string    `json:"createdBy"`
	ReviewedAt   time.Time `json:"reviewedAt,omitempty"`
	ReviewedBy   string    `json:"reviewedBy,omitempty"`
	ReviewNote   string    `json:"reviewNote,omitempty"`
	Handoffs     []Handoff `json:"handoffs,omitempty"`
}

type Target struct {
	System           string `json:"system"`
	Repository       string `json:"repository"`
	ResourceID       string `json:"resourceId"`
	ExpectedRevision string `json:"expectedRevision"`
}

// Handoff is a private, target-specific review artifact, not proof of
// acceptance by the destination registry.
type Handoff struct {
	Kind            string    `json:"kind"`
	Status          string    `json:"status"`
	Target          Target    `json:"target"`
	ProposalID      string    `json:"proposalId"`
	ProposalVersion int64     `json:"proposalVersion"`
	CaseID          string    `json:"caseId"`
	ClaimStatus     string    `json:"claimStatus"`
	Statement       string    `json:"statement"`
	EvidenceRefs    []string  `json:"evidenceRefs"`
	FindingRefs     []string  `json:"findingRefs,omitempty"`
	PreparedAt      time.Time `json:"preparedAt"`
	PreparedBy      string    `json:"preparedBy"`
	Receipt         string    `json:"receipt"`
}

// Action deliberately retains the hosted wire shape; the authenticated
// adapter supplies and checks storage, Space, project, actor and operation.
type Action struct {
	Storage     string    `json:"storage"`
	SpaceID     string    `json:"spaceID"`
	Project     string    `json:"project"`
	OperationID string    `json:"operationId"`
	IfMatch     int64     `json:"ifMatch,omitempty"`
	Type        string    `json:"type"`
	Proposal    *Proposal `json:"proposal,omitempty"`
	ProposalID  string    `json:"proposalId,omitempty"`
	ReviewNote  string    `json:"reviewNote,omitempty"`
	Target      *Target   `json:"target,omitempty"`
}

type Response struct {
	Proposal *Proposal `json:"proposal,omitempty"`
	Export   *Handoff  `json:"export,omitempty"`
	Receipt  string    `json:"receipt"`
}

type Page struct {
	Proposals  []Proposal `json:"proposals"`
	NextCursor string     `json:"nextCursor,omitempty"`
	HasMore    bool       `json:"hasMore"`
}

// EvidenceFacts is an authorized adapter projection of Case evidence. Older
// run evidence may be included only after the adapter verifies the completed
// run and its durable Case-run link inside the same transaction.
type EvidenceFacts struct {
	CaseID      string
	EvidenceIDs []string
	Findings    []FindingFact
}

type FindingFact struct {
	ID           string
	Status       string
	EvidenceRefs []string
}
