package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteConfRejectsLineBreaks(t *testing.T) {
	dir := t.TempDir()
	svc := &Service{confDir: dir, confExt: ".conf"}
	cases := []AddOptions{
		{Name: "worker", Command: "php artisan queue:work\nuser=root", User: "deploy", Autostart: true},
		{Name: "worker", Command: "sleep 1\n[program:x]\ncommand=/bin/sh\nuser=root", User: "deploy", Autostart: true},
		{Name: "worker", Command: "true", User: "deploy\r", Autostart: true},
		{Name: "worker", Command: "true", Directory: "/var/www\nuser=root", Autostart: true},
		{Name: "worker", Command: "true", StdoutLogFile: "/var/log/a\x00", Autostart: true},
		{Name: "worker", Command: "true", Environment: map[string]string{"APP_ENV": "prod\nuser=root"}, Autostart: true},
	}
	for _, opts := range cases {
		path := filepath.Join(dir, "abstrax-worker.conf")
		err := svc.writeConf(path, opts)
		if err == nil {
			t.Fatalf("expected error for %+v", opts)
		}
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Fatalf("config written for %+v", opts)
		}
	}
}

func TestWriteConfRejectsInvalidFields(t *testing.T) {
	dir := t.TempDir()
	svc := &Service{confDir: dir, confExt: ".conf"}
	path := filepath.Join(dir, "abstrax-worker.conf")
	cases := []AddOptions{
		{Name: "worker", Command: "true", User: "Deploy", Autostart: true},
		{Name: "worker", Command: "true", Directory: "relative", Autostart: true},
		{Name: "worker", Command: "true", StopSignal: "on-failure", Autostart: true},
		{Name: "worker", Command: "true", Autorestart: "on-failure", Autostart: true},
		{Name: "worker", Command: "true", ExitCodes: "0,,1", Autostart: true},
		{Name: "worker", Command: "true", Environment: map[string]string{"BAD KEY": "x"}, Autostart: true},
		{Name: "worker", Command: "true", Environment: map[string]string{"APP": `say "hi"`}, Autostart: true},
		{Name: "worker", Command: "true", Environment: map[string]string{"APP": `a\b`}, Autostart: true},
	}
	for _, opts := range cases {
		if err := svc.writeConf(path, opts); err == nil {
			t.Fatalf("expected error for %+v", opts)
		}
	}
}

func TestPercentAndEnvironmentRoundTrip(t *testing.T) {
	dir := t.TempDir()
	svc := &Service{confDir: dir, confExt: ".conf"}
	path := filepath.Join(dir, "abstrax-worker.conf")
	opts := AddOptions{
		Name:          "worker",
		Command:       "date +%F",
		Directory:     "/var/www/app",
		User:          "deploy",
		Processes:     1,
		Autostart:     true,
		Autorestart:   "unexpected",
		StartSecs:     1,
		StartRetries:  3,
		StopSignal:    "TERM",
		StopWaitSecs:  10,
		StdoutLogFile: "/var/log/worker/%(program)s.log",
		Environment: map[string]string{
			"ZED":     "1",
			"MESSAGE": "hello, world=ok",
			"RATE":    "100%",
		},
	}
	if err := svc.writeConf(path, opts); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "command=date +%%F\n") {
		t.Fatalf("percent not escaped:\n%s", text)
	}
	if !strings.Contains(text, `environment=MESSAGE="hello, world=ok",RATE="100%%",ZED="1"`) {
		t.Fatalf("environment =\n%s", text)
	}
	got, err := readConf(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Command != "date +%F" {
		t.Fatalf("command = %q", got.Command)
	}
	if got.Environment["MESSAGE"] != "hello, world=ok" || got.Environment["RATE"] != "100%" {
		t.Fatalf("env = %#v", got.Environment)
	}
	if got.StdoutLogFile != "/var/log/worker/%(program)s.log" {
		t.Fatalf("stdout = %q", got.StdoutLogFile)
	}
}

func TestWriteConfGolden(t *testing.T) {
	dir := t.TempDir()
	svc := &Service{confDir: dir, confExt: ".conf"}
	path := filepath.Join(dir, "abstrax-worker.conf")
	err := svc.writeConf(path, AddOptions{
		Name:         "worker",
		Command:      "php artisan queue:work",
		Directory:    "/var/www/app",
		User:         "deploy",
		Processes:    1,
		Autostart:    true,
		Autorestart:  "unexpected",
		StartSecs:    1,
		StartRetries: 3,
		StopSignal:   "TERM",
		StopWaitSecs: 10,
		Environment:  map[string]string{"PATH": "/usr/bin", "APP_ENV": "production"},
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := `[program:worker]
command=php artisan queue:work
directory=/var/www/app
user=deploy
numprocs=1
autostart=true
autorestart=unexpected
startsecs=1
startretries=3
stopsignal=TERM
stopwaitsecs=10
stdout_logfile=/var/log/supervisor/worker-stdout.log
stderr_logfile=/var/log/supervisor/worker-stderr.log
environment=APP_ENV="production",PATH="/usr/bin"
`
	if string(data) != want {
		t.Fatalf("config =\n%s\nwant =\n%s", data, want)
	}
}
