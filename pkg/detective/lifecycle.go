package detective

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/datatug/datatug-core/pkg/datatug"
)

// AuditEvent deliberately carries metadata only. No parameter values, source
// rows, Case context, findings or free-text notes belong in a generic audit.
type AuditEvent struct {
	ID                string    `json:"id"`
	At                time.Time `json:"at"`
	Actor             string    `json:"actor"`
	Project           string    `json:"project"`
	Action            string    `json:"action"`
	CaseID            string    `json:"caseId,omitempty"`
	CheckID           string    `json:"checkId,omitempty"`
	QueryID           string    `json:"queryId,omitempty"`
	QueryRevision     string    `json:"queryRevision,omitempty"`
	ResultFingerprint string    `json:"resultFingerprint,omitempty"`
	ExecutionID       string    `json:"executionId,omitempty"`
	Source            string    `json:"source,omitempty"`
	ErrorCode         string    `json:"errorCode,omitempty"`
	RunID             string    `json:"runId,omitempty"`
	Environment       string    `json:"environment,omitempty"`
	Outcome           string    `json:"outcome,omitempty"`
	RowCount          int       `json:"rowCount,omitempty"`
	DurationMs        int64     `json:"durationMs,omitempty"`
}

// CreateCase strips server-owned history and starts a versioned active Case.
// Capacity and uniqueness are repository checks inside its transaction.
func CreateCase(item Case, actor string, at time.Time) (Case, AuditEvent, error) {
	if item.WorkItemID != "" || !validSlug(item.ID) || strings.TrimSpace(item.Title) == "" || item.Version != 0 || actor == "" || at.IsZero() {
		return Case{}, AuditEvent{}, ErrInvalid
	}
	var err error
	item, err = cloneCase(item)
	if err != nil {
		return Case{}, AuditEvent{}, err
	}
	item.Status, item.Version = "active", 1
	if item.Context == nil {
		item.Context = map[string]InputValue{}
	}
	item.RunRefs, item.ScenarioRefs, item.ScenarioApplications = nil, nil, nil
	initial := make([]EvidenceRef, 0, len(item.Evidence))
	for _, evidence := range item.Evidence {
		if evidence.Kind != "check-run" {
			initial = append(initial, evidence)
		}
	}
	item.Evidence = initial
	item.CreatedAt, item.UpdatedAt = at, at
	item.TimelineOffset = 0
	item.Timeline = []CaseEvent{{ID: "created", At: at, Actor: actor, Type: "created"}}
	if err := validateCaseContent(item); err != nil {
		return Case{}, AuditEvent{}, err
	}
	return item, AuditEvent{ID: fmt.Sprintf("case-%s-%d", item.ID, item.Version), At: at, Actor: actor, Action: "create-case", CaseID: item.ID}, nil
}

