package detectiveai

import (
	"context"
	"strconv"
	"time"

	"github.com/datatug/datatug-core/pkg/detective"
)

// PrepareContextCaseEdit turns a human-reviewed extraction into a separate
// private Case edit proposal. It does not mutate the Case. Its confirmed
// dispatch must merge only the reviewed context through the Case endpoint,
// preserve server-owned fields, and write its audit/receipt atomically.
func PrepareContextCaseEdit(ctx context.Context, scope Scope, request ContextReviewRequest, ports ConfirmationPorts, now time.Time, makeID func() (string, error)) (Proposal, error) {
	return prepareContextCaseEdit(ctx, scope, request, ports, now, makeID, proposalDigest)
}

func prepareContextCaseEdit(ctx context.Context, scope Scope, request ContextReviewRequest, ports ConfirmationPorts, now time.Time, makeID func() (string, error), digest func(Proposal) (string, error)) (Proposal, error) {
	if ports == nil || !validScope(scope) || !detective.ValidSlug(request.ProposalID) || len(request.Digest) != 64 || !detective.ValidSlug(request.CaseID) || request.ExpectedVersion < 1 || !validReviewedContext(request.Context) || now.IsZero() {
		return Proposal{}, ErrInvalid
	}
	original, err := ports.Get(ctx, request.ProposalID)
	if err != nil {
		return Proposal{}, err
	}
	if original.Scope != scope || original.ID != request.ProposalID {
		return Proposal{}, ErrUnauthorized
	}
	original = cloneProposal(original)
	if validateProposal(original, now) != nil || original.Digest != request.Digest || original.Draft.Kind != KindContext || len(original.Draft.Context) == 0 {
		return Proposal{}, ErrInvalid
	}
	if err := ports.Authorize(ctx, scope, original); err != nil {
		return Proposal{}, ErrUnauthorized
	}
	if err := ports.AuthorizeSources(ctx, scope, original.Sources); err != nil {
		return Proposal{}, ErrUnauthorized
	}
	current, err := ports.Current(ctx, scope)
	if err != nil {
		return Proposal{}, err
	}
	caseResource, found := resource(current.Cases, request.CaseID)
	if !validSnapshot(current) || !sameSnapshot(current, original.Basis) || !found || caseResource.Revision != strconv.FormatInt(request.ExpectedVersion, 10) {
		return Proposal{}, ErrStale
	}
	if makeID == nil {
		makeID = newID
	}
	id, err := makeID()
	if err != nil || !detective.ValidSlug(id) || id == original.ID {
		return Proposal{}, ErrInvalid
	}
	draft := Draft{
		Kind: KindCase, Title: "Apply reviewed context to Case " + request.CaseID,
		Case:        &CaseEdit{CaseID: request.CaseID, ExpectedVersion: request.ExpectedVersion, Context: cloneInputs(request.Context)},
		Sources:     append([]Provenance(nil), original.Draft.Sources...),
		Ambiguities: append([]string(nil), original.Draft.Ambiguities...),
	}
	derived := cloneProposal(Proposal{
		ID: id, ParentProposalID: original.ID, Scope: scope,
		Basis: current, Draft: draft, Sources: original.Sources,
		CreatedAt: now.UTC(), ExpiresAt: now.UTC().Add(15 * time.Minute),
	})
	derived.Digest, err = digest(derived)
	if err != nil {
		return Proposal{}, err
	}
	if err := ports.Authorize(ctx, scope, derived); err != nil {
		return Proposal{}, ErrUnauthorized
	}
	if err := ports.Save(ctx, cloneProposal(derived)); err != nil {
		return Proposal{}, err
	}
	return derived, nil
}

func validReviewedContext(values map[string]detective.InputValue) bool {
	if len(values) == 0 || len(values) > 64 {
		return false
	}
	for name, value := range values {
		if !detective.ValidParameterName(name) || value.Validate() != nil {
			return false
		}
	}
	return true
}
