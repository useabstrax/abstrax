package config

import (
	"path/filepath"
	"testing"
)

func TestApprovedRootsSetGet(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	svc := NewWithPath(path)

	if err := svc.Set(keyProjectsApprovedRoots, []string{"/srv/sites", "/srv/www"}); err != nil {
		t.Fatal(err)
	}

	value, err := svc.Get(keyProjectsApprovedRoots)
	if err != nil {
		t.Fatal(err)
	}
	roots := value.([]string)
	if len(roots) != 2 || roots[0] != "/srv/sites" {
		t.Fatalf("roots = %#v", roots)
	}

	approved, err := svc.ApprovedRoots()
	if err != nil {
		t.Fatal(err)
	}
	if len(approved) != 2 {
		t.Fatalf("approved = %#v", approved)
	}
}
