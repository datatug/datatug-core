package datatug

import (
	"fmt"
	"time"
)

// ValidateOptionalCreatedAt validates the optional `createdAt` creation date of
// a project definition (card, check, scenario). Absent is valid; a present value
// must be an RFC3339 timestamp, the format queries persist.
func ValidateOptionalCreatedAt(createdAt string) error {
	if createdAt == "" {
		return nil
	}
	if _, err := time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return fmt.Errorf("createdAt must be an RFC3339 timestamp: %w", err)
	}
	return nil
}

// NewCreatedAt formats a creation date the way queries persist it: RFC3339 UTC
// with nanoseconds.
func NewCreatedAt(at time.Time) string {
	return at.UTC().Format(time.RFC3339Nano)
}

// StampCardCreatedAt keeps a card's creation date across saves. A card that
// already carries one keeps it; otherwise it inherits the stored card's, and
// only a card that was never saved (existing == nil) is stamped with now.
// A stored card that has no date stays undated: its creation time is unknown.
func StampCardCreatedAt(card, existing *Card, now time.Time) {
	switch {
	case card.CreatedAt != "":
	case existing != nil:
		card.CreatedAt = existing.CreatedAt
	default:
		card.CreatedAt = NewCreatedAt(now)
	}
}
