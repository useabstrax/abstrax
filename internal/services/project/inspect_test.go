package project

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestInspectResponse(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "projects")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}

	state := State{
		Name:       "example",
		Path:       "/var/www/example",
		Domains:    []string{"example.com"},
		Runtime:    RuntimePHP,
		PHPVersion: "8.5",
		Owner:      "example",
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "example.json"), data, 0640); err != nil {
		t.Fatal(err)
	}

	svc := New(false, false)
	svc.stateDir = stateDir

	resp, err := svc.Inspect(context.Background(), "example")
	if err != nil {
		t.Fatal(err)
	}
	if resp.APIVersion != "v1" {
		t.Fatalf("api version %q, want v1", resp.APIVersion)
	}
	if resp.Project.Name != "example" {
		t.Fatalf("name %q", resp.Project.Name)
	}
	if resp.Project.Runtime.Type != "php" || resp.Project.Runtime.Version != "8.5" {
		t.Fatalf("runtime %+v", resp.Project.Runtime)
	}
	if resp.Project.User != "example" {
		t.Fatalf("user %q", resp.Project.User)
	}

	encoded, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	if string(encoded) == "" {
		t.Fatal("empty json")
	}
}

func TestResolveProjectDaemonOwnership(t *testing.T) {
	dir := t.TempDir()
	stateDir := filepath.Join(dir, "projects")
	supervisorDir := filepath.Join(dir, "supervisor")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(supervisorDir, 0755); err != nil {
		t.Fatal(err)
	}

	state := State{
		Name: "myapp",
		Path: "/var/www/myapp",
		Services: []ProjectService{
			{Name: "worker", Type: "worker"},
		},
	}
	data, _ := json.Marshal(state)
	if err := os.WriteFile(filepath.Join(stateDir, "myapp.json"), data, 0640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(supervisorDir, "abstrax-myapp-worker.conf"), []byte("[program:abstrax-myapp-worker]"), 0644); err != nil {
		t.Fatal(err)
	}

	oldDir := supervisorConfDirPath
	supervisorConfDirPath = supervisorDir
	t.Cleanup(func() { supervisorConfDirPath = oldDir })

	svc := New(false, false)
	svc.stateDir = stateDir

	daemon, err := svc.ResolveProjectDaemon(context.Background(), "myapp", "worker")
	if err != nil {
		t.Fatal(err)
	}
	if daemon != "abstrax-myapp-worker" {
		t.Fatalf("daemon %q", daemon)
	}

	_, err = svc.ResolveProjectDaemon(context.Background(), "myapp", "other")
	if err == nil {
		t.Fatal("expected ownership error")
	}
}
