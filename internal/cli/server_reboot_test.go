package cli

import "testing"

func TestServerRebootRequiresConfirmationByDefault(t *testing.T) {
	resetGlobalFlags(t)
	root := NewRootCmd()
	cmd, _, err := root.Find([]string{"server", "reboot"})
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Name() != "reboot" {
		t.Fatalf("command = %s", cmd.Name())
	}

	delay, err := cmd.Flags().GetInt("delay")
	if err != nil {
		t.Fatal(err)
	}
	if delay != 0 {
		t.Fatalf("delay default = %d, want 0", delay)
	}

	// --yes is the global flag confirm.Ask uses. It defaults to false, so
	// server reboot asks before rebooting.
	yes, err := root.PersistentFlags().GetBool("yes")
	if err != nil {
		t.Fatal(err)
	}
	if yes {
		t.Fatal("expected --yes to default to false")
	}
}
