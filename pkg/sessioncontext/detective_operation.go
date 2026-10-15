package sessioncontext

import (
	"context"
	"encoding/hex"
	"strings"
)

// AcceptedDetectiveOperation is metadata-only transport recovery. Its request
// digest is the host's canonical client body, not the native operation payload.
// Adapters compare it before reading mutable Check/Case definitions on retry.
type AcceptedDetectiveOperation struct {
	Identity            DetectiveIdentity `json:"identity"`
	ClientRequestDigest string            `json:"clientRequestDigest"`
}

// ResolveOperation uses the operation receipt and accepted work committed with
// the private pin as one durable mapping. It performs no content authorization,
// private input read, source execution or new native identity generation. The
// configured active-account gate still runs for the accepted actor. Legacy
// acceptances without a client request binding cannot enter this transport path.
func (o *DetectiveOwner) ResolveOperation(ctx context.Context, p Principal, clientOpID string) (AcceptedDetectiveOperation, error) {
	var out AcceptedDetectiveOperation
	n := o.service.nativeNamespace
	if n == nil || p.Actor == "" || p.DatabaseID != n.DatabaseID || p.ProjectID != n.ProjectID {
		return out, ErrForbidden
	}
	if !safeID.MatchString(clientOpID) {
		return out, ErrInvalid
	}
	err := o.service.repo.View(ctx, func(r Reader) error {
		var receipt Receipt
		if _, err := r.Load(ctx, Operations, opKey(p, clientOpID), &receipt); err != nil {
			return err
		}
		if receipt.Action != "pinDetectiveInput" {
			return ErrNotFound
		}
		if receipt.OpID != clientOpID || receipt.Actor != p.Actor || !safeID.MatchString(receipt.SessionID) || !safeID.MatchString(receipt.ResourceID) || receipt.ResourceVersion != 1 || receipt.SessionVersion < 1 {
			return ErrConflict
		}
		var work AcceptedDetectiveWork
		if _, err := r.Load(ctx, DetectiveWork, receipt.ResourceID, &work); err != nil {
			return err
		}
		if work.ClientOperationID != clientOpID || work.AcceptanceDigest == "" || work.AcceptanceDigest != receipt.PayloadDigest || work.SessionID != receipt.SessionID || work.RunID != receipt.ResourceID || work.InputSnapshotID != work.Reference.InputSnapshotID || receipt.Scope == nil || receipt.Scope.EnvironmentID != work.Reference.EnvironmentID || receipt.Scope.IdentityDomain != work.Reference.IdentityDomain {
			return ErrConflict
		}
		clientDigest, err := hex.DecodeString(work.ClientRequestDigest)
		if err != nil || len(clientDigest) != 32 || strings.ToLower(work.ClientRequestDigest) != work.ClientRequestDigest {
			return ErrUnavailable
		}
		d := work.Dispatch
		id := DetectiveIdentity{work.Namespace, work.Owner, work.SessionID, work.RunID, d.NativeStoreID, d.NativeSpaceID, d.NativeProjectID, d.NativeOperationID, d.NativeRequestDigest}
		if err = o.identity(ctx, p, id, false); err != nil {
			return err
		}
		if _, _, err = o.load(ctx, r, id); err != nil {
			return err
		}
		out = AcceptedDetectiveOperation{Identity: id, ClientRequestDigest: work.ClientRequestDigest}
		return nil
	})
	if err != nil {
		return AcceptedDetectiveOperation{}, err
	}
	return out, nil
}
