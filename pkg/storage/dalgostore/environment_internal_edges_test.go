package dalgostore

import (
	"errors"
	"github.com/datatug/datatug-core/pkg/datatug"
	"testing"
)

func TestEncodeTypedNilEnvironment(t *testing.T) {
	var environment *datatug.Environment
	if body, err := encodeAsset(environment); !errors.Is(err, ErrAssetInvalid) || body != nil {
		t.Fatalf("typed nil encoded: %s %v", body, err)
	}
}
