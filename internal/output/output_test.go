package output

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"strings"
	"testing"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() { os.Stdout = old }()

	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()
	_ = w.Close()
	return <-done
}

func TestPrintProgressNDJSON(t *testing.T) {
	out := captureStdout(t, func() {
		PrintProgress("project.add", "validate", "Validating options")
	})
	line := strings.TrimSpace(out)
	var evt ProgressEvent
	if err := json.Unmarshal([]byte(line), &evt); err != nil {
		t.Fatalf("unmarshal: %v\nraw=%q", err, out)
	}
	if evt.Type != "progress" || evt.Action != "project.add" || evt.Step != "validate" || evt.Message != "Validating options" {
		t.Fatalf("unexpected event: %#v", evt)
	}
	if strings.Count(out, "\n") != 1 {
		t.Fatalf("expected single NDJSON line, got %q", out)
	}
}

func TestPrintStreamResultSuccess(t *testing.T) {
	out := captureStdout(t, func() {
		PrintStreamResult(Success("project.add", "Project created.", map[string]string{"name": "demo"}))
	})
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &raw); err != nil {
		t.Fatalf("unmarshal: %v\nraw=%q", err, out)
	}
	if raw["type"] != "result" || raw["status"] != "success" || raw["action"] != "project.add" {
		t.Fatalf("unexpected result: %#v", raw)
	}
	if _, ok := raw["data"]; !ok {
		t.Fatal("expected data field")
	}
}

func TestPrintStreamResultError(t *testing.T) {
	out := captureStdout(t, func() {
		PrintStreamResult(Failure("project.add", "command_error", "boom"))
	})
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if raw["type"] != "result" || raw["status"] != "error" || raw["error_code"] != "command_error" {
		t.Fatalf("unexpected result: %#v", raw)
	}
}

func TestPrintJSONUnchangedShape(t *testing.T) {
	out := captureStdout(t, func() {
		PrintJSON(Success("user.add", "User created.", nil))
	})
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Fatalf("unmarshal: %v\nraw=%q", err, out)
	}
	if _, ok := raw["type"]; ok {
		t.Fatalf("--json output must not include type wrapper, got %#v", raw)
	}
	if raw["status"] != "success" || raw["action"] != "user.add" {
		t.Fatalf("unexpected --json shape: %#v", raw)
	}
	if !strings.Contains(out, "\n  ") {
		t.Fatalf("expected indented --json output, got %q", out)
	}
}

func TestWriteResultStreamVsJSON(t *testing.T) {
	streamOut := captureStdout(t, func() {
		WriteResult(Success("doctor.check", "ok", nil), true)
	})
	jsonOut := captureStdout(t, func() {
		WriteResult(Success("doctor.check", "ok", nil), false)
	})
	if !strings.Contains(streamOut, `"type":"result"`) && !strings.Contains(streamOut, `"type": "result"`) {
		// compact encoder has no spaces
		if !strings.Contains(streamOut, `"type":"result"`) {
			t.Fatalf("stream missing type=result: %q", streamOut)
		}
	}
	var stream map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(streamOut)), &stream); err != nil {
		t.Fatal(err)
	}
	if stream["type"] != "result" {
		t.Fatalf("stream type=%v", stream["type"])
	}
	var plain map[string]interface{}
	if err := json.Unmarshal([]byte(jsonOut), &plain); err != nil {
		t.Fatal(err)
	}
	if _, ok := plain["type"]; ok {
		t.Fatal("json mode should not wrap with type")
	}
}

func TestPrinterProgressOnlyInStreamMode(t *testing.T) {
	p := NewPrinter(false, false, false, false, true)
	out := captureStdout(t, func() {
		p.Progress("project.add", "validate", "Validating options")
	})
	if out != "" {
		t.Fatalf("expected no progress outside stream mode, got %q", out)
	}

	p = NewPrinter(false, true, false, false, true)
	out = captureStdout(t, func() {
		p.Progress("project.add", "validate", "Validating options")
	})
	if !strings.Contains(out, `"step":"validate"`) {
		t.Fatalf("expected progress in stream mode, got %q", out)
	}
}
