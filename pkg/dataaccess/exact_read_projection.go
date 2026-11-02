package dataaccess

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/dal-go/dalgo/access"
	"github.com/dal-go/dalgo/dal"
)

// ExactReadSnapshot is sealed private host configuration. Ordinary JSON encoding
// exposes no private facts. Only DecodeExactReadSnapshot creates a valid value.
// Hosts must collect ALL standing/catalog/mandatory/native facts from one owned
// generation and exclude old writers before enabling exact mode. This type does
// not establish that deployment fence or discover policies in unrelated handles.
type ExactReadSnapshot struct {
	document exactReadDocument
	valid    bool
}

// MarshalJSON deliberately omits every private fact, including future fields.
func (*ExactReadSnapshot) MarshalJSON() ([]byte, error) { return []byte("{}"), nil }

func (ExactReadSnapshot) String() string   { return "[private exact read snapshot]" }
func (ExactReadSnapshot) GoString() string { return "[private exact read snapshot]" }
func (ExactReadSnapshot) LogValue() slog.Value {
	return slog.StringValue("[private exact read snapshot]")
}

type exactReadDocument struct {
	Version         int                 `json:"version"`
	Connection      ConnectionRef       `json:"connection"`
	Generation      int64               `json:"generation"`
	NativeNamespace string              `json:"nativeNamespace"`
	Objects         []ObjectRef         `json:"objects"`
	Standing        []exactReadStanding `json:"standing"`
	Requestable     []ObjectRef         `json:"requestable"`
	Denials         []exactReadDeny     `json:"denials"`
}
type exactReadStanding struct {
	Subject Subject     `json:"subject"`
	Objects []ObjectRef `json:"objects"`
}
type exactReadDeny struct {
	Subject *Subject   `json:"subject,omitempty"`
	Object  *ObjectRef `json:"object,omitempty"`
	Path    string     `json:"path,omitempty"`
}

