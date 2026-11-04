package integrity

import (
	"errors"
	"testing"
	"time"
)

// randomExport seals a random chain and exports a random subset with random
// redactions. It returns the export and whether each window must be reported
// complete.
func randomExport(t *testing.T, f *fixture) (*Export, []sealed, []bool) {
	counts := make([]int, 1+f.rng.IntN(5))
	for i := range counts {
		counts[i] = f.rng.IntN(13)
	}
	ws := f.sealChain(t0.Add(WindowDuration*time.Duration(f.rng.IntN(1000))), Hash{}, counts...)
	exp := fullExport(t, ws)
	complete := make([]bool, len(ws))
	for wi := range exp.Windows {
		ew := &exp.Windows[wi]
		all := f.rng.IntN(3) != 0
		if !all {
			var kept []ExportEvent
			for _, ev := range ew.Events {
				if f.maybe() {
					kept = append(kept, ev)
				}
			}
			ew.Events, ew.Whole = kept, false
		}
		redacted := false
		for ei := range ew.Events {
			if f.rng.IntN(6) == 0 {
				names := FieldNames()
				red, err := ew.Events[ei].Event.Redact(names[f.rng.IntN(len(names))], names[f.rng.IntN(len(names))])
				if err != nil {
					t.Fatal(err)
				}
				ew.Events[ei].Event = red
				redacted = true
			}
		}
		complete[wi] = len(ew.Events) == len(ws[wi].events) && !redacted
	}
	return exp, ws, complete
}

func TestPropertyRandomExportsVerify(t *testing.T) {
	f := newFixture(t, 0xA0D17)
	for iter := range 300 {
		exp, ws, complete := randomExport(t, f)
		rep, err := VerifyExport(exp, f.opts())
		if err != nil {
			t.Fatalf("iter %d: %v", iter, err)
		}
		for i, w := range rep.Windows {
			if w.Complete != complete[i] || w.Root != ws[i].stmt.Root() || len(w.Events) != len(exp.Windows[i].Events) {
				t.Fatalf("iter %d window %d: %+v, want complete=%v", iter, i, w, complete[i])
			}
		}
	}
}

// tamperings mutate an export so it must fail; each returns false when it
// does not apply (e.g. nothing to swap).
var tamperings = map[string]func(f *fixture, exp *Export) bool{
	"flip value byte": func(f *fixture, exp *Export) bool {
		ev := pickEvent(f, exp)
		if ev == nil {
			return false
		}
		for _, i := range f.rng.Perm(len(ev.Event.Fields)) {
			fl := &ev.Event.Fields[i]
			if fl.Revealed() && len(fl.Value) > 0 {
				fl.Value = append([]byte(nil), fl.Value...)
				fl.Value[f.rng.IntN(len(fl.Value))] ^= byte(1 + f.rng.IntN(255))
				return true
			}
		}
		return false
	},
	"flip commitment": func(f *fixture, exp *Export) bool {
		ev := pickEvent(f, exp)
		if ev == nil {
			return false
		}
		ev.Event.Fields[f.rng.IntN(len(ev.Event.Fields))].Commitment[f.rng.IntN(32)] ^= 1
		return true
	},
	"flip proof": func(f *fixture, exp *Export) bool {
		ev := pickEvent(f, exp)
		if ev == nil || len(ev.Proof) == 0 {
			return false
		}
		ev.Proof = append([]Hash(nil), ev.Proof...)
		ev.Proof[f.rng.IntN(len(ev.Proof))][f.rng.IntN(32)] ^= 1
		return true
	},
	"swap event contents": func(f *fixture, exp *Export) bool {
		ew := &exp.Windows[f.rng.IntN(len(exp.Windows))]
		if len(ew.Events) < 2 {
			return false
		}
		i := f.rng.IntN(len(ew.Events) - 1)
		ew.Events[i].Event, ew.Events[i+1].Event = ew.Events[i+1].Event, ew.Events[i].Event
		return true
	},
	"drop event from whole window": func(f *fixture, exp *Export) bool {
		ew := &exp.Windows[f.rng.IntN(len(exp.Windows))]
		if !ew.Whole || len(ew.Events) == 0 {
			return false
		}
		i := f.rng.IntN(len(ew.Events))
		ew.Events = append(ew.Events[:i:i], ew.Events[i+1:]...)
		return true
	},
	"drop window": func(f *fixture, exp *Export) bool {
		i := f.rng.IntN(len(exp.Windows))
		exp.Windows = append(exp.Windows[:i:i], exp.Windows[i+1:]...)
		return true
	},
	"flip signature": func(f *fixture, exp *Export) bool {
		ew := &exp.Windows[f.rng.IntN(len(exp.Windows))]
		r := *ew.Root
		r.Signature = append([]byte(nil), r.Signature...)
		r.Signature[f.rng.IntN(len(r.Signature))] ^= 1
		ew.Root = &r
		return true
	},
	"flip previous root": func(f *fixture, exp *Export) bool {
		if len(exp.Windows) < 2 {
			return false
		}
		ew := &exp.Windows[1+f.rng.IntN(len(exp.Windows)-1)]
		r := *ew.Root
		r.Statement.PrevRoot[0] ^= 1
		ew.Root = &r
		return true
	},
	"shift index": func(f *fixture, exp *Export) bool {
		ev := pickEvent(f, exp)
		if ev == nil {
			return false
		}
		ev.Index++
		return true
	},
}

func pickEvent(f *fixture, exp *Export) *ExportEvent {
	ew := &exp.Windows[f.rng.IntN(len(exp.Windows))]
	if len(ew.Events) == 0 {
		return nil
	}
	return &ew.Events[f.rng.IntN(len(ew.Events))]
}

var tamperSentinels = []error{
	ErrFieldTampered, ErrMalformedEvent, ErrInclusionProof, ErrLeafReordered, ErrLeafIndex, ErrLeafOrder,
	ErrIncompleteWindow, ErrMissingWindow, ErrBadSignature, ErrChainBroken,
}

func TestPropertyRandomTamperingIsDetected(t *testing.T) {
	f := newFixture(t, 0x7A3)
	applied := map[string]int{}
	for iter := range 1200 {
		exp, _, _ := randomExport(t, f)
		names := []string{"flip value byte", "flip commitment", "flip proof", "swap event contents",
			"drop event from whole window", "drop window", "flip signature", "flip previous root", "shift index"}
		name := names[f.rng.IntN(len(names))]
		if !tamperings[name](f, exp) {
			continue
		}
		applied[name]++
		_, err := VerifyExport(exp, f.opts())
		if err == nil {
			t.Fatalf("iter %d: %s not detected", iter, name)
		}
		var ve *VerifyError
		if !errors.As(err, &ve) {
			t.Fatalf("iter %d: %s: %T", iter, name, err)
		}
		known := false
		for _, s := range tamperSentinels {
			known = known || errors.Is(err, s)
		}
		if !known {
			t.Fatalf("iter %d: %s: unexpected error class %v", iter, name, err)
		}
	}
	for name := range tamperings {
		if applied[name] < 15 {
			t.Fatalf("tampering %q applied only %d times", name, applied[name])
		}
	}
}
