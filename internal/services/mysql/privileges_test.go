package mysql

import (
	"context"
	"strings"
	"testing"

	"abstrax/internal/validate"
)

func TestPresetAppIncludesReferences(t *testing.T) {
	privs := PresetPrivileges[PresetApp]
	if !strings.Contains(privs, "REFERENCES") {
		t.Fatalf("app preset = %q, want REFERENCES", privs)
	}
}

func TestParsePrivileges(t *testing.T) {
	for _, spec := range PresetPrivileges {
		if _, err := ParsePrivileges(spec); err != nil {
			t.Fatalf("preset %q: %v", spec, err)
		}
	}
	got, err := ParsePrivileges(" select,  Insert ,create temporary tables ")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ", ") != "SELECT, INSERT, CREATE TEMPORARY TABLES" {
		t.Fatalf("parsed = %#v", got)
	}

	rejected := []string{
		`ALL ON *.* TO 'app'@'localhost' WITH GRANT OPTION, SELECT`,
		"SELECT ON shop.* TO 'app'@'localhost'; DROP DATABASE other; GRANT SELECT",
		"SELECT,,INSERT",
		"ALL, SELECT",
		"ALL PRIVILEGES, SELECT",
		"SELECT (email)",
		"GRANT OPTION",
	}
	for _, raw := range rejected {
		if _, err := ParsePrivileges(raw); err == nil {
			t.Fatalf("expected error for %q", raw)
		}
	}
}

func TestMySQLHost(t *testing.T) {
	ok := []string{"localhost", "%", "10.0.0.12", "10.0.%", "::1", "10.0.0.0/255.255.255.0", "app.internal"}
	for _, host := range ok {
		if err := validate.MySQLHost(host); err != nil {
			t.Fatalf("%q: %v", host, err)
		}
	}
	bad := []string{"", "local host", "localhost'; DROP DATABASE shop; --", "app`db", `a\b`, "host;drop"}
	for _, host := range bad {
		if err := validate.MySQLHost(host); err == nil {
			t.Fatalf("expected error for %q", host)
		}
	}
}

func TestGrantAndUserAddSQL(t *testing.T) {
	svc := &Service{captureSQL: true}
	err := svc.Grant(context.Background(), "app", "shop", "10.0.4.12", "select, insert")
	if err != nil {
		t.Fatal(err)
	}
	if len(svc.sqlLog) != 2 || svc.sqlLog[0] != "GRANT SELECT, INSERT ON `shop`.* TO 'app'@'10.0.4.12'" || svc.sqlLog[1] != "FLUSH PRIVILEGES" {
		t.Fatalf("sql = %#v", svc.sqlLog)
	}

	svc.sqlLog = nil
	err = svc.Grant(context.Background(), "app", "shop", "localhost", `ALL ON *.* TO 'app'@'localhost' WITH GRANT OPTION, SELECT`)
	if err == nil {
		t.Fatal("expected privilege error")
	}
	if len(svc.sqlLog) != 0 {
		t.Fatalf("malicious SQL ran: %#v", svc.sqlLog)
	}

	svc.sqlLog = nil
	err = svc.UserAdd(context.Background(), UserAddOptions{
		Name:       "app",
		Host:       "10.0.4.12",
		Password:   "p'w",
		GrantDB:    "shop",
		Privileges: "SELECT",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"CREATE USER IF NOT EXISTS 'app'@'10.0.4.12' IDENTIFIED BY 'p''w'",
		"GRANT SELECT ON `shop`.* TO 'app'@'10.0.4.12'",
		"FLUSH PRIVILEGES",
	}
	if strings.Join(svc.sqlLog, "\n") != strings.Join(want, "\n") {
		t.Fatalf("sql =\n%s", strings.Join(svc.sqlLog, "\n"))
	}

	svc.sqlLog = nil
	err = svc.UserAdd(context.Background(), UserAddOptions{
		Name:    "app",
		Host:    "localhost",
		GrantDB: "shop`; DROP DATABASE x; --",
	})
	if err == nil {
		t.Fatal("expected database name error")
	}
	if len(svc.sqlLog) != 0 {
		t.Fatalf("malicious SQL ran: %#v", svc.sqlLog)
	}
}
