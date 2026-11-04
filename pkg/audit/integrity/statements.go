package integrity

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"time"
)

const labelStatement = "datatug/audit/v1/statement"

// Statement kinds, bound into every signed message.
const (
	kindRoot      byte = 1
	kindExpiry    byte = 2
	kindTombstone byte = 3
	kindHold      byte = 4
)

// Signer signs statement messages. Production: a Cloud KMS asymmetric key
// held by the platform operator and usable only by the sealing service
// account. KeyID names the key version so verifiers can pick the public key.
type Signer interface {
	KeyID() string
	Sign(ctx context.Context, message []byte) ([]byte, error)
}

// Verifier checks a signature made by the key named keyID. It returns an
// error wrapping [ErrUnknownKey] for an untrusted key.
type Verifier interface {
	Verify(keyID string, message, signature []byte) error
}

type statement interface {
	RootStatement | ExpiryStatement | TombstoneStatement | HoldStatement
	message() []byte
	validate() error
}

// Signed is a statement with its signature.
type Signed[S statement] struct {
	Statement S      `json:"statement"`
	KeyID     string `json:"keyId"`
	Signature []byte `json:"signature"`
}

// Sign validates s and signs it with signer.
func Sign[S statement](ctx context.Context, signer Signer, s S) (Signed[S], error) {
	if err := s.validate(); err != nil {
		return Signed[S]{}, err
	}
	sig, err := signer.Sign(ctx, s.message())
	if err != nil {
		return Signed[S]{}, fmt.Errorf("audit integrity: sign: %w", err)
	}
	return Signed[S]{Statement: s, KeyID: signer.KeyID(), Signature: sig}, nil
}

// Verify validates the statement and checks its signature. A signature
// failure wraps [ErrBadSignature] (and [ErrUnknownKey] when the key is not
// trusted).
func (x Signed[S]) Verify(v Verifier) error {
	if err := x.Statement.validate(); err != nil {
		return err
	}
	if err := v.Verify(x.KeyID, x.Statement.message(), x.Signature); err != nil {
		return fmt.Errorf("%w: %w", ErrBadSignature, err)
	}
	return nil
}

func header(kind byte) *builder {
	w := &builder{}
	w.label(labelStatement)
	w.byte1(kind)
	return w
}

func (s RootStatement) validate() error { return s.Window.Validate() }

func (s RootStatement) message() []byte {
	w := header(kindRoot)
	s.write(w)
	return w.b
}

// ExpiryStatement says "window W, with root R, expires at T". The sealer
// signs it at sealing so that a later absence is pre-explained.
type ExpiryStatement struct {
	Window    WindowID  `json:"window"`
	Root      Hash      `json:"root"`
	ExpiresAt time.Time `json:"expiresAt"`
}

func (s ExpiryStatement) validate() error {
	if err := s.Window.Validate(); err != nil {
		return err
	}
	if s.ExpiresAt.IsZero() {
		return fmt.Errorf("%w: expiry without a time", ErrInvalidStatement)
	}
	return nil
}

func (s ExpiryStatement) message() []byte {
	w := header(kindExpiry)
	w.window(s.Window)
	w.hash(s.Root)
	w.time(s.ExpiresAt)
	return w.b
}

// TombstoneStatement records that a window's events were deleted by
// retention. It carries the window's seal so the chain still verifies
// across the deleted window.
type TombstoneStatement struct {
	Seal      RootStatement `json:"seal"`
	ExpiresAt time.Time     `json:"expiresAt"`
	DeletedAt time.Time     `json:"deletedAt"`
}

func (s TombstoneStatement) validate() error {
	if err := s.Seal.validate(); err != nil {
		return err
	}
	if s.ExpiresAt.IsZero() || s.DeletedAt.IsZero() {
		return fmt.Errorf("%w: tombstone without expiry or deletion time", ErrInvalidStatement)
	}
	if s.DeletedAt.Before(s.ExpiresAt) {
		return ErrTombstoneBeforeExpiry
	}
	return nil
}