// UpdateCase applies a client edit while preserving run links, scenario
// applications, and server-issued lifecycle history from the stored version.
func UpdateCase(old, next Case, actor string, at time.Time) (Case, AuditEvent, error) {
	if old.WorkItemID != next.WorkItemID || !validSlug(next.ID) || old.ID != next.ID || next.Version != old.Version || strings.TrimSpace(next.Title) == "" || actor == "" || at.IsZero() {
		return Case{}, AuditEvent{}, ErrConflict
	}
	if !validTransition(old.Status, next.Status, next.ClosureReason) {
		return Case{}, AuditEvent{}, ErrInvalid
	}
	if old.Status == "archived" {
		return Case{}, AuditEvent{}, ErrInvalid
	}
	if old.Status == "active" && next.Status == "closed" && old.ClosureReason != "" && strings.TrimSpace(next.ClosureReason) == strings.TrimSpace(old.ClosureReason) {
		return Case{}, AuditEvent{}, ErrInvalid
	}
	var err error
	old, err = cloneCase(old)
	if err != nil {
		return Case{}, AuditEvent{}, err
	}
	next, err = cloneCase(next)
	if err != nil {
		return Case{}, AuditEvent{}, err
	}
	next.Timeline = append([]CaseEvent(nil), old.Timeline...)
	next.TimelineOffset = old.TimelineOffset
	next.ScenarioApplications = append([]ScenarioApplication(nil), old.ScenarioApplications...)
	next.ScenarioRefs = append([]string(nil), old.ScenarioRefs...)
	next.RunRefs = append([]string(nil), old.RunRefs...)
	userEvidence := make([]EvidenceRef, 0, len(next.Evidence)+len(old.Evidence))
	for _, item := range next.Evidence {
		if item.Kind != "check-run" {
			userEvidence = append(userEvidence, item)
		}
	}
	for _, item := range old.Evidence {
		if item.Kind == "check-run" {
			userEvidence = append(userEvidence, item)
		}
	}
	next.Evidence = userEvidence
	next.CreatedAt, next.UpdatedAt = old.CreatedAt, at
	next.Version++
	if old.Status == "closed" && next.Status == "active" && next.ClosureReason == "" {
		next.ClosureReason = old.ClosureReason
	}
	if old.Status != next.Status {
		kind := next.Status
		if old.Status == "closed" && next.Status == "active" {
			kind = "reopened"
		}
		next.Timeline = append(next.Timeline, CaseEvent{ID: fmt.Sprintf("lifecycle-%d", next.Version), At: at, Actor: actor, Type: kind, Message: next.ClosureReason})
	}
	changes := []struct {
		changed bool
		kind    string
	}{{!reflect.DeepEqual(old.Context, next.Context), "context-updated"}, {!reflect.DeepEqual(old.Findings, next.Findings), "findings-updated"}, {!reflect.DeepEqual(old.Evidence, next.Evidence), "evidence-updated"}, {!reflect.DeepEqual(old.IncidentRefs, next.IncidentRefs), "incident-links-updated"}, {!reflect.DeepEqual(old.ScenarioRefs, next.ScenarioRefs), "scenario-links-updated"}, {old.Title != next.Title, "title-updated"}}
	for _, change := range changes {
		if change.changed {
			next.Timeline = append(next.Timeline, CaseEvent{ID: fmt.Sprintf("%s-%d", change.kind, next.Version), At: at, Actor: actor, Type: change.kind})
		}
	}
	if err := validateCaseContent(next); err != nil {
		return Case{}, AuditEvent{}, err
	}
	return next, AuditEvent{ID: fmt.Sprintf("case-%s-%d", next.ID, next.Version), At: at, Actor: actor, Action: "update-case", CaseID: next.ID}, nil
}

// ApplyScenario snapshots reusable manual guidance; no step runs automatically.
func ApplyScenario(c Case, scenario Scenario, applicationID, actor string, at time.Time) (Case, AuditEvent, error) {
	if c.Status != "active" || !validSlug(applicationID) || actor == "" || at.IsZero() || len(c.ScenarioApplications) >= 100 || ValidateScenario(scenario) != nil {
		return Case{}, AuditEvent{}, ErrInvalid
	}
	for _, prior := range c.ScenarioApplications {
		if prior.ID == applicationID {
			return Case{}, AuditEvent{}, ErrConflict
		}
	}
	var err error
	c, err = cloneCase(c)
	if err != nil {
		return Case{}, AuditEvent{}, err
	}
	// Copy slices before the application retains its authoring snapshot.
	scenario.Inputs = append([]Input(nil), scenario.Inputs...)
	scenario.Steps = append([]ScenarioStep(nil), scenario.Steps...)
	for i := range scenario.Steps {
		scenario.Steps[i].Bindings = append([]Binding(nil), scenario.Steps[i].Bindings...)
	}
	application := ScenarioApplication{ID: applicationID, ScenarioID: scenario.ID, ScenarioVersion: scenario.Version, Snapshot: scenario, AppliedAt: at, AppliedBy: actor}
	for _, step := range scenario.Steps {
		application.Steps = append(application.Steps, ScenarioStepReceipt{StepID: step.ID, Status: "pending"})
	}
	c.ScenarioApplications = append(c.ScenarioApplications, application)
	linked := false
	for _, ref := range c.ScenarioRefs {
		if ref == scenario.ID {
			linked = true
			break
		}
	}
	if !linked {
		c.ScenarioRefs = append(c.ScenarioRefs, scenario.ID)
	}
	c.Timeline = append(c.Timeline, CaseEvent{ID: "event-" + applicationID, At: at, Actor: actor, Type: "scenario-applied", Ref: applicationID})
	c.Version++
	c.UpdatedAt = at
	return c, AuditEvent{ID: applicationID, At: at, Actor: actor, Action: "scenario-applied", CaseID: c.ID}, nil
}

