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
	if _, err := Seal(WindowID{}, Hash{}, nil); !errors.Is(err, ErrInvalidWindow) {
		t.Fatalf("Seal: want ErrInvalidWindow, got %v", err)
	}
}

func TestSealRootBindsEverything(t *testing.T) {
	w := WindowFor(testSpace, testProject, t0)
	leaves := leavesN(3)
	s, err := Seal(w, Hash{1}, leaves)
	if err != nil {
		t.Fatal(err)
	}
	if s.LeafCount != 3 || s.MerkleRoot != MerkleRoot(leaves) {
		t.Fatal("seal fields")
	}
	base := s.Root()
	variants := []RootStatement{
		{Window: w.Next(), PrevRoot: s.PrevRoot, MerkleRoot: s.MerkleRoot, LeafCount: 3},
		{Window: WindowID{Space: testSpace, Project: "other", Start: w.Start}, PrevRoot: s.PrevRoot, MerkleRoot: s.MerkleRoot, LeafCount: 3},
		{Window: w, PrevRoot: Hash{2}, MerkleRoot: s.MerkleRoot, LeafCount: 3},
		{Window: w, PrevRoot: s.PrevRoot, MerkleRoot: Hash{}, LeafCount: 3},
		{Window: w, PrevRoot: s.PrevRoot, MerkleRoot: s.MerkleRoot, LeafCount: 4},
	}
	for i, v := range variants {
		if v.Root() == base {
			t.Fatalf("variant %d has the same root", i)
		}
	}
	empty, _ := Seal(w, Hash{}, nil)
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
