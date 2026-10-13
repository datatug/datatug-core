package detectiveai

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/detective"
	"github.com/datatug/datatug-core/pkg/investigation"
)

func reviewedDraftFixture(t *testing.T, draft Draft) (Scope, ReviewedDraftRequest, *fakePorts, time.Time) {
	t.Helper()
	scope, snapshot, sources := baseline()
	if err := validateDraft(draft, snapshot, sources); err != nil {
		t.Fatalf("invalid original fixture: %v", err)
	}
	now := time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)
	original := Proposal{ID: "proposal-original", Scope: scope, Basis: snapshot, Draft: cloneDraft(draft), Sources: sourceEvidence(sources), CreatedAt: now, ExpiresAt: now.Add(15 * time.Minute)}
	var err error
	original.Digest, err = proposalDigest(original)
	if err != nil {
		t.Fatal(err)
	}
	ports := &fakePorts{memoryStore: &memoryStore{items: map[string]Proposal{original.ID: original}}, snapshot: cloneSnapshot(snapshot)}
	return scope, ReviewedDraftRequest{ProposalID: original.ID, Digest: original.Digest, Draft: cloneDraft(draft)}, ports, now
}

func TestReviewedEditableDraftsRequireSeparateConfirmation(t *testing.T) {
	for _, tc := range []struct {
		name  string
		draft Draft
		edit  func(*Draft)
	}{
		{"query", typedDrafts()[1], func(d *Draft) { d.Query.Text = "SELECT id, total FROM invoices" }},
		{"check", typedDrafts()[2], func(d *Draft) { d.Check.Description = "Compare invoice details" }},
		{"validator", typedDrafts()[3], func(d *Draft) { d.Validator.Title = "ID exists" }},
		{"scenario", typedDrafts()[4], func(d *Draft) { d.Scenario.Description = "Review each invoice" }},
		{"case", typedDrafts()[5], func(d *Draft) { d.Case.Title = "Reviewed invoice issue" }},
		{"run", runDraft(), func(d *Draft) {
			value := investigation.NewIntegerValue("8")
			d.Run.Inputs["invoice_id"] = detective.InputValue{Scalar: &value}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scope, request, ports, now := reviewedDraftFixture(t, tc.draft)
			tc.edit(&request.Draft)
			derived, err := PrepareReviewedDraft(context.Background(), scope, request, ports, now, func() (string, error) { return "proposal-edited", nil })
			if err != nil || derived.ParentProposalID != request.ProposalID || derived.Draft.Kind != request.Draft.Kind || derived.Digest == request.Digest || ports.dispatches != 0 {
				t.Fatalf("reviewed draft = %+v, %v", derived, err)
			}
			if !reflect.DeepEqual(derived.Draft.Sources, ports.items[request.ProposalID].Draft.Sources) || !reflect.DeepEqual(derived.Sources, ports.items[request.ProposalID].Sources) || ports.sourceChecks != 1 || ports.authorizations != 2 {
				t.Fatal("review lost citations or skipped authority checks")
			}
			if stored := ports.items[derived.ID]; stored.Digest != derived.Digest || stored.ParentProposalID != request.ProposalID {
				t.Fatal("review did not persist a parent-linked immutable proposal")
			}
			if _, err := Confirm(context.Background(), scope, ConfirmRequest{ProposalID: derived.ID, Digest: derived.Digest, OperationID: "confirm-edit"}, ports, now); err != nil || ports.dispatches != 1 {
				t.Fatalf("edited draft confirmation = %v, dispatches=%d", err, ports.dispatches)
			}
		})
	}
}

