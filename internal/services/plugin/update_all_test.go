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
	"testing"
	"time"
)

func TestUpdateAllEmpty(t *testing.T) {
	svc := NewWithPaths(testPaths(t.TempDir()), "http://127.0.0.1")
	result, err := svc.UpdateAll(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Updated) != 0 || len(result.Skipped) != 0 || len(result.Failed) != 0 {
		t.Fatalf("expected no plugins, got %+v", result)
	}
}

func TestUpdateAllSkipsLocalPlugins(t *testing.T) {
	dir := t.TempDir()
	svc := NewWithPaths(testPaths(dir), "http://127.0.0.1")
	if err := svc.Store().Save(&InstallRecord{
		Name:        "custom",
		Version:     "0.1.0",
		Source:      SourceLocal,
		InstalledAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	result, err := svc.UpdateAll(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Reason != SkipReasonLocal || result.Skipped[0].Name != "custom" {
		t.Fatalf("skipped = %+v", result.Skipped)
	}
	if len(result.Updated) != 0 || len(result.Failed) != 0 {
		t.Fatalf("expected only a skip, got %+v", result)
	}
}

func TestUpdateAllRequiresLinuxForRegistryPlugins(t *testing.T) {
	if runtime.GOOS == "linux" {
		t.Skip("registry updates are supported on linux")
	}
	dir := t.TempDir()
	svc := NewWithPaths(testPaths(dir), "http://127.0.0.1")
	if err := svc.Store().Save(&InstallRecord{
		Name:    "deploy",
		Version: "0.1.0",
		Source:  SourceRegistry,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UpdateAll(context.Background(), true); err == nil {
		t.Fatal("expected unsupported platform error")
	}
}

func TestUpdateAllUpdatesRegistryPlugins(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("plugin install is only supported on linux")
	}

	dir := t.TempDir()
	pluginBin := buildTestPlugin(t, dir, "deploy")
	data, err := os.ReadFile(pluginBin)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	checksum := hex.EncodeToString(sum[:])
	var binaryHits int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/plugins/deploy":
			json.NewEncoder(w).Encode(RegistryPlugin{
				Name: "deploy", Publisher: "useabstrax", TrustLevel: TrustOfficial, Status: StatusActive,
			})
		case "/plugins/deploy/versions/latest", "/plugins/current/versions/latest":
			json.NewEncoder(w).Encode(RegistryVersion{
				Version: "1.0.0", RequiresAbstrax: ">=0.0.0", Stable: true, Channel: "stable", ProtocolVersion: 1,
				Platforms: map[string]RegistryPlatformBinary{
					"linux-amd64": {URL: "http://" + r.Host + "/binary", SHA256: checksum, Size: int64(len(data))},
					"linux-arm64": {URL: "http://" + r.Host + "/binary", SHA256: checksum, Size: int64(len(data))},
				},
			})
		case "/plugins/broken/versions/latest":
			http.NotFound(w, r)
		case "/binary":
			binaryHits++
			w.Write(data)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	paths := testPaths(dir)
	svc := NewWithPaths(paths, srv.URL)
	for _, rec := range []InstallRecord{
		{Name: "deploy", Version: "0.9.0", Source: SourceRegistry},
		{Name: "current", Version: "1.0.0", Source: SourceRegistry},
	} {
		if err := svc.Store().Save(&rec); err != nil {
			t.Fatal(err)
		}
	}

	preview, err := svc.UpdateAll(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Updated) != 1 || preview.Updated[0].Name != "deploy" || preview.Updated[0].TargetVersion != "1.0.0" {
		t.Fatalf("preview updated = %+v", preview.Updated)
	}
	if len(preview.Skipped) != 1 || preview.Skipped[0].Name != "current" || preview.Skipped[0].Reason != SkipReasonUpToDate {
		t.Fatalf("preview skipped = %+v", preview.Skipped)
	}
	if binaryHits != 0 {
		t.Fatalf("dry-run downloaded the binary %d times", binaryHits)
	}
	rec, err := svc.Store().Load("deploy")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Version != "0.9.0" {
		t.Fatalf("dry-run changed version to %s", rec.Version)
	}

	if err := svc.Store().Save(&InstallRecord{
		Name:    "broken",
		Version: "0.1.0",
		Source:  SourceRegistry,
	}); err != nil {
		t.Fatal(err)
	}

	result, err := svc.UpdateAll(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Updated) != 1 || result.Updated[0].Name != "deploy" || result.Updated[0].Version != "1.0.0" || result.Updated[0].PreviousVersion != "0.9.0" {
		t.Fatalf("updated = %+v", result.Updated)
	}
	if len(result.Failed) != 1 || result.Failed[0].Name != "broken" || result.Failed[0].Error == "" {
		t.Fatalf("failed = %+v", result.Failed)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].Name != "current" {
		t.Fatalf("skipped = %+v", result.Skipped)
	}
	installed, err := os.ReadFile(filepath.Join(paths.InstallDir, PluginBinaryName("deploy")))
	if err != nil {
		t.Fatal(err)
	}
	if string(installed) != string(data) {
		t.Fatal("installed binary does not match the registry binary")
	}
}
