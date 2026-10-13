package detective

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/investigation"
)

var lifecycleAt = time.Date(2026, 10, 13, 12, 0, 0, 0, time.UTC)

func TestCaseLifecyclePreservesEvidenceAndRecordsReviewableChanges(t *testing.T) {
	item := Case{ID: "case", Title: "Investigate", Context: map[string]InputValue{"invoice_id": {Scalar: ptrValue(investigation.NewIntegerValue("7"))}}, Evidence: []EvidenceRef{{ID: "crm", Kind: "external", Ref: "incident-7"}}, Findings: []Finding{{ID: "hypothesis", Text: "CRM mismatch", Status: "hypothesis", EvidenceRefs: []string{"crm"}}}}
	created, audit, err := CreateCase(item, "alice", lifecycleAt)
	if err != nil || created.Status != "active" || created.Version != 1 || audit.Action != "create-case" || len(created.Timeline) != 1 {
		t.Fatalf("create: %+v %+v %v", created, audit, err)
	}
	item.Context["invoice_id"] = InputValue{Scalar: ptrValue(investigation.NewIntegerValue("9"))}
	if created.Context["invoice_id"].Scalar.Str != "7" {
		t.Fatal("created Case aliased client context")
	}
	changed := created
	changed.Title = "Investigate invoice 7"
	changed.Context = map[string]InputValue{"invoice_id": {Scalar: ptrValue(investigation.NewIntegerValue("7"))}, "customer_id": {Scalar: ptrValue(investigation.NewIntegerValue("3"))}}
	changed.IncidentRefs = []string{"incident-7"}
	changed.Evidence = append(append([]EvidenceRef(nil), created.Evidence...), EvidenceRef{ID: "query", Kind: "query", Ref: "execution-7"})
	changed.Findings = []Finding{{ID: "hypothesis", Text: "Confirmed CRM mismatch", Status: "confirmed", EvidenceRefs: []string{"crm", "query"}}}
	updated, audit, err := UpdateCase(created, changed, "bob", lifecycleAt.Add(time.Minute))
	if err != nil || audit.Action != "update-case" || updated.Version != 2 || len(updated.Timeline) != 6 {
		t.Fatalf("Case change events missing: %+v %+v %v", updated, audit, err)
	}
	for _, kind := range []string{"context-updated", "findings-updated", "evidence-updated", "incident-links-updated", "title-updated"} {
		seen := false
		for _, event := range updated.Timeline {
			seen = seen || event.Type == kind
		}
		if !seen {
			t.Fatalf("missing %s event: %+v", kind, updated.Timeline)
		}
	}
	closedInput := updated
	closedInput.Status = "closed"
	closedInput.ClosureReason = "Reviewed mismatch and linked incident"
	closed, _, err := UpdateCase(updated, closedInput, "alice", lifecycleAt.Add(2*time.Minute))
	if err != nil || closed.Timeline[len(closed.Timeline)-1].Type != "closed" {
		t.Fatalf("closure not persisted: %+v %v", closed, err)
	}
	reopenedInput := closed
	reopenedInput.Status = "active"
	reopenedInput.ClosureReason = ""
	reopened, _, err := UpdateCase(closed, reopenedInput, "alice", lifecycleAt.Add(3*time.Minute))
	if err != nil || reopened.Timeline[len(reopened.Timeline)-1].Type != "reopened" || reopened.ClosureReason != closed.ClosureReason {
		t.Fatalf("reopening lost reason/history: %+v %v", reopened, err)
	}
	closedInput = reopened
	closedInput.Status = "closed"
	closedInput.ClosureReason = "Final review complete"
	closed, _, err = UpdateCase(reopened, closedInput, "alice", lifecycleAt.Add(4*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	archivedInput := closed
	archivedInput.Status = "archived"
	archived, _, err := UpdateCase(closed, archivedInput, "alice", lifecycleAt.Add(5*time.Minute))
	if err != nil || archived.Status != "archived" {
		t.Fatalf("archive: %+v %v", archived, err)
	}
	archivedInput = archived
	archivedInput.Title = "forged"
	if _, _, err := UpdateCase(archived, archivedInput, "alice", lifecycleAt.Add(6*time.Minute)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("archived Case edited: %v", err)
	}
}

func TestCaseMutationsRejectForgedHistoryAndDanglingEvidence(t *testing.T) {
	base, _, err := CreateCase(Case{ID: "case", Title: "Case"}, "alice", lifecycleAt)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []Case{
		{ID: "Bad", Title: "Case"}, {ID: "case", Title: " "}, {ID: "case", Title: "Case", Version: 2},
		{ID: "case", Title: "Case", Context: map[string]InputValue{"bad/name": {Scalar: ptrValue(investigation.NewStringValue("value"))}}},
		{ID: "case", Title: "Case", Evidence: []EvidenceRef{{ID: "one", Kind: "unknown", Ref: "x"}}},
		{ID: "case", Title: "Case", Evidence: []EvidenceRef{{ID: "one", Kind: "external", Ref: ""}}},
		{ID: "case", Title: "Case", Findings: []Finding{{ID: "finding", Text: "Missing", Status: "confirmed", EvidenceRefs: []string{"missing"}}}},
		{ID: "case", Title: "Case", IncidentRefs: []string{"bad\nref"}},
	} {
		if _, _, err := CreateCase(item, "alice", lifecycleAt); err == nil {
			t.Fatalf("invalid Case accepted: %+v", item)
		}
	}
	if _, _, err := CreateCase(Case{ID: "case", Title: "Case"}, "", lifecycleAt); err == nil {
		t.Fatal("empty actor accepted")
	}
	for _, tc := range []struct {
		name string
		edit func(*Case)
		want error
	}{
		{"stale version", func(c *Case) { c.Version++ }, ErrConflict},
		{"id change", func(c *Case) { c.ID = "other" }, ErrConflict},
		{"missing closure reason", func(c *Case) { c.Status = "closed" }, ErrInvalid},
		{"invalid status", func(c *Case) { c.Status = "unknown" }, ErrInvalid},
		{"dangling finding", func(c *Case) {
			c.Findings = []Finding{{ID: "finding", Text: "Finding", Status: "confirmed", EvidenceRefs: []string{"missing"}}}
		}, ErrInvalid},
	} {
		next := base
		tc.edit(&next)
		if _, _, err := UpdateCase(base, next, "alice", lifecycleAt.Add(time.Minute)); !errors.Is(err, tc.want) {
			t.Fatalf("%s: %v", tc.name, err)
		}
	}
	if ValidTransition("unknown", "active", "") || !ValidTransition("active", "closed", "reason") || ValidTransition("active", "closed", " ") || !ValidTransition("closed", "archived", "") {
		t.Fatal("Case transition policy changed")
	}
	for _, event := range []CaseEvent{{Type: "closed"}, {Type: "note", Message: " "}, {Type: "note", Message: "ok", Ref: "forged"}, {Type: "note", Message: strings.Repeat("x", 4097)}, {Type: "link-evidence", Ref: "missing"}} {
		if _, _, err := AppendCaseEvent(base, event, "event", nil, "alice", lifecycleAt); !errors.Is(err, ErrInvalid) {
			t.Fatalf("forged event accepted: %+v: %v", event, err)
		}
	}
	linked, _, err := AppendCaseEvent(base, CaseEvent{Type: "link-evidence", Ref: "run"}, "event", &CheckRun{ID: "run", CaseID: "case"}, "alice", lifecycleAt)
	if err != nil || len(linked.RunRefs) != 1 || len(linked.Evidence) != 1 {
		t.Fatalf("valid existing evidence link failed: %+v %v", linked, err)
	}
	if _, _, err := AppendCaseEvent(linked, CaseEvent{Type: "link-evidence", Ref: "run"}, "next", &CheckRun{ID: "run", CaseID: "case"}, "alice", lifecycleAt); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate linked run accepted: %v", err)
	}
	if _, _, err := AppendCaseEvent(linked, CaseEvent{Type: "note", Message: "Reviewed"}, "event", nil, "alice", lifecycleAt); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate event ID accepted: %v", err)
	}
	if _, _, err := AttachRun(base, CheckRun{ID: "run", CaseID: "wrong"}, "event", "alice", lifecycleAt); !errors.Is(err, ErrInvalid) {
		t.Fatalf("wrong-Case run linked: %v", err)
	}
	attached, _, err := AttachRun(base, CheckRun{ID: "run", CaseID: "case"}, "run-event", "alice", lifecycleAt)
	if err != nil || len(attached.RunRefs) != 1 {
		t.Fatalf("attach: %+v %v", attached, err)
	}
	if _, _, err := AttachRun(attached, CheckRun{ID: "run", CaseID: "case"}, "another-event", "alice", lifecycleAt); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate run attached: %v", err)
	}
	if _, _, err := AttachRun(attached, CheckRun{ID: "other", CaseID: "case"}, "run-event", "alice", lifecycleAt); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate event ID attached: %v", err)
	}
}

