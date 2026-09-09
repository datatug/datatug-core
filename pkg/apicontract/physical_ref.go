package apicontract

// PhysicalRef names a physical column a semantic mapping points at.
// "PhysicalRef = {source: string; collection: string; column: string}" -
// api-contract.md "Shared JSON types".
type PhysicalRef struct {
	Source     string `json:"source"`
	Collection string `json:"collection"`
	Column     string `json:"column"`
}

// Validate enforces all three fields are required per the appendix's type.
func (r PhysicalRef) Validate() error {
	if err := requireNonEmpty("source", r.Source); err != nil {
		return err
	}
	if err := requireNonEmpty("collection", r.Collection); err != nil {
		return err
	}
	if err := requireNonEmpty("column", r.Column); err != nil {
		return err
	}
	return nil
}
