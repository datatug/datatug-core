package main

import (
	"testing"
)

type fakeParser struct {
}

func (p fakeParser) Parse() ([]string, error) {
	return []string{}, nil
}

func TestMainFunc(t *testing.T) {
	t.Run("getParser_no_error", func(t *testing.T) {
		getParser = func() parser {
			return fakeParser{}
		}
		main()
	})
	t.Run("getParser_nil", func(t *testing.T) {
		getParser = func() parser {
			return nil
		}
		defer func() {
			if r := recover(); r == nil {
				t.Fatal("expected panic")
			}
		}()
		main()
	})
}
