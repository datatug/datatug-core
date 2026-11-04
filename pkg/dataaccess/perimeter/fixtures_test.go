package perimeter

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/datatug/datatug-core/pkg/dataaccess"
	"github.com/stretchr/testify/require"
)

var (
	asOf      = time.Date(2026, 11, 3, 21, 4, 0, 0, time.UTC)
	red       = user("red")
	green     = user("green")
	blue      = user("blue")
	mallory   = user("mallory")
	priya     = user("priya")
	customers = dataaccess.ObjectRef{Namespace: "billing", Name: "customers"}
	invoices  = dataaccess.ObjectRef{Namespace: "billing", Name: "invoices"}
	secrets   = dataaccess.ObjectRef{Namespace: "billing", Name: "secrets"}
	objects   = []dataaccess.ObjectRef{customers, invoices, secrets}
	members   = []dataaccess.Subject{red, green, blue}
)

const connID = "prod-billing"

func user(id string) dataaccess.Subject {
	return dataaccess.Subject{Realm: "sneat", Kind: dataaccess.User, ID: id}
}

func connRef() dataaccess.ConnectionRef {
	return dataaccess.ConnectionRef{Space: "space-a", ID: connID,
		Fingerprint: dataaccess.Fingerprint{Engine: "recording", Database: "billing", EngineIdentity: "cluster-a"}}
}

func protection(posture dataaccess.ReadPosture) *dataaccess.ConnectionProtection {
	ref := connRef()
	return &dataaccess.ConnectionProtection{ID: ref.ID, Fingerprint: ref.Fingerprint, Name: "Billing", Environment: "production",
		ReadOnly: true, ReadPosture: posture, AgentRead: dataaccess.AgentInherit, Revision: 1}
}

// exactSnapshot: green has standing read on customers; secrets is under a
// mandatory deny for everyone; invoices is requestable.
func exactSnapshot(t *testing.T) *dataaccess.ExactReadSnapshot {
	t.Helper()
	doc := map[string]any{
		"version": 1, "connection": connRef(), "generation": 1, "nativeNamespace": "billing",
		"objects":     objects,
		"standing":    []map[string]any{{"subject": green, "objects": []dataaccess.ObjectRef{customers}}},
		"requestable": []dataaccess.ObjectRef{invoices},
		"denials":     []map[string]any{{"object": secrets}},
	}
	data, err := json.Marshal(doc)
	require.NoError(t, err)
	s, err := dataaccess.DecodeExactReadSnapshot(data)
	require.NoError(t, err)
	return s
}

func grant(id string, subject dataaccess.Subject, expiresAt time.Time, scope ...dataaccess.ObjectRef) StoredGrant {
	requires := []string{}
	if len(scope) > 0 {
		requires = []string{dataaccess.ObjectsExact}
	}
	return StoredGrant{Fingerprint: connRef().Fingerprint, Grant: dataaccess.Grant{ID: id, Subject: subject, RequestedBy: subject, Permissions: []dataaccess.Permission{dataaccess.DataRead},
		Scope:  dataaccess.GrantScope{Connection: connID, Objects: scope, Requires: requires},
		Reason: "investigate", ExpiresAt: expiresAt, State: dataaccess.Approved,
		Decision: &dataaccess.Decision{By: priya, At: asOf.Add(-time.Hour)}, Revision: 1}}
}

func withState(g StoredGrant, state dataaccess.GrantState) StoredGrant {
	g.State = state
	return g
}

func snapshot(t *testing.T, posture dataaccess.ReadPosture, grants ...StoredGrant) Snapshot {
	t.Helper()
	conn := Connection{Ref: connRef(), Protection: protection(posture), Grants: grants, Inventory: objects}
	if posture == dataaccess.ReadExact {
		conn.Exact = exactSnapshot(t)
	}
	return Snapshot{AsOf: asOf, Connections: []Connection{conn}, Members: members}
}

func resource(o dataaccess.ObjectRef) Resource {
	return Resource{Connection: connID, Namespace: o.Namespace, Name: o.Name}
}

func selfDecided(g StoredGrant) StoredGrant {
	g.Decision = &dataaccess.Decision{By: g.Subject, At: asOf.Add(-time.Minute)}
	return g
}

func unsupported(g StoredGrant) StoredGrant {
	g.Scope.Records = &dataaccess.RecordScope{Language: "tugqtree"}
	return g
}