// RecordScenarioStep records one reviewed step in application order. A
// completed step must cite a persisted, Case-linked run of the same asset.
// runLinked is established by the adapter's direct run-link record, since
// Case.RunRefs may be a bounded recent projection in hosted storage.
func RecordScenarioStep(c Case, applicationID, stepID, status, runID string, run *CheckRun, runLinked bool, actor string, at time.Time) (Case, AuditEvent, error) {
	return recordScenarioStep(c, applicationID, stepID, status, runID, run, runLinked, nil, actor, at)
}

// RecordScenarioQueryStep marks an ordinary saved-query guidance step reviewed
// using a terminal, authorized receipt. No Validator pass is inferred.
func RecordScenarioQueryStep(c Case, applicationID, stepID string, proof ScenarioQueryProof, actor string, at time.Time) (Case, AuditEvent, error) {
	return recordScenarioStep(c, applicationID, stepID, "completed", "", nil, false, &proof, actor, at)
}

func recordScenarioStep(c Case, applicationID, stepID, status, runID string, run *CheckRun, runLinked bool, proof *ScenarioQueryProof, actor string, at time.Time) (Case, AuditEvent, error) {
	if c.Status != "active" || !validSlug(applicationID) || !validSlug(stepID) || !oneOf(status, "completed", "skipped") || actor == "" || at.IsZero() {
		return Case{}, AuditEvent{}, ErrInvalid
	}
	if status == "completed" && proof == nil && (!validSlug(runID) || run == nil) || status == "completed" && proof != nil && (runID != "" || run != nil) || status == "skipped" && (runID != "" || run != nil || proof != nil) {
		return Case{}, AuditEvent{}, ErrInvalid
	}
	var err error
	c, err = cloneCase(c)
	if err != nil {
		return Case{}, AuditEvent{}, err
	}
	applicationIndex, stepIndex := -1, -1
	for i, application := range c.ScenarioApplications {
		if application.ID == applicationID {
			applicationIndex = i
			for j, step := range application.Steps {
				if step.StepID == stepID {
					stepIndex = j
					break
				}
			}
			break
		}
	}
	if applicationIndex < 0 || stepIndex < 0 {
		return Case{}, AuditEvent{}, ErrNotFound
	}
	application := &c.ScenarioApplications[applicationIndex]
	if application.Steps[stepIndex].Status != "pending" {
		return Case{}, AuditEvent{}, ErrConflict
	}
	for _, prior := range application.Steps[:stepIndex] {
		if prior.Status == "pending" {
			return Case{}, AuditEvent{}, ErrInvalid
		}
	}
	if status == "completed" {
		step := application.Snapshot.Steps[stepIndex]
		if proof == nil {
			if !runLinked || run.ID != runID || run.CaseID != c.ID || run.ExecutionStatus != "completed" || !oneOf(run.Outcome, "pass", "fail", "unknown") {
				return Case{}, AuditEvent{}, ErrInvalid
			}
			if step.Kind == "check" && (run.Kind != "check" || run.CheckID != step.Ref) || step.Kind == "query" && (run.Kind != "query-validators" || run.QueryID != step.Ref) {
				return Case{}, AuditEvent{}, ErrInvalid
			}
		} else if step.Kind != "query" || proof.QueryID != step.Ref || proof.Environment == "" || proof.ExecutionStatus != "completed" || proof.Execution.Validate() != nil {
			return Case{}, AuditEvent{}, ErrInvalid
		}
	}
	receipt := &application.Steps[stepIndex]
	receipt.Status, receipt.RunID, receipt.At, receipt.Actor = status, runID, at, actor
	if status == "completed" {
		if proof != nil {
			ref := proof.Execution
			receipt.Execution = &ref
			receipt.Environment = proof.Environment
			receipt.EvidenceOutcome = "unknown"
		} else {
			receipt.Environment = run.Environment
			receipt.EvidenceOutcome = run.Outcome
		}
	} else {
		receipt.EvidenceOutcome = "unknown"
	}
	c.Timeline = append(c.Timeline, CaseEvent{ID: fmt.Sprintf("scenario-step-%s-%d", applicationID, c.Version+1), At: at, Actor: actor, Type: "scenario-step-" + status, Ref: applicationID, Message: stepID})
	c.Version++
	c.UpdatedAt = at
	queryID := ""
	if proof != nil {
		queryID = proof.QueryID
	}
	return c, AuditEvent{ID: fmt.Sprintf("scenario-step-%s-%d", applicationID, c.Version), At: at, Actor: actor, Action: "scenario-step-" + status, CaseID: c.ID, RunID: runID, QueryID: queryID}, nil
}

