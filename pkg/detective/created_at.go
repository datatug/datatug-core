package detective

import "time"

// validOptionalCreatedAt reports whether an optional creation date is absent or
// an RFC3339 timestamp, the format queries persist.
func validOptionalCreatedAt(createdAt string) bool {
	if createdAt == "" {
		return true
	}
	_, err := time.Parse(time.RFC3339Nano, createdAt)
	return err == nil
}
