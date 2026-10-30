package daemon

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	executil "abstrax/internal/exec"
)

const statusFixture = `worker:worker_00             RUNNING   pid 1234, uptime 2:03:04
worker:worker_01             STARTING  
worker:worker_02             BACKOFF   Exited too quickly (process log may have details)
worker:worker_03             FATAL     Exited too quickly (process log may have details)
single                       RUNNING   pid 9, uptime 1 day, 0:00:05
stopped                      STOPPED   Not started
exited                       EXITED    Feb 01 01:02 AM
nginx                        RUNNING   pid 42, uptime 0:10:00
`

func TestParseStatusFields(t *testing.T) {
	grouped := parseStatusOutput(statusFixture)

	running := grouped["worker"][0]
	if running.Status != "RUNNING" || running.PID != 1234 || running.Uptime != "2:03:04" || running.UptimeSeconds != 7384 {
		t.Fatalf("running = %#v", running)
	}
	if grouped["worker"][1].Status != "STARTING" || grouped["worker"][1].PID != 0 || grouped["worker"][1].Uptime != "" {
		t.Fatalf("starting = %#v", grouped["worker"][1])
	}
	if grouped["worker"][2].Status != "BACKOFF" || grouped["worker"][3].Status != "FATAL" {
		t.Fatalf("worker = %#v", grouped["worker"])
	}
	if summarize(grouped["worker"]) != "FATAL" {
		t.Fatalf("summary = %s", summarize(grouped["worker"]))
	}

	single := grouped["single"][0]
	if single.PID != 9 || single.Uptime != "1 day, 0:00:05" || single.UptimeSeconds != 86405 {
		t.Fatalf("single = %#v", single)
	}
	if grouped["stopped"][0].Status != "STOPPED" || grouped["stopped"][0].PID != 0 {
		t.Fatalf("stopped = %#v", grouped["stopped"][0])
	}
	if grouped["exited"][0].Status != "EXITED" || grouped["exited"][0].Description == "" {
		t.Fatalf("exited = %#v", grouped["exited"][0])
	}
}

func TestListHidesUnmanagedPrograms(t *testing.T) {
	restore := stubSupervisor(t)
	defer restore()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "abstrax-worker.conf"), []byte("[program:worker]\ncommand=/bin/true\nnumprocs=4\n"), 0644); err != nil {
		t.Fatal(err)
	}
	runner := executil.New(true, false)
	runner.SetCommandFunc(func(_ context.Context, name string, args []string) (executil.Result, bool, error) {
		if name == "supervisorctl" && len(args) > 0 && args[0] == "status" {
			return executil.Result{Stdout: statusFixture}, true, nil
		}
		return executil.Result{}, false, nil
	})
	svc := &Service{runner: runner, confDir: dir, confExt: ".conf"}

	managed, err := svc.List(context.Background(), ListOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(managed) != 1 || managed[0].Name != "worker" || !managed[0].Managed || managed[0].Status != "FATAL" {
		t.Fatalf("managed = %#v", managed)
	}
	if len(managed[0].Processes) != 4 {
		t.Fatalf("processes = %d", len(managed[0].Processes))
	}
	if managed[0].ConfigPath == "" {
		t.Fatal("expected config path")
	}

	all, err := svc.List(context.Background(), ListOptions{All: true})
	if err != nil {
		t.Fatal(err)
	}
	foundNginx := false
	for _, item := range all {
		if item.Name == "nginx" {
			foundNginx = true
			if item.Managed || item.ConfigPath != "" {
				t.Fatalf("nginx = %#v", item)
			}
		}
	}
	if !foundNginx {
		t.Fatalf("nginx missing from %#v", all)
	}
}

