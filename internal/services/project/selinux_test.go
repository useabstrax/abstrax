package project

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestFContextPattern(t *testing.T) {
	got := fcontextPattern("/home/deploy/almasite")
	want := "/home/deploy/almasite(/.*)?"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestApplyProjectSELinuxLabels(t *testing.T) {
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
		if name == "semanage" && len(args) > 1 && args[1] == "-a" {
			return errAlreadyExists
		}
		return nil
	}

	path := "/home/deploy/almasite"
	note, err := applyProjectSELinuxLabels(path)
	if err != nil {
		t.Fatal(err)
	}
	if note == "" || !strings.Contains(note, "httpd_sys_content_t") {
		t.Fatalf("note = %q", note)
	}

	joined := strings.Join(calls, "\n")
	if !strings.Contains(joined, "semanage fcontext -m -t httpd_sys_content_t "+fcontextPattern(path)) {
		t.Fatalf("missing content fcontext:\n%s", joined)
	}
	if !strings.Contains(joined, "semanage fcontext -m -t httpd_sys_rw_content_t "+fcontextPattern(filepath.Join(path, "storage"))) {
		t.Fatalf("missing writable fcontext:\n%s", joined)
	}
	if !strings.Contains(joined, "restorecon -RF "+path) {
		t.Fatalf("missing restorecon:\n%s", joined)
	}
}

func TestRemoveProjectSELinuxLabels(t *testing.T) {
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
		return nil
	}

	path := "/home/deploy/site"
	if err := removeProjectSELinuxLabels(path); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(calls, "\n")
	if !strings.Contains(joined, "semanage fcontext -d "+fcontextPattern(path)) {
		t.Fatalf("missing delete:\n%s", joined)
	}
	if !strings.Contains(joined, "semanage fcontext -d "+fcontextPattern(filepath.Join(path, "storage"))) {
		t.Fatalf("missing storage delete:\n%s", joined)
	}
}

func TestApplyProjectSELinuxLabelsSkipped(t *testing.T) {
	origEnabled := selinuxEnabledFn
	origRun := runSELinuxCmdFn
	t.Cleanup(func() {
		selinuxEnabledFn = origEnabled
		runSELinuxCmdFn = origRun
	})
	selinuxEnabledFn = func() bool { return false }
	runSELinuxCmdFn = func(string, ...string) error {
		t.Fatal("should not run")
		return nil
	}
	note, err := applyProjectSELinuxLabels("/home/deploy/x")
	if err != nil || note != "" {
		t.Fatalf("note=%q err=%v", note, err)
	}
}

type alreadyExistsError struct{}

func (alreadyExistsError) Error() string { return "already defined" }

var errAlreadyExists error = alreadyExistsError{}
