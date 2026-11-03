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

func TestPayloadToArgsEncodesFalse(t *testing.T) {
	flags, args, err := PayloadToArgs(json.RawMessage(`{"shared-logs":false,"redirect-http":false,"force":true}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(args) != 0 {
		t.Fatalf("args=%#v", args)
	}
	got := strings.Join(flags, " ")
	if !strings.Contains(got, "--shared-logs=false") || !strings.Contains(got, "--redirect-http=false") || !strings.Contains(got, "--force") {
		t.Fatalf("flags = %#v", flags)
	}
	if strings.Contains(got, "--force=false") || strings.Contains(got, "--force=true") {
		t.Fatalf("true should emit bare flag: %#v", flags)
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
