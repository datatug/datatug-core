package reconcile

import (
	"fmt"
	"testing"

	"github.com/datatug/datatug-core/pkg/apicontract"
	"github.com/datatug/datatug-core/pkg/recordsetcompare"
)

// benchRows builds n rows ordered by key, with every 100th row's name
// differing per instance so the engine records some discrepancies.
func benchRows(instance, n int) func(func([]apicontract.TypedValue, error) bool) {
	return func(yield func([]apicontract.TypedValue, error) bool) {
		for i := 0; i < n; i++ {
			name := "name"
			if i%100 == 0 {
				name = fmt.Sprintf("name-%d", instance)
			}
			row := []apicontract.TypedValue{apicontract.NewStringValue(fmt.Sprintf("k%08d", i)), apicontract.NewStringValue(name), apicontract.NewNumberValue(float64(i))}
			if !yield(row, nil) {
				return
			}
		}
	}
}

var benchColumns = []apicontract.Column{{Name: "code", Type: "string"}, {Name: "name", Type: "string"}, {Name: "qty", Type: "number"}}

func benchSources(n, rows int) []Source {
	sources := make([]Source, n)
	for i := range sources {
		sources[i] = Source{Instance: Instance{ID: fmt.Sprintf("i%02d", i)}, Columns: benchColumns, Rows: benchRows(i, rows),
			Info: apicontract.CompareSideInfo{RowCount: rows, Limitations: []apicontract.Limitation{}}}
	}
	return sources
}

// BenchmarkTwoWayCompare is the existing two-way diff on the same input, as
// the baseline the N-way engine must not regress against.
func BenchmarkTwoWayCompare(b *testing.B) {
	const rows = 10000
	receipt := apicontract.CompareSideReceipt{Execution: apicontract.ExecutionRef{StoreID: "s", ProjectID: "p", ExecutionID: "e"}, ExecutedAt: "2026-11-07T00:00:00Z", RowCount: rows, Limitations: []apicontract.Limitation{}}
	b.ReportAllocs()
	for b.Loop() {
		left := recordsetcompare.OrderedRecordset{Columns: benchColumns, Rows: benchRows(0, rows)}
		right := recordsetcompare.OrderedRecordset{Columns: benchColumns, Rows: benchRows(1, rows)}
		if _, err := recordsetcompare.CompareOrdered(left, right, receipt, receipt, recordsetcompare.Options{Key: []string{"code"}}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkReconcile(b *testing.B) {
	const rows = 10000
	for _, n := range []int{2, 3, 5, 20} {
		b.Run(fmt.Sprintf("instances=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := Reconcile(benchSources(n, rows), Rules{Key: []string{"code"}}); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(n*rows), "rows/op")
		})
	}
}
