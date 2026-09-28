package datatug2md

import (
	"errors"
	"io"
	"testing"
	"testing/fstest"
)

type failingWriter struct{}

func (f *failingWriter) Write(p []byte) (n int, err error) {
	return 0, errors.New("write error")
}

func TestWriteReadme_Errors(t *testing.T) {
	t.Run("parse_error", func(t *testing.T) {
		oldFS := templatesFS
		defer func() { templatesFS = oldFS }()
		templatesFS = fstest.MapFS{
			"templates/bad.md": &fstest.MapFile{Data: []byte("{{bad template")},
		}
		err := writeReadme(io.Discard, "test", nil)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("execute_error", func(t *testing.T) {
		err := writeReadme(&failingWriter{}, "test", map[string]interface{}{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}
