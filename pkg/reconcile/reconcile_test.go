package reconcile

import (
	"errors"
	"testing"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func str(v string) apicontract.TypedValue  { return apicontract.NewStringValue(v) }
func num(v float64) apicontract.TypedValue { return apicontract.NewNumberValue(v) }

var countryColumns = []apicontract.Column{{Name: "code", Type: "string"}, {Name: "name", Type: "string"}}

// src builds a source from unordered rows; OrderRows orders them.
func src(t *testing.T, id string, columns []apicontract.Column, rows ...[]apicontract.TypedValue) Source {
	t.Helper()
	ordered, err := OrderRows(apicontract.Recordset{Columns: columns, Rows: rows}, []string{columns[0].Name})
	require.NoError(t, err)
	return Source{
		Instance: Instance{ID: id, Tenant: id, Environment: "production"},
		Columns:  columns, Rows: ordered,
		Info:     apicontract.CompareSideInfo{RowCount: len(rows), Limitations: []apicontract.Limitation{}},
		Snapshot: true,
	}
}

func country(code, name string) []apicontract.TypedValue {
	return []apicontract.TypedValue{str(code), str(name)}
}

func countries(t *testing.T, id string, rows ...[]apicontract.TypedValue) Source {
	return src(t, id, countryColumns, rows...)
}

func standard() [][]apicontract.TypedValue {
	return [][]apicontract.TypedValue{country("FR", "France"), country("IE", "Ireland"), country("US", "United States")}
}

func instanceStatus(result Result) map[string]Status {
	out := map[string]Status{}
	for _, instance := range result.Instances {
		out[instance.Instance.ID] = instance.Status
	}
	return out
}

func TestConsensusFindsOutlierWithoutDeclaringMajorityCorrect(t *testing.T) {
	sources := []Source{
		countries(t, "a", standard()...),
		countries(t, "b", standard()...),
		countries(t, "c", standard()...),
		countries(t, "d", country("FR", "France"), country("US", "United States")), // IE missing
		countries(t, "e", country("FR", "France"), country("IE", "Eire"), country("US", "United States")),
	}
	result, err := Reconcile(sources, Rules{Key: []string{"code"}})
	require.NoError(t, err)

	assert.Equal(t, ModeConsensus, result.Mode)
	assert.Equal(t, StatusDiscrepancies, result.Status)
	assert.Equal(t, 3, result.Summary.Records)
	assert.Equal(t, 2, result.Summary.Consistent)
	assert.Equal(t, 1, result.Summary.WithUnexpected)
	assert.Equal(t, 1, result.Summary.MissingRecords)
	assert.Equal(t, 1, result.Summary.ConflictingRecords)
	require.Len(t, result.Unexpected, 1)
	ie := result.Unexpected[0]
	assert.Equal(t, []apicontract.TypedValue{str("IE")}, ie.Key)
	assert.Equal(t, []string{"d"}, ie.Missing)
	require.Len(t, ie.Fields, 1)
	field := ie.Fields[0]
	assert.Equal(t, "name", field.Column)
	assert.False(t, field.Expected)
	assert.Equal(t, []string{"e"}, field.Deviating)
	require.Len(t, field.Groups, 2)
	assert.Equal(t, []string{"a", "b", "c"}, field.Groups[0].Instances)
	assert.Equal(t, "Ireland", field.Groups[0].Value.Str)
	assert.False(t, field.Groups[0].Authoritative, "consensus never marks a group authoritative")
	assert.Equal(t, map[string]Status{"a": StatusConsistent, "b": StatusConsistent, "c": StatusConsistent, "d": StatusDiscrepancies, "e": StatusDiscrepancies}, instanceStatus(result))
	assert.Equal(t, []string{}, result.Caveats)
}

func TestConsensusTieHasNoConsensus(t *testing.T) {
	result, err := Reconcile([]Source{
		countries(t, "a", country("IE", "Ireland")),
		countries(t, "b", country("IE", "Eire")),
	}, Rules{Key: []string{"code"}})
	require.NoError(t, err)
	field := result.Unexpected[0].Fields[0]
	assert.True(t, field.NoConsensus)
	assert.Empty(t, field.Deviating)
	assert.Equal(t, map[string]Status{"a": StatusDiscrepancies, "b": StatusDiscrepancies}, instanceStatus(result))
}

var productColumns = []apicontract.Column{{Name: "sku", Type: "string"}, {Name: "description", Type: "string"}, {Name: "price", Type: "number"}, {Name: "stock", Type: "number"}}

func product(sku, description string, price, stock float64) []apicontract.TypedValue {
	return []apicontract.TypedValue{str(sku), str(description), num(price), num(stock)}
}

func TestBaselineFieldPoliciesAndPresence(t *testing.T) {
	central := src(t, "central", productColumns, product("P1", "Bolt", 1, 0), product("P2", "Nut", 2, 0))
	a := src(t, "a", productColumns, product("P1", "Bolt", 1.5, 10), product("P2", "Nut", 2, 3))
	b := src(t, "b", productColumns, product("P1", "  BOLT ", 1, 4), product("P2", "Nut washer", 2, 5), product("P9", "Local", 1, 1))
	rules := Rules{
		Key: []string{"sku"}, Baseline: "central",
		Fields: map[string]FieldPolicy{"description": FieldNormalized, "price": FieldInstanceSpecific, "stock": FieldIgnored},
	}
	result, err := Reconcile([]Source{central, a, b}, rules)
	require.NoError(t, err)
	assert.Equal(t, ModeBaseline, result.Mode)
	assert.Equal(t, StatusDiscrepancies, result.Status)
	assert.Equal(t, 3, result.Summary.Records)
	// P1: a's price differs (expected); b's description matches after normalization.
	require.Len(t, result.Expected, 1)
	assert.Equal(t, str("P1"), result.Expected[0].Key[0])
	assert.Equal(t, "price", result.Expected[0].Fields[0].Column)
	assert.True(t, result.Expected[0].Fields[0].Expected)
	assert.True(t, result.Expected[0].Fields[0].Groups[0].Authoritative)
	assert.Equal(t, []string{"a"}, result.Expected[0].Fields[0].Deviating)
	// P2: b's description differs (unexpected); P9: extra in b (unexpected).
	require.Len(t, result.Unexpected, 2)
	assert.Equal(t, []string{"b"}, result.Unexpected[0].Fields[0].Deviating)
	assert.Equal(t, []string{"b"}, result.Unexpected[1].Extra)
	assert.False(t, result.Unexpected[1].ExtraExpected)
	statuses := instanceStatus(result)
	assert.Equal(t, StatusConsistent, statuses["central"])
	assert.Equal(t, StatusConsistent, statuses["a"], "an expected difference is not a discrepancy")
	assert.Equal(t, StatusDiscrepancies, statuses["b"])
	for _, instance := range result.Instances {
		if instance.Instance.ID == "central" {
			assert.True(t, instance.Authoritative)
		}
		if instance.Instance.ID == "a" {
			assert.Equal(t, 1, instance.Expected)
		}
		if instance.Instance.ID == "b" {
			assert.Equal(t, 1, instance.Extra)
			assert.Equal(t, 1, instance.Conflicts)
		}
	}
}

func TestBaselinePresenceRules(t *testing.T) {
	newSources := func() []Source {
		return []Source{
			countries(t, "hq", country("FR", "France"), country("IE", "Ireland")),
			countries(t, "site", country("IE", "Ireland"), country("US", "United States")),
		}
	}
	cases := []struct {
		presence                        Presence
		status                          Status
		unexpected, expected, extra, mi int
	}{
		{PresenceRequired, StatusDiscrepancies, 2, 0, 1, 1},
		{PresenceSubset, StatusDiscrepancies, 1, 1, 1, 0},
		{PresenceSuperset, StatusDiscrepancies, 1, 1, 0, 1},
		{PresenceAny, StatusConsistent, 0, 2, 0, 0},
	}
	for _, tc := range cases {
		t.Run(string(tc.presence), func(t *testing.T) {
			result, err := Reconcile(newSources(), Rules{Key: []string{"code"}, Baseline: "hq", Presence: tc.presence})
			require.NoError(t, err)
			assert.Equal(t, tc.status, result.Status)
			assert.Len(t, result.Unexpected, tc.unexpected)
			assert.Len(t, result.Expected, tc.expected)
			assert.Equal(t, tc.extra, result.Summary.ExtraRecords)
			assert.Equal(t, tc.mi, result.Summary.MissingRecords)
		})
	}
}

func TestPolicyLimitedInstancesAreNeverReportedConsistent(t *testing.T) {
	filtered := countries(t, "filtered", country("FR", "France"))
	filtered.Info.Limitations = []apicontract.Limitation{{Policy: "tenant-row-filter", RowsFiltered: true, HiddenColumns: []string{}}}
	result, err := Reconcile([]Source{countries(t, "a", standard()...), filtered}, Rules{Key: []string{"code"}})
	require.NoError(t, err)
	assert.Equal(t, StatusPartial, result.Status)
	assert.Equal(t, 2, result.Summary.WithInconclusive)
	assert.Len(t, result.Inconclusive, 2)
	assert.Equal(t, []string{"filtered"}, result.Inconclusive[0].Inconclusive)
	assert.Equal(t, StatusPartial, instanceStatus(result)["filtered"])
}

func TestHiddenColumnIsNotComparedAndMakesInstancePartial(t *testing.T) {
	hidden := countries(t, "hidden", country("FR", "Frankreich"))
	hidden.Info.Limitations = []apicontract.Limitation{{Policy: "pii", HiddenColumns: []string{"name"}}}
	result, err := Reconcile([]Source{countries(t, "a", country("FR", "France")), hidden}, Rules{Key: []string{"code"}})
	require.NoError(t, err)
	assert.Equal(t, StatusPartial, result.Status)
	assert.Equal(t, 1, result.Summary.Consistent)
	require.Len(t, result.Columns, 1)
	assert.Equal(t, []string{"hidden"}, result.Columns[0].HiddenIn)
	for _, instance := range result.Instances {
		if instance.Instance.ID == "hidden" {
			assert.Equal(t, []string{"name"}, instance.HiddenColumns)
		}
	}
}

func TestBaselineRowsFilteredMakesExtrasInconclusive(t *testing.T) {
	hq := countries(t, "hq", country("FR", "France"))
	hq.Info.Limitations = []apicontract.Limitation{{Policy: "p", RowsFiltered: true}}
	result, err := Reconcile([]Source{hq, countries(t, "site", country("FR", "France"), country("IE", "Ireland"))}, Rules{Key: []string{"code"}, Baseline: "hq"})
	require.NoError(t, err)
	assert.Equal(t, StatusPartial, result.Status)
	require.Len(t, result.Inconclusive, 1)
	assert.Equal(t, []string{"site"}, result.Inconclusive[0].Inconclusive)
}

func TestUnavailableSourcesAreInconclusive(t *testing.T) {
	denied := Source{Instance: Instance{ID: "denied"}, Unavailable: &Unavailable{Reason: UnavailablePermissionDenied, Message: "no grant"}}
	result, err := Reconcile([]Source{countries(t, "a", standard()...), countries(t, "b", standard()...), denied}, Rules{Key: []string{"code"}})
	require.NoError(t, err)
	assert.Equal(t, StatusInconclusive, result.Status)
	assert.Equal(t, 3, result.Summary.Consistent)
	assert.Equal(t, 2, result.Summary.ComparedInstances)
	statuses := instanceStatus(result)
	assert.Equal(t, StatusInconclusive, statuses["denied"])
	assert.Equal(t, StatusConsistent, statuses["a"])
	assert.Equal(t, UnavailablePermissionDenied, result.Instances[2].Unavailable.Reason)

	result, err = Reconcile([]Source{countries(t, "a", standard()...), denied}, Rules{Key: []string{"code"}})
	require.NoError(t, err)
	assert.Equal(t, StatusInconclusive, result.Status)
	assert.Equal(t, 0, result.Summary.Records)
	assert.Contains(t, result.Caveats[0], "fewer than two")

	result, err = Reconcile([]Source{denied, countries(t, "a", standard()...), countries(t, "b", standard()...)}, Rules{Key: []string{"code"}, Baseline: "denied"})
	require.NoError(t, err)
	assert.Equal(t, StatusInconclusive, result.Status)
	assert.Contains(t, result.Caveats[0], `baseline "denied" could not be read`)
	for _, instance := range result.Instances {
		assert.Equal(t, StatusInconclusive, instance.Status)
	}
}

func TestKeyProblemsMakeInstanceInconclusive(t *testing.T) {
	keyHidden := countries(t, "key-hidden", country("FR", "France"))
	keyHidden.Info.Limitations = []apicontract.Limitation{{Policy: "p", HiddenColumns: []string{"code"}}}
	noKey := src(t, "no-key", []apicontract.Column{{Name: "name", Type: "string"}}, []apicontract.TypedValue{str("France")})
	numericKey := src(t, "numeric-key", []apicontract.Column{{Name: "code", Type: "number"}, {Name: "name", Type: "string"}}, []apicontract.TypedValue{num(1), str("France")})
	result, err := Reconcile([]Source{countries(t, "a", country("FR", "France")), keyHidden, noKey, numericKey, countries(t, "b", country("FR", "France"))}, Rules{Key: []string{"code"}})
	require.NoError(t, err)
	assert.Equal(t, StatusInconclusive, result.Status)
	messages := map[string]string{}
	for _, instance := range result.Instances {
		if instance.Unavailable != nil {
			messages[instance.Instance.ID] = instance.Unavailable.Message
		}
	}
	assert.Equal(t, map[string]string{
		"key-hidden":  `key column "code" is hidden by policy`,
		"no-key":      `key column "code" is missing`,
		"numeric-key": `key column "code" has type "number", other instances use "string"`,
	}, messages)

	// The baseline sets the key type even when it is listed later.
	result, err = Reconcile([]Source{numericKey, countries(t, "hq", country("FR", "France")), countries(t, "b", country("FR", "France"))}, Rules{Key: []string{"code"}, Baseline: "hq"})
	require.NoError(t, err)
	assert.Equal(t, StatusInconclusive, instanceStatus(result)["numeric-key"])
	assert.Equal(t, StatusConsistent, instanceStatus(result)["hq"])

	// A baseline whose key is unusable leaves nothing to compare.
	result, err = Reconcile([]Source{countries(t, "a", country("FR", "France")), noKey}, Rules{Key: []string{"code"}, Baseline: "no-key"})
	require.NoError(t, err)
	assert.Equal(t, StatusInconclusive, result.Status)
}

func TestSchemaDrift(t *testing.T) {
	extraColumn := src(t, "wide", []apicontract.Column{{Name: "code", Type: "string"}, {Name: "name", Type: "string"}, {Name: "iso3", Type: "string"}}, []apicontract.TypedValue{str("FR"), str("France"), str("FRA")})
	typed := src(t, "typed", []apicontract.Column{{Name: "code", Type: "string"}, {Name: "name", Type: "number"}}, []apicontract.TypedValue{str("FR"), num(1)})
	result, err := Reconcile([]Source{countries(t, "a", country("FR", "France")), extraColumn, typed}, Rules{Key: []string{"code"}})
	require.NoError(t, err)
	require.Len(t, result.Columns, 2)
	assert.Equal(t, "iso3", result.Columns[0].Column)
	assert.Equal(t, []string{"a", "typed"}, result.Columns[0].MissingIn)
	assert.True(t, result.Columns[0].Compared)
	assert.Equal(t, "name", result.Columns[1].Column)
	assert.False(t, result.Columns[1].Compared)
	assert.Equal(t, map[string]string{"a": "string", "wide": "string", "typed": "number"}, result.Columns[1].Types)
	assert.Contains(t, result.Caveats, `column "name" has conflicting types across instances and was not compared`)
	assert.Equal(t, 1, result.Summary.Consistent)

	// In baseline mode a column the baseline lacks is reported, not judged.
	result, err = Reconcile([]Source{
		countries(t, "hq", country("FR", "France")),
		src(t, "x", extraColumn.Columns, []apicontract.TypedValue{str("FR"), str("France"), str("FRA")}),
		src(t, "y", extraColumn.Columns, []apicontract.TypedValue{str("FR"), str("France"), str("FRX")}),
	}, Rules{Key: []string{"code"}, Baseline: "hq"})
	require.NoError(t, err)
	assert.Equal(t, StatusConsistent, result.Status)
	assert.Equal(t, []string{"hq"}, result.Columns[0].MissingIn)
}

func TestGroupsOnlyDisclosureOmitsValues(t *testing.T) {
	result, err := Reconcile([]Source{
		countries(t, "a", country("IE", "Ireland")),
		countries(t, "b", country("IE", "Ireland")),
		countries(t, "c", country("IE", "Eire")),
	}, Rules{Key: []string{"code"}, Disclosure: DiscloseGroupsOnly})
	require.NoError(t, err)
	for _, group := range result.Unexpected[0].Fields[0].Groups {
		assert.Nil(t, group.Value)
	}
}

func TestLimitTruncatesDetailButNotCounts(t *testing.T) {
	var left, right [][]apicontract.TypedValue
	for _, code := range []string{"A", "B", "C"} {
		left = append(left, country(code, "x"))
		right = append(right, country(code, "y"))
	}
	result, err := Reconcile([]Source{countries(t, "l", left...), countries(t, "r", right...)}, Rules{Key: []string{"code"}, Limit: 2})
	require.NoError(t, err)
	assert.Len(t, result.Unexpected, 2)
	assert.True(t, result.Truncated)
	assert.Equal(t, 3, result.Summary.WithUnexpected)
}

func TestSnapshotCaveat(t *testing.T) {
	b := countries(t, "b", standard()...)
	b.Snapshot = false
	result, err := Reconcile([]Source{countries(t, "a", standard()...), b}, Rules{Key: []string{"code"}})
	require.NoError(t, err)
	assert.Equal(t, []string{SnapshotCaveat}, result.Caveats)
}

func TestNullValuesAndSameValueAcrossTypes(t *testing.T) {
	cols := []apicontract.Column{{Name: "code", Type: "string"}, {Name: "note", Type: "string"}}
	a := src(t, "a", cols, []apicontract.TypedValue{str("X"), apicontract.NewNullValue()})
	b := src(t, "b", cols, []apicontract.TypedValue{str("X"), str("")})
	result, err := Reconcile([]Source{a, b}, Rules{Key: []string{"code"}, DefaultField: FieldNormalized})
	require.NoError(t, err)
	assert.Equal(t, 1, result.Summary.WithUnexpected, "null and empty string are different values")
}

func TestInvalidRules(t *testing.T) {
	two := func() []Source { return []Source{countries(t, "a"), countries(t, "b")} }
	cases := map[string]Rules{
		"no key":             {},
		"duplicate key":      {Key: []string{"code", "code"}},
		"empty key":          {Key: []string{""}},
		"baseline no name":   {Key: []string{"code"}, Mode: ModeBaseline},
		"consensus baseline": {Key: []string{"code"}, Mode: ModeConsensus, Baseline: "a"},
		"bad mode":           {Key: []string{"code"}, Mode: "vote"},
		"subset consensus":   {Key: []string{"code"}, Presence: PresenceSubset},
		"bad presence":       {Key: []string{"code"}, Presence: "some"},
		"bad default":        {Key: []string{"code"}, DefaultField: "fuzzy"},
		"bad field":          {Key: []string{"code"}, Fields: map[string]FieldPolicy{"name": "fuzzy"}},
		"policy on key":      {Key: []string{"code"}, Fields: map[string]FieldPolicy{"code": FieldIgnored}},
		"bad disclosure":     {Key: []string{"code"}, Disclosure: "hashes"},
		"bad limit":          {Key: []string{"code"}, Limit: MaximumLimit + 1},
		"unknown baseline":   {Key: []string{"code"}, Baseline: "zz"},
	}
	for name, rules := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Reconcile(two(), rules)
			assert.ErrorIs(t, err, ErrInvalidRules)
		})
	}
	_, err := Reconcile([]Source{countries(t, "a")}, Rules{Key: []string{"code"}})
	assert.ErrorIs(t, err, ErrInvalidRules)
	_, err = Reconcile([]Source{countries(t, "a"), countries(t, "a")}, Rules{Key: []string{"code"}})
	assert.ErrorIs(t, err, ErrInvalidRules)
}