func TestScenarioManualProgressAndReceiptIdentity(t *testing.T) {
	c, _, err := CreateCase(Case{ID: "case", Title: "Case"}, "alice", lifecycleAt)
	if err != nil {
		t.Fatal(err)
	}
	scenario := Scenario{ID: "guide", Title: "Guide", Inputs: []Input{{Name: "invoice_id", Type: investigation.ValueTypeInteger, Cardinality: "scalar"}}, Steps: []ScenarioStep{{ID: "inspect", Kind: "query", Ref: "queries/invoice"}, {ID: "verify", Kind: "check", Ref: "invoice-check"}}}
	if err := ValidateScenario(scenario); err != nil {
		t.Fatal(err)
	}
	applied, audit, err := ApplyScenario(c, scenario, "application", "alice", lifecycleAt)
	if err != nil || audit.Action != "scenario-applied" || applied.ScenarioApplications[0].Steps[0].Status != "pending" {
		t.Fatalf("scenario application: %+v %v", applied, err)
	}
	if _, _, err := ApplyScenario(applied, scenario, "application", "alice", lifecycleAt); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate application accepted: %v", err)
	}
	second, _, err := ApplyScenario(applied, scenario, "application-two", "alice", lifecycleAt)
	if err != nil || len(second.ScenarioRefs) != 1 {
		t.Fatalf("scenario ref duplicated: %+v %v", second.ScenarioRefs, err)
	}
	if _, _, err := RecordScenarioStep(applied, "application", "missing", "skipped", "", nil, false, "alice", lifecycleAt); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown step accepted: %v", err)
	}
	if _, _, err := RecordScenarioStep(applied, "application", "verify", "skipped", "", nil, false, "alice", lifecycleAt); !errors.Is(err, ErrInvalid) {
		t.Fatalf("out-of-order step accepted: %v", err)
	}
	progress, _, err := RecordScenarioStep(applied, "application", "inspect", "skipped", "", nil, false, "alice", lifecycleAt)
	if err != nil || progress.ScenarioApplications[0].Steps[0].EvidenceOutcome != "unknown" {
		t.Fatalf("skipped step claimed positive evidence: %+v %v", progress, err)
	}
	if _, _, err := RecordScenarioStep(progress, "application", "inspect", "skipped", "", nil, false, "alice", lifecycleAt); !errors.Is(err, ErrConflict) {
		t.Fatalf("completed step overwritten: %v", err)
	}
	run := CheckRun{ID: "run", Kind: "check", CheckID: "invoice-check", CaseID: "case", ExecutionStatus: "completed", Outcome: "fail", Environment: "qa"}
	if _, _, err := RecordScenarioStep(progress, "application", "verify", "completed", "run", &run, false, "alice", lifecycleAt); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unlinked run accepted: %v", err)
	}
	finished, _, err := RecordScenarioStep(progress, "application", "verify", "completed", "run", &run, true, "alice", lifecycleAt)
	if err != nil || finished.ScenarioApplications[0].Steps[1].EvidenceOutcome != "fail" {
		t.Fatalf("failing Check could not complete manual step: %+v %v", finished, err)
	}
	proof := ScenarioQueryProof{Execution: apicontract.ExecutionRef{ProjectID: "p", StoreID: "evidence", ExecutionID: "receipt"}, QueryID: "queries/invoice", Environment: "qa", ExecutionStatus: "completed"}
	ordinary, _, err := RecordScenarioQueryStep(applied, "application", "inspect", proof, "alice", lifecycleAt)
	if err != nil || ordinary.ScenarioApplications[0].Steps[0].EvidenceOutcome != "unknown" || ordinary.ScenarioApplications[0].Steps[0].Execution.ExecutionID != "receipt" {
		t.Fatalf("ordinary query evidence incorrectly scored: %+v %v", ordinary, err)
	}
	proof.QueryID = "other"
	if _, _, err := RecordScenarioQueryStep(applied, "application", "inspect", proof, "alice", lifecycleAt); !errors.Is(err, ErrInvalid) {
		t.Fatalf("wrong query receipt accepted: %v", err)
	}
	for _, invalid := range []Scenario{
		{ID: "Bad", Title: "Guide", Steps: scenario.Steps},
		{ID: "guide", Title: " ", Steps: scenario.Steps},
		{ID: "guide", Title: "Guide"},
		{ID: "guide", Title: "Guide", Inputs: []Input{{Name: "bad name", Type: investigation.ValueTypeInteger, Cardinality: "scalar"}}, Steps: scenario.Steps},
		{ID: "guide", Title: "Guide", Inputs: []Input{scenario.Inputs[0], scenario.Inputs[0]}, Steps: scenario.Steps},
		{ID: "guide", Title: "Guide", Steps: []ScenarioStep{{ID: "x", Kind: "query", Ref: "../escape"}}},
		{ID: "guide", Title: "Guide", Steps: []ScenarioStep{{ID: "x", Kind: "query", Ref: "q"}, {ID: "x", Kind: "check", Ref: "check"}}},
	} {
		if err := ValidateScenario(invalid); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid Scenario accepted: %+v %v", invalid, err)
		}
	}
}

