package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	executil "abstrax/internal/exec"
	"abstrax/internal/services/user"
)

func TestUserRemoveJSONIncludesStillUsedBy(t *testing.T) {
	sup, cronDir := removeFixtureDirs(t)
	writeRemoveDaemon(t, sup, "abstrax-platform-queue.conf", "platform-queue", "deploy", "4")
	writeRemoveCron(t, cronDir, "abstrax-database-backup", "0 3 * * * deploy php artisan backup:run\n")

	var svc *user.Service
	restore := stubUserRemove(t, func(dry, verbose bool) *user.Service {
		svc = user.New(dry, verbose)
		svc.SetConfigDirs(sup, cronDir)
		svc.SetSudoersDir(t.TempDir())
		svc.SetProcessLister(func(context.Context, string) ([]user.Process, error) {
			return []user.Process{{PID: 4242, Args: "php artisan queue:work"}}, nil
		})
		svc.SetCommandFunc(func(context.Context, string, []string) (executil.Result, bool, error) {
			return executil.Result{}, true, nil
		})
		return svc
	})
	defer restore()

	out, err := captureRootStdout(t, "--json", "--yes", "user", "remove", "deploy")
	if err != nil {
		t.Fatal(err)
	}
	raw := decodeJSON(t, out)
	if raw["status"] != "success" || raw["action"] != "user.remove" {
		t.Fatalf("result = %#v", raw)
	}
	used := stillUsedBy(t, raw)
	if len(used) != 3 {
		t.Fatalf("still_used_by = %#v\n%s", used, out)
	}
	if !hasCommand(svc, "deluser") {
		t.Fatalf("deluser was not recorded: %#v", svcCommands(svc))
	}
}

func TestUserRemoveJSONEmptyStillUsedBy(t *testing.T) {
	sup, cronDir := removeFixtureDirs(t)
	restore := stubUserRemove(t, func(dry, verbose bool) *user.Service {
		svc := user.New(dry, verbose)
		svc.SetConfigDirs(sup, cronDir)
		svc.SetSudoersDir(t.TempDir())
		svc.SetProcessLister(func(context.Context, string) ([]user.Process, error) {
			return nil, nil
		})
		svc.SetCommandFunc(func(context.Context, string, []string) (executil.Result, bool, error) {
			return executil.Result{}, true, nil
		})
		return svc
	})
	defer restore()

	out, err := captureRootStdout(t, "--json", "--yes", "user", "remove", "deploy")
	if err != nil {
		t.Fatal(err)
	}
	raw := decodeJSON(t, out)
	used := stillUsedBy(t, raw)
	if used == nil {
		t.Fatalf("still_used_by missing: %s", out)
	}
	if len(used) != 0 {
		t.Fatalf("still_used_by = %#v", used)
	}
}

func TestUserRemoveIfUnusedJSONRefuses(t *testing.T) {
	sup, cronDir := removeFixtureDirs(t)
	writeRemoveDaemon(t, sup, "abstrax-platform-queue.conf", "platform-queue", "deploy", "4")
	writeRemoveCron(t, cronDir, "abstrax-database-backup", "0 3 * * * deploy php artisan backup:run\n")

	var svc *user.Service
	restore := stubUserRemove(t, func(dry, verbose bool) *user.Service {
		svc = user.New(dry, verbose)
		svc.SetConfigDirs(sup, cronDir)
		svc.SetSudoersDir(t.TempDir())
		svc.SetProcessLister(func(context.Context, string) ([]user.Process, error) {
			return nil, nil
		})
		svc.SetCommandFunc(func(context.Context, string, []string) (executil.Result, bool, error) {
			return executil.Result{}, true, nil
		})
		return svc
	})
	defer restore()

	out, err := captureRootStdout(t, "--json", "--yes", "user", "remove", "deploy", "--if-unused")
	if err == nil {
		t.Fatal("expected non-zero refusal")
	}
	raw := decodeJSON(t, out)
	if raw["status"] != "error" || raw["error_code"] != "user_in_use" {
		t.Fatalf("result = %#v\nerr = %v", raw, err)
	}
	if !strings.Contains(raw["message"].(string), "daemon platform-queue (4 processes)") ||
		!strings.Contains(raw["message"].(string), "cron job database-backup") {
		t.Fatalf("message = %#v", raw["message"])
	}
	used := stillUsedBy(t, raw)
	if len(used) != 2 {
		t.Fatalf("still_used_by = %#v", used)
	}
	if hasCommand(svc, "deluser") {
		t.Fatal("deluser was called")
	}
}

func TestUserRemoveIfUnusedFlag(t *testing.T) {
	cmd := newUserRemoveCmd()
	if cmd.Flags().Lookup("if-unused") == nil {
		t.Fatal("missing --if-unused")
	}
}

func stubUserRemove(t *testing.T, factory func(dry, verbose bool) *user.Service) func() {
	t.Helper()
	resetGlobalFlags(t)
	origPrepare := prepareUserRemove
	origNew := newUserRemoveService
	prepareUserRemove = func() error { return nil }
	newUserRemoveService = factory
	return func() {
		prepareUserRemove = origPrepare
		newUserRemoveService = origNew
	}
}

func removeFixtureDirs(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	sup := filepath.Join(root, "supervisor")
	cronDir := filepath.Join(root, "cron")
	if err := os.MkdirAll(sup, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cronDir, 0o755); err != nil {
		t.Fatal(err)
	}
	return sup, cronDir
}

func writeRemoveDaemon(t *testing.T, dir, name, program, username, numprocs string) {
	t.Helper()
	body := "[program:" + program + "]\nuser=" + username + "\nnumprocs=" + numprocs + "\n"
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeRemoveCron(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func decodeJSON(t *testing.T, out string) map[string]any {
	t.Helper()
	var raw map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &raw); err != nil {
		t.Fatalf("unmarshal %q: %v", out, err)
	}
	return raw
}

func stillUsedBy(t *testing.T, raw map[string]any) []any {
	t.Helper()
	data, ok := raw["data"].(map[string]any)
	if !ok {
		t.Fatalf("data = %#v", raw["data"])
	}
	used, ok := data["still_used_by"].([]any)
	if !ok {
		t.Fatalf("still_used_by = %#v", data["still_used_by"])
	}
	return used
}

func hasCommand(svc *user.Service, name string) bool {
	for _, cmd := range svcCommands(svc) {
		if len(cmd) > 0 && cmd[0] == name {
			return true
		}
	}
	return false
}

func svcCommands(svc *user.Service) [][]string {
	return svc.Commands()
}
