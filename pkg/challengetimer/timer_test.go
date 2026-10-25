package challengetimer

import (
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

type fakeClock struct {
	mu    sync.Mutex
	now   time.Time
	calls int
}

func (c *fakeClock) read() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls++
	return c.now
}

func (c *fakeClock) set(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = time.Unix(0, 0).Add(d)
}

func (c *fakeClock) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.calls
}

func newTestTimer() (*Timer, *fakeClock) {
	c := &fakeClock{}
	c.set(0)
	return New(c.read), c
}

func mustOK(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func wantSnapshot(t *testing.T, timer *Timer, want Snapshot) {
	t.Helper()
	if got := timer.Snapshot(); !reflect.DeepEqual(got, want) {
		t.Fatalf("snapshot = %+v, want %+v", got, want)
	}
}

func TestLifecycle(t *testing.T) {
	timer, clock := newTestTimer()
	clock.set(10 * time.Hour)
	wantSnapshot(t, timer, Snapshot{State: StateReady})
	if clock.callCount() != 0 {
		t.Fatal("ready snapshot observed clock")
	}
	mustOK(t, timer.Start())
	clock.set(10*time.Hour + 1250*time.Millisecond)
	wantSnapshot(t, timer, Snapshot{State: StateRunning, ActiveMillis: 1250})
	clock.set(10*time.Hour + 2*time.Second)
	got, err := timer.Finish()
	mustOK(t, err)
	want := Snapshot{State: StateFinished, ActiveMillis: 2000}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("finish = %+v, want %+v", got, want)
	}
	clock.set(20 * time.Hour)
	wantSnapshot(t, timer, want)
	before := clock.callCount()
	got, err = timer.Finish()
	mustOK(t, err)
	if !reflect.DeepEqual(got, want) || clock.callCount() != before {
		t.Fatalf("repeated finish changed result or read clock: %+v", got)
	}
}

func TestInvalidOperationsDoNotMutate(t *testing.T) {
	for _, state := range []State{StateReady, StateRunning, StatePaused, StateFinished} {
		t.Run(string(state), func(t *testing.T) {
			timer, clock := newTestTimer()
			if state != StateReady {
				mustOK(t, timer.Start())
				mustOK(t, timer.Hint())
				mustOK(t, timer.Retry())
			}
			if state == StatePaused || state == StateFinished {
				clock.set(time.Second)
				mustOK(t, timer.Pause(ReasonManual))
			}
			if state == StateFinished {
				_, err := timer.Finish()
				mustOK(t, err)
			}
			before := timer.Snapshot()
			calls := clock.callCount()
			clock.set(24 * time.Hour)
			ops := []struct {
				name string
				call func() error
				want error
			}{
				{"invalid pause", func() error { return timer.Pause("invalid") }, ErrInvalidReason},
				{"empty pause", func() error { return timer.Pause("") }, ErrInvalidReason},
				{"invalid resume", func() error { return timer.Resume("invalid") }, ErrInvalidReason},
				{"empty resume", func() error { return timer.Resume("") }, ErrInvalidReason},
			}
			if state != StateReady {
				ops = append(ops, struct {
					name string
					call func() error
					want error
				}{"duplicate start", timer.Start, ErrInvalidState})
			}
			if state == StateReady || state == StateFinished {
				for _, op := range []struct {
					name string
					call func() error
				}{
					{"pause", func() error { return timer.Pause(ReasonQuery) }},
					{"resume", func() error { return timer.Resume(ReasonPlayback) }},
					{"hint", timer.Hint}, {"retry", timer.Retry},
				} {
					ops = append(ops, struct {
						name string
						call func() error
						want error
					}{op.name, op.call, ErrInvalidState})
				}
			}
			if state == StateReady {
				got, err := timer.Finish()
				if !errors.Is(err, ErrInvalidState) || !reflect.DeepEqual(got, before) {
					t.Fatalf("finish before start = %+v, %v", got, err)
				}
			}
			for _, op := range ops {
				if err := op.call(); !errors.Is(err, op.want) {
					t.Errorf("%s error = %v, want %v", op.name, err, op.want)
				}
			}
			if clock.callCount() != calls {
				t.Fatal("invalid operation observed clock")
			}
			clock.set(time.Second)
			// Keep the running state's observation at its previous timestamp.
			if state == StateRunning {
				clock.set(0)
			}
			wantSnapshot(t, timer, before)
		})
	}
}

