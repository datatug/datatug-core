// Package reconcile compares one dataset across N database instances in a
// single streaming pass and classifies every difference against an explicit
// consistency policy.
//
// Alignment reuses DALgo recordops.DiffFunc, a K-way merge over key-ordered
// streams that retains one record per input: memory is O(N) records plus the
// bounded detail lists, and the cost is one pass over every input rather than
// N×(N−1)/2 pairwise diffs. Every source is a merge candidate against an empty
// baseline, so each emitted ID carries each instance's full post-policy row
// and authority is applied here, by policy, not by merge position.
//
// The package is read-only by construction: it consumes rows and returns a
// verdict. It never writes, repairs or synchronizes data.
package reconcile

import (
	"errors"
	"fmt"
	"iter"
	"sort"
	"strings"

	"github.com/dal-go/dalgo/recordops"
	"github.com/dal-go/record"
	"github.com/datatug/datatug-core/pkg/apicontract"
)

const (
	// DefaultLimit bounds each detail list when Rules.Limit is zero.
	DefaultLimit = 100
	// MaximumLimit is the largest accepted Rules.Limit.
	MaximumLimit = 10000
)

// ErrInvalidRules reports a policy that cannot be evaluated.
var ErrInvalidRules = errors.New("reconcile: invalid rules")

// SnapshotCaveat is reported when instances were not read from one snapshot.
const SnapshotCaveat = "instances were read independently, not from one snapshot: writes that landed between reads can appear as differences"

type sourcePlan struct {
	index        int
	id           string
	columns      map[string]int // visible column name -> row index
	types        map[string]string
	hidden       map[string]bool
	rowsFiltered bool
	available    bool
}

type columnPlan struct {
	name      string
	policy    FieldPolicy
	compareIn map[int]bool // source index -> compared
}

type run struct {
	rules     Rules
	sources   []Source
	plans     []*sourcePlan
	active    []*sourcePlan // available sources, merge-candidate order
	baseline  int           // source index, -1 in consensus mode
	columns   []columnPlan
	result    *Result
	instances []*InstanceResult
	byID      map[string]int
}

// Reconcile compares sources under rules. It returns an error only for an
// invalid policy or a malformed source stream; unavailable sources, schema
// drift and policy limitations are reported in the Result.
func Reconcile(sources []Source, rules Rules) (Result, error) {
	rules, err := normalizeRules(rules)
	if err != nil {
		return Result{}, err
	}
	r := &run{rules: rules, sources: sources, baseline: -1}
	if err := r.planSources(); err != nil {
		return Result{}, err
	}
	r.planColumns()
	if len(r.active) >= 2 && (rules.Mode == ModeConsensus || r.plans[r.baseline].available) {
		if err := r.merge(); err != nil {
			return Result{}, err
		}
	}
	r.finish()
	return *r.result, nil
}

func normalizeRules(rules Rules) (Rules, error) {
	invalid := func(format string, args ...any) (Rules, error) {
		return Rules{}, fmt.Errorf("%w: "+format, append([]any{ErrInvalidRules}, args...)...)
	}
	if len(rules.Key) == 0 {
		return invalid("key is required")
	}
	seen := map[string]bool{}
	for _, name := range rules.Key {
		if name == "" || seen[name] {
			return invalid("key columns must be non-empty and unique")
		}
		seen[name] = true
	}
	if rules.Mode == "" {
		if rules.Baseline != "" {
			rules.Mode = ModeBaseline
		} else {
			rules.Mode = ModeConsensus
		}
	}
	switch rules.Mode {
	case ModeBaseline:
		if rules.Baseline == "" {
			return invalid("baseline mode requires a baseline instance")
		}
	case ModeConsensus:
		if rules.Baseline != "" {
			return invalid("consensus mode does not take a baseline")
		}
	default:
		return invalid("unknown mode %q", rules.Mode)
	}
	if rules.Presence == "" {
		rules.Presence = PresenceRequired
	}
	switch rules.Presence {
	case PresenceRequired, PresenceAny:
	case PresenceSubset, PresenceSuperset:
		if rules.Mode != ModeBaseline {
			return invalid("presence %q needs baseline mode", rules.Presence)
		}
	default:
		return invalid("unknown presence %q", rules.Presence)
	}
	if rules.DefaultField == "" {
		rules.DefaultField = FieldIdentical
	}
	if !validFieldPolicy(rules.DefaultField) {
		return invalid("unknown default field policy %q", rules.DefaultField)
	}
	for name, policy := range rules.Fields {
		if !validFieldPolicy(policy) {
			return invalid("unknown policy %q for field %q", policy, name)
		}
		if seen[name] {
			return invalid("key column %q cannot carry a field policy", name)
		}
	}
	if rules.Disclosure == "" {
		rules.Disclosure = DiscloseValues
	}
	if rules.Disclosure != DiscloseValues && rules.Disclosure != DiscloseGroupsOnly {
		return invalid("unknown disclosure %q", rules.Disclosure)
	}
	if rules.Limit == 0 {
		rules.Limit = DefaultLimit
	}
	if rules.Limit < 1 || rules.Limit > MaximumLimit {
		return invalid("limit must be between 1 and %d", MaximumLimit)
	}
	return rules, nil
}

