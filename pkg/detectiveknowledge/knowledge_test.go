package detectiveknowledge

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

var knowledgeTime = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

func draft() Proposal {
	return Proposal{ID: "invoice-rule", CaseID: "invoice-case", Kind: "invariant", Title: "Customer exists", Statement: "Invoice customer resolves in CRM", EvidenceRefs: []string{"source-note"}, FindingRefs: []string{"missing-customer"}, ClaimStatus: "confirmed"}
}

func target() Target {
	return Target{System: "meaninggraph", Repository: "datatug/meaninggraph", ResourceID: "invoice.customer", ExpectedRevision: "mg-r17"}
}

func facts() EvidenceFacts {
	return EvidenceFacts{CaseID: "invoice-case", EvidenceIDs: []string{"source-note"}, Findings: []FindingFact{{ID: "missing-customer", Status: "confirmed", EvidenceRefs: []string{"source-note"}}}}
}

func TestKnowledgeJourneyKeepsPrivateVersionedHandoffsAndCopies(t *testing.T) {
	input := draft()
	if err := ValidateActionShape(Action{Type: "save-draft", Proposal: &input}); err != nil {
		t.Fatal(err)
	}
	if err := ValidateReferences(input, facts()); err != nil {
		t.Fatal(err)
	}
	saved, err := SaveDraft(nil, input, 0, "actor", knowledgeTime)
	if err != nil || saved.Version != 1 || saved.Status != "draft" || saved.CreatedBy != "actor" {
		t.Fatalf("saved: %+v %v", saved, err)
	}
	input.EvidenceRefs[0] = "changed"
	if saved.EvidenceRefs[0] != "source-note" {
		t.Fatal("caller changed saved evidence")
	}
	hypothesisDraft := draft()
	hypothesisDraft.ClaimStatus = ""
	hypothesis, err := SaveDraft(nil, hypothesisDraft, 0, "actor", knowledgeTime)
	if err != nil || hypothesis.ClaimStatus != "hypothesis" {
		t.Fatalf("unspecified claim must remain a hypothesis: %+v %v", hypothesis, err)
	}
	if err := ValidateActionShape(Action{Type: "review", ProposalID: saved.ID, IfMatch: 1}); err != nil {
		t.Fatal(err)
	}
	reviewed, err := Review(saved, 1, "review", "Checked Case evidence", "reviewer", knowledgeTime.Add(time.Minute))
	if err != nil || reviewed.Status != "reviewed" || reviewed.Version != 2 {
		t.Fatalf("reviewed: %+v %v", reviewed, err)
	}
	if err := ValidateActionShape(Action{Type: "export", ProposalID: reviewed.ID, IfMatch: 2, Target: ptr(target())}); err != nil {
		t.Fatal(err)
	}
	exported, handoff, err := PrepareHandoff(reviewed, 2, target(), "reviewer", "export-one", knowledgeTime.Add(2*time.Minute))
	if err != nil || exported.Version != 3 || handoff.ProposalVersion != 2 || handoff.Target.ExpectedRevision != "mg-r17" || handoff.Status != "ready-for-target-review" {
		t.Fatalf("handoff: %+v %+v %v", exported, handoff, err)
	}
	handoff.EvidenceRefs[0] = "changed"
	exported.Handoffs[0].FindingRefs[0] = "changed"
	if reviewed.Handoffs != nil || exported.Handoffs[0].EvidenceRefs[0] != "source-note" || reviewed.FindingRefs[0] != "missing-customer" {
		t.Fatal("handoff aliased source or returned projection")
	}
	second, secondHandoff, err := PrepareHandoff(exported, 3, Target{System: "modelspec", Repository: "datatug/modelspec", ResourceID: "invoice", ExpectedRevision: "ms-r8"}, "reviewer", "export-two", knowledgeTime.Add(3*time.Minute))
	if err != nil || second.Version != 4 || secondHandoff.ProposalVersion != 3 || len(second.Handoffs) != 2 {
		t.Fatalf("second handoff: %+v %+v %v", second, secondHandoff, err)
	}
	if second.Handoffs[0].FindingRefs[0] != "changed" {
		t.Fatal("old handoff unexpectedly edited")
	}
}

