package sshcfg

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	executil "abstrax/internal/exec"
)

func TestParseSSHDDump(t *testing.T) {
	cfg := parseSSHDDump("" +
		"port 22\n" +
		"port 2222\n" +
		"permitrootlogin prohibit-password\n" +
		"passwordauthentication no\n" +
		"clientaliveinterval 0\n")
	if cfg.Port != "22, 2222" {
		t.Fatalf("port = %q", cfg.Port)
	}
	if cfg.PermitRootLogin != "prohibit-password" {
		t.Fatalf("permitrootlogin = %q", cfg.PermitRootLogin)
	}
	if cfg.PasswordAuth != "no" || cfg.ClientAliveInterval != "0" {
		t.Fatalf("cfg = %+v", cfg)
	}
}

func TestShowReadsSSHDDump(t *testing.T) {
	dir := t.TempDir()
	svc := newShowService(t, dir)
	writeFile(t, svc.mainConfigPath, "PasswordAuthentication yes\n")
	svc.runner.SetCommandFunc(func(ctx context.Context, name string, args []string) (executil.Result, bool, error) {
		if name == "sshd" && len(args) > 0 && args[0] == "-T" {
			return executil.Result{Stdout: "" +
				"port 22\n" +
				"port 2200\n" +
				"permitrootlogin prohibit-password\n" +
				"passwordauthentication no\n" +
				"clientaliveinterval 0\n", ExitCode: 0}, true, nil
		}
		return executil.Result{}, false, nil
	})

	cfg, err := svc.Show(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != "22, 2200" || cfg.PermitRootLogin != "prohibit-password" || cfg.PasswordAuth != "no" {
		t.Fatalf("cfg = %+v", cfg)
	}
	if cfg.Note != "" {
		t.Fatalf("note = %q", cfg.Note)
	}

	raw := map[string]any{}
	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"port", "permit_root_login", "password_authentication", "client_alive_interval"} {
		if _, ok := raw[key]; !ok {
			t.Fatalf("missing json key %s in %s", key, data)
		}
	}
}

func TestShowDefaultsWhenUnset(t *testing.T) {
	dir := t.TempDir()
	svc := newShowService(t, dir)
	svc.runner.SetCommandFunc(func(ctx context.Context, name string, args []string) (executil.Result, bool, error) {
		return executil.Result{Stdout: "protocol 2\n", ExitCode: 0}, true, nil
	})

	cfg, err := svc.Show(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Port != "22" || cfg.PermitRootLogin != "prohibit-password" || cfg.PasswordAuth != "yes" || cfg.ClientAliveInterval != "0" {
		t.Fatalf("cfg = %+v", cfg)
	}
	if cfg.PortSource != "default" || cfg.PermitRootLoginSource != "default" || cfg.PasswordAuthSource != "default" || cfg.ClientAliveIntervalSource != "default" {
		t.Fatalf("sources = %+v", cfg)
	}
}

func TestShowSources(t *testing.T) {
	dir := t.TempDir()
	svc := newShowService(t, dir)
	include := filepath.Join(dir, "sshd_config.d", "*.conf")
	other := filepath.Join(dir, "sshd_config.d", "50-cloud-init.conf")
	writeFile(t, svc.mainConfigPath, "Include "+include+"\nPort 22\n")
	writeFile(t, svc.managedPath, "PasswordAuthentication no\n")
	writeFile(t, other, "PermitRootLogin prohibit-password\n")
	svc.runner.SetCommandFunc(func(ctx context.Context, name string, args []string) (executil.Result, bool, error) {
		return executil.Result{Stdout: "" +
			"port 22\n" +
			"permitrootlogin prohibit-password\n" +
			"passwordauthentication no\n" +
			"clientaliveinterval 0\n", ExitCode: 0}, true, nil
	})

	cfg, err := svc.Show(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PortSource != svc.mainConfigPath {
		t.Fatalf("port source = %q", cfg.PortSource)
	}
	if cfg.PermitRootLoginSource != other {
		t.Fatalf("permit source = %q", cfg.PermitRootLoginSource)
	}
	if cfg.PasswordAuthSource != "abstrax" {
		t.Fatalf("password source = %q", cfg.PasswordAuthSource)
	}
	if cfg.ClientAliveIntervalSource != "default" {
		t.Fatalf("timeout source = %q", cfg.ClientAliveIntervalSource)
	}
}

func TestShowFallsBackWhenSSHDUnavailable(t *testing.T) {
	dir := t.TempDir()
	svc := newShowService(t, dir)
	writeFile(t, svc.mainConfigPath, "PermitRootLogin yes\nPort 2222\n")
	svc.runner.SetCommandFunc(func(ctx context.Context, name string, args []string) (executil.Result, bool, error) {
		return executil.Result{ExitCode: 1, Stderr: "not available"}, true, nil
	})

	cfg, err := svc.Show(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Note != sshdUnavailableNote {
		t.Fatalf("note = %q", cfg.Note)
	}
	if cfg.PermitRootLogin != "yes" || cfg.Port != "2222" {
		t.Fatalf("cfg = %+v", cfg)
	}
	if cfg.PermitRootLoginSource != svc.mainConfigPath {
		t.Fatalf("permit source = %q", cfg.PermitRootLoginSource)
	}
	if cfg.PasswordAuth != "yes" || cfg.PasswordAuthSource != "default" {
		t.Fatalf("password = %+v", cfg)
	}
	if cfg.ClientAliveInterval != "0" || cfg.ClientAliveIntervalSource != "default" {
		t.Fatalf("timeout = %+v", cfg)
	}
}

func newShowService(t *testing.T, dir string) *Service {
	t.Helper()
	svc := &Service{
		managedPath:    filepath.Join(dir, "sshd_config.d", managedDropInName),
		mainConfigPath: filepath.Join(dir, "sshd_config"),
		runner:         executil.New(false, false),
	}
	return svc
}
