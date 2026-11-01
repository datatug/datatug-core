package dataaccess

import (
	"context"
	"fmt"

	"github.com/dal-go/dalgo/access"
	"github.com/dal-go/dalgo/dal"
)

// TableIdentityRequest contains the written identity of one frozen structured
// source. Connection is the independently authenticated Core source binding;
// neither field is evidence for the verifier to echo back.
type TableIdentityRequest struct {
	Connection ConnectionRef
	Table      access.TableName
}

// TableIdentityFacts identifies the source and native logical table that the
// exact written components execute. Components are values, never SQL syntax.
// This contract does not assert physical relation lineage across DDL changes.
type TableIdentityFacts struct {
	Connection ConnectionRef
	Table      access.TableName
}

// TableIdentityVerifier is a trusted host fact port, not a permission evaluator.
// It must independently corroborate the actual supplied opened DB and establish
// immutable source routing and identifier/compiler semantics for its entire
// secured DB/reader lifetime (or pin an equivalent execution span). Sampling a
// mutable pool's catalog before/after I/O is insufficient. It must refuse when
// native folding, quoting, search paths, temporary aliases or routing prevent
// proof that the written components execute byte-exactly. The closed compiler must
// preserve source identity for every alias admitted by this profile. Lookup is
// read-only identity metadata work: no query payload, mutation or secret
// disclosure is permitted. It receives no grants.
// Core cannot detect a malicious trusted port lying about the supplied handle.
// No production opener is certified merely by implementing this interface.
type TableIdentityVerifier interface {
	VerifyTableIdentity(context.Context, dal.DB, TableIdentityRequest) (TableIdentityFacts, error)
}

// SecureDBWithTableIdentity opts one actual opened DB into the bounded explicit-
// schema key-enumeration profile. The host owns the verifier's execution-span
// invariants; all permission decisions remain in Core and DALgo. Point keys,
// transactions and result transport retain their connection-wide requirements.
// Core permission candidacy and its selected ceiling precede fact lookup.
// Additional DALgo context policies still intersect before data dispatch; their
// denial may follow trusted identity-metadata lookup.
func (a *Authority) SecureDBWithTableIdentity(db dal.DB, verifier TableIdentityVerifier) (Database, error) {
	if isNilValue(verifier) {
		return nil, fmt.Errorf("dataaccess: table identity verifier is required")
	}
	return a.secureDB(db, verifier)
}

type tableIdentityBinding struct {
	db       dal.DB
	verifier TableIdentityVerifier
	span     TableReadSpan // explicit recordset profile, never a transaction disguised as DB
	request  TableIdentityRequest
}

func (b *tableIdentityBinding) verify(ctx context.Context) error {
	var facts TableIdentityFacts
	var err error
	if b.span != nil {
		facts, err = b.span.Verify(ctx, b.request)
	} else {
		facts, err = b.verifier.VerifyTableIdentity(ctx, b.db, b.request)
	}
	if err != nil {
		return err
	}
	if !sameConnection(facts.Connection, b.request.Connection) || facts.Table != b.request.Table {
		return fmt.Errorf("dataaccess: native table identity mismatch")
	}
	return ctx.Err()
}
