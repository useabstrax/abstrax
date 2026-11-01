package user

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	executil "abstrax/internal/exec"
)

const (
	lastlog2Fixture = `mike     pts/0    203.0.113.5      Tue Jan 14 10:11:12 2026
alice                                   **Never logged in**`

	lastlogFixture = `Username         Port     From             Latest
deploy           pts/1    203.0.113.8      Tue Jan 14 10:11:12 +0000 2026
alice                                   **Never logged in**`

	lastIPv6Fixture = `deploy   pts/1        2001:db8::1      Tue Jan 14 10:11:12 2026 - Tue Jan 14 11:00:00 2026  (00:48)`

	lastHostFixture = `deploy   pts/1        web.example      Tue Jan 14 10:11:12 2026 - Tue Jan 14 11:00:00 2026  (00:48)`
)

func TestParseLastlog2Fixture(t *testing.T) {
	got := parseLoginOutput(lastlog2Fixture)
	assertLogin(t, got, "mike", "2026-01-14T10:11:12Z", "203.0.113.5")
	if _, ok := got["alice"]; ok {
		t.Fatal("alice has a last login")
	}
}

func TestParseLastlogFixture(t *testing.T) {
	got := parseLoginOutput(lastlogFixture)
	assertLogin(t, got, "deploy", "2026-01-14T10:11:12Z", "203.0.113.8")
	if _, ok := got["alice"]; ok {
		t.Fatal("alice has a last login")
	}
}

func TestParseLastFixtures(t *testing.T) {
	got := parseLoginOutput(lastIPv6Fixture)
	assertLogin(t, got, "deploy", "2026-01-14T10:11:12Z", "2001:db8::1")

	got = parseLoginOutput(lastHostFixture)
	assertLogin(t, got, "deploy", "2026-01-14T10:11:12Z", "web.example")
}

func TestParseLastlog2FixedWidth(t *testing.T) {
	// lastlog2 prints a 16-wide name, an 8-wide port, and a 42-wide host,
	// with a zoned timestamp (%a %b %e %H:%M:%S %z %Y).
	line := fmt.Sprintf("%-16s %-8.8s %-42s %s", "mike", "pts/0", "203.0.113.5", "Tue Jan  4 09:08:07 +0000 2026")
	out := "Username         Port     From                                       Latest\n" + line
	got := parseLoginOutput(out)
	assertLogin(t, got, "mike", "2026-01-04T09:08:07Z", "203.0.113.5")
}

func TestParseLastIgnoresWtmpTrailer(t *testing.T) {
	out := lastIPv6Fixture + "\n\nwtmp begins Tue Jan 14 00:00:00 2026\n"
	got := parseLoginOutput(out)
	if _, ok := got["wtmp"]; ok {
		t.Fatal("parsed wtmp trailer as a user")
	}
	assertLogin(t, got, "deploy", "2026-01-14T10:11:12Z", "2001:db8::1")
}

func TestInfoLastLoginFromLastlog2(t *testing.T) {
	svc := newLoginService(t, func(name string, args []string) (executil.Result, error) {
		if name == "lastlog2" {
			return executil.Result{Stdout: "mike     pts/0    203.0.113.5      Tue Jan 14 10:11:12 2026"}, nil
		}
		return executil.Result{}, fmt.Errorf("%s not found", name)
	})
	info, err := svc.Info(context.Background(), "mike")
	if err != nil {
		t.Fatal(err)
	}
	if info.LastLoginAt == nil || *info.LastLoginAt != "2026-01-14T10:11:12Z" {
		t.Fatalf("at = %v", info.LastLoginAt)
	}
	if info.LastLoginFrom == nil || *info.LastLoginFrom != "203.0.113.5" {
		t.Fatalf("from = %v", info.LastLoginFrom)
	}
	assertLoginJSON(t, info, "2026-01-14T10:11:12Z", "203.0.113.5")
	if countCmd(svc, "lastlog") != 0 || countCmd(svc, "last") != 0 {
		t.Fatalf("commands = %#v", svc.runner.Commands())
	}
	assertCmd(t, svc, []string{"lastlog2", "-u", "mike"})
}

func TestInfoLastLoginFallsBackToLastlog(t *testing.T) {
	svc := newLoginService(t, func(name string, args []string) (executil.Result, error) {
		if name == "lastlog" {
			return executil.Result{Stdout: lastlogFixture}, nil
		}
		return executil.Result{}, fmt.Errorf("%s not found", name)
	})
	info, err := svc.Info(context.Background(), "deploy")
	if err != nil {
		t.Fatal(err)
	}
	if info.LastLoginFrom == nil || *info.LastLoginFrom != "203.0.113.8" {
		t.Fatalf("from = %v", info.LastLoginFrom)
	}
	if countCmd(svc, "last") != 0 {
		t.Fatal("last was called")
	}
	assertCmd(t, svc, []string{"lastlog", "-u", "deploy"})
}

