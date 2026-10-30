package dataaccess

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/dal-go/dalgo/access"
	"github.com/stretchr/testify/require"
)

func TestMVPWireRejectsUnknownIdentityAndFutureScope(t *testing.T) {
	valid := AccessRequest{Permissions: []Permission{DataRead}, Scope: GrantScope{Connection: "conn", Requires: []string{}},
		Reason: "Investigate incident", ExpiresAt: time.Now().Add(time.Hour), CaseRef: &CaseRef{Kind: "incident", ID: "case-1"}, PriorGrant: "old"}
	encoded, err := json.Marshal(valid)
	require.NoError(t, err)
	decoded, err := DecodeAccessRequest(encoded)
	require.NoError(t, err)
	require.Equal(t, valid.Reason, decoded.Reason)
	for _, addition := range []string{
		`"subject":{"realm":"sneat","kind":"user","id":"owner"}`,
		`"requestedBy":{}`, `"approved":true`, `"unrestricted":true`, `"expiresAt":"2099-01-01T00:00:00Z"`,
		`"EXPIRESAT":"2099-01-01T00:00:00Z"`, `"scope":{"connection":"other","requires":[]}`,
	} {
		t.Run(addition, func(t *testing.T) {
			attack := append(append([]byte(nil), encoded[:len(encoded)-1]...), []byte(","+addition+"}")...)
			_, err := DecodeAccessRequest(attack)
			require.Error(t, err)
		})
	}
	for _, suffix := range []string{` {}`, ` []`, ` true`, ` garbage`} {
		_, err := DecodeAccessRequest(append(append([]byte(nil), encoded...), suffix...))
		require.Error(t, err)
	}
	for _, key := range []string{"schemas", "SCHEMAS", "exclude", "records", "fields", "request", "notBefore", "NOTBEFORE"} {
		for _, value := range []string{"null", "[]", "{}"} {
			wire := strings.Replace(string(encoded), `"connection":"conn"`, `"connection":"conn","`+key+`":`+value, 1)
			_, err := DecodeAccessRequest([]byte(wire))
			require.Error(t, err, wire)
		}
	}
	for _, wire := range []string{`null`, `{}`, `[]`, `{"permissions":["data.write"],"scope":{"connection":"conn","requires":[]}}`} {
		_, err := DecodeAccessRequest([]byte(wire))
		require.Error(t, err)
	}
}

func TestExactObjectFeatureIsExplicitAndReadOnly(t *testing.T) {
	scope := GrantScope{Connection: "conn", Objects: []ObjectRef{{Namespace: "sales", Name: "orders"}, {Name: "customers"}}, Requires: []string{ObjectsExact}}
	require.NoError(t, scope.ValidateMVP([]Permission{DataRead}))
	for name, mutate := range map[string]func(*GrantScope){
		"unknown requires":         func(s *GrantScope) { s.Requires = []string{"objects"} },
		"missing requires":         func(s *GrantScope) { s.Requires = nil },
		"duplicate requires":       func(s *GrantScope) { s.Requires = []string{ObjectsExact, ObjectsExact} },
		"duplicate objects":        func(s *GrantScope) { s.Objects = append(s.Objects, s.Objects[0]) },
		"wildcard":                 func(s *GrantScope) { s.Objects = []ObjectRef{{Name: "ord*"}} },
		"empty object":             func(s *GrantScope) { s.Objects = []ObjectRef{{Namespace: "sales"}} },
		"space namespace":          func(s *GrantScope) { s.Objects = []ObjectRef{{Namespace: " sales", Name: "orders"}} },
		"empty objects":            func(s *GrantScope) { s.Objects = []ObjectRef{} },
		"requires without objects": func(s *GrantScope) { s.Objects = nil },
		"schema":                   func(s *GrantScope) { s.Schemas = []Selector{} },
		"exclude":                  func(s *GrantScope) { s.Exclude = []Selector{} },
		"record":                   func(s *GrantScope) { s.Records = &RecordScope{} },
		"field":                    func(s *GrantScope) { s.Fields = &FieldScope{} },
		"request":                  func(s *GrantScope) { s.Request = &RequestBinding{} },
	} {
		t.Run(name, func(t *testing.T) {
			changed := scope
			changed.Objects = append([]ObjectRef(nil), scope.Objects...)
			mutate(&changed)
			err := changed.ValidateMVP([]Permission{DataRead})
			require.ErrorIs(t, err, access.ErrAccessDenied)
			var denial *Denial
			require.ErrorAs(t, err, &denial)
			require.Equal(t, "UNSUPPORTED_SCOPE", denial.Code)
		})
	}
	for _, permissions := range [][]Permission{nil, {DataWrite}, {QueryRaw}, {DataRead, QueryRaw}, {DataInsert}, {DataUpdate}, {DataDelete}, {DataReveal}, {QueryTugQL}} {
		require.Error(t, scope.ValidateMVP(permissions))
	}
}

