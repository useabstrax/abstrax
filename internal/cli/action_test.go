package cli

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"abstrax/internal/actions"
)

func TestBuiltinActionCommandPathsResolve(t *testing.T) {
	resetGlobalFlags(t)
	root := NewRootCmd()
	for _, action := range actions.All() {
		path, ok := actions.CommandPath(action)
		if !ok {
			t.Errorf("no command path for %q", action)
			continue
		}
		cmd, _, err := root.Find(path)
		if err != nil {
			t.Errorf("Find(%q %v): %v", action, path, err)
			continue
		}
		if cmd == nil || cmd == root && len(path) > 0 {
			t.Errorf("Find(%q %v) returned root", action, path)
		}
	}
}

func TestActionVersionShowJSON(t *testing.T) {
	resetGlobalFlags(t)
	out, err := captureRootStdout(t, "--json", "--action", "version.show")
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Fatalf("unmarshal %q: %v", out, err)
	}
	if raw["status"] != "success" || raw["action"] != "version.show" {
		t.Fatalf("unexpected result: %#v", raw)
	}
}

func TestActionUnknown(t *testing.T) {
	resetGlobalFlags(t)
	_, err := captureRootStdout(t, "--json", "--action", "not.a.real.action")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, actions.ErrUnknownAction) && !strings.Contains(err.Error(), "unknown action") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestActionPluginInstallIsBuiltin(t *testing.T) {
	path, ok := actions.CommandPath(actions.PluginInstall)
	if !ok || len(path) != 2 || path[0] != "plugin" || path[1] != "install" {
		t.Fatalf("plugin.install path = %v ok=%v", path, ok)
	}
}
