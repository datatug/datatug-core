package detectiveai

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"regexp"
	"strconv"
	"time"

	"github.com/datatug/datatug-core/pkg/detective"
)

var operationIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// Confirm accepts only a server-derived scope. It reloads the private stored
// proposal, verifies its digest and current revision basis, reauthorizes the
// principal, then delegates the mutation to an audited deterministic route.
// Dispatch must bind OperationID to Command.Binding and return the same
// receipt for an authorized exact retry; conflicting retries fail closed.
func Confirm(ctx context.Context, scope Scope, request ConfirmRequest, ports ConfirmationPorts, now time.Time) (Receipt, error) {
	if ports == nil || !validScope(scope) || !detective.ValidSlug(request.ProposalID) || !operationIDPattern.MatchString(request.OperationID) || len(request.Digest) != 64 || now.IsZero() {
		return Receipt{}, ErrInvalid
	}
	proposal, err := ports.Get(ctx, request.ProposalID)
	if err != nil {
		return Receipt{}, err
	}
	if proposal.Scope != scope || proposal.ID != request.ProposalID {
		return Receipt{}, ErrUnauthorized
	}
	proposal = cloneProposal(proposal)
	// Integrity is checked against the creation instant first. An authorized
	// exact replay can recover a durable receipt even after the proposal's
	// creation window or the mutation has advanced Case/catalog revisions.
	if err := validateProposal(proposal, proposal.CreatedAt); err != nil || subtle.ConstantTimeCompare([]byte(proposal.Digest), []byte(request.Digest)) != 1 {
		return Receipt{}, ErrInvalid
	}
	if proposal.Draft.Kind == KindContext || proposal.Draft.Kind == KindExplain || !oneOf(proposal.Draft.Kind, KindQuery, KindCheck, KindValidator, KindScenario, KindCase, KindRun) {
		return Receipt{}, ErrUnsupported
	}
	if err := ports.Authorize(ctx, scope, proposal); err != nil {
		return Receipt{}, ErrUnauthorized
	}
	if err := ports.AuthorizeSources(ctx, scope, proposal.Sources); err != nil {
		return Receipt{}, ErrUnauthorized
	}
	binding := confirmationBinding(scope, proposal)
	prior, exists, err := ports.PriorReceipt(ctx, scope, request.OperationID)
	if err != nil {
		return Receipt{}, err
	}
	if exists {
		if !receiptMatches(prior, request, scope, proposal, binding) {
			return Receipt{}, detective.ErrConflict
		}
		return prior, nil
	}
	if !now.Before(proposal.ExpiresAt) {
		return Receipt{}, ErrInvalid
	}
	current, err := ports.Current(ctx, scope)
	if err != nil {
		return Receipt{}, err
	}
	if !validSnapshot(current) || !sameSnapshot(proposal.Basis, current) {
		return Receipt{}, ErrStale
	}
	receipt, err := ports.Dispatch(ctx, Command{Scope: scope, OperationID: request.OperationID, Proposal: proposal, Binding: binding})
	if err != nil {
		return Receipt{}, err
	}
	if !receiptMatches(receipt, request, scope, proposal, binding) {
		return Receipt{}, ErrInvalid
	}
	return receipt, nil
}

func receiptMatches(receipt Receipt, request ConfirmRequest, scope Scope, proposal Proposal, binding detective.OperationBinding) bool {
	return receipt.OperationID == request.OperationID && receipt.ScopeKey == scope.Key && receipt.Project == scope.Project && receipt.Actor == scope.Actor && receipt.ProposalID == proposal.ID && subtle.ConstantTimeCompare([]byte(receipt.PayloadDigest), []byte(binding.PayloadDigest)) == 1 && receipt.ResourceID != "" && !receipt.At.IsZero()
}

// confirmationBinding hashes length-prefixed fields. A verified proposal
// digest already covers its full immutable payload, so no second JSON
// encoding (and no depth-dependent encoder error) is needed here.
func confirmationBinding(scope Scope, proposal Proposal) detective.OperationBinding {
	action := "ai-confirm-" + proposal.Draft.Kind
	data := make([]byte, 0, 256)
	for _, field := range []string{scope.Key, scope.Actor, action, proposal.ID, proposal.Digest} {
		data = strconv.AppendInt(data, int64(len(field)), 10)
		data = append(data, ':')
		data = append(data, field...)
	}
	digest := sha256.Sum256(data)
	return detective.OperationBinding{Project: scope.Key, Actor: scope.Actor, Action: action, ResourceID: proposal.ID, PayloadDigest: hex.EncodeToString(digest[:])}
}
