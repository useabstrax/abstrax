package sshcfg

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnableRootLogin(t *testing.T) {
	for _, tt := range []struct {
		name     string
		keysOnly bool
		want     string
	}{
		{name: "default", want: "PermitRootLogin yes\n"},
		{name: "keys only", keysOnly: true, want: "PermitRootLogin prohibit-password\n"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			svc := newDirectiveService(t, dir)
			include := filepath.Join(dir, "sshd_config.d", "*.conf")
			writeFile(t, svc.mainConfigPath, "Include "+include+"\n")

			err := svc.EnableRootLogin(context.Background(), EnableRootLoginOptions{
				KeysOnly: tt.keysOnly,
			})
			if err != nil {
				t.Fatal(err)
			}
			text := readFile(t, svc.managedPath)
			if !strings.Contains(text, tt.want) {
				t.Fatalf("managed =\n%s", text)
			}
			if tt.keysOnly && strings.Contains(text, "PermitRootLogin yes") {
				t.Fatalf("keys-only wrote yes:\n%s", text)
			}
			if !tt.keysOnly && strings.Contains(text, "prohibit-password") {
				t.Fatalf("default wrote prohibit-password:\n%s", text)
			}
			if strings.Contains(readFile(t, svc.mainConfigPath), "PermitRootLogin") {
				t.Fatal("sshd_config was edited even though the drop-in is read first")
			}
		})
	}
}

func TestEnableRootLoginDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	svc := newDirectiveService(t, dir)
	svc.dryRun = true
	svc.reloadFn = func(context.Context) error {
		t.Fatal("dry-run reloaded sshd")
		return nil
	}
	include := filepath.Join(dir, "sshd_config.d", "*.conf")
	mainPath := svc.mainConfigPath
	writeFile(t, mainPath, "Include "+include+"\n")
	before := readFile(t, mainPath)

	out := captureStdout(t, func() {
		err := svc.EnableRootLogin(context.Background(), EnableRootLoginOptions{
			KeysOnly: true,
			DryRun:   true,
		})
		if err != nil {
			t.Errorf("dry-run: %v", err)
		}
	})
	if readFile(t, mainPath) != before {
		t.Fatal("dry-run changed sshd_config")
	}
	if _, err := os.Stat(svc.managedPath); !os.IsNotExist(err) {
		t.Fatal("dry-run created the managed drop-in")
	}
	want := "would set PermitRootLogin prohibit-password in " + svc.managedPath
	if !strings.Contains(out, want) {
		t.Fatalf("stdout missing %q:\n%s", want, out)
	}
}
