package cli

import (
	"strings"
	"testing"
)

func TestFirewallHelpMentionsInteractiveSSHProtection(t *testing.T) {
	root := NewFirewallCmd()
	for _, name := range []string{"allow", "deny", "allow-ip", "deny-ip"} {
		cmd, _, err := root.Find([]string{name})
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(cmd.Long, "SSH_CONNECTION") || !strings.Contains(cmd.Long, "SSH_CLIENT") || !strings.Contains(cmd.Long, "script") {
			t.Fatalf("%s help = %q", name, cmd.Long)
		}
	}
}

func TestFirewallLockoutHelpMentionsForce(t *testing.T) {
	root := NewFirewallCmd()
	commands := [][]string{
		{"deny"},
		{"rule", "remove"},
		{"remove", "service"},
		{"remove", "port"},
	}
	for _, args := range commands {
		cmd, _, err := root.Find(args)
		if err != nil {
			t.Fatal(err)
		}
		if cmd.Flags().Lookup("force") == nil {
			t.Fatalf("%s missing --force", strings.Join(args, " "))
		}
		if !strings.Contains(cmd.Long, "--force") || !strings.Contains(cmd.Long, "--yes") {
			t.Fatalf("%s help = %q", strings.Join(args, " "), cmd.Long)
		}
	}
}