func TestInfoLastLoginFallsBackToLast(t *testing.T) {
	svc := newLoginService(t, func(name string, args []string) (executil.Result, error) {
		if name == "last" {
			return executil.Result{Stdout: lastHostFixture + "\n\nwtmp begins Tue Jan 14 00:00:00 2026\n"}, nil
		}
		return executil.Result{}, fmt.Errorf("%s not found", name)
	})
	info, err := svc.Info(context.Background(), "deploy")
	if err != nil {
		t.Fatal(err)
	}
	if info.LastLoginFrom == nil || *info.LastLoginFrom != "web.example" {
		t.Fatalf("from = %v", info.LastLoginFrom)
	}
	assertCmd(t, svc, []string{"last", "-F", "-n", "1", "-w", "deploy"})
}

func TestInfoLastLoginUnavailable(t *testing.T) {
	svc := newLoginService(t, func(name string, args []string) (executil.Result, error) {
		return executil.Result{}, fmt.Errorf("%s not found", name)
	})
	info, err := svc.Info(context.Background(), "mike")
	if err != nil {
		t.Fatal(err)
	}
	if info.LastLoginAt != nil || info.LastLoginFrom != nil {
		t.Fatalf("info = %+v", info)
	}
	if info.Locked || info.Expired || info.LoginBlocked {
		t.Fatalf("account state changed: %+v", info)
	}
	assertLoginJSON(t, info, "", "")
}

