package detective

import (
	"context"
	"fmt"
	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/investigation"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
)

func preflightFixture() (Catalog, map[string]InputValue, map[string][]Input) {
	schema := []Input{{Name: "id", Type: investigation.ValueTypeInteger, Cardinality: "scalar", Required: true}}
	value := investigation.NewIntegerValue("42")
	c := Catalog{Checks: map[string]Check{"root": {ID: "root", Title: "Root", Version: 1, Inputs: schema, Steps: []Step{{ID: "query", Kind: "query", QueryID: "query", Bindings: []Binding{{Input: "id", FromInput: "id"}}}}}}}
	return c, map[string]InputValue{"id": {Scalar: &value}}, map[string][]Input{"query": append([]Input(nil), schema...)}
}
func TestBindingPreflightActualMappingAgreement(t *testing.T) {
	c, inputs, metadata := preflightFixture()
	require.Equal(t, BindingInputBindable, PreflightBindings(c, "root", inputs, metadata).State)
	mapped, err := mapBindings(c.Checks["root"].Steps[0].Bindings, inputs, nil, nil)
	require.NoError(t, err)
	require.NoError(t, validateInputs(metadata["query"], mapped))
	// There is no executor/callback/source in the API: all inputs are metadata.
	for _, cardinality := range []string{"set", "range"} {
		cc, values, meta := preflightFixture()
		root := cc.Checks["root"]
		root.Inputs[0].Cardinality = cardinality
		if cardinality == "set" {
			scalar := *values["id"].Scalar
			set := apicontract.TypedValueSet{Values: []Value{scalar}}
			values["id"] = InputValue{Set: &set}
			meta["query"][0].Cardinality = "set"
		} else {
			scalar := *values["id"].Scalar
			values["id"] = InputValue{Range: &ValueRange{Type: "range", Start: scalar, End: scalar}}
			root.Steps[0].Bindings[0].Part = "end"
		}
		cc.Checks["root"] = root
		require.Equal(t, BindingInputBindable, PreflightBindings(cc, "root", values, meta).State)
		mapped, err = mapBindings(root.Steps[0].Bindings, values, nil, nil)
		require.NoError(t, err)
		require.NoError(t, validateInputs(meta["query"], mapped))
	}
}
func TestBindingPreflightSafeInvalidAndMissingReasons(t *testing.T) {
	cases := map[string]func(*Catalog, map[string]InputValue, map[string][]Input){
		"missing": func(c *Catalog, v map[string]InputValue, m map[string][]Input) { delete(v, "id") },
		"unknown": func(c *Catalog, v map[string]InputValue, m map[string][]Input) { v["private-value"] = v["id"] },
		"binding": func(c *Catalog, v map[string]InputValue, m map[string][]Input) {
			r := c.Checks["root"]
			r.Inputs[0].Required = false
			c.Checks["root"] = r
			delete(v, "id")
		},
		"metadata-type": func(c *Catalog, v map[string]InputValue, m map[string][]Input) {
			m["query"][0].Type = investigation.ValueTypeString
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			c, v, m := preflightFixture()
			change(&c, v, m)
			result := PreflightBindings(c, "root", v, m)
			require.Equal(t, BindingMissingInput, result.State)
			require.NotContains(t, result.State+result.CheckID+result.StepID, "private-value")
		})
	}
	for _, change := range []func(*Catalog, map[string][]Input){
		func(c *Catalog, m map[string][]Input) { delete(c.Checks, "root") },
		func(c *Catalog, m map[string][]Input) { r := c.Checks["root"]; r.ID = "wrong"; c.Checks["root"] = r },
		func(c *Catalog, m map[string][]Input) { delete(m, "query") },
		func(c *Catalog, m map[string][]Input) { m["query"] = make([]Input, maxNodes+1) },
		func(c *Catalog, m map[string][]Input) { m["query"] = append(m["query"], m["query"][0]) },
		func(c *Catalog, m map[string][]Input) { m["query"][0].Name = "" },
		func(c *Catalog, m map[string][]Input) { m["query"][0].Type = "invalid" },
		func(c *Catalog, m map[string][]Input) { m["query"][0].Cardinality = "range" },
		func(c *Catalog, m map[string][]Input) {
			for i := 0; i <= maxNodes; i++ {
				m[strings.Repeat("x", i+1)] = nil
			}
		},
	} {
		c, v, m := preflightFixture()
		change(&c, m)
		require.Equal(t, BindingInvalidDefinition, PreflightBindings(c, "root", v, m).State)
	}
	c, v, m := preflightFixture()
	require.Equal(t, BindingInvalidDefinition, PreflightBindings(c, "absent", v, m).State)
}

