// Package challengetimer measures active solving time, excluding pauses.
package challengetimer

import (
	"errors"
	"sort"
	"sync"
	"time"
)

// PauseReason identifies an independent reason to exclude time.
type PauseReason string

const (
	ReasonManual   PauseReason = "manual"
	ReasonHidden   PauseReason = "hidden"
	ReasonQuery    PauseReason = "query"
	ReasonPlayback PauseReason = "playback"
)

// State describes a timer's lifecycle.
type State string

const (
	StateReady    State = "ready"
	StateRunning  State = "running"
	StatePaused   State = "paused"
	StateFinished State = "finished"
)

var (
	// ErrInvalidState indicates an operation outside its allowed lifecycle.
	ErrInvalidState = errors.New("challengetimer: invalid state")
	// ErrInvalidReason indicates an undefined pause reason.
	ErrInvalidReason = errors.New("challengetimer: invalid pause reason")
)

// Snapshot is an independent view of a timer at one instant.
type Snapshot struct {
	State        State
	ActiveMillis int64
	Hints        int
	Retries      int
	Assisted     bool
	PauseReasons []PauseReason
}

// Timer is safe for concurrent use. Create one with New; do not copy it.
// The supplied clock runs under the timer's lock and must not call its methods.
type Timer struct {
	mu      sync.Mutex
	now     func() time.Time
	state   State
	last    time.Time
	elapsed time.Duration
	reasons map[PauseReason]struct{}
	hints   int
	retries int
}

// New creates a ready timer. A nil clock selects time.Now.
func New(now func() time.Time) *Timer {
	if now == nil {
		now = time.Now
	}
	return &Timer{now: now, state: StateReady}
}

// Start begins a fresh timer's clock epoch.
func (t *Timer) Start() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state != StateReady {
		return ErrInvalidState
	}
	t.last = t.now()
	t.state = StateRunning
	return nil
}

func validReason(reason PauseReason) bool {
	switch reason {
	case ReasonManual, ReasonHidden, ReasonQuery, ReasonPlayback:
		return true
	default:
		return false
	}
}

func (t *Timer) started() bool {
	return t.state == StateRunning || t.state == StatePaused
}

// observe advances the high-water clock and charges only active intervals.
// The caller holds mu and has checked that the timer is started/unfinished.
func (t *Timer) observe() {
	now := t.now()
	if now.Before(t.last) {
		now = t.last
	}
	if t.state == StateRunning {
		t.elapsed += now.Sub(t.last)
	}
	t.last = now
}

// Pause adds a reason to pause. Adding an existing reason is idempotent.
func (t *Timer) Pause(reason PauseReason) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !validReason(reason) {
		return ErrInvalidReason
	}
	if !t.started() {
		return ErrInvalidState
	}
	t.observe()
	if t.reasons == nil {
		t.reasons = make(map[PauseReason]struct{})
	}
	t.reasons[reason] = struct{}{}
	t.state = StatePaused
	return nil
}

// Resume removes a pause reason. Removing an absent reason is idempotent.
func (t *Timer) Resume(reason PauseReason) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !validReason(reason) {
		return ErrInvalidReason
	}
	if !t.started() {
		return ErrInvalidState
	}
	t.observe()
	delete(t.reasons, reason)
	if len(t.reasons) == 0 {
		t.state = StateRunning
	}
	return nil
}

// Hint records assistance without imposing a time penalty.
func (t *Timer) Hint() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.started() {
		return ErrInvalidState
	}
	t.observe()
	t.hints++
	return nil
}

// Retry records a retry without imposing a time penalty.
func (t *Timer) Retry() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.started() {
		return ErrInvalidState
	}
	t.observe()
	t.retries++
	return nil
}

// Snapshot returns the current state and an independently owned sorted slice.
func (t *Timer) Snapshot() Snapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.started() {
		t.observe()
	}
	return t.snapshot()
}

func (t *Timer) snapshot() Snapshot {
	var reasons []PauseReason
	for reason := range t.reasons {
		reasons = append(reasons, reason)
	}
	sort.Slice(reasons, func(i, j int) bool { return reasons[i] < reasons[j] })
	return Snapshot{
		State: t.state, ActiveMillis: t.elapsed.Milliseconds(),
		Hints: t.hints, Retries: t.retries, Assisted: t.hints > 0,
		PauseReasons: reasons,
	}
}

// Finish freezes elapsed time and counters, clearing all pause reasons.
// Repeated calls return the same result without observing the clock.
func (t *Timer) Finish() (Snapshot, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state == StateReady {
		return t.snapshot(), ErrInvalidState
	}
	if t.state != StateFinished {
		t.observe()
		t.state = StateFinished
		t.reasons = nil
	}
	return t.snapshot(), nil
}

// Reset restores the ready state and clears the clock epoch and all counters.
func (t *Timer) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.state = StateReady
	t.last = time.Time{}
	t.elapsed = 0
	t.reasons = nil
	t.hints = 0
	t.retries = 0
}
