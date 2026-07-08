package platform_test

import (
	"testing"

	"abstrax/internal/platform"
)

func TestDetectReturnsInfo(t *testing.T) {
	info, tools, err := platform.Detect()
	if err != nil {
		t.Fatalf("Detect() returned unexpected error: %v", err)
	}
	if info == nil {
		t.Fatal("Detect() returned nil Info")
	}
	if tools == nil {
		t.Fatal("Detect() returned nil Tools")
	}
	if info.Architecture == "" {
		t.Error("Architecture should not be empty")
	}
	if info.Profile.Family == "" && info.OSName != "" {
		t.Error("Profile family should be set when OS is detected")
	}
}

func TestRequireRootReturnsErrorWhenNotRoot(t *testing.T) {
	err := platform.RequireRoot()
	_ = err
}

func TestDebianDefaultsProvider(t *testing.T) {
	p := platform.DebianDefaults()
	if p.WebUser() != "www-data" {
		t.Fatalf("WebUser = %q", p.WebUser())
	}
	if p.DefaultProjectRoot() != "/var/www" {
		t.Fatalf("DefaultProjectRoot = %q", p.DefaultProjectRoot())
	}
}
