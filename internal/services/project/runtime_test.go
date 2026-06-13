package project

import "testing"

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
