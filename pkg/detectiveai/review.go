package detectiveai

import (
	"context"
	"reflect"
	"strings"
	"time"

	"github.com/datatug/datatug-core/pkg/detective"
)

// PrepareReviewedDraft binds a human-edited query, Check, Validator, Scenario,
// Case or run draft to a fresh private proposal. It inherits the originally
// validated citations, rechecks current authority and revisions, and leaves
// all mutation to a later explicit Confirm. Extraction-to-Case uses the
// specialized PrepareContextCaseEdit path.
func PrepareReviewedDraft(ctx context.Context, scope Scope, request ReviewedDraftRequest, ports ConfirmationPorts, now time.Time, makeID func() (string, error)) (Proposal, error) {
	return prepareReviewedDraft(ctx, scope, request, ports, now, makeID, proposalDigest)
}

func prepareReviewedDraft(ctx context.Context, scope Scope, request ReviewedDraftRequest, ports ConfirmationPorts, now time.Time, makeID func() (string, error), digest func(Proposal) (string, error)) (Proposal, error) {
	if ports == nil || !validScope(scope) || !detective.ValidSlug(request.ProposalID) || len(request.Digest) != 64 || now.IsZero() {
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
	if validateProposal(original, now) != nil || original.Digest != request.Digest {
		return Proposal{}, ErrInvalid
	}
	if !oneOf(original.Draft.Kind, KindQuery, KindCheck, KindValidator, KindScenario, KindCase, KindRun) || request.Draft.Kind != original.Draft.Kind {
		return Proposal{}, ErrUnsupported
	}
	if !reflect.DeepEqual(request.Draft.Sources, original.Draft.Sources) {
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
	if !validSnapshot(current) || !sameSnapshot(current, original.Basis) {
		return Proposal{}, ErrStale
	}
	// The original proposal's citations were checked against Source.Text
	// before storage. Rebuild only their verified spans so the same validator
	// can check the edited payload without persisting or requesting full text.
	sources := citationSources(original.Sources, original.Draft.Sources)
	if err := validateDraftWithExpectations(request.Draft, current, sources, original.ExpectedCounts); err != nil {
		return Proposal{}, err
	}
	if makeID == nil {
		makeID = newID
	}
	id, err := makeID()
	if err != nil || !detective.ValidSlug(id) || id == original.ID {
		return Proposal{}, ErrInvalid
	}
	derived := cloneProposal(Proposal{
		ID: id, ParentProposalID: original.ID, Scope: scope,
		Basis: current, Draft: request.Draft, Sources: original.Sources,
		ExpectedCounts: original.ExpectedCounts,
		CreatedAt:      now.UTC(), ExpiresAt: now.UTC().Add(15 * time.Minute),
	})
	if derived.Digest, err = digest(derived); err != nil {
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

func citationSources(evidence []SourceEvidence, citations []Provenance) []Source {
	sources := make([]Source, len(evidence))
	for i, item := range evidence {
		text := []byte(strings.Repeat(" ", item.Length))
		for _, citation := range citations {
			if citation.SourceID == item.ID && citation.Quote != "" && citation.Start >= 0 && citation.End <= len(text) && citation.End-citation.Start == len(citation.Quote) {
				copy(text[citation.Start:citation.End], citation.Quote)
			}
		}
		sources[i] = Source{ID: item.ID, Kind: item.Kind, Revision: item.Revision, Text: string(text)}
	}
	return sources
}
