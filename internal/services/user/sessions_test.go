package user

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	executil "abstrax/internal/exec"
)

const loginctlFixture = `[
  {"session":"3","uid":1000,"user":"deploy","seat":"","tty":"pts/1","remote":"true","remote_host":"203.0.113.5","timestamp":"2026-10-31 12:00:00","State":"active","idle":"00:05"},
  {"session":"4","uid":1000,"user":"deploy","seat":"","tty":"pts/2","remote":true,"remote_host":"203.0.113.6","timestamp":"2026-10-31 12:05:00","state":"active"},
  {"session":"5","uid":1000,"user":"deploy","seat":"","tty":"pts/4","remote":"true","remote_host":"203.0.113.7","timestamp":"2026-10-31 12:07:00","idle":true},
  {"session":"9","uid":1001,"user":"other","seat":"","tty":"pts/3","remote":"true","remote_host":"198.51.100.9","timestamp":"2026-10-31 12:06:00"}
]`

const loginctlShape = `[{"session":"3","uid":1000,"user":"deploy","seat":"","tty":"pts/1","remote":"true","remote_host":"203.0.113.5","timestamp":"2026-10-31 12:00:00"}]`

const whoFixture = "" +
	"deploy   pts/1        2026-10-31 12:00 203.0.113.5      12345 00:05\n" +
	"deploy   pts/2        2026-10-31 13:00   .          67890 (198.51.100.20)\n" +
	"deploy   pts/4        2026-10-31 14:00 203.0.113.9      22222\n" +
	"other    pts/3        2026-10-31 12:06 198.51.100.9     99999 00:02\n"

const psFixture = "" +
	"  4100 sshd: deploy@pts/1\n" +
	"  4101 sshd: deploy@notty\n" +
	"  4102 sshd: deploy@pts/2\n" +
	"  4104 sshd: deploy@pts/4\n" +
	"  4105 sshd: deploy@pts/10\n" +
	"  4200 sshd: other@pts/3\n" +
	"  5000 /usr/sbin/cron -f\n" +
	"  5001 php /home/deploy/artisan schedule:run\n" +
	"  5002 supervisor: deploy-worker\n" +
	"  5003 sshd: deploy [priv]\n" +
	"  12345 -bash\n"

func TestParseLoginctlSessions(t *testing.T) {
	sessions, err := parseLoginctlSessions(loginctlShape, "deploy")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("len = %d", len(sessions))
	}
	assertSession(t, sessions[0], "3", "pts/1", "203.0.113.5", "2026-10-31 12:00:00", "", true)

	sessions, err = parseLoginctlSessions(loginctlFixture, "deploy")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 3 {
		t.Fatalf("len = %d, sessions = %+v", len(sessions), sessions)
	}
	assertSession(t, sessions[0], "3", "pts/1", "203.0.113.5", "2026-10-31 12:00:00", "00:05", false)
	assertSession(t, sessions[1], "4", "pts/2", "203.0.113.6", "2026-10-31 12:05:00", "", true)
	assertSession(t, sessions[2], "5", "pts/4", "203.0.113.7", "2026-10-31 12:07:00", "", true)

	other, err := parseLoginctlSessions(loginctlFixture, "other")
	if err != nil {
		t.Fatal(err)
	}
	if len(other) != 1 || other[0].ID != "9" {
		t.Fatalf("other = %+v", other)
	}
}

func TestParseLoginctlEmptyList(t *testing.T) {
	sessions, err := parseLoginctlSessions("[]", "deploy")
	if err != nil {
		t.Fatal(err)
	}
	if sessions == nil || len(sessions) != 0 {
		t.Fatalf("sessions = %#v", sessions)
	}
	body, err := json.Marshal(sessions)
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "[]" {
		t.Fatalf("json = %s", body)
	}
}

func TestSessionIdleJSONNullWhenMissing(t *testing.T) {
	body, err := json.Marshal(Session{
		ID:        "3",
		TTY:       "pts/1",
		SourceIP:  "203.0.113.5",
		LoginTime: "2026-10-31 12:00:00",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), `"idle":null`) {
		t.Fatalf("json = %s", body)
	}
}

