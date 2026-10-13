package detectiveai

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/detective"
)

type fakePorts struct {
	*memoryStore
	snapshot          Snapshot
	authorizeErr      error
	authorizeAt       int
	sourceErr         error
	currentErr        error
	dispatchErr       error
	badReceipt        bool
	advanceOnDispatch bool
	receiptErr        error
	authorizations    int
	sourceChecks      int
	dispatches        int
	receipts          map[string]Receipt
}

func (p *fakePorts) Current(_ context.Context, _ Scope) (Snapshot, error) {
	return p.snapshot, p.currentErr
}
func (p *fakePorts) Authorize(_ context.Context, _ Scope, _ Proposal) error {
	p.authorizations++
	if p.authorizeAt == p.authorizations {
		return ErrUnauthorized
	}
	return p.authorizeErr
}
func (p *fakePorts) AuthorizeSources(_ context.Context, _ Scope, _ []SourceEvidence) error {
	p.sourceChecks++
	return p.sourceErr
}
func (p *fakePorts) PriorReceipt(_ context.Context, _ Scope, operationID string) (Receipt, bool, error) {
	if p.receiptErr != nil {
		return Receipt{}, false, p.receiptErr
	}
	r, exists := p.receipts[operationID]
	return r, exists, nil
}
func (p *fakePorts) Dispatch(_ context.Context, command Command) (Receipt, error) {
	p.dispatches++
	if p.dispatchErr != nil {
		return Receipt{}, p.dispatchErr
	}
	if old, ok := p.receipts[command.OperationID]; ok {
		if old.PayloadDigest != command.Binding.PayloadDigest || old.ScopeKey != command.Scope.Key {
			return Receipt{}, detective.ErrConflict
		}
		return old, nil
	}
	r := Receipt{OperationID: command.OperationID, ScopeKey: command.Scope.Key, Project: command.Scope.Project, Actor: command.Scope.Actor, ProposalID: command.Proposal.ID, PayloadDigest: command.Binding.PayloadDigest, ResourceID: "run-1", At: time.Date(2026, 10, 13, 9, 1, 0, 0, time.UTC)}
	if p.badReceipt {
		r.ScopeKey = "other"
	}
	if p.receipts == nil {
		p.receipts = map[string]Receipt{}
	}
	p.receipts[command.OperationID] = r
	if p.advanceOnDispatch {
		p.snapshot.CatalogRevision = "catalog-after-mutation"
		p.snapshot.Cases[0].Revision = "4"
	}
	return r, nil
}

func proposalFixture(t *testing.T) (Scope, Proposal, *fakePorts, time.Time) {
	t.Helper()
	scope, snapshot, sources := baseline()
	draft := runDraft()
	if err := validateDraft(draft, snapshot, sources); err != nil {
		t.Fatalf("bad fixture: %v", err)
	}
	now := time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)
	proposal := Proposal{ID: "proposal-1", Scope: scope, Basis: snapshot, Draft: draft, Sources: sourceEvidence(sources), CreatedAt: now, ExpiresAt: now.Add(15 * time.Minute)}
	var err error
	proposal.Digest, err = proposalDigest(proposal)
	if err != nil {
		t.Fatal(err)
	}
	ports := &fakePorts{memoryStore: &memoryStore{items: map[string]Proposal{proposal.ID: proposal}}, snapshot: cloneSnapshot(snapshot)}
	return scope, proposal, ports, now
}

