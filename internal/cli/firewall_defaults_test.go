package cli

import (
	"os"
	"strings"
	"testing"

	"abstrax/internal/actions"
)

func TestFirewallDefaultsCommandsResolve(t *testing.T) {
	resetGlobalFlags(t)
	root := NewRootCmd()
	for action, want := range map[string]string{
		actions.FirewallDefaults:    "defaults",
		actions.FirewallDefaultsSet: "set",
		actions.FirewallPing:        "ping",
	} {
		path, ok := actions.CommandPath(action)
		if !ok {
			t.Fatalf("no command path for %s", action)
		}
		cmd, _, err := root.Find(path)
		if err != nil {
			t.Fatalf("Find(%v): %v", path, err)
		}
		if cmd.Name() != want {
			t.Fatalf("Find(%v) = %s, want %s", path, cmd.Name(), want)
		}
	}
}

func TestFirewallDefaultsSetFlagsAndHelp(t *testing.T) {
	cmd, _, err := NewFirewallCmd().Find([]string{"defaults", "set"})
	if err != nil {
		t.Fatal(err)
	}
	for _, flag := range []string{"incoming", "outgoing", "force"} {
		if cmd.Flags().Lookup(flag) == nil {
			t.Fatalf("missing --%s", flag)
		}
	}
	if !strings.Contains(cmd.Long, "--force") || !strings.Contains(cmd.Long, "--yes") {
		t.Fatalf("help = %q", cmd.Long)
	}
}

func TestFirewallDefaultsSetRequiresIncoming(t *testing.T) {
	resetGlobalFlags(t)
	_, err := captureRootStdout(t, "firewall", "defaults", "set")
	if err == nil || !strings.Contains(err.Error(), "--incoming is required") {
		t.Fatalf("err = %v", err)
	}
}

func TestFirewallPingRejectsBadArg(t *testing.T) {
	resetGlobalFlags(t)
	for _, args := range [][]string{
		{"firewall", "ping"},
		{"firewall", "ping", "maybe"},
		{"firewall", "ping", "allow", "block"},
	} {
		if _, err := captureRootStdout(t, args...); err == nil {
			t.Fatalf("%v: expected an error", args)
		}
	}
}

func TestConfirmIncomingAllow(t *testing.T) {
	resetGlobalFlags(t)

	t.Run("yes skips the prompt", func(t *testing.T) {
		ok, err := confirmIncomingAllow(true)
		if err != nil || !ok {
			t.Fatalf("ok = %v, err = %v", ok, err)
		}
	})

	t.Run("no terminal without yes is refused", func(t *testing.T) {
		withStdin(t, "")
		ok, err := confirmIncomingAllow(false)
		if err == nil || ok {
			t.Fatalf("ok = %v, err = %v", ok, err)
		}
	})

	t.Run("answering no declines", func(t *testing.T) {
		withStdin(t, "n\n")
		ok, err := confirmIncomingAllow(false)
		if err != nil || ok {
			t.Fatalf("ok = %v, err = %v", ok, err)
		}
	})

	t.Run("answering yes confirms", func(t *testing.T) {
		withStdin(t, "y\n")
		ok, err := confirmIncomingAllow(false)
		if err != nil || !ok {
			t.Fatalf("ok = %v, err = %v", ok, err)
		}
	})
}

func withStdin(t *testing.T, input string) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.WriteString(input); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	old := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = old
		_ = r.Close()
	})
}
