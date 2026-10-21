package detective

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
	var visit func(string, map[string]InputValue, int) BindingPreflight
	visit = func(id string, values map[string]InputValue, depth int) BindingPreflight {
		out := BindingPreflight{State: BindingInputBindable, CheckID: id}
		nodes++
		if depth > maxDepth || nodes > maxNodes {
			out.State = BindingInvalidDefinition
			return out
		}
		check := c.Checks[id]
		if validateInputs(check.Inputs, values) != nil {
			out.State = BindingMissingInput
			return out
		}
		deferred := false
		for _, step := range check.Steps {
			mapped, err := mapBindings(step.Bindings, values, nil, nil)
			if err != nil {
				out.State = BindingMissingInput
				out.StepID = step.ID
				return out
			}
			if step.Kind == "check" {
				child := visit(step.CheckID, mapped, depth+1)
				if child.State == BindingDeferredRow {
					deferred = true
				} else if child.State != BindingInputBindable {
					return child
				}
				continue
			}
			if validateInputs(queryInputs[step.QueryID], mapped) != nil {
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
				row := false
				for _, binding := range validator.Bindings {
					if binding.FromColumn != "" {
						row = true
					}
				}
				if row {
					deferred = true
					continue
				}
				bound, err := mapBindings(validator.Bindings, values, nil, nil)
				if err != nil {
					out.State = BindingMissingInput
					out.StepID = step.ID
					return out
				}
				child := visit(validator.CheckID, bound, depth+1)
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
		return out
	}
	return visit(root, inputs, 0)
}
