package sshcfg

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetPortStopsWhenFirewallFails(t *testing.T) {
	dir := t.TempDir()
	var steps []string
	svc := testSSHService(dir)
	svc.openFirewallFn = func(context.Context, int) error {
		steps = append(steps, "firewall")
		return os.ErrPermission
	}
	svc.labelFn = func(context.Context, int) error {
		steps = append(steps, "selinux")
		return nil
	}
	svc.selinuxEnforcingFn = func() bool { return true }

	_, err := svc.SetPort(context.Background(), SetPortOptions{Port: 2222, AllowFirewall: true})
	if err == nil {
		t.Fatal("expected firewall error")
	}
	if len(steps) != 1 || steps[0] != "firewall" {
		t.Fatalf("steps = %#v", steps)
	}
	if _, statErr := os.Stat(svc.managedPath); !os.IsNotExist(statErr) {
		t.Fatal("config was written after the firewall failed")
	}
}

func TestSetPortKeepsOldPortUntilAsked(t *testing.T) {
	dir := t.TempDir()
	svc := readySSHService(dir)
	writeFile(t, svc.managedPath, "PermitRootLogin no\nPort 22\n")

	result, err := svc.SetPort(context.Background(), SetPortOptions{Port: 2222, AllowFirewall: true})
	if err != nil {
		t.Fatal(err)
	}
	text := readFile(t, svc.managedPath)
	if !strings.Contains(text, "Port 22\n") || !strings.Contains(text, "Port 2222\n") || !strings.Contains(text, "PermitRootLogin no\n") {
		t.Fatalf("config =\n%s", text)
	}
	if !result.OldPortKept {
		t.Fatal("expected the old port to stay open")
	}

	_, err = svc.SetPort(context.Background(), SetPortOptions{Port: 2222, AllowFirewall: true, CloseOldPort: true})
	if err != nil {
		t.Fatal(err)
	}
	text = readFile(t, svc.managedPath)
	if strings.Contains(text, "Port 22\n") || !strings.Contains(text, "Port 2222\n") {
		t.Fatalf("config after close =\n%s", text)
	}
	if len(svc.closed) != 1 || svc.closed[0] != 22 {
		t.Fatalf("closed = %#v", svc.closed)
	}
}

func TestSetPortSocketDropIn(t *testing.T) {
	dir := t.TempDir()
	svc := readySSHService(dir)
	svc.socketActiveFn = func(context.Context) bool { return true }
	var restarted bool
	svc.restartSocketFn = func(context.Context) error {
		restarted = true
		return nil
	}
	writeFile(t, svc.mainConfigPath, "Port 22\n")

	if _, err := svc.SetPort(context.Background(), SetPortOptions{Port: 2222}); err != nil {
		t.Fatal(err)
	}
	text := readFile(t, svc.socketDropIn)
	if !strings.Contains(text, "ListenStream=\nListenStream=22\nListenStream=2222\n") {
		t.Fatalf("drop-in =\n%s", text)
	}
	if !restarted {
		t.Fatal("socket was not restarted")
	}
}

func TestSetPortSELinuxOnlyWhenEnforcing(t *testing.T) {
	dir := t.TempDir()
	svc := readySSHService(dir)
	labeled := false
	svc.labelFn = func(context.Context, int) error {
		labeled = true
		return nil
	}
	svc.selinuxEnforcingFn = func() bool { return false }
	if _, err := svc.SetPort(context.Background(), SetPortOptions{Port: 2222}); err != nil {
		t.Fatal(err)
	}
	if labeled {
		t.Fatal("semanage ran while SELinux was not enforcing")
	}

	svc.selinuxEnforcingFn = func() bool { return true }
	if _, err := svc.SetPort(context.Background(), SetPortOptions{Port: 2222}); err != nil {
		t.Fatal(err)
	}
	if !labeled {
		t.Fatal("semanage was not run")
	}
}

func TestSetPortRollsBackWhenBannerFails(t *testing.T) {
	dir := t.TempDir()
	svc := readySSHService(dir)
	original := "Port 22\n"
	writeFile(t, svc.managedPath, original)
	svc.bannerFn = func(int) error { return os.ErrDeadlineExceeded }

	_, err := svc.SetPort(context.Background(), SetPortOptions{Port: 2222, AllowFirewall: true})
	if err == nil {
		t.Fatal("expected banner error")
	}
	if readFile(t, svc.managedPath) != original {
		t.Fatalf("config was not restored:\n%s", readFile(t, svc.managedPath))
	}
}

type recordingService struct {
	*Service
	closed []int
}

func readySSHService(dir string) *recordingService {
	svc := testSSHService(dir)
	rec := &recordingService{Service: svc}
	svc.openFirewallFn = func(context.Context, int) error { return nil }
	svc.closeFirewallFn = func(_ context.Context, port int) error {
		rec.closed = append(rec.closed, port)
		return nil
	}
	svc.selinuxEnforcingFn = func() bool { return false }
	svc.socketActiveFn = func(context.Context) bool { return false }
	svc.validateFn = func(context.Context) error { return nil }
	svc.reloadFn = func(context.Context) error { return nil }
	svc.bannerFn = func(int) error { return nil }
	return rec
}

func testSSHService(dir string) *Service {
	return &Service{
		managedPath:    filepath.Join(dir, "sshd_config.d", "abstrax.conf"),
		mainConfigPath: filepath.Join(dir, "sshd_config"),
		socketDropIn:   filepath.Join(dir, "ssh.socket.d", "abstrax-port.conf"),
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
