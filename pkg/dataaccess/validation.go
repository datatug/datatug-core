package dataaccess

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/dal-go/dalgo/access"
	"github.com/datatug/datatug-core/pkg/apicontract"
)

// Denial is safe to show across execution surfaces. It carries no query,
// credential, policy expression, resolved value, or untrusted upstream error.
type Denial struct {
	Connection string     `json:"connection"`
	Capability Permission `json:"capability"`
	Code       string     `json:"code"`
}

func (e *Denial) Error() string {
	return fmt.Sprintf("data access denied: connection=%q capability=%q code=%s", e.Connection, e.Capability, e.Code)
}

func (e *Denial) Unwrap() error { return access.ErrAccessDenied }

func unsupported(connection string) error {
	return &Denial{Connection: connection, Capability: DataRead, Code: "UNSUPPORTED_SCOPE"}
}

func canonical(value string) bool {
	return value != "" && len(value) <= 256 && utf8.ValidString(value) &&
		strings.TrimSpace(value) == value && !strings.ContainsFunc(value, unicode.IsControl)
}

func (s Subject) Validate() error {
	if !canonical(s.Realm) || !canonical(s.ID) || (s.Kind != User && s.Kind != Service && s.Kind != Agent) {
		return fmt.Errorf("dataaccess: invalid subject")
	}
	return nil
}

// ValidateMVP admits connection reads and exact-object read requests. Execution
// of object grants remains gated until generic scope composition is proven.
// All not-yet-supported fields and unknown required features fail closed.
func (s GrantScope) ValidateMVP(permissions []Permission) error {
	if !canonical(s.Connection) || len(permissions) != 1 || permissions[0] != DataRead ||
		s.Schemas != nil || s.Exclude != nil || s.Records != nil || s.Fields != nil || s.Request != nil || s.Requires == nil {
		return unsupported(s.Connection)
	}
	if len(s.Objects) == 0 {
		if s.Objects != nil || len(s.Requires) != 0 {
			return unsupported(s.Connection)
		}
		return nil
	}
	if len(s.Requires) != 1 || s.Requires[0] != ObjectsExact {
		return unsupported(s.Connection)
	}
	seen := make(map[ObjectRef]bool, len(s.Objects))
	for _, object := range s.Objects {
		if !canonical(object.Name) || (object.Namespace != "" && !canonical(object.Namespace)) ||
			strings.ContainsAny(object.Name+object.Namespace, "*?[]") || seen[object] {
			return unsupported(s.Connection)
		}
		seen[object] = true
	}
	return nil
}

func (r AccessRequest) ValidateMVP() error {
	if err := r.Scope.ValidateMVP(r.Permissions); err != nil {
		return err
	}
	if !canonical(r.Reason) || r.ExpiresAt.IsZero() || (r.PriorGrant != "" && !canonical(r.PriorGrant)) ||
		(r.CaseRef != nil && (!canonical(r.CaseRef.Kind) || !canonical(r.CaseRef.ID))) {
		return fmt.Errorf("dataaccess: invalid access request")
	}
	return nil
}

func (g Grant) ValidateMVP() error {
	if err := (AccessRequest{Permissions: g.Permissions, Scope: g.Scope, Reason: g.Reason,
		ExpiresAt: g.ExpiresAt, CaseRef: g.CaseRef, PriorGrant: g.PriorGrant}).ValidateMVP(); err != nil {
		return err
	}
	if err := g.Subject.Validate(); err != nil {
		return err
	}
	if err := g.RequestedBy.Validate(); err != nil {
		return err
	}
	if g.NotBefore != nil {
		return unsupported(g.Scope.Connection)
	}
	if !canonical(g.ID) || g.Revision <= 0 ||
		(g.State != Requested && g.State != Approved && g.State != Refused && g.State != Withdrawn && g.State != Revoked) ||
		(g.Decision != nil && (g.Decision.By.Validate() != nil || g.Decision.At.IsZero())) {
		return fmt.Errorf("dataaccess: invalid grant")
	}
	return nil
}

// DecodeAccessRequest and DecodeGrant are the MVP wire admission points. They
// reuse the core decoder's duplicate/case-folded key, unknown-field and trailing
// input checks. Unsupported field presence is refused even when its value is
// null or an empty collection. Plain json.Unmarshal is not wire admission.
func DecodeAccessRequest(data []byte) (AccessRequest, error) {
	var request AccessRequest
	if err := decodeMVP(data, &request); err != nil {
		return AccessRequest{}, err
	}
	if err := request.ValidateMVP(); err != nil {
		return AccessRequest{}, err
	}
	return request, nil
}

func DecodeGrant(data []byte) (Grant, error) {
	var grant Grant
	if err := decodeMVP(data, &grant); err != nil {
		return Grant{}, err
	}
	if err := grant.ValidateMVP(); err != nil {
		return Grant{}, err
	}
	return grant, nil
}

func decodeMVP(data []byte, value any) error {
	if err := apicontract.DecodeStrict(data, value, "schemas", "exclude", "records", "fields", "request", "notBefore"); err != nil {
		return &Denial{Capability: DataRead, Code: "INVALID_CONTRACT"}
	}
	return nil
}
