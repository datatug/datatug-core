package dataaccess

import (
	"context"
	"fmt"
	"reflect"

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
	if nilTableIdentityVerifier(verifier) {
		return nil, fmt.Errorf("dataaccess: table identity verifier is required")
	}
	return a.secureDB(db, verifier)
}

func nilTableIdentityVerifier(verifier TableIdentityVerifier) bool {
	if verifier == nil {
		return true
	}
	switch reflect.ValueOf(verifier).Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return reflect.ValueOf(verifier).IsNil()
	default:
		return false
	}
}

type tableIdentityBinding struct {
	db       dal.DB
	verifier TableIdentityVerifier
	request  TableIdentityRequest
}

func (b *tableIdentityBinding) verify(ctx context.Context) error {
	facts, err := b.verifier.VerifyTableIdentity(ctx, b.db, b.request)
	if err != nil {
		return err
	}
	if facts.Connection.Space != b.request.Connection.Space || facts.Connection.ID != b.request.Connection.ID ||
		!sameFingerprint(facts.Connection.Fingerprint, b.request.Connection.Fingerprint) || facts.Table != b.request.Table {
		return fmt.Errorf("dataaccess: native table identity mismatch")
	}
	return ctx.Err()
}