func validFieldPolicy(policy FieldPolicy) bool {
	switch policy {
	case FieldIdentical, FieldNormalized, FieldInstanceSpecific, FieldIgnored:
		return true
	}
	return false
}

func (r *run) planSources() error {
	if len(r.sources) < 2 {
		return fmt.Errorf("%w: at least two sources are required", ErrInvalidRules)
	}
	r.result = &Result{
		Mode: r.rules.Mode, Baseline: r.rules.Baseline, Key: append([]string(nil), r.rules.Key...),
		Presence: r.rules.Presence, Disclosure: r.rules.Disclosure,
		Instances: []InstanceResult{}, Columns: []ColumnStatus{},
		Unexpected: []RecordDiscrepancy{}, Expected: []RecordDiscrepancy{}, Inconclusive: []RecordDiscrepancy{},
		Caveats: []string{},
	}
	r.byID = map[string]int{}
	for i, source := range r.sources {
		id := source.Instance.ID
		if _, duplicate := r.byID[id]; id == "" || duplicate {
			return fmt.Errorf("%w: instance ids must be non-empty and unique", ErrInvalidRules)
		}
		r.byID[id] = i
		if r.rules.Mode == ModeBaseline && id == r.rules.Baseline {
			r.baseline = i
		}
	}
	if r.rules.Mode == ModeBaseline && r.baseline < 0 {
		return fmt.Errorf("%w: baseline %q is not among the sources", ErrInvalidRules, r.rules.Baseline)
	}
	for i, source := range r.sources {
		id := source.Instance.ID
		plan := &sourcePlan{index: i, id: id, columns: map[string]int{}, types: map[string]string{}, hidden: map[string]bool{}}
		r.plans = append(r.plans, plan)
		instance := &InstanceResult{Instance: source.Instance, ReadAt: source.ReadAt, Authoritative: id == r.rules.Baseline}
		r.instances = append(r.instances, instance)
		if source.Unavailable != nil {
			unavailable := *source.Unavailable
			instance.Unavailable = &unavailable
			continue
		}
		if source.Rows == nil {
			return fmt.Errorf("reconcile: instance %q: rows stream is required unless the source is unavailable", id)
		}
		if err := (apicontract.Recordset{Columns: source.Columns}).Validate(); err != nil {
			return fmt.Errorf("reconcile: instance %q: %w", id, err)
		}
		if err := source.Info.Validate(); err != nil {
			return fmt.Errorf("reconcile: instance %q: %w", id, err)
		}
		for _, limitation := range source.Info.Limitations {
			plan.rowsFiltered = plan.rowsFiltered || limitation.RowsFiltered
			for _, column := range limitation.HiddenColumns {
				plan.hidden[column] = true
			}
		}
		for index, column := range source.Columns {
			if plan.hidden[column.Name] {
				continue
			}
			if _, duplicate := plan.columns[column.Name]; duplicate {
				return fmt.Errorf("reconcile: instance %q: duplicate column %q", id, column.Name)
			}
			plan.columns[column.Name] = index
			plan.types[column.Name] = column.Type
		}
		instance.RowsFiltered = plan.rowsFiltered
		instance.HiddenColumns = sortedKeys(plan.hidden)
		if reason := r.keyProblem(plan); reason != "" {
			instance.Unavailable = &Unavailable{Reason: UnavailableNotExamined, Message: reason}
			continue
		}
		plan.available = true
		r.active = append(r.active, plan)
	}
	return nil
}

