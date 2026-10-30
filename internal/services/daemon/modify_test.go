package daemon

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	executil "abstrax/internal/exec"
)

func TestReadConfRoundTrip(t *testing.T) {
	dir := t.TempDir()
	svc := &Service{runner: executil.New(true, false), confDir: dir, confExt: ".conf"}
	original := AddOptions{
		Name:          "worker",
		Command:       "php artisan queue:work",
		Directory:     "/srv/app",
		User:          "deploy",
		Processes:     3,
		Autostart:     true,
		Autorestart:   "always",
		StartSecs:     2,
		StartRetries:  4,
		StopSignal:    "QUIT",
		StopWaitSecs:  3600,
		ExitCodes:     "0,2",
		StdoutLogFile: "/var/log/worker.out",
		StderrLogFile: "/var/log/worker.err",
		Environment:   map[string]string{"APP_ENV": "production", "QUEUE": "emails"},
	}
	path := filepath.Join(dir, "abstrax-worker.conf")
	if err := svc.writeConf(path, original); err != nil {
		t.Fatal(err)
	}
	got, err := readConf(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Command != original.Command || got.Directory != original.Directory || got.User != original.User || got.Processes != original.Processes {
		t.Fatalf("identity = %#v", got)
	}
	if got.Autorestart != "always" || got.StartSecs != 2 || got.StartRetries != 4 || got.StopSignal != "QUIT" || got.StopWaitSecs != 3600 {
		t.Fatalf("policy = %#v", got)
	}
	if got.ExitCodes != "0,2" || got.StdoutLogFile != original.StdoutLogFile || got.StderrLogFile != original.StderrLogFile {
		t.Fatalf("logs = %#v", got)
	}
	if got.Environment["APP_ENV"] != "production" || got.Environment["QUEUE"] != "emails" {
		t.Fatalf("env = %#v", got.Environment)
	}

	if err := svc.writeConf(path+"2", got); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(path)
	second, _ := os.ReadFile(path + "2")
	if string(first) != string(second) {
		t.Fatalf("rewrite differed:\n%s\n---\n%s", first, second)
	}
}

func TestModifyKeepsUnspecifiedSettings(t *testing.T) {
	restore := stubSupervisorInstalled(t)
	defer restore()

	dir := t.TempDir()
	svc := &Service{runner: executil.New(true, false), confDir: dir, confExt: ".conf"}
	path := filepath.Join(dir, "abstrax-worker.conf")
	if err := svc.writeConf(path, AddOptions{
		Name: "worker", Command: "php artisan queue:work", Directory: "/srv/app",
		User: "deploy", Processes: 3, Autostart: true, Autorestart: "always",
		StartSecs: 2, StartRetries: 4, StopSignal: "QUIT", StopWaitSecs: 3600,
		ExitCodes: "0,2", StdoutLogFile: "/var/log/worker.out", StderrLogFile: "/var/log/worker.err",
		Environment: map[string]string{"APP_ENV": "production"},
	}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Modify(context.Background(), AddOptions{
		Name: "worker", Processes: 4, Changed: map[string]bool{"processes": true},
	}); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(after), "numprocs=4") || strings.Contains(string(after), "numprocs=3") {
		t.Fatalf("processes not updated:\n%s", after)
	}
	for _, line := range strings.Split(string(before), "\n") {
		if strings.HasPrefix(line, "numprocs=") || line == "" {
			continue
		}
		if !strings.Contains(string(after), line) {
			t.Fatalf("lost line %q\nbefore:\n%s\nafter:\n%s", line, before, after)
		}
	}
}

func TestModifySingleFlags(t *testing.T) {
	restore := stubSupervisorInstalled(t)
	defer restore()

	dir := t.TempDir()
	svc := &Service{runner: executil.New(true, false), confDir: dir, confExt: ".conf"}
	base := AddOptions{
		Name: "worker", Command: "true", Directory: "/srv", User: "deploy",
		Processes: 1, Autostart: true, Autorestart: "unexpected",
		StartSecs: 1, StartRetries: 3, StopSignal: "TERM", StopWaitSecs: 10,
	}
	cases := []struct {
		changed string
		opts    AddOptions
		want    string
	}{
		{"autostart", AddOptions{Autostart: false, Changed: map[string]bool{"autostart": true}}, "autostart=false"},
		{"autorestart", AddOptions{Autorestart: "always", Changed: map[string]bool{"autorestart": true}}, "autorestart=always"},
		{"startsecs", AddOptions{StartSecs: 8, Changed: map[string]bool{"startsecs": true}}, "startsecs=8"},
		{"startretries", AddOptions{StartRetries: 9, Changed: map[string]bool{"startretries": true}}, "startretries=9"},
		{"stopsignal", AddOptions{StopSignal: "INT", Changed: map[string]bool{"stopsignal": true}}, "stopsignal=INT"},
		{"stopwaitsecs", AddOptions{StopWaitSecs: 30, Changed: map[string]bool{"stopwaitsecs": true}}, "stopwaitsecs=30"},
		{"exitcodes", AddOptions{ExitCodes: "0,2", Changed: map[string]bool{"exitcodes": true}}, "exitcodes=0,2"},
		{"stdout-logfile", AddOptions{StdoutLogFile: "/tmp/out.log", Changed: map[string]bool{"stdout-logfile": true}}, "stdout_logfile=/tmp/out.log"},
		{"stderr-logfile", AddOptions{StderrLogFile: "/tmp/err.log", Changed: map[string]bool{"stderr-logfile": true}}, "stderr_logfile=/tmp/err.log"},
	}
	for _, tt := range cases {
		t.Run(tt.changed, func(t *testing.T) {
			path := filepath.Join(dir, "abstrax-worker.conf")
			if err := svc.writeConf(path, base); err != nil {
				t.Fatal(err)
			}
			tt.opts.Name = "worker"
			if _, err := svc.Modify(context.Background(), tt.opts); err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(data), tt.want) {
				t.Fatalf("missing %s in\n%s", tt.want, data)
			}
			if !strings.Contains(string(data), "command=true") || !strings.Contains(string(data), "directory=/srv") || !strings.Contains(string(data), "user=deploy") {
				t.Fatalf("other fields changed:\n%s", data)
			}
		})
	}
}

