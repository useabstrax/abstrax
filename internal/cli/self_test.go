package cli

import "testing"

func TestSelfUpdatePluginsFlag(t *testing.T) {
	cmd := newSelfUpdateCmd()
	flag := cmd.Flags().Lookup("plugins")
	if flag == nil {
		t.Fatal("expected --plugins flag")
	}
	if flag.DefValue != "false" {
		t.Fatalf("default = %q, want false", flag.DefValue)
	}
}