func TestListReadsLoginLogOnce(t *testing.T) {
	svc := newListService(t, func(name string, args []string) (executil.Result, error) {
		switch name {
		case "lastlog2":
			if len(args) != 0 {
				t.Errorf("lastlog2 args = %#v", args)
			}
			return executil.Result{Stdout: lastlog2Fixture}, nil
		case "lastlog", "last":
			t.Errorf("unexpected %s", name)
			return executil.Result{}, fmt.Errorf("%s not found", name)
		default:
			return executil.Result{}, nil
		}
	})
	users, err := svc.List(context.Background(), ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 3 {
		t.Fatalf("users = %+v", users)
	}
	if countCmd(svc, "lastlog2") != 1 {
		t.Fatalf("lastlog2 calls = %d, commands = %#v", countCmd(svc, "lastlog2"), svc.runner.Commands())
	}
	if countCmd(svc, "lastlog") != 0 || countCmd(svc, "last") != 0 {
		t.Fatalf("commands = %#v", svc.runner.Commands())
	}
	byName := indexUsers(users)
	if byName["mike"].LastLoginAt == nil || *byName["mike"].LastLoginAt != "2026-01-14T10:11:12Z" {
		t.Fatalf("mike = %+v", byName["mike"])
	}
	if byName["mike"].LastLoginFrom == nil || *byName["mike"].LastLoginFrom != "203.0.113.5" {
		t.Fatalf("mike from = %v", byName["mike"].LastLoginFrom)
	}
	alice := byName["alice"]
	if alice.LastLoginAt != nil || alice.LastLoginFrom != nil {
		t.Fatalf("alice = %+v", alice)
	}
	assertLoginJSON(t, &alice, "", "")
}

func TestListFallsBackToOneLastlogCall(t *testing.T) {
	svc := newListService(t, func(name string, args []string) (executil.Result, error) {
		switch name {
		case "lastlog2":
			return executil.Result{}, fmt.Errorf("not found")
		case "lastlog":
			if len(args) != 0 {
				t.Errorf("lastlog args = %#v", args)
			}
			return executil.Result{Stdout: lastlogFixture}, nil
		case "last":
			t.Errorf("last was called")
			return executil.Result{}, fmt.Errorf("not found")
		default:
			return executil.Result{}, nil
		}
	})
	users, err := svc.List(context.Background(), ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if countCmd(svc, "lastlog2") != 1 || countCmd(svc, "lastlog") != 1 || countCmd(svc, "last") != 0 {
		t.Fatalf("commands = %#v", svc.runner.Commands())
	}
	byName := indexUsers(users)
	if byName["deploy"].LastLoginFrom == nil || *byName["deploy"].LastLoginFrom != "203.0.113.8" {
		t.Fatalf("deploy = %+v", byName["deploy"])
	}
	if byName["alice"].LastLoginAt != nil {
		t.Fatal("alice has a last login")
	}
}

func TestListLoginLookupFailure(t *testing.T) {
	svc := newListService(t, func(name string, args []string) (executil.Result, error) {
		switch name {
		case "lastlog2", "lastlog", "last":
			return executil.Result{}, fmt.Errorf("%s not found", name)
		default:
			return executil.Result{}, nil
		}
	})
	users, err := svc.List(context.Background(), ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(users) != 3 {
		t.Fatalf("len = %d", len(users))
	}
	if countCmd(svc, "last") != 0 {
		t.Fatal("list called last")
	}
	for _, u := range users {
		if u.LastLoginAt != nil || u.LastLoginFrom != nil {
			t.Fatalf("%s has a last login", u.Username)
		}
	}
}

func newLoginService(t *testing.T, login func(name string, args []string) (executil.Result, error)) *Service {
	t.Helper()
	return newTestService(t, t.TempDir(), func(_ context.Context, name string, args []string) (executil.Result, bool, error) {
		switch name {
		case "id":
			user := "mike"
			if len(args) > 0 {
				user = args[len(args)-1]
			}
			return executil.Result{Stdout: fmt.Sprintf("uid=1000(%s) gid=1000(%s) groups=1000(%s)", user, user, user)}, true, nil
		case "getent":
			user := "mike"
			if len(args) > 1 {
				user = args[len(args)-1]
			}
			if len(args) > 0 && args[0] == "passwd" {
				return executil.Result{Stdout: fmt.Sprintf("%s:x:1000:1000::/home/%s:/bin/bash", user, user)}, true, nil
			}
			return executil.Result{Stdout: user + ":!:19000:0:99999:7:::"}, true, nil
		case "passwd":
			user := "mike"
			if len(args) > 0 {
				user = args[len(args)-1]
			}
			return executil.Result{Stdout: user + " P 2020-01-01 0 99999 7 -1"}, true, nil
		default:
			res, err := login(name, args)
			return res, true, err
		}
	})
}

func newListService(t *testing.T, login func(name string, args []string) (executil.Result, error)) *Service {
	t.Helper()
	const passwd = "mike:x:1000:1000::/home/mike:/bin/bash\n" +
		"alice:x:1001:1001::/home/alice:/bin/bash\n" +
		"deploy:x:1002:1002::/home/deploy:/bin/bash\n"
	return newTestService(t, t.TempDir(), func(_ context.Context, name string, args []string) (executil.Result, bool, error) {
		switch name {
		case "getent":
			if len(args) > 0 && args[0] == "passwd" && len(args) == 1 {
				return executil.Result{Stdout: passwd}, true, nil
			}
			return executil.Result{}, true, nil
		case "id", "passwd":
			return executil.Result{}, true, nil
		default:
			res, err := login(name, args)
			return res, true, err
		}
	})
}

func assertLogin(t *testing.T, got map[string]loginRecord, user, at, from string) {
	t.Helper()
	rec, ok := got[user]
	if !ok {
		t.Fatalf("missing %s in %#v", user, got)
	}
	if rec.at == nil || *rec.at != at {
		t.Fatalf("%s at = %v, want %s", user, rec.at, at)
	}
	if rec.from == nil || *rec.from != from {
		gotFrom := "<nil>"
		if rec.from != nil {
			gotFrom = *rec.from
		}
		t.Fatalf("%s from = %s, want %s", user, gotFrom, from)
	}
}

func assertLoginJSON(t *testing.T, info *UserInfo, at, from string) {
	t.Helper()
	data, err := json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	gotAt, ok := payload["last_login_at"]
	if !ok {
		t.Fatalf("last_login_at omitted: %s", data)
	}
	gotFrom, ok := payload["last_login_from"]
	if !ok {
		t.Fatalf("last_login_from omitted: %s", data)
	}
	if at == "" {
		if gotAt != nil {
			t.Fatalf("last_login_at = %#v", gotAt)
		}
	} else if gotAt != at {
		t.Fatalf("last_login_at = %#v", gotAt)
	}
	if from == "" {
		if gotFrom != nil {
			t.Fatalf("last_login_from = %#v", gotFrom)
		}
	} else if gotFrom != from {
		t.Fatalf("last_login_from = %#v", gotFrom)
	}
}

func assertCmd(t *testing.T, svc *Service, want []string) {
	t.Helper()
	for _, cmd := range svc.runner.Commands() {
		if cmd[0] != want[0] {
			continue
		}
		if len(cmd) != len(want) {
			t.Fatalf("command = %#v, want %#v", cmd, want)
		}
		for i := range want {
			if cmd[i] != want[i] {
				t.Fatalf("command = %#v, want %#v", cmd, want)
			}
		}
		return
	}
	t.Fatalf("missing %#v in %#v", want, svc.runner.Commands())
}

func countCmd(svc *Service, name string) int {
	n := 0
	for _, cmd := range svc.runner.Commands() {
		if len(cmd) > 0 && cmd[0] == name {
			n++
		}
	}
	return n
}

func indexUsers(users []UserInfo) map[string]UserInfo {
	out := make(map[string]UserInfo, len(users))
	for _, u := range users {
		out[u.Username] = u
	}
	return out
}
