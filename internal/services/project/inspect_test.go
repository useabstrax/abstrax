package project

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"abstrax/internal/platform"
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

func writeInspectState(t *testing.T, svc *Service, state State) {
	t.Helper()
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(svc.stateDir, state.Name+".json"), data, 0640); err != nil {
		t.Fatal(err)
	}
}

func newInspectService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	svc := New(false, false)
	svc.stateDir = filepath.Join(dir, "projects")
	svc.nginxAvail = filepath.Join(dir, "sites-available")
	svc.nginxEnabled = filepath.Join(dir, "sites-enabled")
	svc.nginxConfDir = filepath.Join(dir, "conf.d")
	for _, d := range []string{svc.stateDir, svc.nginxAvail, svc.nginxEnabled, svc.nginxConfDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}
	return svc
}

func TestInspectExtendedFields(t *testing.T) {
	svc := newInspectService(t)
	svc.nginxLayout = platform.NginxSitesAvailableEnabled

	vhostPath := filepath.Join(svc.nginxAvail, "abstrax-site")
	if err := os.WriteFile(vhostPath, []byte("server {}"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(vhostPath, filepath.Join(svc.nginxEnabled, "abstrax-site")); err != nil {
		t.Fatal(err)
	}

	created := time.Date(2026, 11, 1, 10, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 11, 2, 12, 30, 0, 0, time.FixedZone("X", 3600))
	writeInspectState(t, svc, State{
		Name:       "site",
		Path:       "/var/www/site",
		Domains:    []string{"site.test"},
		WebServer:  WebServerNginx,
		Runtime:    RuntimePHP,
		PHPVersion: "8.5",
		PublicDir:  "public",
		ProxyPort:  3000,
		SSLEnabled: true,
		VhostPath:  vhostPath,
		SharedLogs: true,
		Owner:      "site",
		CreatedAt:  created,
		UpdatedAt:  updated,
	})

	resp, err := svc.Inspect(context.Background(), "site")
	if err != nil {
		t.Fatal(err)
	}
	p := resp.Project
	if p.WebServer != "nginx" {
		t.Fatalf("web_server %q", p.WebServer)
	}
	if p.PublicDir != "public" || p.ProxyPort != 3000 {
		t.Fatalf("public_dir/proxy_port = %q/%d", p.PublicDir, p.ProxyPort)
	}
	if !p.Vhost.Present || p.Vhost.Path != vhostPath || !p.Vhost.Enabled {
		t.Fatalf("vhost %+v", p.Vhost)
	}
	if !p.SharedLogs {
		t.Fatal("shared_logs should be true")
	}
	if !p.SSL.Enabled {
		t.Fatal("ssl.enabled should be true")
	}
	if p.CreatedAt != "2026-11-01T10:00:00Z" || p.UpdatedAt != "2026-11-02T11:30:00Z" {
		t.Fatalf("timestamps %q %q", p.CreatedAt, p.UpdatedAt)
	}

	raw, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["api_version"] != "v1" {
		t.Fatalf("api_version %v", decoded["api_version"])
	}
	proj := decoded["project"].(map[string]any)
	for _, key := range []string{"web_server", "vhost", "public_dir", "proxy_port", "shared_logs", "ssl", "created_at", "updated_at"} {
		if _, ok := proj[key]; !ok {
			t.Errorf("missing key %q in %s", key, raw)
		}
	}
}

func TestInspectOmitsUnsetFields(t *testing.T) {
	svc := newInspectService(t)
	writeInspectState(t, svc, State{Name: "bare", Path: "/var/www/bare", Owner: "bare"})

	resp, err := svc.Inspect(context.Background(), "bare")
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(resp)
	body := string(raw)
	for _, key := range []string{`"public_dir"`, `"proxy_port"`, `"created_at"`, `"updated_at"`} {
		if strings.Contains(body, key) {
			t.Errorf("%s should be omitted: %s", key, body)
		}
	}
	for _, want := range []string{`"vhost":{"present":false,"path":"","enabled":false}`, `"shared_logs":false`, `"ssl":{"enabled":false}`, `"web_server":""`} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %s in %s", want, body)
		}
	}
}

func TestInspectVhostEnabled(t *testing.T) {
	t.Run("no vhost", func(t *testing.T) {
		svc := newInspectService(t)
		got := svc.inspectVhost(&State{})
		if got.Present || got.Enabled || got.Path != "" {
			t.Fatalf("%+v", got)
		}
	})

	t.Run("debian enabled and disabled", func(t *testing.T) {
		svc := newInspectService(t)
		svc.nginxLayout = platform.NginxSitesAvailableEnabled
		path := filepath.Join(svc.nginxAvail, "abstrax-a")
		if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		state := &State{VhostPath: path}

		if got := svc.inspectVhost(state); !got.Present || got.Enabled {
			t.Fatalf("no symlink: %+v", got)
		}
		if err := svc.enableVhostFile(path); err != nil {
			t.Fatal(err)
		}
		if got := svc.inspectVhost(state); !got.Present || !got.Enabled {
			t.Fatalf("symlinked: %+v", got)
		}
		if err := svc.disableVhostFile(path); err != nil {
			t.Fatal(err)
		}
		if got := svc.inspectVhost(state); !got.Present || got.Enabled {
			t.Fatalf("disabled: %+v", got)
		}
	})

	t.Run("debian dangling symlink", func(t *testing.T) {
		svc := newInspectService(t)
		svc.nginxLayout = platform.NginxSitesAvailableEnabled
		path := filepath.Join(svc.nginxAvail, "abstrax-b")
		if err := os.Symlink(path, filepath.Join(svc.nginxEnabled, "abstrax-b")); err != nil {
			t.Fatal(err)
		}
		if got := svc.inspectVhost(&State{VhostPath: path}); got.Enabled {
			t.Fatalf("dangling link should not be enabled: %+v", got)
		}
	})

	t.Run("rhel enabled and disabled", func(t *testing.T) {
		svc := newInspectService(t)
		svc.nginxLayout = platform.NginxConfD
		path := filepath.Join(svc.nginxConfDir, "abstrax-c.conf")
		if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		state := &State{VhostPath: path}

		if got := svc.inspectVhost(state); !got.Present || !got.Enabled {
			t.Fatalf("enabled: %+v", got)
		}
		if err := svc.disableVhostFile(path); err != nil {
			t.Fatal(err)
		}
		if got := svc.inspectVhost(state); !got.Present || got.Enabled {
			t.Fatalf("renamed .disabled: %+v", got)
		}
	})

	t.Run("unknown layout falls back to file existence", func(t *testing.T) {
		svc := newInspectService(t)
		svc.nginxLayout = platform.NginxLayoutUnknown
		path := filepath.Join(svc.nginxAvail, "abstrax-d")
		state := &State{VhostPath: path}
		if got := svc.inspectVhost(state); !got.Present || got.Enabled {
			t.Fatalf("missing file: %+v", got)
		}
		if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
		if got := svc.inspectVhost(state); !got.Enabled {
			t.Fatalf("existing file: %+v", got)
		}
		if got := svc.inspectVhost(&State{VhostPath: path + ".disabled"}); got.Enabled {
			t.Fatalf(".disabled path: %+v", got)
		}
	})
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
