package integrity

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"time"
)

// Hash is a SHA-256 digest. It marshals as lowercase hex.
type Hash [sha256.Size]byte

// String returns the lowercase hex form.
func (h Hash) String() string { return hex.EncodeToString(h[:]) }

// IsZero reports whether h is all zero bytes (the genesis previous root).
func (h Hash) IsZero() bool { return h == Hash{} }

// MarshalText implements encoding.TextMarshaler.
func (h Hash) MarshalText() ([]byte, error) { return []byte(h.String()), nil }

// UnmarshalText implements encoding.TextUnmarshaler. Only the canonical
// lowercase 64-character hex form is accepted.
func (h *Hash) UnmarshalText(b []byte) error {
	parsed, err := ParseHash(string(b))
	if err != nil {
		return err
	}
	*h = parsed
	return nil
}

// ParseHash parses the canonical lowercase hex form of a hash.
func ParseHash(s string) (Hash, error) {
	var h Hash
	if len(s) != 2*len(h) {
		return h, fmt.Errorf("%w: length %d", ErrMalformedHash, len(s))
	}
	if _, err := hex.Decode(h[:], []byte(s)); err != nil {
		return h, fmt.Errorf("%w: %v", ErrMalformedHash, err)
	}
	if h.String() != s {
		return Hash{}, fmt.Errorf("%w: not lowercase", ErrMalformedHash)
	}
	return h, nil
}

// builder accumulates an unambiguous binary encoding.
type builder struct{ b []byte }

func (w *builder) byte1(v byte)   { w.b = append(w.b, v) }
func (w *builder) u32(v uint32)   { w.b = binary.BigEndian.AppendUint32(w.b, v) }
func (w *builder) u64(v uint64)   { w.b = binary.BigEndian.AppendUint64(w.b, v) }
func (w *builder) hash(h Hash)    { w.b = append(w.b, h[:]...) }
func (w *builder) label(s string) { w.b = append(append(w.b, s...), 0) }
func (w *builder) str(s string)   { w.u32(uint32(len(s))); w.b = append(w.b, s...) }

// time writes seconds and nanoseconds since the Unix epoch; the zero time is
// written as its own (distinct) value.
func (w *builder) time(t time.Time) {
	w.u64(uint64(t.Unix()))
	w.u32(uint32(t.Nanosecond()))
}

func (w *builder) window(id WindowID) {
	w.str(id.Space)
	w.str(id.Project)
	w.time(id.Start)
}

func (w *builder) sum() Hash { return sha256.Sum256(w.b) }