func TestReviewedDraftRejectsTamperingStaleAndUnauthorizedEdits(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*Scope, *ReviewedDraftRequest, *fakePorts, *time.Time)
		want error
	}{
		{"missing ports", func(_ *Scope, _ *ReviewedDraftRequest, _ *fakePorts, _ *time.Time) {}, ErrInvalid},
		{"bad scope", func(s *Scope, _ *ReviewedDraftRequest, _ *fakePorts, _ *time.Time) { s.Key = "" }, ErrInvalid},
		{"invalid proposal ID", func(_ *Scope, r *ReviewedDraftRequest, _ *fakePorts, _ *time.Time) { r.ProposalID = "../other" }, ErrInvalid},
		{"invalid digest", func(_ *Scope, r *ReviewedDraftRequest, _ *fakePorts, _ *time.Time) { r.Digest = "wrong" }, ErrInvalid},
		{"missing time", func(_ *Scope, _ *ReviewedDraftRequest, _ *fakePorts, now *time.Time) { *now = time.Time{} }, ErrInvalid},
		{"foreign scope", func(s *Scope, _ *ReviewedDraftRequest, _ *fakePorts, _ *time.Time) {
			s.Key = "store:cloud/space:foreign/project:demo/branch:main"
		}, ErrUnauthorized},
		{"changed digest", func(_ *Scope, r *ReviewedDraftRequest, _ *fakePorts, _ *time.Time) {
			r.Digest = strings.Repeat("0", 64)
		}, ErrInvalid},
		{"expired", func(_ *Scope, _ *ReviewedDraftRequest, _ *fakePorts, now *time.Time) {
			*now = now.Add(16 * time.Minute)
		}, ErrInvalid},
		{"kind switch", func(_ *Scope, r *ReviewedDraftRequest, _ *fakePorts, _ *time.Time) { r.Draft.Kind = KindCase }, ErrUnsupported},
		{"citation edit", func(_ *Scope, r *ReviewedDraftRequest, _ *fakePorts, _ *time.Time) {
			r.Draft.Sources[0].Quote = "invoice 8"
		}, ErrInvalid},
		{"actor revoked", func(_ *Scope, _ *ReviewedDraftRequest, p *fakePorts, _ *time.Time) {
			p.authorizeErr = errors.New("revoked")
		}, ErrUnauthorized},
		{"source revoked", func(_ *Scope, _ *ReviewedDraftRequest, p *fakePorts, _ *time.Time) {
			p.sourceErr = errors.New("revoked")
		}, ErrUnauthorized},
		{"derived write denied", func(_ *Scope, _ *ReviewedDraftRequest, p *fakePorts, _ *time.Time) { p.authorizeAt = 2 }, ErrUnauthorized},
		{"catalog changed", func(_ *Scope, _ *ReviewedDraftRequest, p *fakePorts, _ *time.Time) {
			p.snapshot.CatalogRevision = "later"
		}, ErrStale},
		{"invalid catalog", func(_ *Scope, _ *ReviewedDraftRequest, p *fakePorts, _ *time.Time) { p.snapshot.CatalogRevision = "" }, ErrStale},
		{"invalid revised inputs", func(_ *Scope, r *ReviewedDraftRequest, _ *fakePorts, _ *time.Time) { r.Draft.Run.Inputs = nil }, ErrInvalid},
		{"check changed", func(_ *Scope, r *ReviewedDraftRequest, _ *fakePorts, _ *time.Time) { r.Draft.Run.CheckID = "missing" }, ErrStale},
		{"oversized edited payload", func(_ *Scope, r *ReviewedDraftRequest, _ *fakePorts, _ *time.Time) {
			r.Draft.Title = strings.Repeat("x", maxDraftJSON)
		}, ErrInvalid},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scope, request, ports, now := reviewedDraftFixture(t, runDraft())
			tc.edit(&scope, &request, ports, &now)
			var auth ConfirmationPorts = ports
			if tc.name == "missing ports" {
				auth = nil
			}
			if _, err := PrepareReviewedDraft(context.Background(), scope, request, auth, now, func() (string, error) { return "proposal-edited", nil }); !errors.Is(err, tc.want) {
				t.Fatalf("expected %v, got %v", tc.want, err)
			}
			if ports.items["proposal-edited"].ID != "" || ports.dispatches != 0 {
				t.Fatal("invalid review caused a durable write")
			}
		})
	}
}

func TestReviewedDraftSurfacesStoreAndDigestFailuresWithoutMutation(t *testing.T) {
	scope, request, ports, now := reviewedDraftFixture(t, runDraft())
	delete(ports.items, request.ProposalID)
	if _, err := PrepareReviewedDraft(context.Background(), scope, request, ports, now, nil); !errors.Is(err, detective.ErrNotFound) {
		t.Fatalf("missing parent = %v", err)
	}
	scope, request, ports, now = reviewedDraftFixture(t, runDraft())
	ports.currentErr = errors.New("current catalog unavailable")
	if _, err := PrepareReviewedDraft(context.Background(), scope, request, ports, now, nil); !errors.Is(err, ports.currentErr) {
		t.Fatalf("current catalog error = %v", err)
	}
	ports.currentErr = nil
	for _, id := range []string{"../escape", request.ProposalID} {
		if _, err := PrepareReviewedDraft(context.Background(), scope, request, ports, now, func() (string, error) { return id, nil }); !errors.Is(err, ErrInvalid) {
			t.Fatalf("unsafe derived ID %q = %v", id, err)
		}
	}
	if _, err := PrepareReviewedDraft(context.Background(), scope, request, ports, now, func() (string, error) { return "", errors.New("id generator failed") }); !errors.Is(err, ErrInvalid) {
		t.Fatalf("failed ID generator = %v", err)
	}
	digestErr := errors.New("canonical digest unavailable")
	if _, err := prepareReviewedDraft(context.Background(), scope, request, ports, now, nil, func(Proposal) (string, error) { return "", digestErr }); !errors.Is(err, digestErr) {
		t.Fatalf("digest failure = %v", err)
	}
	ports.err = errors.New("private proposal store unavailable")
	if _, err := PrepareReviewedDraft(context.Background(), scope, request, ports, now, nil); !errors.Is(err, ports.err) {
		t.Fatalf("private store failure = %v", err)
	}
	ports.err = nil
	derived, err := PrepareReviewedDraft(context.Background(), scope, request, ports, now, nil)
	if err != nil || !detective.ValidSlug(derived.ID) || derived.ID == request.ProposalID {
		t.Fatalf("default ID generator = %+v, %v", derived, err)
	}
	if len(ports.items) != 2 || ports.dispatches != 0 {
		t.Fatal("failed review caused a durable mutation")
	}
}

func TestReviewedDraftRejectsUnsupportedOriginalKinds(t *testing.T) {
	for _, draft := range []Draft{typedDrafts()[0], typedDrafts()[7]} {
		scope, request, ports, now := reviewedDraftFixture(t, draft)
		if _, err := PrepareReviewedDraft(context.Background(), scope, request, ports, now, func() (string, error) { return "proposal-edited", nil }); !errors.Is(err, ErrUnsupported) || len(ports.items) != 1 {
			t.Fatalf("unsupported review kind %q = %v", draft.Kind, err)
		}
	}
}
