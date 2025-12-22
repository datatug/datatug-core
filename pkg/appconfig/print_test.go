package appconfig

import (
	"bytes"
	"testing"
)

func TestPrintSettings(t *testing.T) {
	settings := Settings{
		Projects: []*ProjectConfig{
			{ID: "p1", Path: "/path/1"},
		},
	}

	t.Run("yaml", func(t *testing.T) {
		buf := new(bytes.Buffer)
		err := PrintSettings(settings, FormatYaml, buf)
		if err != nil {
			t.Fatalf("failed to print settings: %v", err)
		}
		if buf.Len() == 0 {
			t.Error("expected non-empty output")
		}
	})

	t.Run("unsupported", func(t *testing.T) {
		buf := new(bytes.Buffer)
		err := PrintSettings(settings, "json", buf)
		if err == nil {
			t.Error("expected error for unsupported format")
		}
	})
}

type mockSection struct {
	empty bool
}

func (m mockSection) IsEmpty() bool {
	return m.empty
}

func TestPrintSection(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		buf := new(bytes.Buffer)
		section := mockSection{empty: true}
		err := PrintSection(section, FormatYaml, buf)
		if err != nil {
			t.Fatalf("failed to print section: %v", err)
		}
		if buf.Len() != 0 {
			t.Error("expected empty output for empty section")
		}
	})

	t.Run("not_empty_yaml", func(t *testing.T) {
		buf := new(bytes.Buffer)
		section := mockSection{empty: false}
		err := PrintSection(section, FormatYaml, buf)
		if err != nil {
			t.Fatalf("failed to print section: %v", err)
		}
		if buf.Len() == 0 {
			t.Error("expected non-empty output for non-empty section")
		}
	})

	t.Run("unsupported", func(t *testing.T) {
		buf := new(bytes.Buffer)
		section := mockSection{empty: false}
		err := PrintSection(section, "json", buf)
		if err == nil {
			t.Error("expected error for unsupported format")
		}
	})
}