func TestConfirmBindsFullScopeAndReauthorizesOnReplay(t *testing.T) {
	scope, proposal, ports, now := proposalFixture(t)
	request := ConfirmRequest{ProposalID: proposal.ID, Digest: proposal.Digest, OperationID: "op-1"}
	ports.advanceOnDispatch = true
	first, err := Confirm(context.Background(), scope, request, ports, now)
	if err != nil || first.ScopeKey != scope.Key || ports.dispatches != 1 || ports.authorizations != 1 || ports.sourceChecks != 1 {
		t.Fatalf("first confirmation = %#v, %v", first, err)
	}
	again, err := Confirm(context.Background(), scope, request, ports, now)
	if err != nil || again != first || ports.authorizations != 2 || ports.sourceChecks != 2 || ports.dispatches != 1 {
		t.Fatalf("replay skipped authorization: %#v, %v", again, err)
	}
	ports.sourceErr = errors.New("source revoked")
	if _, err := Confirm(context.Background(), scope, request, ports, now); !errors.Is(err, ErrUnauthorized) || ports.dispatches != 1 {
		t.Fatalf("revoked-source retry should stop before dispatch: %v", err)
	}
	ports.sourceErr = nil
	other := scope
	other.Key = "store:cloud/space:s2/project:demo/branch:main"
	if _, err := Confirm(context.Background(), other, request, ports, now); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("same slug in another Space accepted: %v", err)
	}
	if replay, err := Confirm(context.Background(), scope, request, ports, now.Add(16*time.Minute)); err != nil || replay != first {
		t.Fatalf("expired exact retry lost its durable receipt: %+v %v", replay, err)
	}
	conflict := first
	conflict.PayloadDigest = "foreign"
	ports.receipts[request.OperationID] = conflict
	if _, err := Confirm(context.Background(), scope, request, ports, now); !errors.Is(err, detective.ErrConflict) {
		t.Fatalf("conflicting operation receipt accepted: %v", err)
	}
}

func TestConfirmTreatsSerializedEmptyCollectionsAsUnchanged(t *testing.T) {
	scope, proposal, ports, now := proposalFixture(t)
	proposal.Basis.Validators = []Resource{}
	proposal.Basis.Checks[0].Inputs = []detective.Input{}
	proposal.Digest, _ = proposalDigest(proposal)
	encoded, err := json.Marshal(proposal)
	if err != nil {
		t.Fatal(err)
	}
	var stored Proposal
	if err := json.Unmarshal(encoded, &stored); err != nil {
		t.Fatal(err)
	}
	ports.items[proposal.ID] = stored
	ports.snapshot = cloneSnapshot(stored.Basis)
	ports.snapshot.Validators = nil
	ports.snapshot.Checks[0].Inputs = nil
	if _, err := Confirm(context.Background(), scope, ConfirmRequest{ProposalID: stored.ID, Digest: stored.Digest, OperationID: "unchanged"}, ports, now); err != nil {
		t.Fatalf("unchanged JSON catalog was stale or lost its digest: %v", err)
	}
	if sameSnapshot(Snapshot{CatalogRevision: "r", Environments: []string{}, Queries: []Resource{}, Checks: []Resource{}, Validators: []Resource{}, Scenarios: []Resource{}, Cases: []Resource{}, Runs: []Resource{}}, Snapshot{CatalogRevision: "r"}) == false {
		t.Fatal("empty and nil collections differ semantically")
	}
}

