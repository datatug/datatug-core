package dtprojcreator

import "context"

// IsCancelled is a helper to check for context cancellation
func IsCancelled(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}