// keyProblem returns why the instance cannot be aligned, or "".
func (r *run) keyProblem(plan *sourcePlan) string {
	for _, name := range r.rules.Key {
		if plan.hidden[name] {
			return fmt.Sprintf("key column %q is hidden by policy", name)
		}
		if _, ok := plan.columns[name]; !ok {
			return fmt.Sprintf("key column %q is missing", name)
		}
	}
	keyTypes := r.keyTypes()
	if keyTypes == nil || plan.index == r.baseline {
		return ""
	}
	for i, name := range r.rules.Key {
		if plan.types[name] != keyTypes[i] {
			return fmt.Sprintf("key column %q has type %q, other instances use %q", name, plan.types[name], keyTypes[i])
		}
	}
	return ""
}

// keyTypes returns the key column types of the reference instance (the
// baseline, else the first instance whose key is complete), or nil.
func (r *run) keyTypes() []string {
	reference := r.baseline
	if reference < 0 {
		if len(r.active) == 0 {
			return nil
		}
		reference = r.active[0].index
	}
	if r.sources[reference].Unavailable != nil {
		return nil
	}
	return columnTypes(r.sources[reference].Columns, r.rules.Key)
}

func (r *run) policyFor(column string) FieldPolicy {
	if policy, ok := r.rules.Fields[column]; ok {
		return policy
	}
	return r.rules.DefaultField
}

func (r *run) planColumns() {
	keySet := map[string]bool{}
	for _, name := range r.rules.Key {
		keySet[name] = true
	}
	names := map[string]bool{}
	for _, plan := range r.plans {
		for name := range plan.columns {
			names[name] = true
		}
		for name := range plan.hidden {
			names[name] = true
		}
	}
	for _, name := range sortedKeys(names) {
		if keySet[name] {
			continue
		}
		policy := r.policyFor(name)
		if policy == FieldIgnored {
			continue
		}
		status := ColumnStatus{Column: name, Policy: policy, Compared: true}
		types := map[string]string{}
		distinct := map[string]bool{}
		for _, plan := range r.active {
			switch {
			case plan.hidden[name]:
				status.HiddenIn = append(status.HiddenIn, plan.id)
			default:
				t, ok := plan.types[name]
				if !ok {
					status.MissingIn = append(status.MissingIn, plan.id)
					continue
				}
				types[plan.id] = t
				distinct[t] = true
			}
		}
		if len(distinct) > 1 {
			status.Types = types
			status.Compared = false
		}
		if len(status.MissingIn) > 0 || len(status.HiddenIn) > 0 || !status.Compared {
			r.result.Columns = append(r.result.Columns, status)
		}
		if !status.Compared {
			continue
		}
		column := columnPlan{name: name, policy: policy, compareIn: map[int]bool{}}
		for _, plan := range r.active {
			if _, ok := types[plan.id]; ok {
				column.compareIn[plan.index] = true
			}
		}
		r.columns = append(r.columns, column)
	}
}

func (r *run) stream(plan *sourcePlan) recordops.RecordSeq[string] {
	source := r.sources[plan.index]
	return func(yield func(record.WithID[string], error) bool) {
		var zero record.WithID[string]
		count := 0
		fail := func(format string, args ...any) {
			yield(zero, fmt.Errorf("reconcile: instance %q: "+format, append([]any{plan.id}, args...)...))
		}
		for row, err := range source.Rows {
			if err != nil {
				fail("%w", err)
				return
			}
			if len(row) != len(source.Columns) {
				fail("row %d has %d values, want %d", count, len(row), len(source.Columns))
				return
			}
			for index, value := range row {
				if err := value.Validate(); err != nil {
					fail("row %d column %d: %w", count, index, err)
					return
				}
				if value.Type != apicontract.ValueTypeNull && string(value.Type) != source.Columns[index].Type {
					fail("row %d column %d does not match declared column type", count, index)
					return
				}
			}
			key := make([]apicontract.TypedValue, len(r.rules.Key))
			data := make(map[string]any, len(r.rules.Key)+len(r.columns))
			for i, name := range r.rules.Key {
				key[i] = row[plan.columns[name]]
				if key[i].Type == apicontract.ValueTypeNull {
					fail("row %d has null key column %q", count, name)
					return
				}
				data[name] = key[i]
			}
			for _, column := range r.columns {
				if column.compareIn[plan.index] {
					data[column.name] = row[plan.columns[column.name]]
				}
			}
			id, _ := apicontract.TypedKeySortKey(key) // key values validated above
			count++
			if !yield(record.WithID[string]{ID: id, Record: record.NewRecordWithoutKey(data)}, nil) {
				return
			}
		}
		if count != source.Info.RowCount {
			fail("stream yielded %d rows, side info declares %d", count, source.Info.RowCount)
			return
		}
		r.instances[plan.index].RowCount = count
	}
}

