package cli

import (
	"testing"
)

func TestIsBuiltinCommand(t *testing.T) {
	cases := []struct {
		name   string
		builtin bool
	}{
		{"project", true},
		{"plugin", true},
		{"deploy", false},
		{"example", false},
	}
	for _, tc := range cases {
		if got := isBuiltinCommand(tc.name); got != tc.builtin {
			t.Errorf("isBuiltinCommand(%q) = %v, want %v", tc.name, got, tc.builtin)
		}
	}
}

func TestIsUnknownCommand(t *testing.T) {
	if !isUnknownCommand(errUnknown("unknown command \"deploy\" for \"abstrax\"")) {
		t.Fatal("expected unknown command")
	}
	if isUnknownCommand(errUnknown("some other error")) {
		t.Fatal("did not expect unknown command")
	}
}

type errUnknown string

func (e errUnknown) Error() string { return string(e) }
