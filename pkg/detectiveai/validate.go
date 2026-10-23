package detectiveai

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/detective"
)

const (
	maxText         = 16 * 1024
	maxDraftJSON    = 32 * 1024
	maxProposalJSON = 128 * 1024
	maxResources    = 200
	maxSources      = 32
	maxProposals    = 6
)

func validScope(scope Scope) bool {
	return scope.Key != "" && len(scope.Key) <= 512 && scope.Project != "" && len(scope.Project) <= 256 && scope.Actor != "" && len(scope.Actor) <= 256
}

func validSnapshot(snapshot Snapshot) bool {
	if snapshot.CatalogRevision == "" || len(snapshot.CatalogRevision) > 256 || len(snapshot.Environments) > 32 {
		return false
	}
	for _, environment := range snapshot.Environments {
		if environment == "" || len(environment) > 128 {
			return false
		}
	}
	for _, list := range [][]Resource{snapshot.Queries, snapshot.Checks, snapshot.Validators, snapshot.Scenarios, snapshot.Cases, snapshot.Runs} {
		if len(list) > maxResources {
			return false
		}
		seen := map[string]bool{}
		for _, item := range list {
			if item.ID == "" || len(item.ID) > 512 || item.Revision == "" || len(item.Revision) > 256 || len(item.Title) > 256 || seen[item.ID] || len(item.Inputs) > 64 {
				return false
			}
			seen[item.ID] = true
		}
	}
	return true
}

func sourceSet(sources []Source) (map[string]Source, bool) {
	if len(sources) > maxSources {
		return nil, false
	}
	seen := map[string]Source{}
	total := 0
	for _, source := range sources {
		_, duplicate := seen[source.ID]
		if source.ID == "" || len(source.ID) > 128 || source.Kind == "" || len(source.Kind) > 64 || source.Revision == "" || len(source.Revision) > 256 || source.Text == "" || len(source.Text) > maxText || duplicate {
			return nil, false
		}
		total += len(source.Text)
		if total > 32*1024 {
			return nil, false
		}
		seen[source.ID] = source
	}
	return seen, true
}

func resource(list []Resource, id string) (Resource, bool) {
	for _, item := range list {
		if item.ID == id {
			return item, true
		}
	}
	return Resource{}, false
}

func validateProvenance(items []Provenance, allowed map[string]Source) bool {
	if len(items) == 0 || len(items) > maxSources {
		return false
	}
	for _, item := range items {
		source, found := allowed[item.SourceID]
		if !found || item.Start < 0 || item.End <= item.Start || item.End > len(source.Text) || len(item.Quote) > 512 || item.Quote != "" && item.Quote != source.Text[item.Start:item.End] || item.Confidence < 0 || item.Confidence > 1 || item.Confidence != item.Confidence {
			return false
		}
	}
	return true
}

func validateDraft(d Draft, snapshot Snapshot, sources []Source) error {
	return validateDraftWithExpectations(d, snapshot, sources, nil)
}

