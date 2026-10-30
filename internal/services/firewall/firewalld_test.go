package firewall

import (
	"context"
	"strings"
	"testing"

	executil "abstrax/internal/exec"
)

const firewalldListAll = `public (active)
  services: cockpit dhcpv6-client ssh
  ports: 8080/tcp 9090/udp
  rich rules:
	rule family="ipv4" source address="10.0.0.12" port port="3306" protocol="tcp" accept
	rule family="ipv6" source address="2001:db8::1" port port="3306" protocol="tcp" accept
	rule family="ipv4" port port="25" protocol="tcp" reject
	rule family="ipv4" source address="203.0.113.5" reject
	rule family="ipv6" source address="2001:db8::/64" drop
`

func TestParseFirewalldRichRules(t *testing.T) {
	rules := parseFirewalldRules(firewalldListAll)
	if len(rules) != 10 {
		t.Fatalf("got %d rules, want 10: %#v", len(rules), rules)
	}

	mysql := findRule(t, rules, "10.0.0.12", "3306")
	if mysql.Kind != RuleKindRich || mysql.Action != "ALLOW" || mysql.Protocol != "tcp" || mysql.IPv6 {
		t.Fatalf("mysql = %#v", mysql)
	}
	v6 := findRule(t, rules, "2001:db8::1", "3306")
	if !v6.IPv6 || v6.Action != "ALLOW" {
		t.Fatalf("v6 = %#v", v6)
	}
	smtp := findRule(t, rules, "", "25")
	if smtp.Action != "DENY" || smtp.From != "" {
		t.Fatalf("smtp = %#v", smtp)
	}
	rejectIP := findRule(t, rules, "203.0.113.5", "")
	if rejectIP.Action != "DENY" || rejectIP.Port != "" {
		t.Fatalf("reject ip = %#v", rejectIP)
	}
	drop := findRule(t, rules, "2001:db8::/64", "")
	if drop.Action != "DENY" || !drop.IPv6 {
		t.Fatalf("drop = %#v", drop)
	}

	if mysql.ID == v6.ID || len(mysql.ID) != 8 {
		t.Fatalf("ids = %s %s", mysql.ID, v6.ID)
	}
}

func TestFirewalldStableIDIgnoresPosition(t *testing.T) {
	withExtra := parseFirewalldRules(firewalldListAll)
	without := parseFirewalldRules(`public (active)
  services: ssh
  rich rules:
	rule family="ipv4" source address="10.0.0.12" port port="3306" protocol="tcp" accept
`)
	var before, after Rule
	for _, r := range withExtra {
		if r.From == "10.0.0.12" && r.Port == "3306" {
			before = r
		}
	}
	for _, r := range without {
		if r.From == "10.0.0.12" && r.Port == "3306" {
			after = r
		}
	}
	if before.ID == "" || before.ID != after.ID {
		t.Fatalf("id changed from %s (%s) to %s (%s)", before.ID, before.Number, after.ID, after.Number)
	}
	if before.Number == after.Number {
		t.Fatalf("expected position to change")
	}
}

func TestFirewalldAllowFromUsesRichRule(t *testing.T) {
	runner := executil.New(true, false)
	svc := &Service{runner: runner, backend: "firewalld"}
	err := svc.addRule(context.Background(), "allow", AllowOptions{Port: "3306", From: "10.0.0.12"})
	if err != nil {
		t.Fatal(err)
	}
	joined := joinCommands(runner.Commands())
	want := `firewall-cmd --permanent --add-rich-rule=rule family="ipv4" source address="10.0.0.12" port port="3306" protocol="tcp" accept`
	if !strings.Contains(joined, want) {
		t.Fatalf("commands:\n%s", joined)
	}
	if strings.Contains(joined, "--add-port") || strings.Contains(joined, "--add-service") {
		t.Fatalf("opened the port to everyone:\n%s", joined)
	}
}