func emptyStream(func(record.WithID[string], error) bool) {}

func (r *run) merge() error {
	candidates := make([]recordops.RecordSeq[string], len(r.active))
	for i, plan := range r.active {
		candidates[i] = r.stream(plan)
	}
	diffs := recordops.DiffFunc[string](emptyStream, candidates,
		func(a, b string) bool { return a < b }, recordops.WithIncludeMatched())
	for diff, err := range diffs {
		if err != nil {
			switch {
			case errors.Is(err, recordops.ErrDuplicateID):
				return fmt.Errorf("reconcile: duplicate key in a source stream: %w", err)
			case errors.Is(err, recordops.ErrUnsortedInput):
				return fmt.Errorf("reconcile: a source stream is not strictly ordered by key: %w", err)
			default:
				return err
			}
		}
		r.evaluate(diff)
	}
	return nil
}

// rows maps source index -> field values for one aligned key.
type alignedRows map[int]map[string]apicontract.TypedValue

func (r *run) evaluate(diff recordops.IDDiff[string]) {
	rows := alignedRows{}
	for i, state := range diff.Candidates {
		if state.Status != recordops.Extra {
			continue
		}
		fields := make(map[string]apicontract.TypedValue, len(state.Fields))
		for _, field := range state.Fields {
			fields[field.Name] = field.Value.(apicontract.TypedValue)
		}
		rows[r.active[i].index] = fields
	}
	var first map[string]apicontract.TypedValue
	for _, plan := range r.active {
		if fields, ok := rows[plan.index]; ok {
			first = fields
			break
		}
	}
	record := RecordDiscrepancy{Key: make([]apicontract.TypedValue, len(r.rules.Key))}
	for i, name := range r.rules.Key {
		record.Key[i] = first[name]
	}
	summary := &r.result.Summary
	summary.Records++

	unexpected, expected, inconclusive := r.evaluatePresence(&record, rows)
	if r.rules.Mode == ModeConsensus || rows[r.baseline] != nil {
		u, e := r.evaluateFields(&record, rows)
		unexpected = unexpected || u
		expected = expected || e
	}

	switch {
	case unexpected:
		summary.WithUnexpected++
		r.keep(&r.result.Unexpected, record)
	case inconclusive:
		summary.WithInconclusive++
		r.keep(&r.result.Inconclusive, record)
	case expected:
		summary.WithExpectedOnly++
		r.keep(&r.result.Expected, record)
	default:
		summary.Consistent++
	}
}

func (r *run) keep(list *[]RecordDiscrepancy, record RecordDiscrepancy) {
	if len(*list) < r.rules.Limit {
		*list = append(*list, record)
		return
	}
	r.result.Truncated = true
}

// evaluatePresence fills Present/Missing/Extra/Inconclusive and returns which
// classes of difference the record carries.
func (r *run) evaluatePresence(record *RecordDiscrepancy, rows alignedRows) (unexpected, expected, inconclusive bool) {
	var absent []*sourcePlan
	for _, plan := range r.active {
		if _, ok := rows[plan.index]; ok {
			record.Present = append(record.Present, plan.id)
		} else {
			absent = append(absent, plan)
		}
	}
	summary := &r.result.Summary
	if r.rules.Mode == ModeBaseline && rows[r.baseline] == nil {
		// The authority lacks the record: holders are extra, unless the
		// authority's own rows were filtered by policy.
		if r.plans[r.baseline].rowsFiltered {
			for _, id := range record.Present {
				record.Inconclusive = append(record.Inconclusive, id)
				r.instance(id).Inconclusive++
			}
			record.Present = nil
			return false, false, true
		}
		record.Extra = record.Present
		record.ExtraExpected = r.rules.Presence == PresenceSuperset || r.rules.Presence == PresenceAny
		for _, id := range record.Extra {
			if record.ExtraExpected {
				r.instance(id).Expected++
			} else {
				r.instance(id).Extra++
			}
		}
		if record.ExtraExpected {
			summary.ExpectedDifferences++
			return false, true, false
		}
		summary.ExtraRecords++
		return true, false, false
	}
	if len(absent) == 0 {
		return false, false, false
	}
	record.MissingExpected = r.rules.Presence == PresenceAny || r.rules.Presence == PresenceSubset
	for _, plan := range absent {
		if plan.rowsFiltered {
			record.Inconclusive = append(record.Inconclusive, plan.id)
			r.instances[plan.index].Inconclusive++
			inconclusive = true
			continue
		}
		record.Missing = append(record.Missing, plan.id)
		if record.MissingExpected {
			r.instances[plan.index].Expected++
		} else {
			r.instances[plan.index].Missing++
		}
	}
	if len(record.Missing) == 0 {
		return false, false, inconclusive
	}
	if record.MissingExpected {
		summary.ExpectedDifferences++
		return false, true, inconclusive
	}
	summary.MissingRecords++
	return true, false, inconclusive
}

