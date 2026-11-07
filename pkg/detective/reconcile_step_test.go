package detective

import (
	"context"
	"errors"
	"testing"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/reconcile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func reconcileCatalog(t *testing.T, steps ...Step) Catalog {
	t.Helper()
	check := Check{ID: "root", Title: "Reference data agrees", Steps: steps}
	reader := mapReader{checks: map[string]Check{"root": check}, validators: map[string]PublishedValidator{}, locals: map[string]QueryValidators{}, queries: map[string]bool{"q": true}}
	catalog, err := LoadCatalog(context.Background(), "root", "", reader)
	require.NoError(t, err)
	return catalog
}

func runReconcileCheck(t *testing.T, ctx context.Context, catalog Catalog, options ...Option) CheckRun {
	t.Helper()
	runner := func(context.Context, string, string, map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
		return coreResult([]string{"x"}, nil), nil
	}
	run, err := EvaluateCheck(ctx, RunRequest{ID: "run-1", CheckID: "root", Environment: "factory-a-prod"}, catalog, runner, nil, options...)
	require.NoError(t, err)
	return run
}

func TestReconcileStepOutcomes(t *testing.T) {
	catalog := reconcileCatalog(t, Step{ID: "countries", Kind: "reconcile", Profile: "reference-countries"})
	cases := map[reconcile.Status]struct{ outcome, severity string }{
		reconcile.StatusConsistent:    {"pass", ""},
		reconcile.StatusDiscrepancies: {"fail", "error"},
		reconcile.StatusPartial:       {"unknown", ""},
		reconcile.StatusInconclusive:  {"unknown", ""},
	}
	for status, want := range cases {
		t.Run(string(status), func(t *testing.T) {
			var asked string
			run := runReconcileCheck(t, context.Background(), catalog, WithReconciler(func(_ context.Context, profile string) (reconcile.Result, error) {
				asked = profile
				return reconcile.Result{Status: status}, nil
			}))
			assert.Equal(t, "reference-countries", asked)
			require.Len(t, run.Steps, 1)
			step := run.Steps[0]
			assert.Equal(t, "reconcile", step.Kind)
			assert.Equal(t, "reference-countries", step.Profile)
			assert.Equal(t, "completed", step.ExecutionStatus)
			assert.Equal(t, want.outcome, step.Outcome)
			assert.Equal(t, want.severity, step.Severity)
			require.NotNil(t, step.Reconciliation)
			assert.Equal(t, status, step.Reconciliation.Status)
			assert.Equal(t, want.outcome, run.Outcome)
			assert.Equal(t, 0, run.QueryExecutions)
		})
	}
}

func TestReconcileStepNeverPassesWithoutAResult(t *testing.T) {
	catalog := reconcileCatalog(t, Step{ID: "countries", Kind: "reconcile", Profile: "reference-countries", OnUnavailable: "warn"})

	run := runReconcileCheck(t, context.Background(), catalog)
	assert.Equal(t, ReconcileUnavailableCode, run.Steps[0].ErrorCode)
	assert.Equal(t, "unknown", run.Outcome)

	run = runReconcileCheck(t, context.Background(), catalog, WithReconciler(func(context.Context, string) (reconcile.Result, error) {
		return reconcile.Result{}, &QueryFailure{Code: "PERMISSION_DENIED"}
	}))
	assert.Equal(t, "PERMISSION_DENIED", run.Steps[0].ErrorCode)
	assert.Equal(t, "error", run.Steps[0].ExecutionStatus)
	assert.Nil(t, run.Steps[0].Reconciliation)

	run = runReconcileCheck(t, context.Background(), catalog, WithReconciler(func(context.Context, string) (reconcile.Result, error) {
		return reconcile.Result{}, errors.New("driver said secret things")
	}))
	assert.Equal(t, "INTERNAL", run.Steps[0].ErrorCode)
	assert.NotContains(t, run.Steps[0].Message, "secret")

	// A deadline reached between steps stops the reconcile step before it runs.
	two := reconcileCatalog(t, Step{ID: "rows", Kind: "query", QueryID: "q"}, Step{ID: "countries", Kind: "reconcile", Profile: "reference-countries"})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	runner := func(context.Context, string, string, map[string]apicontract.TypedValueOrSet) (apicontract.Result, error) {
		cancel()
		return coreResult([]string{"x"}, nil), nil
	}
	run, err := EvaluateCheck(ctx, RunRequest{ID: "run-1", CheckID: "root", Environment: "factory-a-prod"}, two, runner, nil, WithReconciler(func(context.Context, string) (reconcile.Result, error) {
		t.Fatal("must not run after cancellation")
		return reconcile.Result{}, nil
	}))
	require.NoError(t, err)
	require.Len(t, run.Steps, 2)
	assert.Equal(t, "TIMEOUT", run.Steps[1].ErrorCode)
	assert.NotEqual(t, "pass", run.Outcome)
}

func TestReconcileStepBesideQueryStep(t *testing.T) {
	catalog := reconcileCatalog(t,
		Step{ID: "rows", Kind: "query", QueryID: "q"},
		Step{ID: "countries", Kind: "reconcile", Profile: "reference-countries"},
	)
	run := runReconcileCheck(t, context.Background(), catalog, WithReconciler(func(context.Context, string) (reconcile.Result, error) {
		return reconcile.Result{Status: reconcile.StatusDiscrepancies}, nil
	}))
	require.Len(t, run.Steps, 2)
	assert.Equal(t, "fail", run.Outcome)
	assert.Equal(t, 1, run.QueryExecutions)
	preflight := PreflightBindings(catalog, "root", map[string]InputValue{}, map[string][]Input{"q": {}})
	assert.Equal(t, BindingInputBindable, preflight.State)
}

func TestReconcileStepValidation(t *testing.T) {
	valid := Step{ID: "countries", Kind: "reconcile", Profile: "reference-countries"}
	assert.NoError(t, ValidateCheck(Check{ID: "root", Title: "T", Steps: []Step{valid}}))
	for name, mutate := range map[string]func(*Step){
		"no profile":    func(s *Step) { s.Profile = "" },
		"bad profile":   func(s *Step) { s.Profile = "Bad" },
		"query id":      func(s *Step) { s.QueryID = "q" },
		"check id":      func(s *Step) { s.CheckID = "c" },
		"validators":    func(s *Step) { s.Validators = []Validator{{Kind: "row-count", Scope: "set"}} },
		"bindings":      func(s *Step) { s.Bindings = []Binding{{FromInput: "x"}} },
		"bad policy":    func(s *Step) { s.OnUnavailable = "explode" },
		"profile query": func(s *Step) { *s = Step{ID: "q", Kind: "query", QueryID: "q", Profile: "p"} },
	} {
		step := valid
		mutate(&step)
		assert.ErrorIs(t, ValidateCheck(Check{ID: "root", Title: "T", Steps: []Step{step}}), ErrInvalid, name)
	}
}
