package detective

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/datatug/datatug-core/pkg/investigation"
)

type failingCatalogReader struct {
	mapReader
	checkErr, validatorErr, localErr, queryErr error
}

func (r failingCatalogReader) GetCheck(ctx context.Context, id string) (Check, error) {
	if r.checkErr != nil {
		return Check{}, r.checkErr
	}
	return r.mapReader.GetCheck(ctx, id)
}
func (r failingCatalogReader) GetPublishedValidator(ctx context.Context, id string) (PublishedValidator, error) {
	if r.validatorErr != nil {
		return PublishedValidator{}, r.validatorErr
	}
	return r.mapReader.GetPublishedValidator(ctx, id)
}
func (r failingCatalogReader) GetQueryValidators(ctx context.Context, id string) (QueryValidators, bool, error) {
	if r.localErr != nil {
		return QueryValidators{}, false, r.localErr
	}
	return r.mapReader.GetQueryValidators(ctx, id)
}
func (r failingCatalogReader) QueryExists(ctx context.Context, id string) (bool, error) {
	if r.queryErr != nil {
		return false, r.queryErr
	}
	return r.mapReader.QueryExists(ctx, id)
}

func TestCatalogLoadingRejectsMissingMalformedAndUnavailableDependencies(t *testing.T) {
	ctx := context.Background()
	base := mapReader{checks: map[string]Check{"root": {ID: "root", Title: "Root", Steps: []Step{{ID: "query", Kind: "query", QueryID: "q"}}}}, validators: map[string]PublishedValidator{}, locals: map[string]QueryValidators{}, queries: map[string]bool{"q": true}}
	for _, roots := range [][2]string{{"", ""}, {"root", "q"}} {
		if _, err := LoadCatalog(ctx, roots[0], roots[1], base); !errors.Is(err, ErrInvalid) {
			t.Fatalf("ambiguous roots accepted: %v", err)
		}
	}
	if _, err := LoadCatalog(ctx, "root", "", nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("nil repository accepted: %v", err)
	}
	for _, tc := range []struct {
		name   string
		reader failingCatalogReader
		check  string
		query  string
		want   error
	}{
		{"check storage error", failingCatalogReader{mapReader: base, checkErr: errors.New("storage")}, "root", "", nil},
		{"query existence error", failingCatalogReader{mapReader: base, queryErr: errors.New("storage")}, "root", "", nil},
		{"query local error", failingCatalogReader{mapReader: base, localErr: errors.New("storage")}, "root", "", nil},
		{"invalid root query", failingCatalogReader{mapReader: base}, "", "../escape", ErrInvalid},
		{"missing query", failingCatalogReader{mapReader: base}, "", "missing", ErrNotFound},
		{"missing check", failingCatalogReader{mapReader: base}, "missing", "", ErrNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := LoadCatalog(ctx, tc.check, tc.query, tc.reader)
			if err == nil || tc.want != nil && !errors.Is(err, tc.want) {
				t.Fatalf("catalog error: %v", err)
			}
		})
	}
	malformed := base
	malformed.checks = map[string]Check{"root": {ID: "other", Title: "Root", Steps: []Step{{ID: "query", Kind: "query", QueryID: "q"}}}}
	if _, err := LoadCatalog(ctx, "root", "", malformed); !errors.Is(err, ErrInvalid) {
		t.Fatalf("wrong Check identity accepted: %v", err)
	}
	malformed = base
	malformed.locals = map[string]QueryValidators{"q": {QueryID: "other", Validators: []Validator{{ID: "required", Kind: "non-null", Scope: "row", Severity: "error", Field: "x"}}}}
	if _, err := LoadCatalog(ctx, "", "q", malformed); !errors.Is(err, ErrInvalid) {
		t.Fatalf("wrong query-local identity accepted: %v", err)
	}
	malformed = base
	malformed.locals = map[string]QueryValidators{"q": {QueryID: "q", Validators: []Validator{{ID: "ref", ValidatorID: "published"}}}}
	if _, err := LoadCatalog(ctx, "", "q", failingCatalogReader{mapReader: malformed, validatorErr: errors.New("storage")}); err == nil {
		t.Fatal("promoted catalog storage error ignored")
	}
	malformed.validators = map[string]PublishedValidator{"published": {ID: "other", Title: "Invalid", Validator: Validator{ID: "required", Kind: "non-null", Scope: "row", Severity: "error", Field: "x"}}}
	if _, err := LoadCatalog(ctx, "", "q", malformed); !errors.Is(err, ErrInvalid) {
		t.Fatalf("promoted identity mismatch accepted: %v", err)
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := LoadCatalog(cancelled, "root", "", base); !errors.Is(err, context.Canceled) {
		t.Fatalf("catalog ignored cancellation before dependency walk: %v", err)
	}
}

