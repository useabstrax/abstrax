package user

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	executil "abstrax/internal/exec"
)

func TestRemoveUsageSources(t *testing.T) {
	t.Run("daemon", func(t *testing.T) {
		svc, buf := newRemoveService(t)
		writeDaemon(t, svc.supervisorDir, "abstrax-platform-queue.conf", "platform-queue", "deploy", 4)
		result, err := svc.Remove(context.Background(), RemoveOptions{Username: "deploy"})
		if err != nil {
			t.Fatal(err)
		}
		assertUsed(t, result.StillUsedBy, UsedBy{Type: "daemon", Name: "platform-queue", Processes: 4})
		if !strings.Contains(buf.String(), "daemon platform-queue (4 processes)") {
			t.Fatalf("warning = %q", buf.String())
		}
		assertCalled(t, svc, "deluser")
	})

	t.Run("cron", func(t *testing.T) {
		svc, buf := newRemoveService(t)
		writeCron(t, svc.cronDir, "abstrax-database-backup", "0 3 * * * deploy php artisan backup:run\n")
		result, err := svc.Remove(context.Background(), RemoveOptions{Username: "deploy"})
		if err != nil {
			t.Fatal(err)
		}
		assertUsed(t, result.StillUsedBy, UsedBy{Type: "cron", Name: "database-backup"})
		if !strings.Contains(buf.String(), "cron job database-backup") {
			t.Fatalf("warning = %q", buf.String())
		}
		assertCalled(t, svc, "deluser")
	})

	t.Run("both", func(t *testing.T) {
		svc, buf := newRemoveService(t)
		writeDaemon(t, svc.supervisorDir, "abstrax-platform-queue.conf", "platform-queue", "deploy", 4)
		writeCron(t, svc.cronDir, "abstrax-database-backup", "0 3 * * * deploy php artisan backup:run\n")
		result, err := svc.Remove(context.Background(), RemoveOptions{Username: "deploy", Force: true})
		if err != nil {
			t.Fatal(err)
		}
		want := "deploy is still used by:\n  daemon platform-queue (4 processes)\n  cron job database-backup"
		if strings.TrimSuffix(buf.String(), "\n") != want {
			t.Fatalf("warning = %q", buf.String())
		}
		if len(result.StillUsedBy) != 2 {
			t.Fatalf("used = %+v", result.StillUsedBy)
		}
		assertCalled(t, svc, "deluser")
		assertNotCalled(t, svc, "pkill")
	})

	t.Run("neither", func(t *testing.T) {
		svc, buf := newRemoveService(t)
		writeDaemon(t, svc.supervisorDir, "abstrax-other.conf", "other", "someone", 2)
		writeCron(t, svc.cronDir, "abstrax-other-job", "0 3 * * * someone php artisan backup:run\n")
		if err := os.WriteFile(filepath.Join(svc.supervisorDir, "not-managed.conf"), []byte("user=deploy\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		result, err := svc.Remove(context.Background(), RemoveOptions{Username: "deploy"})
		if err != nil {
			t.Fatal(err)
		}
		if buf.Len() != 0 {
			t.Fatalf("unexpected warning %q", buf.String())
		}
		if len(result.StillUsedBy) != 0 {
			t.Fatalf("used = %+v", result.StillUsedBy)
		}
		assertCalled(t, svc, "deluser")
	})
}

func TestRemoveWarnsAndStillRemoves(t *testing.T) {
	svc, buf := newRemoveService(t)
	writeDaemon(t, svc.supervisorDir, "abstrax-platform-queue.conf", "platform-queue", "deploy", 4)
	writeCron(t, svc.cronDir, "abstrax-database-backup", "@daily deploy php artisan backup:run\n")
	_, err := svc.Remove(context.Background(), RemoveOptions{Username: "deploy"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "deploy is still used by:") {
		t.Fatalf("warning = %q", buf.String())
	}
	assertLastArgs(t, svc, []string{"deluser", "deploy"})
}

func TestRemoveIfUnused(t *testing.T) {
	t.Run("refuses", func(t *testing.T) {
		svc, buf := newRemoveService(t)
		writeDaemon(t, svc.supervisorDir, "abstrax-platform-queue.conf", "platform-queue", "deploy", 4)
		writeCron(t, svc.cronDir, "abstrax-database-backup", "0 3 * * * deploy php artisan backup:run\n")
		result, err := svc.Remove(context.Background(), RemoveOptions{Username: "deploy", IfUnused: true, Force: true})
		if err == nil {
			t.Fatal("expected refusal")
		}
		var inUse *InUseError
		if !errors.As(err, &inUse) {
			t.Fatalf("error = %T %v", err, err)
		}
		want := "deploy is still used by:\n  daemon platform-queue (4 processes)\n  cron job database-backup"
		if err.Error() != want {
			t.Fatalf("error = %q", err.Error())
		}
		if buf.Len() != 0 {
			t.Fatalf("warning printed on refusal: %q", buf.String())
		}
		if len(result.StillUsedBy) != 2 {
			t.Fatalf("used = %+v", result.StillUsedBy)
		}
		assertNotCalled(t, svc, "deluser")
		assertNotCalled(t, svc, "pkill")
	})

	t.Run("succeeds when unused", func(t *testing.T) {
		svc, _ := newRemoveService(t)
		result, err := svc.Remove(context.Background(), RemoveOptions{Username: "deploy", IfUnused: true})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.StillUsedBy) != 0 {
			t.Fatalf("used = %+v", result.StillUsedBy)
		}
		assertCalled(t, svc, "deluser")
	})
}

func TestRemoveJSONStillUsedBy(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		svc, _ := newRemoveService(t)
		result, err := svc.Remove(context.Background(), RemoveOptions{Username: "deploy"})
		if err != nil {
			t.Fatal(err)
		}
		raw := marshalResult(t, result)
		if !strings.Contains(raw, `"still_used_by":[]`) {
			t.Fatalf("json = %s", raw)
		}
	})

	t.Run("populated", func(t *testing.T) {
		svc, _ := newRemoveService(t)
		writeDaemon(t, svc.supervisorDir, "abstrax-platform-queue.conf", "platform-queue", "deploy", 4)
		writeCron(t, svc.cronDir, "abstrax-database-backup", "0 3 * * * deploy php artisan backup:run\n")
		svc.SetProcessLister(func(context.Context, string) ([]Process, error) {
			return []Process{{PID: 4242, Args: "php artisan queue:work"}}, nil
		})
		result, err := svc.Remove(context.Background(), RemoveOptions{Username: "deploy"})
		if err != nil {
			t.Fatal(err)
		}
		raw := marshalResult(t, result)
		for _, fragment := range []string{
			`"type":"daemon"`,
			`"name":"platform-queue"`,
			`"processes":4`,
			`"type":"cron"`,
			`"name":"database-backup"`,
			`"type":"process"`,
			`"pid":4242`,
			`"args":"php artisan queue:work"`,
			`"still_used_by":[`,
		} {
			if !strings.Contains(raw, fragment) {
				t.Fatalf("json missing %s: %s", fragment, raw)
			}
		}
	})
}

