package detective

import (
	"context"
	"errors"
	"time"

	"github.com/datatug/datatug-core/pkg/reconcile"
)

// Reconciler executes a reconciliation profile for a reconcile step. The
// implementation owns profile loading, instance selection and per-instance
// policy-enforced execution; it returns the engine's structured result.
type Reconciler func(ctx context.Context, profileID string) (reconcile.Result, error)

// Option configures check evaluation.
type Option func(*engine)

// WithReconciler lets reconcile steps run. Without it they report an
// execution error, never a pass.
func WithReconciler(reconciler Reconciler) Option {
	return func(e *engine) { e.reconciler = reconciler }
}

// ReconcileUnavailableCode is the error code of a reconcile step run by an
// engine without a Reconciler.
const ReconcileUnavailableCode = "RECONCILE_UNAVAILABLE"

func (e *engine) runReconcileStep(ctx context.Context, step Step) StepResult {
	res := StepResult{StepID: step.ID, Kind: "reconcile", Profile: step.Profile, ExecutionStatus: "completed", Outcome: "unknown", EvidenceOutcome: "unknown"}
	if ctx.Err() != nil {
		res.ExecutionStatus, res.ErrorCode, res.Message = "error", "TIMEOUT", "check run deadline or cancellation reached"
		return res
	}
	if e.reconciler == nil {
		res.ExecutionStatus, res.ErrorCode, res.Message = "error", ReconcileUnavailableCode, "this runner cannot execute reconciliation steps"
		return res
	}
	started := time.Now()
	result, err := e.reconciler(ctx, step.Profile)
	res.DurationMs = time.Since(started).Milliseconds()
	if err != nil {
		res.ExecutionStatus, res.Message, res.ErrorCode = "error", "reconciliation could not execute", "INTERNAL"
		var failure *QueryFailure
		if errors.As(err, &failure) && validExecutionCode(failure.Code) {
			res.ErrorCode = failure.Code
		}
		return res
	}
	res.Reconciliation = &result
	switch result.Status {
	case reconcile.StatusConsistent:
		res.Outcome, res.EvidenceOutcome = "pass", "pass"
	case reconcile.StatusDiscrepancies:
		res.Outcome, res.EvidenceOutcome, res.Severity = "fail", "fail", "error"
		res.Message = "unexpected differences between instances"
	default:
		// Partial or inconclusive: something could not be compared, so the
		// step can never pass.
		res.Message = "some instances could not be fully compared"
	}
	return res
}