func TestCatalogGraphChecksEveryNestedBindingBeforeExecution(t *testing.T) {
	input := Input{Name: "invoice_id", Type: investigation.ValueTypeInteger, Cardinality: "scalar", Required: true}
	root := Check{ID: "root", Title: "Root", Inputs: []Input{input}, Steps: []Step{{ID: "query", Kind: "query", QueryID: "q", Bindings: []Binding{{Input: "invoice_id", FromInput: "invoice_id"}}}}}
	child := Check{ID: "child", Title: "Child", Inputs: []Input{input}, Steps: []Step{{ID: "query", Kind: "query", QueryID: "q2"}}}
	local := QueryValidators{QueryID: "q", Validators: []Validator{{ID: "each", Kind: "check", Scope: "row", Severity: "error", CheckID: "child", Bindings: []Binding{{Input: "invoice_id", FromColumn: "invoice_id"}}}}}
	check := func(t *testing.T, checks map[string]Check, locals map[string]QueryValidators, validators map[string]PublishedValidator, want error) {
		t.Helper()
		err := ValidateCatalog(Catalog{Checks: checks, Validators: validators, QueryValidators: locals})
		if want == nil && err != nil || want != nil && !errors.Is(err, want) {
			t.Fatalf("graph validation: got %v want %v", err, want)
		}
	}
	check(t, map[string]Check{"root": root}, nil, nil, nil)
	check(t, map[string]Check{"wrong": root}, nil, nil, ErrInvalid)
	check(t, map[string]Check{"root": root}, map[string]QueryValidators{"wrong": local}, nil, ErrInvalid)
	check(t, map[string]Check{"root": root}, nil, map[string]PublishedValidator{"wrong": {ID: "published", Title: "Published", Validator: Validator{ID: "required", Kind: "non-null", Scope: "row", Severity: "error", Field: "x"}}}, ErrInvalid)
	missing := root
	missing.Steps = []Step{{ID: "child", Kind: "check", CheckID: "child"}}
	check(t, map[string]Check{"root": missing}, nil, nil, ErrNotFound)
	check(t, map[string]Check{"root": missing, "child": child}, nil, nil, ErrInvalid)
	missing.Steps[0].Bindings = []Binding{{Input: "invoice_id", FromInput: "invoice_id"}}
	check(t, map[string]Check{"root": missing, "child": child}, nil, nil, nil)
	malformed := root
	malformed.Steps = []Step{{ID: "query", Kind: "query", QueryID: "q", Bindings: []Binding{{Input: "invoice_id", FromColumn: "invoice_id"}}}}
	check(t, map[string]Check{"root": malformed}, nil, nil, ErrInvalid)
	malformed.Steps[0].Bindings = []Binding{{Input: "invoice_id", FromInput: "unknown"}}
	check(t, map[string]Check{"root": malformed}, nil, nil, ErrInvalid)
	check(t, map[string]Check{"root": root, "child": child}, map[string]QueryValidators{"q": local}, nil, nil)
	check(t, map[string]Check{"root": root}, map[string]QueryValidators{"q": local}, nil, ErrNotFound)
	local.Validators[0].Bindings = nil
	check(t, map[string]Check{"root": root, "child": child}, map[string]QueryValidators{"q": local}, nil, ErrInvalid)
	local.Validators[0].Bindings = []Binding{{Input: "invoice_id", FromColumn: "invoice_id"}}
	local.Validators[0].ValidatorID = "published"
	local.Validators[0].Kind = ""
	check(t, map[string]Check{"root": root, "child": child}, map[string]QueryValidators{"q": local}, nil, ErrNotFound)
	check(t, map[string]Check{"root": root, "child": child}, map[string]QueryValidators{"q": local}, map[string]PublishedValidator{"published": {ID: "published", Title: "Published", Validator: Validator{ID: "each", Kind: "check", Scope: "row", Severity: "error", CheckID: "child", Bindings: []Binding{{Input: "invoice_id", FromColumn: "invoice_id"}}}}}, nil)
}