func validateDraftWithExpectations(d Draft, snapshot Snapshot, sources []Source, expectedCounts map[string]int) error {
	if boundedDraft(d) != nil || strings.TrimSpace(d.Title) == "" || len(d.Title) > 256 || len(d.Ambiguities) > 16 {
		return ErrInvalid
	}
	allowed, ok := sourceSet(sources)
	if !ok || !validateProvenance(d.Sources, allowed) {
		return ErrInvalid
	}
	for _, ambiguity := range d.Ambiguities {
		if strings.TrimSpace(ambiguity) == "" || len(ambiguity) > 256 {
			return ErrInvalid
		}
	}
	count := 0
	for _, present := range []bool{d.Context != nil, d.Query != nil, d.Check != nil, d.Validator != nil, d.Scenario != nil, d.Case != nil, d.Run != nil, d.Explanation != nil} {
		if present {
			count++
		}
	}
	if count != 1 {
		return ErrInvalid
	}
	switch d.Kind {
	case KindContext:
		if len(d.Context) == 0 || len(d.Context) > 64 {
			return ErrInvalid
		}
		for name, value := range d.Context {
			if !detective.ValidParameterName(name) || value.Value.Validate() != nil || !validateProvenance(value.Provenance, allowed) {
				return ErrInvalid
			}
			if expected, found := expectedCounts[name]; found && (expected < 1 || expected > 100 || inputCount(value.Value) != expected) {
				return ErrInvalid
			}
		}
		for name := range expectedCounts {
			if _, found := d.Context[name]; !found {
				return ErrInvalid
			}
		}
	case KindQuery:
		if d.Query == nil || !detective.ValidQueryRef(d.Query.ID) || strings.TrimSpace(d.Query.Title) == "" || len(d.Query.Title) > 256 || strings.TrimSpace(d.Query.Text) == "" || len(d.Query.Text) > maxText || !oneOf(d.Query.Type, "SQL", "TugQL", "StructuredSQL", "HTTP") || len(d.Query.ConnectionID) > 128 || len(d.Query.Inputs) > 64 {
			return ErrInvalid
		}
		if previous, found := resource(snapshot.Queries, d.Query.ID); found && previous.Revision != d.Query.ExpectedRevision {
			return ErrStale
		}
		if !validInputs(d.Query.Inputs) {
			return ErrInvalid
		}
	case KindCheck:
		if d.Check == nil || detective.ValidateCheck(*d.Check) != nil || !validCheckReferences(*d.Check, snapshot) {
			return ErrInvalid
		}
	case KindValidator:
		if d.Validator == nil || detective.ValidatePublishedValidator(*d.Validator) != nil || !validValidatorReferences(d.Validator.Validator, snapshot) {
			return ErrInvalid
		}
	case KindScenario:
		if d.Scenario == nil || detective.ValidateScenario(*d.Scenario) != nil || !validScenarioReferences(*d.Scenario, snapshot) {
			return ErrInvalid
		}
	case KindCase:
		if d.Case == nil || !detective.ValidSlug(d.Case.CaseID) || d.Case.ExpectedVersion < 1 || !validCaseEdit(*d.Case) {
			return ErrInvalid
		}
		item, found := resource(snapshot.Cases, d.Case.CaseID)
		if !found || item.Revision != strconv.FormatInt(d.Case.ExpectedVersion, 10) {
			return ErrStale
		}
	case KindRun:
		if d.Run == nil || !detective.ValidSlug(d.Run.CheckID) || !hasEnvironment(snapshot, d.Run.Environment) || d.Run.CaseID != "" && !detective.ValidSlug(d.Run.CaseID) {
			return ErrInvalid
		}
		check, found := resource(snapshot.Checks, d.Run.CheckID)
		if !found || d.Run.CaseID != "" && !hasResource(snapshot.Cases, d.Run.CaseID) {
			return ErrStale
		}
		if detective.ValidateInputs(check.Inputs, d.Run.Inputs) != nil {
			return ErrInvalid
		}
	case KindExplain:
		if d.Explanation == nil || !hasResource(snapshot.Runs, d.Explanation.RunID) || strings.TrimSpace(d.Explanation.Summary) == "" || strings.TrimSpace(d.Explanation.Basis) == "" || len(d.Explanation.Summary) > 2048 || len(d.Explanation.Basis) > 2048 {
			return ErrInvalid
		}
	default:
		return ErrUnsupported
	}
	return nil
}

func inputCount(value detective.InputValue) int {
	if value.Set != nil {
		return len(value.Set.Values)
	}
	return 1
}

func validExpectedCounts(counts map[string]int) bool {
	if len(counts) > 64 {
		return false
	}
	for name, count := range counts {
		if !detective.ValidParameterName(name) || count < 1 || count > 100 {
			return false
		}
	}
	return true
}

func validInputs(inputs []detective.Input) bool {
	seen := map[string]bool{}
	for _, input := range inputs {
		if !detective.ValidParameterName(input.Name) || !detective.ValidInputType(input.Type) || !oneOf(input.Cardinality, "scalar", "set", "range") || seen[input.Name] {
			return false
		}
		seen[input.Name] = true
	}
	return true
}

func validCheckReferences(check detective.Check, snapshot Snapshot) bool {
	for _, step := range check.Steps {
		if step.Kind == "query" && !hasResource(snapshot.Queries, step.QueryID) || step.Kind == "check" && !hasResource(snapshot.Checks, step.CheckID) {
			return false
		}
		for _, v := range step.Validators {
			if !validValidatorReferences(v, snapshot) {
				return false
			}
		}
	}
	return true
}

func validValidatorReferences(v detective.Validator, snapshot Snapshot) bool {
	return (v.ValidatorID == "" || hasResource(snapshot.Validators, v.ValidatorID)) && (v.CheckID == "" || hasResource(snapshot.Checks, v.CheckID))
}

func validScenarioReferences(scenario detective.Scenario, snapshot Snapshot) bool {
	for _, step := range scenario.Steps {
		if step.Kind == "query" && !hasResource(snapshot.Queries, step.Ref) || step.Kind == "check" && !hasResource(snapshot.Checks, step.Ref) {
			return false
		}
	}
	return true
}

