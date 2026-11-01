package mysql

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	executil "abstrax/internal/exec"
)

func TestUserPasswordSQLMySQLAndMariaDB(t *testing.T) {
	cases := []struct {
		engine   string
		password string
		want     string
	}{
		{"8.0.40", "simple", "ALTER USER 'app'@'10.0.4.12' IDENTIFIED BY 'simple'"},
		{"8.0.40", `p'w`, "ALTER USER 'app'@'10.0.4.12' IDENTIFIED BY 'p''w'"},
		{"8.0.40", `a\b`, `ALTER USER 'app'@'10.0.4.12' IDENTIFIED BY 'a\\b'`},
		{"10.11.6-MariaDB", `p'a\s`, `ALTER USER 'app'@'10.0.4.12' IDENTIFIED BY 'p''a\\s'`},
	}
	for _, tc := range cases {
		t.Run(tc.engine+"/"+tc.password, func(t *testing.T) {
			svc := passwordService(tc.engine, "10.0.4.12")
			result, err := svc.UserPassword(context.Background(), UserPasswordOptions{
				Name:     "app",
				Host:     "10.0.4.12",
				Password: tc.password,
			})
			if err != nil {
				t.Fatal(err)
			}
			if result.Generated {
				t.Fatal("provided password was marked generated")
			}
			if result.Host != "10.0.4.12" || result.Hosts != nil {
				t.Fatalf("result = %#v", result)
			}
			sql := strings.Join(svc.sqlLog, "\n")
			if sql != tc.want {
				t.Fatalf("sql = %q, want %q", sql, tc.want)
			}
			for _, banned := range []string{"IDENTIFIED WITH", "IDENTIFIED VIA", "caching_sha2_password", "mysql_native_password"} {
				if strings.Contains(sql, banned) {
					t.Fatalf("%s changed the auth plugin: %s", tc.engine, sql)
				}
			}
		})
	}

	t.Run("generated", func(t *testing.T) {
		svc := passwordService("8.0.40", "10.0.4.12")
		result, err := svc.UserPassword(context.Background(), UserPasswordOptions{
			Name: "app",
			Host: "10.0.4.12",
		})
		if err != nil {
			t.Fatal(err)
		}
		if !result.Generated || result.Password == "" {
			t.Fatalf("result = %#v", result)
		}
		want := "ALTER USER 'app'@'10.0.4.12' IDENTIFIED BY '" + escapeSQLString(result.Password) + "'"
		if strings.Join(svc.sqlLog, "\n") != want {
			t.Fatalf("sql = %#v", svc.sqlLog)
		}
	})
}

func TestUserPasswordHostResolution(t *testing.T) {
	const listed = "10.0.4.12\n127.0.0.1\nlocalhost"
	cases := []struct {
		host string
		want []string
		json string
	}{
		{
			host: "",
			want: []string{"10.0.4.12", "127.0.0.1", "localhost"},
			json: `{"name":"app","hosts":["10.0.4.12","127.0.0.1","localhost"],"password":"pw","password_generated":false}`,
		},
		{
			host: "localhost",
			want: []string{"localhost", "127.0.0.1"},
			json: `{"name":"app","hosts":["localhost","127.0.0.1"],"password":"pw","password_generated":false}`,
		},
		{
			host: "10.0.4.12",
			want: []string{"10.0.4.12"},
			json: `{"name":"app","host":"10.0.4.12","password":"pw","password_generated":false}`,
		},
	}
	for _, tc := range cases {
		name := tc.host
		if name == "" {
			name = "all"
		}
		t.Run(name, func(t *testing.T) {
			svc := passwordService("8.0.40", listed)
			result, err := svc.UserPassword(context.Background(), UserPasswordOptions{
				Name:     "app",
				Host:     tc.host,
				Password: "pw",
			})
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, sql := range svc.sqlLog {
				if !strings.HasPrefix(sql, "ALTER USER ") {
					t.Fatalf("unexpected statement %q", sql)
				}
				got = append(got, accountHost(t, sql))
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("hosts = %#v, want %#v\nsql = %#v", got, tc.want, svc.sqlLog)
			}
			data, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != tc.json {
				t.Fatalf("json = %s", data)
			}
		})
	}
}

