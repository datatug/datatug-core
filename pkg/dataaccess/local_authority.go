package dataaccess

import (
	"fmt"
	"time"
)

// LocalConnectionRef is constructed by the host from its owned source registry
// and independently resolved native identity. It is never model execution input.
// AuthorityID is local provenance, not an enrolled Space or an approval actor.
type LocalConnectionRef struct {
	AuthorityID string
	ID          string
	Fingerprint Fingerprint
}

// NewLocalAuthority uses the same resolver/evaluator/lease boundary for a source
// with no enrolled Space. The trusted host resolver supplies its current baseline
// and source facts and refuses lost local ownership. Local resolutions cannot
// carry grants; no control-plane decision is fabricated for this route.
func NewLocalAuthority(subject Subject, connection LocalConnectionRef, resolve Resolver) (*Authority, error) {
	if err := subject.Validate(); err != nil {
		return nil, err
	}
	fingerprint := connection.Fingerprint
	if resolve == nil || !canonical(connection.AuthorityID) || !canonical(connection.ID) ||
		!canonical(fingerprint.Engine) || !canonical(fingerprint.Database) ||
		(fingerprint.EngineIdentity == "" && fingerprint.Address == "") {
		return nil, fmt.Errorf("dataaccess: local owner, resolver and resolved source required")
	}
	ref := ConnectionRef{ID: connection.ID, Fingerprint: fingerprint, LocalAuthorityID: connection.AuthorityID}
	return &Authority{request: ResolveRequest{Subject: subject, Connection: ref}, resolve: resolve, now: time.Now}, nil
}

func sameConnection(a, b ConnectionRef) bool {
	return a.Space == b.Space && a.LocalAuthorityID == b.LocalAuthorityID && a.ID == b.ID && sameFingerprint(a.Fingerprint, b.Fingerprint)
}
