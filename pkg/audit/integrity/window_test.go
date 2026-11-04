package integrity

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestWindowFor(t *testing.T) {
	at := time.Date(2026, 11, 3, 21, 9, 59, 999, time.FixedZone("x", 3600))
	w := WindowFor("s1", "", at)
	if !w.Start.Equal(time.Date(2026, 11, 3, 20, 5, 0, 0, time.UTC)) {
		t.Fatalf("start %s", w.Start)
	}
	if err := w.Validate(); err != nil {
		t.Fatal(err)
	}
	if got, want := w.Key(), "space=s1|project=|2026-11-03T20:05"; got != want {
		t.Fatalf("key %q, want %q", got, want)
	}
	odd := WindowID{Space: "a|b", Project: "p=q", Start: w.Start}
	if got, want := odd.Key(), "space=a%7Cb|project=p%3Dq|2026-11-03T20:05"; got != want {
		t.Fatalf("key %q, want %q", got, want)
	}
	if !w.Next().Prev().Equal(w) || w.Next().Equal(w) {
		t.Fatal("Next/Prev")
	}
	if w.Equal(WindowID{Space: "s1", Project: "x", Start: w.Start}) || w.Equal(WindowID{Space: "s2", Start: w.Start}) {
		t.Fatal("Equal must compare space and project")
	}
}

func TestWindowValidate(t *testing.T) {
	cases := []struct {
		name string
		w    WindowID
	}{
		{"no space", WindowID{Start: t0}},
		{"zero start", WindowID{Space: "s"}},
		{"misaligned", WindowID{Space: "s", Start: t0.Add(time.Minute)}},
		{"nanos", WindowID{Space: "s", Start: t0.Add(time.Nanosecond)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.w.Validate(); !errors.Is(err, ErrInvalidWindow) {
				t.Fatalf("want ErrInvalidWindow, got %v", err)
			}
		})
	}
	if _, err := Seal(WindowID{}, Hash{}, nil, t0.Add(2*RetentionFloor)); !errors.Is(err, ErrInvalidWindow) {
		t.Fatalf("Seal: want ErrInvalidWindow, got %v", err)
	}
}

func TestSealRootBindsEverything(t *testing.T) {
	w := WindowFor(testSpace, testProject, t0)
	leaves := leavesN(3)
	s, err := Seal(w, Hash{1}, leaves, t0.Add(2*RetentionFloor))
	if err != nil {
		t.Fatal(err)
	}
	if s.LeafCount != 3 || s.MerkleRoot != MerkleRoot(leaves) {
		t.Fatal("seal fields")
	}
	base := s.Root()
	x := s.ExpiresAt
	variants := []RootStatement{
		{Window: w.Next(), PrevRoot: s.PrevRoot, MerkleRoot: s.MerkleRoot, LeafCount: 3, ExpiresAt: x},
		{Window: WindowID{Space: testSpace, Project: "other", Start: w.Start}, PrevRoot: s.PrevRoot, MerkleRoot: s.MerkleRoot, LeafCount: 3, ExpiresAt: x},
		{Window: w, PrevRoot: Hash{2}, MerkleRoot: s.MerkleRoot, LeafCount: 3, ExpiresAt: x},
		{Window: w, PrevRoot: s.PrevRoot, MerkleRoot: Hash{}, LeafCount: 3, ExpiresAt: x},
		{Window: w, PrevRoot: s.PrevRoot, MerkleRoot: s.MerkleRoot, LeafCount: 4, ExpiresAt: x},
		{Window: w, PrevRoot: s.PrevRoot, MerkleRoot: s.MerkleRoot, LeafCount: 3, ExpiresAt: x.Add(time.Second)},
	}
	for i, v := range variants {
		if v.Root() == base {
			t.Fatalf("variant %d has the same root", i)
		}
	}
	if _, err := Seal(w, Hash{}, nil, time.Time{}); !errors.Is(err, ErrInvalidStatement) {
		t.Fatalf("Seal without expiry: %v", err)
	}
	if _, err := Seal(w, Hash{}, nil, w.End().Add(RetentionFloor-time.Nanosecond)); !errors.Is(err, ErrRetentionTooShort) {
		t.Fatalf("Seal below the retention floor: %v", err)
	}
	if _, err := Seal(w, Hash{}, nil, w.End().Add(RetentionFloor)); err != nil {
		t.Fatalf("Seal at the retention floor: %v", err)
	}
	empty, _ := Seal(w, Hash{}, nil, t0.Add(2*RetentionFloor))
	if empty.LeafCount != 0 || empty.MerkleRoot != MerkleRoot(nil) {
		t.Fatal("empty window seal")
	}
}

func TestHashText(t *testing.T) {
	h := Hash{0xab, 0x01}
	b, err := json.Marshal(h)
	if err != nil {
		t.Fatal(err)
	}
	var back Hash
	if err := json.Unmarshal(b, &back); err != nil || back != h {
		t.Fatalf("json round trip: %v", err)
	}
	if h.IsZero() || !(Hash{}).IsZero() {
		t.Fatal("IsZero")
	}
	for _, bad := range []string{"", "ab", h.String()[:63] + "g", "AB" + h.String()[2:]} {
		if _, err := ParseHash(bad); !errors.Is(err, ErrMalformedHash) {
			t.Fatalf("%q: want ErrMalformedHash, got %v", bad, err)
		}
	}
	if err := back.UnmarshalText([]byte("zz")); !errors.Is(err, ErrMalformedHash) {
		t.Fatalf("UnmarshalText: %v", err)
	}
}

func TestWindowClosedAfterGrace(t *testing.T) {
	w := WindowFor(testSpace, testProject, t0)
	if !w.End().Equal(t0.Add(WindowDuration)) {
		t.Fatal("End")
	}
	lastInstant := w.End().Add(-time.Nanosecond)
	if WindowFor(testSpace, testProject, lastInstant) != w {
		t.Fatal("an event at the last instant belongs to the window")
	}
	for _, tc := range []struct {
		now  time.Time
		want bool
	}{
		{w.Start, false},
		{w.End(), false},
		{w.End().Add(SealGrace - time.Nanosecond), false},
		{w.End().Add(SealGrace), true},
	} {
		if got := w.Closed(tc.now); got != tc.want {
			t.Fatalf("Closed(%s) = %v", tc.now, got)
		}
	}
}
