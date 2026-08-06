package platform

import "testing"

func TestFirewallStrategyDefaultsWhenBackendMissing(t *testing.T) {
	if got := firewallStrategy("none", "debian"); got != "ufw" {
		t.Fatalf("debian without ufw: got %q, want ufw", got)
	}
	if got := firewallStrategy("iptables", "debian"); got != "ufw" {
		t.Fatalf("debian with iptables only: got %q, want ufw", got)
	}
	if got := firewallStrategy("ufw", "debian"); got != "ufw" {
		t.Fatalf("debian with ufw: got %q, want ufw", got)
	}
	if got := firewallStrategy("none", "rhel"); got != "firewalld" {
		t.Fatalf("rhel without firewalld: got %q, want firewalld", got)
	}
	if got := firewallStrategy("firewalld", "rhel"); got != "firewalld" {
		t.Fatalf("rhel with firewalld: got %q, want firewalld", got)
	}
	if got := firewallStrategy("none", "unknown"); got != "unknown" {
		t.Fatalf("unknown family: got %q, want unknown", got)
	}
}