func TestParseWhoSessions(t *testing.T) {
	sessions, err := parseWhoSessions(whoFixture, "deploy")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 3 {
		t.Fatalf("len = %d, sessions = %+v", len(sessions), sessions)
	}
	assertSession(t, sessions[0], "12345", "pts/1", "203.0.113.5", "2026-10-31 12:00", "00:05", false)
	assertSession(t, sessions[1], "67890", "pts/2", "198.51.100.20", "2026-10-31 13:00", ".", false)
	assertSession(t, sessions[2], "22222", "pts/4", "203.0.113.9", "2026-10-31 14:00", "", true)

	none, err := parseWhoSessions("", "deploy")
	if err != nil {
		t.Fatal(err)
	}
	if none == nil || len(none) != 0 {
		t.Fatalf("empty = %#v", none)
	}

	otherOnly, err := parseWhoSessions(whoFixture, "nobody")
	if err != nil {
		t.Fatal(err)
	}
	if len(otherOnly) != 0 {
		t.Fatalf("nobody = %+v", otherOnly)
	}
}

func TestTerminateCommandsLoginctl(t *testing.T) {
	sessions := []Session{
		{ID: "3", TTY: "pts/1"},
		{ID: "4", TTY: "pts/2"},
	}
	one, err := terminateCommands(sessionBackendLoginctl, "deploy", sessions[:1], psFixture)
	if err != nil {
		t.Fatal(err)
	}
	wantOne := [][]string{{"loginctl", "terminate-session", "3"}}
	if !reflect.DeepEqual(one, wantOne) {
		t.Fatalf("one = %#v", one)
	}

	all, err := terminateCommands(sessionBackendLoginctl, "deploy", sessions, psFixture)
	if err != nil {
		t.Fatal(err)
	}
	wantAll := [][]string{
		{"loginctl", "terminate-session", "3"},
		{"loginctl", "terminate-session", "4"},
	}
	if !reflect.DeepEqual(all, wantAll) {
		t.Fatalf("all = %#v", all)
	}
	for _, cmd := range all {
		if strings.Contains(strings.Join(cmd, " "), "terminate-user") {
			t.Fatalf("terminate-user in %v", cmd)
		}
	}
}

func TestTerminateCommandsWhoSkipsDaemons(t *testing.T) {
	sessions := []Session{
		{ID: "12345", TTY: "pts/1"},
		{ID: "67890", TTY: "pts/2"},
	}
	one, err := terminateCommands(sessionBackendWho, "deploy", sessions[:1], psFixture)
	if err != nil {
		t.Fatal(err)
	}
	wantOne := [][]string{
		{"kill", "-HUP", "4100"},
		{"kill", "-KILL", "4100"},
	}
	if !reflect.DeepEqual(one, wantOne) {
		t.Fatalf("one = %#v", one)
	}

	all, err := terminateCommands(sessionBackendWho, "deploy", sessions, psFixture)
	if err != nil {
		t.Fatal(err)
	}
	wantAll := [][]string{
		{"kill", "-HUP", "4100"},
		{"kill", "-KILL", "4100"},
		{"kill", "-HUP", "4102"},
		{"kill", "-KILL", "4102"},
	}
	if !reflect.DeepEqual(all, wantAll) {
		t.Fatalf("all = %#v", all)
	}

	forbidden := []string{"4101", "4105", "4200", "5000", "5001", "5002", "5003", "12345"}
	for _, cmd := range all {
		for _, pid := range forbidden {
			for _, arg := range cmd {
				if arg == pid {
					t.Fatalf("targeted %s in %v", pid, cmd)
				}
			}
		}
	}
}

func TestSessionsUsesLoginctlJSON(t *testing.T) {
	whoCalled := false
	svc := serviceWith(func(_ context.Context, name string, args []string) (executil.Result, bool, error) {
		if name == "who" {
			whoCalled = true
		}
		if name == "loginctl" && len(args) > 0 && args[0] == "list-sessions" {
			return executil.Result{Stdout: loginctlShape}, true, nil
		}
		return executil.Result{}, true, fmtError("unexpected " + name)
	})
	sessions, err := svc.Sessions(context.Background(), "deploy")
	if err != nil {
		t.Fatal(err)
	}
	if whoCalled {
		t.Fatal("who was called even though loginctl returned JSON")
	}
	if len(sessions) != 1 || sessions[0].ID != "3" || sessions[0].Idle != nil {
		t.Fatalf("sessions = %+v", sessions)
	}
}