func TestModifyEnvironment(t *testing.T) {
	restore := stubSupervisorInstalled(t)
	defer restore()

	dir := t.TempDir()
	svc := &Service{runner: executil.New(true, false), confDir: dir, confExt: ".conf"}
	path := filepath.Join(dir, "abstrax-worker.conf")
	base := AddOptions{
		Name: "worker", Command: "true", Processes: 1, Autostart: true,
		Environment: map[string]string{"A": "1", "B": "2"},
	}
	if err := svc.writeConf(path, base); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Modify(context.Background(), AddOptions{
		Name: "worker", Environment: map[string]string{"B": "3", "C": "4"},
		Changed: map[string]bool{"environment": true},
	}); err != nil {
		t.Fatal(err)
	}
	got, err := readConf(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Environment["A"] != "1" || got.Environment["B"] != "3" || got.Environment["C"] != "4" {
		t.Fatalf("merged = %#v", got.Environment)
	}

	if _, err := svc.Modify(context.Background(), AddOptions{
		Name: "worker", UnsetEnvironment: []string{"A"},
		Changed: map[string]bool{"unset-environment": true},
	}); err != nil {
		t.Fatal(err)
	}
	got, err = readConf(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.Environment["A"]; ok || got.Environment["C"] != "4" {
		t.Fatalf("after unset = %#v", got.Environment)
	}

	if _, err := svc.Modify(context.Background(), AddOptions{
		Name: "worker", Environment: map[string]string{"D": "5"},
		Changed: map[string]bool{"clear-environment": true, "environment": true},
	}); err != nil {
		t.Fatal(err)
	}
	got, err = readConf(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Environment) != 1 || got.Environment["D"] != "5" {
		t.Fatalf("after clear = %#v", got.Environment)
	}
}

func TestAutorestartRejectsOnFailure(t *testing.T) {
	err := validateAutorestart("on-failure")
	if err == nil || !strings.Contains(err.Error(), "always, unexpected, or false") {
		t.Fatalf("err = %v", err)
	}
	merged, err := mergeDaemonOptions(AddOptions{Autorestart: "unexpected"}, AddOptions{
		Autorestart: "on-failure",
		Changed:     map[string]bool{"autorestart": true},
	})
	if err == nil {
		t.Fatalf("merged = %#v", merged)
	}
}

func stubSupervisorInstalled(t *testing.T) func() {
	t.Helper()
	orig := supervisorInstalled
	supervisorInstalled = func(string) bool { return true }
	return func() { supervisorInstalled = orig }
}
