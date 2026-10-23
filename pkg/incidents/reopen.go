package incidents

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// ReopenedPayload is an explicit lifecycle operation, never a backwards generic
// status update. Stores must additionally require Mutation.ExpectedSeq and use
// the authenticated actor and server wall time. The expected epoch rejects a
// stale reopen even if a caller accidentally reuses a previous payload.
type ReopenedPayload struct {
	Reason                  string `json:"reason"`
	ExpectedResolutionEpoch uint64 `json:"expectedResolutionEpoch"`
}

func (p ReopenedPayload) Validate() error {
	if strings.TrimSpace(p.Reason) == "" || len(p.Reason) > 4096 || p.ExpectedResolutionEpoch == math.MaxUint64 {
		return fmt.Errorf("reopen requires a reason and an incrementable resolution epoch")
	}
	return nil
}
func (p *Incident) applyReopenEvent(event Event) error {
	payload := validatedPayload[ReopenedPayload](event.Payload)
	if p.Status != StatusResolved && p.Status != StatusWatching && p.Status != StatusClosed {
		return fmt.Errorf("only a resolved, watching or closed incident can reopen")
	}
	if payload.ExpectedResolutionEpoch != p.ResolutionEpoch {
		return fmt.Errorf("reopen resolution epoch conflict")
	}
	p.ResolutionEpoch++
	// VisibleAt is the store's commit/visibility clock, so backdated occurrence
	// timestamps cannot admit proof produced before this reopening was committed.
	at := event.VisibleAt.UTC()
	p.ReopenedAt = &at
	p.ReopenEventID = event.ID
	p.Status = StatusInvestigating
	p.Outcome = ""
	p.Resolution = nil
	p.ResolutionEventID = ""
	p.NoteEntries = append(p.NoteEntries, Note{EventID: event.ID, Body: payload.Reason})
	p.Notes = append(p.Notes, payload.Reason)
	return nil
}
func (i Incident) validateResolutionEpoch() error {
	if i.ResolutionEpoch == 0 {
		if i.ReopenedAt != nil {
			return fmt.Errorf("legacy resolution epoch cannot have reopening time")
		}
		return nil
	}
	if i.ReopenedAt == nil || i.ReopenedAt.IsZero() {
		return fmt.Errorf("resolution epoch requires reopening time")
	}
	_, offset := i.ReopenedAt.Zone()
	if offset != 0 {
		return fmt.Errorf("reopening time must be UTC")
	}
	return nil
}

// ValidateIncidentResolutionProof adds the incident's current lifecycle epoch to
// the existing immutable criterion/scope/definition/freshness validation. Hosts
// must derive ResolutionEpoch and EvaluatedAt from current server-owned state;
// changing only the epoch on a previously produced run cannot make it current.
func ValidateIncidentResolutionProof(i Incident, p ResolutionProof, at time.Time) error {
	if err := i.validateResolutionEpoch(); err != nil {
		return err
	}
	if i.ResolutionCriterion == nil {
		return fmt.Errorf("verified resolution requires a criterion")
	}
	if p.ResolutionEpoch != i.ResolutionEpoch {
		return fmt.Errorf("resolution proof belongs to a different reopening epoch")
	}
	if i.ReopenedAt != nil && !p.EvaluatedAt.After(*i.ReopenedAt) {
		return fmt.Errorf("resolution run must be evaluated after reopening")
	}
	return ValidateResolutionProof(*i.ResolutionCriterion, p, at)
}
func cloneReopenedAt(at *time.Time) *time.Time {
	if at == nil {
		return nil
	}
	value := *at
	return &value
}

// CanTransitionIncidentStatus is the version-one Work lifecycle for ordinary
// status events. Terminal reopening is deliberately absent: it needs an explicit
// incident.reopened event to fence prior evidence. Legacy version-zero status
// events retain their historic forward-only folding rules.
func CanTransitionIncidentStatus(from, to Status) bool {
	switch from {
	case StatusOpen:
		return to == StatusInvestigating || to == StatusMitigating
	case StatusInvestigating:
		return to == StatusMitigating || to == StatusRecovering
	case StatusMitigating:
		return to == StatusInvestigating || to == StatusRecovering
	case StatusRecovering:
		return to == StatusInvestigating || to == StatusResolved
	case StatusResolved:
		return to == StatusWatching || to == StatusClosed
	case StatusWatching:
		return to == StatusClosed
	default:
		return false
	}
}
