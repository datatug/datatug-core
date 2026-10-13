package detectiveknowledge

import (
	"strings"
	"time"

	"github.com/datatug/datatug-core/pkg/detective"
)

// SaveDraft returns a new private projection. Its caller must validate Case
// references and commit the projection, audit and receipt atomically.
func SaveDraft(old *Proposal, draft Proposal, ifMatch int64, actor string, at time.Time) (Proposal, error) {
	if ValidateDraft(draft) != nil || ifMatch < 0 || !validActorTime(actor, at) {
		return Proposal{}, ErrInvalid
	}
	if old == nil {
		if ifMatch != 0 {
			return Proposal{}, ErrConflict
		}
		draft.CreatedAt, draft.CreatedBy = at, actor
	} else {
		if old.Status != "draft" || old.Version != ifMatch || old.ID != draft.ID || old.CaseID != draft.CaseID {
			return Proposal{}, ErrConflict
		}
		draft.CreatedAt, draft.CreatedBy = old.CreatedAt, old.CreatedBy
	}
	if draft.ClaimStatus == "" {
		draft.ClaimStatus = "hypothesis"
	}
	draft.Status = "draft"
	draft.Version = ifMatch + 1
	draft.EvidenceRefs = append([]string(nil), draft.EvidenceRefs...)
	draft.FindingRefs = append([]string(nil), draft.FindingRefs...)
	return draft, nil
}

// Review is the only transition to a reviewed or rejected proposal. A
// rejected proposal cannot be exported or silently edited back into a draft.
func Review(old Proposal, ifMatch int64, decision, note, actor string, at time.Time) (Proposal, error) {
	if !oneOf(decision, "review", "reject") || len(note) > 2000 || strings.ContainsRune(note, 0) || !validActorTime(actor, at) {
		return Proposal{}, ErrInvalid
	}
	if old.Status != "draft" || old.Version != ifMatch || ifMatch < 1 {
		return Proposal{}, ErrConflict
	}
	next := cloneProposal(old)
	next.Status = "reviewed"
	if decision == "reject" {
		next.Status = "rejected"
	}
	next.Version++
	next.ReviewedAt, next.ReviewedBy, next.ReviewNote = at, actor, note
	return next, nil
}

// PrepareHandoff records a bounded target-specific review receipt. The
// handoff cites the reviewed proposal version before this append increments it.
func PrepareHandoff(old Proposal, ifMatch int64, target Target, actor, receipt string, at time.Time) (Proposal, Handoff, error) {
	if ValidateTarget(&target) != nil || !detective.ValidSlug(receipt) || !validActorTime(actor, at) {
		return Proposal{}, Handoff{}, ErrInvalid
	}
	if old.Status != "reviewed" || old.Version != ifMatch || ifMatch < 2 {
		return Proposal{}, Handoff{}, ErrConflict
	}
	if len(old.Handoffs) >= MaxHandoffs {
		return Proposal{}, Handoff{}, ErrCapacity
	}
	next := cloneProposal(old)
	handoff := Handoff{
		Kind: "knowledge-handoff", Status: "ready-for-target-review",
		Target: target, ProposalID: old.ID, ProposalVersion: old.Version,
		CaseID: old.CaseID, ClaimStatus: old.ClaimStatus, Statement: old.Statement,
		EvidenceRefs: append([]string(nil), old.EvidenceRefs...),
		FindingRefs:  append([]string(nil), old.FindingRefs...),
		PreparedAt:   at, PreparedBy: actor, Receipt: receipt,
	}
	next.Version++
	next.Handoffs = append(next.Handoffs, handoff)
	return next, cloneHandoff(handoff), nil
}

func validActorTime(actor string, at time.Time) bool {
	return actor != "" && len(actor) <= 256 && !strings.ContainsRune(actor, 0) && !at.IsZero()
}

func cloneProposal(p Proposal) Proposal {
	p.EvidenceRefs = append([]string(nil), p.EvidenceRefs...)
	p.FindingRefs = append([]string(nil), p.FindingRefs...)
	if len(p.Handoffs) > 0 {
		copyHandoffs := make([]Handoff, len(p.Handoffs))
		for i, h := range p.Handoffs {
			copyHandoffs[i] = cloneHandoff(h)
		}
		p.Handoffs = copyHandoffs
	}
	return p
}

func cloneHandoff(h Handoff) Handoff {
	h.EvidenceRefs = append([]string(nil), h.EvidenceRefs...)
	h.FindingRefs = append([]string(nil), h.FindingRefs...)
	return h
}