func (s TombstoneStatement) message() []byte {
	w := header(kindTombstone)
	s.Seal.write(w)
	w.time(s.ExpiresAt)
	w.time(s.DeletedAt)
	return w.b
}

// HoldAction is what a [HoldStatement] records.
type HoldAction string

// Hold actions.
const (
	HoldPlace   HoldAction = "place"
	HoldRelease HoldAction = "release"
)

// HoldStatement records placing or releasing a legal hold on the windows
// starting From through To. Actors are the pseudonymous people who placed or
// released it; a release names at least two distinct people. Who may do so is
// decided by the authorization layer, not here.
type HoldStatement struct {
	HoldID  string     `json:"holdId"`
	Space   string     `json:"space"`
	Project string     `json:"project"`
	From    time.Time  `json:"from"`
	To      time.Time  `json:"to"`
	Action  HoldAction `json:"action"`
	At      time.Time  `json:"at"`
	Actors  []ActorRef `json:"actors"`
}

func (s HoldStatement) validate() error {
	from := WindowID{Space: s.Space, Project: s.Project, Start: s.From}
	if err := from.Validate(); err != nil {
		return err
	}
	if err := (WindowID{Space: s.Space, Project: s.Project, Start: s.To}).Validate(); err != nil {
		return err
	}
	switch {
	case s.HoldID == "":
		return fmt.Errorf("%w: hold without id", ErrInvalidStatement)
	case s.To.Before(s.From):
		return fmt.Errorf("%w: hold ends before it starts", ErrInvalidStatement)
	case s.At.IsZero():
		return fmt.Errorf("%w: hold without a time", ErrInvalidStatement)
	case s.Action != HoldPlace && s.Action != HoldRelease:
		return fmt.Errorf("%w: hold action %q", ErrInvalidStatement, s.Action)
	case len(s.Actors) == 0:
		return fmt.Errorf("%w: hold without actors", ErrInvalidStatement)
	}
	if s.Action == HoldRelease && distinct(s.Actors) < 2 {
		return ErrHoldReleaseNeedsTwo
	}
	return nil
}

func distinct(refs []ActorRef) int {
	seen := map[ActorRef]bool{}
	for _, r := range refs {
		if r != "" {
			seen[r] = true
		}
	}
	return len(seen)
}

func (s HoldStatement) message() []byte {
	w := header(kindHold)
	w.str(s.HoldID)
	w.str(s.Space)
	w.str(s.Project)
	w.time(s.From)
	w.time(s.To)
	w.str(string(s.Action))
	w.time(s.At)
	w.u32(uint32(len(s.Actors)))
	for _, a := range s.Actors {
		w.str(string(a))
	}
	return w.b
}

// covers reports whether the hold's range includes window w.
func (s HoldStatement) covers(w WindowID) bool {
	return s.Space == w.Space && s.Project == w.Project && !w.Start.Before(s.From) && !w.Start.After(s.To)
}

// Ed25519Signer signs with an Ed25519 private key (tests, local tools).
type Ed25519Signer struct {
	keyID string
	key   ed25519.PrivateKey
}

// NewEd25519Signer returns a signer for key under keyID.
func NewEd25519Signer(keyID string, key ed25519.PrivateKey) *Ed25519Signer {
	return &Ed25519Signer{keyID: keyID, key: key}
}

// KeyID implements [Signer].
func (s *Ed25519Signer) KeyID() string { return s.keyID }

// Sign implements [Signer].
func (s *Ed25519Signer) Sign(ctx context.Context, message []byte) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return ed25519.Sign(s.key, message), nil
}

// Ed25519Verifier trusts the listed public keys by key id.
type Ed25519Verifier map[string]ed25519.PublicKey

// Verify implements [Verifier].
func (v Ed25519Verifier) Verify(keyID string, message, signature []byte) error {
	pub, ok := v[keyID]
	if !ok || len(pub) != ed25519.PublicKeySize {
		return fmt.Errorf("%w: %q", ErrUnknownKey, keyID)
	}
	if !ed25519.Verify(pub, message, signature) {
		return ErrBadSignature
	}
	return nil
}
