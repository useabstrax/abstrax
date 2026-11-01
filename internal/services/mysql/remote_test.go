package mysql

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"abstrax/internal/platform"
)

func TestRemoteDropInPathPerProvider(t *testing.T) {
	none := func(string) bool { return false }
	cases := []struct {
		name   string
		p      platform.Provider
		suffix string
	}{
		{"ubuntu mysql", ubuntuMySQLProvider(t), "/mysql.conf.d/" + remoteDropInName},
		{"debian mariadb", platform.DebianDefaults(), "/mariadb.conf.d/" + remoteDropInName},
		{"rhel mariadb", platform.RHELDefaults(), "/my.cnf.d/" + remoteDropInName},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir, err := chooseRemoteConfigDir(tc.p.DatabaseConfigPaths(), preferRemoteConfigDir(tc.p), none)
			if err != nil {
				t.Fatal(err)
			}
			if !pathFromProvider(tc.p, dir) {
				t.Fatalf("dir %q is not one of DatabaseConfigPaths()", dir)
			}
			got := filepath.Join(dir, remoteDropInName)
			if !strings.HasSuffix(got, tc.suffix) {
				t.Fatalf("drop-in path = %q, want suffix %q", got, tc.suffix)
			}
		})
	}
}

func TestChooseRemoteConfigDirPrefersExistingConventional(t *testing.T) {
	ubuntu := ubuntuMySQLProvider(t)
	dir, err := chooseRemoteConfigDir(ubuntu.DatabaseConfigPaths(), preferRemoteConfigDir(ubuntu), func(path string) bool {
		return strings.Contains(path, "conf.d")
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(dir, "mysql.conf.d") {
		t.Fatalf("ubuntu dir = %q", dir)
	}

	debian := platform.DebianDefaults()
	dir, err = chooseRemoteConfigDir(debian.DatabaseConfigPaths(), preferRemoteConfigDir(debian), func(path string) bool {
		return strings.Contains(path, "conf.d")
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(dir, "mariadb.conf.d") {
		t.Fatalf("debian dir = %q", dir)
	}

	rhel := platform.RHELDefaults()
	dir, err = chooseRemoteConfigDir(rhel.DatabaseConfigPaths(), preferRemoteConfigDir(rhel), func(path string) bool {
		return path == "/etc/my.cnf" || path == "/etc/my.cnf.d"
	})
	if err != nil {
		t.Fatal(err)
	}
	if dir != "/etc/my.cnf.d" {
		t.Fatalf("rhel dir = %q", dir)
	}
}

func TestChooseRemoteConfigDirUsesExistingWhenConventionalMissing(t *testing.T) {
	ubuntu := ubuntuMySQLProvider(t)
	dir, err := chooseRemoteConfigDir(ubuntu.DatabaseConfigPaths(), preferRemoteConfigDir(ubuntu), func(path string) bool {
		return strings.HasSuffix(path, "/conf.d")
	})
	if err != nil {
		t.Fatal(err)
	}
	if dir != "/etc/mysql/conf.d" {
		t.Fatalf("dir = %q", dir)
	}
}

func TestRemoteDropInContents(t *testing.T) {
	ubuntu := ubuntuMySQLProvider(t)
	mysqlBody := remoteDropInContents("10.1.2.3", ubuntu.DatabaseEngine())
	if ubuntu.DatabaseEngine() != platform.DatabaseMySQL {
		t.Fatalf("engine = %s", ubuntu.DatabaseEngine())
	}
	if mysqlBody != "[mysqld]\nbind-address = 10.1.2.3\nmysqlx-bind-address = 10.1.2.3\n" {
		t.Fatalf("mysql drop-in:\n%s", mysqlBody)
	}

	for _, p := range []platform.Provider{platform.DebianDefaults(), platform.RHELDefaults()} {
		if p.DatabaseEngine() != platform.DatabaseMariaDB {
			t.Fatalf("engine = %s", p.DatabaseEngine())
		}
		body := remoteDropInContents("0.0.0.0", p.DatabaseEngine())
		if strings.Contains(body, "mysqlx-bind-address") {
			t.Fatalf("MariaDB drop-in should not set mysqlx-bind-address:\n%s", body)
		}
		if body != "[mysqld]\nbind-address = 0.0.0.0\n" {
			t.Fatalf("mariadb drop-in:\n%s", body)
		}
	}
}

func TestValidateBindAddress(t *testing.T) {
	for _, bind := range []string{"0.0.0.0", "127.0.0.1", "::1", "192.0.2.10", "2001:db8::1"} {
		if err := ValidateBindAddress(bind); err != nil {
			t.Errorf("ValidateBindAddress(%q) = %v", bind, err)
		}
	}
	for _, bind := range []string{"", " ", "0.0.0.0 ", "0.0.0.0\n", "\"0.0.0.0\"", "'0.0.0.0'", "0.0.0.0 extra", "not-an-ip"} {
		if err := ValidateBindAddress(bind); err == nil {
			t.Errorf("ValidateBindAddress(%q) succeeded", bind)
		}
	}
}

func TestEnableRemoteDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	restarts := 0
	svc := remoteTestService(dir, func(context.Context, string) error {
		restarts++
		return nil
	}, func(context.Context) error { return nil })
	st, err := svc.EnableRemote(context.Background(), RemoteEnableOptions{Bind: "0.0.0.0", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if restarts != 0 {
		t.Fatalf("restarts = %d", restarts)
	}
	if st.ManagedFile != filepath.Join(dir, remoteDropInName) || st.BindAddress != "0.0.0.0" || !st.Enabled {
		t.Fatalf("status = %+v", st)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("dry-run wrote %d entries", len(entries))
	}
}

func TestEnableRemoteWritesDropInAndRestarts(t *testing.T) {
	dir := t.TempDir()
	restarts := 0
	readies := 0
	svc := remoteTestService(dir, func(_ context.Context, name string) error {
		if name != databaseProvider().DatabaseServiceName() {
			t.Fatalf("service = %q", name)
		}
		restarts++
		return nil
	}, func(context.Context) error {
		readies++
		return nil
	})
	st, err := svc.EnableRemote(context.Background(), RemoteEnableOptions{Bind: "192.0.2.10"})
	if err != nil {
		t.Fatal(err)
	}
	if restarts != 1 || readies != 1 {
		t.Fatalf("restarts=%d readies=%d", restarts, readies)
	}
	data, err := os.ReadFile(filepath.Join(dir, remoteDropInName))
	if err != nil {
		t.Fatal(err)
	}
	want := remoteDropInContents("192.0.2.10", databaseProvider().DatabaseEngine())
	if string(data) != want {
		t.Fatalf("file = %q, want %q", data, want)
	}
	if st.BindAddress != "192.0.2.10" || st.Port != 3306 {
		t.Fatalf("status = %+v", st)
	}
}

func TestEnableRemoteRollbackRemovesDropIn(t *testing.T) {
	t.Run("restart fails", func(t *testing.T) {
		assertEnableRollback(t, func(n int) error {
			if n == 1 {
				return errString("restart failed")
			}
			return nil
		}, func(context.Context) error { return nil })
	})
	t.Run("readiness fails", func(t *testing.T) {
		assertEnableRollback(t, func(int) error { return nil }, func(context.Context) error {
			return errString("not ready")
		})
	})
}

func TestDisableRemoteRollbackRestoresDropIn(t *testing.T) {
	original := "[mysqld]\nbind-address = 0.0.0.0\n"
	t.Run("restart fails", func(t *testing.T) {
		assertDisableRollback(t, original, func(n int) error {
			if n == 1 {
				return errString("restart failed")
			}
			return nil
		}, func(context.Context) error { return nil })
	})
	t.Run("readiness fails", func(t *testing.T) {
		assertDisableRollback(t, original, func(int) error { return nil }, func(context.Context) error {
			return errString("not ready")
		})
	})
}

func TestDisableRemoteDryRunWritesNothing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, remoteDropInName)
	original := []byte("[mysqld]\nbind-address = 0.0.0.0\n")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	restarts := 0
	svc := remoteTestService(dir, func(context.Context, string) error {
		restarts++
		return nil
	}, func(context.Context) error { return nil })
	if _, err := svc.DisableRemote(context.Background(), RemoteDisableOptions{DryRun: true}); err != nil {
		t.Fatal(err)
	}
	if restarts != 0 {
		t.Fatalf("restarts = %d", restarts)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatalf("file changed: %q", got)
	}
}

func TestDisableRemoteRefusesForeignBind(t *testing.T) {
	dir := t.TempDir()
	restarts := 0
	svc := remoteTestService(dir, func(context.Context, string) error {
		restarts++
		return nil
	}, func(context.Context) error { return nil })
	svc.serverUpFn = func(context.Context) (bool, error) { return true, nil }
	svc.queryFn = func(context.Context, string) (string, error) {
		return "bind_address\t0.0.0.0\n", nil
	}
	_, err := svc.DisableRemote(context.Background(), RemoteDisableOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "does not come from") || !strings.Contains(err.Error(), "was not turned off") {
		t.Fatal(err)
	}
	if restarts != 0 {
		t.Fatalf("restarts = %d", restarts)
	}
}

func TestDisableRemoteRefusesMismatchedDropIn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, remoteDropInName)
	original := []byte("[mysqld]\nbind-address = 192.0.2.10\n")
	if err := os.WriteFile(path, original, 0644); err != nil {
		t.Fatal(err)
	}
	restarts := 0
	svc := remoteTestService(dir, func(context.Context, string) error {
		restarts++
		return nil
	}, func(context.Context) error { return nil })
	svc.serverUpFn = func(context.Context) (bool, error) { return true, nil }
	svc.queryFn = func(context.Context, string) (string, error) {
		return "bind_address\t0.0.0.0\n", nil
	}
	_, err := svc.DisableRemote(context.Background(), RemoteDisableOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "does not come from") || !strings.Contains(err.Error(), "was not turned off") {
		t.Fatal(err)
	}
	if restarts != 0 {
		t.Fatalf("restarts = %d", restarts)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(original) {
		t.Fatalf("file changed: %q", got)
	}
}

func TestDisableRemoteRemovesMatchingDropIn(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, remoteDropInName)
	if err := os.WriteFile(path, []byte(remoteDropInContents("0.0.0.0", platform.DatabaseMariaDB)), 0644); err != nil {
		t.Fatal(err)
	}
	calls := 0
	svc := remoteTestService(dir, func(context.Context, string) error { return nil }, func(context.Context) error { return nil })
	svc.serverUpFn = func(context.Context) (bool, error) { return true, nil }
	svc.queryFn = func(context.Context, string) (string, error) {
		calls++
		if calls == 1 {
			return "Variable_name\tValue\nbind_address\t0.0.0.0\n", nil
		}
		return "bind_address\t127.0.0.1\n", nil
	}
	st, err := svc.DisableRemote(context.Background(), RemoteDisableOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if st.Enabled || st.BindAddress != "127.0.0.1" {
		t.Fatalf("status = %+v", st)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("drop-in still present: %v", err)
	}
}

func TestRemoteStatusBindAddress(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		name    string
		output  string
		enabled bool
		bind    string
	}{
		{"loopback", "bind_address\t127.0.0.1\n", false, "127.0.0.1"},
		{"ipv6 loopback", "bind_address\t::1\n", false, "::1"},
		{"all interfaces", "Variable_name\tValue\nbind_address\t0.0.0.0\n", true, "0.0.0.0"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := remoteTestService(dir, nil, nil)
			svc.serverUpFn = func(context.Context) (bool, error) { return true, nil }
			svc.queryFn = func(context.Context, string) (string, error) { return tc.output, nil }
			st, err := svc.RemoteStatus(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if st.Enabled != tc.enabled || st.BindAddress != tc.bind {
				t.Fatalf("status = %+v", st)
			}
			if st.Port != 3306 || st.ManagedFile != filepath.Join(dir, remoteDropInName) {
				t.Fatalf("status = %+v", st)
			}
		})
	}
}

func TestRemoteStatusServerStopped(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, remoteDropInName)
	if err := os.WriteFile(path, []byte("[mysqld]\nbind-address = 192.0.2.8\n"), 0644); err != nil {
		t.Fatal(err)
	}
	svc := remoteTestService(dir, nil, nil)
	svc.serverUpFn = func(context.Context) (bool, error) { return false, nil }
	st, err := svc.RemoteStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if st.Enabled || st.BindAddress != "unknown" {
		t.Fatalf("status = %+v", st)
	}
	if !strings.Contains(st.Note, "unknown") || !strings.Contains(st.Note, "192.0.2.8") {
		t.Fatalf("note = %q", st.Note)
	}
}

