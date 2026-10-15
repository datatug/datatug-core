package sessioncontext

import (
	"context"
	"errors"
	"time"
)

const Redactions = "sessionContextRedactions"

// SessionRedaction is a private authority marker, not an erasure claim or a grant.
// Creation/scrubbing belongs to the later accepted owner-deletion command.
type SessionRedaction struct {
	SessionID  string          `json:"sessionId"`
	Owner      string          `json:"owner"`
	Namespace  NativeNamespace `json:"namespace"`
	DeleteOpID string          `json:"deleteOpId"`
	Version    int64           `json:"version"`
	RedactedAt time.Time       `json:"redactedAt"`
}

func guardSessionRedaction(ctx context.Context, reader Reader, sessionID string) error {
	var marker SessionRedaction
	_, err := reader.Load(ctx, Redactions, sessionID, &marker)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	// Any marker denies content, including a corrupted marker. Status-only
	// authorized marker inspection is deliberately not introduced by this slice.
	return ErrReadOnly
}
