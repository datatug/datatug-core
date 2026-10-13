package detectiveai

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/detective"
	"github.com/datatug/datatug-core/pkg/investigation"
)

func contextReviewFixture(t *testing.T) (Scope, ContextReviewRequest, *fakePorts, time.Time) {
	t.Helper()
	scope, snapshot, sources := baseline()
	draft := typedDrafts()[0]
	if err := validateDraft(draft, snapshot, sources); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)
	proposal := Proposal{ID: "proposal-context", Scope: scope, Basis: cloneSnapshot(snapshot), Draft: draft, Sources: sourceEvidence(sources), CreatedAt: now, ExpiresAt: now.Add(15 * time.Minute)}
	var err error
	proposal.Digest, err = proposalDigest(proposal)
	if err != nil {
		t.Fatal(err)
	}
	value := investigation.NewIntegerValue("8") // Explicit human correction of extracted invoice 7.
	request := ContextReviewRequest{ProposalID: proposal.ID, Digest: proposal.Digest, CaseID: "case-a", ExpectedVersion: 3, Context: map[string]detective.InputValue{"invoice_id": {Scalar: &value}}}
	ports := &fakePorts{memoryStore: &memoryStore{items: map[string]Proposal{proposal.ID: proposal}}, snapshot: cloneSnapshot(snapshot)}
	return scope, request, ports, now
}

func TestReviewedContextBecomesBoundCaseEditThenConfirms(t *testing.T) {
	scope, request, ports, now := contextReviewFixture(t)
	derived, err := PrepareContextCaseEdit(context.Background(), scope, request, ports, now, func() (string, error) { return "proposal-reviewed", nil })
	if err != nil || derived.ParentProposalID != request.ProposalID || derived.Draft.Kind != KindCase || derived.Draft.Case.ExpectedVersion != 3 || derived.Draft.Case.Context["invoice_id"].Scalar.Str != "8" || ports.dispatches != 0 {
		t.Fatalf("review proposal: %+v %v", derived, err)
	}
	request.Context["invoice_id"].Scalar.Str = "999"
	stored := ports.items[derived.ID]
	if stored.Draft.Case.Context["invoice_id"].Scalar.Str != "8" {
		t.Fatal("human edit mutated retained review payload")
	}
	ports.advanceOnDispatch = true
	confirmation := ConfirmRequest{ProposalID: derived.ID, Digest: derived.Digest, OperationID: "apply-context"}
	receipt, err := Confirm(context.Background(), scope, confirmation, ports, now)
	if err != nil || receipt.ProposalID != derived.ID || ports.dispatches != 1 {
		t.Fatalf("case edit confirmation: %+v %v", receipt, err)
	}
	if replay, err := Confirm(context.Background(), scope, confirmation, ports, now); err != nil || replay != receipt || ports.dispatches != 1 {
		t.Fatalf("same edit replayed instead of reconciled: %+v %v", replay, err)
	}
	ports.sourceErr = errors.New("source revoked")
	if _, err := Confirm(context.Background(), scope, confirmation, ports, now); !errors.Is(err, ErrUnauthorized) || ports.dispatches != 1 {
		t.Fatalf("revoked source disclosed retained edit: %v", err)
	}
}

