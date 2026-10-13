package detectiveai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"strings"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/detective"
	"github.com/datatug/datatug-core/pkg/investigation"
	"github.com/strongo/aichat/ai"
)

type memoryStore struct {
	items map[string]Proposal
	err   error
}

func (s *memoryStore) Save(_ context.Context, p Proposal) error {
	if s.err != nil {
		return s.err
	}
	if s.items == nil {
		s.items = map[string]Proposal{}
	}
	s.items[p.ID] = p
	return nil
}

func (s *memoryStore) Get(_ context.Context, id string) (Proposal, error) {
	item, ok := s.items[id]
	if !ok {
		return Proposal{}, detective.ErrNotFound
	}
	return item, nil
}

type fakeProvider struct {
	steps [][]ai.Event
	calls int
	seen  []ai.ChatRequest
	err   error
}

func (*fakeProvider) Name() string { return "test" }
func (p *fakeProvider) Stream(_ context.Context, request ai.ChatRequest) iter.Seq2[ai.Event, error] {
	p.seen = append(p.seen, request)
	step := p.steps[p.calls]
	p.calls++
	return func(yield func(ai.Event, error) bool) {
		for _, event := range step {
			if !yield(event, nil) {
				return
			}
		}
		if p.err != nil {
			yield(ai.Event{Type: ai.EventError}, p.err)
		}
	}
}

func baseline() (Scope, Snapshot, []Source) {
	return Scope{Key: "store:local/space:s1/project:demo/branch:main", Project: "demo", Actor: "user-1"}, Snapshot{
		CatalogRevision: "catalog-5",
		Environments:    []string{"local", "qa"},
		Queries:         []Resource{{ID: "invoice-list", Title: "Invoices", Revision: "q-3", Inputs: []detective.Input{{Name: "invoice_id", Type: investigation.ValueTypeInteger, Cardinality: "scalar"}}}},
		Checks:          []Resource{{ID: "invoice-check", Title: "Invoice Check", Revision: "2", Inputs: []detective.Input{{Name: "invoice_id", Type: investigation.ValueTypeInteger, Cardinality: "scalar", Required: true}}}},
		Scenarios:       []Resource{{ID: "invoice-scenario", Title: "Invoice Guide", Revision: "1", Inputs: []detective.Input{{Name: "invoice_id", Type: investigation.ValueTypeInteger, Cardinality: "scalar"}}}},
		Cases:           []Resource{{ID: "case-a", Title: "Case A", Revision: "3"}},
		Runs:            []Resource{{ID: "run-a", Title: "Failed run", Revision: "1"}},
	}, []Source{{ID: "ticket", Kind: "paste", Revision: "ticket-1", Text: "invoice 7"}}
}

func runDraft() Draft {
	value := investigation.NewIntegerValue("7")
	return Draft{Kind: KindRun, Title: "Check invoice 7", Run: &RunDraft{CheckID: "invoice-check", Environment: "local", Inputs: map[string]detective.InputValue{"invoice_id": {Scalar: &value}}}, Sources: []Provenance{{SourceID: "ticket", Start: 0, End: 9, Quote: "invoice 7", Confidence: .9}}}
}

func toolCall(id, name string, value any) ai.Event {
	raw, _ := json.Marshal(value)
	return ai.Event{Type: ai.EventToolCall, ToolCall: &ai.ToolCall{ID: id, Name: name, Arguments: raw}}
}