func TestKnowledgeReferencesRequireCaseEvidenceAndConfirmedFinding(t *testing.T) {
	base := draft()
	for _, tc := range []struct {
		name string
		edit func(*Proposal, *EvidenceFacts)
	}{
		{"wrong case", func(_ *Proposal, f *EvidenceFacts) { f.CaseID = "other" }},
		{"missing evidence", func(_ *Proposal, f *EvidenceFacts) { f.EvidenceIDs = nil }},
		{"duplicate available evidence", func(_ *Proposal, f *EvidenceFacts) { f.EvidenceIDs = append(f.EvidenceIDs, "source-note") }},
		{"invalid available evidence", func(_ *Proposal, f *EvidenceFacts) { f.EvidenceIDs = []string{"Bad"} }},
		{"missing finding", func(_ *Proposal, f *EvidenceFacts) { f.Findings = nil }},
		{"duplicate finding", func(_ *Proposal, f *EvidenceFacts) { f.Findings = append(f.Findings, f.Findings[0]) }},
		{"invalid finding", func(_ *Proposal, f *EvidenceFacts) { f.Findings[0].ID = "Bad" }},
		{"invalid finding status", func(_ *Proposal, f *EvidenceFacts) { f.Findings[0].Status = "claimed" }},
		{"hypothesis finding", func(_ *Proposal, f *EvidenceFacts) { f.Findings[0].Status = "hypothesis" }},
		{"missing evidence support", func(_ *Proposal, f *EvidenceFacts) { f.Findings[0].EvidenceRefs = []string{"other"} }},
		{"unsupported evidence", func(_ *Proposal, f *EvidenceFacts) { f.Findings[0].EvidenceRefs = nil }},
		{"confirmed without finding", func(p *Proposal, _ *EvidenceFacts) { p.FindingRefs = nil }},
		{"invalid claim status", func(p *Proposal, _ *EvidenceFacts) { p.ClaimStatus = "unknown" }},
		{"invalid ref", func(p *Proposal, _ *EvidenceFacts) { p.EvidenceRefs[0] = "Bad" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, f := cloneProposal(base), facts()
			tc.edit(&p, &f)
			if err := ValidateReferences(p, f); !errors.Is(err, ErrInvalid) {
				t.Fatalf("got %v", err)
			}
		})
	}
	tooMany := cloneProposal(base)
	tooMany.EvidenceRefs = make([]string, 21)
	if err := ValidateReferences(tooMany, facts()); !errors.Is(err, ErrInvalid) {
		t.Fatal("oversized reference list must fail closed")
	}
	hypothesis := cloneProposal(base)
	hypothesis.ClaimStatus = "hypothesis"
	hypothesis.FindingRefs = nil
	if err := ValidateReferences(hypothesis, facts()); err != nil {
		t.Fatal(err)
	}
	hypothesis.FindingRefs = []string{"missing-customer"}
	f := facts()
	f.Findings[0].Status = "hypothesis"
	if err := ValidateReferences(hypothesis, f); err != nil {
		t.Fatal(err)
	}
}