func TestBindingPreflightNestedRowsAndCycles(t *testing.T) {
	c, v, m := preflightFixture()
	child := c.Checks["root"]
	child.ID = "child"
	child.Steps = []Step{{ID: "leaf", Kind: "query", QueryID: "child-query", Bindings: []Binding{{Input: "id", FromInput: "id"}}}}
	m["child-query"] = m["query"]
	c.Checks["child"] = child
	root := c.Checks["root"]
	root.Steps = []Step{{ID: "nested", Kind: "check", CheckID: "child", Bindings: []Binding{{Input: "id", FromInput: "id"}}}}
	c.Checks["root"] = root
	require.Equal(t, BindingInputBindable, PreflightBindings(c, "root", v, m).State)
	// Invalid child input is surfaced from the actual shared mapper.
	child.Inputs[0].Required = false
	root.Inputs[0].Required = false
	c.Checks["root"] = root
	c.Checks["child"] = child
	require.Equal(t, BindingMissingInput, PreflightBindings(c, "root", nil, m).State)
	c, v, m = preflightFixture()
	child = c.Checks["root"]
	child.ID = "child"
	child.Steps = []Step{{ID: "leaf", Kind: "query", QueryID: "child-query", Bindings: []Binding{{Input: "id", FromInput: "id"}}}}
	m["child-query"] = m["query"]
	c.Checks["child"] = child
	root = c.Checks["root"]
	root.Steps[0].Validators = []Validator{{ID: "row", Scope: "row", Kind: "check", Severity: "error", CheckID: "child", Bindings: []Binding{{Input: "id", FromColumn: "id"}}}}
	c.Checks["root"] = root
	require.Equal(t, BindingDeferredRow, PreflightBindings(c, "root", v, m).State)
	// Promoted and query-local row validators use exactly the catalog's resolution.
	validator := root.Steps[0].Validators[0]
	root.Steps[0].Validators = []Validator{{ID: "promoted-use", ValidatorID: "promoted"}}
	c.Checks["root"] = root
	c.Validators = map[string]PublishedValidator{"promoted": {ID: "promoted", Title: "Promoted", Version: 1, Validator: validator}}
	require.Equal(t, BindingDeferredRow, PreflightBindings(c, "root", v, m).State)
	root.Steps[0].Validators = nil
	c.Checks["root"] = root
	c.QueryValidators = map[string]QueryValidators{"query": {QueryID: "query", Version: 1, Validators: []Validator{{ID: "promoted-use", ValidatorID: "promoted"}}}}
	require.Equal(t, BindingDeferredRow, PreflightBindings(c, "root", v, m).State)
	// A root cycle is invalid definition, never predicted fail/pass.
	root.Steps = []Step{{ID: "cycle", Kind: "check", CheckID: "root", Bindings: []Binding{{Input: "id", FromInput: "id"}}}}
	c.Checks["root"] = root
	require.Equal(t, BindingInvalidDefinition, PreflightBindings(c, "root", v, m).State)
}

