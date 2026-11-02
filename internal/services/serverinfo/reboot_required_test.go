package serverinfo

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"

	executil "abstrax/internal/exec"
)

const needsRestartingRequired = `Core libraries or services have been updated since boot-up:
  * kernel
  * glibc
  * systemd

Reboot is required to fully utilize these updates.
More information: https://access.redhat.com/solutions/27943
`

const needsRestartingClear = `No core libraries or services have been updated since boot-up.
Reboot should not be necessary.
`

func TestReadDebianRebootRequired(t *testing.T) {
	dir := t.TempDir()
	paths := RebootFiles{
		Required: filepath.Join(dir, "reboot-required"),
		Packages: filepath.Join(dir, "reboot-required.pkgs"),
	}
	if err := os.WriteFile(paths.Required, []byte("*** System restart required ***\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(paths.Packages, []byte("linux-image-5.15.0-91-generic\nlibc6\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadDebianReboot(paths)
	if err != nil {
		t.Fatal(err)
	}
	want := RebootStatus{
		Required: true,
		Packages: []string{"linux-image-5.15.0-91-generic", "libc6"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v", got)
	}
	assertRebootJSON(t, got, `{"required":true,"packages":["linux-image-5.15.0-91-generic","libc6"]}`)
}

func TestReadDebianRebootNotRequired(t *testing.T) {
	dir := t.TempDir()
	got, err := ReadDebianReboot(RebootFiles{
		Required: filepath.Join(dir, "reboot-required"),
		Packages: filepath.Join(dir, "reboot-required.pkgs"),
	})
	if err != nil {
		t.Fatal(err)
	}
	want := RebootStatus{Required: false, Packages: []string{}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v", got)
	}
	assertRebootJSON(t, got, `{"required":false,"packages":[]}`)
}

func TestReadDebianRebootRequiredWithoutPackageList(t *testing.T) {
	dir := t.TempDir()
	required := filepath.Join(dir, "reboot-required")
	if err := os.WriteFile(required, []byte("*** System restart required ***\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadDebianReboot(RebootFiles{
		Required: required,
		Packages: filepath.Join(dir, "missing.pkgs"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !got.Required || len(got.Packages) != 0 {
		t.Fatalf("got %#v", got)
	}
}

func TestReadRHELRebootRequired(t *testing.T) {
	runner := executil.New(false, false)
	runner.SetCommandFunc(func(ctx context.Context, name string, args []string) (executil.Result, bool, error) {
		if name != "needs-restarting" || !reflect.DeepEqual(args, []string{"-r"}) {
			return executil.Result{}, true, fmt.Errorf("unexpected %s %v", name, args)
		}
		return executil.Result{Stdout: needsRestartingRequired, ExitCode: 1}, true, fmt.Errorf("exit status 1")
	})
	got, err := ReadRHELReboot(context.Background(), runner)
	if err != nil {
		t.Fatal(err)
	}
	want := RebootStatus{Required: true, Packages: []string{"kernel", "glibc", "systemd"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v", got)
	}
	assertRebootJSON(t, got, `{"required":true,"packages":["kernel","glibc","systemd"]}`)
}

func TestReadRHELRebootNotRequired(t *testing.T) {
	runner := executil.New(false, false)
	runner.SetCommandFunc(func(ctx context.Context, name string, args []string) (executil.Result, bool, error) {
		return executil.Result{Stdout: needsRestartingClear, ExitCode: 0}, true, nil
	})
	got, err := ReadRHELReboot(context.Background(), runner)
	if err != nil {
		t.Fatal(err)
	}
	want := RebootStatus{Required: false, Packages: []string{}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v", got)
	}
}

func TestReadRHELRebootNeedsRestartingMissing(t *testing.T) {
	runner := executil.New(false, false)
	runner.SetCommandFunc(func(ctx context.Context, name string, args []string) (executil.Result, bool, error) {
		return executil.Result{}, true, &exec.Error{Name: name, Err: exec.ErrNotFound}
	})
	got, err := ReadRHELReboot(context.Background(), runner)
	if err != nil {
		t.Fatal(err)
	}
	want := RebootStatus{
		Required: false,
		Packages: []string{},
		Note:     "needs-restarting is not installed",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v", got)
	}
	assertRebootJSON(t, got, `{"required":false,"packages":[],"note":"needs-restarting is not installed"}`)
}

func TestReadRHELRebootOtherError(t *testing.T) {
	runner := executil.New(false, false)
	runner.SetCommandFunc(func(ctx context.Context, name string, args []string) (executil.Result, bool, error) {
		return executil.Result{Stderr: "failed", ExitCode: 2}, true, fmt.Errorf("exit status 2")
	})
	if _, err := ReadRHELReboot(context.Background(), runner); err == nil {
		t.Fatal("expected error")
	}
}

func assertRebootJSON(t *testing.T, status RebootStatus, want string) {
	t.Helper()
	got, err := json.Marshal(status)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("json = %s", got)
	}
}
