package recordsetcompare

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

func TestArchitectureDoesNotImportSchemaComparator(t *testing.T) {
	packages, err := parser.ParseDir(token.NewFileSet(), ".", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, pkg := range packages {
		for _, file := range pkg.Files {
			ast.Inspect(file, func(node ast.Node) bool {
				importSpec, ok := node.(*ast.ImportSpec)
				if !ok {
					return true
				}
				path, _ := strconv.Unquote(importSpec.Path.Value)
				if strings.HasSuffix(path, "/pkg/comparator") {
					t.Errorf("recordsetcompare must not import %s", path)
				}
				return true
			})
		}
	}
}
