package selfupdate

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceExecutable(t *testing.T) {
	dir := t.TempDir()

	dest := filepath.Join(dir, "abstrax")
	src := filepath.Join(dir, "abstrax-new")

	if err := os.WriteFile(dest, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := replaceExecutable(src, dest); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("got %q, want %q", data, "new")
	}
}
