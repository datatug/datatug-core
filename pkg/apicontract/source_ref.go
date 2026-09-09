package apicontract

// SourceRef names a stable project-local source, resolved through the
// project registry for the requested environment - never a filesystem path,
// arbitrary HTTP URL or credential. "SourceRef = {source: string, collection:
// string}" - api-contract.md "Scope and identity".
type SourceRef struct {
	Source     string `json:"source"`
	Collection string `json:"collection"`
}

// Validate enforces both fields are required per the appendix's type.
func (r SourceRef) Validate() error {
	if err := requireNonEmpty("source", r.Source); err != nil {
		return err
	}
	if err := requireNonEmpty("collection", r.Collection); err != nil {
		return err
	}
	return nil
}
