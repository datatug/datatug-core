package dto

import (
	"encoding/json"
	"testing"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/stretchr/testify/require"
)

func demoSaveQuery() SaveQueryRequest {
	return SaveQueryRequest{
		ProjectRef: ProjectRef{StoreID: "github", ProjectID: "demo-project-1"},
		Branch:     "investigate", ExpectedBranchHead: "0123456789abcdef", OperationID: "save-1", IfNoneMatch: true,
		Query: datatug.QueryDefWithFolderPath{
			FolderPath: "~",
			QueryDef: datatug.QueryDef{
				ProjectItem: datatug.ProjectItem{ProjItemBrief: datatug.ProjItemBrief{ID: "customer", Title: "Customers"}},
				Type:        datatug.QueryTypeDTQL, Text: "SELECT CustomerId, FirstName FROM Customer",
				Federation: &datatug.QueryFederation{OVDBBaseURL: "https://demodb.dev/ovdb", Tables: []datatug.QueryFederationTable{{Database: "chinook", Name: "Customer", Fields: []string{"CustomerId", "FirstName"}}}},
			},
		},
	}
}

func TestSaveQueryContractRoundTripAndRetryBinding(t *testing.T) {
	request := demoSaveQuery()
	require.NoError(t, request.Validate())
	encoded, err := json.Marshal(request)
	require.NoError(t, err)
	var decoded SaveQueryRequest
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	require.Equal(t, request, decoded)
	digest, err := request.PayloadDigest()
	require.NoError(t, err)
	scope := OperationScope{ActorID: "actor-1", StoreID: "github", ProjectID: request.ProjectID, Branch: request.Branch, Kind: "save-query"}
	receipt := OperationReceipt{Scope: scope, OperationID: request.OperationID, PayloadDigest: digest}
	require.NoError(t, receipt.MatchSaveRetry(scope, decoded))

	changed := decoded
	changed.Query.Text = "SELECT CustomerId FROM Customer"
	require.ErrorIs(t, receipt.MatchSaveRetry(scope, changed), ErrOperationConflict)
	changed = decoded
	changed.ExpectedBranchHead = "different-head"
	require.ErrorIs(t, receipt.MatchSaveRetry(scope, changed), ErrOperationConflict)
	changed = decoded
	changed.Query.Federation = &datatug.QueryFederation{OVDBBaseURL: "https://demodb.dev/ovdb", Tables: []datatug.QueryFederationTable{{Database: "adventureworks", Schema: "Person", Name: "Person", Fields: []string{"BusinessEntityID"}}}}
	require.ErrorIs(t, receipt.MatchSaveRetry(scope, changed), ErrOperationConflict)
	otherActor := scope
	otherActor.ActorID = "actor-2"
	require.ErrorIs(t, receipt.MatchSaveRetry(otherActor, decoded), ErrOperationConflict)
	otherProject := scope
	otherProject.ProjectID = "another"
	require.ErrorIs(t, receipt.MatchSaveRetry(otherProject, decoded), ErrOperationConflict)
	otherBranch := scope
	otherBranch.Branch = "main"
	require.ErrorIs(t, receipt.MatchSaveRetry(otherBranch, decoded), ErrOperationConflict)
	changed = decoded
	changed.OperationID = "save-2"
	require.ErrorIs(t, receipt.MatchSaveRetry(scope, changed), ErrOperationConflict)
}

func TestSaveQueryContractPreconditions(t *testing.T) {
	request := demoSaveQuery()
	cases := map[string]func(*SaveQueryRequest){
		"missing project":    func(r *SaveQueryRequest) { r.ProjectID = "" },
		"missing operation":  func(r *SaveQueryRequest) { r.OperationID = "" },
		"unsafe operation":   func(r *SaveQueryRequest) { r.OperationID = "../escape" },
		"missing condition":  func(r *SaveQueryRequest) { r.IfNoneMatch = false },
		"both conditions":    func(r *SaveQueryRequest) { r.IfMatch = "rev1" },
		"missing head":       func(r *SaveQueryRequest) { r.ExpectedBranchHead = "" },
		"missing branch":     func(r *SaveQueryRequest) { r.Branch = "" },
		"branch whitespace":  func(r *SaveQueryRequest) { r.Branch = " investigate " },
		"credential in body": func(r *SaveQueryRequest) { r.Query.Text = "SELECT 'password=secret'" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			invalid := request
			change(&invalid)
			require.Error(t, invalid.Validate())
		})
	}
	request.IfNoneMatch = false
	request.IfMatch = "rev1"
	require.NoError(t, request.Validate())
}

func TestSaveQueryDigestRejectsUnencodablePayload(t *testing.T) {
	request := demoSaveQuery()
	request.Query.Parameters = datatug.Parameters{{DefaultValue: func() {}}}
	_, err := request.PayloadDigest()
	require.ErrorContains(t, err, "encode save query payload")
	receipt := OperationReceipt{}
	require.ErrorContains(t, receipt.MatchSaveRetry(OperationScope{}, request), "encode save query payload")
}