func TestCronCommentMailtoAndOtherUserAreNotMatches(t *testing.T) {
	svc, buf := newRemoveService(t)
	writeCron(t, svc.cronDir, "abstrax-not-deploy", ""+
		"# 0 3 * * * deploy php artisan backup:run\n"+
		"MAILTO=deploy\n"+
		"PATH=/usr/bin\n"+
		"0 4 * * * other php artisan backup:run\n"+
		"@daily other echo deploy\n")
	writeCron(t, svc.cronDir, "abstrax-database-backup", ""+
		"# comment mentioning deploy\n"+
		"MAILTO=ops\n"+
		"@daily deploy php artisan backup:run\n")
	result, err := svc.Remove(context.Background(), RemoveOptions{Username: "deploy"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.StillUsedBy) != 1 || result.StillUsedBy[0].Name != "database-backup" {
		t.Fatalf("used = %+v", result.StillUsedBy)
	}
	if strings.Contains(buf.String(), "not-deploy") {
		t.Fatalf("warning = %q", buf.String())
	}
}

func TestProcessEntriesAreWarnedAndNotKilled(t *testing.T) {
	t.Run("warns and removes", func(t *testing.T) {
		svc, buf := newRemoveService(t)
		svc.SetProcessLister(func(_ context.Context, username string) ([]Process, error) {
			if username != "deploy" {
				return nil, nil
			}
			return []Process{{PID: 4242, Args: "php artisan queue:work"}}, nil
		})
		result, err := svc.Remove(context.Background(), RemoveOptions{Username: "deploy"})
		if err != nil {
			t.Fatal(err)
		}
		if strings.TrimSuffix(buf.String(), "\n") != "deploy is still used by:\n  process 4242 php artisan queue:work" {
			t.Fatalf("warning = %q", buf.String())
		}
		if len(result.StillUsedBy) != 1 || result.StillUsedBy[0].PID != 4242 {
			t.Fatalf("used = %+v", result.StillUsedBy)
		}
		assertCalled(t, svc, "deluser")
		assertNotCalled(t, svc, "pkill")
	})

	t.Run("if-unused refuses", func(t *testing.T) {
		svc, _ := newRemoveService(t)
		svc.SetProcessLister(func(context.Context, string) ([]Process, error) {
			return []Process{{PID: 7, Args: "sleep 30"}}, nil
		})
		_, err := svc.Remove(context.Background(), RemoveOptions{Username: "deploy", IfUnused: true})
		if err == nil || !strings.Contains(err.Error(), "process 7 sleep 30") {
			t.Fatalf("error = %v", err)
		}
		assertNotCalled(t, svc, "deluser")
		assertNotCalled(t, svc, "pkill")
	})
}

func TestDaemonIniAndDefaultProcessCount(t *testing.T) {
	svc, buf := newRemoveService(t)
	writeDaemon(t, svc.supervisorDir, "abstrax-worker.ini", "worker", "deploy", 0)
	writeDaemon(t, svc.supervisorDir, "abstrax-other.conf", "other", "root", 3)
	result, err := svc.Remove(context.Background(), RemoveOptions{Username: "deploy"})
	if err != nil {
		t.Fatal(err)
	}
	assertUsed(t, result.StillUsedBy, UsedBy{Type: "daemon", Name: "worker", Processes: 1})
	if !strings.Contains(buf.String(), "daemon worker (1 process)") {
		t.Fatalf("warning = %q", buf.String())
	}
}

func TestParsePS(t *testing.T) {
	got := parsePS("  4242 php artisan queue:work\n99\nnot-a-pid ignored\n")
	if len(got) != 2 || got[0].PID != 4242 || got[0].Args != "php artisan queue:work" || got[1].PID != 99 || got[1].Args != "" {
		t.Fatalf("parsed = %+v", got)
	}
}

func newRemoveService(t *testing.T) (*Service, *bytes.Buffer) {
	t.Helper()
	buf := &bytes.Buffer{}
	runner := executil.New(false, false)
	runner.SetCommandFunc(func(context.Context, string, []string) (executil.Result, bool, error) {
		return executil.Result{}, true, nil
	})
	dir := t.TempDir()
	supervisorDir := filepath.Join(dir, "supervisor")
	cronDir := filepath.Join(dir, "cron")
	if err := os.MkdirAll(supervisorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cronDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return &Service{
		runner:        runner,
		sudoersDir:    filepath.Join(dir, "sudoers"),
		supervisorDir: supervisorDir,
		cronDir:       cronDir,
		processes: func(context.Context, string) ([]Process, error) {
			return nil, nil
		},
		out: buf,
	}, buf
}

func writeDaemon(t *testing.T, dir, name, program, user string, numprocs int) {
	t.Helper()
	var b strings.Builder
	b.WriteString("[program:" + program + "]\n")
	b.WriteString("user=" + user + "\n")
	if numprocs > 0 {
		b.WriteString("numprocs=" + strconv.Itoa(numprocs) + "\n")
	}
	if err := os.WriteFile(filepath.Join(dir, name), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeCron(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertUsed(t *testing.T, got []UsedBy, want UsedBy) {
	t.Helper()
	if len(got) != 1 || got[0] != want {
		t.Fatalf("used = %+v, want %+v", got, want)
	}
}

func assertCalled(t *testing.T, svc *Service, name string) {
	t.Helper()
	if !hasCommand(svc, name) {
		t.Fatalf("commands = %#v, want %s", svc.runner.Commands(), name)
	}
}

func assertNotCalled(t *testing.T, svc *Service, name string) {
	t.Helper()
	if hasCommand(svc, name) {
		t.Fatalf("commands = %#v, did not want %s", svc.runner.Commands(), name)
	}
}

func hasCommand(svc *Service, name string) bool {
	for _, cmd := range svc.runner.Commands() {
		if len(cmd) > 0 && cmd[0] == name {
			return true
		}
	}
	return false
}

func marshalResult(t *testing.T, result *RemoveResult) string {
	t.Helper()
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}
