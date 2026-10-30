package cron

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModifyAppliesOutputOptions(t *testing.T) {
	dir := t.TempDir()
	svc := &Service{cronDir: dir}

	job, err := svc.Add(context.Background(), AddOptions{
		ID:       "worker",
		User:     "www-data",
		Command:  "/usr/bin/php artisan schedule:run",
		Schedule: "0 * * * *",
		Enabled:  true,
	})
	if err != nil {
		t.Fatal(err)
	}

	updated, err := svc.Modify(context.Background(), ModifyOptions{
		ID:     job.ID,
		Output: "/var/log/worker.log",
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(updated.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "> /var/log/worker.log 2>&1") {
		t.Fatalf("expected output redirect, got:\n%s", data)
	}
}

func TestCronRoundTripKeepsFieldsSeparate(t *testing.T) {
	dir := t.TempDir()
	svc := &Service{cronDir: dir}

	_, err := svc.Add(context.Background(), AddOptions{
		ID:           "backup",
		User:         "deploy",
		Command:      "php artisan schedule:run",
		Schedule:     "0 3 * * *",
		WorkingDir:   "/srv/my app",
		Output:       "/var/log/cron log/app.log",
		AppendOutput: true,
		Enabled:      true,
		Env: map[string]string{
			"GREETING": "hello world",
			"MESSAGE":  "nightly backup of the main database now",
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	info, err := svc.Info(context.Background(), "backup")
	if err != nil {
		t.Fatal(err)
	}
	if info.Command != "php artisan schedule:run" {
		t.Fatalf("command = %q", info.Command)
	}
	if info.WorkingDir != "/srv/my app" || info.Output != "/var/log/cron log/app.log" || !info.AppendOutput {
		t.Fatalf("fields = %#v", info)
	}
	if info.Env["GREETING"] != "hello world" || info.Env["MESSAGE"] != "nightly backup of the main database now" {
		t.Fatalf("env = %#v", info.Env)
	}
	if info.Schedule != "0 3 * * *" {
		t.Fatalf("schedule = %q", info.Schedule)
	}

	data, err := os.ReadFile(info.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "cd '/srv/my app' && php artisan schedule:run >> '/var/log/cron log/app.log' 2>&1") {
		t.Fatalf("cron line:\n%s", content)
	}
	if !strings.Contains(content, "# abstrax:job ") {
		t.Fatalf("missing job metadata:\n%s", content)
	}
}

func TestModifyCommandKeepsDirectoryAndOutput(t *testing.T) {
	dir := t.TempDir()
	svc := &Service{cronDir: dir}

	_, err := svc.Add(context.Background(), AddOptions{
		ID:         "backup",
		User:       "deploy",
		Command:    "php artisan schedule:run",
		Schedule:   "0 3 * * *",
		WorkingDir: "/a",
		Output:     "/a.log",
		Enabled:    true,
	})
	if err != nil {
		t.Fatal(err)
	}

	updated, err := svc.Modify(context.Background(), ModifyOptions{
		ID:      "backup",
		Command: "new",
		Changed: map[string]bool{"command": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Command != "new" || updated.WorkingDir != "/a" || updated.Output != "/a.log" {
		t.Fatalf("job = %#v", updated)
	}
	data, err := os.ReadFile(updated.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if strings.Contains(content, "cd /a && cd") || strings.Contains(content, "> /a.log 2>&1 >") {
		t.Fatalf("nested command:\n%s", content)
	}
	if !strings.Contains(content, "cd /a && new > /a.log 2>&1") {
		t.Fatalf("cron line:\n%s", content)
	}

	replaced, err := svc.Modify(context.Background(), ModifyOptions{
		ID:         "backup",
		WorkingDir: "/b",
		Changed:    map[string]bool{"working-dir": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if replaced.WorkingDir != "/b" || replaced.Command != "new" || replaced.Output != "/a.log" {
		t.Fatalf("replaced = %#v", replaced)
	}
	data, err = os.ReadFile(replaced.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	content = string(data)
	if strings.Contains(content, "cd /a") || !strings.Contains(content, "cd /b && new > /a.log 2>&1") {
		t.Fatalf("working dir was nested:\n%s", content)
	}
}

func TestModifyClearsWorkingDir(t *testing.T) {
	dir := t.TempDir()
	svc := &Service{cronDir: dir}
	if _, err := svc.Add(context.Background(), AddOptions{
		ID: "job", User: "root", Command: "true", Schedule: "* * * * *",
		WorkingDir: "/tmp", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	job, err := svc.Modify(context.Background(), ModifyOptions{
		ID:         "job",
		WorkingDir: "",
		Changed:    map[string]bool{"working-dir": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if job.WorkingDir != "" {
		t.Fatalf("working dir = %q", job.WorkingDir)
	}
}

func TestReadLegacyCronFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "abstrax-backup")
	body := `# Managed by Abstrax - do not edit manually
# abstrax:cron id=backup user=deploy created=2026-10-01T03:00:00Z
#
0 3 * * * deploy cd /srv/app && php artisan schedule:run >> /var/log/cron.log 2>&1
`
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	svc := &Service{cronDir: dir}
	job, err := svc.Info(context.Background(), "backup")
	if err != nil {
		t.Fatal(err)
	}
	if job.Command != "cd /srv/app && php artisan schedule:run >> /var/log/cron.log 2>&1" {
		t.Fatalf("command = %q", job.Command)
	}
	if job.WorkingDir != "" || job.Output != "" {
		t.Fatalf("legacy file should not split fields: %#v", job)
	}
	if job.Created != "2026-10-01T03:00:00Z" {
		t.Fatalf("created = %q", job.Created)
	}
}

func TestEnvValueWithSpacesDoesNotReplaceSchedule(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "abstrax-backup")
	body := `# abstrax:cron id=backup user=deploy created=2026-10-01T03:00:00Z
MESSAGE=nightly backup of the main database now
0 3 * * * deploy /usr/bin/true
`
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	svc := &Service{cronDir: dir}
	job, err := svc.Info(context.Background(), "backup")
	if err != nil {
		t.Fatal(err)
	}
	if job.Schedule != "0 3 * * *" || job.Command != "/usr/bin/true" || job.User != "deploy" {
		t.Fatalf("job = %#v", job)
	}
	if job.Env["MESSAGE"] != "nightly backup of the main database now" {
		t.Fatalf("env = %#v", job.Env)
	}
}

func TestRebootMacroEnableDisable(t *testing.T) {
	dir := t.TempDir()
	svc := &Service{cronDir: dir}
	if _, err := svc.Add(context.Background(), AddOptions{
		ID: "boot", User: "root", Command: "/usr/local/bin/start", Schedule: "@reboot", Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}

	info, err := svc.Info(context.Background(), "boot")
	if err != nil {
		t.Fatal(err)
	}
	if info.Schedule != "@reboot" || info.Command != "/usr/local/bin/start" || !info.Enabled {
		t.Fatalf("info = %#v", info)
	}

	jobs, err := svc.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 1 || jobs[0].Schedule != "@reboot" {
		t.Fatalf("list = %#v", jobs)
	}

	if err := svc.Disable(context.Background(), "boot"); err != nil {
		t.Fatal(err)
	}
	disabled, err := svc.Info(context.Background(), "boot")
	if err != nil {
		t.Fatal(err)
	}
	if disabled.Enabled {
		t.Fatal("expected disabled")
	}
	data, err := os.ReadFile(disabled.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "#DISABLED# @reboot root /usr/local/bin/start") {
		t.Fatalf("disabled file:\n%s", data)
	}

	if err := svc.Enable(context.Background(), "boot"); err != nil {
		t.Fatal(err)
	}
	enabled, err := svc.Info(context.Background(), "boot")
	if err != nil {
		t.Fatal(err)
	}
	if !enabled.Enabled || enabled.Schedule != "@reboot" || enabled.Command != "/usr/local/bin/start" {
		t.Fatalf("enabled = %#v", enabled)
	}
	data, err = os.ReadFile(enabled.FilePath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "#DISABLED#") {
		t.Fatalf("still disabled:\n%s", data)
	}
	if strings.Count(string(data), "@reboot root /usr/local/bin/start") != 1 {
		t.Fatalf("schedule line duplicated:\n%s", data)
	}
}

func TestModifyPreservesCreatedTimestamp(t *testing.T) {
	dir := t.TempDir()
	svc := &Service{cronDir: dir}
	job, err := svc.Add(context.Background(), AddOptions{
		ID: "job", User: "root", Command: "true", Schedule: "@hourly", Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := svc.Modify(context.Background(), ModifyOptions{
		ID:      "job",
		Command: "false",
		Changed: map[string]bool{"command": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Created != job.Created || updated.Created == "" {
		t.Fatalf("created changed from %q to %q", job.Created, updated.Created)
	}
	if updated.Updated == "" {
		t.Fatal("expected updated timestamp")
	}
}

func TestModifyPreservesCommandWithoutOutputFlags(t *testing.T) {
	dir := t.TempDir()
	svc := &Service{cronDir: dir}

	job, err := svc.Add(context.Background(), AddOptions{
		ID:       "worker",
		User:     "www-data",
		Command:  "/usr/bin/true",
		Schedule: "0 * * * *",
		Enabled:  true,
	})
	if err != nil {
		t.Fatal(err)
	}

	updated, err := svc.Modify(context.Background(), ModifyOptions{
		ID:       job.ID,
		Schedule: "5 * * * *",
	})
	if err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(dir, filepath.Base(job.FilePath)))
	_ = updated
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	if !strings.Contains(content, "5 * * * * www-data /usr/bin/true") {
		t.Fatalf("expected bare command, got:\n%s", content)
	}
	if strings.Contains(content, ">>") {
		t.Fatalf("did not expect redirects, got:\n%s", content)
	}
}