func TestOverlappingPauses(t *testing.T) {
	timer, clock := newTestTimer()
	mustOK(t, timer.Start())
	clock.set(time.Second)
	mustOK(t, timer.Resume(ReasonManual)) // Absent reason while running.
	mustOK(t, timer.Pause(ReasonManual))
	clock.set(2 * time.Second)
	mustOK(t, timer.Pause(ReasonHidden))
	mustOK(t, timer.Pause(ReasonManual)) // Set, not reference counting.
	clock.set(3 * time.Second)
	mustOK(t, timer.Resume(ReasonManual))
	mustOK(t, timer.Resume(ReasonManual)) // Absent while still paused.
	wantSnapshot(t, timer, Snapshot{
		State: StatePaused, ActiveMillis: 1000, PauseReasons: []PauseReason{ReasonHidden},
	})
	clock.set(4 * time.Second)
	mustOK(t, timer.Resume(ReasonHidden))
	clock.set(5 * time.Second)
	wantSnapshot(t, timer, Snapshot{State: StateRunning, ActiveMillis: 2000})
}

func TestQueryAndPlaybackExclusion(t *testing.T) {
	timer, clock := newTestTimer()
	mustOK(t, timer.Start())
	clock.set(time.Second)
	mustOK(t, timer.Pause(ReasonQuery))
	clock.set(3 * time.Second)
	mustOK(t, timer.Pause(ReasonPlayback))
	clock.set(5 * time.Second)
	mustOK(t, timer.Resume(ReasonQuery))
	wantSnapshot(t, timer, Snapshot{
		State: StatePaused, ActiveMillis: 1000, PauseReasons: []PauseReason{ReasonPlayback},
	})
	clock.set(7 * time.Second)
	mustOK(t, timer.Resume(ReasonPlayback))
	clock.set(8 * time.Second)
	got, err := timer.Finish()
	mustOK(t, err)
	if got.ActiveMillis != 2000 || got.State != StateFinished {
		t.Fatalf("finish = %+v", got)
	}
}

func TestCountersAndFinishWhilePaused(t *testing.T) {
	timer, clock := newTestTimer()
	mustOK(t, timer.Start())
	clock.set(time.Second)
	mustOK(t, timer.Retry())
	wantSnapshot(t, timer, Snapshot{State: StateRunning, ActiveMillis: 1000, Retries: 1})
	clock.set(2 * time.Second)
	mustOK(t, timer.Hint())
	mustOK(t, timer.Pause(ReasonQuery))
	clock.set(10 * time.Second)
	mustOK(t, timer.Hint())
	mustOK(t, timer.Retry())
	got, err := timer.Finish()
	mustOK(t, err)
	want := Snapshot{State: StateFinished, ActiveMillis: 2000, Hints: 2, Retries: 2, Assisted: true}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("finish = %+v, want %+v", got, want)
	}
	clock.set(100 * time.Second)
	again, err := timer.Finish()
	mustOK(t, err)
	if !reflect.DeepEqual(again, want) {
		t.Fatalf("repeat finish = %+v", again)
	}
}

func TestResetFromEveryState(t *testing.T) {
	for _, state := range []State{StateReady, StateRunning, StatePaused, StateFinished} {
		t.Run(string(state), func(t *testing.T) {
			timer, clock := newTestTimer()
			clock.set(10 * time.Hour)
			if state != StateReady {
				mustOK(t, timer.Start())
				clock.set(11 * time.Hour)
				mustOK(t, timer.Hint())
				mustOK(t, timer.Retry())
			}
			if state == StatePaused || state == StateFinished {
				mustOK(t, timer.Pause(ReasonHidden))
			}
			if state == StateFinished {
				_, err := timer.Finish()
				mustOK(t, err)
			}
			timer.Reset()
			wantSnapshot(t, timer, Snapshot{State: StateReady})
			clock.set(-time.Second) // A new epoch need not reach the old high water.
			mustOK(t, timer.Start())
			clock.set(0)
			wantSnapshot(t, timer, Snapshot{State: StateRunning, ActiveMillis: 1000})
		})
	}
}

func TestBackwardsClock(t *testing.T) {
	timer, clock := newTestTimer()
	clock.set(100 * time.Millisecond)
	mustOK(t, timer.Start())
	clock.set(200 * time.Millisecond)
	wantSnapshot(t, timer, Snapshot{State: StateRunning, ActiveMillis: 100})
	clock.set(150 * time.Millisecond)
	mustOK(t, timer.Pause(ReasonManual))
	clock.set(180 * time.Millisecond)
	mustOK(t, timer.Resume(ReasonManual))
	clock.set(230 * time.Millisecond)
	wantSnapshot(t, timer, Snapshot{State: StateRunning, ActiveMillis: 130})
	clock.set(240 * time.Millisecond)
	mustOK(t, timer.Pause(ReasonQuery))
	clock.set(300 * time.Millisecond)
	wantSnapshot(t, timer, Snapshot{
		State: StatePaused, ActiveMillis: 140, PauseReasons: []PauseReason{ReasonQuery},
	})
	clock.set(260 * time.Millisecond)
	mustOK(t, timer.Resume(ReasonQuery))
	clock.set(320 * time.Millisecond)
	wantSnapshot(t, timer, Snapshot{State: StateRunning, ActiveMillis: 160})
	clock.set(250 * time.Millisecond)
	got, err := timer.Finish()
	mustOK(t, err)
	if got.ActiveMillis != 160 {
		t.Fatalf("backwards finish lost elapsed: %+v", got)
	}
}

