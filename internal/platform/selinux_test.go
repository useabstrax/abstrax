package platform_test

import (
	"strings"
	"testing"

	"abstrax/internal/platform"
)

func TestSELinuxWarning(t *testing.T) {
	if got := platform.SELinuxWarning(platform.SELinuxDisabled, ""); got != "" {
		t.Fatalf("disabled should not warn: %q", got)
	}
	got := platform.SELinuxWarning(platform.SELinuxEnforcing, "nginx configuration")
	if !strings.Contains(got, "SELinux is enforcing") {
		t.Fatalf("missing enforcing text: %q", got)
	}
	if !strings.Contains(got, "will not disable SELinux") {
		t.Fatalf("missing non-disable guarantee: %q", got)
	}
	if !strings.Contains(got, "nginx configuration") {
		t.Fatalf("missing context: %q", got)
	}
}

func TestUnsupportedErrorMentionsRHEL(t *testing.T) {
	err := &platform.UnsupportedError{Profile: platform.Profile{
		DistroName:   "Fedora Linux 40",
		DistroID:     "fedora",
		Family:       "unknown",
		SupportLevel: platform.SupportUnsupported,
	}}
	msg := err.Error()
	if !strings.Contains(msg, "Rocky Linux 9+") {
		t.Fatalf("error should mention Rocky: %s", msg)
	}
	if !strings.Contains(msg, "AlmaLinux 9+") {
		t.Fatalf("error should mention Alma: %s", msg)
	}
}
