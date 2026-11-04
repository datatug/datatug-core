package integrity

import (
	"crypto/rand"
	"encoding/base32"
	"fmt"
	"io"
	"strings"
	"sync"
)

// ActorRef is a Space-scoped pseudonymous person id ("act-…"). It is random,
// never derived from the user id, and resolvable only through the erasable
// identity map.
type ActorRef string

const actorRefPrefix = "act-"

var refEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// NewActorRef draws a 128-bit random pseudonym from r, or from crypto/rand
// when r is nil.
func NewActorRef(r io.Reader) (ActorRef, error) {
	if r == nil {
		r = rand.Reader
	}
	var b [16]byte
	if _, err := io.ReadFull(r, b[:]); err != nil {
		return "", fmt.Errorf("audit integrity: read pseudonym: %w", err)
	}
	return ActorRef(actorRefPrefix + strings.ToLower(refEncoding.EncodeToString(b[:]))), nil
}

// IdentityMap is the reference, in-memory shape of the erasable pseudonym ↔
// user map (production: spaces/{s}/ext/datatug/audit/_identity/actors/{ref}).
// It is safe for concurrent use.
type IdentityMap struct {
	mu     sync.Mutex
	rand   io.Reader
	byUser map[spaceKey]ActorRef
	byRef  map[spaceKey]string
}

type spaceKey struct{ space, id string }

// maxPseudonymAttempts bounds retries on a (practically impossible) collision.
const maxPseudonymAttempts = 3

// NewIdentityMap returns an empty map drawing pseudonyms from r (nil: crypto/rand).
func NewIdentityMap(r io.Reader) *IdentityMap {
	return &IdentityMap{rand: r, byUser: map[spaceKey]ActorRef{}, byRef: map[spaceKey]string{}}
}

// Pseudonym returns the user's pseudonym in space, creating one on first use.
func (m *IdentityMap) Pseudonym(space, userID string) (ActorRef, error) {
	if space == "" || userID == "" {
		return "", ErrInvalidArgument
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if ref, ok := m.byUser[spaceKey{space, userID}]; ok {
		return ref, nil
	}
	for range maxPseudonymAttempts {
		ref, err := NewActorRef(m.rand)
		if err != nil {
			return "", err
		}
		if _, taken := m.byRef[spaceKey{space, string(ref)}]; taken {
			continue
		}
		m.byUser[spaceKey{space, userID}] = ref
		m.byRef[spaceKey{space, string(ref)}] = userID
		return ref, nil
	}
	return "", ErrPseudonymCollision
}

// Resolve returns the user behind ref in space, if not erased.
func (m *IdentityMap) Resolve(space string, ref ActorRef) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.byRef[spaceKey{space, string(ref)}]
	return u, ok
}

// Erase deletes the user's entry in space and returns the pseudonym that no
// longer resolves. Events and roots are untouched.
func (m *IdentityMap) Erase(space, userID string) (ActorRef, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ref, ok := m.byUser[spaceKey{space, userID}]
	if !ok {
		return "", false
	}
	delete(m.byUser, spaceKey{space, userID})
	delete(m.byRef, spaceKey{space, string(ref)})
	return ref, true
}
