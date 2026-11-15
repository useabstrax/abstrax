package plugin

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestPluginFContextPattern(t *testing.T) {
	got := pluginFContextPattern("/usr/local/lib/abstrax/plugins")
	want := "/usr/local/lib/abstrax/plugins(/.*)?"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestEnsurePluginSELinuxLabelsSkippedWhenDisabled(t *testing.T) {
	origEnabled := selinuxEnabledFn
	origRun := runSELinuxCmdFn
	t.Cleanup(func() {
		selinuxEnabledFn = origEnabled
		runSELinuxCmdFn = origRun
	})

	selinuxEnabledFn = func() bool { return false }
	runSELinuxCmdFn = func(string, ...string) error {
		t.Fatal("SELinux command should not run when disabled")
		return nil
	}

	svc := NewWithPaths(testPaths(t.TempDir()), "http://127.0.0.1:1")
	if err := svc.ensurePluginSELinuxLabels(filepath.Join(svc.Paths().InstallDir, "abstrax-x")); err != nil {
		t.Fatal(err)
	}
}

func TestEnsurePluginSELinuxLabelsSetsFContextAndRestorecon(t *testing.T) {
	origEnabled := selinuxEnabledFn
	origRun := runSELinuxCmdFn
	t.Cleanup(func() {
		selinuxEnabledFn = origEnabled
		runSELinuxCmdFn = origRun
	})

	selinuxEnabledFn = func() bool { return true }
	var calls []string
	runSELinuxCmdFn = func(name string, args ...string) error {
		calls = append(calls, name+" "+strings.Join(args, " "))
		if name == "semanage" && len(args) > 0 && args[1] == "-a" {
			return fmtError("already exists")
		}
		return nil
	}

	dir := t.TempDir()
	svc := NewWithPaths(testPaths(dir), "http://127.0.0.1:1")
	binary := filepath.Join(svc.Paths().InstallDir, "abstrax-monitoring")
	if err := svc.ensurePluginSELinuxLabels(binary); err != nil {
		t.Fatal(err)
	}

	joined := strings.Join(calls, "\n")
	pattern := pluginFContextPattern(svc.Paths().InstallDir)
	if !strings.Contains(joined, "semanage fcontext -a -t bin_t "+pattern) {
		t.Fatalf("missing -a call:\n%s", joined)
	}
	if !strings.Contains(joined, "semanage fcontext -m -t bin_t "+pattern) {
		t.Fatalf("missing -m fallback:\n%s", joined)
	}
	if !strings.Contains(joined, "restorecon -F "+binary) {
		t.Fatalf("missing binary restorecon:\n%s", joined)
	}
	if !strings.Contains(joined, "restorecon -RF "+filepath.Clean(svc.Paths().InstallDir)) {
		t.Fatalf("missing dir restorecon:\n%s", joined)
	}
}

type simpleError string

func (e simpleError) Error() string { return string(e) }

func fmtError(msg string) error { return simpleError(msg) }
