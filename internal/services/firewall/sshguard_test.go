package firewall

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"

	executil "abstrax/internal/exec"
)

func TestSSHCheckJSON(t *testing.T) {
	want := `{"ssh_check":{"port":22,"blocked":false}}`
	cases := []any{
		SSHCheckResult{SSHCheck: &SSHCheck{Port: 22, Blocked: false}},
		RuleChangeResult{SSHCheck: &SSHCheck{Port: 22, Blocked: false}},
	}
	for _, v := range cases {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != want {
			t.Fatalf("json = %s", b)
		}
	}
}

func TestSSHReachableRules(t *testing.T) {
	open := Rule{Action: "ALLOW", Direction: "in", Port: "22", Protocol: "tcp"}
	limited := Rule{Action: "ALLOW", Direction: "in", Port: "22", Protocol: "tcp", From: "203.0.113.5"}
	if !sshTCPReachable([]Rule{open, limited}, 22) {
		t.Fatal("two allows should remain reachable")
	}
	if !sshTCPReachable([]Rule{limited}, 22) {
		t.Fatal("an IP-restricted allow should count as access")
	}
	if sshTCPReachable(nil, 22) {
		t.Fatal("no rules should not count as access")
	}

	denied := insertDenyBeforeAllows([]Rule{open}, Rule{Action: "DENY", Direction: "in", Port: "22", Protocol: "tcp"})
	if sshTCPReachable(denied, 22) {
		t.Fatal("a deny placed before the allow should block SSH")
	}
	other := insertDenyBeforeAllows([]Rule{open}, simulatedDenyRule(AllowOptions{Port: "80", Protocol: "tcp"}))
	if !sshTCPReachable(other, 22) {
		t.Fatal("denying another port should leave SSH reachable")
	}
	fromOne := insertDenyBeforeAllows([]Rule{open}, simulatedDenyRule(AllowOptions{Port: "22", Protocol: "tcp", From: "203.0.113.9"}))
	if !sshTCPReachable(fromOne, 22) {
		t.Fatal("a deny from one address should leave other SSH access")
	}

	if !sshTCPReachable([]Rule{{Action: "ALLOW", Port: "ssh", Kind: RuleKindService, Target: "ssh"}}, 22) {
		t.Fatal("firewalld ssh service should count on port 22")
	}
	if sshTCPReachable([]Rule{{Action: "ALLOW", Port: "ssh", Kind: RuleKindService, Target: "ssh"}}, 2222) {
		t.Fatal("firewalld ssh service should not count on a non-default port")
	}
	if !sshTCPReachable([]Rule{{Action: "ALLOW", Direction: "in", Port: "OpenSSH"}}, 22) {
		t.Fatal("UFW OpenSSH profile should count on port 22")
	}
}