func TestFirewalldAllowHTTPSFromDoesNotAddService(t *testing.T) {
	runner := executil.New(true, false)
	svc := &Service{runner: runner, backend: "firewalld"}
	err := svc.addRule(context.Background(), "allow", AllowOptions{Port: "443", From: "10.0.0.12"})
	if err != nil {
		t.Fatal(err)
	}
	joined := joinCommands(runner.Commands())
	want := `rule family="ipv4" source address="10.0.0.12" port port="443" protocol="tcp" accept`
	if !strings.Contains(joined, want) {
		t.Fatalf("commands:\n%s", joined)
	}
	if strings.Contains(joined, "--add-service") || strings.Contains(joined, "--add-port") {
		t.Fatalf("opened https to everyone:\n%s", joined)
	}
}

func TestFirewalldDenyPortBothFamilies(t *testing.T) {
	runner := executil.New(true, false)
	svc := &Service{runner: runner, backend: "firewalld"}
	if err := svc.addRule(context.Background(), "deny", AllowOptions{Port: "25"}); err != nil {
		t.Fatal(err)
	}
	joined := joinCommands(runner.Commands())
	if !strings.Contains(joined, `rule family="ipv4" port port="25" protocol="tcp" reject`) {
		t.Fatalf("missing ipv4 reject:\n%s", joined)
	}
	if !strings.Contains(joined, `rule family="ipv6" port port="25" protocol="tcp" reject`) {
		t.Fatalf("missing ipv6 reject:\n%s", joined)
	}
}

func TestFirewalldAllowIPv6Source(t *testing.T) {
	runner := executil.New(true, false)
	svc := &Service{runner: runner, backend: "firewalld"}
	err := svc.firewalldAllowIP(context.Background(), AllowOptions{From: "2001:db8::1", Port: "22"})
	if err != nil {
		t.Fatal(err)
	}
	joined := joinCommands(runner.Commands())
	want := `rule family="ipv6" source address="2001:db8::1" port port="22" protocol="tcp" accept`
	if !strings.Contains(joined, want) {
		t.Fatalf("commands:\n%s", joined)
	}
}

func TestFirewalldRemoveRichRuleByID(t *testing.T) {
	rules := parseFirewalldRules(firewalldListAll)
	var mysql Rule
	for _, r := range rules {
		if r.From == "10.0.0.12" && r.Port == "3306" {
			mysql = r
		}
	}

	runner := executil.New(true, false)
	runner.SetCommandFunc(func(_ context.Context, name string, args []string) (executil.Result, bool, error) {
		if name == "firewall-cmd" && len(args) == 1 && args[0] == "--list-all" {
			return executil.Result{Stdout: firewalldListAll}, true, nil
		}
		if name == "firewall-cmd" && len(args) == 1 && args[0] == "--state" {
			return executil.Result{Stdout: "running"}, true, nil
		}
		return executil.Result{}, false, nil
	})
	svc := &Service{runner: runner, backend: "firewalld"}
	if err := svc.RuleRemove(context.Background(), mysql.ID); err != nil {
		t.Fatal(err)
	}
	joined := joinCommands(runner.Commands())
	want := `--remove-rich-rule=rule family="ipv4" source address="10.0.0.12" port port="3306" protocol="tcp" accept`
	if !strings.Contains(joined, want) {
		t.Fatalf("commands:\n%s", joined)
	}
}

func findRule(t *testing.T, rules []Rule, from, port string) Rule {
	t.Helper()
	for _, r := range rules {
		if r.From == from && r.Port == port {
			return r
		}
	}
	t.Fatalf("no rule from %q port %q", from, port)
	return Rule{}
}

func joinCommands(cmds [][]string) string {
	lines := make([]string, len(cmds))
	for i, cmd := range cmds {
		lines[i] = strings.Join(cmd, " ")
	}
	return strings.Join(lines, "\n")
}
