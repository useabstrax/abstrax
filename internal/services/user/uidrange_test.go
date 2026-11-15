package user

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	executil "abstrax/internal/exec"
)

func TestIsSystemUIDUsesLoginDefsRange(t *testing.T) {
	r := uidRange{min: 1000, max: 60000}
	cases := []struct {
		uid  int
		want bool
	}{
		{0, false},
		{1, true},
		{999, true},
		{1000, false},
		{1001, false},
		{60000, false},
		{60001, true},
		{65534, true}, // nobody
	}
	for _, tc := range cases {
		if got := isSystemUID(tc.uid, r); got != tc.want {
			t.Fatalf("uid %d: got %v want %v", tc.uid, got, tc.want)
		}
	}
}

func TestLoadUIDRangeFromLoginDefs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "login.defs")
	body := "# comment\nUID_MIN\t2000\nUID_MAX 50000\n"
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	orig := loginDefsPath
	loginDefsPath = path
	t.Cleanup(func() { loginDefsPath = orig })

	r := loadUIDRange()
	if r.min != 2000 || r.max != 50000 {
		t.Fatalf("range = %+v", r)
	}
}

func TestListMarksNobodySystemAndFillsLocked(t *testing.T) {
	dir := t.TempDir()
	defs := filepath.Join(dir, "login.defs")
	if err := os.WriteFile(defs, []byte("UID_MIN 1000\nUID_MAX 60000\n"), 0644); err != nil {
		t.Fatal(err)
	}
	orig := loginDefsPath
	loginDefsPath = defs
	t.Cleanup(func() { loginDefsPath = orig })

	const passwd = "root:x:0:0:root:/root:/bin/bash\n" +
		"nobody:x:65534:65534:nobody:/nonexistent:/usr/sbin/nologin\n" +
		"mike:x:1000:1000::/home/mike:/bin/bash\n" +
		"locked:x:1001:1001::/home/locked:/bin/bash\n"

	svc := newTestService(t, dir, func(_ context.Context, name string, args []string) (executil.Result, bool, error) {
		switch name {
		case "getent":
			if len(args) > 0 && args[0] == "passwd" && len(args) == 1 {
				return executil.Result{Stdout: passwd}, true, nil
			}
			if len(args) >= 2 && args[0] == "shadow" {
				return executil.Result{Stdout: args[1] + ":!:1:0:99999:7:::\n"}, true, nil
			}
			return executil.Result{}, true, nil
		case "id":
			return executil.Result{Stdout: "users"}, true, nil
		case "passwd":
			user := ""
			if len(args) >= 2 {
				user = args[1]
			}
			status := "P"
			if user == "locked" {
				status = "L"
			}
			return executil.Result{Stdout: user + " " + status + " 2020-01-01 0 99999 7 -1\n"}, true, nil
		case "lastlog2", "lastlog", "last":
			return executil.Result{}, true, nil
		default:
			return executil.Result{}, true, nil
		}
	})

	users, err := svc.List(context.Background(), ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	byName := indexUsers(users)

	if byName["nobody"].IsSystem != true {
		t.Fatalf("nobody = %+v", byName["nobody"])
	}
	if byName["mike"].IsSystem {
		t.Fatalf("mike should be regular: %+v", byName["mike"])
	}
	if byName["root"].IsSystem {
		t.Fatalf("root should not be marked system: %+v", byName["root"])
	}
	if !byName["locked"].Locked {
		t.Fatalf("locked user not locked: %+v", byName["locked"])
	}
	if byName["mike"].Locked {
		t.Fatalf("mike unexpectedly locked: %+v", byName["mike"])
	}

	raw, err := json.Marshal(byName["locked"])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"locked":true`) {
		t.Fatalf("json missing locked=true: %s", raw)
	}

	systemOnly, err := svc.List(context.Background(), ListOptions{System: true})
	if err != nil {
		t.Fatal(err)
	}
	sys := indexUsers(systemOnly)
	if _, ok := sys["nobody"]; !ok {
		t.Fatalf("system list = %+v", systemOnly)
	}
	if _, ok := sys["mike"]; ok {
		t.Fatal("mike should not appear in --system")
	}
}
