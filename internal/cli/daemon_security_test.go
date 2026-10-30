package cli

import (
	"strings"
	"testing"
)

func TestDaemonAddRejectsUnsafeValues(t *testing.T) {
	cases := [][]string{
		{"add", "worker", "--user", "deploy", "--command", "php artisan queue:work\nuser=root"},
		{"add", "worker", "--user", "deploy", "--command", "sleep 1\n[program:x]\ncommand=/bin/sh\nuser=root"},
		{"add", "worker", "--user", "deploy\r", "--command", "true"},
		{"add", "worker", "--command", "true", "--directory", "/var/www\x00"},
		{"add", "worker", "--command", "true", "--stdout-logfile", "relative.log"},
		{"add", "worker", "--command", "true", "--stopsignal", "NOPE"},
		{"add", "worker", "--command", "true", "--autorestart", "on-failure"},
		{"add", "worker", "--command", "true", "--exitcodes", "1,a"},
		{"add", "worker", "--command", "true", "--environment", "BAD KEY=value"},
		{"add", "worker", "--command", "true", "--environment", `APP=say "hi"`},
	}
	for _, args := range cases {
		cmd := NewDaemonCmd()
		cmd.SetArgs(args)
		err := cmd.Execute()
		if err == nil {
			t.Fatalf("expected error for %q", strings.Join(args, " "))
		}
	}
}

func TestDaemonModifyRejectsLineBreak(t *testing.T) {
	cmd := NewDaemonCmd()
	cmd.SetArgs([]string{"modify", "worker", "--command", "true\nuser=root"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected error")
	}
}
