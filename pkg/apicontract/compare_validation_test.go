package apicontract

import (
	"strings"
	"testing"
)

func TestCompareSideInfoValidation(t *testing.T) {
	if err := (CompareSideInfo{RowCount: -1}).Validate(); err == nil || !strings.Contains(err.Error(), "rowCount") {
		t.Fatalf("negative row count error = %v", err)
	}
	if err := (CompareSideInfo{Limitations: []Limitation{{Policy: ""}}}).Validate(); err == nil || !strings.Contains(err.Error(), "limitations") {
		t.Fatalf("invalid limitation error = %v", err)
	}
}

func TestCompareDiffRejectsInvalidSideContext(t *testing.T) {
	diff := CompareDiff{}
	if err := diff.Validate(CompareSideInfo{RowCount: -1}, CompareSideInfo{}); err == nil || !strings.Contains(err.Error(), "leftInfo") {
		t.Fatalf("invalid left context error = %v", err)
	}
	if err := diff.Validate(CompareSideInfo{}, CompareSideInfo{RowCount: -1}); err == nil || !strings.Contains(err.Error(), "rightInfo") {
		t.Fatalf("invalid right context error = %v", err)
	}
}
