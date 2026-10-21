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

func TestIndependentReviewCachedDAG(t *testing.T) {
	c, v, m := preflightFixture()
	root := c.Checks["root"]
	root.Steps = nil
	for i := 0; i < 32; i++ {
		root.Steps = append(root.Steps, Step{ID: strings.Repeat("a", i+1), Kind: "check", CheckID: "child", Bindings: []Binding{{Input: "id", FromInput: "id"}}})
	}
	c.Checks["root"] = root
	child := root
	child.ID = "child"
	child.Steps = nil
	for i := 0; i < 32; i++ {
		child.Steps = append(child.Steps, Step{ID: strings.Repeat("b", i+1), Kind: "check", CheckID: "leaf", Bindings: []Binding{{Input: "id", FromInput: "id"}}})
	}
	c.Checks["child"] = child
	leaf := root
	leaf.ID = "leaf"
	leaf.Steps = []Step{{ID: "query", Kind: "query", QueryID: "query", Bindings: []Binding{{Input: "id", FromInput: "id"}}}}
	c.Checks["leaf"] = leaf
	require.NoError(t, ValidateCatalog(c))
	preflight := PreflightBindings(c, "root", v, m)
	calls := 0
	engine := newEngine(c, "demo", func(context.Context, string, string, map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
		calls++
		return apicontract.Result{}, nil
	}, nil)
	actual := engine.runCheck(context.Background(), "root", v, 0, map[string]bool{})
	t.Logf("preflight=%s engine.status=%s nodes=%d runnerCalls=%d", preflight.State, actual.ExecutionStatus, engine.nodes, calls)
	require.Equal(t, "completed", actual.ExecutionStatus)
	require.Equal(t, 3, engine.nodes)
	require.Equal(t, 1, calls)
	require.Equal(t, BindingInputBindable, preflight.State)
}
func TestIndependentReviewMixedRowMissingInput(t *testing.T) {
	c, v, m := preflightFixture()
	root := c.Checks["root"]
	root.Inputs = append(root.Inputs, Input{Name: "optional", Type: investigation.ValueTypeInteger, Cardinality: "scalar"})
	root.Steps[0].Validators = []Validator{{ID: "row", Scope: "row", Kind: "check", Severity: "error", CheckID: "child", Bindings: []Binding{{Input: "id", FromColumn: "id"}, {Input: "other", FromInput: "optional"}}}}
	c.Checks["root"] = root
	child := Check{ID: "child", Title: "Child", Version: 1, Inputs: []Input{{Name: "id", Type: investigation.ValueTypeInteger, Cardinality: "scalar", Required: true}, {Name: "other", Type: investigation.ValueTypeInteger, Cardinality: "scalar", Required: true}}, Steps: []Step{{ID: "query", Kind: "query", QueryID: "child-query"}}}
	c.Checks["child"] = child
	m["child-query"] = nil
	require.NoError(t, ValidateCatalog(c))
	preflight := PreflightBindings(c, "root", v, m)
	_, err := mapBindings(root.Steps[0].Validators[0].Bindings, v, []apicontract.Column{{Name: "id"}}, []Value{*v["id"].Scalar})
	t.Logf("preflight=%s realRowMapperError=%v", preflight.State, err)
	require.Error(t, err)
	require.Equal(t, BindingMissingInput, preflight.State)
}
func TestIndependentReviewRequiredNullQuery(t *testing.T) {
	c, v, m := preflightFixture()
	null := Value{Type: investigation.ValueTypeNull}
	v["id"] = InputValue{Scalar: &null}
	t.Logf("required query parameter null preflight=%s", PreflightBindings(c, "root", v, m).State)
	require.Equal(t, BindingMissingInput, PreflightBindings(c, "root", v, m).State)
}

func TestPreflightDeferredRowStillValidatesDeterministicDescendants(t *testing.T) {
	c, v, m := preflightFixture()
	root := c.Checks["root"]
	root.Steps[0].Validators = []Validator{{ID: "row", Scope: "row", Kind: "check", Severity: "error", CheckID: "child", Bindings: []Binding{{Input: "id", FromColumn: "id"}}}}
	c.Checks["root"] = root
	c.Checks["child"] = Check{ID: "child", Title: "Child", Version: 1, Inputs: []Input{{Name: "id", Type: investigation.ValueTypeInteger, Cardinality: "scalar", Required: true}, {Name: "optional", Type: investigation.ValueTypeInteger, Cardinality: "scalar"}}, Steps: []Step{{ID: "query", Kind: "query", QueryID: "child-query", Bindings: []Binding{{Input: "other", FromInput: "optional"}}}}}
	m["child-query"] = []Input{{Name: "other", Type: investigation.ValueTypeInteger, Cardinality: "scalar", Required: true}}
	require.NoError(t, ValidateCatalog(c))
	require.Equal(t, BindingMissingInput, PreflightBindings(c, "root", v, m).State)
	child := c.Checks["child"]
	child.Steps[0].Bindings = nil
	c.Checks["child"] = child
	require.Equal(t, BindingMissingInput, PreflightBindings(c, "root", v, m).State)
	child.Steps[0].Bindings = []Binding{{Input: "other", FromInput: "id"}}
	c.Checks["child"] = child
	require.Equal(t, BindingDeferredRow, PreflightBindings(c, "root", v, m).State)
}
func TestPreflightQueryNullAndOptionalAdmission(t *testing.T) {
	for _, required := range []bool{true, false} {
		for _, present := range []bool{true, false} {
			c, v, m := preflightFixture()
			root := c.Checks["root"]
			root.Inputs[0].Required = false
			c.Checks["root"] = root
			m["query"][0].Required = required
			null := Value{Type: investigation.ValueTypeNull}
			v["id"] = InputValue{Scalar: &null}
			if !present {
				root.Steps[0].Bindings = nil
				c.Checks["root"] = root
			}
			expected := BindingInputBindable
			if required {
				expected = BindingMissingInput
			}
			require.Equal(t, expected, PreflightBindings(c, "root", v, m).State)
		}
	}
}