func TestUFWSSHGuard(t *testing.T) {
	const only = "[ 1] 22/tcp ALLOW IN Anywhere\n"
	const two = "[ 1] 22/tcp ALLOW IN Anywhere\n[ 2] 22/tcp ALLOW IN 203.0.113.5\n"
	const bothPorts = "[ 1] 22/tcp ALLOW IN Anywhere\n[ 2] 2222/tcp ALLOW IN Anywhere\n"

	t.Run("remove only ssh allow", func(t *testing.T) {
		svc := newUFWGuard(t, true, 22, only)
		id := ufwRuleID(t, only, "22", "")
		err := stderrOK(t, func() error {
			_, err := svc.RuleRemove(context.Background(), id, GuardOptions{})
			return err
		})
		if err == nil || err.Error() != sshBlockedError(22).Error() {
			t.Fatalf("err = %v", err)
		}
		if hasUFWDelete(svc.runner.Commands()) {
			t.Fatalf("deleted despite refusal:\n%s", joinCommands(svc.runner.Commands()))
		}
	})

	t.Run("force removes only ssh allow", func(t *testing.T) {
		svc := newUFWGuard(t, true, 22, only)
		id := ufwRuleID(t, only, "22", "")
		var check *SSHCheck
		err := stderrOK(t, func() error {
			var err error
			check, err = svc.RuleRemove(context.Background(), id, GuardOptions{Force: true})
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if check != nil {
			t.Fatalf("force ran the check: %#v", check)
		}
		if !hasUFWDelete(svc.runner.Commands()) {
			t.Fatalf("not removed:\n%s", joinCommands(svc.runner.Commands()))
		}
	})

	t.Run("remove one of two ssh allows", func(t *testing.T) {
		svc := newUFWGuard(t, true, 22, two)
		id := ufwRuleID(t, two, "22", "")
		var check *SSHCheck
		err := stderrOK(t, func() error {
			var err error
			check, err = svc.RuleRemove(context.Background(), id, GuardOptions{})
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		if check == nil || check.Port != 22 || check.Blocked {
			t.Fatalf("check = %#v", check)
		}
		if !hasUFWDelete(svc.runner.Commands()) {
			t.Fatal("expected the open rule to be removed")
		}
	})

	t.Run("deny ssh while active", func(t *testing.T) {
		svc := newUFWGuard(t, true, 22, only)
		err := stderrOK(t, func() error {
			_, err := svc.Deny(context.Background(), AllowOptions{Port: "22", Protocol: "tcp"})
			return err
		})
		if err == nil || err.Error() != sshBlockedError(22).Error() {
			t.Fatalf("err = %v", err)
		}
		if hasUFWDeny(svc.runner.Commands()) {
			t.Fatalf("denied despite refusal:\n%s", joinCommands(svc.runner.Commands()))
		}
	})

	t.Run("deny another port while active", func(t *testing.T) {
		svc := newUFWGuard(t, true, 22, only)
		result, err := svc.Deny(context.Background(), AllowOptions{Port: "80", Protocol: "tcp"})
		if err != nil {
			t.Fatal(err)
		}
		if result.SSHCheck == nil || result.SSHCheck.Port != 22 || result.SSHCheck.Blocked {
			t.Fatalf("check = %#v", result.SSHCheck)
		}
		if !hasUFWDeny(svc.runner.Commands()) {
			t.Fatal("expected deny 80")
		}
	})

	t.Run("inactive firewall warns and allows", func(t *testing.T) {
		svc := newUFWGuard(t, false, 22, only)
		var result RuleChangeResult
		stderr := captureStderr(t, func() {
			var err error
			result, err = svc.Deny(context.Background(), AllowOptions{Port: "22", Protocol: "tcp"})
			if err != nil {
				t.Fatal(err)
			}
		})
		if !strings.Contains(stderr, "firewall enable will need --allow-ssh") {
			t.Fatalf("stderr = %q", stderr)
		}
		if result.SSHCheck == nil || result.SSHCheck.Port != 22 || !result.SSHCheck.Blocked {
			t.Fatalf("check = %#v", result.SSHCheck)
		}
		if !hasUFWDeny(svc.runner.Commands()) {
			t.Fatal("expected the deny to run")
		}
	})

	t.Run("injected port", func(t *testing.T) {
		svc := newUFWGuard(t, true, 2222, bothPorts)
		id22 := ufwRuleID(t, bothPorts, "22", "")
		if _, err := svc.RuleRemove(context.Background(), id22, GuardOptions{}); err != nil {
			t.Fatal(err)
		}

		svc = newUFWGuard(t, true, 2222, bothPorts)
		id2222 := ufwRuleID(t, bothPorts, "2222", "")
		_, err := svc.RuleRemove(context.Background(), id2222, GuardOptions{})
		if err == nil || err.Error() != sshBlockedError(2222).Error() {
			t.Fatalf("err = %v", err)
		}

		svc = newUFWGuard(t, true, 2222, bothPorts)
		if _, err := svc.Deny(context.Background(), AllowOptions{Port: "22", Protocol: "tcp"}); err != nil {
			t.Fatal(err)
		}
		svc = newUFWGuard(t, true, 2222, bothPorts)
		_, err = svc.Deny(context.Background(), AllowOptions{Port: "2222", Protocol: "tcp"})
		if err == nil || err.Error() != sshBlockedError(2222).Error() {
			t.Fatalf("deny err = %v", err)
		}
	})
}

func TestFirewalldSSHGuard(t *testing.T) {
	const onlySSH = "public (active)\n  services: ssh\n  ports:\n  rich rules:\n"
	const sshAndIP = "public (active)\n  services: ssh\n  ports:\n  rich rules:\n\trule family=\"ipv4\" source address=\"203.0.113.5\" port port=\"22\" protocol=\"tcp\" accept\n"
	const onlyPort = "public (active)\n  services: cockpit\n  ports: 22/tcp\n  rich rules:\n"
	const portAndIP = "public (active)\n  services: cockpit\n  ports: 22/tcp\n  rich rules:\n\trule family=\"ipv4\" source address=\"203.0.113.5\" port port=\"22\" protocol=\"tcp\" accept\n"
	const customPort = "public (active)\n  services: ssh\n  ports: 2222/tcp\n  rich rules:\n"

	t.Run("remove only ssh allow", func(t *testing.T) {
		svc := newFirewalldGuard(t, true, 22, onlySSH)
		id := firewalldRuleID(t, onlySSH, "ssh", "")
		_, err := svc.RuleRemove(context.Background(), id, GuardOptions{})
		if err == nil || err.Error() != sshBlockedError(22).Error() {
			t.Fatalf("err = %v", err)
		}
		if strings.Contains(joinCommands(svc.runner.Commands()), "--remove-service") {
			t.Fatalf("removed despite refusal:\n%s", joinCommands(svc.runner.Commands()))
		}
	})

	t.Run("force removes only ssh allow", func(t *testing.T) {
		svc := newFirewalldGuard(t, true, 22, onlySSH)
		id := firewalldRuleID(t, onlySSH, "ssh", "")
		check, err := svc.RuleRemove(context.Background(), id, GuardOptions{Force: true})
		if err != nil {
			t.Fatal(err)
		}
		if check != nil {
			t.Fatalf("force ran the check: %#v", check)
		}
		if !strings.Contains(joinCommands(svc.runner.Commands()), "--remove-service=ssh") {
			t.Fatalf("commands:\n%s", joinCommands(svc.runner.Commands()))
		}
	})

	t.Run("remove one of two ssh allows", func(t *testing.T) {
		svc := newFirewalldGuard(t, true, 22, sshAndIP)
		id := firewalldRuleID(t, sshAndIP, "ssh", "")
		check, err := svc.RuleRemove(context.Background(), id, GuardOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if check == nil || check.Port != 22 || check.Blocked {
			t.Fatalf("check = %#v", check)
		}
		if !strings.Contains(joinCommands(svc.runner.Commands()), "--remove-service=ssh") {
			t.Fatal("expected ssh service removal")
		}
	})

	t.Run("remove service ssh is the last allow", func(t *testing.T) {
		svc := newFirewalldGuard(t, true, 22, onlySSH)
		_, err := svc.RemoveService(context.Background(), "ssh", GuardOptions{})
		if err == nil || err.Error() != sshBlockedError(22).Error() {
			t.Fatalf("err = %v", err)
		}
		if strings.Contains(joinCommands(svc.runner.Commands()), "--remove-service") {
			t.Fatalf("removed despite refusal:\n%s", joinCommands(svc.runner.Commands()))
		}
	})

	t.Run("remove port is the last allow", func(t *testing.T) {
		svc := newFirewalldGuard(t, true, 22, onlyPort)
		_, err := svc.RemovePort(context.Background(), "22", "tcp", GuardOptions{})
		if err == nil || err.Error() != sshBlockedError(22).Error() {
			t.Fatalf("err = %v", err)
		}
		if strings.Contains(joinCommands(svc.runner.Commands()), "--remove-port") {
			t.Fatalf("removed despite refusal:\n%s", joinCommands(svc.runner.Commands()))
		}
	})

	t.Run("remove port leaves an ip allow", func(t *testing.T) {
		svc := newFirewalldGuard(t, true, 22, portAndIP)
		check, err := svc.RemovePort(context.Background(), "22", "tcp", GuardOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if check == nil || check.Blocked {
			t.Fatalf("check = %#v", check)
		}
		if !strings.Contains(joinCommands(svc.runner.Commands()), "--remove-port=22/tcp") {
			t.Fatalf("commands:\n%s", joinCommands(svc.runner.Commands()))
		}
	})

	t.Run("deny ssh while active", func(t *testing.T) {
		svc := newFirewalldGuard(t, true, 22, onlySSH)
		_, err := svc.Deny(context.Background(), AllowOptions{Port: "22", Protocol: "tcp"})
		if err == nil || err.Error() != sshBlockedError(22).Error() {
			t.Fatalf("err = %v", err)
		}
		if strings.Contains(joinCommands(svc.runner.Commands()), "reject") {
			t.Fatalf("denied despite refusal:\n%s", joinCommands(svc.runner.Commands()))
		}
	})

	t.Run("inactive firewall warns and allows", func(t *testing.T) {
		svc := newFirewalldGuard(t, false, 22, onlySSH)
		var check *SSHCheck
		stderr := captureStderr(t, func() {
			var err error
			check, err = svc.RemoveService(context.Background(), "ssh", GuardOptions{})
			if err != nil {
				t.Fatal(err)
			}
		})
		if !strings.Contains(stderr, "firewall enable will need --allow-ssh") {
			t.Fatalf("stderr = %q", stderr)
		}
		if check == nil || check.Port != 22 || !check.Blocked {
			t.Fatalf("check = %#v", check)
		}
		if !strings.Contains(joinCommands(svc.runner.Commands()), "--remove-service=ssh") {
			t.Fatal("expected removal")
		}
	})

	t.Run("injected port", func(t *testing.T) {
		svc := newFirewalldGuard(t, true, 2222, customPort)
		if _, err := svc.RemoveService(context.Background(), "ssh", GuardOptions{}); err != nil {
			t.Fatal(err)
		}

		svc = newFirewalldGuard(t, true, 2222, customPort)
		_, err := svc.RemovePort(context.Background(), "2222", "tcp", GuardOptions{})
		if err == nil || err.Error() != sshBlockedError(2222).Error() {
			t.Fatalf("err = %v", err)
		}

		svc = newFirewalldGuard(t, true, 2222, customPort)
		result, err := svc.Deny(context.Background(), AllowOptions{Port: "22", Protocol: "tcp"})
		if err != nil {
			t.Fatal(err)
		}
		if result.SSHCheck == nil || result.SSHCheck.Port != 2222 || result.SSHCheck.Blocked {
			t.Fatalf("check = %#v", result.SSHCheck)
		}

		svc = newFirewalldGuard(t, true, 2222, customPort)
		_, err = svc.Deny(context.Background(), AllowOptions{Port: "2222", Protocol: "tcp"})
		if err == nil || err.Error() != sshBlockedError(2222).Error() {
			t.Fatalf("deny err = %v", err)
		}
	})
}

func newUFWGuard(t *testing.T, active bool, port int, rules string) *Service {
	t.Helper()
	state := "inactive"
	if active {
		state = "active"
	}
	output := "Status: " + state + "\n\n" + rules
	return newGuardSvc(t, "ufw", port, func(_ context.Context, name string, args []string) (executil.Result, bool, error) {
		if name == "ufw" && len(args) >= 1 && args[0] == "status" {
			return executil.Result{Stdout: output}, true, nil
		}
		return executil.Result{}, false, nil
	})
}

func newFirewalldGuard(t *testing.T, active bool, port int, list string) *Service {
	t.Helper()
	return newGuardSvc(t, "firewalld", port, func(_ context.Context, name string, args []string) (executil.Result, bool, error) {
		if name != "firewall-cmd" || len(args) != 1 {
			return executil.Result{}, false, nil
		}
		switch args[0] {
		case "--state":
			if active {
				return executil.Result{Stdout: "running"}, true, nil
			}
			// firewall-cmd exits non-zero when it is not running. "not running"
			// also contains the substring "running", so the error is what marks it inactive.
			return executil.Result{Stdout: "not running", ExitCode: 252}, true, fmt.Errorf("exit status 252")
		case "--list-all":
			return executil.Result{Stdout: list}, true, nil
		default:
			return executil.Result{}, false, nil
		}
	})
}

func newGuardSvc(t *testing.T, backend string, port int, fn executil.CommandFunc) *Service {
	t.Helper()
	t.Setenv("SSH_CONNECTION", "")
	t.Setenv("SSH_CLIENT", "")
	t.Setenv("SUDO_USER", "")
	runner := executil.New(true, false)
	runner.SetCommandFunc(fn)
	return &Service{
		runner:  runner,
		backend: backend,
		sshPort: func() (int, error) { return port, nil },
	}
}

func ufwRuleID(t *testing.T, rules, port, from string) string {
	t.Helper()
	for _, r := range parseUFWRules(rules) {
		if r.Port == port && r.From == from {
			return r.ID
		}
	}
	t.Fatalf("no ufw rule port %q from %q", port, from)
	return ""
}

func firewalldRuleID(t *testing.T, list, port, from string) string {
	t.Helper()
	for _, r := range parseFirewalldRules(list) {
		if r.Port == port && r.From == from {
			return r.ID
		}
	}
	t.Fatalf("no firewalld rule port %q from %q", port, from)
	return ""
}

func hasUFWDelete(cmds [][]string) bool {
	for _, c := range cmds {
		if len(c) >= 3 && c[0] == "ufw" && c[1] == "--force" && c[2] == "delete" {
			return true
		}
	}
	return false
}

func hasUFWDeny(cmds [][]string) bool {
	for _, c := range cmds {
		if len(c) >= 2 && c[0] == "ufw" && c[1] == "deny" {
			return true
		}
	}
	return false
}

func stderrOK(t *testing.T, fn func() error) error {
	t.Helper()
	var err error
	stderr := captureStderr(t, func() {
		err = fn()
	})
	if strings.Contains(stderr, "firewall enable will need --allow-ssh") {
		t.Fatalf("unexpected inactive warning: %s", stderr)
	}
	return err
}

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stderr
	os.Stderr = w
	defer func() { os.Stderr = old }()

	fn()
	_ = w.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatal(err)
	}
	return buf.String()
}