func TestSessionsFallsBackToWho(t *testing.T) {
	svc := serviceWith(func(_ context.Context, name string, args []string) (executil.Result, bool, error) {
		if name == "loginctl" {
			return executil.Result{}, true, errors.New("loginctl not found")
		}
		if name == "who" {
			return executil.Result{Stdout: whoFixture}, true, nil
		}
		return executil.Result{}, true, fmtError("unexpected " + name)
	})
	sessions, err := svc.Sessions(context.Background(), "deploy")
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 3 || sessions[0].ID != "12345" {
		t.Fatalf("sessions = %+v", sessions)
	}
}

func TestSessionsEmptyWhenLoginctlHasNone(t *testing.T) {
	svc := serviceWith(func(_ context.Context, name string, _ []string) (executil.Result, bool, error) {
		if name == "loginctl" {
			return executil.Result{Stdout: "[]"}, true, nil
		}
		return executil.Result{}, true, fmtError("unexpected " + name)
	})
	sessions, err := svc.Sessions(context.Background(), "deploy")
	if err != nil {
		t.Fatal(err)
	}
	if sessions == nil || len(sessions) != 0 {
		t.Fatalf("sessions = %#v", sessions)
	}
}

func TestLogoutOneLoginctlSession(t *testing.T) {
	svc := loginctlLogoutService(t)
	result, err := svc.Logout(context.Background(), LogoutOptions{Username: "deploy", SessionID: "3"})
	if err != nil {
		t.Fatal(err)
	}
	assertTerminated(t, svc, []string{"3"})
	if len(result.Ended) != 1 || result.Ended[0].ID != "3" {
		t.Fatalf("ended = %+v", result.Ended)
	}
}

func TestLogoutAllLoginctlSessions(t *testing.T) {
	svc := loginctlLogoutService(t)
	result, err := svc.Logout(context.Background(), LogoutOptions{Username: "deploy", All: true})
	if err != nil {
		t.Fatal(err)
	}
	assertTerminated(t, svc, []string{"3", "4", "5"})
	if len(result.Ended) != 3 {
		t.Fatalf("ended = %+v", result.Ended)
	}
}

