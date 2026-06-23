package cli

import (
	"testing"

	"abstrax/internal/globals"
)

func TestSkipConfirm(t *testing.T) {
	globals.Flags.Yes = false
	if skipConfirm(false) {
		t.Fatal("expected confirmation")
	}
	if !skipConfirm(true) {
		t.Fatal("expected skip with force")
	}
	globals.Flags.Yes = true
	if !skipConfirm(false) {
		t.Fatal("expected skip with global --yes")
	}
	globals.Flags.Yes = false
}
