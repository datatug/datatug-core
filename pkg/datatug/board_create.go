package datatug

import "context"

// GuardedBoardCreator is the optional owning create-only Board capability.
// Implementations validate the complete definition and refuse any existing
// target, including a target won by another process after an earlier read.
// authorize is called after provider preparation, immediately before atomic
// no-replace publication; serving callers must supply their current authority
// check. A nil callback is reserved for trusted in-process callers.
// An error after publication may leave the definition committed: this is not
// an idempotent operation or evidence that cancellation undid a durable write.
type GuardedBoardCreator interface {
	CreateBoardGuarded(ctx context.Context, board *Board, authorize func() error) error
}
