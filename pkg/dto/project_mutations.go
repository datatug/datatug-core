package dto

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/datatug/datatug-core/pkg/datatug"
	"github.com/strongo/validation"
)

var operationIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// These errors have stable meanings across local, Firestore and Git adapters.
// HTTP transports map conflicts to 409 and unsupported capabilities to 400.
var (
	ErrQueryRevisionConflict  = errors.New("query revision conflict")
	ErrBranchHeadConflict     = errors.New("branch head conflict")
	ErrOperationConflict      = errors.New("operation id was used for another mutation")
	ErrUnsupportedCapability  = errors.New("project store does not support this operation")
	ErrInitializationRequired = errors.New("repository needs an initialized branch")
)

// SaveQueryRequest is the additive, storage-neutral query write contract used
// by the local API and DataTug Cloud. Existing create_query and update_query
// contracts remain unchanged. A Git store requires Branch and
// ExpectedBranchHead; a non-Git store rejects those fields at the adapter.
// OperationID lets an adapter recover a committed result after a retry.
type SaveQueryRequest struct {
	ProjectRef
	Branch             string                         `json:"branch,omitempty"`
	ExpectedBranchHead string                         `json:"expectedBranchHead,omitempty"`
	OperationID        string                         `json:"operationId"`
	IfNoneMatch        bool                           `json:"ifNoneMatch,omitempty"`
	IfMatch            string                         `json:"ifMatch,omitempty"`
	Query              datatug.QueryDefWithFolderPath `json:"query"`
}

// Validate checks provider-independent inputs. Store adapters additionally
// validate the selected branch, repository, revision and project authority.
func (v SaveQueryRequest) Validate() error {
	if err := v.ProjectRef.Validate(); err != nil {
		return err
	}
	if !operationIDPattern.MatchString(v.OperationID) {
		return validation.NewErrBadRequestFieldValue("operationId", "must be 1..128 safe characters")
	}
	if v.IfNoneMatch == (v.IfMatch != "") {
		return validation.NewErrBadRequestFieldValue("ifNoneMatch/ifMatch", "set exactly one write condition")
	}
	if (v.Branch == "") != (v.ExpectedBranchHead == "") {
		return validation.NewErrBadRequestFieldValue("branch/expectedBranchHead", "set both or neither")
	}
	if strings.TrimSpace(v.Branch) != v.Branch || strings.TrimSpace(v.ExpectedBranchHead) != v.ExpectedBranchHead {
		return validation.NewErrBadRequestFieldValue("branch/expectedBranchHead", "must not contain surrounding whitespace")
	}
	if err := v.Query.Validate(); err != nil {
		return err
	}
	return nil
}

