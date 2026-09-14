package recordsetcompare

import (
	"os/exec"
	"strings"
	"testing"
)

func TestArchitectureDoesNotImportSchemaComparator(t *testing.T) {
	output, err := exec.Command("go", "list", "-deps", ".").CombinedOutput()
	if err != nil {
		t.Fatalf("go list dependencies: %v\n%s", err, output)
	}
	for _, dependency := range strings.Fields(string(output)) {
		if strings.HasSuffix(dependency, "/pkg/comparator") {
			t.Fatalf("recordsetcompare must not depend on %s", dependency)
		}
	}
}