func (r *run) instance(id string) *InstanceResult {
	return r.instances[r.byID[id]]
}

func (r *run) evaluateFields(record *RecordDiscrepancy, rows alignedRows) (unexpected, expected bool) {
	deviatingUnexpected := map[int]bool{}
	deviatingExpected := map[int]bool{}
	for _, column := range r.columns {
		difference, deviating, ok := r.compareField(column, rows)
		if !ok {
			continue
		}
		record.Fields = append(record.Fields, difference)
		target := deviatingUnexpected
		if difference.Expected {
			target = deviatingExpected
			expected = true
		} else {
			unexpected = true
		}
		for _, index := range deviating {
			target[index] = true
		}
	}
	if unexpected {
		r.result.Summary.ConflictingRecords++
	} else if expected {
		r.result.Summary.ExpectedDifferences++
	}
	for index := range deviatingUnexpected {
		r.instances[index].Conflicts++
	}
	for index := range deviatingExpected {
		if !deviatingUnexpected[index] {
			r.instances[index].Expected++
		}
	}
	return unexpected, expected
}

type group struct {
	identity string
	value    apicontract.TypedValue
	members  []int
}

// compareField groups the instances holding a column by normalized value.
// It returns ok=false when all compared instances agree.
func (r *run) compareField(column columnPlan, rows alignedRows) (FieldDifference, []int, bool) {
	var groups []*group
	byIdentity := map[string]*group{}
	for _, plan := range r.active {
		fields, present := rows[plan.index]
		if !present || !column.compareIn[plan.index] {
			continue
		}
		value := fields[column.name]
		identity := valueIdentity(value, column.policy)
		g := byIdentity[identity]
		if g == nil {
			g = &group{identity: identity, value: value}
			byIdentity[identity] = g
			groups = append(groups, g)
		}
		g.members = append(g.members, plan.index)
	}
	if len(groups) < 2 {
		return FieldDifference{}, nil, false
	}
	if r.rules.Mode == ModeBaseline && groupHolding(groups, r.baseline) < 0 {
		// The authority does not hold this column: drift is reported in
		// Result.Columns; values cannot be judged against it.
		return FieldDifference{}, nil, false
	}
	sort.SliceStable(groups, func(i, j int) bool {
		if len(groups[i].members) != len(groups[j].members) {
			return len(groups[i].members) > len(groups[j].members)
		}
		return groups[i].identity < groups[j].identity
	})
	difference := FieldDifference{Column: column.name, Policy: column.policy, Expected: column.policy == FieldInstanceSpecific}
	reference := -1 // index into groups of the group others deviate from
	if r.rules.Mode == ModeBaseline {
		reference = groupHolding(groups, r.baseline)
	} else if len(groups[0].members) > len(groups[1].members) {
		reference = 0
	} else {
		difference.NoConsensus = true
	}
	var deviating []int
	for gi, g := range groups {
		vg := ValueGroup{Authoritative: r.rules.Mode == ModeBaseline && gi == reference}
		if r.rules.Disclosure == DiscloseValues {
			value := g.value
			vg.Value = &value
		}
		for _, member := range g.members {
			vg.Instances = append(vg.Instances, r.plans[member].id)
		}
		difference.Groups = append(difference.Groups, vg)
		if reference >= 0 && gi != reference {
			deviating = append(deviating, g.members...)
		}
	}
	if difference.NoConsensus {
		for _, g := range groups {
			deviating = append(deviating, g.members...)
		}
	}
	sort.Ints(deviating)
	for _, index := range deviating {
		difference.Deviating = append(difference.Deviating, r.plans[index].id)
	}
	if difference.NoConsensus {
		difference.Deviating = nil
	}
	return difference, deviating, true
}

