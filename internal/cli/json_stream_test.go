package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"

	"abstrax/internal/globals"
)

func resetGlobalFlags(t *testing.T) {
	t.Helper()
	globals.Flags = &globals.GlobalFlags{}
	t.Cleanup(func() {
		globals.Flags = &globals.GlobalFlags{}
	})
}

func captureRootStdout(t *testing.T, args ...string) (string, error) {
	t.Helper()
	oldArgs := os.Args
	oldStdout := os.Stdout
	os.Args = append([]string{"abstrax"}, args...)
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() {
		os.Args = oldArgs
		os.Stdout = oldStdout
	}()

	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	root := NewRootCmd()
	root.SetArgs(args)
	cmdErr := root.Execute()
	_ = w.Close()
	out := <-done
	return out, cmdErr
}

func TestJSONStreamMutuallyExclusiveWithJSON(t *testing.T) {
	resetGlobalFlags(t)
	_, err := captureRootStdout(t, "version", "--json", "--json-stream")
	if err == nil {
		t.Fatal("expected mutual exclusion error")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVersionJSONStreamEmitsResultLine(t *testing.T) {
	resetGlobalFlags(t)
	out, err := captureRootStdout(t, "version", "--json-stream")
	if err != nil {
		t.Fatal(err)
	}
	line := strings.TrimSpace(out)
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(line), &raw); err != nil {
		t.Fatalf("unmarshal %q: %v", out, err)
	}
	if raw["type"] != "result" || raw["status"] != "success" || raw["action"] != "version.show" {
		t.Fatalf("unexpected stream result: %#v", raw)
	}
}

func TestVersionJSONUnchanged(t *testing.T) {
	resetGlobalFlags(t)
	out, err := captureRootStdout(t, "version", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Fatalf("unmarshal %q: %v", out, err)
	}
	if _, ok := raw["type"]; ok {
		t.Fatalf("--json must not wrap with type: %#v", raw)
	}
	if raw["status"] != "success" || raw["action"] != "version.show" {
		t.Fatalf("unexpected --json result: %#v", raw)
	}
}