func rowsOf(rows ...[]apicontract.TypedValue) func(func([]apicontract.TypedValue, error) bool) {
	return func(yield func([]apicontract.TypedValue, error) bool) {
		for _, row := range rows {
			if !yield(row, nil) {
				return
			}
		}
	}
}

func TestMalformedSources(t *testing.T) {
	good := func() Source { return countries(t, "good", country("FR", "France")) }
	bad := func(rows func(func([]apicontract.TypedValue, error) bool), count int) Source {
		return Source{Instance: Instance{ID: "bad"}, Columns: countryColumns, Rows: rows, Info: apicontract.CompareSideInfo{RowCount: count, Limitations: []apicontract.Limitation{}}}
	}
	streamErr := errors.New("boom")
	cases := map[string]Source{
		"nil rows":        {Instance: Instance{ID: "bad"}, Columns: countryColumns, Info: apicontract.CompareSideInfo{Limitations: []apicontract.Limitation{}}},
		"invalid columns": {Instance: Instance{ID: "bad"}, Columns: []apicontract.Column{{Name: "", Type: "string"}}, Rows: rowsOf(), Info: apicontract.CompareSideInfo{Limitations: []apicontract.Limitation{}}},
		"invalid info":    {Instance: Instance{ID: "bad"}, Columns: countryColumns, Rows: rowsOf(), Info: apicontract.CompareSideInfo{RowCount: -1}},
		"duplicate col":   {Instance: Instance{ID: "bad"}, Columns: []apicontract.Column{{Name: "code", Type: "string"}, {Name: "code", Type: "string"}}, Rows: rowsOf(), Info: apicontract.CompareSideInfo{Limitations: []apicontract.Limitation{}}},
		"stream error": bad(func(yield func([]apicontract.TypedValue, error) bool) {
			yield(nil, streamErr)
		}, 1),
		"short row":     bad(rowsOf([]apicontract.TypedValue{str("FR")}), 1),
		"invalid value": bad(rowsOf([]apicontract.TypedValue{str("FR"), {Type: "weird"}}), 1),
		"wrong type":    bad(rowsOf([]apicontract.TypedValue{str("FR"), num(1)}), 1),
		"null key":      bad(rowsOf([]apicontract.TypedValue{apicontract.NewNullValue(), str("x")}), 1),
		"row count":     bad(rowsOf(country("FR", "France")), 2),
		"duplicate key": bad(rowsOf(country("FR", "France"), country("FR", "France")), 2),
		"unsorted":      bad(rowsOf(country("IE", "Ireland"), country("FR", "France")), 2),
	}
	for name, source := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Reconcile([]Source{good(), source}, Rules{Key: []string{"code"}})
			require.Error(t, err)
			assert.NotErrorIs(t, err, ErrInvalidRules)
		})
	}
}

func TestOrderRows(t *testing.T) {
	rows, err := OrderRows(apicontract.Recordset{Columns: countryColumns, Rows: [][]apicontract.TypedValue{country("IE", "Ireland"), country("FR", "France")}}, []string{"code"})
	require.NoError(t, err)
	var codes []string
	for row := range rows {
		codes = append(codes, row[0].Str)
		break // early stop is honoured
	}
	assert.Equal(t, []string{"FR"}, codes)

	_, err = OrderRows(apicontract.Recordset{Columns: countryColumns}, []string{"nope"})
	assert.Error(t, err)
	_, err = OrderRows(apicontract.Recordset{Columns: countryColumns, Rows: [][]apicontract.TypedValue{{}}}, []string{"code"})
	assert.Error(t, err)
	_, err = OrderRows(apicontract.Recordset{Columns: countryColumns, Rows: [][]apicontract.TypedValue{{apicontract.NewNullValue(), str("x")}}}, []string{"code"})
	assert.Error(t, err)
}