func TestGrantWireRoundTripAndValidation(t *testing.T) {
	grant := fixtureGrant(time.Now())
	wire, err := json.Marshal(grant)
	require.NoError(t, err)
	decoded, err := DecodeGrant(wire)
	require.NoError(t, err)
	require.Equal(t, grant.ID, decoded.ID)
	require.Equal(t, grant.Subject, decoded.Subject)
	for name, mutate := range map[string]func(*Grant){
		"bad subject":                    func(g *Grant) { g.Subject.ID = "" },
		"bad requester":                  func(g *Grant) { g.RequestedBy.Kind = "application" },
		"expiry missing":                 func(g *Grant) { g.ExpiresAt = time.Time{} },
		"reason missing":                 func(g *Grant) { g.Reason = "" },
		"bad prior":                      func(g *Grant) { g.PriorGrant = " bad" },
		"bad case":                       func(g *Grant) { g.CaseRef = &CaseRef{Kind: "incident"} },
		"later not-before":               func(g *Grant) { now := time.Now(); g.NotBefore = &now },
		"bad id":                         func(g *Grant) { g.ID = "" },
		"no revision":                    func(g *Grant) { g.Revision = 0 },
		"computed expired is not stored": func(g *Grant) { g.State = Expired },
		"unknown state":                  func(g *Grant) { g.State = "active" },
		"invalid decision":               func(g *Grant) { g.Decision = &Decision{} },
		"unsupported permission":         func(g *Grant) { g.Permissions = []Permission{DataWrite} },
	} {
		t.Run(name, func(t *testing.T) {
			changed := grant
			mutate(&changed)
			require.Error(t, changed.ValidateMVP())
			encoded, err := json.Marshal(changed)
			require.NoError(t, err)
			_, err = DecodeGrant(encoded)
			require.Error(t, err)
		})
	}
	_, err = DecodeGrant([]byte(`{"scope":{"connection":"one","CONNECTION":"two","requires":[]}}`))
	require.Error(t, err)
	for _, state := range []GrantState{Requested, Approved, Refused, Withdrawn, Revoked} {
		grant.State = state
		require.NoError(t, grant.ValidateMVP())
	}
}

func TestSubjectAndSafeDenial(t *testing.T) {
	for _, kind := range []SubjectKind{User, Service, Agent} {
		require.NoError(t, (Subject{Realm: "realm", Kind: kind, ID: "id"}).Validate())
	}
	for _, subject := range []Subject{{}, {Realm: "realm", Kind: User, ID: " id"}, {Realm: "realm", Kind: User, ID: "id\n"},
		{Realm: strings.Repeat("a", 257), Kind: User, ID: "id"}, {Realm: "realm", Kind: User, ID: string([]byte{255})}} {
		require.Error(t, subject.Validate())
	}
	err := &Denial{Connection: "orders", Capability: DataRead, Code: "READ_DENIED"}
	require.True(t, errors.Is(err, access.ErrAccessDenied))
	require.Contains(t, err.Error(), `connection="orders"`)
	require.Contains(t, err.Error(), `capability="data.read"`)
	require.Contains(t, err.Error(), "READ_DENIED")
}
