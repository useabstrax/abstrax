package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLogPathsFromConf(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "abstrax-worker.conf")
	content := `[program:worker]
command=/usr/bin/worker
stdout_logfile=/var/log/custom-stdout.log
stderr_logfile=/var/log/custom-stderr.log
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	stdout, stderr := logPathsFromConf(path, "worker")
	if stdout != "/var/log/custom-stdout.log" {
		t.Fatalf("stdout = %q", stdout)
	}
	if stderr != "/var/log/custom-stderr.log" {
		t.Fatalf("stderr = %q", stderr)
	}
}

func TestLogPathsFromConfDefaults(t *testing.T) {
	stdout, stderr := logPathsFromConf("/missing.conf", "worker")
	if stdout != "/var/log/supervisor/worker-stdout.log" {
		t.Fatalf("stdout = %q", stdout)
	}
	if stderr != "/var/log/supervisor/worker-stderr.log" {
		t.Fatalf("stderr = %q", stderr)
	}
}
