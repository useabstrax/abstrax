package firewall

import (
	"testing"
)

func TestParseFirewalldRulesKinds(t *testing.T) {
	out := `
public (active)
  services: cockpit dhcpv6-client http https ssh
  ports: 8080/tcp 9090/udp
  rich rules:
`
	rules := parseFirewalldRules(out)
	if len(rules) != 7 {
		t.Fatalf("got %d rules, want 7: %#v", len(rules), rules)
	}
	if rules[0].Kind != RuleKindService || rules[0].Target != "cockpit" || rules[0].ID != "1" {
		t.Fatalf("first rule = %#v", rules[0])
	}
	http := rules[2]
	if http.Kind != RuleKindService || http.Target != "http" {
		t.Fatalf("http rule = %#v", http)
	}
	port := rules[5]
	if port.Kind != RuleKindPort || port.Target != "8080/tcp" || port.ID != "6" {
		t.Fatalf("port rule = %#v", port)
	}
}

func TestParseUFWRulesUnchanged(t *testing.T) {
	out := `[ 1] 22/tcp                     ALLOW IN    Anywhere
[ 2] 80/tcp                     ALLOW IN    Anywhere
`
	rules := parseUFWRules(out)
	if len(rules) != 2 {
		t.Fatalf("got %d rules", len(rules))
	}
	if rules[0].ID != "1" || rules[0].Port != "22/tcp" {
		t.Fatalf("rule0 = %#v", rules[0])
	}
	if rules[0].Kind != "" {
		t.Fatalf("ufw rules should not set firewalld Kind")
	}
}

func TestSplitPortProto(t *testing.T) {
	port, proto := splitPortProto("8080/tcp")
	if port != "8080" || proto != "tcp" {
		t.Fatalf("%s/%s", port, proto)
	}
	port, proto = splitPortProto("53")
	if port != "53" || proto != "tcp" {
		t.Fatalf("%s/%s", port, proto)
	}
}
