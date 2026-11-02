package dataaccess

import (
	"context"
	"fmt"
	"time"

	"github.com/dal-go/dalgo/access"
)

// operationReadPolicy is an immutable grant snapshot. It compiles ordinary
// DALgo rules for each request's authoritative resources, never a policy subset
// retained from an earlier query. DALgo remains the decision evaluator.
type operationReadPolicy struct {
	connection  time.Time
	objects     map[ObjectRef]time.Time
	ceiling     time.Time
	evaluatedAt time.Time
	exact       bool
	now         func() time.Time
	subject     Subject
}

func newOperationReadPolicy(resolution Resolution, started time.Time, now func() time.Time) *operationReadPolicy {
	p := &operationReadPolicy{objects: make(map[ObjectRef]time.Time), ceiling: started.Add(min(resolution.ValidUntil.Sub(resolution.ServerTime), time.Minute)), now: now, evaluatedAt: now(), subject: resolution.Subject}
	if resolution.Protection.ReadPosture == ReadExact {
		p.exact = true
		for _, object := range resolution.ExactRead.Baseline {
			p.objects[object] = p.ceiling
		}
	}
	if resolution.Protection.ReadPosture == ReadAll {
		p.connection = p.ceiling
	}
	for _, grant := range resolution.Grants {
		if grant.State != Approved || !grant.ExpiresAt.After(resolution.ServerTime) {
			continue
		}
		deadline := started.Add(grant.ExpiresAt.Sub(resolution.ServerTime))
		if p.ceiling.Before(deadline) {
			deadline = p.ceiling
		}
		if len(grant.Scope.Objects) == 0 && !p.exact {
			if p.connection.Before(deadline) {
				p.connection = deadline
			}
		}
		for _, object := range grant.Scope.Objects {
			if p.objects[object].Before(deadline) {
				p.objects[object] = deadline
			}
		}
	}
	return p
}

func (*operationReadPolicy) Name() string { return "datatug-core-read-composition" }

// plan inspects Table metadata before Kind: qualified structured sources retain
// DALgo's opaque kind. A genuine opaque/unmapped source never gets a table rule.
func (p *operationReadPolicy) planAt(request access.Request, now time.Time) ([]access.Rule, time.Time, error) {
	deadline := p.ceiling
	rules := []access.Rule{}
	if now.Before(p.connection) {
		rules = append(rules, access.Root(access.Allow(access.Read, "connection")))
	}
	if len(request.Resources) == 0 {
		return nil, time.Time{}, fmt.Errorf("dataaccess: operation resources required")
	}
	seen := make(map[access.TableName]bool)
	for _, resource := range request.Resources {
		allowedUntil := p.connection
		if table, mapped := resource.Table(); mapped {
			if table.Database != "" || !canonical(table.Schema) || !canonical(table.Name) || !canonicalReadIdentity(resource, table) {
				return nil, time.Time{}, fmt.Errorf("dataaccess: unsupported table identity")
			}
			if objectDeadline := p.objects[ObjectRef{Namespace: table.Schema, Name: table.Name}]; allowedUntil.Before(objectDeadline) {
				allowedUntil = objectDeadline
			}
			if now.Before(allowedUntil) && !seen[table] {
				seen[table] = true
				allow := access.Allow(access.Read, fmt.Sprintf("table-%d", len(rules)))
				if resource.Kind() == access.PathResource {
					// DALgo's trusted native-namespace delegate preserves Table metadata on
					// its canonical collection resource; keep this same selected-object rule.
					rules = append(rules, access.Collection(table.Name, allow))
				} else {
					rules = append(rules, access.TableScope(table, allow))
				}
			}
		} else if p.exact || resource.Kind() != access.PathResource {
			return nil, time.Time{}, fmt.Errorf("dataaccess: unsupported operation resource")
		}
		if allowedUntil.Before(deadline) {
			deadline = allowedUntil
		}
	}
	return rules, deadline, nil
}

func (p *operationReadPolicy) Decide(ctx context.Context, request access.Request) access.Decision {
	rules, deadline, err := p.planAt(request, p.evaluatedAt)
	contextDeadline, bounded := ctx.Deadline()
	if err != nil || ctx.Err() != nil || !bounded || contextDeadline.After(deadline) || !p.evaluatedAt.Before(deadline) {
		return access.Decision{Operation: request.Operation, Policy: p.Name(), Effect: "deny", Code: access.CodeEnforcementUnsupported, Explanation: "read composition unavailable"}
	}
	return p.decideRules(ctx, request, rules)
}

func (p *operationReadPolicy) decideRules(ctx context.Context, request access.Request, rules []access.Rule) access.Decision {
	if principal, ok := access.PrincipalFrom(ctx); ok && principal.Subject != nil {
		if principal.Subject.Realm != p.subject.Realm || string(principal.Subject.Kind) != string(p.subject.Kind) || principal.Subject.ID != p.subject.ID {
			return access.Decision{Operation: request.Operation, Policy: p.Name(), Effect: "deny", Code: access.CodeEnforcementUnsupported, Explanation: "read subject mismatch"}
		}
		policy, err := access.NewPolicyForRealm(p.subject.Realm, p.Name(), rules...)
		if err != nil {
			return access.Decision{Operation: request.Operation, Policy: p.Name(), Effect: "deny", Code: access.CodeEnforcementUnsupported, Explanation: "read composition unavailable"}
		}
		return policy.Decide(ctx, request)
	}
	return access.MustPolicy(p.Name(), rules...).Decide(ctx, request)
}

func (p *operationReadPolicy) Authorize(ctx context.Context, request access.Request) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	now := p.now()
	rules, deadline, err := p.planAt(request, now)
	if err != nil || !now.Before(deadline) {
		return &access.DeniedError{Decision: access.Decision{Operation: request.Operation, Policy: p.Name(), Effect: "deny", Code: access.CodeEnforcementUnsupported, Explanation: "read composition unavailable"}}
	}
	decision := p.decideRules(ctx, request, rules)
	if !decision.Allowed {
		return &access.DeniedError{Decision: decision}
	}
	return nil
}

func (p *operationReadPolicy) deadlineFor(request *access.Request) (time.Time, error) {
	// No operation binding means connection-wide authority, including result
	// capture/import/release and the existing point/transaction profile. Object
	// grants must not silently substitute for that independent permission.
	if request == nil {
		if !p.now().Before(p.connection) {
			return time.Time{}, fmt.Errorf("dataaccess: connection read unavailable")
		}
		return p.connection, nil
	}
	if err := p.Authorize(context.Background(), *request); err != nil {
		return time.Time{}, err
	}
	_, deadline, _ := p.planAt(*request, p.now())
	return deadline, nil
}

func copyReadRequest(request *access.Request) *access.Request {
	if request == nil {
		return nil
	}
	// Frozen profiles use only operation/resources. Do not retain Query, column
	// slices or caller-provided execution metadata as future authority.
	return &access.Request{Operation: request.Operation, Resources: append([]access.Resource(nil), request.Resources...)}
}

// Decide reads only its captured values and frozen assessment time;
// it performs no clock/provider/callback I/O during DALgo inspection. Dynamic
// deadlines remain checked by Authorize/deadlineFor and Authority guards.
// Inspection of a retained policy is NOT fresh authority: metadata consumers
// must freshly resolve/project rather than use its captured assessment.
func (*operationReadPolicy) InspectionPure() bool { return true }

func canonicalReadIdentity(resource access.Resource, table access.TableName) bool {
	return resource.Kind() != access.PathResource || resource.String() == access.CollectionResourceFor(nil, table.Name).String()
}
