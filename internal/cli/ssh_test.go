package cli

import (
	"bytes"
	"strings"
	"testing"
)

func TestEnableRootLoginHelp(t *testing.T) {
	cmd := newSSHConfigEnableRootLoginCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"--keys-only",
		"PermitRootLogin yes",
		"prohibit-password",
		"disable-root-login",
		"every method",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("help missing %q:\n%s", want, out)
		}
	}
}

func TestDisableRootLoginHelp(t *testing.T) {
	cmd := newSSHConfigDisableRootLoginCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "every method") {
		t.Fatalf("help missing every method:\n%s", out)
	}
}