func TestKnowledgeActionAndTargetRejectAmbiguousPayloads(t *testing.T) {
	base := draft()
	badDrafts := []func(*Proposal){
		func(p *Proposal) { p.ID = "Bad" }, func(p *Proposal) { p.CaseID = "Bad" },
		func(p *Proposal) { p.Kind = "fact" }, func(p *Proposal) { p.Title = " " },
		func(p *Proposal) { p.Title = string(make([]byte, 201)) },
		func(p *Proposal) { p.Statement = " " },
		func(p *Proposal) { p.Statement = string(make([]byte, 4001)) },
		func(p *Proposal) { p.Title = "a\x00b" }, func(p *Proposal) { p.Statement = "a\x00b" },
		func(p *Proposal) { p.ClaimStatus = "asserted" },
		func(p *Proposal) { p.EvidenceRefs = nil },
		func(p *Proposal) { p.EvidenceRefs = make([]string, 21) },
		func(p *Proposal) { p.FindingRefs = make([]string, 21) },
		func(p *Proposal) { p.EvidenceRefs = []string{"source-note", "source-note"} },
		func(p *Proposal) { p.FindingRefs = []string{"missing-customer", "missing-customer"} },
		func(p *Proposal) { p.FindingRefs = []string{"Bad"} },
		func(p *Proposal) { p.Status = "reviewed" }, func(p *Proposal) { p.Version = 2 },
		func(p *Proposal) { p.CreatedAt = knowledgeTime }, func(p *Proposal) { p.CreatedBy = "actor" },
		func(p *Proposal) { p.ReviewedAt = knowledgeTime }, func(p *Proposal) { p.ReviewedBy = "actor" },
		func(p *Proposal) { p.ReviewNote = "forged" },
		func(p *Proposal) { p.Handoffs = []Handoff{{Receipt: "forged"}} },
	}
	for i, edit := range badDrafts {
		p := cloneProposal(base)
		edit(&p)
		if err := ValidateDraft(p); !errors.Is(err, ErrInvalid) {
			t.Fatalf("draft %d: %v", i, err)
		}
	}
	valid := Action{Type: "save-draft", Proposal: &base}
	for _, edit := range []func(*Action){
		func(a *Action) { a.IfMatch = -1 }, func(a *Action) { a.ReviewNote = string(make([]byte, 2001)) },
		func(a *Action) { a.ReviewNote = "a\x00b" }, func(a *Action) { a.Proposal = nil },
		func(a *Action) { a.ProposalID = "other" }, func(a *Action) { a.Target = ptr(target()) },
		func(a *Action) { a.ReviewNote = "forged" }, func(a *Action) { a.Type = "unknown" },
	} {
		a := valid
		edit(&a)
		if err := ValidateActionShape(a); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid action accepted: %+v", a)
		}
	}
	for _, a := range []Action{
		{Type: "review", ProposalID: "Bad"}, {Type: "review", ProposalID: base.ID, Proposal: &base},
		{Type: "reject", ProposalID: base.ID, Target: ptr(target())},
		{Type: "export", ProposalID: base.ID, Target: nil},
		{Type: "export", ProposalID: base.ID, Target: ptr(target()), ReviewNote: "secret"},
	} {
		if err := ValidateActionShape(a); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid action accepted: %+v", a)
		}
	}
	for _, edit := range []func(*Target){
		func(x *Target) { x.System = "registry" }, func(x *Target) { x.Repository = "datatug" },
		func(x *Target) { x.Repository = "datatug/" }, func(x *Target) { x.Repository = "/modelspec" },
		func(x *Target) { x.Repository = "datatug/group/modelspec" },
		func(x *Target) { x.Repository = "../modelspec" }, func(x *Target) { x.Repository = "datatug/.." },
		func(x *Target) { x.Repository = "datatug /modelspec" },
		func(x *Target) { x.ResourceID = "" }, func(x *Target) { x.ExpectedRevision = " " },
		func(x *Target) { x.ExpectedRevision = "x\n" },
		func(x *Target) { x.ResourceID = string(make([]byte, 513)) },
	} {
		x := target()
		edit(&x)
		if err := ValidateTarget(&x); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid target accepted: %+v", x)
		}
	}
	if err := ValidateTarget(nil); !errors.Is(err, ErrInvalid) {
		t.Fatal(err)
	}
}

