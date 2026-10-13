package detective

import "encoding/json"

// cloneValue gives ownership of nested slices, maps and pointers to the
// receiving boundary. Definitions and evidence must not retain caller memory.
func cloneValue[T any](value T) (T, error) {
	var out T
	data, err := json.Marshal(value)
	if err != nil {
		return out, ErrInvalid
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return out, ErrInvalid
	}
	return out, nil
}

func cloneCatalog(c Catalog) (Catalog, error) {
	copy, err := cloneValue(c)
	if err != nil {
		return Catalog{}, err
	}
	copy.verified = true
	copy.signature = catalogSignature(copy)
	return copy, nil
}