func TestAssistantMakesReviewableProposalWithoutExecuting(t *testing.T) {
	scope, snapshot, sources := baseline()
	store := &memoryStore{}
	provider := &fakeProvider{steps: [][]ai.Event{
		{toolCall("discovery", "discover", discoveryRequest{Kind: "check", InputNames: []string{"invoice_id"}, Environment: "local"}), toolCall("proposal", "submit_proposal", runDraft()), {Type: ai.EventCompleted, StopReason: ai.StopReasonToolCalls}},
		{{Type: ai.EventTextDelta, Text: "Review the proposed run."}, {Type: ai.EventCompleted, StopReason: ai.StopReasonEnd}},
	}}
	assistant := Assistant{Provider: provider, Store: store, Now: func() time.Time { return time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC) }, NewID: func() (string, error) { return "proposal-1", nil }}
	response, err := assistant.Run(context.Background(), Request{Scope: scope, Snapshot: snapshot, Text: "Investigate invoice 7", Sources: sources})
	if err != nil || len(response.Proposals) != 1 || len(response.Matches) != 1 || response.Text != "Review the proposed run." {
		t.Fatalf("proposal-only run = %#v, %v", response, err)
	}
	if provider.calls != 2 || len(provider.seen[0].Tools) != 2 || provider.seen[0].Product != "datatug" || provider.seen[0].MaxTokens != 1200 {
		t.Fatalf("wrong bounded agent contract: %#v", provider.seen[0])
	}
	stored := store.items["proposal-1"]
	encoded, _ := json.Marshal(stored)
	if strings.Contains(string(encoded), `"text":"invoice 7"`) || stored.Sources[0].Digest == "" || stored.Sources[0].Length != 9 {
		t.Fatalf("stored proposal retained pasted source: %s", encoded)
	}
	if err := validateProposal(stored, stored.CreatedAt); err != nil {
		t.Fatalf("stored proposal invalid: %v", err)
	}
	response.Proposals[0].Draft.Run.CheckID = "altered"
	if store.items["proposal-1"].Draft.Run.CheckID != "invoice-check" {
		t.Fatal("response mutation altered stored evidence")
	}
}

func TestAssistantRejectsSpoofingAndBadSourceSpans(t *testing.T) {
	scope, snapshot, sources := baseline()
	cases := []struct {
		name string
		call ai.Event
	}{
		{"spoofed project", ai.Event{Type: ai.EventToolCall, ToolCall: &ai.ToolCall{ID: "1", Name: "submit_proposal", Arguments: json.RawMessage(`{"kind":"run-check","title":"x","project":"other"}`)}}},
		{"made-up source", toolCall("1", "submit_proposal", Draft{Kind: KindRun, Title: "x", Run: runDraft().Run, Sources: []Provenance{{SourceID: "other", Start: 0, End: 2, Confidence: .9}}})},
		{"false quote", toolCall("1", "submit_proposal", Draft{Kind: KindRun, Title: "x", Run: runDraft().Run, Sources: []Provenance{{SourceID: "ticket", Start: 0, End: 9, Quote: "invoice 8", Confidence: .9}}})},
		{"unknown check", toolCall("1", "submit_proposal", Draft{Kind: KindRun, Title: "x", Run: &RunDraft{CheckID: "invented", Environment: "local"}, Sources: runDraft().Sources})},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &memoryStore{}
			provider := &fakeProvider{steps: [][]ai.Event{{tc.call, {Type: ai.EventCompleted, StopReason: ai.StopReasonToolCalls}}, {{Type: ai.EventCompleted, StopReason: ai.StopReasonEnd}}}}
			response, err := (Assistant{Provider: provider, Store: store}).Run(context.Background(), Request{Scope: scope, Snapshot: snapshot, Text: "review", Sources: sources})
			if err != nil || len(response.Proposals) != 0 || len(store.items) != 0 {
				t.Fatalf("untrusted call accepted: %#v, %v", response, err)
			}
		})
	}
}

