package storage

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestJsonFileName(t *testing.T) {
	tests := []struct {
		id     string
		suffix string
		want   string
	}{
		{"test", "", "test.json"},
		{"test", BoardFileSuffix, "test.board.json"},
		{"test", DbCatalogFileSuffix, "test.db.json"},
		{"test", DbCatalogObjectFileSuffix, "test.objects.json"},
		{"test", DbCatalogRefsFileSuffix, "test.refs.json"},
		{"test", DbModelFileSuffix, "test.dbmodel.json"},
		{"test", DbServerFileSuffix, "test.dbserver.json"},
		{"test", RecordsetFileSuffix, "test.recordset.json"},
		{"test", EntityFileSuffix, "test.entity.json"},
		{"test", ServerFileSuffix, "test.server.json"},
		{"test", ColumnsFileSuffix, "test.columns.json"},
		{"test", QueryFileSuffix, "test.query.json"},
	}
	for _, tt := range tests {
		t.Run(tt.id+"_"+tt.suffix, func(t *testing.T) {
			assert.Equal(t, tt.want, JsonFileName(tt.id, tt.suffix))
		})
	}

	assert.Panics(t, func() {
		JsonFileName("test", "unknown")
	})
}

func TestGetProjItemIDFromFileName(t *testing.T) {
	tests := []struct {
		fileName string
		wantId   string
		wantSfx  string
	}{
		{"test.board.json", "test", "board"},
		{"test.db.json", "test", "db"},
		{"my.test.query.json", "my.test", "query"},
		{"short.json", "", ""},
		{"nojson", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.fileName, func(t *testing.T) {
			id, sfx := GetProjItemIDFromFileName(tt.fileName)
			assert.Equal(t, tt.wantId, id)
			assert.Equal(t, tt.wantSfx, sfx)
		})
	}
}
