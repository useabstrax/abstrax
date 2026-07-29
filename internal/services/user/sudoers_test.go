package user

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func requireVisudo(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("visudo"); err != nil {
		t.Skip("visudo not available")
	}
}

func testService(t *testing.T, dryRun bool) *Service {
	t.Helper()
	return &Service{
		runner:     nil, // unused by sudoers helpers
		dryRun:     dryRun,
		sudoersDir: t.TempDir(),
	}
}

func TestSudoersPath(t *testing.T) {
	svc := New(false, false)
	path := svc.sudoersPath("concept")
	if path != "/etc/sudoers.d/abstrax-concept" {
		t.Fatalf("sudoersPath = %q", path)
	}
	base := filepath.Base(path)
	if strings.Contains(base, ".") || strings.HasSuffix(base, "~") {
		t.Fatalf("sudoers basename %q must not contain '.' or end with '~' or sudo will ignore it", base)
	}
}

func TestSudoersPathUsesInjectedDir(t *testing.T) {
	svc := testService(t, false)
	path := svc.sudoersPath("deploy")
	if filepath.Dir(path) != svc.sudoersDir {
		t.Fatalf("path dir = %q, want %q", filepath.Dir(path), svc.sudoersDir)
	}
	if filepath.Base(path) != "abstrax-deploy" {
		t.Fatalf("basename = %q", filepath.Base(path))
	}
}

func TestSudoersContent(t *testing.T) {
	got := sudoersContent("concept")
	want := "# Managed by Abstrax - do not edit manually\nconcept ALL=(ALL) NOPASSWD:ALL\n"
	if got != want {
		t.Fatalf("sudoersContent =\n%q\nwant:\n%q", got, want)
	}
}

func TestWritePasswordlessSudo(t *testing.T) {
	requireVisudo(t)
	svc := testService(t, false)

	if err := svc.writePasswordlessSudo("concept"); err != nil {
		t.Fatal(err)
	}

	path := svc.sudoersPath("concept")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != sudoersContent("concept") {
		t.Fatalf("file content =\n%q\nwant:\n%q", data, sudoersContent("concept"))
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0440 {
		t.Fatalf("mode = %o, want 0440", info.Mode().Perm())
	}

	// Temp files with a '.' in the name must not be left behind.
	entries, err := os.ReadDir(svc.sudoersDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".") {
			t.Fatalf("leftover temp/ignored file in sudoers dir: %s", e.Name())
		}
	}
}

func TestWritePasswordlessSudoDryRun(t *testing.T) {
	svc := testService(t, true)

	if err := svc.writePasswordlessSudo("concept"); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(svc.sudoersPath("concept")); !os.IsNotExist(err) {
		t.Fatalf("dry-run should not create sudoers file, stat err = %v", err)
	}
}

func TestWritePasswordlessSudoIdempotent(t *testing.T) {
	requireVisudo(t)
	svc := testService(t, false)

	if err := svc.writePasswordlessSudo("concept"); err != nil {
		t.Fatal(err)
	}
	if err := svc.writePasswordlessSudo("concept"); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(svc.sudoersPath("concept"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != sudoersContent("concept") {
		t.Fatalf("unexpected content after rewrite:\n%s", data)
	}
}

func TestRemovePasswordlessSudo(t *testing.T) {
	requireVisudo(t)
	svc := testService(t, false)

	if err := svc.writePasswordlessSudo("concept"); err != nil {
		t.Fatal(err)
	}
	if err := svc.removePasswordlessSudo("concept"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(svc.sudoersPath("concept")); !os.IsNotExist(err) {
		t.Fatalf("expected file removed, stat err = %v", err)
	}
}

func TestRemovePasswordlessSudoMissingIsOK(t *testing.T) {
	svc := testService(t, false)
	if err := svc.removePasswordlessSudo("missing"); err != nil {
		t.Fatal(err)
	}
}

func TestRemovePasswordlessSudoDryRun(t *testing.T) {
	requireVisudo(t)
	svc := testService(t, false)
	if err := svc.writePasswordlessSudo("concept"); err != nil {
		t.Fatal(err)
	}

	svc.dryRun = true
	if err := svc.removePasswordlessSudo("concept"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(svc.sudoersPath("concept")); err != nil {
		t.Fatalf("dry-run remove should leave file in place: %v", err)
	}
}

func TestValidateSudoersFileRejectsInvalid(t *testing.T) {
	requireVisudo(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "bad")
	if err := os.WriteFile(path, []byte("this is not valid sudoers syntax!!!\n"), 0440); err != nil {
		t.Fatal(err)
	}
	err := validateSudoersFile(path)
	if err == nil {
		t.Fatal("expected validation error for invalid sudoers")
	}
	if !strings.Contains(err.Error(), "invalid sudoers drop-in") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateSudoersFileAcceptsManagedContent(t *testing.T) {
	requireVisudo(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "good")
	if err := os.WriteFile(path, []byte(sudoersContent("concept")), 0440); err != nil {
		t.Fatal(err)
	}
	if err := validateSudoersFile(path); err != nil {
		t.Fatal(err)
	}
}

func TestGrantSudoDryRunWritesNothing(t *testing.T) {
	svc := New(true, false)
	svc.sudoersDir = t.TempDir()

	if err := svc.GrantSudo(context.Background(), "concept", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(svc.sudoersPath("concept")); !os.IsNotExist(err) {
		t.Fatalf("dry-run GrantSudo should not create sudoers file, stat err = %v", err)
	}
}

func TestRevokeSudoDryRunLeavesDropIn(t *testing.T) {
	requireVisudo(t)
	dir := t.TempDir()

	writer := &Service{dryRun: false, sudoersDir: dir}
	if err := writer.writePasswordlessSudo("concept"); err != nil {
		t.Fatal(err)
	}

	revoker := New(true, false)
	revoker.sudoersDir = dir
	if err := revoker.RevokeSudo(context.Background(), "concept", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "abstrax-concept")); err != nil {
		t.Fatalf("dry-run RevokeSudo should leave drop-in: %v", err)
	}
}
