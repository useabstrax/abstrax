package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"abstrax/internal/globals"
)

func TestParsePluginAction(t *testing.T) {
	parsed, ok := ParsePluginAction("plugin.deploy.now")
	if !ok || parsed.Plugin != "deploy" || len(parsed.Command) != 1 || parsed.Command[0] != "now" {
		t.Fatalf("got %#v ok=%v", parsed, ok)
	}

	parsed, ok = ParsePluginAction("plugin.composer.self_update")
	if !ok || parsed.Plugin != "composer" || parsed.Command[0] != "self-update" {
		t.Fatalf("got %#v ok=%v", parsed, ok)
	}

	if _, ok := ParsePluginAction("plugin.install"); ok {
		t.Fatal("plugin.install must not parse as a plugin binary action")
	}
	if _, ok := ParsePluginAction("user.add"); ok {
		t.Fatal("user.add must not parse as a plugin action")
	}
	if _, ok := ParsePluginAction("plugin..now"); ok {
		t.Fatal("empty segment should fail")
	}
}

func TestDispatchActionHello(t *testing.T) {
	dir := t.TempDir()
	buildTestPlugin(t, dir, "example")
	paths := testPaths(dir)
	store := NewStore(paths.RecordDir)
	dispatcher, err := NewDispatcher(paths, store)
	if err != nil {
		t.Fatal(err)
	}

	parsed, ok := ParsePluginAction("plugin.example.hello")
	if !ok {
		t.Fatal("parse")
	}
	exitCode, err := dispatcher.DispatchAction(context.Background(), parsed, json.RawMessage(`{"args":["Abstrax"]}`), &globals.GlobalFlags{JSON: true}, DispatchOptions{})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("exit code %d", exitCode)
	}
}

func TestDispatchActionUnknownCommand(t *testing.T) {
	dir := t.TempDir()
	buildTestPlugin(t, dir, "example")
	paths := testPaths(dir)
	store := NewStore(paths.RecordDir)
	dispatcher, err := NewDispatcher(paths, store)
	if err != nil {
		t.Fatal(err)
	}

	parsed, ok := ParsePluginAction("plugin.example.missing")
	if !ok {
		t.Fatal("parse")
	}
	_, err = dispatcher.DispatchAction(context.Background(), parsed, json.RawMessage(`{}`), &globals.GlobalFlags{}, DispatchOptions{})
	if err == nil || !errors.Is(err, ErrUnknownPluginCommand) {
		t.Fatalf("got %v", err)
	}
	if !strings.Contains(err.Error(), "missing") {
		t.Fatalf("error = %v", err)
	}
}
