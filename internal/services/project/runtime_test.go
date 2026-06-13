package project

import (
	"testing"

	"abstrax/internal/services/config"
)

func TestNodeMajor(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"24", "24"},
		{"24.1.0", "24"},
		{"v24.1.0", "24"},
	}
	for _, tc := range tests {
		if got := nodeMajor(tc.in); got != tc.want {
			t.Fatalf("nodeMajor(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRubyMajorMinor(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"4.0", "4.0"},
		{"4.0.1", "4.0"},
		{"3", "3"},
	}
	for _, tc := range tests {
		if got := rubyMajorMinor(tc.in); got != tc.want {
			t.Fatalf("rubyMajorMinor(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestRuntimeSpecLabel(t *testing.T) {
	spec := RuntimeSpec{Runtime: RuntimePHP, Version: "8.5"}
	if got := spec.label(); got != "PHP 8.5" {
		t.Fatalf("label() = %q, want %q", got, "PHP 8.5")
	}
}

func TestRuntimeSpecFromAddDefaults(t *testing.T) {
	spec := runtimeSpecFromAdd(AddOptions{Runtime: RuntimeNode})
	if spec.Version != DefaultNodeVersion {
		t.Fatalf("version = %q, want %q", spec.Version, DefaultNodeVersion)
	}
}

func TestPHPPackagesFromConfig(t *testing.T) {
	pkgs := config.PHPPackages("8.5", config.DefaultPHPExtensions)
	if len(pkgs) < 3 {
		t.Fatalf("expected fpm, cli, and extensions, got %#v", pkgs)
	}
	if pkgs[0] != "php8.5-fpm" || pkgs[1] != "php8.5-cli" {
		t.Fatalf("base packages = %#v", pkgs[:2])
	}
}