func TestReviewedContextRejectsSpoofingAndStaleCase(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*Scope, *ContextReviewRequest, *fakePorts, *time.Time)
		want error
	}{
		{"bad scope", func(s *Scope, _ *ContextReviewRequest, _ *fakePorts, _ *time.Time) { s.Key = "" }, ErrInvalid},
		{"bad proposal id", func(_ *Scope, r *ContextReviewRequest, _ *fakePorts, _ *time.Time) { r.ProposalID = "../x" }, ErrInvalid},
		{"bad digest shape", func(_ *Scope, r *ContextReviewRequest, _ *fakePorts, _ *time.Time) { r.Digest = "wrong" }, ErrInvalid},
		{"bad case id", func(_ *Scope, r *ContextReviewRequest, _ *fakePorts, _ *time.Time) { r.CaseID = "Bad" }, ErrInvalid},
		{"missing version", func(_ *Scope, r *ContextReviewRequest, _ *fakePorts, _ *time.Time) { r.ExpectedVersion = 0 }, ErrInvalid},
		{"empty context", func(_ *Scope, r *ContextReviewRequest, _ *fakePorts, _ *time.Time) { r.Context = nil }, ErrInvalid},
		{"invalid key", func(_ *Scope, r *ContextReviewRequest, _ *fakePorts, _ *time.Time) {
			r.Context = map[string]detective.InputValue{"bad key": r.Context["invoice_id"]}
		}, ErrInvalid},
		{"invalid typed value", func(_ *Scope, r *ContextReviewRequest, _ *fakePorts, _ *time.Time) {
			r.Context["invoice_id"] = detective.InputValue{}
		}, ErrInvalid},
		{"too many fields", func(_ *Scope, r *ContextReviewRequest, _ *fakePorts, _ *time.Time) {
			r.Context = map[string]detective.InputValue{}
			for i := 0; i < 65; i++ {
				r.Context["input"+string(rune('A'+i))] = detective.InputValue{Scalar: ptr(investigation.NewIntegerValue("1"))}
			}
		}, ErrInvalid},
		{"zero clock", func(_ *Scope, _ *ContextReviewRequest, _ *fakePorts, at *time.Time) { *at = time.Time{} }, ErrInvalid},
		{"other Space", func(s *Scope, _ *ContextReviewRequest, _ *fakePorts, _ *time.Time) {
			s.Key = "store:cloud/space:s2/project:demo/branch:main"
		}, ErrUnauthorized},
		{"digest mismatch", func(_ *Scope, r *ContextReviewRequest, _ *fakePorts, _ *time.Time) {
			r.Digest = "0000000000000000000000000000000000000000000000000000000000000000"
		}, ErrInvalid},
		{"expired source", func(_ *Scope, _ *ContextReviewRequest, _ *fakePorts, at *time.Time) { *at = at.Add(16 * time.Minute) }, ErrInvalid},
		{"not extraction", func(_ *Scope, _ *ContextReviewRequest, p *fakePorts, _ *time.Time) {
			original := p.items["proposal-context"]
			original.Draft = runDraft()
			original.Digest, _ = proposalDigest(original)
			p.items[original.ID] = original
		}, ErrInvalid},
		{"actor denied", func(_ *Scope, _ *ContextReviewRequest, p *fakePorts, _ *time.Time) {
			p.authorizeErr = errors.New("denied")
		}, ErrUnauthorized},
		{"case edit denied", func(_ *Scope, _ *ContextReviewRequest, p *fakePorts, _ *time.Time) {
			p.authorizeAt = 2
		}, ErrUnauthorized},
		{"source denied", func(_ *Scope, _ *ContextReviewRequest, p *fakePorts, _ *time.Time) {
			p.sourceErr = errors.New("denied")
		}, ErrUnauthorized},
		{"catalog stale", func(_ *Scope, _ *ContextReviewRequest, p *fakePorts, _ *time.Time) {
			p.snapshot.CatalogRevision = "catalog-6"
		}, ErrStale},
		{"case version stale", func(_ *Scope, _ *ContextReviewRequest, p *fakePorts, _ *time.Time) {
			p.snapshot.Cases[0].Revision = "4"
		}, ErrStale},
		{"case not in catalog", func(_ *Scope, r *ContextReviewRequest, _ *fakePorts, _ *time.Time) { r.CaseID = "other-case" }, ErrStale},
		{"invalid catalog", func(_ *Scope, _ *ContextReviewRequest, p *fakePorts, _ *time.Time) { p.snapshot.CatalogRevision = "" }, ErrStale},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scope, request, ports, now := contextReviewFixture(t)
			tc.edit(&scope, &request, ports, &now)
			if _, err := PrepareContextCaseEdit(context.Background(), scope, request, ports, now, func() (string, error) { return "proposal-reviewed", nil }); !errors.Is(err, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, err)
			}
		})
	}
}

func TestReviewedContextSurfacesAdapterFailuresAndRejectsUnsafeIDs(t *testing.T) {
	scope, request, ports, now := contextReviewFixture(t)
	for _, id := range []string{"../escape", request.ProposalID} {
		if _, err := PrepareContextCaseEdit(context.Background(), scope, request, ports, now, func() (string, error) { return id, nil }); !errors.Is(err, ErrInvalid) {
			t.Fatalf("unsafe derived ID %q accepted: %v", id, err)
		}
	}
	if _, err := PrepareContextCaseEdit(context.Background(), scope, request, ports, now, func() (string, error) { return "", errors.New("random failed") }); !errors.Is(err, ErrInvalid) {
		t.Fatalf("ID failure lost: %v", err)
	}
	ports.currentErr = errors.New("catalog unavailable")
	if _, err := PrepareContextCaseEdit(context.Background(), scope, request, ports, now, nil); !errors.Is(err, ports.currentErr) {
		t.Fatalf("catalog error lost: %v", err)
	}
	ports.currentErr = nil
	digestErr := errors.New("canonical payload unavailable")
	if _, err := prepareContextCaseEdit(context.Background(), scope, request, ports, now, nil, func(Proposal) (string, error) { return "", digestErr }); !errors.Is(err, digestErr) {
		t.Fatalf("digest failure lost: %v", err)
	}
	if len(ports.items) != 1 {
		t.Fatal("failed digest wrote a proposal")
	}
	ports.err = errors.New("private proposal store unavailable")
	if _, err := PrepareContextCaseEdit(context.Background(), scope, request, ports, now, nil); !errors.Is(err, ports.err) {
		t.Fatalf("save error lost: %v", err)
	}
	ports.err = nil
	delete(ports.items, request.ProposalID)
	if _, err := PrepareContextCaseEdit(context.Background(), scope, request, ports, now, nil); !errors.Is(err, detective.ErrNotFound) {
		t.Fatalf("missing extraction not surfaced: %v", err)
	}
}

func TestReviewedContextRejectsOversizedEditedValueBeforePersistence(t *testing.T) {
	scope, request, ports, now := contextReviewFixture(t)
	value := investigation.NewStringValue(strings.Repeat("s", 1<<20))
	request.Context = map[string]detective.InputValue{"notes": {Scalar: &value}}
	if _, err := PrepareContextCaseEdit(context.Background(), scope, request, ports, now, func() (string, error) { return "review-big", nil }); !errors.Is(err, ErrInvalid) {
		t.Fatalf("oversized edited context accepted: %v", err)
	}
	if len(ports.items) != 1 || ports.items["review-big"].ID != "" {
		t.Fatal("oversized reviewed context reached durable proposal store")
	}
}

func ptr[T any](value T) *T { return &value }
