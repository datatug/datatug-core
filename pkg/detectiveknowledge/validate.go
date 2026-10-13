package detectiveknowledge

import (
	"strings"

	"github.com/datatug/datatug-core/pkg/detective"
)

// ValidateActionShape checks only portable payload shape. An adapter must
// authorize the server-issued scope, operation ID, evidence and target access.
func ValidateActionShape(a Action) error {
	if a.IfMatch < 0 || len(a.ReviewNote) > 2000 || strings.ContainsRune(a.ReviewNote, 0) {
		return ErrInvalid
	}
	switch a.Type {
	case "save-draft":
		if a.Proposal == nil || a.ProposalID != "" || a.Target != nil || a.ReviewNote != "" {
			return ErrInvalid
		}
		return ValidateDraft(*a.Proposal)
	case "review", "reject":
		if a.Proposal != nil || !detective.ValidSlug(a.ProposalID) || a.Target != nil {
			return ErrInvalid
		}
	case "export":
		if a.Proposal != nil || !detective.ValidSlug(a.ProposalID) || a.ReviewNote != "" {
			return ErrInvalid
		}
		return ValidateTarget(a.Target)
	default:
		return ErrInvalid
	}
	return nil
}

// ValidateDraft rejects client-supplied lifecycle and handoff fields. These
// are issued solely by the transactional adapter through pure transitions.
func ValidateDraft(p Proposal) error {
	if !detective.ValidSlug(p.ID) || !detective.ValidSlug(p.CaseID) || !oneOf(p.Kind, "field-meaning", "relationship", "invariant", "query", "scenario") || strings.TrimSpace(p.Title) == "" || len(p.Title) > 200 || strings.TrimSpace(p.Statement) == "" || len(p.Statement) > 4000 || strings.ContainsRune(p.Title, 0) || strings.ContainsRune(p.Statement, 0) || !oneOf(p.ClaimStatus, "", "hypothesis", "confirmed") || len(p.EvidenceRefs) == 0 || len(p.EvidenceRefs) > 20 || len(p.FindingRefs) > 20 {
		return ErrInvalid
	}
	if p.Status != "" || p.Version != 0 || !p.CreatedAt.IsZero() || p.CreatedBy != "" || !p.ReviewedAt.IsZero() || p.ReviewedBy != "" || p.ReviewNote != "" || len(p.Handoffs) != 0 {
		return ErrInvalid
	}
	return validateRefs(p.EvidenceRefs, p.FindingRefs)
}

func ValidateTarget(t *Target) error {
	if t == nil || !oneOf(t.System, "meaninggraph", "modelspec") {
		return ErrInvalid
	}
	parts := strings.Split(t.Repository, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || parts[0] == "." || parts[1] == "." || parts[0] == ".." || parts[1] == ".." || strings.ContainsAny(t.Repository, " \\\t") {
		return ErrInvalid
	}
	for _, value := range []string{t.Repository, t.ResourceID, t.ExpectedRevision} {
		if value == "" || len(value) > 512 || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\x00\r\n") {
			return ErrInvalid
		}
	}
	return nil
}

// ValidateReferences establishes that each proposal reference belongs to its
// Case. A confirmed claim also needs confirmed Findings supporting every
// selected evidence ID. The adapter must construct facts from an authorized
// snapshot, including verified durable Case-run links for old run evidence.
func ValidateReferences(p Proposal, facts EvidenceFacts) error {
	if !detective.ValidSlug(p.CaseID) || p.CaseID != facts.CaseID || validateRefs(p.EvidenceRefs, p.FindingRefs) != nil || !oneOf(p.ClaimStatus, "hypothesis", "confirmed") {
		return ErrInvalid
	}
	evidence := make(map[string]bool, len(facts.EvidenceIDs))
	for _, id := range facts.EvidenceIDs {
		if !detective.ValidSlug(id) || evidence[id] {
			return ErrInvalid
		}
		evidence[id] = true
	}
	for _, id := range p.EvidenceRefs {
		if !evidence[id] {
			return ErrInvalid
		}
	}
	findings := make(map[string]FindingFact, len(facts.Findings))
	for _, f := range facts.Findings {
		if !detective.ValidSlug(f.ID) || findings[f.ID].ID != "" || !oneOf(f.Status, "hypothesis", "confirmed") {
			return ErrInvalid
		}
		findings[f.ID] = f
	}
	if p.ClaimStatus == "confirmed" && len(p.FindingRefs) == 0 {
		return ErrInvalid
	}
	confirmed := map[string]bool{}
	for _, id := range p.FindingRefs {
		f, ok := findings[id]
		if !ok || p.ClaimStatus == "confirmed" && f.Status != "confirmed" {
			return ErrInvalid
		}
		if f.Status == "confirmed" {
			for _, ref := range f.EvidenceRefs {
				if !evidence[ref] {
					return ErrInvalid
				}
				confirmed[ref] = true
			}
		}
	}
	if p.ClaimStatus == "confirmed" {
		for _, id := range p.EvidenceRefs {
			if !confirmed[id] {
				return ErrInvalid
			}
		}
	}
	return nil
}

func validateRefs(evidenceRefs, findingRefs []string) error {
	if len(evidenceRefs) == 0 || len(evidenceRefs) > 20 || len(findingRefs) > 20 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, id := range evidenceRefs {
		if !detective.ValidSlug(id) || seen[id] {
			return ErrInvalid
		}
		seen[id] = true
	}
	clear(seen)
	for _, id := range findingRefs {
		if !detective.ValidSlug(id) || seen[id] {
			return ErrInvalid
		}
		seen[id] = true
	}
	return nil
}

func oneOf(value string, allowed ...string) bool {
	for _, item := range allowed {
		if value == item {
			return true
		}
	}
	return false
}
