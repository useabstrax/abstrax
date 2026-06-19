package mysql

import (
	"encoding/json"
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