// AppendCaseEvent accepts only client notes and links to existing run evidence.
// The repository supplies the verified run and server-issued event ID.
func AppendCaseEvent(c Case, event CaseEvent, eventID string, run *CheckRun, actor string, at time.Time) (Case, AuditEvent, error) {
	if c.Status == "archived" || !oneOf(event.Type, "note", "link-evidence") || !validSlug(eventID) || actor == "" || at.IsZero() {
		return Case{}, AuditEvent{}, ErrInvalid
	}
	if event.Type == "note" {
		if strings.TrimSpace(event.Message) == "" || len(event.Message) > 4096 || event.Ref != "" || run != nil {
			return Case{}, AuditEvent{}, ErrInvalid
		}
	} else if !validSlug(event.Ref) || run == nil || run.ID != event.Ref {
		return Case{}, AuditEvent{}, ErrInvalid
	}
	var err error
	c, err = cloneCase(c)
	if err != nil {
		return Case{}, AuditEvent{}, err
	}
	for _, prior := range c.Timeline {
		if prior.ID == eventID {
			return Case{}, AuditEvent{}, ErrConflict
		}
	}
	event.ID, event.At, event.Actor = eventID, at, actor
	c.Timeline = append(c.Timeline, event)
	if event.Type == "link-evidence" {
		for _, ref := range c.RunRefs {
			if ref == event.Ref {
				return Case{}, AuditEvent{}, ErrConflict
			}
		}
		c.RunRefs = append(c.RunRefs, event.Ref)
		c.Evidence = append(c.Evidence, EvidenceRef{ID: "evidence-" + event.Ref, Kind: "check-run", Ref: event.Ref})
	}
	if err := validateCaseContent(c); err != nil {
		return Case{}, AuditEvent{}, err
	}
	c.Version++
	c.UpdatedAt = at
	return c, AuditEvent{ID: eventID, At: at, Actor: actor, Action: event.Type, CaseID: c.ID}, nil
}

// AttachRun is server-only and requires an execution bound to this Case.
func AttachRun(c Case, run CheckRun, eventID, actor string, at time.Time) (Case, AuditEvent, error) {
	if c.Status == "archived" || run.CaseID != c.ID || !validSlug(run.ID) || !validSlug(eventID) || actor == "" || at.IsZero() {
		return Case{}, AuditEvent{}, ErrInvalid
	}
	var err error
	c, err = cloneCase(c)
	if err != nil {
		return Case{}, AuditEvent{}, err
	}
	for _, prior := range c.Timeline {
		if prior.ID == eventID {
			return Case{}, AuditEvent{}, ErrConflict
		}
	}
	for _, ref := range c.RunRefs {
		if ref == run.ID {
			return Case{}, AuditEvent{}, ErrConflict
		}
	}
	c.Timeline = append(c.Timeline, CaseEvent{ID: eventID, At: at, Actor: actor, Type: "check-run", Ref: run.ID})
	c.RunRefs = append(c.RunRefs, run.ID)
	c.Evidence = append(c.Evidence, EvidenceRef{ID: "evidence-" + run.ID, Kind: "check-run", Ref: run.ID})
	if err := validateCaseContent(c); err != nil {
		return Case{}, AuditEvent{}, err
	}
	c.Version++
	c.UpdatedAt = at
	return c, AuditEvent{ID: eventID, At: at, Actor: actor, Action: "check-run", CaseID: c.ID, RunID: run.ID}, nil
}