func TestBindingPreflightRecursiveAndValidatorBoundaries(t *testing.T) {
	base := func() (Catalog, map[string]InputValue, map[string][]Input) {
		c, v, m := preflightFixture()
		child := c.Checks["root"]
		child.ID = "child"
		child.Inputs = append([]Input(nil), child.Inputs...)
		child.Steps = []Step{{ID: "leaf", Kind: "query", QueryID: "leaf", Bindings: []Binding{{Input: "id", FromInput: "id"}}}}
		c.Checks["child"] = child
		m["leaf"] = append([]Input(nil), m["query"]...)
		return c, v, m
	}
	for _, via := range []string{"step", "validator"} {
		for _, state := range []string{"valid", "missing", "map-missing", "deferred"} {
			t.Run(via+state, func(t *testing.T) {
				c, v, m := base()
				root := c.Checks["root"]
				binding := []Binding{{Input: "id", FromInput: "id"}}
				if via == "step" {
					root.Steps = []Step{{ID: "nested", Kind: "check", CheckID: "child", Bindings: binding}}
				} else {
					root.Steps[0].Validators = []Validator{{ID: "bound", Scope: "row", Kind: "check", Severity: "error", CheckID: "child", Bindings: binding}, {ID: "non-null", Scope: "row", Kind: "non-null", Severity: "error", Field: "id"}}
				}
				expected := BindingInputBindable
				if state == "map-missing" {
					root.Inputs[0].Required = false
					if via == "validator" {
						root.Steps[0].Bindings = nil
						m["query"] = nil
					}
					delete(v, "id")
					expected = BindingMissingInput
				}
				if state == "missing" {
					m["leaf"][0].Type = investigation.ValueTypeString
					expected = BindingMissingInput
				}
				if state == "deferred" {
					leaf := c.Checks["child"]
					leaf.Steps[0].Validators = []Validator{{ID: "row", Scope: "row", Kind: "check", Severity: "error", CheckID: "last", Bindings: []Binding{{Input: "id", FromColumn: "id"}}}}
					c.Checks["child"] = leaf
					last := leaf
					last.ID = "last"
					last.Steps = []Step{{ID: "end", Kind: "query", QueryID: "last", Bindings: binding}}
					c.Checks["last"] = last
					m["last"] = append([]Input(nil), m["query"]...)
					expected = BindingDeferredRow
				}
				c.Checks["root"] = root
				require.Equal(t, expected, PreflightBindings(c, "root", v, m).State)
			})
		}
	}
	c, v, m := base()
	root := c.Checks["root"]
	root.Steps = nil
	for i := 0; i < 32; i++ {
		root.Steps = append(root.Steps, Step{ID: strings.Repeat("x", i+1), Kind: "check", CheckID: "child", Bindings: []Binding{{Input: "id", FromInput: "id"}}})
	}
	c.Checks["root"] = root
	child := c.Checks["child"]
	child.Steps = nil
	for i := 0; i < 32; i++ {
		child.Steps = append(child.Steps, Step{ID: strings.Repeat("x", i+1), Kind: "check", CheckID: "last", Bindings: []Binding{{Input: "id", FromInput: "id"}}})
	}
	c.Checks["child"] = child
	last := root
	last.ID = "last"
	last.Steps = []Step{{ID: "end", Kind: "query", QueryID: "query", Bindings: []Binding{{Input: "id", FromInput: "id"}}}}
	c.Checks["last"] = last
	require.Equal(t, BindingInputBindable, PreflightBindings(c, "root", v, m).State)
}

func TestBindingPreflightNeverExecutesAndMatchesEngineParameters(t *testing.T) {
	c, v, m := preflightFixture()
	calls := 0
	runner := func(_ context.Context, query, environment string, params map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
		calls++
		require.Equal(t, "query", query)
		require.Equal(t, "demo", environment)
		require.Equal(t, v["id"].Scalar, params["id"].Scalar)
		return apicontract.Result{}, nil
	}
	engine := newEngine(c, "demo", runner, nil)
	require.Equal(t, BindingInputBindable, PreflightBindings(c, "root", v, m).State)
	require.Zero(t, calls)
	result := engine.runCheck(context.Background(), "root", v, 0, map[string]bool{})
	require.Equal(t, 1, calls)
	require.Equal(t, "completed", result.ExecutionStatus)
}

func TestBindingPreflightRejectsCorruptInputAndQueryBounds(t *testing.T) {
	c, v, m := preflightFixture()
	for i := 0; i <= maxNodes; i++ {
		v[strings.Repeat("x", i+1)] = v["id"]
	}
	require.Equal(t, BindingInvalidDefinition, PreflightBindings(c, "root", v, m).State)
	c, v, m = preflightFixture()
	root := c.Checks["root"]
	root.Inputs = nil
	for i := 0; i <= maxNodes; i++ {
		root.Inputs = append(root.Inputs, Input{Name: fmt.Sprintf("input%d", i), Type: investigation.ValueTypeInteger, Cardinality: "scalar"})
	}
	root.Steps[0].Bindings = nil
	c.Checks["root"] = root
	require.Equal(t, BindingInvalidDefinition, PreflightBindings(c, "root", v, m).State)
	c, v, m = preflightFixture()
	root = c.Checks["root"]
	root.Steps[0].QueryID = "../invalid"
	c.Checks["root"] = root
	m["../invalid"] = m["query"]
	require.Equal(t, BindingInvalidDefinition, PreflightBindings(c, "root", v, m).State)
}
