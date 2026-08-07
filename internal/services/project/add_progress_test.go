package project

import (
	"context"
	"path/filepath"
	"testing"
)

func TestAddEmitsProgressSteps(t *testing.T) {
	root := t.TempDir()
	setupFilesystem(t, root)

	stateDir := filepath.Join(root, "state")
	projectPath := filepath.Join(root, "var", "www", "demo")

	svc := New(false, false)
	svc.stateDir = stateDir
	svc.SetIdentityResolver(&mockIdentity{
		homes: testHomes(root),
	})

	var steps []string
	var messages []string
	opts := AddOptions{
		Name:      "demo",
		Path:      projectPath,
		WebServer: WebServerNginx,
		NoVhost:   true,
		Runtime:   RuntimeStatic,
		User:      SharedWebUser,
		Group:     SharedWebGroup,
		Yes:       true,
		Progress: func(step, message string) {
			steps = append(steps, step)
			messages = append(messages, message)
		},
	}

	state, err := svc.Add(context.Background(), opts)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if state == nil || state.Name != "demo" {
		t.Fatalf("unexpected state: %#v", state)
	}

	want := []string{
		"validate",
		"identity",
		"path",
		"runtime",
		"directories",
		"ownership",
		"state",
		"complete",
	}
	if len(steps) != len(want) {
		t.Fatalf("steps=%v want=%v messages=%v", steps, want, messages)
	}
	for i := range want {
		if steps[i] != want[i] {
			t.Fatalf("step[%d]=%q want %q (all=%v)", i, steps[i], want[i], steps)
		}
	}
	if messages[0] != "Validating options" {
		t.Fatalf("first message=%q", messages[0])
	}
	if messages[len(messages)-1] != "Project created successfully" {
		t.Fatalf("last message=%q", messages[len(messages)-1])
	}
}
