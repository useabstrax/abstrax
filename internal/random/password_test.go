package random

import "testing"

func TestPasswordLength(t *testing.T) {
	pw, err := Password()
	if err != nil {
		t.Fatal(err)
	}
	if len(pw) != defaultPasswordLength {
		t.Fatalf("Password() length = %d, want %d", len(pw), defaultPasswordLength)
	}
}

func TestPasswordN(t *testing.T) {
	pw, err := PasswordN(32)
	if err != nil {
		t.Fatal(err)
	}
	if len(pw) != 32 {
		t.Fatalf("PasswordN(32) length = %d, want 32", len(pw))
	}
}

func TestPasswordNInvalid(t *testing.T) {
	if _, err := PasswordN(0); err == nil {
		t.Fatal("expected error for zero length")
	}
}