func TestLogoutAllWhenNone(t *testing.T) {
	svc := serviceWith(func(_ context.Context, name string, _ []string) (executil.Result, bool, error) {
		if name == "loginctl" {
			return executil.Result{Stdout: "[]"}, true, nil
		}
		return executil.Result{}, true, nil
	})
	result, err := svc.Logout(context.Background(), LogoutOptions{Username: "deploy", All: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Ended == nil || len(result.Ended) != 0 {
		t.Fatalf("ended = %#v", result.Ended)
	}
	assertTerminated(t, svc, nil)
}

func TestLogoutMissingSession(t *testing.T) {
	svc := loginctlLogoutService(t)
	_, err := svc.Logout(context.Background(), LogoutOptions{Username: "deploy", SessionID: "9"})
	if err == nil {
		t.Fatal("expected missing session error")
	}
	assertTerminated(t, svc, nil)
}

func TestLogoutRequiresSessionOrAll(t *testing.T) {
	svc := loginctlLogoutService(t)
	if _, err := svc.Logout(context.Background(), LogoutOptions{Username: "deploy"}); err == nil {
		t.Fatal("expected error")
	}
	if _, err := svc.Logout(context.Background(), LogoutOptions{Username: "deploy", SessionID: "3", All: true}); err == nil {
		t.Fatal("expected error")
	}
	assertTerminated(t, svc, nil)
}

func TestLogoutWhoSignalsOnlySSHDPts(t *testing.T) {
	svc := serviceWith(func(_ context.Context, name string, args []string) (executil.Result, bool, error) {
		switch name {
		case "loginctl":
			return executil.Result{}, true, errors.New("loginctl not found")
		case "who":
			return executil.Result{Stdout: whoFixture}, true, nil
		case "ps":
			return executil.Result{Stdout: psFixture}, true, nil
		case "kill":
			return executil.Result{}, true, nil
		default:
			return executil.Result{}, true, fmtError("unexpected " + name + " " + strings.Join(args, " "))
		}
	})

	if _, err := svc.Logout(context.Background(), LogoutOptions{Username: "deploy", SessionID: "12345"}); err != nil {
		t.Fatal(err)
	}
	assertKills(t, svc, [][]string{
		{"kill", "-HUP", "4100"},
		{"kill", "-KILL", "4100"},
	})

	svc = serviceWith(func(_ context.Context, name string, _ []string) (executil.Result, bool, error) {
		switch name {
		case "loginctl":
			return executil.Result{}, true, errors.New("loginctl not found")
		case "who":
			return executil.Result{Stdout: whoFixture}, true, nil
		case "ps":
			return executil.Result{Stdout: psFixture}, true, nil
		case "kill":
			return executil.Result{}, true, nil
		default:
			return executil.Result{}, true, nil
		}
	})
	if _, err := svc.Logout(context.Background(), LogoutOptions{Username: "deploy", All: true}); err != nil {
		t.Fatal(err)
	}
	assertKills(t, svc, [][]string{
		{"kill", "-HUP", "4100"},
		{"kill", "-KILL", "4100"},
		{"kill", "-HUP", "4102"},
		{"kill", "-KILL", "4102"},
		{"kill", "-HUP", "4104"},
		{"kill", "-KILL", "4104"},
	})
}

func assertSession(t *testing.T, got Session, id, tty, ip, login, idle string, idleNil bool) {
	t.Helper()
	if got.ID != id || got.TTY != tty || got.SourceIP != ip || got.LoginTime != login {
		t.Fatalf("session = %+v, want id=%s tty=%s ip=%s login=%s", got, id, tty, ip, login)
	}
	if idleNil {
		if got.Idle != nil {
			t.Fatalf("idle = %q, want nil", *got.Idle)
		}
		return
	}
	if got.Idle == nil || *got.Idle != idle {
		t.Fatalf("idle = %v, want %q", got.Idle, idle)
	}
}

func assertTerminated(t *testing.T, svc *Service, ids []string) {
	t.Helper()
	var got []string
	for _, cmd := range svc.runner.Commands() {
		joined := strings.Join(cmd, " ")
		if strings.Contains(joined, "terminate-user") || cmd[0] == "pkill" || cmd[0] == "killall" || cmd[0] == "kill" {
			t.Fatalf("unexpected command %v", cmd)
		}
		if len(cmd) >= 3 && cmd[0] == "loginctl" && cmd[1] == "terminate-session" {
			got = append(got, cmd[2])
		}
	}
	if ids == nil {
		ids = []string{}
	}
	if got == nil {
		got = []string{}
	}
	if !reflect.DeepEqual(got, ids) {
		t.Fatalf("terminated %v, want %v", got, ids)
	}
}

func assertKills(t *testing.T, svc *Service, want [][]string) {
	t.Helper()
	var got [][]string
	for _, cmd := range svc.runner.Commands() {
		joined := strings.Join(cmd, " ")
		if strings.Contains(joined, "terminate-user") || cmd[0] == "pkill" || cmd[0] == "killall" {
			t.Fatalf("unexpected command %v", cmd)
		}
		if cmd[0] == "kill" {
			got = append(got, cmd)
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("kills = %#v, want %#v", got, want)
	}
	forbidden := []string{"4101", "4105", "4200", "5000", "5001", "5002", "5003", "12345", "67890", "22222"}
	for _, cmd := range got {
		for _, arg := range cmd {
			for _, pid := range forbidden {
				if arg == pid {
					t.Fatalf("targeted %s in %v", pid, cmd)
				}
			}
		}
	}
}

func loginctlLogoutService(t *testing.T) *Service {
	t.Helper()
	return serviceWith(func(_ context.Context, name string, args []string) (executil.Result, bool, error) {
		if name == "loginctl" && len(args) > 0 && args[0] == "list-sessions" {
			return executil.Result{Stdout: loginctlFixture}, true, nil
		}
		if name == "loginctl" && len(args) > 0 && args[0] == "terminate-session" {
			return executil.Result{}, true, nil
		}
		return executil.Result{}, true, fmtError("unexpected " + name)
	})
}

func serviceWith(fn executil.CommandFunc) *Service {
	runner := executil.New(false, false)
	runner.SetCommandFunc(fn)
	return &Service{runner: runner}
}

type fmtError string

func (e fmtError) Error() string { return string(e) }