// DecodeExactReadSnapshot admits only this bounded table-local deny profile.
// Unknown policy kinds, permissions, expressions or fields refuse; there is no
// custom-policy escape hatch. Names must match the trusted native inventory
// byte-for-byte, including namespace. Folded collisions conservatively refuse.
func DecodeExactReadSnapshot(data []byte) (*ExactReadSnapshot, error) {
	var doc exactReadDocument
	if len(data) > 1048576 || tableDecode(data, &doc) != nil || doc.Version != 1 || doc.Generation <= 0 || doc.Connection.LocalAuthorityID != "" ||
		!canonical(doc.Connection.Space) || !canonical(doc.Connection.ID) || !canonical(doc.Connection.Fingerprint.Engine) || !canonical(doc.Connection.Fingerprint.Database) ||
		(doc.Connection.Fingerprint.EngineIdentity == "" && doc.Connection.Fingerprint.Address == "") || doc.Standing == nil || doc.Denials == nil || len(doc.Standing) > 1000 || len(doc.Denials) > 1000 {
		return nil, fmt.Errorf("dataaccess: exact configuration unavailable")
	}
	var err error
	doc.Objects, err = canonicalExactObjects(doc.Objects)
	if err != nil {
		return nil, err
	}
	for i, object := range doc.Objects {
		for _, prior := range doc.Objects[:i] {
			if strings.EqualFold(object.Namespace, prior.Namespace) && strings.EqualFold(object.Name, prior.Name) {
				return nil, fmt.Errorf("dataaccess: ambiguous native identity")
			}
		}
	}
	snapshot := &ExactReadSnapshot{document: doc}
	if doc.NativeNamespace != "" && !canonical(doc.NativeNamespace) {
		return nil, fmt.Errorf("dataaccess: native namespace unavailable")
	}
	doc.Requestable, err = snapshot.objects(doc.Requestable)
	if err != nil {
		return nil, err
	}
	subjects := map[Subject]bool{}
	for i, entry := range doc.Standing {
		if entry.Subject.Validate() != nil || entry.Subject.Kind == Agent || subjects[entry.Subject] {
			return nil, fmt.Errorf("dataaccess: standing subject unavailable")
		}
		subjects[entry.Subject] = true
		doc.Standing[i].Objects, err = snapshot.objects(entry.Objects)
		if err != nil {
			return nil, err
		}
	}
	for _, rule := range doc.Denials {
		if rule.Subject != nil && rule.Subject.Validate() != nil {
			return nil, fmt.Errorf("dataaccess: policy principal unavailable")
		}
		if (rule.Object == nil) == (rule.Path == "") {
			return nil, fmt.Errorf("dataaccess: exact policy selector required")
		}
		if rule.Object != nil {
			if !snapshot.known(*rule.Object) {
				return nil, fmt.Errorf("dataaccess: unknown native object")
			}
		}
		if rule.Path != "" {
			for _, object := range doc.Objects {
				if object.Namespace != doc.NativeNamespace {
					return nil, fmt.Errorf("dataaccess: ambiguous collection namespace")
				}
			}
			if doc.NativeNamespace == "" || (!strings.HasPrefix(rule.Path, "/") || strings.Contains(rule.Path[1:], "/") || strings.ContainsAny(rule.Path, "*?[]{}")) {
				return nil, fmt.Errorf("dataaccess: unsupported collection path")
			}
			if rule.Path != "/" && !snapshot.known(ObjectRef{Namespace: doc.NativeNamespace, Name: rule.Path[1:]}) {
				return nil, fmt.Errorf("dataaccess: unknown native path")
			}
		}
	}
	snapshot.document = doc
	snapshot.valid = true
	return snapshot, nil
}
func (s *ExactReadSnapshot) known(o ObjectRef) bool {
	if !validExactObject(o) {
		return false
	}
	for _, native := range s.document.Objects {
		if native == o {
			return true
		}
	}
	return false
}
func (s *ExactReadSnapshot) objects(objects []ObjectRef) ([]ObjectRef, error) {
	objects, err := canonicalExactObjects(objects)
	if err != nil {
		return nil, err
	}
	for _, o := range objects {
		if !s.known(o) {
			return nil, fmt.Errorf("dataaccess: unverified native identity")
		}
	}
	return objects, nil
}

