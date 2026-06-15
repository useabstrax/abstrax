package plugin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func fixturePath(name string) string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		panic("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "openapi", "fixtures", name)
}

func loadFixture(t *testing.T, name string) json.RawMessage {
	t.Helper()
	data, err := os.ReadFile(fixturePath(name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return data
}

func TestRegistryClientFixtures(t *testing.T) {
	fixtures := map[string]string{
		"/registry":                              "registry-metadata.json",
		"/plugins":                               "plugin-list.json",
		"/plugins/example":                       "plugin-detail.json",
		"/plugins/example/versions/0.1.0":        "plugin-version.json",
		"/plugins/example/versions/latest":       "plugin-version.json",
		"/plugins/missing":                       "error-response.json",
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path == "/plugins/example/versions/latest" {
			w.Header().Set("Content-Type", "application/json")
			w.Write(loadFixture(t, "plugin-version.json"))
			return
		}
		if path == "/plugins/missing" {
			w.WriteHeader(http.StatusNotFound)
			w.Write(loadFixture(t, "error-response.json"))
			return
		}
		fixture, ok := fixtures[path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write(loadFixture(t, fixture))
	}))
	defer srv.Close()

	client := NewCachedRegistryClient(srv.URL, NewRegistryCache(t.TempDir(), 0))

	t.Run("GetRegistry", func(t *testing.T) {
		resp, err := client.GetRegistry(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if resp.Registry.Name != "Abstrax Plugin Registry" {
			t.Fatalf("registry name = %q", resp.Registry.Name)
		}
	})

	t.Run("ListPlugins", func(t *testing.T) {
		plugins, err := client.ListPlugins(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if len(plugins) != 1 || plugins[0].Publisher != "useabstrax" {
			t.Fatalf("plugins = %+v", plugins)
		}
	})

	t.Run("GetPlugin", func(t *testing.T) {
		plugin, err := client.GetPlugin(context.Background(), "example")
		if err != nil {
			t.Fatal(err)
		}
		if plugin.Publisher != "useabstrax" {
			t.Fatalf("publisher = %q", plugin.Publisher)
		}
	})

	t.Run("GetLatestVersion", func(t *testing.T) {
		version, err := client.GetLatestVersion(context.Background(), "example", LatestVersionOptions{
			AbstraxVersion: "0.2.0",
			Platform:       "linux-amd64",
			Channel:        "stable",
		})
		if err != nil {
			t.Fatal(err)
		}
		if version.Version != "0.1.0" {
			t.Fatalf("version = %q", version.Version)
		}
		if version.Platforms["linux-amd64"].Size != 123456 {
			t.Fatalf("size = %d", version.Platforms["linux-amd64"].Size)
		}
	})

	t.Run("GetVersion", func(t *testing.T) {
		version, err := client.GetVersion(context.Background(), "example", "0.1.0")
		if err != nil {
			t.Fatal(err)
		}
		if version.ProtocolVersion != 1 {
			t.Fatalf("protocol_version = %d", version.ProtocolVersion)
		}
	})

	t.Run("PluginNotFound", func(t *testing.T) {
		_, err := client.GetPlugin(context.Background(), "missing")
		if err == nil || !strings.Contains(err.Error(), ErrRegistryPluginNotFound.Error()) {
			t.Fatalf("expected ErrRegistryPluginNotFound, got %v", err)
		}
	})
}

func TestParseRegistryHTTPError(t *testing.T) {
	body := []byte(`{"error":{"code":"unsupported_platform","message":"No binary is available for the requested platform."}}`)
	err := parseRegistryHTTPError("http://example.test/latest", http.StatusNotFound, body)
	if err == nil || !strings.Contains(err.Error(), ErrUnsupportedPlatform.Error()) {
		t.Fatalf("got %v", err)
	}
}
