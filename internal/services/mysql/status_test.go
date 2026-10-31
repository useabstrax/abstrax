package mysql

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"abstrax/internal/platform"
	"abstrax/internal/services/svcmanager"
)

func TestParseServerVersion(t *testing.T) {
	cases := []struct {
		raw     string
		engine  string
		version string
	}{
		{"8.4.3", "mysql", "8.4.3"},
		{"8.0.39-0ubuntu0.24.04.1", "mysql", "8.0.39"},
		{"10.11.6-MariaDB-0+deb12u1", "mariadb", "10.11.6"},
		{"11.4.2-MariaDB", "mariadb", "11.4.2"},
		{"VERSION()\n8.4.3\n", "mysql", "8.4.3"},
		{"mysqld  Ver 8.0.39-0ubuntu0.24.04.1 for Linux on x86_64 ((Ubuntu))", "mysql", "8.0.39"},
		{"mariadbd  Ver 10.11.6-MariaDB-0+deb12u1 for debian-linux-gnu on x86_64", "mariadb", "10.11.6"},
	}
	for _, tc := range cases {
		engine, version, ok := ParseServerVersion(tc.raw)
		if !ok {
			t.Fatalf("ParseServerVersion(%q) not ok", tc.raw)
		}
		if engine != tc.engine || version != tc.version {
			t.Fatalf("ParseServerVersion(%q) = %s %s, want %s %s", tc.raw, engine, version, tc.engine, tc.version)
		}
	}

	if _, _, ok := ParseServerVersion(""); ok {
		t.Fatal("empty version string should not parse")
	}
	if _, _, ok := ParseServerVersion("not a version"); ok {
		t.Fatal("non-version string should not parse")
	}
}

func TestStatusNotInstalled(t *testing.T) {
	svc := &Service{
		packageInstalledFn: func(string) bool { return false },
		daemonBinaryFn: func() (string, error) {
			return "", fmt.Errorf("mysqld or mariadbd not found in PATH")
		},
		serviceStatusFn: func(context.Context, string) (*svcmanager.ServiceStatus, error) {
			t.Fatal("service status should not be queried when nothing is installed")
			return nil, nil
		},
		queryFn: func(context.Context, string) (string, error) {
			t.Fatal("database should not be queried when nothing is installed")
			return "", nil
		},
	}

	st, err := svc.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Installed {
		t.Fatal("expected installed=false")
	}
	wantEngine := string(platform.Resolve().DatabaseEngine())
	if st.Engine != wantEngine {
		t.Fatalf("engine = %q, want platform engine %q", st.Engine, wantEngine)
	}
	if st.Version != nil {
		t.Fatalf("version = %q, want null", *st.Version)
	}

	raw, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["installed"] != false || decoded["engine"] != wantEngine || decoded["version"] != nil {
		t.Fatalf("json = %s", raw)
	}
	for _, key := range []string{"service", "active", "enabled", "port", "socket"} {
		if _, ok := decoded[key]; ok {
			t.Fatalf("json included %s: %s", key, raw)
		}
	}
}

func TestStatusInstalledStoppedUsesBinaryVersion(t *testing.T) {
	svc := &Service{
		cfg:                &Config{Port: 3307, Socket: "/tmp/mysql.sock"},
		packageInstalledFn: func(string) bool { return false },
		daemonBinaryFn: func() (string, error) {
			return "/usr/sbin/mysqld", nil
		},
		daemonVersionFn: func(_ context.Context, binary string) (string, error) {
			if binary != "/usr/sbin/mysqld" {
				t.Fatalf("binary = %q", binary)
			}
			return "mysqld  Ver 8.0.39-0ubuntu0.24.04.1 for Linux on x86_64 ((Ubuntu))", nil
		},
		serviceStatusFn: func(_ context.Context, name string) (*svcmanager.ServiceStatus, error) {
			want := platform.Resolve().DatabaseServiceName()
			if name != want {
				t.Fatalf("service = %q, want %q", name, want)
			}
			return &svcmanager.ServiceStatus{Name: name, Active: "inactive", Sub: "dead", Enabled: "disabled"}, nil
		},
		queryFn: func(context.Context, string) (string, error) {
			t.Fatal("database should not be queried when the service is stopped")
			return "", nil
		},
	}

	st, err := svc.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.Installed {
		t.Fatal("expected installed")
	}
	if st.Engine != "mysql" {
		t.Fatalf("engine = %q", st.Engine)
	}
	if st.Version == nil || *st.Version != "8.0.39" {
		t.Fatalf("version = %v", st.Version)
	}
	if st.Service != platform.Resolve().DatabaseServiceName() {
		t.Fatalf("service = %q", st.Service)
	}
	if st.Active == nil || *st.Active {
		t.Fatalf("active = %v", st.Active)
	}
	if st.Enabled == nil || *st.Enabled {
		t.Fatalf("enabled = %v", st.Enabled)
	}
	if st.Port == nil || *st.Port != 3307 {
		t.Fatalf("port = %v", st.Port)
	}
	if st.Socket != "/tmp/mysql.sock" {
		t.Fatalf("socket = %q", st.Socket)
	}
}