// BindContext installs the SAME trusted namespace and compiled mandatory rules
// for actual secured native execution. Hosts must use this snapshot's closed
// configuration in both projection and execution, not an untracked source.
func (s *ExactReadSnapshot) BindContext(ctx context.Context, subject Subject) (context.Context, error) {
	if s == nil || !s.valid || ctx == nil || subject.Validate() != nil {
		return nil, fmt.Errorf("dataaccess: exact binding unavailable")
	}
	rules := []access.Rule{access.Root(access.Allow(access.Read, "mandatory-baseline"))}
	denied := map[ObjectRef]bool{}
	rootDenied := false
	for _, rule := range s.document.Denials {
		if rule.Subject != nil && *rule.Subject != subject {
			continue
		}
		if rule.Object != nil {
			denied[*rule.Object] = true
		}
		if rule.Path == "/" {
			rootDenied = true
		}
	}
	for _, object := range s.document.Objects {
		if !rootDenied && !denied[object] {
			rules = append(rules, access.TableScope(access.TableName{Schema: object.Namespace, Name: object.Name}, access.Allow(access.Read)))
		}
	}
	for i, rule := range s.document.Denials {
		if rule.Subject != nil && *rule.Subject != subject {
			continue
		}
		deny := access.Deny(access.Read, fmt.Sprintf("mandatory-%d", i))
		if rule.Object != nil {
			rules = append(rules, access.TableScope(access.TableName{Schema: rule.Object.Namespace, Name: rule.Object.Name}, deny))
		} else if rule.Path == "/" {
			rules = append(rules, access.Root(deny))
		} else {
			rules = append(rules, access.Collection(rule.Path[1:], deny))
		}
	}
	policy, err := access.NewPolicyForRealm(subject.Realm, "datatug-exact-mandatory", rules...)
	if err != nil {
		return nil, fmt.Errorf("dataaccess: exact policy compilation unavailable")
	}
	ctx = access.WithPrincipal(ctx, access.Principal{Subject: &access.PrincipalRef{Realm: subject.Realm, Kind: access.PrincipalKind(subject.Kind), ID: subject.ID}})
	if s.document.NativeNamespace != "" {
		ctx, err = access.WithNativeTableNamespace(ctx, s.document.NativeNamespace)
		if err != nil {
			return nil, err
		}
	}
	return context.WithValue(access.WithPolicy(ctx, policy), exactBindingKey{}, subject), nil
}
func (s *ExactReadSnapshot) allowed(ctx context.Context, object ObjectRef) (bool, error) {
	if !s.known(object) {
		return false, nil
	}
	// A clean metadata-only context avoids inheriting a prior operation's Core
	// allowlist during fresh resolve. ALL mandatory facts must be in the sealed
	// snapshot; unrelated ambient/hidden policy configurations are unsupported.
	q := dal.From(dal.NewQualifiedRootCollectionRef(object.Namespace, object.Name, "")).NewQuery().SelectIntoRecordset()
	request := access.Request{Operation: access.Query, Resources: access.ResourcesForQuery(q), Query: q}
	assessment, err := access.AssessContextPlan(ctx, request)
	if canceled := ctx.Err(); canceled != nil {
		return false, canceled
	}
	return exactAssessmentAllowed(assessment, err)
}

// ProjectExactRead produces only positive read authority, never an audit view.
// Effective ExpiresAt caps must already be computed by the owning resolver.
// No generation, private deny name or private-policy fingerprint is returned.
// ctx supplies cancellation/deadline only. Hosts must qualify that the sealed
// snapshot contains every active mandatory constraint and BindContext governs
// the same native execution source. Ambient/hidden policy composition is not
// qualified by this API and must not be enabled merely because decoding passed.
func ProjectExactRead(ctx context.Context, r Resolution, s *ExactReadSnapshot) (Resolution, error) {
	if ctx == nil || s == nil || !s.valid || !sameConnection(r.Connection, s.document.Connection) || r.Subject.Validate() != nil || r.Subject.Kind == Agent ||
		r.Protection.ReadPosture != ReadExact || !r.Protection.ReadOnly || r.Protection.MandatoryFineGrained ||
		r.Protection.ID != r.Connection.ID || !sameFingerprint(r.Protection.Fingerprint, r.Connection.Fingerprint) ||
		r.ServerTime.IsZero() || !r.ValidUntil.After(r.ServerTime) || (r.Protection.AgentRead != AgentInherit && r.Protection.AgentRead != AgentNone) {
		return Resolution{}, fmt.Errorf("dataaccess: exact projection unavailable")
	}
	if err := ctx.Err(); err != nil {
		return Resolution{}, err
	}
	evaluated, err := s.BindContext(exactAssessmentContext{Context: ctx}, r.Subject)
	if err != nil {
		return Resolution{}, err
	}
	baseline := []ObjectRef{}
	for _, entry := range s.document.Standing {
		if entry.Subject == r.Subject {
			for _, object := range entry.Objects {
				allow, err := s.allowed(evaluated, object)
				if err != nil {
					return Resolution{}, err
				}
				if allow {
					baseline = append(baseline, object)
				}
			}
		}
	}
	r.ExactRead = &ExactReadPolicy{Version: 1, Subject: r.Subject, Connection: r.Connection, Baseline: baseline}
	grants := []Grant{}
	for _, g := range r.Grants {
		if (len(g.Scope.Objects) > 0 && !exactGrantFeatures(g)) || g.ValidateMVP() != nil || g.Subject != r.Subject || g.Scope.Connection != r.Connection.ID {
			return Resolution{}, fmt.Errorf("dataaccess: invalid exact grant")
		}
		if g.State != Approved || !g.ExpiresAt.After(r.ServerTime) || len(g.Scope.Objects) == 0 {
			continue
		}
		objects := []ObjectRef{}
		for _, object := range g.Scope.Objects {
			if !validExactObject(object) {
				return Resolution{}, fmt.Errorf("dataaccess: malformed exact grant object")
			}
			allow, err := s.allowed(evaluated, object)
			if err != nil {
				return Resolution{}, err
			}
			if allow {
				objects = append(objects, object)
			}
		}
		if len(objects) == 0 {
			continue
		}
		objects, _ = canonicalExactObjects(objects)
		g.Scope = GrantScope{Connection: r.Connection.ID, Objects: objects, Requires: []string{ObjectsExact}}
		g.Permissions = []Permission{DataRead}
		g.Reason = "read authority"
		g.CaseRef = nil
		g.Decision = nil
		g.PriorGrant = ""
		grants = append(grants, g)
	}
	r.Grants = grants
	return canonicalExactResolution(r)
}

