package dalgostore

import (
	"context"
	"errors"
	"testing"

	"github.com/dal-go/dalgo/adapters/dalgo2memory"
	"github.com/datatug/datatug-core/pkg/datatug"
)

func TestEnvironmentSummaryFailureReturnsNoPartialWire(t *testing.T) {
	db := dalgo2memory.New(dalgo2memory.FirestoreProfile())
	s := NewProjectStore(db, "p")
	env := &datatug.Environment{}
	env.SetID("dev")
	env.Title = "Dev"
	if err := s.SaveEnvironment(context.Background(), env); err != nil {
		t.Fatal(err)
	}
	old := summarizeStoredEnvironment
	t.Cleanup(func() { summarizeStoredEnvironment = old })
	want := errors.New("summary codec unavailable")
	summarizeStoredEnvironment = func(datatug.Environment) (datatug.EnvironmentSummary, error) {
		return datatug.EnvironmentSummary{}, want
	}
	if out, err := s.LoadEnvironmentSummary(context.Background(), "dev"); out != nil || !errors.Is(err, want) {
		t.Fatal("summary failure exposed partial metadata", out, err)
	}
}
