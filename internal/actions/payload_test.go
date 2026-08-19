package actions

import (
	"encoding/json"
	"strings"
	"testing"

	"abstrax/internal/globals"
)

func TestPayloadToArgs(t *testing.T) {
	flags, args, err := PayloadToArgs(json.RawMessage(`{"args":["alice"],"system":true,"comment":"hi","keep":5}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 1 || args[0] != "alice" {
		t.Fatalf("args = %#v", args)
	}
	got := strings.Join(flags, " ")
	if !strings.Contains(got, "--comment=hi") || !strings.Contains(got, "--keep=5") || !strings.Contains(got, "--system") {
		t.Fatalf("flags = %#v", flags)
	}
}

func TestPayloadToArgsOmitsFalse(t *testing.T) {
	flags, args, err := PayloadToArgs(json.RawMessage(`{"force":false}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(flags) != 0 || len(args) != 0 {
		t.Fatalf("flags=%#v args=%#v", flags, args)
	}
}

func TestPayloadToArgsRejectsObject(t *testing.T) {
	_, _, err := PayloadToArgs(json.RawMessage(`{"nested":{"a":1}}`))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPayloadToArgsEmpty(t *testing.T) {
	flags, args, err := PayloadToArgs(nil)
	if err != nil || flags != nil || args != nil {
		t.Fatalf("empty payload: flags=%#v args=%#v err=%v", flags, args, err)
	}
}

func TestGlobalArgsAlwaysYes(t *testing.T) {
	got := GlobalArgs(&globals.GlobalFlags{JSON: true, Verbose: true})
	joined := strings.Join(got, " ")
	if !strings.Contains(joined, "--json") || !strings.Contains(joined, "--yes") || !strings.Contains(joined, "--verbose") {
		t.Fatalf("got %#v", got)
	}
	if strings.Contains(joined, "--json-stream") {
		t.Fatalf("unexpected json-stream: %#v", got)
	}
}
