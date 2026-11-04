package dataaccess

import (
	"context"
	"fmt"
	"slices"
	"strings"
)

// ExactReadPolicy contains only the subject's effective standing table scope.
// It is an authenticated authority projection, never stored policy or native
// identity proof. Private policy generations and denied names are not wire data.
type ExactReadPolicy struct {
	Version    int           `json:"version"`
	Subject    Subject       `json:"subject"`
	Connection ConnectionRef `json:"connection"`
	Baseline   []ObjectRef   `json:"baseline"`
}

func (p *ExactReadPolicy) UnmarshalJSON(data []byte) error {
	if p == nil {
		return fmt.Errorf("dataaccess: exact policy target required")
	}
	*p = ExactReadPolicy{}
	type wirePolicy ExactReadPolicy
	var wire wirePolicy
	if err := tableDecode(data, &wire); err != nil {
		return err
	}
	*p = ExactReadPolicy(wire)
	return nil
}

func validExactObject(object ObjectRef) bool {
	return canonical(object.Namespace) && canonical(object.Name) && !strings.ContainsAny(object.Namespace+object.Name, "*?[]/{}")
}

func canonicalExactObjects(objects []ObjectRef) ([]ObjectRef, error) {
	if objects == nil || len(objects) > 1000 {
		return nil, fmt.Errorf("dataaccess: complete bounded exact object list required")
	}
	out := make([]ObjectRef, len(objects))
	copy(out, objects)
	slices.SortFunc(out, func(a, b ObjectRef) int {
		if n := strings.Compare(a.Namespace, b.Namespace); n != 0 {
			return n
		}
		return strings.Compare(a.Name, b.Name)
	})
	for i, object := range out {
		if !validExactObject(object) || (i > 0 && out[i-1] == object) {
			return nil, fmt.Errorf("dataaccess: unsupported exact object identity")
		}
	}
	return out, nil
}

func canonicalExactResolution(r Resolution) (Resolution, error) {
	p := r.ExactRead
	if p == nil {
		if r.Protection.ReadPosture == ReadExact {
			return Resolution{}, fmt.Errorf("dataaccess: exact policy missing")
		}
		return r, nil
	}
	if r.Protection.ReadPosture != ReadExact || p.Version != 1 || p.Subject != r.Subject || p.Subject.Validate() != nil ||
		p.Connection.LocalAuthorityID != "" || r.Connection.LocalAuthorityID != "" || !sameConnection(p.Connection, r.Connection) || (r.Subject.Kind == Agent && r.Protection.AgentRead != AgentInherit) {
		return Resolution{}, fmt.Errorf("dataaccess: selected exact policy binding unsupported")
	}
	copy := *p
	var err error
	copy.Baseline, err = canonicalExactObjects(p.Baseline)
	if err != nil {
		return Resolution{}, err
	}
	r.ExactRead = &copy
	grants := make([]Grant, len(r.Grants))
	for i, g := range r.Grants {
		if g.ValidateMVP() != nil || !exactGrantFeatures(g) || len(g.Scope.Objects) == 0 || g.Subject != r.Subject || !exactAgentGrantBound(r, g) || g.Scope.Connection != r.Connection.ID || g.Reason != "read authority" || g.CaseRef != nil || g.Decision != nil || g.PriorGrant != "" || g.Forwarded != nil || g.EffectiveAt != nil {
			return Resolution{}, fmt.Errorf("dataaccess: invalid exact grant projection")
		}
		g.Scope.Objects, err = canonicalExactObjects(g.Scope.Objects)
		if err != nil {
			return Resolution{}, err
		}
		g.Scope.Requires = []string{ObjectsExact}
		g.Permissions = []Permission{DataRead}
		grants[i] = g
	}
	r.Grants = grants
	if r.AgentUser != nil {
		user, err := canonicalExactResolution(*r.AgentUser)
		if err != nil {
			return Resolution{}, err
		}
		r.AgentUser = &user
	}
	return r, nil
}

// exactAgentGrantBound admits an agent's own projected grant only when bound to
// the agent user of the same resolution; a human grant never reaches an agent.
func exactAgentGrantBound(r Resolution, g Grant) bool {
	if r.Subject.Kind != Agent {
		return g.AgentUser == nil
	}
	return r.AgentUser != nil && g.AgentUser != nil && *g.AgentUser == r.AgentUser.Subject
}

type exactReadProfileKey struct{}

func withExactReadProfile(ctx context.Context) context.Context {
	return context.WithValue(ctx, exactReadProfileKey{}, true)
}

// Keep this admission explicit even if the generic MVP validator later grows.
func exactGrantFeatures(g Grant) bool {
	return g.NotBefore == nil && g.Scope.Schemas == nil && g.Scope.Exclude == nil && g.Scope.Records == nil && g.Scope.Fields == nil && g.Scope.Request == nil && len(g.Permissions) == 1 && g.Permissions[0] == DataRead && len(g.Scope.Requires) == 1 && g.Scope.Requires[0] == ObjectsExact
}

type exactBindingKey struct{}
type exactReadBinding struct {
	Subject    Subject
	Connection ConnectionRef
}

func checkExactBinding(ctx context.Context, subject Subject, connection ConnectionRef) error {
	if bound, ok := ctx.Value(exactBindingKey{}).(exactReadBinding); ok && (bound.Subject != subject || !sameConnection(bound.Connection, connection)) {
		return fmt.Errorf("dataaccess: exact binding unavailable")
	}
	return nil
}

// Source callbacks receive cancellation/deadline and policies, not admission.
type sourceReadContext struct{ context.Context }

func (c sourceReadContext) Value(key any) any {
	if _, ok := key.(exactReadProfileKey); ok {
		return nil
	}
	return c.Context.Value(key)
}