// ValidateScenario checks query range bindings per step. Scenario inputs may
// be consumed by query steps or by the Check steps they configure.
func ValidateScenario(scenario Scenario) error {
	if !validSlug(scenario.ID) || strings.TrimSpace(scenario.Title) == "" || len(scenario.Steps) == 0 || len(scenario.Steps) > 32 || len(scenario.Inputs) > 64 || datatug.ValidateOptionalCreatedAt(scenario.CreatedAt) != nil {
		return ErrInvalid
	}
	inputNames := map[string]bool{}
	inputs := make(map[string]Input, len(scenario.Inputs))
	for _, input := range scenario.Inputs {
		if !validParameterName(input.Name) || inputNames[input.Name] || !validInputType(input.Type) || !oneOf(input.Cardinality, "scalar", "set", "range") {
			return ErrInvalid
		}
		if input.Cardinality == "range" && !validRangeInputType(input.Type) {
			return ErrInvalid
		}
		inputNames[input.Name] = true
		inputs[input.Name] = input
	}
	stepIDs := map[string]bool{}
	for _, step := range scenario.Steps {
		if !validSlug(step.ID) || stepIDs[step.ID] || !oneOf(step.Kind, "query", "check") || step.Ref == "" || step.Kind == "check" && !validSlug(step.Ref) || step.Kind == "query" && !validQueryRef(step.Ref) {
			return ErrInvalid
		}
		stepIDs[step.ID] = true
		if step.Kind == "check" {
			if len(step.Bindings) != 0 {
				return ErrInvalid
			}
			continue
		}
		if err := validateScenarioQueryBindings(step.Bindings, inputs); err != nil {
			return err
		}
	}
	return nil
}

func validateScenarioQueryBindings(bindings []Binding, inputs map[string]Input) error {
	if validateBindings(bindings) != nil {
		return ErrInvalid
	}
	rangeParts := make(map[string]map[string]bool)
	for _, binding := range bindings {
		if binding.FromColumn != "" {
			return ErrInvalid
		}
		input, ok := inputs[binding.FromInput]
		if !ok {
			return ErrInvalid
		}
		if input.Cardinality == "range" {
			if !oneOf(binding.Part, "start", "end") {
				return ErrInvalid
			}
			parts := rangeParts[binding.FromInput]
			if parts == nil {
				parts = make(map[string]bool, 2)
				rangeParts[binding.FromInput] = parts
			}
			if parts[binding.Part] {
				return ErrInvalid
			}
			parts[binding.Part] = true
		} else if binding.Part != "" {
			return ErrInvalid
		}
	}
	for _, parts := range rangeParts {
		if len(parts) != 2 || !parts["start"] || !parts["end"] {
			return ErrInvalid
		}
	}
	return nil
}

func ValidatePublishedValidator(item PublishedValidator) error {
	if !validSlug(item.ID) || strings.TrimSpace(item.Title) == "" || item.Validator.ValidatorID != "" || item.Validator.ID == "" {
		return ErrInvalid
	}
	return validateValidator(item.Validator)
}

func ValidateQueryValidators(item QueryValidators) error {
	if !validQueryRef(item.QueryID) || len(item.Validators) == 0 || len(item.Validators) > 32 {
		return ErrInvalid
	}
	seen := map[string]bool{}
	for _, v := range item.Validators {
		if validateValidator(v) != nil || seen[v.ID] {
			return ErrInvalid
		}
		seen[v.ID] = true
	}
	return nil
}

