package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMigrateLegacyProjects(t *testing.T) {
	dir := t.TempDir()
	legacyDir := filepath.Join(dir, "legacy")
	newDir := filepath.Join(dir, "projects")

	if err := os.MkdirAll(legacyDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "myapp.json"), []byte(`{"name":"myapp"}`), 0640); err != nil {
		t.Fatal(err)
	}

	if err := migrateProjects(newDir, legacyDir); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(newDir, "myapp.json")); err != nil {
		t.Fatalf("expected migrated project file: %v", err)
	}
	if _, err := os.Stat(filepath.Join(legacyDir, "myapp.json")); !os.IsNotExist(err) {
		t.Fatal("expected legacy project file to be moved")
	}
}
