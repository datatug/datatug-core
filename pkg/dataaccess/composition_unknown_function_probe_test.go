package dataaccess

import (
	"github.com/dal-go/dalgo/dal"
	"testing"
)

// This probe keeps the required unknown-function refusal assertion. A failure
// blocks broad production admission; it must not be converted into an allow.
func TestCOMPUnknownAggregateNameRefusedBeforeDispatch(t *testing.T) {
	query := dal.From(dal.NewRootCollectionRef("docs", "")).NewQuery().SelectColumns(
		dal.Column{Expression: dal.NewAggregate("UNKNOWN_PROOF_FUNCTION", false, dal.Field("value"))},
	)
	compositionReadVector(t, query, false, compositionPolicy(t, ReadAll))
}
