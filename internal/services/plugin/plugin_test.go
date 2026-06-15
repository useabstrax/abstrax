package plugin

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPluginBinaryName(t *testing.T) {
	if got := PluginBinaryName("deploy"); got != "abstrax-deploy" {
		t.Fatalf("got %q, want abstrax-deploy", got)
	}
}

func TestDiscovererSearchOrder(t *testing.T) {
	dir := t.TempDir()
	system := filepath.Join(dir, "system")
	user := filepath.Join(dir, "user")
	alt := filepath.Join(dir, "alt")
	for _, d := range []string{system, user, alt} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatal(err)
		}
	}

	systemBinary := filepath.Join(system, "abstrax-example")
	if err := os.WriteFile(systemBinary, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	userBinary := filepath.Join(user, "abstrax-example")
	if err := os.WriteFile(userBinary, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	paths := &Paths{
		SystemPluginDirs: []string{system, alt},
		UserPluginDir:    user,
		InstallDir:       user,
	}
	d := NewDiscoverer(paths)
	got, err := d.FindBinary("example")
	if err != nil {
		t.Fatal(err)
	}
	if got != systemBinary {
		t.Fatalf("got %q, want %q", got, systemBinary)
	}
}

func TestDiscovererPATHFallback(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "abstrax-backup")
	if err := os.WriteFile(binary, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", dir)
	paths := &Paths{SystemPluginDirs: []string{filepath.Join(dir, "empty")}}
	d := NewDiscoverer(paths)
	got, err := d.FindBinary("backup")
	if err != nil {
		t.Fatal(err)
	}
	if got != binary {
		t.Fatalf("got %q, want %q", got, binary)
	}
}

func TestDiscovererNotFound(t *testing.T) {
	dir := t.TempDir()
	paths := &Paths{SystemPluginDirs: []string{dir}}
	d := NewDiscoverer(paths)
	_, err := d.FindBinary("missing")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateMetadata(t *testing.T) {
	meta := &Metadata{
		ProtocolVersion: ProtocolVersion,
		Name:            "example",
		DisplayName:     "Example",
		Description:     "An example plugin",
		Version:         "0.1.0",
		RequiresAbstrax: ">=0.0.0",
		Commands:        []MetadataCommand{{Name: "hello", Description: "Greet"}},
	}
	if err := ValidateMetadata(meta, "example"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateMetadataUnsupportedProtocol(t *testing.T) {
	meta := &Metadata{
		ProtocolVersion: 99,
		Name:            "example",
		DisplayName:     "Example",
		Version:         "0.1.0",
		RequiresAbstrax: ">=0.0.0",
	}
	if err := ValidateMetadata(meta, "example"); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateMetadataNameMismatch(t *testing.T) {
	meta := &Metadata{
		ProtocolVersion: ProtocolVersion,
		Name:            "other",
		DisplayName:     "Example",
		Version:         "0.1.0",
		RequiresAbstrax: ">=0.0.0",
	}
	if err := ValidateMetadata(meta, "example"); err == nil {
		t.Fatal("expected error")
	}
}

func TestVerifyFileSHA256(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bin")
	content := []byte("plugin-binary")
	if err := os.WriteFile(path, content, 0o755); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(content)
	expected := hex.EncodeToString(sum[:])
	if err := verifyFileSHA256(path, expected); err != nil {
		t.Fatal(err)
	}
	if err := verifyFileSHA256(path, "deadbeef"); err == nil {
		t.Fatal("expected checksum mismatch")
	}
}

func TestAtomicInstall(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "abstrax-example-new")
	dest := filepath.Join(dir, "abstrax-example")
	if err := os.WriteFile(dest, []byte("old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("new"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := atomicInstall(src, dest); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "new" {
		t.Fatalf("got %q, want new", data)
	}
}

func TestDispatchArgumentPassthrough(t *testing.T) {
	dir := t.TempDir()
	binary := buildTestPlugin(t, dir, "example")
	paths := testPaths(dir)
	store := NewStore(paths.RecordDir)
	dispatcher, err := NewDispatcher(paths, store)
	if err != nil {
		t.Fatal(err)
	}

	exitCode, err := dispatcher.Dispatch(context.Background(), "example", []string{"hello", "Abstrax"}, DispatchOptions{})
	if err != nil {
		t.Fatalf("dispatch failed: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("exit code %d, want 0", exitCode)
	}
	_ = binary
}

func TestDispatchExitCode(t *testing.T) {
	dir := t.TempDir()
	buildFailingPlugin(t, dir, "fail")
	paths := testPaths(dir)
	store := NewStore(paths.RecordDir)
	dispatcher, err := NewDispatcher(paths, store)
	if err != nil {
		t.Fatal(err)
	}

	exitCode, err := dispatcher.Dispatch(context.Background(), "fail", []string{"run"}, DispatchOptions{})
	if err == nil {
		t.Fatal("expected exit error")
	}
	if exitCode != 42 {
		t.Fatalf("exit code %d, want 42", exitCode)
	}
}

func TestDispatchBlockedPlugin(t *testing.T) {
	dir := t.TempDir()
	buildTestPlugin(t, dir, "blocked")
	paths := testPaths(dir)
	store := NewStore(paths.RecordDir)
	if err := store.Save(&InstallRecord{
		Name:           "blocked",
		Version:        "1.0.0",
		RegistryStatus: StatusBlocked,
		BinaryPath:     filepath.Join(dir, "plugins", "abstrax-blocked"),
	}); err != nil {
		t.Fatal(err)
	}
	dispatcher, err := NewDispatcher(paths, store)
	if err != nil {
		t.Fatal(err)
	}
	_, err = dispatcher.Dispatch(context.Background(), "blocked", nil, DispatchOptions{})
	if err == nil {
		t.Fatal("expected blocked error")
	}
}

func TestDispatchAllowBlockedOverride(t *testing.T) {
	dir := t.TempDir()
	buildTestPlugin(t, dir, "blocked")
	paths := testPaths(dir)
	store := NewStore(paths.RecordDir)
	if err := store.Save(&InstallRecord{
		Name:           "blocked",
		Version:        "1.0.0",
		RegistryStatus: StatusBlocked,
		BinaryPath:     filepath.Join(dir, "plugins", "abstrax-blocked"),
	}); err != nil {
		t.Fatal(err)
	}
	dispatcher, err := NewDispatcher(paths, store)
	if err != nil {
		t.Fatal(err)
	}
	exitCode, err := dispatcher.Dispatch(context.Background(), "blocked", []string{"hello"}, DispatchOptions{
		AllowBlocked: []string{"blocked"},
	})
	if err != nil {
		t.Fatalf("dispatch failed: %v", err)
	}
	if exitCode != 0 {
		t.Fatalf("exit code %d, want 0", exitCode)
	}
}

func TestRegistryInstallAndRemove(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping install test in short mode")
	}
	platform, err := CurrentPlatform()
	if err != nil {
		t.Skipf("skipping on unsupported platform: %v", err)
	}
	_ = platform
	dir := t.TempDir()
	pluginBin := buildTestPlugin(t, dir, "deploy")
	data, err := os.ReadFile(pluginBin)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	checksum := hex.EncodeToString(sum[:])

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/plugins/deploy":
			json.NewEncoder(w).Encode(RegistryPlugin{
				Name: "deploy", Publisher: "useabstrax", TrustLevel: TrustOfficial, Status: StatusActive,
			})
		case "/plugins/deploy/versions/latest":
			q := r.URL.Query()
			if q.Get("channel") == "" {
				t.Fatalf("expected channel query parameter")
			}
			json.NewEncoder(w).Encode(RegistryVersion{
				Version: "1.0.0", RequiresAbstrax: ">=0.1.0", Stable: true, Channel: "stable", ProtocolVersion: 1,
				Platforms: map[string]RegistryPlatformBinary{
					"linux-amd64": {URL: "http://" + r.Host + "/binary", SHA256: checksum, Size: int64(len(data))},
					"linux-arm64": {URL: "http://" + r.Host + "/binary", SHA256: checksum, Size: int64(len(data))},
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
	result, err := svc.Install(context.Background(), InstallOptions{Name: "deploy"})
	if err != nil {
		t.Fatalf("install failed: %v", err)
	}
	if result.Version != "1.0.0" {
		t.Fatalf("version %q, want 1.0.0", result.Version)
	}
	if err := svc.Remove("deploy"); err != nil {
		t.Fatal(err)
	}
}

func TestRegistryBlockedInstall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(RegistryPlugin{
			Name: "bad", Status: StatusBlocked,
		})
	}))
	defer srv.Close()

	dir := t.TempDir()
	svc := NewWithPaths(testPaths(dir), srv.URL)
	_, err := svc.Install(context.Background(), InstallOptions{Name: "bad"})
	if err == nil {
		t.Fatal("expected blocked install error")
	}
}

func TestRegistryCache(t *testing.T) {
	dir := t.TempDir()
	requests := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		json.NewEncoder(w).Encode(RegistryPluginsResponse{
			Plugins: []RegistryPluginSummary{{Name: "deploy", LatestVersion: "1.0.0", Publisher: "useabstrax"}},
			Meta:    RegistryPagination{CurrentPage: 1, LastPage: 1, PerPage: 20, Total: 1},
		})
	}))
	defer srv.Close()

	cache := NewRegistryCache(filepath.Join(dir, "cache"), RegistryCacheTTL)
	client := NewCachedRegistryClient(srv.URL, cache)
	if _, err := client.ListPlugins(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ListPlugins(context.Background()); err != nil {
		t.Fatal(err)
	}
	if requests != 1 {
		t.Fatalf("requests %d, want 1", requests)
	}
}

func testPaths(dir string) *Paths {
	pluginDir := filepath.Join(dir, "plugins")
	return &Paths{
		SystemPluginDirs: []string{pluginDir},
		UserPluginDir:    pluginDir,
		RecordDir:        filepath.Join(dir, "records"),
		MetadataCache:    filepath.Join(dir, "cache", "metadata.json"),
		RegistryCacheDir: filepath.Join(dir, "cache", "registry"),
		InstallDir:       pluginDir,
	}
}

func buildTestPlugin(t *testing.T, dir, name string) string {
	t.Helper()
	pluginDir := filepath.Join(dir, "plugins")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(pluginDir, "abstrax-"+name)
	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "plugin" ] && [ "$2" = "metadata" ]; then
  cat <<'EOF'
{"protocol_version":1,"name":%q,"display_name":"Test","description":"Test plugin","version":"1.0.0","requires_abstrax":">=0.0.0","commands":[{"name":"hello","description":"Hello"}]}
EOF
  exit 0
fi
if [ "$1" = "hello" ]; then
  echo "Hello, ${2:-world}!"
  exit 0
fi
exit 0
`, name)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func buildFailingPlugin(t *testing.T, dir, name string) string {
	t.Helper()
	pluginDir := filepath.Join(dir, "plugins")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(pluginDir, "abstrax-"+name)
	script := fmt.Sprintf(`#!/bin/sh
if [ "$1" = "plugin" ] && [ "$2" = "metadata" ]; then
  cat <<'EOF'
{"protocol_version":1,"name":%q,"display_name":"Fail","description":"Fail","version":"1.0.0","requires_abstrax":">=0.0.0","commands":[]}
EOF
  exit 0
fi
if [ "$1" = "run" ]; then
  exit 42
fi
exit 0
`, name)
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestFetchMetadataFromGoExample(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not available")
	}
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("could not determine test path")
	}
	cliRoot := filepath.Join(filepath.Dir(filename), "..", "..", "..")

	dir := t.TempDir()
	out := filepath.Join(dir, "abstrax-example")
	cmd := exec.Command("go", "build", "-o", out, "./cmd/abstrax-example")
	cmd.Dir = cliRoot
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("could not build example plugin: %v: %s", err, output)
	}

	meta, err := FetchMetadata(context.Background(), out)
	if err != nil {
		t.Fatal(err)
	}
	if meta.Name != "example" || meta.ProtocolVersion != ProtocolVersion {
		t.Fatalf("unexpected metadata: %+v", meta)
	}
	if !strings.Contains(meta.DisplayName, "Example") {
		t.Fatalf("display name %q", meta.DisplayName)
	}
}
