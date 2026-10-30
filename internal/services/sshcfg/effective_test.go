package sshcfg

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	executil "abstrax/internal/exec"
	"abstrax/internal/platform/debian"
	"abstrax/internal/platform/rhel"
)

func TestAbstraxSSHConfigSortsBeforeCloudInit(t *testing.T) {
	for _, name := range []string{filepath.Base(debian.AbstraxSSHConfig), filepath.Base(rhel.AbstraxSSHConfig)} {
		if name != managedDropInName {
			t.Fatalf("managed file = %s, want %s", name, managedDropInName)
		}
		if name >= "50-cloud-init.conf" {
			t.Fatalf("%s does not sort before 50-cloud-init.conf", name)
		}
	}
}

func TestDisablePasswordAuthBeatsCloudInit(t *testing.T) {
	dir := t.TempDir()
	svc := newDirectiveService(t, dir)
	var order []string
	svc.validateFn = func(context.Context) error {
		order = append(order, "validate")
		return nil
	}
	svc.reloadFn = func(context.Context) error {
		order = append(order, "reload")
		return nil
	}
	svc.runner.SetCommandFunc(func(ctx context.Context, name string, args []string) (executil.Result, bool, error) {
		if name == "sshd" && len(args) > 0 && args[0] == "-T" {
			order = append(order, "effective")
			return executil.Result{Stdout: dumpEffective(svc), ExitCode: 0}, true, nil
		}
		return executil.Result{ExitCode: 0}, true, nil
	})

	include := filepath.Join(dir, "sshd_config.d", "*.conf")
	cloud := filepath.Join(dir, "sshd_config.d", "50-cloud-init.conf")
	writeFile(t, svc.mainConfigPath, "Include "+include+"\n")
	writeFile(t, cloud, "PasswordAuthentication yes\n")

	if err := svc.DisablePasswordAuth(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if strings.Join(order, ",") != "validate,reload,effective" {
		t.Fatalf("order = %v", order)
	}
	text := readFile(t, svc.managedPath)
	if !strings.Contains(text, "PasswordAuthentication no\n") {
		t.Fatalf("managed =\n%s", text)
	}
	if readFile(t, cloud) != "PasswordAuthentication yes\n" {
		t.Fatalf("cloud-init file changed:\n%s", readFile(t, cloud))
	}
	if strings.Contains(readFile(t, svc.mainConfigPath), "PasswordAuthentication") {
		t.Fatal("sshd_config was edited even though the drop-in is read first")
	}
	if got := svc.effectiveValues()["passwordauthentication"]; got != "no" {
		t.Fatalf("effective password authentication = %q", got)
	}
}

func TestWriteMigratesLegacyFile(t *testing.T) {
	dir := t.TempDir()
	svc := newDirectiveService(t, dir)
	include := filepath.Join(dir, "sshd_config.d", "*.conf")
	legacy := filepath.Join(dir, "sshd_config.d", legacyManagedFile)
	writeFile(t, svc.mainConfigPath, "Include "+include+"\n")
	writeFile(t, legacy, "PermitRootLogin no\nClientAliveInterval 120\n")

	if err := svc.DisablePasswordAuth(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatal("legacy drop-in was not removed")
	}
	text := readFile(t, svc.managedPath)
	for _, want := range []string{"PermitRootLogin no\n", "ClientAliveInterval 120\n", "PasswordAuthentication no\n"} {
		if !strings.Contains(text, want) {
			t.Fatalf("managed file missing %q:\n%s", want, text)
		}
	}
}

func TestWriteUpdatesMainConfigWithoutInclude(t *testing.T) {
	dir := t.TempDir()
	svc := newDirectiveService(t, dir)
	writeFile(t, svc.mainConfigPath, "Port 22\nPasswordAuthentication yes\n")

	if err := svc.DisablePasswordAuth(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	main := readFile(t, svc.mainConfigPath)
	if strings.Contains(main, "PasswordAuthentication yes") || !strings.Contains(main, "PasswordAuthentication no\n") {
		t.Fatalf("main config =\n%s", main)
	}
	if strings.Contains(main, "Include") {
		t.Fatalf("Include was added:\n%s", main)
	}
	if !strings.Contains(readFile(t, svc.managedPath), "PasswordAuthentication no\n") {
		t.Fatal("managed drop-in was not written")
	}
	if got := svc.effectiveValues()["passwordauthentication"]; got != "no" {
		t.Fatalf("effective = %q", got)
	}
}

func TestWriteUpdatesDirectiveBeforeInclude(t *testing.T) {
	dir := t.TempDir()
	svc := newDirectiveService(t, dir)
	include := filepath.Join(dir, "sshd_config.d", "*.conf")
	writeFile(t, svc.mainConfigPath, "PasswordAuthentication yes\nInclude "+include+"\n")
	writeFile(t, filepath.Join(dir, "sshd_config.d", "50-cloud-init.conf"), "PasswordAuthentication no\n")

	if err := svc.DisablePasswordAuth(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	main := readFile(t, svc.mainConfigPath)
	if !strings.HasPrefix(main, "PasswordAuthentication no\n") {
		t.Fatalf("main config =\n%s", main)
	}
	if got := svc.effectiveValues()["passwordauthentication"]; got != "no" {
		t.Fatalf("effective = %q", got)
	}
}

func TestEffectiveMismatchNamesEarlierFile(t *testing.T) {
	dir := t.TempDir()
	svc := newDirectiveService(t, dir)
	include := filepath.Join(dir, "sshd_config.d", "*.conf")
	earlier := filepath.Join(dir, "sshd_config.d", "00-0-first.conf")
	writeFile(t, svc.mainConfigPath, "Include "+include+"\n")
	writeFile(t, earlier, "PasswordAuthentication yes\n")

	err := svc.DisablePasswordAuth(context.Background(), false)
	if err == nil {
		t.Fatal("expected effective-config error")
	}
	msg := err.Error()
	for _, want := range []string{"PasswordAuthentication is still yes", earlier, "sets it first"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("error %q does not contain %q", msg, want)
		}
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	svc := newDirectiveService(t, dir)
	svc.dryRun = true
	svc.reloadFn = func(context.Context) error {
		t.Fatal("dry-run reloaded sshd")
		return nil
	}
	mainPath := svc.mainConfigPath
	legacy := filepath.Join(dir, "sshd_config.d", legacyManagedFile)
	writeFile(t, mainPath, "PasswordAuthentication yes\n")
	writeFile(t, legacy, "ClientAliveInterval 30\n")
	beforeMain := readFile(t, mainPath)
	beforeLegacy := readFile(t, legacy)

	out := captureStdout(t, func() {
		err := svc.DisablePasswordAuth(context.Background(), true)
		if err != nil {
			t.Errorf("dry-run: %v", err)
		}
	})
	if readFile(t, mainPath) != beforeMain || readFile(t, legacy) != beforeLegacy {
		t.Fatal("dry-run changed config files")
	}
	if _, err := os.Stat(svc.managedPath); !os.IsNotExist(err) {
		t.Fatal("dry-run created the managed drop-in")
	}
	for _, want := range []string{
		"would move " + legacy + " to " + svc.managedPath,
		"would set PasswordAuthentication no in " + mainPath,
		"would set PasswordAuthentication no in " + svc.managedPath,
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("stdout missing %q:\n%s", want, out)
		}
	}
}

func TestRewriteGlobalDirectiveSkipsMatchBlock(t *testing.T) {
	in := "PasswordAuthentication yes\nMatch User root\nPasswordAuthentication yes\n"
	got := rewriteGlobalDirective(in, "PasswordAuthentication", "no")
	want := "PasswordAuthentication no\nMatch User root\nPasswordAuthentication yes\n"
	if got != want {
		t.Fatalf("got %q", got)
	}

	inserted := rewriteGlobalDirective("Port 22\nMatch User root\n", "PasswordAuthentication", "no")
	want = "Port 22\nPasswordAuthentication no\nMatch User root\n"
	if inserted != want {
		t.Fatalf("inserted %q", inserted)
	}
}

func TestSetPortMovesLegacyDropIn(t *testing.T) {
	dir := t.TempDir()
	svc := readySSHService(dir)
	legacy := filepath.Join(dir, "sshd_config.d", legacyManagedFile)
	writeFile(t, legacy, "PermitRootLogin no\nPort 22\n")

	if _, err := svc.SetPort(context.Background(), SetPortOptions{Port: 2222}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(legacy); !os.IsNotExist(err) {
		t.Fatal("legacy drop-in was not removed")
	}
	text := readFile(t, svc.managedPath)
	for _, want := range []string{"PermitRootLogin no\n", "Port 22\n", "Port 2222\n"} {
		if !strings.Contains(text, want) {
			t.Fatalf("managed file missing %q:\n%s", want, text)
		}
	}
}

func TestSetPortRestoresLegacyDropIn(t *testing.T) {
	dir := t.TempDir()
	svc := readySSHService(dir)
	legacy := filepath.Join(dir, "sshd_config.d", legacyManagedFile)
	original := "PermitRootLogin no\nPort 22\n"
	writeFile(t, legacy, original)
	svc.bannerFn = func(int) error { return os.ErrDeadlineExceeded }

	if _, err := svc.SetPort(context.Background(), SetPortOptions{Port: 2222}); err == nil {
		t.Fatal("expected banner error")
	}
	if _, err := os.Stat(svc.managedPath); !os.IsNotExist(err) {
		t.Fatalf("managed file left behind:\n%s", readFile(t, svc.managedPath))
	}
	if readFile(t, legacy) != original {
		t.Fatalf("legacy = %q", readFile(t, legacy))
	}
}

func newDirectiveService(t *testing.T, dir string) *Service {
	t.Helper()
	svc := &Service{
		managedPath:    filepath.Join(dir, "sshd_config.d", managedDropInName),
		mainConfigPath: filepath.Join(dir, "sshd_config"),
	}
	svc.runner = executil.New(false, false)
	svc.runner.SetCommandFunc(func(ctx context.Context, name string, args []string) (executil.Result, bool, error) {
		if name == "sshd" && len(args) > 0 && args[0] == "-T" {
			return executil.Result{Stdout: dumpEffective(svc), ExitCode: 0}, true, nil
		}
		return executil.Result{ExitCode: 0}, true, nil
	})
	svc.validateFn = func(context.Context) error { return nil }
	svc.reloadFn = func(context.Context) error { return nil }
	return svc
}

func dumpEffective(svc *Service) string {
	vals := svc.effectiveValues()
	var b strings.Builder
	for _, key := range []string{"port", "permitrootlogin", "passwordauthentication", "clientaliveinterval"} {
		if value, ok := vals[key]; ok {
			b.WriteString(key + " " + value + "\n")
		}
	}
	return b.String()
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()
	_ = w.Close()
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(r); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}
