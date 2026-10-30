package user

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	executil "abstrax/internal/exec"
)

func TestLockAndUnlockCommands(t *testing.T) {
	dir := t.TempDir()
	svc := newTestService(t, dir, func(_ context.Context, name string, args []string) (executil.Result, bool, error) {
		if name == "getent" && len(args) >= 2 && args[0] == "shadow" {
			return executil.Result{Stdout: "demo:!:19000:0:99999:7::30000:"}, true, nil
		}
		return executil.Result{}, true, nil
	})

	if err := svc.Lock(context.Background(), LockOptions{Username: "demo"}); err != nil {
		t.Fatal(err)
	}
	assertLastArgs(t, svc, []string{"usermod", "--lock", "--expiredate", "1", "demo"})
	date, err := daysToDate("30000")
	if err != nil {
		t.Fatal(err)
	}
	record, ok, err := svc.savedLock("demo")
	if err != nil || !ok || record.PasswordOnly || record.Expiry != date {
		t.Fatalf("record = %+v ok=%v err=%v", record, ok, err)
	}

	if err := svc.Unlock(context.Background(), LockOptions{Username: "demo"}); err != nil {
		t.Fatal(err)
	}
	assertLastArgs(t, svc, []string{"usermod", "--unlock", "--expiredate", date, "demo"})
	if _, err := os.Stat(filepath.Join(dir, "user-locks.json")); !os.IsNotExist(err) {
		t.Fatal("lock state was not removed")
	}
}

func TestLockPasswordOnly(t *testing.T) {
	dir := t.TempDir()
	svc := newTestService(t, dir, func(context.Context, string, []string) (executil.Result, bool, error) {
		return executil.Result{}, true, nil
	})
	if err := svc.Lock(context.Background(), LockOptions{Username: "demo", PasswordOnly: true}); err != nil {
		t.Fatal(err)
	}
	assertLastArgs(t, svc, []string{"usermod", "--lock", "demo"})

	if err := svc.Unlock(context.Background(), LockOptions{Username: "demo"}); err != nil {
		t.Fatal(err)
	}
	assertLastArgs(t, svc, []string{"usermod", "--unlock", "demo"})
}

func TestUnlockWithoutSavedExpiry(t *testing.T) {
	dir := t.TempDir()
	svc := newTestService(t, dir, func(context.Context, string, []string) (executil.Result, bool, error) {
		return executil.Result{}, true, nil
	})
	if err := svc.Unlock(context.Background(), LockOptions{Username: "demo"}); err != nil {
		t.Fatal(err)
	}
	assertLastArgs(t, svc, []string{"usermod", "--unlock", "--expiredate", "", "demo"})
}

func TestAccountStateCombinations(t *testing.T) {
	now := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		name    string
		status  string
		expire  string
		locked  bool
		expired bool
		blocked bool
	}{
		{"open", "demo P 2020-01-01 0 99999 7 -1", "", false, false, false},
		{"password locked", "demo L 2020-01-01 0 99999 7 -1", "", true, false, false},
		{"expired only", "demo P 2020-01-01 0 99999 7 -1", "1", false, true, false},
		{"login blocked", "demo L 2020-01-01 0 99999 7 -1", "1", true, true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			info := &UserInfo{Username: "demo"}
			parts := strings.Fields(c.status)
			info.Locked = len(parts) >= 2 && parts[1] == "L"
			info.Expired = accountExpired(c.expire, now)
			info.LoginBlocked = info.Locked && info.Expired
			if info.Locked != c.locked || info.Expired != c.expired || info.LoginBlocked != c.blocked {
				t.Fatalf("got locked=%v expired=%v blocked=%v", info.Locked, info.Expired, info.LoginBlocked)
			}
		})
	}
}

func TestInfoReportsLoginState(t *testing.T) {
	dir := t.TempDir()
	svc := newTestService(t, dir, func(_ context.Context, name string, args []string) (executil.Result, bool, error) {
		switch name {
		case "id":
			return executil.Result{Stdout: "uid=1000(demo) gid=1000(demo) groups=1000(demo)"}, true, nil
		case "getent":
			if args[0] == "passwd" {
				return executil.Result{Stdout: "demo:x:1000:1000::/home/demo:/bin/bash"}, true, nil
			}
			return executil.Result{Stdout: "demo:!:19000:0:99999:7::1:"}, true, nil
		case "passwd":
			return executil.Result{Stdout: "demo L 2020-01-01 0 99999 7 -1"}, true, nil
		default:
			return executil.Result{}, true, nil
		}
	})
	info, err := svc.Info(context.Background(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if !info.Locked || !info.Expired || !info.LoginBlocked {
		t.Fatalf("info = %+v", info)
	}
}

func newTestService(t *testing.T, dir string, fn executil.CommandFunc) *Service {
	t.Helper()
	runner := executil.New(false, false)
	runner.SetCommandFunc(fn)
	return &Service{
		runner:        runner,
		lockStatePath: filepath.Join(dir, "user-locks.json"),
	}
}

func assertLastArgs(t *testing.T, svc *Service, want []string) {
	t.Helper()
	cmds := svc.runner.Commands()
	if len(cmds) == 0 {
		t.Fatal("no commands")
	}
	got := cmds[len(cmds)-1]
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("args = %#v, want %#v", got, want)
	}
}
