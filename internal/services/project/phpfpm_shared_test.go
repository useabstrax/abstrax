package project

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	executil "abstrax/internal/exec"
	"abstrax/internal/platform"
)

// rhelSharedProvider forces RHEL-family Profile so shared-pool logic runs in tests.
type rhelSharedProvider struct {
	runtimeTestProvider
}

func (p rhelSharedProvider) Profile() platform.Profile {
	prof := p.Provider.Profile()
	prof.Family = "rhel"
	return prof
}

func (p rhelSharedProvider) PHPFPMBinary(string) string { return "php-fpm" }

func TestEnsureSharedPHPPoolRHEL(t *testing.T) {
	root, err := os.MkdirTemp("/tmp", "abx")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })

	old := platformProvider
	platformProvider = rhelSharedProvider{runtimeTestProvider{Provider: old, root: root}}
	t.Cleanup(func() { platformProvider = old })

	if !needsManagedSharedPHPPool() {
		t.Fatal("expected RHEL shared pool management")
	}

	svc := New(false, false)
	var reloads []string
	svc.phpfpmReloader = func(_ context.Context, version string) error {
		reloads = append(reloads, version)
		return nil
	}
	svc.runner.SetCommandFunc(func(_ context.Context, name string, args []string) (executil.Result, bool, error) {
		if strings.Contains(name, "php-fpm") && len(args) == 1 && args[0] == "--test" {
			return executil.Result{}, true, nil
		}
		t.Fatalf("unexpected command: %s %v", name, args)
		return executil.Result{}, false, nil
	})

	id := RuntimeIdentity{
		Mode:          OwnershipShared,
		User:          "nginx",
		Group:         "nginx",
		WebServerUser: "nginx",
	}
	conf, err := svc.ensureSharedPHPPool(context.Background(), "8.5", id)
	if err != nil {
		t.Fatal(err)
	}
	if conf == nil || conf.PoolName != sharedPHPPoolName {
		t.Fatalf("conf = %#v", conf)
	}
	data, err := os.ReadFile(conf.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		"[abstrax-web]",
		"user = nginx",
		"group = nginx",
		"listen.owner = nginx",
		"listen.group = nginx",
		"listen = " + conf.SocketPath,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("missing %q in:\n%s", want, content)
		}
	}
	if len(reloads) != 1 || reloads[0] != "8.5" {
		t.Fatalf("reloads = %v", reloads)
	}

	// Second call is a no-op and does not rewrite or reload.
	reloads = nil
	again, err := svc.ensureSharedPHPPool(context.Background(), "8.5", id)
	if err != nil {
		t.Fatal(err)
	}
	if again.ConfigPath != conf.ConfigPath {
		t.Fatalf("path changed: %q vs %q", again.ConfigPath, conf.ConfigPath)
	}
	if len(reloads) != 0 {
		t.Fatalf("expected no reload on existing pool, got %v", reloads)
	}
}

func TestRemovePHPPoolSkipsSharedWebPool(t *testing.T) {
	root, err := os.MkdirTemp("/tmp", "abx")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })

	old := platformProvider
	platformProvider = rhelSharedProvider{runtimeTestProvider{Provider: old, root: root}}
	t.Cleanup(func() { platformProvider = old })

	conf := sharedPHPPoolPaths("8.5")
	if err := os.MkdirAll(filepath.Dir(conf.ConfigPath), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(conf.ConfigPath, []byte("[abstrax-web]\n"), 0644); err != nil {
		t.Fatal(err)
	}

	svc := New(false, false)
	svc.phpfpmReloader = func(context.Context, string) error {
		t.Fatal("should not reload when skipping shared pool removal")
		return nil
	}
	state := &State{
		Name:          "demo",
		PHPVersion:    "8.5",
		PHPPoolName:   sharedPHPPoolName,
		PHPSocketPath: conf.SocketPath,
	}
	if err := svc.removePHPPool(context.Background(), state); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(conf.ConfigPath); err != nil {
		t.Fatalf("shared pool should remain: %v", err)
	}
}

func TestPHPSocketForStateSharedRHEL(t *testing.T) {
	old := platformProvider
	platformProvider = rhelSharedProvider{runtimeTestProvider{Provider: old, root: "/tmp/abx-sock"}}
	t.Cleanup(func() { platformProvider = old })

	got := phpSocketForState(&State{PHPVersion: "8.5", OwnershipMode: OwnershipShared})
	want := sharedPHPPoolPaths("8.5").SocketPath
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
