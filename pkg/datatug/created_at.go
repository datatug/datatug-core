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

// StampCardCreatedAt makes a card's creation date immutable across saves. A
// stored card's date always wins, so an overwrite or a backdated save cannot
// change it. Only when the stored card has no date (or there is none) does the
// caller's date apply, and when that is empty too the card is stamped with now.
// It mutates card.
func StampCardCreatedAt(card, existing *Card, now time.Time) {
	switch {
	case existing != nil && existing.CreatedAt != "":
		card.CreatedAt = existing.CreatedAt
	case card.CreatedAt == "":
		card.CreatedAt = NewCreatedAt(now)
	}
}
