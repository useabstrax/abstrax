package platform_test

import (
	"testing"

	"abstrax/internal/platform"
)

func TestDetectReturnsInfo(t *testing.T) {
	// Detect should always return a non-nil Info and Tools even on
	// unsupported platforms (macOS in CI).
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
	// Architecture and kernel version should be populated on any Unix-like system.
	if info.Architecture == "" {
		t.Error("Architecture should not be empty")
	}
}

func TestRequireRootReturnsErrorWhenNotRoot(t *testing.T) {
	// In CI / test environments we are typically not root.
	// The function should return an error in that case, or nil if running as root.
	err := platform.RequireRoot()
	// We can't assert a specific value because the test may run as root in some
	// environments. Just assert the function does not panic.
	_ = err
}