func TestStatusInstalledPackageWithoutBinary(t *testing.T) {
	svc := &Service{
		packageInstalledFn: func(name string) bool { return name == "mysql-server" || name == "mariadb-server" },
		daemonBinaryFn: func() (string, error) {
			return "", fmt.Errorf("mysqld or mariadbd not found in PATH")
		},
		serviceStatusFn: func(_ context.Context, name string) (*svcmanager.ServiceStatus, error) {
			return &svcmanager.ServiceStatus{Name: name, Active: "inactive", Enabled: "enabled"}, nil
		},
	}

	st, err := svc.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.Installed {
		t.Fatal("expected installed from package")
	}
	if st.Version != nil {
		t.Fatalf("version = %q, want null", *st.Version)
	}
	if st.Engine != string(platform.Resolve().DatabaseEngine()) {
		t.Fatalf("engine = %q", st.Engine)
	}
	if st.Active == nil || *st.Active {
		t.Fatalf("active = %v", st.Active)
	}
	if st.Enabled == nil || !*st.Enabled {
		t.Fatalf("enabled = %v", st.Enabled)
	}
}

func TestStatusRunningReadsServer(t *testing.T) {
	svc := &Service{
		cfg:                &Config{Port: 3307, Socket: "/tmp/ignored.sock"},
		packageInstalledFn: func(string) bool { return true },
		daemonBinaryFn: func() (string, error) {
			return "/usr/sbin/mariadbd", nil
		},
		daemonVersionFn: func(context.Context, string) (string, error) {
			t.Fatal("binary version should not be used when the server answers")
			return "", nil
		},
		serviceStatusFn: func(_ context.Context, name string) (*svcmanager.ServiceStatus, error) {
			return &svcmanager.ServiceStatus{Name: name, Active: "active", Sub: "running", Enabled: "enabled"}, nil
		},
		queryFn: func(_ context.Context, sql string) (string, error) {
			switch {
			case strings.Contains(sql, "VERSION()"):
				return "VERSION()\n10.11.6-MariaDB-0+deb12u1\n", nil
			case strings.Contains(sql, "@@port"):
				return "@@port\t@@socket\n3306\t/var/run/mysqld/mysqld.sock\n", nil
			default:
				return "", fmt.Errorf("unexpected query %s", sql)
			}
		},
	}

	st, err := svc.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !st.Installed || st.Engine != "mariadb" || st.Version == nil || *st.Version != "10.11.6" {
		t.Fatalf("status = %+v", st)
	}
	if st.Active == nil || !*st.Active || st.Enabled == nil || !*st.Enabled {
		t.Fatalf("service flags = active %v enabled %v", st.Active, st.Enabled)
	}
	if st.Port == nil || *st.Port != 3306 || st.Socket != "/var/run/mysqld/mysqld.sock" {
		t.Fatalf("endpoint = port %v socket %q", st.Port, st.Socket)
	}

	raw, err := json.Marshal(st)
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{
		`"installed":true`,
		`"engine":"mariadb"`,
		`"version":"10.11.6"`,
		`"active":true`,
		`"enabled":true`,
		`"port":3306`,
		`"/var/run/mysqld/mysqld.sock"`,
	} {
		if !strings.Contains(string(raw), fragment) {
			t.Fatalf("json %s missing %s", raw, fragment)
		}
	}
}

func TestStatusConnectionFailureDoesNotError(t *testing.T) {
	svc := &Service{
		cfg:                &Config{Port: 3307, Socket: "/tmp/mysql.sock"},
		packageInstalledFn: func(string) bool { return true },
		daemonBinaryFn: func() (string, error) {
			return "/usr/sbin/mysqld", nil
		},
		daemonVersionFn: func(context.Context, string) (string, error) {
			return "mysqld  Ver 8.4.3 for Linux on x86_64 (MySQL Community Server - GPL)", nil
		},
		serviceStatusFn: func(_ context.Context, name string) (*svcmanager.ServiceStatus, error) {
			return &svcmanager.ServiceStatus{Name: name, Active: "active", Enabled: "enabled"}, nil
		},
		queryFn: func(context.Context, string) (string, error) {
			return "", fmt.Errorf("mysql connection failed")
		},
	}

	st, err := svc.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Version == nil || *st.Version != "8.4.3" || st.Engine != "mysql" {
		t.Fatalf("status = %+v", st)
	}
	if st.Active == nil || !*st.Active {
		t.Fatalf("active = %v", st.Active)
	}
	if st.Port == nil || *st.Port != 3307 || st.Socket != "/tmp/mysql.sock" {
		t.Fatalf("endpoint = port %v socket %q", st.Port, st.Socket)
	}
}