func TestAssistantBoundsAndStoreFailure(t *testing.T) {
	scope, snapshot, sources := baseline()
	request := Request{Scope: scope, Snapshot: snapshot, Text: "review", Sources: sources}
	valid := &fakeProvider{steps: [][]ai.Event{{{Type: ai.EventCompleted, StopReason: ai.StopReasonEnd}}}}
	for _, tc := range []struct {
		name string
		edit func(*Request)
	}{
		{"missing scope", func(r *Request) { r.Scope.Key = "" }},
		{"empty text", func(r *Request) { r.Text = " " }},
		{"oversize text", func(r *Request) { r.Text = strings.Repeat("a", maxText+1) }},
		{"invalid catalog", func(r *Request) { r.Snapshot.CatalogRevision = "" }},
		{"bad source", func(r *Request) { r.Sources[0].Text = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := request
			r.Sources = append([]Source(nil), sources...)
			tc.edit(&r)
			if _, err := (Assistant{Provider: valid, Store: &memoryStore{}}).Run(context.Background(), r); !errors.Is(err, ErrInvalid) {
				t.Fatalf("expected invalid: %v", err)
			}
		})
	}
	if _, err := (Assistant{Store: &memoryStore{}}).Run(context.Background(), request); !errors.Is(err, ErrInvalid) {
		t.Fatalf("nil provider accepted: %v", err)
	}
	if _, err := (Assistant{Provider: valid}).Run(context.Background(), request); !errors.Is(err, ErrInvalid) {
		t.Fatalf("nil store accepted: %v", err)
	}
	store := &memoryStore{err: errors.New("private store failed")}
	provider := &fakeProvider{steps: [][]ai.Event{{toolCall("1", "submit_proposal", runDraft()), {Type: ai.EventCompleted, StopReason: ai.StopReasonToolCalls}}}}
	if _, err := (Assistant{Provider: provider, Store: store}).Run(context.Background(), request); err == nil || len(store.items) != 0 {
		t.Fatalf("store failure must abort proposal: %v", err)
	}
}

func TestDiscoveryFiltersOnlyAuthorizedCatalogSignatures(t *testing.T) {
	_, snapshot, _ := baseline()
	for _, tc := range []struct {
		request discoveryRequest
		want    string
	}{
		{discoveryRequest{Kind: "query", InputNames: []string{"invoice_id"}}, "invoice-list"},
		{discoveryRequest{Kind: "scenario", InputNames: []string{"invoice_id"}}, "invoice-scenario"},
		{discoveryRequest{Kind: "check", InputNames: []string{"unknown"}}, ""},
	} {
		matches := discover(snapshot, tc.request)
		if tc.want == "" && len(matches) != 0 || tc.want != "" && (len(matches) != 1 || matches[0].ID != tc.want) {
			t.Fatalf("discovery leaked or missed resource: %#v", matches)
		}
	}
	snapshot.Checks = append(snapshot.Checks, Resource{ID: "qa-only", Title: "QA", Revision: "1", Environment: "qa"})
	if len(discover(snapshot, discoveryRequest{Kind: "check", Environment: "local"})) != 1 {
		t.Fatal("environment-specific resource leaked")
	}
	for i := 0; i < 25; i++ {
		snapshot.Checks = append(snapshot.Checks, Resource{ID: fmt.Sprintf("extra-%d", i), Revision: "1"})
	}
	if len(discover(snapshot, discoveryRequest{Kind: "check"})) != 20 {
		t.Fatal("discovery result cap not enforced")
	}
	for _, query := range []discoveryRequest{{Kind: "unknown"}, {Kind: "check", Environment: "missing"}, {Kind: "query", InputNames: []string{""}}} {
		if validDiscovery(query, snapshot) {
			t.Fatalf("invalid discovery accepted: %#v", query)
		}
	}
	if validDiscovery(discoveryRequest{Kind: "check", InputNames: make([]string, 65)}, snapshot) {
		t.Fatal("oversized discovery accepted")
	}
}

