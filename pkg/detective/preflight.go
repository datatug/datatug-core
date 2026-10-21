package detective

import (
	"github.com/datatug/datatug-core/pkg/investigation"
	"sort"
	"strings"
)

// BindingPreflight describes binding availability, never a Check outcome or
// source permission. Reasons contain only fixed codes and admitted identities.
type BindingPreflight struct {
	State   string `json:"state"`
	CheckID string `json:"checkId"`
	StepID  string `json:"stepId,omitempty"`
}

const (
	BindingInputBindable     = "input-bindable"
	BindingMissingInput      = "missing/incompatible-input"
	BindingInvalidDefinition = "invalid/unavailable-definition"
	BindingDeferredRow       = "deferred-row-binding"
)

// PreflightBindings uses the execution engine's catalog/input/binding rules.
// queryInputs is authoritative saved-query parameter metadata admitted by the
// host, keyed by query reference. It accepts no executor, SQL, or source handle.
func PreflightBindings(c Catalog, root string, inputs map[string]InputValue, queryInputs map[string][]Input) BindingPreflight {
	result := BindingPreflight{State: BindingInvalidDefinition, CheckID: root}
	if len(c.Checks) > maxNodes || len(c.Validators) > maxNodes || len(c.QueryValidators) > maxNodes || len(queryInputs) > maxNodes || len(inputs) > maxNodes || ValidateCatalog(c) != nil {
		return result
	}
	if _, ok := c.Checks[root]; !ok {
		return result
	}
	// Validate parameter schemas even for dependencies whose values need a row.
	for _, check := range c.Checks {
		if len(check.Inputs) > maxNodes {
			return result
		}
		for _, step := range check.Steps {
			if step.Kind != "query" {
				continue
			}
			schema, ok := queryInputs[step.QueryID]
			if !ok || !validQueryRef(step.QueryID) || len(schema) > maxNodes {
				return result
			}
			seen := map[string]bool{}
			for _, in := range schema {
				if seen[in.Name] || !validParameterName(in.Name) || !validInputType(in.Type) || !oneOf(in.Cardinality, "scalar", "set") {
					return result
				}
				seen[in.Name] = true
			}
		}
	}
	nodes := 0
	memo := map[string]BindingPreflight{}
	var visit func(string, map[string]InputValue, map[string]bool, int) BindingPreflight
	visit = func(id string, values map[string]InputValue, unknown map[string]bool, depth int) BindingPreflight {
		names := make([]string, 0, len(unknown))
		for name := range unknown {
			names = append(names, name)
		}
		sort.Strings(names)
		key := cacheKey(id, "", values) + "/" + strings.Join(names, ",")
		out := BindingPreflight{State: BindingInputBindable, CheckID: id}
		if depth > maxDepth || nodes >= maxNodes {
			out.State = BindingInvalidDefinition
			return out
		}
		if prior, ok := memo[key]; ok {
			return prior
		}
		nodes++
		check := c.Checks[id]
		if validatePartialInputs(check.Inputs, values, unknown, false) != nil {
			out.State = BindingMissingInput
			return out
		}
		deferred := len(unknown) > 0
		for _, step := range check.Steps {
			mapped, pending, err := mapPartialBindings(step.Bindings, values, unknown)
			if err != nil {
				out.State = BindingMissingInput
				out.StepID = step.ID
				return out
			}
			if step.Kind == "check" {
				child := visit(step.CheckID, mapped, pending, depth+1)
				if child.State == BindingDeferredRow {
					deferred = true
				} else if child.State != BindingInputBindable {
					return child
				}
				continue
			}
			if validatePendingQueryBindings(check.Inputs, queryInputs[step.QueryID], step.Bindings, pending) != nil || validatePartialInputs(queryInputs[step.QueryID], mapped, pending, true) != nil {
				out.State = BindingMissingInput
				out.StepID = step.ID
				return out
			}
			validators := append([]Validator(nil), step.Validators...)
			validators = append(validators, c.QueryValidators[step.QueryID].Validators...)
			for _, validator := range validators {
				if validator.ValidatorID != "" {
					validator = c.Validators[validator.ValidatorID].Validator
				}
				if validator.Kind != "check" {
					continue
				}
				bound, pending, err := mapPartialBindings(validator.Bindings, values, unknown)
				if err != nil {
					out.State = BindingMissingInput
					out.StepID = step.ID
					return out
				}
				child := visit(validator.CheckID, bound, pending, depth+1)
				if child.State == BindingDeferredRow {
					deferred = true
				} else if child.State != BindingInputBindable {
					return child
				}
			}
		}
		if deferred {
			out.State = BindingDeferredRow
		}
		memo[key] = out
		return out
	}
	return visit(root, inputs, nil, 0)
}

// Unknown values are names only: no invented row value is passed to the mapper.
// All independently knowable FromInput bindings still use the real mapper.
func mapPartialBindings(bindings []Binding, values map[string]InputValue, unknown map[string]bool) (map[string]InputValue, map[string]bool, error) {
	known := map[string]InputValue{}
	pending := map[string]bool{}
	for _, binding := range bindings {
		if binding.FromColumn != "" || unknown[binding.FromInput] {
			pending[binding.Input] = true
			continue
		}
		mapped, err := mapBindings([]Binding{binding}, values, nil, nil)
		if err != nil {
			return nil, nil, err
		}
		known[binding.Input] = mapped[binding.Input]
	}
	return known, pending, nil
}

func validatePartialInputs(schema []Input, values map[string]InputValue, unknown map[string]bool, query bool) error {
	partial := append([]Input(nil), schema...)
	for i := range partial {
		if unknown[partial[i].Name] {
			partial[i].Required = false
		}
	}
	if !query {
		return validateInputs(partial, values)
	}
	// Match owning hosted parameter admission, which differs from Check inputs:
	// null is allowed only for optional parameters; every set item is checked.
	byName := map[string]Input{}
	for _, in := range partial {
		byName[in.Name] = in
		if in.Required {
			if _, ok := values[in.Name]; !ok {
				return ErrInvalid
			}
		}
	}
	for name, value := range values {
		in, ok := byName[name]
		if !ok || value.Validate() != nil || (value.Set != nil) != (in.Cardinality == "set") || value.Range != nil {
			return ErrInvalid
		}
		valid := func(item Value) bool {
			return item.Type == in.Type || item.Type == investigation.ValueTypeNull && !in.Required
		}
		if value.Scalar != nil && !valid(*value.Scalar) {
			return ErrInvalid
		}
		if value.Set != nil {
			for _, item := range value.Set.Values {
				if !valid(item) {
					return ErrInvalid
				}
			}
		}
	}
	return nil
}

// Row-dependent values are unknown, but destination existence and propagated
// scalar/set shape are known without inventing a row or executing the query.
func validatePendingQueryBindings(source, target []Input, bindings []Binding, pending map[string]bool) error {
	sources := map[string]Input{}
	targets := map[string]Input{}
	for _, in := range source {
		sources[in.Name] = in
	}
	for _, in := range target {
		targets[in.Name] = in
	}
	for _, binding := range bindings {
		if !pending[binding.Input] {
			continue
		}
		expected, ok := targets[binding.Input]
		if !ok {
			return ErrInvalid
		}
		provided := sources[binding.FromInput]
		cardinality := provided.Cardinality
		if binding.Part != "" {
			cardinality = "scalar"
		}
		if cardinality != expected.Cardinality || provided.Type != expected.Type && (expected.Required || cardinality == "set") {
			return ErrInvalid
		}
	}
	return nil
}
