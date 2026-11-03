package firewall

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	executil "abstrax/internal/exec"
)

func TestParseUFWDefaultsFixtures(t *testing.T) {
	cases := []struct {
		name string
		out  string
		want Defaults
		ok   bool
	}{
		{
			name: "deny allow disabled",
			out:  "Status: active\nLogging: on (low)\nDefault: deny (incoming), allow (outgoing), disabled (routed)\nNew profiles: skip\n",
			want: Defaults{Incoming: "deny", Outgoing: "allow", Routed: "disabled"},
			ok:   true,
		},
		{
			name: "allow incoming reject outgoing",
			out:  "Status: active\nDefault: allow (incoming), reject (outgoing), disabled (routed)\n",
			want: Defaults{Incoming: "allow", Outgoing: "reject", Routed: "disabled"},
			ok:   true,
		},
		{
			name: "routed allowed",
			out:  "Status: active\nDefault: reject (incoming), deny (outgoing), allow (routed)\n",
			want: Defaults{Incoming: "reject", Outgoing: "deny", Routed: "allow"},
			ok:   true,
		},
		{
			name: "no routed",
			out:  "Status: active\nDefault: deny (incoming), allow (outgoing)\n",
			want: Defaults{Incoming: "deny", Outgoing: "allow"},
			ok:   true,
		},
		{
			name: "inactive has no default line",
			out:  "Status: inactive\n",
			ok:   false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseUFWDefaults(tc.out)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if ok && got != tc.want {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestUFWDefaultsInactiveFallsBackToFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "ufw")
	writeFile(t, file, "# comment\nIPV6=yes\nDEFAULT_INPUT_POLICY=\"DROP\"\nDEFAULT_OUTPUT_POLICY=\"ACCEPT\"\nDEFAULT_FORWARD_POLICY=\"DROP\"\n")
	svc := newDefaultsSvc(t, "ufw", func(_ context.Context, name string, args []string) (executil.Result, bool, error) {
		if name == "ufw" && len(args) == 2 && args[0] == "status" && args[1] == "verbose" {
			return executil.Result{Stdout: "Status: inactive"}, true, nil
		}
		return executil.Result{}, false, nil
	})
	svc.ufwDefaultFile = file

	got, err := svc.Defaults(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := Defaults{Incoming: "deny", Outgoing: "allow", Routed: "disabled", Ping: PingUnknown}
	if *got != want {
		t.Fatalf("got %#v, want %#v", *got, want)
	}
}

func TestUFWDefaultsIncludePing(t *testing.T) {
	svc := newUFWPingSvc(t, ufwBeforeAccept, ufwBefore6Accept)
	svc.runner.SetCommandFunc(ufwVerbose("deny", "allow"))
	got, err := svc.Defaults(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := Defaults{Incoming: "deny", Outgoing: "allow", Routed: "disabled", Ping: PingAllow}
	if *got != want {
		t.Fatalf("got %#v, want %#v", *got, want)
	}

	data, err := json.Marshal(Status{Active: true, Backend: "ufw", Defaults: got})
	if err != nil {
		t.Fatal(err)
	}
	wantJSON := `{"active":true,"backend":"ufw","defaults":{"incoming":"deny","outgoing":"allow","routed":"disabled","ping":"allow"}}`
	if string(data) != wantJSON {
		t.Fatalf("json = %s", data)
	}
}

func TestFirewalldTargetMapping(t *testing.T) {
	cases := map[string]string{
		"default":    PolicyReject,
		"REJECT":     PolicyReject,
		"%%REJECT%%": PolicyReject,
		"DROP":       PolicyDeny,
		"ACCEPT":     PolicyAllow,
		" drop\n":    PolicyDeny,
	}
	for target, want := range cases {
		got, err := firewalldTargetPolicy(target)
		if err != nil || got != want {
			t.Fatalf("%q -> %q, %v; want %q", target, got, err, want)
		}
	}
	if _, err := firewalldTargetPolicy("bogus"); err == nil {
		t.Fatal("expected an error for an unknown target")
	}
}

func TestFirewalldDefaultsRead(t *testing.T) {
	for _, tc := range []struct {
		target, ping string
		queryErr     bool
		want         Defaults
	}{
		{"%%REJECT%%", "no", true, Defaults{Incoming: "reject", Outgoing: "allow", Ping: PingAllow}},
		{"DROP", "yes", false, Defaults{Incoming: "deny", Outgoing: "allow", Ping: PingBlock}},
		{"ACCEPT", "no", true, Defaults{Incoming: "allow", Outgoing: "allow", Ping: PingAllow}},
	} {
		svc := newDefaultsSvc(t, "firewalld", firewalldFunc("public", tc.target, tc.ping, tc.queryErr))
		got, err := svc.Defaults(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if *got != tc.want {
			t.Fatalf("target %s: got %#v, want %#v", tc.target, *got, tc.want)
		}
		joined := joinCommands(svc.runner.Commands())
		if !strings.Contains(joined, "firewall-cmd --permanent --zone=public --get-target") ||
			!strings.Contains(joined, "firewall-cmd --zone=public --query-icmp-block=echo-request") {
			t.Fatalf("commands:\n%s", joined)
		}
	}
}

func TestFirewalldSetIncoming(t *testing.T) {
	for policy, target := range map[string]string{
		"deny":   "DROP",
		"reject": "%%REJECT%%",
		"allow":  "ACCEPT",
	} {
		svc := newDefaultsSvc(t, "firewalld", firewalldFunc("work", "ACCEPT", "no", true))
		if _, err := svc.SetDefaults(context.Background(), DefaultsSetOptions{Incoming: policy, Force: true}); err != nil {
			t.Fatalf("%s: %v", policy, err)
		}
		joined := joinCommands(svc.runner.Commands())
		want := "firewall-cmd --permanent --zone=work --set-target=" + target
		if !strings.Contains(joined, want) {
			t.Fatalf("%s: commands:\n%s", policy, joined)
		}
		if !strings.Contains(joined, "firewall-cmd --reload") {
			t.Fatalf("%s: no reload:\n%s", policy, joined)
		}
	}
}

func TestFirewalldOutgoingNotSupported(t *testing.T) {
	svc := newDefaultsSvc(t, "firewalld", firewalldFunc("public", "DROP", "no", true))
	_, err := svc.SetDefaults(context.Background(), DefaultsSetOptions{Incoming: "deny", Outgoing: "deny"})
	if err == nil || !strings.Contains(err.Error(), "not supported on firewalld") {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(joinCommands(svc.runner.Commands()), "--set-target") {
		t.Fatal("changed the target despite refusing")
	}
}

func TestSetDefaultsValidation(t *testing.T) {
	svc := newDefaultsSvc(t, "ufw", ufwVerbose("deny", "allow"))
	for _, opts := range []DefaultsSetOptions{
		{Incoming: "drop"},
		{Incoming: "deny", Outgoing: "maybe"},
		{},
	} {
		if _, err := svc.SetDefaults(context.Background(), opts); err == nil {
			t.Fatalf("expected an error for %#v", opts)
		}
	}
	if len(svc.runner.Commands()) != 0 {
		t.Fatalf("ran commands:\n%s", joinCommands(svc.runner.Commands()))
	}
}

func TestUFWSetDefaultsCommands(t *testing.T) {
	svc := newDefaultsSvc(t, "ufw", ufwVerbose("allow", "allow"))
	svc.ufwBeforeRules = filepath.Join(t.TempDir(), "missing")
	_, err := svc.SetDefaults(context.Background(), DefaultsSetOptions{Incoming: "reject", Outgoing: "deny", Force: true})
	if err != nil {
		t.Fatal(err)
	}
	joined := joinCommands(svc.runner.Commands())
	for _, want := range []string{"ufw default reject incoming", "ufw default deny outgoing"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q:\n%s", want, joined)
		}
	}
}

func TestSetDefaultsDryRunReportsRequestedPolicy(t *testing.T) {
	svc := newDefaultsSvc(t, "ufw", ufwVerbose("allow", "allow"))
	svc.dryRun = true
	result, err := svc.SetDefaults(context.Background(), DefaultsSetOptions{Incoming: "deny", Outgoing: "reject", Force: true})
	if err != nil {
		t.Fatal(err)
	}
	if result.Defaults.Incoming != "deny" || result.Defaults.Outgoing != "reject" {
		t.Fatalf("defaults = %#v", result.Defaults)
	}
}

func TestSetDefaultsSSHSafety(t *testing.T) {
	const noSSH = "[ 1] 80/tcp ALLOW IN Anywhere\n"
	const withSSH = "[ 1] 22/tcp ALLOW IN Anywhere\n"

	build := func(t *testing.T, active bool, current, rules string) *Service {
		t.Helper()
		state := "inactive"
		if active {
			state = "active"
		}
		svc := newGuardSvc(t, "ufw", 22, func(_ context.Context, name string, args []string) (executil.Result, bool, error) {
			if name != "ufw" || len(args) < 2 || args[0] != "status" {
				return executil.Result{}, false, nil
			}
			if args[1] == "verbose" {
				return executil.Result{Stdout: fmt.Sprintf("Status: %s\nDefault: %s (incoming), allow (outgoing), disabled (routed)\n", state, current)}, true, nil
			}
			return executil.Result{Stdout: "Status: " + state + "\n\n" + rules}, true, nil
		})
		svc.ufwBeforeRules = filepath.Join(t.TempDir(), "missing")
		return svc
	}
	hasDefault := func(svc *Service, policy string) bool {
		return strings.Contains(joinCommands(svc.runner.Commands()), "ufw default "+policy+" incoming")
	}

	for _, policy := range []string{"deny", "reject"} {
		t.Run(policy+" without ssh rule is refused", func(t *testing.T) {
			svc := build(t, true, "allow", noSSH)
			_, err := svc.SetDefaults(context.Background(), DefaultsSetOptions{Incoming: policy})
			if err == nil || err.Error() != sshBlockedError(22).Error() {
				t.Fatalf("err = %v", err)
			}
			if hasDefault(svc, policy) {
				t.Fatal("applied the policy despite refusing")
			}
		})

		t.Run(policy+" without ssh rule with force", func(t *testing.T) {
			svc := build(t, true, "allow", noSSH)
			result, err := svc.SetDefaults(context.Background(), DefaultsSetOptions{Incoming: policy, Force: true})
			if err != nil {
				t.Fatal(err)
			}
			if result.SSHCheck != nil {
				t.Fatalf("force ran the check: %#v", result.SSHCheck)
			}
			if !hasDefault(svc, policy) {
				t.Fatal("policy not applied")
			}
		})
	}

	t.Run("deny with ssh rule", func(t *testing.T) {
		svc := build(t, true, "allow", withSSH)
		result, err := svc.SetDefaults(context.Background(), DefaultsSetOptions{Incoming: "deny"})
		if err != nil {
			t.Fatal(err)
		}
		if result.SSHCheck == nil || result.SSHCheck.Port != 22 || result.SSHCheck.Blocked {
			t.Fatalf("check = %#v", result.SSHCheck)
		}
		if !hasDefault(svc, "deny") {
			t.Fatal("policy not applied")
		}
	})

	t.Run("already blocking skips the check", func(t *testing.T) {
		svc := build(t, true, "deny", noSSH)
		result, err := svc.SetDefaults(context.Background(), DefaultsSetOptions{Incoming: "reject"})
		if err != nil {
			t.Fatal(err)
		}
		if result.SSHCheck != nil || !hasDefault(svc, "reject") {
			t.Fatalf("result = %#v", result)
		}
	})

	t.Run("inactive firewall warns and allows", func(t *testing.T) {
		svc := build(t, false, "allow", noSSH)
		stderr := captureStderr(t, func() {
			if _, err := svc.SetDefaults(context.Background(), DefaultsSetOptions{Incoming: "deny"}); err != nil {
				t.Fatal(err)
			}
		})
		if !strings.Contains(stderr, "firewall enable will need --allow-ssh") {
			t.Fatalf("stderr = %q", stderr)
		}
	})

	t.Run("allow needs no ssh rule", func(t *testing.T) {
		svc := build(t, true, "deny", noSSH)
		if _, err := svc.SetDefaults(context.Background(), DefaultsSetOptions{Incoming: "allow"}); err != nil {
			t.Fatal(err)
		}
		if !hasDefault(svc, "allow") {
			t.Fatal("policy not applied")
		}
	})
}

// UFW ping.

const ufwBeforeAccept = `# rules.before
*filter
:ufw-before-input - [0:0]
-A ufw-before-input -i lo -j ACCEPT
# ok icmp codes for INPUT
-A ufw-before-input -p icmp --icmp-type destination-unreachable -j ACCEPT
-A ufw-before-input -p icmp --icmp-type time-exceeded -j ACCEPT
-A ufw-before-input -p icmp --icmp-type parameter-problem -j ACCEPT
-A ufw-before-input -p icmp --icmp-type echo-request -j ACCEPT

# ok icmp code for FORWARD
-A ufw-before-forward -p icmp --icmp-type echo-request -j ACCEPT
COMMIT
`

const ufwBefore6Accept = `# rules.before
*filter
-A ufw6-before-input -p icmpv6 --icmp-type destination-unreachable -j ACCEPT
-A ufw6-before-input -p icmpv6 --icmp-type echo-request -j ACCEPT
-A ufw6-before-forward -p icmpv6 --icmp-type echo-request -j ACCEPT
COMMIT
`

func TestUFWPingRoundTrip(t *testing.T) {
	svc := newUFWPingSvc(t, ufwBeforeAccept, ufwBefore6Accept)
	ctx := context.Background()

	if got := svc.ufwPing(); got != PingAllow {
		t.Fatalf("initial ping = %s", got)
	}

	if _, err := svc.SetPing(ctx, "block"); err != nil {
		t.Fatal(err)
	}
	if got := svc.ufwPing(); got != PingBlock {
		t.Fatalf("after block = %s", got)
	}
	v4 := readFile(t, svc.ufwBeforeRules)
	if !strings.Contains(v4, "-A ufw-before-input -p icmp --icmp-type echo-request -j DROP") {
		t.Fatalf("before.rules:\n%s", v4)
	}
	if !strings.Contains(v4, "-A ufw-before-forward -p icmp --icmp-type echo-request -j ACCEPT") {
		t.Fatalf("forward rule changed:\n%s", v4)
	}
	if !strings.Contains(v4, "destination-unreachable -j ACCEPT") {
		t.Fatalf("other ICMP rules changed:\n%s", v4)
	}
	v6 := readFile(t, svc.ufwBefore6Rules)
	if !strings.Contains(v6, "-A ufw6-before-input -p icmpv6 --icmp-type echo-request -j DROP") {
		t.Fatalf("before6.rules:\n%s", v6)
	}
	if got := readFile(t, svc.ufwBeforeRules+".bak"); got != ufwBeforeAccept {
		t.Fatalf("before.rules.bak differs:\n%s", got)
	}
	if got := readFile(t, svc.ufwBefore6Rules+".bak"); got != ufwBefore6Accept {
		t.Fatalf("before6.rules.bak differs:\n%s", got)
	}
	if !strings.Contains(joinCommands(svc.runner.Commands()), "ufw reload") {
		t.Fatal("ufw was not reloaded")
	}

	if _, err := svc.SetPing(ctx, "allow"); err != nil {
		t.Fatal(err)
	}
	if got := svc.ufwPing(); got != PingAllow {
		t.Fatalf("after allow = %s", got)
	}
	if got := readFile(t, svc.ufwBeforeRules); got != ufwBeforeAccept {
		t.Fatalf("round trip changed before.rules:\n%s", got)
	}
	if got := readFile(t, svc.ufwBefore6Rules); got != ufwBefore6Accept {
		t.Fatalf("round trip changed before6.rules:\n%s", got)
	}
}

func TestUFWPingKeepsFileMode(t *testing.T) {
	svc := newUFWPingSvc(t, ufwBeforeAccept, ufwBefore6Accept)
	if err := os.Chmod(svc.ufwBeforeRules, 0o640); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetPing(context.Background(), "block"); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(svc.ufwBeforeRules)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Fatalf("mode = %v", info.Mode().Perm())
	}
}

func TestUFWPingNoChangeLeavesFiles(t *testing.T) {
	svc := newUFWPingSvc(t, ufwBeforeAccept, ufwBefore6Accept)
	if _, err := svc.SetPing(context.Background(), "allow"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(svc.ufwBeforeRules + ".bak"); !os.IsNotExist(err) {
		t.Fatalf("backup written for a no-op: %v", err)
	}
}

func TestUFWPingMissingBefore6IsIgnored(t *testing.T) {
	svc := newUFWPingSvc(t, ufwBeforeAccept, ufwBefore6Accept)
	if err := os.Remove(svc.ufwBefore6Rules); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SetPing(context.Background(), "block"); err != nil {
		t.Fatal(err)
	}
	if got := svc.ufwPing(); got != PingBlock {
		t.Fatalf("ping = %s", got)
	}
}

func TestUFWPingMixedAndUnknown(t *testing.T) {
	svc := newUFWPingSvc(t, ufwBeforeAccept, strings.ReplaceAll(ufwBefore6Accept, "echo-request -j ACCEPT", "echo-request -j DROP"))
	if got := svc.ufwPing(); got != PingMixed {
		t.Fatalf("ping = %s", got)
	}
	svc = newUFWPingSvc(t, "*filter\nCOMMIT\n", "")
	if got := svc.ufwPing(); got != PingUnknown {
		t.Fatalf("ping = %s", got)
	}
	if _, err := svc.SetPing(context.Background(), "block"); err == nil {
		t.Fatal("expected an error when no echo-request rule exists")
	}
}

func TestUFWPingRestoresFilesWhenReloadFails(t *testing.T) {
	svc := newUFWPingSvc(t, ufwBeforeAccept, ufwBefore6Accept)
	svc.runner.SetCommandFunc(func(_ context.Context, name string, args []string) (executil.Result, bool, error) {
		if name == "ufw" && len(args) == 1 && args[0] == "reload" {
			return executil.Result{}, true, fmt.Errorf("reload failed")
		}
		return executil.Result{}, false, nil
	})
	if _, err := svc.SetPing(context.Background(), "block"); err == nil {
		t.Fatal("expected an error")
	}
	if got := readFile(t, svc.ufwBeforeRules); got != ufwBeforeAccept {
		t.Fatalf("before.rules not restored:\n%s", got)
	}
	if got := readFile(t, svc.ufwBefore6Rules); got != ufwBefore6Accept {
		t.Fatalf("before6.rules not restored:\n%s", got)
	}
}

func TestUFWPingDryRunDoesNotWrite(t *testing.T) {
	svc := newUFWPingSvc(t, ufwBeforeAccept, ufwBefore6Accept)
	svc.dryRun = true
	stdout := captureStdout(t, func() {
		if _, err := svc.SetPing(context.Background(), "block"); err != nil {
			t.Fatal(err)
		}
	})
	if !strings.Contains(stdout, "would set echo-request to DROP") {
		t.Fatalf("stdout = %q", stdout)
	}
	if got := readFile(t, svc.ufwBeforeRules); got != ufwBeforeAccept {
		t.Fatalf("dry run wrote before.rules:\n%s", got)
	}
}

func TestSetPingRejectsBadMode(t *testing.T) {
	svc := newUFWPingSvc(t, ufwBeforeAccept, ufwBefore6Accept)
	if _, err := svc.SetPing(context.Background(), "maybe"); err == nil {
		t.Fatal("expected an error")
	}
}

// firewalld ping.

func TestFirewalldSetPing(t *testing.T) {
	for mode, flag := range map[string]string{
		"block": "--add-icmp-block=echo-request",
		"allow": "--remove-icmp-block=echo-request",
	} {
		svc := newDefaultsSvc(t, "firewalld", firewalldFunc("public", "DROP", "no", true))
		result, err := svc.SetPing(context.Background(), mode)
		if err != nil {
			t.Fatal(err)
		}
		if result.Ping != mode || result.Backend != "firewalld" {
			t.Fatalf("result = %#v", result)
		}
		joined := joinCommands(svc.runner.Commands())
		want := "firewall-cmd --permanent --zone=public " + flag
		if !strings.Contains(joined, want) || !strings.Contains(joined, "firewall-cmd --reload") {
			t.Fatalf("%s: commands:\n%s", mode, joined)
		}
	}
}

func TestStatusWithDefaultsLeavesDefaultsUnsetOnFailure(t *testing.T) {
	svc := newDefaultsSvc(t, "ufw", func(_ context.Context, name string, args []string) (executil.Result, bool, error) {
		if name != "ufw" || len(args) < 2 || args[0] != "status" {
			return executil.Result{}, false, nil
		}
		if args[1] == "verbose" {
			return executil.Result{}, true, fmt.Errorf("boom")
		}
		return executil.Result{Stdout: "Status: active\n\n[ 1] 22/tcp ALLOW IN Anywhere\n"}, true, nil
	})
	status, err := svc.StatusWithDefaults(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !status.Active || status.Defaults != nil || len(status.Rules) != 1 {
		t.Fatalf("status = %#v", status)
	}
}

func TestDefaultsResultJSON(t *testing.T) {
	data, err := json.Marshal(DefaultsResult{
		Backend:  "ufw",
		Defaults: &Defaults{Incoming: "deny", Outgoing: "allow", Routed: "disabled", Ping: "block"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := `{"backend":"ufw","defaults":{"incoming":"deny","outgoing":"allow","routed":"disabled","ping":"block"}}`
	if string(data) != want {
		t.Fatalf("json = %s", data)
	}
}

// helpers

func newDefaultsSvc(t *testing.T, backend string, fn executil.CommandFunc) *Service {
	t.Helper()
	return newGuardSvc(t, backend, 22, fn)
}

func newUFWPingSvc(t *testing.T, before, before6 string) *Service {
	t.Helper()
	dir := t.TempDir()
	svc := newDefaultsSvc(t, "ufw", nil)
	svc.ufwBeforeRules = filepath.Join(dir, "before.rules")
	svc.ufwBefore6Rules = filepath.Join(dir, "before6.rules")
	writeFile(t, svc.ufwBeforeRules, before)
	writeFile(t, svc.ufwBefore6Rules, before6)
	return svc
}

func ufwVerbose(incoming, outgoing string) executil.CommandFunc {
	return func(_ context.Context, name string, args []string) (executil.Result, bool, error) {
		if name == "ufw" && len(args) == 2 && args[0] == "status" && args[1] == "verbose" {
			out := fmt.Sprintf("Status: active\nDefault: %s (incoming), %s (outgoing), disabled (routed)\n", incoming, outgoing)
			return executil.Result{Stdout: out}, true, nil
		}
		return executil.Result{}, false, nil
	}
}

// firewalldFunc answers the firewall-cmd queries used for defaults. ping is the
// --query-icmp-block answer. firewall-cmd exits 1 for "no", queryErr mimics that.
func firewalldFunc(zone, target, ping string, queryErr bool) executil.CommandFunc {
	return func(_ context.Context, name string, args []string) (executil.Result, bool, error) {
		if name != "firewall-cmd" {
			return executil.Result{}, false, nil
		}
		joined := strings.Join(args, " ")
		switch {
		case joined == "--get-default-zone":
			return executil.Result{Stdout: zone}, true, nil
		case joined == "--permanent --zone="+zone+" --get-target":
			return executil.Result{Stdout: target}, true, nil
		case joined == "--zone="+zone+" --query-icmp-block=echo-request":
			if queryErr {
				return executil.Result{Stdout: ping, ExitCode: 1}, true, fmt.Errorf("exit status 1")
			}
			return executil.Result{Stdout: ping}, true, nil
		}
		return executil.Result{}, false, nil
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	old := os.Stdout
	os.Stdout = w
	fn()
	os.Stdout = old
	_ = w.Close()
	data, err := readAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func readAll(r *os.File) (string, error) {
	var sb strings.Builder
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		sb.Write(buf[:n])
		if err != nil {
			break
		}
	}
	return sb.String(), nil
}