func TestPreflightQueryParameterContractSetsAndUnknowns(t *testing.T) {
	null := Value{Type: investigation.ValueTypeNull}
	integer := investigation.NewIntegerValue("1")
	text := investigation.NewStringValue("one")
	for _, required := range []bool{true, false} {
		schema := []Input{{Name: "id", Type: investigation.ValueTypeInteger, Cardinality: "set", Required: required}}
		for _, item := range []Value{integer, null, text} {
			values := map[string]InputValue{"id": {Set: &apicontract.TypedValueSet{Values: []Value{item}}}}
			err := validatePartialInputs(schema, values, nil, true)
			require.Equal(t, item.Type != investigation.ValueTypeInteger, err != nil)
		}
	}
	schema := []Input{{Name: "id", Type: investigation.ValueTypeInteger, Cardinality: "scalar", Required: true}}
	for _, values := range []map[string]InputValue{{"other": {Scalar: &integer}}, {"id": {}}, {"id": {Range: &ValueRange{Type: "range", Start: integer, End: integer}}}} {
		require.Error(t, validatePartialInputs(schema, values, nil, true))
	}
	require.NoError(t, validatePartialInputs(schema, nil, map[string]bool{"id": true}, true))
}

func TestPreflightUniqueInputCacheCapacityMatchesEngine(t *testing.T) {
	c, _, m := preflightFixture()
	values := map[string]InputValue{}
	schema := []Input{}
	for i := 0; i < 32; i++ {
		name := fmt.Sprintf("input%d", i)
		value := investigation.NewIntegerValue(fmt.Sprint(i))
		values[name] = InputValue{Scalar: &value}
		schema = append(schema, Input{Name: name, Type: investigation.ValueTypeInteger, Cardinality: "scalar", Required: true})
	}
	root := Check{ID: "root", Title: "Root", Version: 1, Inputs: schema}
	child := Check{ID: "child", Title: "Child", Version: 1, Inputs: schema}
	for i := 0; i < 32; i++ {
		bindings := []Binding{}
		for j := 0; j < 32; j++ {
			source := j
			switch j {
			case 0:
				source = i
			case i:
				source = 0
			}
			bindings = append(bindings, Binding{Input: fmt.Sprintf("input%d", j), FromInput: fmt.Sprintf("input%d", source)})
		}
		root.Steps = append(root.Steps, Step{ID: fmt.Sprintf("step-%d", i), Kind: "check", CheckID: "child", Bindings: bindings})
		child.Steps = append(child.Steps, Step{ID: fmt.Sprintf("step-%d", i), Kind: "check", CheckID: "leaf", Bindings: []Binding{{Input: "a", FromInput: "input0"}, {Input: "b", FromInput: fmt.Sprintf("input%d", i)}}})
	}
	leaf := Check{ID: "leaf", Title: "Leaf", Version: 1, Inputs: []Input{{Name: "a", Type: investigation.ValueTypeInteger, Cardinality: "scalar", Required: true}, {Name: "b", Type: investigation.ValueTypeInteger, Cardinality: "scalar", Required: true}}, Steps: []Step{{ID: "query", Kind: "query", QueryID: "query"}}}
	c.Checks = map[string]Check{"root": root, "child": child, "leaf": leaf}
	m["query"] = nil
	require.NoError(t, ValidateCatalog(c))
	require.Equal(t, BindingInvalidDefinition, PreflightBindings(c, "root", values, m).State)
	engine := newEngine(c, "demo", func(context.Context, string, string, map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
		return apicontract.Result{}, nil
	}, nil)
	actual := engine.runCheck(context.Background(), "root", values, 0, map[string]bool{})
	require.Equal(t, "error", actual.ExecutionStatus)
}

