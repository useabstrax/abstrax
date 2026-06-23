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
