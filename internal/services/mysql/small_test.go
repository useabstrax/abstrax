package mysql

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNeedsSmallServerConfig(t *testing.T) {
	cases := []struct {
		kb   int
		want bool
	}{
		{0, false},
		{-1, false},
		{843 * 1024, true},
		{1024 * 1024, true},
		{1024*1024 + 1, false},
		{2 * 1024 * 1024, false},
	}
	for _, tc := range cases {
		if got := needsSmallServerConfig(tc.kb); got != tc.want {
			t.Fatalf("needsSmallServerConfig(%d) = %v, want %v", tc.kb, got, tc.want)
		}
	}
}

func TestParseMemTotalKB(t *testing.T) {
	kb, err := parseMemTotalKB("MemTotal:        843212 kB\nMemFree:          1234 kB\n")
	if err != nil {
		t.Fatal(err)
	}
	if kb != 843212 {
		t.Fatalf("kb = %d", kb)
	}
	if _, err := parseMemTotalKB("MemFree: 1\n"); err == nil {
		t.Fatal("expected error when MemTotal is missing")
	}
}

func TestEnsureSmallServerConfigWritesDropIn(t *testing.T) {
	dir := t.TempDir()
	svc := &Service{
		memTotalFn:  func() (int, error) { return 843 * 1024, nil },
		remoteDirFn: func() (string, error) { return dir, nil },
	}

	wrote, err := svc.ensureSmallServerConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !wrote {
		t.Fatal("expected small-server config to be written")
	}

	path := filepath.Join(dir, smallDropInName)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, fragment := range []string{
		"[mysqld]",
		"innodb_buffer_pool_size = 64M",
		"performance_schema = OFF",
		"max_connections = 50",
	} {
		if !strings.Contains(body, fragment) {
			t.Fatalf("config missing %q:\n%s", fragment, body)
		}
	}

	wrote, err = svc.ensureSmallServerConfig()
	if err != nil {
		t.Fatal(err)
	}
	if !wrote {
		t.Fatal("expected existing config to still report as in place")
	}
}

func TestEnsureSmallServerConfigSkipsLargerHosts(t *testing.T) {
	dir := t.TempDir()
	svc := &Service{
		memTotalFn:  func() (int, error) { return 2 * 1024 * 1024, nil },
		remoteDirFn: func() (string, error) { return dir, nil },
	}

	wrote, err := svc.ensureSmallServerConfig()
	if err != nil {
		t.Fatal(err)
	}
	if wrote {
		t.Fatal("expected no small-server config on a larger host")
	}
	if _, err := os.Stat(filepath.Join(dir, smallDropInName)); !os.IsNotExist(err) {
		t.Fatalf("drop-in should not exist, err=%v", err)
	}
}