// ProjectAgentExactRead keeps User authentication/membership admission in the
// host, then narrows its effective standing baseline under a fixed Agent. Human
// grants never enter either the candidate set or Agent projection.
func ProjectAgentExactRead(ctx context.Context, user Resolution, agent Subject, s *ExactReadSnapshot) (Resolution, error) {
	if agent.Kind != Agent || agent.Validate() != nil || user.Subject.Kind != User || agent.Realm != user.Subject.Realm || user.Protection.AgentRead != AgentInherit {
		return Resolution{}, fmt.Errorf("dataaccess: agent projection unavailable")
	}
	// Validate/bind the fixed Agent configuration before compiling the User view.
	// Candidate authority still comes only from User-effective standing scope.
	if s == nil || !s.valid || ctx == nil {
		return Resolution{}, fmt.Errorf("dataaccess: agent projection unavailable")
	}
	evaluated, err := s.BindContext(exactAssessmentContext{Context: ctx}, agent)
	if err != nil {
		return Resolution{}, err
	}
	user.Grants = []Grant{}
	projected, err := ProjectExactRead(ctx, user, s)
	if err != nil {
		return Resolution{}, err
	}
	baseline := []ObjectRef{}
	for _, o := range projected.ExactRead.Baseline {
		allow, err := s.allowed(evaluated, o)
		if err != nil {
			return Resolution{}, err
		}
		if allow {
			baseline = append(baseline, o)
		}
	}
	projected.Subject = agent
	projected.ExactRead = &ExactReadPolicy{Version: 1, Subject: agent, Connection: projected.Connection, Baseline: baseline}
	return projected, nil
}

// RequestableObjects is intentionally disclosed configuration, separate from
// effective read metadata. Hosts must not substitute their old catalog port.
func (s *ExactReadSnapshot) RequestableObjects() []ObjectRef {
	if s == nil || !s.valid {
		return nil
	}
	out := make([]ObjectRef, len(s.document.Requestable))
	copy(out, s.document.Requestable)
	return out
}

// Metadata inspection retains cancellation/deadline but replaces only layers
// configured in the sealed snapshot. Ambient/hidden policies are NOT collected;
// hosts must prove the complete same-source configuration before enabling it.
type exactAssessmentContext struct{ context.Context }

func (exactAssessmentContext) Value(any) any { return nil }

func exactAssessmentAllowed(assessment access.Assessment, err error) (bool, error) {
	if err != nil || !assessment.Complete || len(assessment.Restrictions) != 0 {
		return false, fmt.Errorf("dataaccess: exact assessment unavailable")
	}
	return assessment.Outcome == access.AssessmentAllow, nil
}
