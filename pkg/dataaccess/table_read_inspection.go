package dataaccess

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/dal-go/dalgo/access"
)

// TableReadInspection is a read-only view of one exact-table read decision.
// It is never authority: it carries no lease, capability or data value, and a
// host must never use it to admit an operation.
type TableReadInspection struct {
	// Admitted is the result of the same Authority admission that
	// ReadTableResult runs before it opens any source, evaluated at the
	// resolution's ServerTime instead of the local clock.
	Admitted bool
	// Code is the Core denial code when Admitted is false.
	Code string
	// Until is the admitted lease deadline on the ServerTime clock. It is zero
	// when admission is refused.
	Until time.Time
	// Assessment is a reduced view of DALgo AssessPlan over the read rules
	// that the same Core compiler produces for this request at ServerTime,
	// intersected with the sealed exact mandatory policy when one is supplied.
	// Rule IDs, row restrictions and explanations are dropped. Policy names and
	// codes still distinguish a mandatory deny from no match, so this value is
	// private-snapshot-grade: never serialise it to a caller who may not inspect
	// the subject's access.
	Assessment InspectedAssessment
}

// InspectedAssessment is the reduced, rule-free DALgo assessment.
type InspectedAssessment struct {
	Outcome  access.AssessmentOutcome
	Policies []InspectedPolicy
}

// InspectedPolicy is one policy's decision: its name and fixed reason code.
type InspectedPolicy struct {
	Policy  string
	Allowed bool
	Code    access.ReasonCode
}

func reduceAssessment(assessment access.Assessment) InspectedAssessment {
	reduced := InspectedAssessment{Outcome: assessment.Outcome, Policies: make([]InspectedPolicy, len(assessment.Policies))}
	for i, p := range assessment.Policies {
		reduced.Policies[i] = InspectedPolicy{Policy: p.Decision.Policy, Allowed: p.Decision.Allowed, Code: p.Decision.Code}
	}
	return reduced
}

// InspectTableRead answers whether ReadTableResult would admit a preview of one
// exact object for an already resolved authority snapshot, and why. It runs the
// unchanged Authority admission path and the unchanged read-policy compiler with
// the clock fixed at resolution.ServerTime, then asks DALgo AssessPlan for the
// policy metadata. It opens no source, reads no row and retains nothing.
//
// The resolution must be what the trusted resolver would return for the
// subject and connection, including ProjectExactRead for posture exact-v1;
// mandatory must be the same sealed snapshot the host binds for execution.
func InspectTableRead(ctx context.Context, resolution Resolution, mandatory *ExactReadSnapshot, object ObjectRef) (TableReadInspection, error) {
	if ctx == nil {
		return TableReadInspection{}, fmt.Errorf("dataaccess: inspection context required")
	}
	_, request, err := TableRead{Object: object, MaxRows: 1, MaxBytes: 1}.query()
	if err != nil {
		return TableReadInspection{}, err
	}
	at := resolution.ServerTime
	authority, err := NewAuthority(resolution.Subject, resolution.Connection, func(context.Context, ResolveRequest) (Resolution, error) {
		return resolution, nil
	})
	if err != nil {
		return TableReadInspection{}, err
	}
	authority.now = func() time.Time { return at }
	var inspection TableReadInspection
	if bound, err := authority.authorizeRequest(withExactReadProfile(ctx), request); err != nil {
		inspection.Code = "READ_DENIED"
		var denial *Denial
		if errors.As(err, &denial) {
			inspection.Code = denial.Code
		}
	} else {
		inspection.Admitted = true
		inspection.Until = bound.deadline
	}
	assessment, err := assessTableRead(ctx, resolution, mandatory, *request, at)
	inspection.Assessment = reduceAssessment(assessment)
	return inspection, err
}

func assessTableRead(ctx context.Context, resolution Resolution, mandatory *ExactReadSnapshot, request access.Request, at time.Time) (access.Assessment, error) {
	unavailable := access.Assessment{Outcome: access.AssessmentIndeterminate, Policies: []access.PolicyAssessment{}, Restrictions: []access.AssessmentRestriction{}}
	canonicalResolution, err := canonicalExactResolution(resolution)
	if err != nil {
		return unavailable, nil
	}
	policy := newOperationReadPolicy(canonicalResolution, at, func() time.Time { return at })
	if resolution.Subject.Kind == Agent {
		// Report what the agent may actually read: never more than its user.
		if canonicalResolution.AgentUser == nil {
			return unavailable, nil
		}
		policy = policy.intersect(newOperationReadPolicy(*canonicalResolution.AgentUser, at, func() time.Time { return at }))
	}
	// planAt refuses only identities that TableRead.query cannot construct; a
	// refusal leaves no rule, which DALgo evaluates as default deny.
	rules, _, _ := policy.planAt(request, at)
	subject := resolution.Subject
	// Like ProjectExactRead, inspection replaces ambient context policies with
	// exactly the configured layers, keeping only cancellation and deadline.
	inspected := access.WithPrincipal(exactAssessmentContext{Context: ctx}, access.Principal{Subject: &access.PrincipalRef{Realm: subject.Realm, Kind: access.PrincipalKind(subject.Kind), ID: subject.ID}})
	if mandatory != nil {
		inspected, err = mandatory.BindContext(exactAssessmentContext{Context: ctx}, subject)
		if err != nil {
			return unavailable, nil
		}
	}
	return access.AssessContextPlan(access.WithPolicy(inspected, inspectedReadRules{policy: policy, rules: rules}), request)
}

// inspectedReadRules evaluates the captured compiled rules with the same
// decideRules function Authority uses after its deadline checks.
type inspectedReadRules struct {
	policy *operationReadPolicy
	rules  []access.Rule
}

func (p inspectedReadRules) Name() string { return p.policy.Name() }

func (p inspectedReadRules) Decide(ctx context.Context, request access.Request) access.Decision {
	return p.policy.decideRules(ctx, request, p.rules)
}

func (p inspectedReadRules) Authorize(ctx context.Context, request access.Request) error {
	if decision := p.Decide(ctx, request); !decision.Allowed {
		return &access.DeniedError{Decision: decision}
	}
	return nil
}

func (inspectedReadRules) InspectionPure() bool { return true }