func TestAuditRecursivelySummarizesExecutionMetadataWithoutRows(t *testing.T) {
	run := CheckRun{ID: "run", Kind: "check", CheckID: "root", CaseID: "case", Environment: "qa", Actor: "alice", Outcome: "fail", StartedAt: lifecycleAt, Steps: []StepResult{{StepID: "nested", Kind: "check", Children: []StepResult{{StepID: "query", Kind: "query", QueryID: "invoice", Source: "db", RowCount: 3, ErrorCode: "SOURCE_UNAVAILABLE", QueryRevision: "revision", ResultFingerprint: "fingerprint", Execution: &apicontract.ExecutionRef{ExecutionID: "receipt"}}}}}}
	event := AuditForRun(run)
	if event.RowCount != 3 || event.QueryID != "invoice" || event.ErrorCode != "SOURCE_UNAVAILABLE" || event.QueryRevision != "revision" || event.ResultFingerprint != "fingerprint" || event.ExecutionID != "receipt" || event.CaseID != "case" {
		t.Fatalf("nested audit metadata lost: %+v", event)
	}
	if strings.Contains(event.Action, "row") || strings.Contains(event.Action, "parameter") {
		t.Fatalf("audit exposed data: %+v", event)
	}
}

func TestLifecycleRejectsMalformedStateAtEveryMutationBoundary(t *testing.T) {
	base, _, err := CreateCase(Case{ID: "case", Title: "Case"}, "alice", lifecycleAt)
	if err != nil {
		t.Fatal(err)
	}
	corrupt := base
	corrupt.Context = map[string]InputValue{"bad": {}}
	validNext := base
	validNext.Title = "Updated"
	if _, _, err := UpdateCase(corrupt, validNext, "alice", lifecycleAt); !errors.Is(err, ErrInvalid) {
		t.Fatalf("corrupt stored Case updated: %v", err)
	}
	if _, _, err := UpdateCase(base, corrupt, "alice", lifecycleAt); !errors.Is(err, ErrInvalid) {
		t.Fatalf("corrupt submitted Case updated: %v", err)
	}
	closed := base
	closed.Status, closed.ClosureReason = "closed", "old reason"
	closedNext := closed
	closedNext.Version = base.Version
	if _, _, err := UpdateCase(closed, closedNext, "alice", lifecycleAt); err != nil {
		t.Fatalf("stable closed Case could not be edited: %v", err)
	}
	scenario := Scenario{ID: "guide", Title: "Guide", Steps: []ScenarioStep{{ID: "query", Kind: "query", Ref: "invoice"}}}
	if _, _, err := ApplyScenario(corrupt, scenario, "application", "alice", lifecycleAt); !errors.Is(err, ErrInvalid) {
		t.Fatalf("corrupt Case accepted for Scenario: %v", err)
	}
	if _, _, err := ApplyScenario(base, Scenario{ID: "bad"}, "application", "alice", lifecycleAt); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid Scenario applied: %v", err)
	}
	if _, _, err := RecordScenarioStep(corrupt, "application", "query", "skipped", "", nil, false, "alice", lifecycleAt); !errors.Is(err, ErrInvalid) {
		t.Fatalf("corrupt Case progress mutated: %v", err)
	}
	if _, _, err := AppendCaseEvent(corrupt, CaseEvent{Type: "note", Message: "note"}, "event", nil, "alice", lifecycleAt); !errors.Is(err, ErrInvalid) {
		t.Fatalf("corrupt Case timeline mutated: %v", err)
	}
	if _, _, err := AttachRun(corrupt, CheckRun{ID: "run", CaseID: "case"}, "event", "alice", lifecycleAt); !errors.Is(err, ErrInvalid) {
		t.Fatalf("corrupt Case linked to run: %v", err)
	}
	applied, _, err := ApplyScenario(base, scenario, "application", "alice", lifecycleAt)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		status string
		runID  string
		run    *CheckRun
	}{
		{"unknown", "", nil},
		{"completed", "", nil},
		{"completed", "run", &CheckRun{ID: "run", Kind: "check", CheckID: "wrong", CaseID: "case", ExecutionStatus: "completed", Outcome: "pass"}},
		{"skipped", "run", nil},
	} {
		if _, _, err := RecordScenarioStep(applied, "application", "query", tc.status, tc.runID, tc.run, true, "alice", lifecycleAt); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid progress accepted: %+v %v", tc, err)
		}
	}
	if err := ValidatePublishedValidator(PublishedValidator{ID: "Bad", Title: "Bad"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid published Validator accepted: %v", err)
	}
	if err := ValidateQueryValidators(QueryValidators{QueryID: "invoice"}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("empty query Validator catalog accepted: %v", err)
	}
}

