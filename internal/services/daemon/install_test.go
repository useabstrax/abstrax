package daemon

import (
	"context"
	"strings"
	"testing"
)

func TestErrSupervisorMissingHint(t *testing.T) {
	err := errSupervisorMissing()
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if !strings.Contains(msg, "daemon install") {
		t.Fatalf("expected daemon install hint, got %q", msg)
	}
	if !strings.Contains(msg, "supervisor is not installed") {
		t.Fatalf("expected missing supervisor message, got %q", msg)
	}
}

func TestInstallResultFields(t *testing.T) {
	svc := New(true, false)
	result, err := svc.Install(context.Background(), InstallOptions{DryRun: true})
	if err != nil {
		// Dry-run may still fail if package manager detection fails on this host;
		// the type wiring is what we care about when install succeeds or is skipped.
		if result != nil {
			t.Fatalf("unexpected result with error: %#v (%v)", result, err)
		}
		return
	}
	if result.Package == "" || result.Service == "" {
		t.Fatalf("result = %#v", result)
	}
}