func TestIndependentR2UnknownQueryTarget(t *testing.T) {
	for _, mode := range []string{"undeclared", "set"} {
		t.Run(mode, func(t *testing.T) {
			c, v, m := preflightFixture()
			root := c.Checks["root"]
			root.Steps[0].Validators = []Validator{{ID: "row", Scope: "row", Kind: "check", Severity: "error", CheckID: "child", Bindings: []Binding{{Input: "id", FromColumn: "id"}}}}
			c.Checks["root"] = root
			c.Checks["child"] = Check{ID: "child", Title: "Child", Version: 1, Inputs: []Input{{Name: "id", Type: investigation.ValueTypeInteger, Cardinality: "scalar", Required: true}}, Steps: []Step{{ID: "query", Kind: "query", QueryID: "child-query", Bindings: []Binding{{Input: "id", FromInput: "id"}}}}}
			if mode == "undeclared" {
				m["child-query"] = nil
			} else {
				m["child-query"] = []Input{{Name: "id", Type: investigation.ValueTypeInteger, Cardinality: "set", Required: true}}
			}
			require.NoError(t, ValidateCatalog(c))
			preflight := PreflightBindings(c, "root", v, m)
			mapped, err := mapBindings(c.Checks["child"].Steps[0].Bindings, v, nil, nil)
			require.NoError(t, err)
			actualErr := validatePartialInputs(m["child-query"], mapped, nil, true)
			t.Logf("mode=%s preflight=%s actual query admission=%v", mode, preflight.State, actualErr)
			require.Error(t, actualErr)
			require.Equal(t, BindingMissingInput, preflight.State)
		})
	}
}
func TestIndependentR2CacheAtExactCapacity(t *testing.T) {
	c, _, m := preflightFixture()
	c.Checks = map[string]Check{}
	values := map[string]InputValue{}
	schema := []Input{}
	for i := 0; i < 31; i++ {
		name := fmt.Sprintf("input%d", i)
		value := investigation.NewIntegerValue(fmt.Sprint(i))
		values[name] = InputValue{Scalar: &value}
		schema = append(schema, Input{Name: name, Type: investigation.ValueTypeInteger, Cardinality: "scalar", Required: true})
	}
	root := Check{ID: "root", Title: "Root", Version: 1, Inputs: schema}
	bindings := []Binding{}
	for _, in := range schema {
		bindings = append(bindings, Binding{Input: in.Name, FromInput: in.Name})
	}
	for i := 0; i < 4; i++ {
		id := fmt.Sprintf("child-%d", i)
		child := Check{ID: id, Title: id, Version: 1, Inputs: schema}
		count := 31
		if i == 3 {
			count = 30
		}
		for j := 0; j < count; j++ {
			child.Steps = append(child.Steps, Step{ID: fmt.Sprintf("step-%d", j), Kind: "check", CheckID: fmt.Sprintf("leaf-%d", i), Bindings: []Binding{{Input: "id", FromInput: fmt.Sprintf("input%d", j)}}})
		}
		c.Checks[id] = child
		root.Steps = append(root.Steps, Step{ID: id, Kind: "check", CheckID: id, Bindings: bindings})
		leafID := fmt.Sprintf("leaf-%d", i)
		c.Checks[leafID] = Check{ID: leafID, Title: leafID, Version: 1, Inputs: []Input{{Name: "id", Type: investigation.ValueTypeInteger, Cardinality: "scalar", Required: true}}, Steps: []Step{{ID: "query", Kind: "query", QueryID: "query"}}}
	}
	root.Steps = append(root.Steps, Step{ID: "repeat", Kind: "check", CheckID: "child-0", Bindings: bindings})
	c.Checks["root"] = root
	m["query"] = nil
	require.NoError(t, ValidateCatalog(c))
	preflight := PreflightBindings(c, "root", values, m)
	engine := newEngine(c, "demo", func(context.Context, string, string, map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
		return apicontract.Result{}, nil
	}, nil)
	actual := engine.runCheck(context.Background(), "root", values, 0, map[string]bool{})
	t.Logf("preflight=%s actualEngine=%s engineNodes=%d", preflight.State, actual.ExecutionStatus, engine.nodes)
	require.Equal(t, 128, engine.nodes)
	require.Equal(t, "error", actual.ExecutionStatus)
	require.Equal(t, BindingInvalidDefinition, preflight.State)
}

func TestPendingRangeEndpointShapeMatchesActualMapping(t *testing.T) {
	source := []Input{{Name: "window", Type: investigation.ValueTypeInteger, Cardinality: "range", Required: true}}
	target := []Input{{Name: "id", Type: investigation.ValueTypeInteger, Cardinality: "scalar", Required: true}}
	bindings := []Binding{{Input: "id", FromInput: "window", Part: "end"}}
	one := investigation.NewIntegerValue("1")
	two := investigation.NewIntegerValue("2")
	mapped, err := mapBindings(bindings, map[string]InputValue{"window": {Range: &ValueRange{Type: "range", Start: one, End: two}}}, nil, nil)
	require.NoError(t, err)
	require.NoError(t, validatePartialInputs(target, mapped, nil, true))
	require.NoError(t, validatePendingQueryBindings(source, target, bindings, map[string]bool{"id": true}))
	target[0].Cardinality = "set"
	require.Error(t, validatePendingQueryBindings(source, target, bindings, map[string]bool{"id": true}))
}
