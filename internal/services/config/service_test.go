package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEffectiveUsesDefaultsWhenFileMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	svc := NewWithPath(path)

	effective, err := svc.Effective()
	if err != nil {
		t.Fatal(err)
	}

	if len(effective.PHP.Extensions) != len(DefaultPHPExtensions) {
		t.Fatalf("extensions len = %d, want %d", len(effective.PHP.Extensions), len(DefaultPHPExtensions))
	}
}

func TestSetAddRemoveReset(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	svc := NewWithPath(path)

	if err := svc.Set(keyPHPExtensions, []string{"mysql", "xml"}); err != nil {
		t.Fatal(err)
	}

	value, err := svc.Get(keyPHPExtensions)
	if err != nil {
		t.Fatal(err)
	}
	exts := value.([]string)
	if len(exts) != 2 || exts[0] != "mysql" {
		t.Fatalf("get = %#v", exts)
	}

	if err := svc.Add(keyPHPExtensions, "curl"); err != nil {
		t.Fatal(err)
	}
	value, err = svc.Get(keyPHPExtensions)
	if err != nil {
		t.Fatal(err)
	}
	exts = value.([]string)
	if len(exts) != 3 {
		t.Fatalf("after add = %#v", exts)
	}

	if err := svc.Remove(keyPHPExtensions, "xml"); err != nil {
		t.Fatal(err)
	}
	value, err = svc.Get(keyPHPExtensions)
	if err != nil {
		t.Fatal(err)
	}
	exts = value.([]string)
	if len(exts) != 2 || exts[1] != "curl" {
		t.Fatalf("after remove = %#v", exts)
	}

	if err := svc.Reset(keyPHPExtensions); err != nil {
		t.Fatal(err)
	}
	value, err = svc.Get(keyPHPExtensions)
	if err != nil {
		t.Fatal(err)
	}
	exts = value.([]string)
	if len(exts) != len(DefaultPHPExtensions) {
		t.Fatalf("after reset = %#v", exts)
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("expected config file to be removed after key reset to defaults")
	}
}

func TestPHPPackages(t *testing.T) {
	pkgs := PHPPackages("8.5", []string{"mysql", "xml"})
	want := []string{"php8.5-fpm", "php8.5-cli", "php8.5-mysql", "php8.5-xml"}
	if len(pkgs) != len(want) {
		t.Fatalf("packages = %#v", pkgs)
	}
	for i, pkg := range want {
		if pkgs[i] != pkg {
			t.Fatalf("pkgs[%d] = %q, want %q", i, pkgs[i], pkg)
		}
	}
}

func TestSetDedupesValues(t *testing.T) {
	dir := t.TempDir()
	svc := NewWithPath(filepath.Join(dir, "config.json"))

	if err := svc.Set(keyPHPExtensions, []string{"mysql", "mysql", "xml"}); err != nil {
		t.Fatal(err)
	}

	value, err := svc.Get(keyPHPExtensions)
	if err != nil {
		t.Fatal(err)
	}
	exts := value.([]string)
	if len(exts) != 2 {
		t.Fatalf("deduped = %#v", exts)
	}
}

func TestParseKeyRejectsUnknown(t *testing.T) {
	if _, err := ParseKey("php.version"); err == nil {
		t.Fatal("expected error for unknown key")
	}
}
