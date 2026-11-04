package integrity

import (
	"fmt"
	"net/url"
	"time"
)

// WindowDuration is the length of a sealing window (permission model §19.4).
const WindowDuration = 5 * time.Minute

const labelWindow = "datatug/audit/v1/window"

// WindowID identifies one 5-minute window of one Space and project.
// Space-level events use an empty Project.
type WindowID struct {
	Space   string    `json:"space"`
	Project string    `json:"project"`
	Start   time.Time `json:"start"`
}

// WindowFor returns the window an event arriving at t falls into.
func WindowFor(space, project string, t time.Time) WindowID {
	return WindowID{Space: space, Project: project, Start: t.UTC().Truncate(WindowDuration)}
}

// Validate checks that the window has a Space and a 5-minute-aligned start.
func (w WindowID) Validate() error {
	if w.Space == "" {
		return fmt.Errorf("%w: empty space", ErrInvalidWindow)
	}
	if w.Start.IsZero() || w.Start.Nanosecond() != 0 || w.Start.Unix()%int64(WindowDuration/time.Second) != 0 {
		return fmt.Errorf("%w: start %s is not aligned to %s", ErrInvalidWindow, w.Start.Format(time.RFC3339Nano), WindowDuration)
	}
	return nil
}

// Key is the delimiter-safe window key used as the store document id and in
// the event's window field, e.g. "space=s1|project=shop|2026-11-03T21:05".
func (w WindowID) Key() string {
	return "space=" + url.QueryEscape(w.Space) + "|project=" + url.QueryEscape(w.Project) + "|" +
		w.Start.UTC().Format("2006-01-02T15:04")
}

// Next is the following window.
func (w WindowID) Next() WindowID {
	return WindowID{Space: w.Space, Project: w.Project, Start: w.Start.Add(WindowDuration)}
}

// Prev is the preceding window.
func (w WindowID) Prev() WindowID {
	return WindowID{Space: w.Space, Project: w.Project, Start: w.Start.Add(-WindowDuration)}
}

// Equal reports whether both identify the same window.
func (w WindowID) Equal(o WindowID) bool {
	return w.Space == o.Space && w.Project == o.Project && w.Start.Equal(o.Start)
}

// RootStatement is a sealed window: what the sealer signs and anchors.
type RootStatement struct {
	Window     WindowID `json:"window"`
	PrevRoot   Hash     `json:"prevRoot"` // zero for the first window ever sealed
	MerkleRoot Hash     `json:"merkleRoot"`
	LeafCount  uint64   `json:"leafCount"`
}

// Seal builds the root statement of window w over leaves in insertion order,
// chained to prev (the previous window's [RootStatement.Root], or zero).
func Seal(w WindowID, prev Hash, leaves []Hash) (RootStatement, error) {
	if err := w.Validate(); err != nil {
		return RootStatement{}, err
	}
	return RootStatement{Window: w, PrevRoot: prev, MerkleRoot: MerkleRoot(leaves), LeafCount: uint64(len(leaves))}, nil
}

// Root is the window root: a hash over the window identity, the previous
// root, the Merkle root and the leaf count. The next window commits to it.
func (s RootStatement) Root() Hash {
	w := builder{}
	w.byte1(0x02)
	w.label(labelWindow)
	s.write(&w)
	return w.sum()
}

func (s RootStatement) write(w *builder) {
	w.window(s.Window)
	w.hash(s.PrevRoot)
	w.hash(s.MerkleRoot)
	w.u64(s.LeafCount)
}