func TestCaseEvidenceIntegrityAndCapacity(t *testing.T) {
	caseItem := Case{ID: "case", Title: "Case"}
	if err := validateCaseContent(caseItem); err != nil {
		t.Fatalf("empty Case unexpectedly invalid: %v", err)
	}
	caseItem.Evidence = []EvidenceRef{{ID: "proof", Kind: "external", Ref: "external-1"}}
	caseItem.Findings = []Finding{{ID: "finding", Text: "Observed", Status: "confirmed", EvidenceRefs: []string{"proof", "proof"}}}
	if err := validateCaseContent(caseItem); !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate finding evidence accepted: %v", err)
	}
	caseItem.Findings[0].EvidenceRefs = []string{"proof"}
	caseItem.Findings = append(caseItem.Findings, caseItem.Findings[0])
	if err := validateCaseContent(caseItem); !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate finding ID accepted: %v", err)
	}
	caseItem.Findings = nil
	caseItem.Evidence = make([]EvidenceRef, 201)
	for i := range caseItem.Evidence {
		caseItem.Evidence[i] = EvidenceRef{ID: "proof-" + strconv.Itoa(i), Kind: "external", Ref: "source"}
	}
	if err := validateCaseContent(caseItem); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unbounded external evidence accepted: %v", err)
	}
	caseItem.Evidence = nil
	caseItem.Findings = make([]Finding, 201)
	for i := range caseItem.Findings {
		caseItem.Findings[i] = Finding{ID: "finding-" + strconv.Itoa(i), Text: "Reviewed", Status: "hypothesis"}
	}
	if err := validateCaseContent(caseItem); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unbounded findings accepted: %v", err)
	}
}

func TestCaseRejectsRepeatedClosureReasonAndEvidenceIDCollision(t *testing.T) {
	base, _, err := CreateCase(Case{ID: "case", Title: "Case", Evidence: []EvidenceRef{{ID: "evidence-run", Kind: "external", Ref: "external"}}}, "alice", lifecycleAt)
	if err != nil {
		t.Fatal(err)
	}
	activeWithOldClosure := base
	activeWithOldClosure.ClosureReason = "previous resolution"
	closed := activeWithOldClosure
	closed.Status = "closed"
	if _, _, err := UpdateCase(activeWithOldClosure, closed, "alice", lifecycleAt); !errors.Is(err, ErrInvalid) {
		t.Fatalf("closed without a new reason: %v", err)
	}
	if _, _, err := AppendCaseEvent(base, CaseEvent{Type: "link-evidence", Ref: "run"}, "event", &CheckRun{ID: "run", CaseID: "case"}, "alice", lifecycleAt); !errors.Is(err, ErrInvalid) {
		t.Fatalf("duplicate evidence ID added to Case: %v", err)
	}
}
