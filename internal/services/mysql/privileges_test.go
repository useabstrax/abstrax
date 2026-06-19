package mysql

import (
	"strings"
	"testing"
)

func TestPresetAppIncludesReferences(t *testing.T) {
	privs := PresetPrivileges[PresetApp]
	if !strings.Contains(privs, "REFERENCES") {
		t.Fatalf("app preset = %q, want REFERENCES", privs)
	}
}