// PayloadDigest binds an operation receipt to the complete immutable save
// intent, including its selected branch/head and query body. OperationID is
// intentionally excluded so retries with the same intent have one digest.
// Call Validate before computing it; this method does not authorize a write.
func (v SaveQueryRequest) PayloadDigest() (string, error) {
	payload := struct {
		ProjectRef
		Branch             string                         `json:"branch,omitempty"`
		ExpectedBranchHead string                         `json:"expectedBranchHead,omitempty"`
		IfNoneMatch        bool                           `json:"ifNoneMatch,omitempty"`
		IfMatch            string                         `json:"ifMatch,omitempty"`
		Query              datatug.QueryDefWithFolderPath `json:"query"`
	}{v.ProjectRef, v.Branch, v.ExpectedBranchHead, v.IfNoneMatch, v.IfMatch, v.Query}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode save query payload: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

// OperationScope is derived by the server from a verified actor and resolved
// store. A browser-supplied actor ID must never be trusted here.
type OperationScope struct {
	ActorID   string
	StoreID   string
	ProjectID string
	Branch    string
	Kind      string
}

// OperationReceipt describes a previously committed mutation. A provider
// persists it durably and rechecks actor authorization before replaying it.
type OperationReceipt struct {
	Scope         OperationScope
	OperationID   string
	PayloadDigest string
	Result        SaveQueryResponse
}

// MatchSaveRetry rejects a reused operation ID with a different scope or
// payload. Authorization must already have been rechecked by the caller.
func (r OperationReceipt) MatchSaveRetry(scope OperationScope, request SaveQueryRequest) error {
	digest, err := request.PayloadDigest()
	if err != nil {
		return err
	}
	if r.Scope != scope || r.OperationID != request.OperationID || r.PayloadDigest != digest {
		return ErrOperationConflict
	}
	return nil
}

// ProjectCapabilities tells clients which controls a selected store offers;
// the server must still authorize every individual operation.
type ProjectCapabilities struct {
	QueryRead      bool `json:"queryRead"`
	QuerySave      bool `json:"querySave"`
	Branches       bool `json:"branches"`
	BranchMerge    bool `json:"branchMerge"`
	ReviewedCommit bool `json:"reviewedCommit"`
	PullCurrent    bool `json:"pullCurrent"`
	PushCurrent    bool `json:"pushCurrent"`
}

// GetQueryRequest loads the complete query pair at an explicit selected Git
// branch. Firestore rejects Branch; local and GitHub stores require it when
// branch mode is enabled.
type GetQueryRequest struct {
	ProjectRef
	ID     string `json:"id"`
	Branch string `json:"branch,omitempty"`
}

type GetQueryResponse = SaveQueryResponse

// ProjectQueryAdapter is the storage-neutral seam for DataTug Cloud's
// Firestore/GitHub adapters and the CLI's local filestore adapter. Scope is
// created from the authenticated serving principal; implementations check
// authorization on every call, including operation-receipt replay.
type ProjectQueryAdapter interface {
	Capabilities(context.Context, ProjectRef) (ProjectCapabilities, error)
	GetQuery(context.Context, GetQueryRequest) (*GetQueryResponse, error)
	SaveQuery(context.Context, OperationScope, SaveQueryRequest) (*SaveQueryResponse, error)
}

// SaveQueryResponse reports the bytes persisted and the revision to send as
// IfMatch on the next write. A Git adapter also returns the resulting branch
// head; both revisions are opaque to clients.
type SaveQueryResponse struct {
	Query      datatug.QueryDefWithFolderPath `json:"query"`
	Revision   string                         `json:"revision"`
	BranchHead string                         `json:"branchHead,omitempty"`
}

// ProjectBranchRef addresses one named Git branch. Cloud project stores do
// not support branches and must return an unsupported-capability response.
type ProjectBranchRef struct {
	ProjectRef
	Branch string `json:"branch"`
}

// CreateBranchRequest creates a branch only while SourceHead still matches.
type CreateBranchRequest struct {
	ProjectRef
	Branch      string `json:"branch"`
	FromBranch  string `json:"fromBranch"`
	SourceHead  string `json:"sourceHead"`
	OperationID string `json:"operationId"`
}

// CompareBranchesRequest pins both sides so a diff cannot silently change
// between the user's review and subsequent merge request.
type CompareBranchesRequest struct {
	ProjectRef
	SourceBranch string `json:"sourceBranch"`
	SourceHead   string `json:"sourceHead"`
	TargetBranch string `json:"targetBranch"`
	TargetHead   string `json:"targetHead"`
}

// MergeBranchesRequest asks the Git provider to merge the reviewed heads.
// An adapter must return an explicit pending/proposal result when branch
// protection requires a PR; it must never report that as merged.
type MergeBranchesRequest struct {
	CompareBranchesRequest
	OperationID string `json:"operationId"`
}

type MergeBranchesResponse struct {
	State       string `json:"state"` // merged | proposal | conflict | pending
	TargetHead  string `json:"targetHead,omitempty"`
	ProposalURL string `json:"proposalUrl,omitempty"`
}