func TestSubmillisecondIntervalsAccumulate(t *testing.T) {
	timer, clock := newTestTimer()
	mustOK(t, timer.Start())
	clock.set(600 * time.Microsecond)
	mustOK(t, timer.Pause(ReasonManual))
	wantSnapshot(t, timer, Snapshot{
		State: StatePaused, PauseReasons: []PauseReason{ReasonManual},
	})
	clock.set(time.Second)
	mustOK(t, timer.Resume(ReasonManual))
	clock.set(time.Second + 300*time.Microsecond)
	wantSnapshot(t, timer, Snapshot{State: StateRunning})
	clock.set(time.Second + 600*time.Microsecond)
	mustOK(t, timer.Pause(ReasonHidden))
	clock.set(2 * time.Second)
	mustOK(t, timer.Resume(ReasonHidden))
	clock.set(2*time.Second + 900*time.Microsecond)
	got, err := timer.Finish()
	mustOK(t, err)
	if got.ActiveMillis != 2 {
		t.Fatalf("fractional intervals did not accumulate: %+v", got)
	}
}

func TestSnapshotReasonsAreSortedAndIndependent(t *testing.T) {
	timer, _ := newTestTimer()
	mustOK(t, timer.Start())
	for _, reason := range []PauseReason{ReasonQuery, ReasonPlayback, ReasonManual, ReasonHidden} {
		mustOK(t, timer.Pause(reason))
	}
	want := Snapshot{State: StatePaused, PauseReasons: []PauseReason{
		ReasonHidden, ReasonManual, ReasonPlayback, ReasonQuery,
	}}
	wantSnapshot(t, timer, want)
	old := timer.Snapshot()
	old.PauseReasons[0] = "corrupt"
	wantSnapshot(t, timer, want)
	mustOK(t, timer.Resume(ReasonManual))
	if old.PauseReasons[1] != ReasonManual || len(old.PauseReasons) != 4 {
		t.Fatalf("timer mutated earlier snapshot: %+v", old)
	}
}

func TestNilClock(t *testing.T) {
	timer := New(nil)
	mustOK(t, timer.Start())
	got, err := timer.Finish()
	mustOK(t, err)
	if got.State != StateFinished || got.ActiveMillis < 0 {
		t.Fatalf("default clock result = %+v", got)
	}
}

func TestConcurrentUse(t *testing.T) {
	timer, clock := newTestTimer()
	mustOK(t, timer.Start())
	const workers = 32
	const iterations = 50
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for range iterations {
				for _, call := range []func() error{
					timer.Hint, timer.Retry,
					func() error { return timer.Pause(ReasonQuery) },
					func() error { return timer.Resume(ReasonQuery) },
				} {
					if err := call(); err != nil {
						t.Errorf("concurrent active operation: %v", err)
					}
				}
				_ = timer.Snapshot()
				clock.set(0)
			}
		})
	}
	wg.Wait()
	want := Snapshot{State: StateRunning, Hints: workers * iterations, Retries: workers * iterations, Assisted: true}
	wantSnapshot(t, timer, want)
	want.State = StateFinished
	for range workers {
		wg.Go(func() {
			got, err := timer.Finish()
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Errorf("concurrent finish = %+v, %v; want %+v", got, err, want)
			}
		})
	}
	wg.Wait()
	// Mix lifecycle operations as well: results depend on linearization, but
	// every operation must remain race-free and return only its documented error.
	for range workers {
		wg.Go(func() {
			for range iterations {
				timer.Reset()
				for _, call := range []func() error{
					timer.Start, timer.Hint, timer.Retry,
					func() error { return timer.Pause(ReasonPlayback) },
					func() error { return timer.Resume(ReasonPlayback) },
				} {
					if err := call(); err != nil && !errors.Is(err, ErrInvalidState) {
						t.Errorf("concurrent mutation error = %v", err)
					}
				}
				_ = timer.Snapshot()
				if _, err := timer.Finish(); err != nil && !errors.Is(err, ErrInvalidState) {
					t.Errorf("concurrent finish error = %v", err)
				}
			}
		})
	}
	wg.Wait()
}
