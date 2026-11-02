package cli

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	executil "abstrax/internal/exec"
	"abstrax/internal/globals"
	"abstrax/internal/services/user"
)

const logoutLoginctlFixture = `[{"session":"3","uid":1000,"user":"deploy","seat":"","tty":"pts/1","remote":"true","remote_host":"203.0.113.5","timestamp":"2026-10-31 12:00:00"},{"session":"4","uid":1000,"user":"deploy","seat":"","tty":"pts/2","remote":"true","remote_host":"203.0.113.6","timestamp":"2026-10-31 12:05:00"}]`

func TestLogoutDeclinedRecordsNoTerminate(t *testing.T) {
	resetGlobalFlags(t)
	runner := logoutRunner(t)
	withLogoutHooks(t, func(string, bool) (bool, error) {
		return false, nil
	}, runner)

	cmd := newUserLogoutCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"deploy", "--all"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, command := range runner.Commands() {
		joined := strings.Join(command, " ")
		if strings.Contains(joined, "terminate-session") || strings.Contains(joined, "terminate-user") || command[0] == "kill" || command[0] == "pkill" {
			t.Fatalf("terminate command recorded: %v", command)
		}
	}
}

func TestLogoutYesRecordsTerminate(t *testing.T) {
	resetGlobalFlags(t)
	globals.Flags.Yes = true
	runner := logoutRunner(t)
	withLogoutHooks(t, nil, runner)

	cmd := newUserLogoutCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"deploy", "--all"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var ids []string
	for _, command := range runner.Commands() {
		if len(command) >= 3 && command[0] == "loginctl" && command[1] == "terminate-session" {
			ids = append(ids, command[2])
		}
		if strings.Contains(strings.Join(command, " "), "terminate-user") {
			t.Fatalf("terminate-user recorded: %v", command)
		}
	}
	if strings.Join(ids, ",") != "3,4" {
		t.Fatalf("terminated %v", ids)
	}
}

func TestLogoutRequiresOneSelector(t *testing.T) {
	cases := [][]string{
		{"deploy"},
		{"deploy", "--session", "3", "--all"},
	}
	for _, args := range cases {
		cmd := newUserLogoutCmd()
		buf := &bytes.Buffer{}
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs(args)
		err := cmd.Execute()
		if err == nil {
			t.Fatalf("expected error for %q", strings.Join(args, " "))
		}
		if strings.Contains(err.Error(), "root") {
			t.Fatalf("flag error fell through to root check: %v", err)
		}
	}
}

func TestLogoutHelpMentionsDaemons(t *testing.T) {
	cmd := newUserLogoutCmd()
	buf := &bytes.Buffer{}
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, phrase := range []string{
		"Daemons and cron jobs",
		"does not kill every process the user owns",
		"terminate-user",
	} {
		if !strings.Contains(out, phrase) {
			t.Fatalf("help missing %q:\n%s", phrase, out)
		}
	}
}

func TestSessionsRejectsInvalidUsername(t *testing.T) {
	resetGlobalFlags(t)
	cmd := newUserSessionsCmd()
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs([]string{"Bad Name"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected invalid username")
	}
	if strings.Contains(err.Error(), "root") {
		t.Fatalf("sessions required root: %v", err)
	}
}

func withLogoutHooks(t *testing.T, ask func(string, bool) (bool, error), runner *executil.Runner) {
	t.Helper()
	oldAsk, oldRoot, oldExec := askConfirm, ensureRoot, executeUserLogout
	t.Cleanup(func() {
		askConfirm = oldAsk
		ensureRoot = oldRoot
		executeUserLogout = oldExec
	})
	if ask != nil {
		askConfirm = ask
	}
	ensureRoot = func() error { return nil }
	executeUserLogout = func(ctx context.Context, opts user.LogoutOptions) (*user.LogoutResult, error) {
		svc := user.New(opts.DryRun, false)
		svc.SetRunner(runner)
		return svc.Logout(ctx, opts)
	}
}

func logoutRunner(t *testing.T) *executil.Runner {
	t.Helper()
	runner := executil.New(false, false)
	runner.SetCommandFunc(func(_ context.Context, name string, args []string) (executil.Result, bool, error) {
		if name == "loginctl" && len(args) > 0 && args[0] == "list-sessions" {
			return executil.Result{Stdout: logoutLoginctlFixture}, true, nil
		}
		return executil.Result{}, true, nil
	})
	return runner
}