func TestConfirmRejectsStaleSpoofedAndUnsupportedProposals(t *testing.T) {
	scope, proposal, ports, now := proposalFixture(t)
	request := ConfirmRequest{ProposalID: proposal.ID, Digest: proposal.Digest, OperationID: "op-1"}
	for _, tc := range []struct {
		name string
		edit func(*Proposal, *fakePorts, *ConfirmRequest, *time.Time)
		want error
	}{
		{"expired", func(_ *Proposal, _ *fakePorts, _ *ConfirmRequest, at *time.Time) { *at = at.Add(16 * time.Minute) }, ErrInvalid},
		{"digest mismatch", func(_ *Proposal, _ *fakePorts, req *ConfirmRequest, _ *time.Time) { req.Digest = "bad" }, ErrInvalid},
		{"tampered payload", func(p *Proposal, _ *fakePorts, _ *ConfirmRequest, _ *time.Time) { p.Draft.Run.CheckID = "other" }, ErrInvalid},
		{"catalog changed", func(_ *Proposal, p *fakePorts, _ *ConfirmRequest, _ *time.Time) {
			p.snapshot.CatalogRevision = "catalog-6"
		}, ErrStale},
		{"query revised", func(_ *Proposal, p *fakePorts, _ *ConfirmRequest, _ *time.Time) {
			p.snapshot.Queries[0].Revision = "q-4"
		}, ErrStale},
		{"permission denied", func(_ *Proposal, p *fakePorts, _ *ConfirmRequest, _ *time.Time) {
			p.authorizeErr = errors.New("denied")
		}, ErrUnauthorized},
		{"source revoked", func(_ *Proposal, p *fakePorts, _ *ConfirmRequest, _ *time.Time) { p.sourceErr = errors.New("denied") }, ErrUnauthorized},
		{"invalid operation", func(_ *Proposal, _ *fakePorts, req *ConfirmRequest, _ *time.Time) { req.OperationID = "../escape" }, ErrInvalid},
		{"invalid proposal ID", func(_ *Proposal, _ *fakePorts, req *ConfirmRequest, _ *time.Time) { req.ProposalID = "../escape" }, ErrInvalid},
		{"unsupported explanation", func(p *Proposal, _ *fakePorts, req *ConfirmRequest, _ *time.Time) {
			p.Draft = Draft{Kind: KindExplain, Title: "failure", Explanation: &FailureExplanation{RunID: "run-a", Summary: "mismatch", Basis: "observed"}, Sources: runDraft().Sources}
			p.Digest, _ = proposalDigest(*p)
			req.Digest = p.Digest
		}, ErrUnsupported},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fresh := proposal
			fresh.Draft = runDraft()
			fresh.Basis = cloneSnapshot(proposal.Basis)
			current := cloneSnapshot(fresh.Basis)
			p := &fakePorts{memoryStore: &memoryStore{items: map[string]Proposal{}}, snapshot: current}
			req, at := request, now
			tc.edit(&fresh, p, &req, &at)
			p.items[fresh.ID] = fresh
			if _, err := Confirm(context.Background(), scope, req, p, at); !errors.Is(err, tc.want) || p.dispatches != 0 {
				t.Fatalf("unexpected confirmation: %v, dispatches=%d", err, p.dispatches)
			}
		})
	}
	_ = ports
}

func TestConfirmRejectsBadDispatcherReceiptAndErrors(t *testing.T) {
	scope, proposal, ports, now := proposalFixture(t)
	request := ConfirmRequest{ProposalID: proposal.ID, Digest: proposal.Digest, OperationID: "op-1"}
	ports.badReceipt = true
	if _, err := Confirm(context.Background(), scope, request, ports, now); !errors.Is(err, ErrInvalid) {
		t.Fatalf("foreign receipt accepted: %v", err)
	}
	ports.badReceipt = false
	ports.receipts = nil
	ports.dispatchErr = detective.ErrConflict
	if _, err := Confirm(context.Background(), scope, request, ports, now); !errors.Is(err, detective.ErrConflict) {
		t.Fatalf("dispatch conflict lost: %v", err)
	}
	ports.dispatchErr = nil
	ports.receiptErr = errors.New("receipt store unavailable")
	if _, err := Confirm(context.Background(), scope, request, ports, now); err == nil || errors.Is(err, ErrStale) {
		t.Fatalf("receipt read error flattened: %v", err)
	}
	ports.receiptErr = nil
	ports.currentErr = errors.New("catalog unavailable")
	if _, err := Confirm(context.Background(), scope, request, ports, now); err == nil || errors.Is(err, ErrStale) {
		t.Fatalf("catalog read failure flattened: %v", err)
	}
	ports.currentErr = nil
	ports.snapshot.CatalogRevision = ""
	if _, err := Confirm(context.Background(), scope, request, ports, now); !errors.Is(err, ErrStale) {
		t.Fatalf("invalid current snapshot accepted: %v", err)
	}
	request.ProposalID = "missing"
	if _, err := Confirm(context.Background(), scope, request, ports, now); !errors.Is(err, detective.ErrNotFound) {
		t.Fatalf("missing private proposal not surfaced: %v", err)
	}
}