func TestAgentToolAndOutputBounds(t *testing.T) {
	scope, snapshot, sources := baseline()
	request := Request{Scope: scope, Snapshot: snapshot, Text: "review", Sources: sources}
	provider := &fakeProvider{steps: [][]ai.Event{{toolCall("bad-discovery", "discover", discoveryRequest{Kind: "unsupported"}), {Type: ai.EventCompleted, StopReason: ai.StopReasonToolCalls}}, {{Type: ai.EventCompleted, StopReason: ai.StopReasonEnd}}}}
	response, err := (Assistant{Provider: provider, Store: &memoryStore{}}).Run(context.Background(), request)
	if err != nil || len(response.Matches) != 0 {
		t.Fatalf("bad discovery escaped validation: %#v %v", response, err)
	}
	var calls []ai.Event
	for i := 0; i < 7; i++ {
		calls = append(calls, toolCall(fmt.Sprintf("proposal-%d", i), "submit_proposal", runDraft()))
	}
	calls = append(calls, ai.Event{Type: ai.EventCompleted, StopReason: ai.StopReasonToolCalls})
	provider = &fakeProvider{steps: [][]ai.Event{calls, {{Type: ai.EventCompleted, StopReason: ai.StopReasonEnd}}}}
	next := 0
	response, err = (Assistant{Provider: provider, Store: &memoryStore{}, NewID: func() (string, error) { next++; return fmt.Sprintf("p-%d", next), nil }}).Run(context.Background(), request)
	if err != nil || len(response.Proposals) != maxProposals || next != maxProposals {
		t.Fatalf("proposal bound broken: %d, %d, %v", len(response.Proposals), next, err)
	}
	for _, generate := range []func() (string, error){func() (string, error) { return "", errors.New("random source failed") }, func() (string, error) { return "", nil }, func() (string, error) { return "../escape", nil }} {
		provider = &fakeProvider{steps: [][]ai.Event{{toolCall("proposal", "submit_proposal", runDraft()), {Type: ai.EventCompleted, StopReason: ai.StopReasonToolCalls}}}}
		if _, err := (Assistant{Provider: provider, Store: &memoryStore{}, NewID: generate}).Run(context.Background(), request); err == nil {
			t.Fatal("failed proposal identity did not abort")
		}
	}
	generated, err := newID()
	if err != nil || !detective.ValidSlug(generated) {
		t.Fatalf("generated proposal ID is unsafe for the common store: %q, %v", generated, err)
	}
	provider = &fakeProvider{steps: [][]ai.Event{{{Type: ai.EventTextDelta, Text: strings.Repeat("a", 8*1024+1)}, {Type: ai.EventCompleted, StopReason: ai.StopReasonEnd}}}}
	if _, err := (Assistant{Provider: provider, Store: &memoryStore{}}).Run(context.Background(), request); !errors.Is(err, ErrInvalid) {
		t.Fatalf("oversize output accepted: %v", err)
	}
	provider = &fakeProvider{steps: [][]ai.Event{{}}, err: errors.New("provider unavailable")}
	if _, err := (Assistant{Provider: provider, Store: &memoryStore{}}).Run(context.Background(), request); err == nil {
		t.Fatal("provider failure swallowed")
	}
}

func TestInferenceRequestCapsCatalogSourcesAndExpectedCounts(t *testing.T) {
	scope, snapshot, sources := baseline()
	request := Request{Scope: scope, Snapshot: snapshot, Text: "review", Sources: sources}
	provider := &fakeProvider{steps: [][]ai.Event{{{Type: ai.EventCompleted, StopReason: ai.StopReasonEnd}}}}
	request.ExpectedCounts = map[string]int{"order_ids": 0}
	if _, err := (Assistant{Provider: provider, Store: &memoryStore{}}).Run(context.Background(), request); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid user-entered count accepted: %v", err)
	}
	request.ExpectedCounts = nil
	for i := 0; i < 199; i++ {
		request.Snapshot.Cases = append(request.Snapshot.Cases, Resource{ID: fmt.Sprintf("case-%d", i), Title: strings.Repeat("x", 256), Revision: "1"})
		request.Snapshot.Runs = append(request.Snapshot.Runs, Resource{ID: fmt.Sprintf("run-%d", i), Title: strings.Repeat("x", 256), Revision: "1"})
	}
	if _, err := (Assistant{Provider: provider, Store: &memoryStore{}}).Run(context.Background(), request); !errors.Is(err, ErrInvalid) {
		t.Fatalf("oversize authorized catalog accepted: %v", err)
	}
}