func TestKnowledgeTransitionsEnforceOptimisticStateAndCapacity(t *testing.T) {
	base := draft()
	for _, tc := range []struct {
		old     *Proposal
		draft   Proposal
		version int64
		actor   string
		at      time.Time
		want    error
	}{
		{nil, base, 1, "actor", knowledgeTime, ErrConflict},
		{nil, base, -1, "actor", knowledgeTime, ErrInvalid},
		{nil, base, 0, "", knowledgeTime, ErrInvalid},
		{nil, base, 0, "actor", time.Time{}, ErrInvalid},
		{nil, base, 0, "a\x00b", knowledgeTime, ErrInvalid},
		{nil, base, 0, string(make([]byte, 257)), knowledgeTime, ErrInvalid},
		{nil, Proposal{ID: "Bad"}, 0, "actor", knowledgeTime, ErrInvalid},
	} {
		_, err := SaveDraft(tc.old, tc.draft, tc.version, tc.actor, tc.at)
		if !errors.Is(err, tc.want) {
			t.Fatalf("save expected %v, got %v", tc.want, err)
		}
	}
	saved, _ := SaveDraft(nil, base, 0, "actor", knowledgeTime)
	changed := draft()
	changed.Title = "Updated title"
	updated, err := SaveDraft(&saved, changed, 1, "editor", knowledgeTime.Add(time.Minute))
	if err != nil || updated.Version != 2 || updated.CreatedBy != "actor" || updated.Title != "Updated title" {
		t.Fatalf("update: %+v %v", updated, err)
	}
	for _, mutation := range []func(*Proposal){
		func(p *Proposal) { p.Status = "reviewed" },
		func(p *Proposal) { p.Version = 4 },
		func(p *Proposal) { p.ID = "different" },
		func(p *Proposal) { p.CaseID = "different" },
	} {
		old := saved
		mutation(&old)
		if _, err := SaveDraft(&old, changed, 1, "actor", knowledgeTime); !errors.Is(err, ErrConflict) {
			t.Fatalf("stale edit: %v", err)
		}
	}
	for _, tc := range []struct {
		old                   Proposal
		version               int64
		decision, note, actor string
		at                    time.Time
		want                  error
	}{
		{saved, 1, "approve", "", "actor", knowledgeTime, ErrInvalid},
		{saved, 1, "review", string(make([]byte, 2001)), "actor", knowledgeTime, ErrInvalid},
		{saved, 1, "review", "x\x00y", "actor", knowledgeTime, ErrInvalid},
		{saved, 1, "review", "", "", knowledgeTime, ErrInvalid},
		{saved, 1, "review", "", "actor", time.Time{}, ErrInvalid},
		{saved, 0, "review", "", "actor", knowledgeTime, ErrConflict},
	} {
		_, err := Review(tc.old, tc.version, tc.decision, tc.note, tc.actor, tc.at)
		if !errors.Is(err, tc.want) {
			t.Fatalf("review expected %v, got %v", tc.want, err)
		}
	}
	rejected, err := Review(saved, 1, "reject", "unsupported", "actor", knowledgeTime)
	if err != nil || rejected.Status != "rejected" || rejected.Version != 2 {
		t.Fatalf("rejection: %+v %v", rejected, err)
	}
	if _, err := Review(rejected, 2, "review", "", "actor", knowledgeTime); !errors.Is(err, ErrConflict) {
		t.Fatal(err)
	}
	reviewed, _ := Review(saved, 1, "review", "", "actor", knowledgeTime)
	for _, tc := range []struct {
		old            Proposal
		version        int64
		target         Target
		actor, receipt string
		at             time.Time
		want           error
	}{
		{reviewed, 2, Target{}, "actor", "receipt", knowledgeTime, ErrInvalid},
		{reviewed, 2, target(), "actor", "Bad", knowledgeTime, ErrInvalid},
		{reviewed, 2, target(), "", "receipt", knowledgeTime, ErrInvalid},
		{reviewed, 2, target(), "actor", "receipt", time.Time{}, ErrInvalid},
		{saved, 1, target(), "actor", "receipt", knowledgeTime, ErrConflict},
		{reviewed, 1, target(), "actor", "receipt", knowledgeTime, ErrConflict},
	} {
		_, _, err := PrepareHandoff(tc.old, tc.version, tc.target, tc.actor, tc.receipt, tc.at)
		if !errors.Is(err, tc.want) {
			t.Fatalf("handoff expected %v, got %v", tc.want, err)
		}
	}
	full := reviewed
	full.Handoffs = make([]Handoff, MaxHandoffs)
	if _, _, err := PrepareHandoff(full, 2, target(), "actor", "receipt", knowledgeTime); !errors.Is(err, ErrCapacity) {
		t.Fatal(err)
	}
	if reflect.DeepEqual(saved, reviewed) {
		t.Fatal("review failed to change projection")
	}
}

func ptr[T any](v T) *T { return &v }