// valueIdentity is the canonical comparison identity of a value under a
// field policy. It is internal: identities are never emitted, because a hash
// or encoding of a low-entropy value discloses the value.
func valueIdentity(value apicontract.TypedValue, policy FieldPolicy) string {
	if policy == FieldNormalized && value.Type == apicontract.ValueTypeString {
		value.Str = strings.ToLower(strings.Join(strings.Fields(value.Str), " "))
	}
	identity, _ := apicontract.TypedValueSortKey(value) // validated on read
	return identity
}

func (r *run) finish() {
	result := r.result
	result.Summary.Instances = len(r.sources)
	result.Summary.ComparedInstances = len(r.active)
	snapshot := true
	compared := len(r.active) >= 2 && (r.rules.Mode == ModeConsensus || r.plans[r.baseline].available)
	anyPartial, anyInconclusive := false, !compared
	for i, instance := range r.instances {
		plan := r.plans[i]
		snapshot = snapshot && r.sources[i].Snapshot
		switch {
		case !plan.available || !compared:
			instance.Status = StatusInconclusive
			anyInconclusive = true
		case instance.Missing+instance.Extra+instance.Conflicts > 0:
			instance.Status = StatusDiscrepancies
		case plan.rowsFiltered || len(plan.hidden) > 0 || instance.Inconclusive > 0:
			instance.Status = StatusPartial
			anyPartial = true
		default:
			instance.Status = StatusConsistent
		}
		result.Instances = append(result.Instances, *instance)
	}
	switch {
	case result.Summary.WithUnexpected > 0:
		result.Status = StatusDiscrepancies
	case anyInconclusive:
		result.Status = StatusInconclusive
	case anyPartial || result.Summary.WithInconclusive > 0:
		result.Status = StatusPartial
	default:
		result.Status = StatusConsistent
	}
	if !compared {
		if r.rules.Mode == ModeBaseline && !r.plans[r.baseline].available {
			result.Caveats = append(result.Caveats, fmt.Sprintf("baseline %q could not be read: nothing was compared", r.rules.Baseline))
		} else {
			result.Caveats = append(result.Caveats, "fewer than two instances could be read: nothing was compared")
		}
	}
	if !snapshot {
		result.Caveats = append(result.Caveats, SnapshotCaveat)
	}
	for _, column := range result.Columns {
		if !column.Compared {
			result.Caveats = append(result.Caveats, fmt.Sprintf("column %q has conflicting types across instances and was not compared", column.Column))
		}
	}
}

func groupHolding(groups []*group, index int) int {
	for gi, g := range groups {
		for _, member := range g.members {
			if member == index {
				return gi
			}
		}
	}
	return -1
}

// columnTypes returns the declared types of names, or nil if one is missing.
func columnTypes(columns []apicontract.Column, names []string) []string {
	types := make([]string, len(names))
	for i, name := range names {
		for _, column := range columns {
			if column.Name == name {
				types[i] = column.Type
			}
		}
		if types[i] == "" {
			return nil
		}
	}
	return types
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// OrderRows orders a materialized recordset by key for use as Source.Rows.
// It holds the whole recordset in memory; prefer an ordered query for large
// datasets.
func OrderRows(recordset apicontract.Recordset, key []string) (iter.Seq2[[]apicontract.TypedValue, error], error) {
	indexes := make([]int, len(key))
	for i, name := range key {
		indexes[i] = -1
		for c, column := range recordset.Columns {
			if column.Name == name {
				indexes[i] = c
			}
		}
		if indexes[i] < 0 {
			return nil, fmt.Errorf("reconcile: key column %q is missing", name)
		}
	}
	type keyed struct {
		sortKey string
		row     []apicontract.TypedValue
	}
	rows := make([]keyed, len(recordset.Rows))
	for r, row := range recordset.Rows {
		values := make([]apicontract.TypedValue, len(indexes))
		for i, index := range indexes {
			if index >= len(row) {
				return nil, fmt.Errorf("reconcile: row %d is shorter than the column list", r)
			}
			values[i] = row[index]
		}
		sortKey, err := apicontract.TypedKeySortKey(values)
		if err != nil {
			return nil, fmt.Errorf("reconcile: row %d key: %w", r, err)
		}
		rows[r] = keyed{sortKey: sortKey, row: row}
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].sortKey < rows[j].sortKey })
	return func(yield func([]apicontract.TypedValue, error) bool) {
		for _, item := range rows {
			if !yield(item.row, nil) {
				return
			}
		}
	}, nil
}