func TestUserPasswordUserNotFound(t *testing.T) {
	cases := []struct {
		name  string
		hosts string
		host  string
	}{
		{"no accounts", "", ""},
		{"localhost missing", "10.0.4.12", "localhost"},
		{"specific host missing", "localhost\n127.0.0.1", "10.0.4.12"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := passwordService("8.0.40", tc.hosts)
			_, err := svc.UserPassword(context.Background(), UserPasswordOptions{
				Name:     "app",
				Host:     tc.host,
				Password: "pw",
			})
			if err == nil {
				t.Fatal("expected error")
			}
			for _, sql := range svc.sqlLog {
				if strings.Contains(sql, "ALTER USER") {
					t.Fatalf("ran SQL despite missing user: %#v", svc.sqlLog)
				}
			}
			if len(svc.sqlLog) != 0 {
				t.Fatalf("sql = %#v", svc.sqlLog)
			}
		})
	}
}

func TestUserPasswordDryRunRedactsPassword(t *testing.T) {
	const password = `s3cret'\pw`
	svc := &Service{
		runner:  executil.New(true, true),
		dryRun:  true,
		verbose: true,
		cfg:     &Config{Host: "127.0.0.1", Port: 3306, User: "root"},
		queryFn: hostQuery("8.0.40", "localhost"),
	}
	var stdout string
	result, err := func() (*UserPasswordResult, error) {
		var runErr error
		var res *UserPasswordResult
		stdout = captureStdout(t, func() {
			res, runErr = svc.UserPassword(context.Background(), UserPasswordOptions{
				Name:     "app",
				Host:     "localhost",
				Password: password,
				DryRun:   true,
			})
		})
		return res, runErr
	}()
	if err != nil {
		t.Fatal(err)
	}
	if result.Password != password {
		t.Fatalf("result password = %q", result.Password)
	}
	if !strings.Contains(stdout, "ALTER USER 'app'@'localhost' IDENTIFIED BY '[redacted]'") {
		t.Fatalf("stdout = %q", stdout)
	}
	if strings.Contains(stdout, "s3cret") || strings.Contains(stdout, `s3cret\\''pw`) || strings.Contains(stdout, escapeSQLString(password)) {
		t.Fatalf("dry-run leaked the password: %q", stdout)
	}
	for _, cmd := range svc.runner.Commands() {
		joined := strings.Join(cmd, " ")
		if strings.Contains(joined, "s3cret") || strings.Contains(joined, escapeSQLString(password)) {
			t.Fatalf("logged command leaked the password: %q", joined)
		}
	}
}

func TestUserRemoveDropsLocalHosts(t *testing.T) {
	svc := passwordService("8.0.40", "10.0.4.12\n127.0.0.1\nlocalhost")
	if err := svc.UserRemove(context.Background(), "app", "localhost"); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"DROP USER IF EXISTS 'app'@'localhost'",
		"DROP USER IF EXISTS 'app'@'127.0.0.1'",
	}
	if strings.Join(svc.sqlLog, "\n") != strings.Join(want, "\n") {
		t.Fatalf("sql = %#v", svc.sqlLog)
	}

	svc.sqlLog = nil
	if err := svc.UserRemove(context.Background(), "app", "10.0.4.12"); err != nil {
		t.Fatal(err)
	}
	if strings.Join(svc.sqlLog, "\n") != "DROP USER IF EXISTS 'app'@'10.0.4.12'" {
		t.Fatalf("specific host sql = %#v", svc.sqlLog)
	}

	svc.sqlLog = nil
	if err := svc.UserRemove(context.Background(), "app", ""); err != nil {
		t.Fatal(err)
	}
	all := []string{
		"DROP USER IF EXISTS 'app'@'10.0.4.12'",
		"DROP USER IF EXISTS 'app'@'127.0.0.1'",
		"DROP USER IF EXISTS 'app'@'localhost'",
	}
	if strings.Join(svc.sqlLog, "\n") != strings.Join(all, "\n") {
		t.Fatalf("all hosts sql = %#v", svc.sqlLog)
	}
}

func passwordService(version, hosts string) *Service {
	return &Service{
		captureSQL: true,
		queryFn:    hostQuery(version, hosts),
	}
}

func hostQuery(version, hosts string) func(context.Context, string) (string, error) {
	return func(_ context.Context, sql string) (string, error) {
		switch {
		case strings.Contains(sql, "SELECT Host FROM mysql.user"):
			body := "Host\n"
			if hosts != "" {
				body += hosts + "\n"
			}
			return body, nil
		case strings.Contains(sql, "SELECT VERSION()"):
			return "VERSION()\n" + version + "\n", nil
		default:
			return "", errQuery(sql)
		}
	}
}

func accountHost(t *testing.T, sql string) string {
	t.Helper()
	const marker = "@'"
	i := strings.Index(sql, marker)
	if i < 0 {
		t.Fatalf("no host in %q", sql)
	}
	rest := sql[i+len(marker):]
	j := strings.Index(rest, "'")
	if j < 0 {
		t.Fatalf("no host in %q", sql)
	}
	return rest[:j]
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