func TestControlUsesGroupTarget(t *testing.T) {
	restore := stubSupervisor(t)
	defer restore()

	runner := executil.New(true, false)
	svc := &Service{runner: runner, confDir: t.TempDir(), confExt: ".conf"}
	ctx := context.Background()
	if err := svc.Start(ctx, "worker"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Stop(ctx, "worker"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Restart(ctx, "worker"); err != nil {
		t.Fatal(err)
	}
	got := joinCmds(runner.Commands())
	for _, want := range []string{
		"supervisorctl start worker:*",
		"supervisorctl stop worker:*",
		"supervisorctl restart worker:*",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in:\n%s", want, got)
		}
	}
}

func TestLogsReturnsLastLines(t *testing.T) {
	restore := stubSupervisor(t)
	defer restore()

	dir := t.TempDir()
	stdout := filepath.Join(dir, "stdout.log")
	stderr := filepath.Join(dir, "stderr.log")
	writeLines(t, stdout, 10, "out")
	writeLines(t, stderr, 10, "err")
	conf := "[program:worker]\ncommand=/bin/true\nnumprocs=2\nstdout_logfile=" + stdout + "\nstderr_logfile=" + stderr + "\n"
	if err := os.WriteFile(filepath.Join(dir, "abstrax-worker.conf"), []byte(conf), 0644); err != nil {
		t.Fatal(err)
	}
	svc := &Service{runner: executil.New(true, false), confDir: dir, confExt: ".conf"}

	got, err := svc.Logs(context.Background(), LogOptions{Name: "worker", Lines: 3})
	if err != nil {
		t.Fatal(err)
	}
	if got != "out-7\nout-8\nout-9" {
		t.Fatalf("stdout = %q", got)
	}

	got, err = svc.Logs(context.Background(), LogOptions{Name: "worker", Lines: 2, Stderr: true})
	if err != nil {
		t.Fatal(err)
	}
	if got != "err-8\nerr-9" {
		t.Fatalf("stderr = %q", got)
	}
}

func TestLogsFollowArgumentOrder(t *testing.T) {
	restore := stubSupervisor(t)
	defer restore()

	dir := t.TempDir()
	conf := "[program:worker]\ncommand=/bin/true\nnumprocs=2\n"
	if err := os.WriteFile(filepath.Join(dir, "abstrax-worker.conf"), []byte(conf), 0644); err != nil {
		t.Fatal(err)
	}
	runner := executil.New(true, false)
	svc := &Service{runner: runner, confDir: dir, confExt: ".conf"}
	_, err := svc.Logs(context.Background(), LogOptions{Name: "worker", Follow: true, Stderr: true, Process: 1})
	if err != nil {
		t.Fatal(err)
	}
	got := joinCmds(runner.Commands())
	if !strings.Contains(got, "supervisorctl tail -f worker:worker_01 stderr") {
		t.Fatalf("commands:\n%s", got)
	}
}

func TestInfoReadsConfig(t *testing.T) {
	dir := t.TempDir()
	conf := `[program:worker]
command=/usr/bin/php artisan queue:work
directory=/srv/app
user=deploy
numprocs=2
autostart=true
autorestart=always
startsecs=2
startretries=4
stopsignal=TERM
stopwaitsecs=15
exitcodes=0,2
stdout_logfile=/var/log/worker.out
stderr_logfile=/var/log/worker.err
environment=APP_ENV="production",QUEUE="emails"
`
	if err := os.WriteFile(filepath.Join(dir, "abstrax-worker.conf"), []byte(conf), 0644); err != nil {
		t.Fatal(err)
	}
	orig := supervisorInstalled
	supervisorInstalled = func(string) bool { return false }
	t.Cleanup(func() { supervisorInstalled = orig })

	svc := &Service{runner: executil.New(true, false), confDir: dir, confExt: ".conf"}
	info, err := svc.Info(context.Background(), "worker")
	if err != nil {
		t.Fatal(err)
	}
	if info.Command != "/usr/bin/php artisan queue:work" || info.Directory != "/srv/app" || info.User != "deploy" || info.Numprocs != 2 {
		t.Fatalf("info = %#v", info)
	}
	if info.Autorestart != "always" || info.StartSecs != 2 || info.StopWaitSecs != 15 || info.ExitCodes != "0,2" {
		t.Fatalf("settings = %#v", info)
	}
	if info.Environment["APP_ENV"] != "production" || info.Environment["QUEUE"] != "emails" {
		t.Fatalf("env = %#v", info.Environment)
	}
}

func stubSupervisor(t *testing.T) func() {
	t.Helper()
	orig := supervisorInstalled
	supervisorInstalled = func(string) bool { return true }
	return func() { supervisorInstalled = orig }
}

func writeLines(t *testing.T, path string, n int, prefix string) {
	t.Helper()
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString(prefix)
		b.WriteString("-")
		b.WriteString(strconv.Itoa(i))
		b.WriteString("\n")
	}
	if err := os.WriteFile(path, []byte(b.String()), 0644); err != nil {
		t.Fatal(err)
	}
}

func joinCmds(cmds [][]string) string {
	lines := make([]string, len(cmds))
	for i, cmd := range cmds {
		lines[i] = strings.Join(cmd, " ")
	}
	return strings.Join(lines, "\n")
}
