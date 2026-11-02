package cron

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	executil "abstrax/internal/exec"
)

func TestRunBuildsCommand(t *testing.T) {
	stubLookup(t, map[string]string{"deploy": "/home/deploy"})
	refuseExecution(t)

	dir := t.TempDir()
	runner := executil.New(false, false)
	var calls [][]string
	runner.SetCommandFunc(func(_ context.Context, name string, args []string) (executil.Result, bool, error) {
		calls = append(calls, append([]string{name}, args...))
		return executil.Result{Stdout: "ok\n", ExitCode: 0}, true, nil
	})
	svc := &Service{cronDir: dir, runner: runner}
	if _, err := svc.Add(context.Background(), AddOptions{
		ID:           "backup",
		User:         "deploy",
		Command:      "id -un; pwd; env",
		Schedule:     "* * * * *",
		WorkingDir:   "/srv/app",
		Output:       "/var/log/backup.log",
		AppendOutput: true,
		Enabled:      true,
		Env: map[string]string{
			"APP_ENV": "production",
			"PATH":    "/opt/bin",
		},
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Run(context.Background(), RunOptions{ID: "backup", Timeout: 5 * time.Minute}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Run(context.Background(), RunOptions{ID: "backup", UseJobOutput: true}); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 {
		t.Fatalf("calls = %d", len(calls))
	}

	tool := "sudo"
	if executil.Exists("runuser") {
		tool = "runuser"
	}
	wantEnv := []string{
		"APP_ENV=production",
		"HOME=/home/deploy",
		"PATH=/opt/bin",
		"SHELL=/bin/sh",
	}
	assertRunArgv(t, calls[0], tool, "deploy", wantEnv, "cd /srv/app && id -un; pwd; env")
	if strings.Contains(calls[0][len(calls[0])-1], "/var/log/backup.log") {
		t.Fatalf("default run applied job output: %q", calls[0][len(calls[0])-1])
	}
	assertRunArgv(t, calls[1], tool, "deploy", wantEnv, "cd /srv/app && id -un; pwd; env >> /var/log/backup.log 2>&1")
	if len(runner.Commands()) != 2 {
		t.Fatalf("recorded %d commands", len(runner.Commands()))
	}
}

func TestRunPassesExitCode(t *testing.T) {
	dir := t.TempDir()
	runner := executil.New(false, false)
	runner.SetCommandFunc(func(context.Context, string, []string) (executil.Result, bool, error) {
		return executil.Result{ExitCode: 3, Stdout: "nope\n"}, true, nil
	})
	svc := &Service{cronDir: dir, runner: runner}
	if _, err := svc.Add(context.Background(), AddOptions{
		ID: "backup", User: "deploy", Command: "exit 3", Schedule: "* * * * *", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	res, err := svc.Run(context.Background(), RunOptions{ID: "backup"})
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode == nil || *res.ExitCode != 3 {
		t.Fatalf("exit code = %v", res.ExitCode)
	}
	if res.Output != "nope\n" || res.OutputTruncated || res.TimedOut || res.Disabled || res.DryRun {
		t.Fatalf("result = %#v", res)
	}
	if _, err := time.Parse(time.RFC3339, res.StartedAt); err != nil {
		t.Fatal(err)
	}
	if _, err := time.Parse(time.RFC3339, res.FinishedAt); err != nil {
		t.Fatal(err)
	}
}

func TestRunLimitedPassesExitCode(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	code, timedOut, err := runLimited(context.Background(), []string{"sh", "-c", "exit 3"}, time.Second, io.Discard, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if timedOut || code != 3 {
		t.Fatalf("code=%d timedOut=%v", code, timedOut)
	}
}

func TestRunTimeoutKillsProcessGroup(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
	dir := t.TempDir()
	pidFile := filepath.Join(dir, "child.pid")
	marker := filepath.Join(dir, "done")
	t.Cleanup(func() {
		data, err := os.ReadFile(pidFile)
		if err != nil {
			return
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil || pid <= 0 {
			return
		}
		_ = syscall.Kill(pid, syscall.SIGKILL)
	})

	script := fmt.Sprintf("sleep 30 & echo $! > %s; wait; echo done > %s", shellQuote(pidFile), shellQuote(marker))
	start := time.Now()
	code, timedOut, err := runLimited(context.Background(), []string{"sh", "-c", script}, time.Second, io.Discard, io.Discard)
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if !timedOut || code != 124 {
		t.Fatalf("code=%d timedOut=%v elapsed=%s", code, timedOut, elapsed)
	}
	if elapsed > 8*time.Second {
		t.Fatalf("took %s; process group was not killed", elapsed)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("child finished after the timeout")
	}
	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	if processAlive(pid) {
		t.Fatalf("child %d still running", pid)
	}
}

func TestRunReportsTimeout(t *testing.T) {
	dir := t.TempDir()
	svc := &Service{cronDir: dir}
	if _, err := svc.Add(context.Background(), AddOptions{
		ID: "backup", User: "deploy", Command: "sleep 30", Schedule: "* * * * *", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	orig := executeArgv
	executeArgv = func(_ context.Context, _ []string, timeout time.Duration, stdout, _ io.Writer) (int, bool, error) {
		if timeout != time.Second {
			return 0, false, fmt.Errorf("timeout = %s", timeout)
		}
		_, _ = io.WriteString(stdout, "partial")
		return 124, true, nil
	}
	t.Cleanup(func() { executeArgv = orig })

	res, err := svc.Run(context.Background(), RunOptions{ID: "backup", Timeout: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode == nil || *res.ExitCode != 124 || !res.TimedOut {
		t.Fatalf("result = %#v", res)
	}
	if res.Output != "partial" || res.OutputTruncated {
		t.Fatalf("output = %q truncated=%v", res.Output, res.OutputTruncated)
	}
}

func TestRunTruncatesOutput(t *testing.T) {
	dir := t.TempDir()
	runner := executil.New(false, false)
	payload := "HEAD" + strings.Repeat("a", maxRunOutput) + "TAIL"
	runner.SetCommandFunc(func(context.Context, string, []string) (executil.Result, bool, error) {
		return executil.Result{Stdout: payload, ExitCode: 0}, true, nil
	})
	svc := &Service{cronDir: dir, runner: runner}
	if _, err := svc.Add(context.Background(), AddOptions{
		ID: "backup", User: "deploy", Command: "true", Schedule: "* * * * *", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	res, err := svc.Run(context.Background(), RunOptions{ID: "backup"})
	if err != nil {
		t.Fatal(err)
	}
	if !res.OutputTruncated || len(res.Output) != maxRunOutput {
		t.Fatalf("len=%d truncated=%v", len(res.Output), res.OutputTruncated)
	}
	if strings.Contains(res.Output, "HEAD") || !strings.HasSuffix(res.Output, "TAIL") {
		t.Fatalf("kept the head or lost the tail: prefix=%q suffix=%q", res.Output[:4], res.Output[len(res.Output)-4:])
	}
}

func TestCaptureWriterKeepsTailAndLines(t *testing.T) {
	payload := "HEAD" + strings.Repeat("b", maxRunOutput) + "TAIL"
	var lines []string
	capw := newCapture(maxRunOutput, nil, func(line string) {
		lines = append(lines, line)
	})
	w := capw.stdout()
	mid := len(payload) / 2
	if _, err := w.Write([]byte(payload[:mid])); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(w, "\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(payload[mid:])); err != nil {
		t.Fatal(err)
	}
	capw.flush()

	out, truncated := capw.output()
	if !truncated || len(out) != maxRunOutput {
		t.Fatalf("len=%d truncated=%v", len(out), truncated)
	}
	if strings.Contains(out, "HEAD") || !strings.HasSuffix(out, "TAIL") {
		t.Fatal("capture writer did not keep the tail")
	}
	if len(lines) == 0 || !strings.Contains(lines[0], "HEAD") {
		t.Fatalf("lines = %#v", lines)
	}
	joined := strings.Join(lines, "")
	if !strings.Contains(joined, "TAIL") {
		t.Fatalf("flushed lines = %#v", lines)
	}
}

func TestRunUnknownID(t *testing.T) {
	svc := &Service{cronDir: t.TempDir()}
	_, err := svc.Run(context.Background(), RunOptions{ID: "missing"})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "not found") || !strings.Contains(err.Error(), "missing") {
		t.Fatal(err)
	}
}

func TestRunDisabledJob(t *testing.T) {
	dir := t.TempDir()
	runner := executil.New(false, false)
	called := false
	runner.SetCommandFunc(func(context.Context, string, []string) (executil.Result, bool, error) {
		called = true
		return executil.Result{ExitCode: 0, Stdout: "ran\n"}, true, nil
	})
	svc := &Service{cronDir: dir, runner: runner}
	if _, err := svc.Add(context.Background(), AddOptions{
		ID: "backup", User: "deploy", Command: "true", Schedule: "* * * * *", Enabled: false,
	}); err != nil {
		t.Fatal(err)
	}

	res, err := svc.Run(context.Background(), RunOptions{ID: "backup"})
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("disabled job was not run")
	}
	if !res.Disabled || res.ExitCode == nil || *res.ExitCode != 0 {
		t.Fatalf("result = %#v", res)
	}
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"disabled":true`) || !strings.Contains(string(raw), `"exit_code":0`) {
		t.Fatalf("json = %s", raw)
	}
}

func TestRunDryRunDoesNotExecute(t *testing.T) {
	origLookup := lookupUser
	lookupUser = func(string) (*user.User, error) {
		return nil, errors.New("missing")
	}
	t.Cleanup(func() { lookupUser = origLookup })
	refuseExecution(t)

	dir := t.TempDir()
	runner := executil.New(false, false)
	runner.SetCommandFunc(func(context.Context, string, []string) (executil.Result, bool, error) {
		t.Fatal("dry-run executed the command")
		return executil.Result{}, true, nil
	})
	svc := &Service{cronDir: dir, runner: runner}
	if _, err := svc.Add(context.Background(), AddOptions{
		ID: "backup", User: "deploy", Command: "true", Schedule: "* * * * *", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	res, err := svc.Run(context.Background(), RunOptions{ID: "backup", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !res.DryRun || res.ExitCode != nil {
		t.Fatalf("result = %#v", res)
	}
	if !strings.Contains(strings.Join(res.Argv, " "), "HOME=/home/deploy") {
		t.Fatalf("argv = %#v", res.Argv)
	}
	raw, err := json.Marshal(res)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "exit_code") || !strings.Contains(string(raw), `"dry_run":true`) {
		t.Fatalf("json = %s", raw)
	}
	if len(runner.Commands()) != 1 {
		t.Fatalf("recorded %d commands", len(runner.Commands()))
	}
}

func TestHomeDirFallback(t *testing.T) {
	orig := lookupUser
	lookupUser = func(string) (*user.User, error) {
		return nil, errors.New("missing")
	}
	t.Cleanup(func() { lookupUser = orig })
	if homeDir("root") != "/root" {
		t.Fatalf("root home = %q", homeDir("root"))
	}
	if homeDir("deploy") != "/home/deploy" {
		t.Fatalf("deploy home = %q", homeDir("deploy"))
	}
}

func assertRunArgv(t *testing.T, argv []string, tool, user string, env []string, script string) {
	t.Helper()
	prefix := []string{tool, "-u", user, "--", "env", "-i"}
	if len(argv) < len(prefix)+len(env)+3 {
		t.Fatalf("argv = %#v", argv)
	}
	for i, want := range prefix {
		if argv[i] != want {
			t.Fatalf("argv prefix = %#v", argv[:len(prefix)])
		}
	}
	gotEnv := argv[len(prefix) : len(prefix)+len(env)]
	if strings.Join(gotEnv, "\n") != strings.Join(env, "\n") {
		t.Fatalf("env = %#v", gotEnv)
	}
	tail := argv[len(prefix)+len(env):]
	if len(tail) != 3 || tail[0] != "/bin/sh" || tail[1] != "-c" || tail[2] != script {
		t.Fatalf("shell = %#v", tail)
	}
}

func stubLookup(t *testing.T, homes map[string]string) {
	t.Helper()
	orig := lookupUser
	lookupUser = func(name string) (*user.User, error) {
		home, ok := homes[name]
		if !ok {
			return nil, errors.New("no such user")
		}
		return &user.User{Username: name, HomeDir: home}, nil
	}
	t.Cleanup(func() { lookupUser = orig })
}

func refuseExecution(t *testing.T) {
	t.Helper()
	orig := executeArgv
	executeArgv = func(context.Context, []string, time.Duration, io.Writer, io.Writer) (int, bool, error) {
		t.Fatal("command was executed")
		return 0, false, nil
	}
	t.Cleanup(func() { executeArgv = orig })
}

func processAlive(pid int) bool {
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "stat=").Output()
	if err != nil {
		return false
	}
	state := strings.TrimSpace(string(out))
	return state != "" && !strings.HasPrefix(state, "Z")
}
