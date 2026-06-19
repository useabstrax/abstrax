package mysql

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestEscapeSQLString(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"plain", "plain"},
		{"it's", "it''s"},
		{"a'b'c", "a''b''c"},
	}
	for _, tc := range tests {
		if got := escapeSQLString(tc.in); got != tc.want {
			t.Errorf("escapeSQLString(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestBuildSetRootPasswordSQL(t *testing.T) {
	sql := buildSetRootPasswordSQL("secret'pass")
	if !strings.Contains(sql, "secret''pass") {
		t.Fatalf("expected escaped password in SQL, got: %s", sql)
	}
	for _, fragment := range []string{
		"ALTER USER 'root'@'localhost'",
		"caching_sha2_password",
		"CREATE USER IF NOT EXISTS 'root'@'127.0.0.1'",
		"GRANT ALL PRIVILEGES ON *.* TO 'root'@'127.0.0.1'",
		"FLUSH PRIVILEGES",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("expected %q in SQL, got: %s", fragment, sql)
		}
	}
}

func TestBuildSecureInstallSQL(t *testing.T) {
	sql := buildSecureInstallSQL("pw")
	for _, fragment := range []string{
		"ALTER USER 'root'@'localhost'",
		"CREATE USER IF NOT EXISTS 'root'@'127.0.0.1'",
		"caching_sha2_password",
		"DELETE FROM mysql.user WHERE User=''",
		"DROP DATABASE IF EXISTS test",
		"FLUSH PRIVILEGES",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("expected %q in SQL, got: %s", fragment, sql)
		}
	}
}

func TestCnfQuoteValue(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"plain", `"plain"`},
		{`a"b`, `"a\"b"`},
		{"pass#word", `"pass#word"`},
		{`a\b`, `"a\\b"`},
	}
	for _, tc := range tests {
		if got := cnfQuoteValue(tc.in); got != tc.want {
			t.Errorf("cnfQuoteValue(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestWriteTempMySQLClientCnf(t *testing.T) {
	path, err := writeTempMySQLClientCnf(Config{
		User:     "root",
		Password: "sec#ret",
		Host:     "127.0.0.1",
		Port:     3306,
		Socket:   "/var/run/mysqld/mysqld.sock",
	})
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(path)

	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode: got %o want 0600", info.Mode().Perm())
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, fragment := range []string{
		"[client]",
		"user=root",
		`password="sec#ret"`,
		"host=127.0.0.1",
		"socket=/var/run/mysqld/mysqld.sock",
	} {
		if !strings.Contains(content, fragment) {
			t.Fatalf("expected %q in cnf, got:\n%s", fragment, content)
		}
	}
}

func TestConfiguredClientArgsUsesDefaultsFile(t *testing.T) {
	svc := &Service{
		cfg: &Config{
			Host:     "127.0.0.1",
			Port:     3306,
			User:     "root",
			Password: "secret",
		},
	}

	args, cleanup, err := svc.configuredClientArgs(svc.connectionConfig())
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	if len(args) != 1 || !strings.HasPrefix(args[0], "--defaults-extra-file=") {
		t.Fatalf("expected defaults-extra-file arg, got %v", args)
	}
	for _, arg := range args {
		if strings.HasPrefix(arg, "-p") {
			t.Fatalf("password must not appear on command line, got %v", args)
		}
	}
}

func TestRootPasswordResultJSON(t *testing.T) {
	result := RootPasswordResult{
		RootPassword: "test-pass",
		Generated:    true,
	}
	data, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, `"root_password":"test-pass"`) {
		t.Fatalf("unexpected JSON: %s", s)
	}
	if !strings.Contains(s, `"password_generated":true`) {
		t.Fatalf("unexpected JSON: %s", s)
	}
}
