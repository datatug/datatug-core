package apicontract

// Limitation reports an applied restriction, not the number of rejected rows
// or their values. Only names visible in authorized metadata may appear in
// HiddenColumns or Policy; use a generic policy label and an empty
// HiddenColumns where names are protected. api-contract.md "Shared JSON
// types" / "Limitations report applied restrictions...".
type Limitation struct {
	Policy        string   `json:"policy"`
	RowsFiltered  bool     `json:"rowsFiltered"`
	HiddenColumns []string `json:"hiddenColumns"`
}

// Validate enforces Policy is required - a limitation with no named policy
// cannot be attributed to a rule, and the appendix requires attribution
// ("never silent, always attributable to a named rule").
func (l Limitation) Validate() error {
	return requireNonEmpty("policy", l.Policy)
}