func TestGraphRejectsCyclesAndDanglingPromotionsAcrossEntryPaths(t *testing.T) {
	base := Check{ID: "root", Title: "Root", Steps: []Step{{ID: "q", Kind: "query", QueryID: "q"}}}
	loop := base
	loop.Steps = []Step{{ID: "self", Kind: "check", CheckID: "root"}}
	if err := ValidateCatalog(Catalog{Checks: map[string]Check{"root": loop}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("direct cycle accepted: %v", err)
	}
	child := Check{ID: "child", Title: "Child", Inputs: []Input{{Name: "id", Type: investigation.ValueTypeInteger, Cardinality: "scalar", Required: true}}, Steps: []Step{{ID: "q", Kind: "query", QueryID: "q"}}}
	parent := Check{ID: "root", Title: "Root", Inputs: []Input{{Name: "id", Type: investigation.ValueTypeString, Cardinality: "scalar"}}, Steps: []Step{{ID: "child", Kind: "check", CheckID: "child", Bindings: []Binding{{Input: "id", FromInput: "id"}}}}}
	if err := ValidateCatalog(Catalog{Checks: map[string]Check{"root": parent, "child": child}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("type-mismatched child binding accepted: %v", err)
	}
	local := QueryValidators{QueryID: "q", Validators: []Validator{{ID: "ref", ValidatorID: "missing"}}}
	if err := ValidateCatalog(Catalog{Checks: map[string]Check{"root": base}, QueryValidators: map[string]QueryValidators{"q": local}}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("dangling local promoted Validator accepted: %v", err)
	}
	local.Validators[0] = Validator{ID: "child", Kind: "check", Scope: "row", Severity: "error", CheckID: "missing"}
	if err := ValidateCatalog(Catalog{Checks: map[string]Check{"root": base}, QueryValidators: map[string]QueryValidators{"q": local}}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("dangling local child Check accepted: %v", err)
	}
	local.Validators[0].CheckID = "root"
	if err := ValidateCatalog(Catalog{Checks: map[string]Check{"root": base}, QueryValidators: map[string]QueryValidators{"q": local}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("cycle through query-local Validator accepted: %v", err)
	}
	local.Validators[0].CheckID = "child"
	if err := ValidateCatalog(Catalog{Checks: map[string]Check{"root": base, "child": child}, QueryValidators: map[string]QueryValidators{"q": local}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing local child binding accepted: %v", err)
	}
}

func TestCatalogBoundsWideDependencyGraphs(t *testing.T) {
	ctx := context.Background()
	reader := mapReader{checks: map[string]Check{}, validators: map[string]PublishedValidator{}, locals: map[string]QueryValidators{}, queries: map[string]bool{}}
	root := Check{ID: "root", Title: "Root"}
	for i := 0; i < 5; i++ {
		childID := "child-" + strconv.Itoa(i)
		root.Steps = append(root.Steps, Step{ID: childID, Kind: "check", CheckID: childID})
		child := Check{ID: childID, Title: childID}
		for j := 0; j < 32; j++ {
			queryID := "query-" + strconv.Itoa(i) + "-" + strconv.Itoa(j)
			child.Steps = append(child.Steps, Step{ID: "step-" + strconv.Itoa(j), Kind: "query", QueryID: queryID})
			reader.queries[queryID] = true
		}
		reader.checks[childID] = child
	}
	reader.checks["root"] = root
	if _, err := LoadCatalog(ctx, "root", "", reader); !errors.Is(err, ErrCapacity) {
		t.Fatalf("more than %d distinct query dependencies accepted: %v", maxNodes, err)
	}
	reader = mapReader{checks: map[string]Check{}, validators: map[string]PublishedValidator{}, locals: map[string]QueryValidators{}, queries: map[string]bool{"q": true}}
	root = Check{ID: "root", Title: "Root"}
	for i := 0; i < 5; i++ {
		childID := "child-" + strconv.Itoa(i)
		root.Steps = append(root.Steps, Step{ID: childID, Kind: "check", CheckID: childID})
		child := Check{ID: childID, Title: childID}
		for j := 0; j < 32; j++ {
			grandID := "grand-" + strconv.Itoa(i) + "-" + strconv.Itoa(j)
			child.Steps = append(child.Steps, Step{ID: "step-" + strconv.Itoa(j), Kind: "check", CheckID: grandID})
			reader.checks[grandID] = Check{ID: grandID, Title: grandID, Steps: []Step{{ID: "q", Kind: "query", QueryID: "q"}}}
		}
		reader.checks[childID] = child
	}
	reader.checks["root"] = root
	if _, err := LoadCatalog(ctx, "root", "", reader); !errors.Is(err, ErrCapacity) {
		t.Fatalf("more than %d Check dependencies accepted: %v", maxNodes, err)
	}
	if ValidSlug("root_underscore") || ValidQueryRef(strings.Repeat("a", 513)) || ValidQueryRef("invoice/@bad") {
		t.Fatal("catalog path/slug bounds changed")
	}
}

func TestCatalogRejectsInlineMissingPromotionAndSharesOneCheckedChild(t *testing.T) {
	reader := mapReader{checks: map[string]Check{}, validators: map[string]PublishedValidator{}, locals: map[string]QueryValidators{}, queries: map[string]bool{"q": true}}
	reader.checks["root"] = Check{ID: "root", Title: "Root", Steps: []Step{{ID: "q", Kind: "query", QueryID: "q", Validators: []Validator{{ID: "ref", ValidatorID: "missing"}}}}}
	if _, err := LoadCatalog(context.Background(), "root", "", reader); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing inline published Validator accepted: %v", err)
	}
	reader.checks["child"] = Check{ID: "child", Title: "Child", Steps: []Step{{ID: "q", Kind: "query", QueryID: "q"}}}
	reader.checks["root"] = Check{ID: "root", Title: "Root", Steps: []Step{{ID: "first", Kind: "check", CheckID: "child"}, {ID: "second", Kind: "check", CheckID: "child"}}}
	catalog, err := LoadCatalog(context.Background(), "root", "", reader)
	if err != nil || len(catalog.Checks) != 2 {
		t.Fatalf("shared child could not be loaded once: %+v %v", catalog.Checks, err)
	}
}

func TestStandaloneQueryLocalGraphValidatesCheckReferencesAndBindings(t *testing.T) {
	local := QueryValidators{QueryID: "query", Validators: []Validator{{ID: "child", Kind: "check", Scope: "row", Severity: "error", CheckID: "child"}}}
	if err := ValidateCatalog(Catalog{QueryValidators: map[string]QueryValidators{"query": local}}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing standalone query-local child accepted: %v", err)
	}
	child := Check{ID: "child", Title: "Child", Inputs: []Input{{Name: "id", Type: investigation.ValueTypeInteger, Cardinality: "scalar", Required: true}}, Steps: []Step{{ID: "source", Kind: "query", QueryID: "source"}}}
	if err := ValidateCatalog(Catalog{Checks: map[string]Check{"child": child}, QueryValidators: map[string]QueryValidators{"query": local}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("missing standalone local child binding accepted: %v", err)
	}
	local.Validators[0].Bindings = []Binding{{Input: "id", FromColumn: "id"}}
	if err := ValidateCatalog(Catalog{Checks: map[string]Check{"child": child}, QueryValidators: map[string]QueryValidators{"query": local}}); err != nil {
		t.Fatalf("valid standalone local child rejected: %v", err)
	}
}
