package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestE2EPluginInstallAndHello(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping e2e install test in short mode")
	}

	platform, err := CurrentPlatform()
	if err != nil {
		t.Skipf("unsupported platform: %v", err)
	}

	dir := t.TempDir()
	pluginBin := buildTestPlugin(t, dir, "example")
	data, err := os.ReadFile(pluginBin)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	checksum := hex.EncodeToString(sum[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/plugins/example":
			json.NewEncoder(w).Encode(RegistryPlugin{
				Name: "example", DisplayName: "Example Plugin", Publisher: "useabstrax",
				TrustLevel: TrustOfficial, Status: StatusActive,
			})
		case "/plugins/example/versions/latest":
			json.NewEncoder(w).Encode(RegistryVersion{
				Version: "0.1.0", RequiresAbstrax: ">=0.1.0", Stable: true, Channel: "stable", ProtocolVersion: 1,
				Platforms: map[string]RegistryPlatformBinary{
					platform: {URL: "http://" + r.Host + "/binary", SHA256: checksum, Size: int64(len(data))},
				},
			})
		case "/binary":
			w.Write(data)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	paths := testPaths(dir)
	svc := NewWithPaths(paths, srv.URL)
	result, err := svc.Install(context.Background(), InstallOptions{Name: "example"})
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}
	if result.Name != "example" {
		t.Fatalf("result name = %q", result.Name)
	}

	dispatcher, err := svc.NewDispatcher()
	if err != nil {
		t.Fatal(err)
	}
	exitCode, err := dispatcher.Dispatch(context.Background(), "example", []string{"hello", "Abstrax"}, DispatchOptions{})
	if err != nil {
		t.Fatalf("dispatch failed: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("exit code = %d", exitCode)
	}

	if err := svc.Remove("example"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Store().Load("example"); err == nil {
		t.Fatal("expected record removed")
	}
}

func TestPluginMetadataFixtureValidates(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	fixturePath := filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "openapi", "fixtures", "plugin-metadata.json")
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}

	var meta Metadata
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.ProtocolVersion != ProtocolVersion {
		t.Fatalf("protocol_version = %d", meta.ProtocolVersion)
	}
	if meta.Name == "" || meta.DisplayName == "" || meta.Description == "" || meta.Version == "" || meta.RequiresAbstrax == "" {
		t.Fatal("missing required metadata fields")
	}
	if len(meta.Commands) == 0 {
		t.Fatal("expected commands")
	}
}

func TestReleaseManifestFixtureFields(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	fixturePath := filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "openapi", "fixtures", "release-manifest.json")
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.ProtocolVersion != 1 {
		t.Fatalf("protocol_version = %d", manifest.ProtocolVersion)
	}
	if manifest.Channel != "stable" {
		t.Fatalf("channel = %q", manifest.Channel)
	}
	if manifest.Platforms["linux-amd64"].Size == 0 {
		t.Fatal("expected platform size")
	}
}

func TestProjectInspectFixtureMatchesSDK(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	fixturePath := filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "openapi", "fixtures", "project-inspect.json")
	data, err := os.ReadFile(fixturePath)
	if err != nil {
		t.Fatal(err)
	}

	var inspect InspectResponse
	if err := json.Unmarshal(data, &inspect); err != nil {
		t.Fatal(err)
	}
	if inspect.APIVersion != "v1" {
		t.Fatalf("api_version = %q", inspect.APIVersion)
	}
	if !strings.Contains(inspect.Project.Runtime.Type, "php") {
		t.Fatalf("runtime = %+v", inspect.Project.Runtime)
	}
}

// InspectResponse mirrors the CLI project inspect API for contract tests.
type InspectResponse struct {
	APIVersion string         `json:"api_version"`
	Project    InspectProject `json:"project"`
}

type InspectProject struct {
	Name     string           `json:"name"`
	Path     string           `json:"path"`
	User     string           `json:"user"`
	Runtime  InspectRuntime   `json:"runtime"`
	Domains  []string         `json:"domains"`
	Services []InspectService `json:"services"`
}

type InspectRuntime struct {
	Type    string `json:"type"`
	Version string `json:"version"`
}

type InspectService struct {
	Name string `json:"name"`
	Type string `json:"type"`
}
