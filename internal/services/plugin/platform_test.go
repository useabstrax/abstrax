package plugin

import (
	"runtime"
	"testing"
)

func TestCurrentPlatform(t *testing.T) {
	platform, err := CurrentPlatform()
	if runtime.GOOS != "linux" {
		if err == nil {
			t.Fatal("expected error on non-linux")
		}
		return
	}
	switch runtime.GOARCH {
	case "amd64":
		if platform != "linux-amd64" {
			t.Fatalf("got %q", platform)
		}
	case "arm64":
		if platform != "linux-arm64" {
			t.Fatalf("got %q", platform)
		}
	}
}

func TestInstallFromRegistryLinux(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("install integration requires linux")
	}
	// Covered by TestRegistryInstallAndRemove on Linux CI.
}