func remoteTestService(dir string, restart func(context.Context, string) error, ready func(context.Context) error) *Service {
	return &Service{
		cfg:         &Config{Host: "127.0.0.1", Port: 3306, User: "root"},
		remoteDirFn: func() (string, error) { return dir, nil },
		restartFn:   restart,
		readyFn:     ready,
	}
}

func assertEnableRollback(t *testing.T, restart func(int) error, ready func(context.Context) error) {
	t.Helper()
	dir := t.TempDir()
	restarts := 0
	svc := remoteTestService(dir, func(ctx context.Context, name string) error {
		restarts++
		return restart(restarts)
	}, ready)
	_, err := svc.EnableRemote(context.Background(), RemoteEnableOptions{Bind: "0.0.0.0"})
	if err == nil {
		t.Fatal("expected error")
	}
	if restarts != 2 {
		t.Fatalf("restarts = %d", restarts)
	}
	if _, statErr := os.Stat(filepath.Join(dir, remoteDropInName)); !os.IsNotExist(statErr) {
		t.Fatalf("drop-in still present: %v", statErr)
	}
}

func assertDisableRollback(t *testing.T, original string, restart func(int) error, ready func(context.Context) error) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, remoteDropInName)
	if err := os.WriteFile(path, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}
	restarts := 0
	svc := remoteTestService(dir, func(ctx context.Context, name string) error {
		restarts++
		return restart(restarts)
	}, ready)
	svc.serverUpFn = func(context.Context) (bool, error) { return false, nil }
	_, err := svc.DisableRemote(context.Background(), RemoteDisableOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
	if restarts != 2 {
		t.Fatalf("restarts = %d", restarts)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != original {
		t.Fatalf("restored %q, want %q", got, original)
	}
}

func pathFromProvider(p platform.Provider, dir string) bool {
	for _, path := range p.DatabaseConfigPaths() {
		if path == dir {
			return true
		}
	}
	return false
}

func ubuntuMySQLProvider(t *testing.T) platform.Provider {
	t.Helper()
	p, err := platform.For(&platform.Info{
		Family: "debian",
		Profile: platform.Profile{
			Family:       "debian",
			DistroID:     "ubuntu",
			DistroName:   "Ubuntu 24.04 LTS",
			SupportLevel: platform.SupportOfficial,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return p
}

type errString string

func (e errString) Error() string { return string(e) }
