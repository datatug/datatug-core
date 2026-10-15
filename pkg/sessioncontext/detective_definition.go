package sessioncontext

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/datatug/datatug-core/pkg/detective"
)

const MaxDetectiveQueryBytes = 64 << 10
const MaxDetectiveDefinitionBytes = 512 << 10

type DetectiveAcceptedQueryPin struct {
	QueryID        string          `json:"queryId"`
	Revision       string          `json:"revision"`
	DefinitionJSON json.RawMessage `json:"definitionJson"`
}
type DetectiveScenarioStepIdentity struct {
	ScenarioID      string `json:"scenarioId"`
	ScenarioVersion int64  `json:"scenarioVersion"`
	ApplicationID   string `json:"applicationId"`
	StepID          string `json:"stepId"`
}
type DetectiveCheckDefinitionPin struct {
	SchemaVersion       int                            `json:"schemaVersion"`
	Check               detective.Check                `json:"check"`
	Dependencies        []detective.Check              `json:"dependencies"`
	PublishedValidators []detective.PublishedValidator `json:"publishedValidators"`
	QueryValidators     []detective.QueryValidators    `json:"queryValidators"`
	Queries             []DetectiveAcceptedQueryPin    `json:"queries"`
	Scenario            *DetectiveScenarioStepIdentity `json:"scenario,omitempty"`
}

// CanonicalDetectiveDefinition validates a complete private executable snapshot.
// It uses existing saved-query and catalog contracts, not source admission.
// Errors never include query text or validator/provider diagnostics.
func CanonicalDetectiveDefinition(ctx context.Context, definition DetectiveCheckDefinitionPin) (DetectiveCheckDefinitionPin, string, error) {
	var out DetectiveCheckDefinitionPin
	for _, query := range definition.Queries {
		if len(query.DefinitionJSON) > MaxDetectiveQueryBytes {
			return out, "", ErrInvalid
		}
	}
	raw, err := json.Marshal(definition)
	if err != nil || len(raw) > MaxDetectiveDefinitionBytes || apicontract.DecodeStrict(raw, &out) != nil || out.SchemaVersion != 1 || out.Check.Version <= 0 {
		return out, "", ErrInvalid
	}
	if out.Scenario != nil && (!safeID.MatchString(out.Scenario.ScenarioID) || !safeID.MatchString(out.Scenario.ApplicationID) || !safeID.MatchString(out.Scenario.StepID) || out.Scenario.ScenarioVersion <= 0) {
		return out, "", ErrInvalid
	}
	r := pinnedCatalogReader{checks: map[string]detective.Check{}, validators: map[string]detective.PublishedValidator{}, locals: map[string]detective.QueryValidators{}, queries: map[string]bool{}}
	for _, check := range append([]detective.Check{out.Check}, out.Dependencies...) {
		if _, exists := r.checks[check.ID]; exists || check.Version <= 0 {
			return out, "", ErrInvalid
		}
		r.checks[check.ID] = check
	}
	for _, validator := range out.PublishedValidators {
		if _, exists := r.validators[validator.ID]; exists {
			return out, "", ErrInvalid
		}
		r.validators[validator.ID] = validator
	}
	for _, local := range out.QueryValidators {
		if _, exists := r.locals[local.QueryID]; exists {
			return out, "", ErrInvalid
		}
		r.locals[local.QueryID] = local
	}
	for i, item := range out.Queries {
		var query datatug.QueryDefWithFolderPath
		if r.queries[item.QueryID] || !detective.ValidQueryRef(item.QueryID) || strings.TrimSpace(item.Revision) == "" || len(item.Revision) > 256 || len(item.DefinitionJSON) > MaxDetectiveQueryBytes || apicontract.DecodeStrict(item.DefinitionJSON, &query) != nil {
			return out, "", ErrInvalid
		}
		folder, id := "", item.QueryID
		if at := strings.LastIndexByte(id, '/'); at >= 0 {
			folder, id = id[:at], id[at+1:]
		}
		if query.ID != id || query.FolderPath != folder || len(query.Targets) != 0 || query.QueryDef.Validate() != nil || query.Type == "folder" {
			return out, "", ErrInvalid
		}
		// Validation uses the existing saved-query DTO semantics. Afterwards,
		// preserve interface-valued numbers without rounding in the owned JSON.
		decoder := json.NewDecoder(bytes.NewReader(item.DefinitionJSON))
		decoder.UseNumber()
		_ = decoder.Decode(&query)
		encoded, encodeErr := json.Marshal(query)
		if encodeErr != nil || len(encoded) > MaxDetectiveQueryBytes {
			return out, "", ErrInvalid
		}
		out.Queries[i].DefinitionJSON = encoded
		r.queries[item.QueryID] = true
	}
	catalog, err := detective.LoadCatalog(ctx, out.Check.ID, "", r)
	if err != nil || len(catalog.Checks) != len(r.checks) || len(catalog.Validators) != len(r.validators) || len(catalog.QueryValidators) != len(r.locals) {
		return out, "", ErrInvalid
	}
	usedQueries := map[string]bool{}
	for _, check := range catalog.Checks {
		for _, step := range check.Steps {
			if step.Kind == "query" {
				usedQueries[step.QueryID] = true
			}
		}
	}
	if len(usedQueries) != len(r.queries) {
		return out, "", ErrInvalid
	}
	sort.Slice(out.Dependencies, func(i, j int) bool { return out.Dependencies[i].ID < out.Dependencies[j].ID })
	sort.Slice(out.PublishedValidators, func(i, j int) bool { return out.PublishedValidators[i].ID < out.PublishedValidators[j].ID })
	sort.Slice(out.QueryValidators, func(i, j int) bool { return out.QueryValidators[i].QueryID < out.QueryValidators[j].QueryID })
	sort.Slice(out.Queries, func(i, j int) bool { return out.Queries[i].QueryID < out.Queries[j].QueryID })
	encoded, _ := json.Marshal(out) // owned, validated values were successfully encoded above
	if len(encoded) > MaxDetectiveDefinitionBytes {
		return out, "", ErrInvalid
	}
	digest := sha256.Sum256(encoded)
	return out, hex.EncodeToString(digest[:]), nil
}

type pinnedCatalogReader struct {
	checks     map[string]detective.Check
	validators map[string]detective.PublishedValidator
	locals     map[string]detective.QueryValidators
	queries    map[string]bool
}

func (r pinnedCatalogReader) GetCheck(_ context.Context, id string) (detective.Check, error) {
	v, ok := r.checks[id]
	if !ok {
		return v, detective.ErrNotFound
	}
	return v, nil
}
func (r pinnedCatalogReader) GetPublishedValidator(_ context.Context, id string) (detective.PublishedValidator, error) {
	v, ok := r.validators[id]
	if !ok {
		return v, detective.ErrNotFound
	}
	return v, nil
}
func (r pinnedCatalogReader) GetQueryValidators(_ context.Context, id string) (detective.QueryValidators, bool, error) {
	v, ok := r.locals[id]
	return v, ok, nil
}
func (r pinnedCatalogReader) QueryExists(_ context.Context, id string) (bool, error) {
	return r.queries[id], nil
}