// ValidateFindingFields validates the package-independent fields of one native
// Finding: its ID grammar, non-empty text, closed status and its evidence
// references' shape and uniqueness. Reference resolution against a Case's own
// evidence remains validateCaseContent's job; the training package reuses this
// narrow helper with its separate package evidence namespace and its own
// 1000-finding limit instead of constructing a fake persisted Case.
func ValidateFindingFields(f Finding) error {
	if !validSlug(f.ID) || f.Text == "" || !oneOf(f.Status, "hypothesis", "confirmed") {
		return ErrInvalid
	}
	seenRefs := map[string]bool{}
	for _, ref := range f.EvidenceRefs {
		if ref == "" || seenRefs[ref] {
			return ErrInvalid
		}
		seenRefs[ref] = true
	}
	return nil
}

func validateCaseContent(c Case) error {
	if c.Context == nil {
		c.Context = map[string]InputValue{}
	}
	for name, value := range c.Context {
		if !validParameterName(name) || value.Validate() != nil {
			return ErrInvalid
		}
	}
	evidenceIDs := map[string]bool{}
	for _, e := range c.Evidence {
		if !validSlug(e.ID) || !oneOf(e.Kind, "check-run", "query", "external") || e.Ref == "" {
			return ErrInvalid
		}
		if evidenceIDs[e.ID] {
			return ErrInvalid
		}
		evidenceIDs[e.ID] = true
	}
	findingIDs := map[string]bool{}
	for _, f := range c.Findings {
		if err := ValidateFindingFields(f); err != nil {
			return ErrInvalid
		}
		if findingIDs[f.ID] {
			return ErrInvalid
		}
		findingIDs[f.ID] = true
		for _, ref := range f.EvidenceRefs {
			if !evidenceIDs[ref] {
				return ErrInvalid
			}
		}
	}
	for _, ref := range c.IncidentRefs {
		if ref == "" || len(ref) > 256 || strings.ContainsAny(ref, "\n\r\x00") {
			return ErrInvalid
		}
	}
	userEvidence := 0
	for _, evidence := range c.Evidence {
		if evidence.Kind != "check-run" {
			userEvidence++
		}
	}
	if userEvidence > 200 || len(c.Findings) > 200 {
		return ErrInvalid
	}
	return nil
}

func validTransition(old, next, reason string) bool {
	switch old {
	case "active":
		return next == "active" || next == "closed" && strings.TrimSpace(reason) != ""
	case "closed":
		return next == "closed" || next == "active" || next == "archived"
	case "archived":
		return next == "archived"
	}
	return false
}

func cloneCase(c Case) (Case, error) {
	return cloneValue(c)
}

// AuditForRun recursively summarizes immutable execution metadata only.
func AuditForRun(run CheckRun) AuditEvent {
	rows, source, queryID, errorCode, revision, fingerprint, executionID := summarizeSteps(run.Steps)
	return AuditEvent{ID: run.ID, At: run.StartedAt, Actor: run.Actor, Action: "run-" + run.Kind, CheckID: run.CheckID, QueryID: queryID, QueryRevision: revision, ResultFingerprint: fingerprint, ExecutionID: executionID, Source: source, ErrorCode: errorCode, CaseID: run.CaseID, RunID: run.ID, Environment: run.Environment, Outcome: run.Outcome, RowCount: rows, DurationMs: run.DurationMs}
}

func summarizeSteps(steps []StepResult) (rows int, source, queryID, errorCode, revision, fingerprint, executionID string) {
	for _, step := range steps {
		rows += step.RowCount
		if source == "" {
			source = step.Source
		}
		if queryID == "" {
			queryID = step.QueryID
		}
		if errorCode == "" {
			errorCode = step.ErrorCode
		}
		if revision == "" {
			revision = step.QueryRevision
		}
		if fingerprint == "" {
			fingerprint = step.ResultFingerprint
		}
		if executionID == "" && step.Execution != nil {
			executionID = step.Execution.ExecutionID
		}
		childRows, childSource, childQuery, childCode, childRevision, childFingerprint, childExecutionID := summarizeSteps(step.Children)
		rows += childRows
		if source == "" {
			source = childSource
		}
		if queryID == "" {
			queryID = childQuery
		}
		if errorCode == "" {
			errorCode = childCode
		}
		if revision == "" {
			revision = childRevision
		}
		if fingerprint == "" {
			fingerprint = childFingerprint
		}
		if executionID == "" {
			executionID = childExecutionID
		}
	}
	return
}
