package core_test

import (
	"os/exec"
	"strings"
	"testing"
)

// TestNoFleetImport fails if any package in this module (transitively) imports
// the github.com/rancher/fleet module. Uses go list so it checks real import
// graphs, not string scanning.
func TestNoFleetImport(t *testing.T) {
	// "go list -deps" lists all transitive dependencies of all packages.
	out, err := exec.Command("go", "list", "-deps", "./...").Output()
	if err != nil {
		t.Fatalf("go list -deps failed: %v", err)
	}
	banned := "github.com/rancher/fleet"
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if strings.HasPrefix(line, banned+"/") || line == banned {
			t.Errorf("found banned import: %s", line)
		}
	}
}