func validCaseEdit(edit CaseEdit) bool {
	if len(edit.Title) > 256 || len(edit.Context) > 64 || len(edit.Findings) > 100 || len(edit.ClosureReason) > 2048 || !oneOf(edit.Status, "", "active", "closed") {
		return false
	}
	for name, value := range edit.Context {
		if !detective.ValidParameterName(name) || value.Validate() != nil {
			return false
		}
	}
	for _, finding := range edit.Findings {
		if !detective.ValidSlug(finding.ID) || strings.TrimSpace(finding.Text) == "" || len(finding.Text) > 2048 || !oneOf(finding.Status, "hypothesis", "confirmed") {
			return false
		}
	}
	return true
}

func hasResource(list []Resource, id string) bool {
	_, found := resource(list, id)
	return found
}

func hasEnvironment(snapshot Snapshot, id string) bool {
	for _, environment := range snapshot.Environments {
		if id == environment {
			return true
		}
	}
	return false
}

func oneOf(value string, options ...string) bool {
	for _, option := range options {
		if value == option {
			return true
		}
	}
	return false
}

func proposalDigest(proposal Proposal) (string, error) {
	proposal.Digest = ""
	raw, err := json.Marshal(proposal)
	if err != nil || len(raw) > maxProposalJSON {
		return "", ErrInvalid
	}
	binding, err := detective.BindOperation(proposal.Scope.Key, proposal.Scope.Actor, "ai-proposal", proposal.ID, proposal)
	if err != nil {
		return "", ErrInvalid
	}
	return binding.PayloadDigest, nil
}

func validateProposal(proposal Proposal, now time.Time) error {
	if !validScope(proposal.Scope) || !validSnapshot(proposal.Basis) || !detective.ValidSlug(proposal.ID) || proposal.ParentProposalID != "" && !detective.ValidSlug(proposal.ParentProposalID) || proposal.CreatedAt.IsZero() || !proposal.ExpiresAt.After(proposal.CreatedAt) || !now.Before(proposal.ExpiresAt) {
		return ErrInvalid
	}
	if !validStoredSources(proposal.Sources) || !validExpectedCounts(proposal.ExpectedCounts) {
		return ErrInvalid
	}
	digest, err := proposalDigest(proposal)
	if err != nil || subtle.ConstantTimeCompare([]byte(digest), []byte(proposal.Digest)) != 1 {
		return ErrInvalid
	}
	return nil
}

func sameSnapshot(a, b Snapshot) bool {
	normalize := func(value Snapshot) Snapshot {
		value = cloneSnapshot(value)
		if len(value.Environments) == 0 {
			value.Environments = nil
		}
		trim := func(items []Resource) []Resource {
			if len(items) == 0 {
				return nil
			}
			for i := range items {
				if len(items[i].Inputs) == 0 {
					items[i].Inputs = nil
				}
			}
			return items
		}
		value.Queries = trim(value.Queries)
		value.Checks = trim(value.Checks)
		value.Validators = trim(value.Validators)
		value.Scenarios = trim(value.Scenarios)
		value.Cases = trim(value.Cases)
		value.Runs = trim(value.Runs)
		return value
	}
	return reflect.DeepEqual(normalize(a), normalize(b))
}

func boundedDraft(d Draft) error {
	raw, err := json.Marshal(d)
	if err != nil || len(raw) > maxDraftJSON {
		return ErrInvalid
	}
	return nil
}

func strictDecode(data []byte, target any) error {
	if len(data) == 0 || len(data) > maxDraftJSON {
		return ErrInvalid
	}
	if err := apicontract.DecodeStrict(data, target, "project", "actor", "scope", "authority", "securityContextId", "credentials"); err != nil {
		return ErrInvalid
	}
	return nil
}

func sourceEvidence(sources []Source) []SourceEvidence {
	evidence := make([]SourceEvidence, 0, len(sources))
	for _, source := range sources {
		digest := sha256.Sum256([]byte(source.Text))
		evidence = append(evidence, SourceEvidence{ID: source.ID, Kind: source.Kind, Revision: source.Revision, Digest: hex.EncodeToString(digest[:]), Length: len(source.Text)})
	}
	return evidence
}

func validStoredSources(sources []SourceEvidence) bool {
	if len(sources) > maxSources {
		return false
	}
	seen := map[string]bool{}
	for _, source := range sources {
		if source.ID == "" || len(source.ID) > 128 || source.Kind == "" || len(source.Kind) > 64 || source.Revision == "" || len(source.Revision) > 256 || source.Length < 1 || source.Length > maxText || len(source.Digest) != 64 || seen[source.ID] {
			return false
		}
		if _, err := hex.DecodeString(source.Digest); err != nil {
			return false
		}
		seen[source.ID] = true
	}
	return true
}
