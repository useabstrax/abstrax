package project

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildNginxConfigWritesProjectLogs(t *testing.T) {
	conf, err := buildNginxConfig(vhostConfig{
		Name:    "example",
		Path:    "/var/www/example",
		Domains: []string{"example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}
	access := "    access_log /var/log/nginx/example-access.log;\n"
	errorLine := "    error_log /var/log/nginx/example-error.log;\n"
	accessAt := strings.Index(conf, access)
	errorAt := strings.Index(conf, errorLine)
	if accessAt < 0 || errorAt < 0 || accessAt > errorAt {
		t.Fatalf("config missing project logs:\n%s", conf)
	}
	if !strings.Contains(conf, "    index index.html index.htm index.php;\n"+access+errorLine+"\n    location / {\n") {
		t.Fatalf("log lines changed the surrounding vhost:\n%s", conf)
	}
}

func TestBuildNginxConfigSharedLogsOmitsDirectives(t *testing.T) {
	conf, err := buildNginxConfig(vhostConfig{
		Name:       "example",
		Path:       "/var/www/example",
		Domains:    []string{"example.com"},
		LogDir:     "/tmp/should-not-appear",
		SharedLogs: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(conf, "access_log") || strings.Contains(conf, "error_log") {
		t.Fatalf("shared logs config wrote log directives:\n%s", conf)
	}
	if !strings.Contains(conf, "    index index.html index.htm index.php;\n\n    location / {\n") {
		t.Fatalf("shared logs config changed the vhost:\n%s", conf)
	}
}

func TestBuildNginxConfigRejectsUnsafeLogNames(t *testing.T) {
	for _, name := range []string{"..", "../etc", "foo/bar", `foo\bar`, "a..b"} {
		_, err := buildNginxConfig(vhostConfig{
			Name: name,
			Path: "/var/www/example",
		})
		if err == nil {
			t.Fatalf("name %q: expected error", name)
		}
	}

	conf, err := buildNginxConfig(vhostConfig{
		Name:       "..",
		Path:       "/var/www/example",
		SharedLogs: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(conf, "access_log") || strings.Contains(conf, "error_log") {
		t.Fatalf("shared logs config wrote log directives:\n%s", conf)
	}
}

func TestRenderNginxConfigUsesServiceLogDir(t *testing.T) {
	logDir := t.TempDir()
	svc := &Service{nginxLogDir: logDir}
	conf, err := svc.renderNginxConfig(vhostConfig{
		Name: "example",
		Path: "/var/www/example",
	})
	if err != nil {
		t.Fatal(err)
	}
	access := filepath.Join(logDir, "example-access.log")
	errorPath := filepath.Join(logDir, "example-error.log")
	if !strings.Contains(conf, "    access_log "+access+";\n") {
		t.Fatalf("config = %s", conf)
	}
	if !strings.Contains(conf, "    error_log "+errorPath+";\n") {
		t.Fatalf("config = %s", conf)
	}
}

func TestInfoAndInspectLogPaths(t *testing.T) {
	logDir := t.TempDir()
	svc := New(false, false)
	svc.stateDir = t.TempDir()
	svc.nginxLogDir = logDir

	state := &State{
		Name:      "example",
		Path:      "/var/www/example",
		Domains:   []string{"example.com"},
		Runtime:   RuntimeStatic,
		WebServer: WebServerNginx,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := svc.saveState(state); err != nil {
		t.Fatal(err)
	}

	info, err := svc.Info(context.Background(), "example")
	if err != nil {
		t.Fatal(err)
	}
	access := filepath.Join(logDir, "example-access.log")
	errorPath := filepath.Join(logDir, "example-error.log")
	if info.AccessLogPath != access || info.ErrorLogPath != errorPath {
		t.Fatalf("info paths = %q %q", info.AccessLogPath, info.ErrorLogPath)
	}

	if err := svc.SaveState(info); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(svc.statePath("example"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "access_log_path") || strings.Contains(string(raw), "error_log_path") {
		t.Fatalf("log paths were persisted:\n%s", raw)
	}

	inspected, err := svc.Inspect(context.Background(), "example")
	if err != nil {
		t.Fatal(err)
	}
	if inspected.Project.AccessLogPath != access || inspected.Project.ErrorLogPath != errorPath {
		t.Fatalf("inspect paths = %q %q", inspected.Project.AccessLogPath, inspected.Project.ErrorLogPath)
	}
	encoded, err := json.Marshal(inspected)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"access_log_path":"`+access+`"`) {
		t.Fatalf("inspect json = %s", encoded)
	}

	state.SharedLogs = true
	if err := svc.saveState(state); err != nil {
		t.Fatal(err)
	}
	info, err = svc.Info(context.Background(), "example")
	if err != nil {
		t.Fatal(err)
	}
	if info.AccessLogPath != "" || info.ErrorLogPath != "" {
		t.Fatalf("shared info paths = %q %q", info.AccessLogPath, info.ErrorLogPath)
	}
	encoded, err = json.Marshal(info)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "access_log_path") || strings.Contains(string(encoded), "error_log_path") {
		t.Fatalf("shared info json included log paths: %s", encoded)
	}

	inspected, err = svc.Inspect(context.Background(), "example")
	if err != nil {
		t.Fatal(err)
	}
	if inspected.Project.AccessLogPath != "" || inspected.Project.ErrorLogPath != "" {
		t.Fatalf("shared inspect paths = %q %q", inspected.Project.AccessLogPath, inspected.Project.ErrorLogPath)
	}
	encoded, err = json.Marshal(inspected)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "access_log_path") || strings.Contains(string(encoded), "error_log_path") {
		t.Fatalf("shared inspect json included log paths: %s", encoded)
	}
}

func TestModifySharedLogsPersists(t *testing.T) {
	logDir := t.TempDir()
	svc := New(false, false)
	svc.stateDir = t.TempDir()
	svc.nginxLogDir = logDir

	if err := svc.saveState(&State{
		Name:       "example",
		Path:       "/var/www/example",
		WebServer:  WebServerNginx,
		SharedLogs: true,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	got, err := svc.Modify(context.Background(), ModifyOptions{
		Name:      "example",
		AddDomain: "example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !got.SharedLogs {
		t.Fatal("modify cleared shared logs")
	}
	if got.AccessLogPath != "" || got.ErrorLogPath != "" {
		t.Fatalf("paths = %q %q", got.AccessLogPath, got.ErrorLogPath)
	}
	raw, err := os.ReadFile(svc.statePath("example"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"shared_logs": true`) {
		t.Fatalf("state = %s", raw)
	}

	off := false
	got, err = svc.Modify(context.Background(), ModifyOptions{
		Name:       "example",
		SharedLogs: &off,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.SharedLogs {
		t.Fatal("modify did not turn shared logs off")
	}
	if got.AccessLogPath != filepath.Join(logDir, "example-access.log") || got.ErrorLogPath != filepath.Join(logDir, "example-error.log") {
		t.Fatalf("paths = %q %q", got.AccessLogPath, got.ErrorLogPath)
	}
	raw, err = os.ReadFile(svc.statePath("example"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "shared_logs") {
		t.Fatalf("shared logs stayed in state:\n%s", raw)
	}
}

func TestRemoveDeleteFilesRemovesLogs(t *testing.T) {
	logDir := t.TempDir()
	projectDir := filepath.Join(t.TempDir(), "example")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "index.html"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}
	access := filepath.Join(logDir, "example-access.log")
	errorPath := filepath.Join(logDir, "example-error.log")
	other := filepath.Join(logDir, "other.log")
	for _, path := range []string{access, errorPath, other} {
		if err := os.WriteFile(path, []byte("log\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	svc := New(false, false)
	svc.stateDir = t.TempDir()
	svc.nginxLogDir = logDir
	if err := svc.saveState(&State{
		Name:      "example",
		Path:      projectDir,
		WebServer: WebServerNginx,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	if err := svc.Remove(context.Background(), RemoveOptions{
		Name:        "example",
		DeleteFiles: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(access); !os.IsNotExist(err) {
		t.Fatalf("access log still present: %v", err)
	}
	if _, err := os.Stat(errorPath); !os.IsNotExist(err) {
		t.Fatalf("error log still present: %v", err)
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatalf("unrelated log removed: %v", err)
	}
	if _, err := os.Stat(projectDir); !os.IsNotExist(err) {
		t.Fatalf("project dir still present: %v", err)
	}
}

func TestRemoveKeepsLogs(t *testing.T) {
	logDir := t.TempDir()
	projectDir := filepath.Join(t.TempDir(), "example")
	if err := os.MkdirAll(projectDir, 0755); err != nil {
		t.Fatal(err)
	}
	access := filepath.Join(logDir, "example-access.log")
	errorPath := filepath.Join(logDir, "example-error.log")
	for _, path := range []string{access, errorPath} {
		if err := os.WriteFile(path, []byte("log\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	svc := New(false, false)
	svc.stateDir = t.TempDir()
	svc.nginxLogDir = logDir
	if err := svc.saveState(&State{
		Name:      "example",
		Path:      projectDir,
		WebServer: WebServerNginx,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}); err != nil {
		t.Fatal(err)
	}

	if err := svc.Remove(context.Background(), RemoveOptions{
		Name:      "example",
		KeepFiles: true,
	}); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{access, errorPath, projectDir} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("%s removed: %v", path, err)
		}
	}
}

func TestRemoveDoesNotDeleteLogsOutsideLogDir(t *testing.T) {
	parent := t.TempDir()
	logDir := filepath.Join(parent, "nginx")
	if err := os.MkdirAll(logDir, 0755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(parent, "secret-access.log")
	if err := os.WriteFile(outside, []byte("keep"), 0644); err != nil {
		t.Fatal(err)
	}
	svc := &Service{nginxLogDir: logDir}
	if err := svc.removeProjectLogs("../secret"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("file outside the log dir was removed: %v", err)
	}
}

func TestVhostOptionsCarrySharedLogs(t *testing.T) {
	cfg := vhostOptionsFromAdd(
		AddOptions{Name: "example", SharedLogs: true},
		&State{Name: "example"},
		&ValidatedPaths{ProjectPath: "/var/www/example"},
	)
	if !cfg.SharedLogs {
		t.Fatal("add options dropped shared logs")
	}
	state := &State{Name: "example", Path: "/var/www/example", SharedLogs: true}
	if !state.vhostConfig().SharedLogs {
		t.Fatal("state dropped shared logs")
	}
}
