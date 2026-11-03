package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	executil "abstrax/internal/exec"
	"abstrax/internal/platform"
)

// runtimeTestProvider keeps PHP-FPM pool and socket paths inside a temp dir.
type runtimeTestProvider struct {
	platform.Provider
	root string
}

func (p runtimeTestProvider) PHPFPMPoolDir(version string) string {
	return filepath.Join(p.root, "etc", "php", version, "fpm", "pool.d")
}

func (p runtimeTestProvider) PHPFPMDefaultSocket(version string) string {
	return filepath.Join(p.root, "run", "php"+version+"-fpm.sock")
}

func (p runtimeTestProvider) PHPFPMProjectSocket(version, suffix string) string {
	return filepath.Join(p.root, "run", "php"+version+"-fpm-"+suffix+".sock")
}

type runtimeFixture struct {
	t         *testing.T
	root      string
	svc       *Service
	events    []string
	nginxTest error
	vhostPath string
}

func newRuntimeFixture(t *testing.T) *runtimeFixture {
	t.Helper()
	// Unix socket paths are length limited, so keep the root short.
	root, err := os.MkdirTemp("/tmp", "abx")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })

	oldProvider := platformProvider
	platformProvider = runtimeTestProvider{Provider: oldProvider, root: root}
	t.Cleanup(func() { platformProvider = oldProvider })

	f := &runtimeFixture{t: t, root: root}
	svc := New(false, false)
	svc.stateDir = filepath.Join(root, "state")
	svc.nginxLayout = platform.NginxSitesAvailableEnabled
	svc.nginxAvail = filepath.Join(root, "nginx", "sites-available")
	svc.nginxEnabled = filepath.Join(root, "nginx", "sites-enabled")
	svc.nginxConfDir = filepath.Join(root, "nginx", "conf.d")
	svc.nginxLogDir = filepath.Join(root, "log")
	svc.runtimeInstalled = func(RuntimeSpec) bool { return true }
	svc.phpfpmReloader = func(_ context.Context, version string) error {
		f.events = append(f.events, "fpm-reload:"+version)
		return nil
	}
	svc.runner.SetCommandFunc(func(_ context.Context, name string, args []string) (executil.Result, bool, error) {
		joined := name + " " + strings.Join(args, " ")
		switch {
		case name == "nginx" && len(args) == 1 && args[0] == "-t":
			f.events = append(f.events, "nginx-test")
			if f.nginxTest != nil {
				return executil.Result{ExitCode: 1, Stderr: "nginx: configuration file test failed"}, true, f.nginxTest
			}
			return executil.Result{}, true, nil
		case name == "nginx":
			f.events = append(f.events, "nginx-reload")
			return executil.Result{}, true, nil
		case strings.HasPrefix(name, "php-fpm"):
			return executil.Result{}, true, nil
		default:
			f.t.Fatalf("unexpected command: %s", joined)
			return executil.Result{}, false, nil
		}
	})
	f.svc = svc
	return f
}

// addProject writes state plus an enabled vhost, as Add would have.
func (f *runtimeFixture) addProject(state *State) *State {
	f.t.Helper()
	if state.Name == "" {
		state.Name = "demo"
	}
	if state.Path == "" {
		state.Path = filepath.Join(f.root, "var", "www", state.Name)
	}
	state.WebServer = WebServerNginx
	state.Domains = []string{"demo.test"}
	state.CreatedAt = time.Now().Add(-time.Hour)
	state.UpdatedAt = state.CreatedAt
	if state.OwnershipMode == OwnershipIsolated {
		state.Owner = "mike"
		state.Group = "mike"
		state.OwnerUID = 1000
		state.OwnerGID = 1000
	}

	if state.Runtime == RuntimePHP && state.OwnershipMode == OwnershipIsolated {
		conf := buildPHPPoolPaths(state.Name, state.PHPVersion)
		f.writePool(conf)
		state.PHPPoolName = conf.PoolName
		state.PHPSocketPath = conf.SocketPath
	}

	conf, err := f.svc.renderNginxConfig(state.vhostConfig())
	if err != nil {
		f.t.Fatal(err)
	}
	f.vhostPath = filepath.Join(f.svc.nginxAvail, "abstrax-"+state.Name)
	state.VhostPath = f.vhostPath
	if err := os.MkdirAll(f.svc.nginxAvail, 0755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(f.vhostPath, []byte(conf), 0644); err != nil {
		f.t.Fatal(err)
	}
	if err := f.svc.enableVhostFile(f.vhostPath); err != nil {
		f.t.Fatal(err)
	}

	if err := f.svc.saveState(state); err != nil {
		f.t.Fatal(err)
	}
	return state
}

func (f *runtimeFixture) writePool(conf PHPPoolConfig) {
	f.t.Helper()
	if err := os.MkdirAll(filepath.Dir(conf.ConfigPath), 0755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(conf.ConfigPath, []byte("[pool]\n"), 0644); err != nil {
		f.t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(conf.SocketPath), 0755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(conf.SocketPath, nil, 0644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *runtimeFixture) vhost() string {
	f.t.Helper()
	data, err := os.ReadFile(f.vhostPath)
	if err != nil {
		f.t.Fatal(err)
	}
	return string(data)
}

func (f *runtimeFixture) stored(name string) *State {
	f.t.Helper()
	state, err := f.svc.Info(context.Background(), name)
	if err != nil {
		f.t.Fatal(err)
	}
	return state
}

func indexOf(events []string, want string) int {
	for i, e := range events {
		if e == want {
			return i
		}
	}
	return -1
}

func TestSetRuntimePHPToNodeIsolatedRemovesPool(t *testing.T) {
	f := newRuntimeFixture(t)
	old := f.addProject(&State{
		Runtime:       RuntimePHP,
		PHPVersion:    "8.4",
		PublicDir:     "public",
		OwnershipMode: OwnershipIsolated,
	})
	pool := old.PHPPoolPath()
	if !fileExists(pool) || !fileExists(old.PHPSocketPath) {
		t.Fatalf("fixture pool missing: %s", pool)
	}

	got, err := f.svc.SetRuntime(context.Background(), SetRuntimeOptions{
		Name:        "demo",
		Runtime:     RuntimeNode,
		NodeVersion: "22",
		ProxyPort:   4000,
	})
	if err != nil {
		t.Fatalf("SetRuntime: %v", err)
	}

	if fileExists(pool) {
		t.Fatalf("pool config %s should be removed", pool)
	}
	if fileExists(old.PHPSocketPath) {
		t.Fatalf("pool socket %s should be removed", old.PHPSocketPath)
	}
	if indexOf(f.events, "fpm-reload:8.4") < 0 {
		t.Fatalf("expected php-fpm 8.4 reload, events=%v", f.events)
	}

	for _, st := range []*State{got, f.stored("demo")} {
		if st.Runtime != RuntimeNode || st.NodeVersion != "22" || st.ProxyPort != 4000 {
			t.Fatalf("node fields wrong: %#v", st)
		}
		if st.PHPVersion != "" || st.PHPPoolName != "" || st.PHPSocketPath != "" || st.PublicDir != "" || st.RubyVersion != "" {
			t.Fatalf("stale fields left behind: %#v", st)
		}
		if st.Owner != "mike" || st.OwnershipMode != OwnershipIsolated || st.VhostPath != f.vhostPath {
			t.Fatalf("identity/vhost changed: %#v", st)
		}
	}
	if !got.UpdatedAt.After(old.UpdatedAt) {
		t.Fatalf("updated_at not advanced")
	}

	vhost := f.vhost()
	if !strings.Contains(vhost, "proxy_pass http://127.0.0.1:4000;") || strings.Contains(vhost, "fastcgi_pass") {
		t.Fatalf("vhost not regenerated for node:\n%s", vhost)
	}

	// Inspect reads the same state.
	savedSupervisorDir := supervisorConfDirPath
	supervisorConfDirPath = t.TempDir()
	defer func() { supervisorConfDirPath = savedSupervisorDir }()
	resp, err := f.svc.Inspect(context.Background(), "demo")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Project.Runtime.Type != "node" || resp.Project.Runtime.Version != "22" {
		t.Fatalf("inspect runtime = %#v", resp.Project.Runtime)
	}
}

func TestSetRuntimeNodeToPHPIsolatedCreatesPool(t *testing.T) {
	f := newRuntimeFixture(t)
	f.addProject(&State{
		Runtime:       RuntimeNode,
		NodeVersion:   "22",
		ProxyPort:     4000,
		OwnershipMode: OwnershipIsolated,
	})

	got, err := f.svc.SetRuntime(context.Background(), SetRuntimeOptions{
		Name:       "demo",
		Runtime:    RuntimePHP,
		PHPVersion: "8.4",
		PublicDir:  "public",
	})
	if err != nil {
		t.Fatalf("SetRuntime: %v", err)
	}

	conf := buildPHPPoolPaths("demo", "8.4")
	data, err := os.ReadFile(conf.ConfigPath)
	if err != nil {
		t.Fatalf("pool not created: %v", err)
	}
	if !strings.Contains(string(data), "user = mike") || !strings.Contains(string(data), "listen = "+conf.SocketPath) {
		t.Fatalf("unexpected pool config:\n%s", data)
	}

	st := f.stored("demo")
	if got.PHPPoolName != conf.PoolName || st.PHPPoolName != conf.PoolName || st.PHPSocketPath != conf.SocketPath {
		t.Fatalf("pool fields not saved: %#v", st)
	}
	if st.Runtime != RuntimePHP || st.PHPVersion != "8.4" || st.PublicDir != "public" {
		t.Fatalf("php fields wrong: %#v", st)
	}
	if st.NodeVersion != "" || st.ProxyPort != 0 || st.RubyVersion != "" {
		t.Fatalf("stale fields left behind: %#v", st)
	}

	vhost := f.vhost()
	if !strings.Contains(vhost, "fastcgi_pass unix:"+conf.SocketPath+";") || strings.Contains(vhost, "proxy_pass") {
		t.Fatalf("vhost not regenerated for php:\n%s", vhost)
	}
	if !strings.Contains(vhost, filepath.Join(st.Path, "public")) {
		t.Fatalf("vhost root missing public dir:\n%s", vhost)
	}
}

func TestSetRuntimePHPVersionChangeSwitchesPoolSafely(t *testing.T) {
	f := newRuntimeFixture(t)
	old := f.addProject(&State{
		Runtime:       RuntimePHP,
		PHPVersion:    "8.4",
		PublicDir:     "public",
		OwnershipMode: OwnershipIsolated,
	})
	oldPool := old.PHPPoolPath()

	got, err := f.svc.SetRuntime(context.Background(), SetRuntimeOptions{
		Name:       "demo",
		Runtime:    RuntimePHP,
		PHPVersion: "8.5",
	})
	if err != nil {
		t.Fatalf("SetRuntime: %v", err)
	}

	newConf := buildPHPPoolPaths("demo", "8.5")
	if !fileExists(newConf.ConfigPath) {
		t.Fatalf("new pool missing")
	}
	if fileExists(oldPool) || fileExists(old.PHPSocketPath) {
		t.Fatalf("old pool should be removed")
	}
	if got.PHPVersion != "8.5" || got.PHPSocketPath != newConf.SocketPath || got.PublicDir != "public" {
		t.Fatalf("state wrong: %#v", got)
	}
	if !strings.Contains(f.vhost(), "fastcgi_pass unix:"+newConf.SocketPath+";") {
		t.Fatalf("vhost still points at old socket:\n%s", f.vhost())
	}

	newReload := indexOf(f.events, "fpm-reload:8.5")
	vhostReload := indexOf(f.events, "nginx-reload")
	oldReload := indexOf(f.events, "fpm-reload:8.4")
	if newReload < 0 || vhostReload < 0 || oldReload < 0 || !(newReload < vhostReload && vhostReload < oldReload) {
		t.Fatalf("want new pool, then vhost, then old pool removal; events=%v", f.events)
	}
}

func TestSetRuntimePHPSameVersionKeepsPool(t *testing.T) {
	f := newRuntimeFixture(t)
	old := f.addProject(&State{
		Runtime:       RuntimePHP,
		PHPVersion:    "8.4",
		OwnershipMode: OwnershipIsolated,
	})

	got, err := f.svc.SetRuntime(context.Background(), SetRuntimeOptions{
		Name:      "demo",
		Runtime:   RuntimePHP,
		PublicDir: "web",
	})
	if err != nil {
		t.Fatalf("SetRuntime: %v", err)
	}
	if got.PHPVersion != "8.4" || got.PublicDir != "web" || got.PHPPoolName != old.PHPPoolName || got.PHPSocketPath != old.PHPSocketPath {
		t.Fatalf("state wrong: %#v", got)
	}
	if !fileExists(old.PHPPoolPath()) {
		t.Fatalf("pool should be kept")
	}
	if indexOf(f.events, "fpm-reload:8.4") >= 0 {
		t.Fatalf("pool should not be touched, events=%v", f.events)
	}
}

func TestSetRuntimeSharedModeLeavesPoolsAlone(t *testing.T) {
	f := newRuntimeFixture(t)
	f.addProject(&State{
		Runtime:    RuntimePHP,
		PHPVersion: "8.4",
		PublicDir:  "public",
	})

	if _, err := f.svc.SetRuntime(context.Background(), SetRuntimeOptions{Name: "demo", Runtime: RuntimeRuby}); err != nil {
		t.Fatalf("php->ruby: %v", err)
	}
	st := f.stored("demo")
	if st.Runtime != RuntimeRuby || st.RubyVersion != DefaultRubyVersion || st.PHPVersion != "" || st.PublicDir != "" {
		t.Fatalf("state wrong: %#v", st)
	}

	if _, err := f.svc.SetRuntime(context.Background(), SetRuntimeOptions{Name: "demo", Runtime: RuntimePHP}); err != nil {
		t.Fatalf("ruby->php: %v", err)
	}
	st = f.stored("demo")
	if st.Runtime != RuntimePHP || st.PHPVersion != DefaultPHPVersion || st.RubyVersion != "" || st.PHPPoolName != "" || st.PHPSocketPath != "" {
		t.Fatalf("state wrong: %#v", st)
	}
	if strings.Contains(strings.Join(f.events, ","), "fpm-reload") {
		t.Fatalf("shared mode must not touch pools, events=%v", f.events)
	}
	want := platformProvider.PHPFPMDefaultSocket(DefaultPHPVersion)
	if !strings.Contains(f.vhost(), "fastcgi_pass unix:"+want+";") {
		t.Fatalf("shared vhost should use default socket %s:\n%s", want, f.vhost())
	}
}

func TestSetRuntimeFieldClearing(t *testing.T) {
	f := newRuntimeFixture(t)
	f.addProject(&State{
		Runtime:     RuntimeNode,
		NodeVersion: "22",
		ProxyPort:   4100,
	})
	ctx := context.Background()

	// node -> ruby keeps the proxy port and drops node_version.
	if _, err := f.svc.SetRuntime(ctx, SetRuntimeOptions{Name: "demo", Runtime: RuntimeRuby, RubyVersion: "3.4"}); err != nil {
		t.Fatal(err)
	}
	st := f.stored("demo")
	if st.Runtime != RuntimeRuby || st.RubyVersion != "3.4" || st.ProxyPort != 4100 || st.NodeVersion != "" {
		t.Fatalf("node->ruby: %#v", st)
	}

	// ruby -> php clears proxy port and ruby_version.
	if _, err := f.svc.SetRuntime(ctx, SetRuntimeOptions{Name: "demo", Runtime: RuntimePHP, PublicDir: "public"}); err != nil {
		t.Fatal(err)
	}
	st = f.stored("demo")
	if st.Runtime != RuntimePHP || st.ProxyPort != 0 || st.RubyVersion != "" || st.PublicDir != "public" {
		t.Fatalf("ruby->php: %#v", st)
	}

	// php -> static clears the version but keeps public_dir.
	if _, err := f.svc.SetRuntime(ctx, SetRuntimeOptions{Name: "demo", Runtime: RuntimeStatic}); err != nil {
		t.Fatal(err)
	}
	st = f.stored("demo")
	if st.Runtime != RuntimeStatic || st.PHPVersion != "" || st.PublicDir != "public" || st.ProxyPort != 0 {
		t.Fatalf("php->static: %#v", st)
	}
	vhost := f.vhost()
	if strings.Contains(vhost, "fastcgi_pass") || strings.Contains(vhost, "proxy_pass") || !strings.Contains(vhost, "try_files $uri $uri/ =404;") {
		t.Fatalf("static vhost wrong:\n%s", vhost)
	}

	// static -> node: public_dir is cleared and the proxy port starts unset.
	if _, err := f.svc.SetRuntime(ctx, SetRuntimeOptions{Name: "demo", Runtime: RuntimeNode}); err != nil {
		t.Fatal(err)
	}
	st = f.stored("demo")
	if st.Runtime != RuntimeNode || st.NodeVersion != DefaultNodeVersion || st.PublicDir != "" || st.ProxyPort != 0 {
		t.Fatalf("static->node: %#v", st)
	}
}

func TestSetRuntimeEnsuresRuntimeBeforeMutating(t *testing.T) {
	f := newRuntimeFixture(t)
	f.addProject(&State{
		Runtime:       RuntimeNode,
		NodeVersion:   "22",
		ProxyPort:     4000,
		OwnershipMode: OwnershipIsolated,
	})
	before := f.vhost()

	var specs []RuntimeSpec
	f.svc.runtimeInstalled = func(spec RuntimeSpec) bool {
		specs = append(specs, spec)
		if f.vhost() != before || fileExists(buildPHPPoolPaths("demo", "8.4").ConfigPath) {
			t.Errorf("runtime was checked after files were changed")
		}
		return true
	}

	if _, err := f.svc.SetRuntime(context.Background(), SetRuntimeOptions{Name: "demo", Runtime: RuntimePHP, PHPVersion: "8.4"}); err != nil {
		t.Fatal(err)
	}
	if len(specs) != 1 || specs[0] != (RuntimeSpec{Runtime: RuntimePHP, Version: "8.4"}) {
		t.Fatalf("ensureRuntime specs = %#v", specs)
	}
}

func TestSetRuntimeRollsBackWhenNginxTestFails(t *testing.T) {
	t.Run("node to php removes the new pool", func(t *testing.T) {
		f := newRuntimeFixture(t)
		old := f.addProject(&State{
			Runtime:       RuntimeNode,
			NodeVersion:   "22",
			ProxyPort:     4000,
			OwnershipMode: OwnershipIsolated,
		})
		beforeVhost := f.vhost()
		beforeState := mustRead(t, f.svc.statePath("demo"))
		f.nginxTest = errors.New("exit status 1")

		got, err := f.svc.SetRuntime(context.Background(), SetRuntimeOptions{Name: "demo", Runtime: RuntimePHP, PHPVersion: "8.4"})
		if err == nil || !strings.Contains(err.Error(), "nginx config validation failed") {
			t.Fatalf("err = %v", err)
		}
		if got != nil {
			t.Fatalf("state returned on failure: %#v", got)
		}
		if f.vhost() != beforeVhost {
			t.Fatalf("vhost bytes changed:\n%s", f.vhost())
		}
		if fileExists(buildPHPPoolPaths("demo", "8.4").ConfigPath) {
			t.Fatalf("new pool should be removed")
		}
		if mustRead(t, f.svc.statePath("demo")) != beforeState {
			t.Fatalf("state file changed")
		}
		if old.Runtime != RuntimeNode {
			t.Fatalf("in-memory state changed: %#v", old)
		}
		if _, err := os.Lstat(filepath.Join(f.svc.nginxEnabled, "abstrax-demo")); err != nil {
			t.Fatalf("vhost should still be enabled: %v", err)
		}
	})

	t.Run("php to node keeps the old pool", func(t *testing.T) {
		f := newRuntimeFixture(t)
		old := f.addProject(&State{
			Runtime:       RuntimePHP,
			PHPVersion:    "8.4",
			OwnershipMode: OwnershipIsolated,
		})
		beforeVhost := f.vhost()
		beforeState := mustRead(t, f.svc.statePath("demo"))
		f.nginxTest = errors.New("exit status 1")

		if _, err := f.svc.SetRuntime(context.Background(), SetRuntimeOptions{Name: "demo", Runtime: RuntimeNode}); err == nil {
			t.Fatal("expected error")
		}
		if f.vhost() != beforeVhost || mustRead(t, f.svc.statePath("demo")) != beforeState {
			t.Fatalf("vhost or state changed")
		}
		if !fileExists(old.PHPPoolPath()) || !fileExists(old.PHPSocketPath) {
			t.Fatalf("old pool must survive a failed change")
		}
	})

	t.Run("php version change keeps old pool and drops new", func(t *testing.T) {
		f := newRuntimeFixture(t)
		old := f.addProject(&State{
			Runtime:       RuntimePHP,
			PHPVersion:    "8.4",
			OwnershipMode: OwnershipIsolated,
		})
		beforeVhost := f.vhost()
		f.nginxTest = errors.New("exit status 1")

		if _, err := f.svc.SetRuntime(context.Background(), SetRuntimeOptions{Name: "demo", Runtime: RuntimePHP, PHPVersion: "8.5"}); err == nil {
			t.Fatal("expected error")
		}
		if f.vhost() != beforeVhost {
			t.Fatalf("vhost bytes changed")
		}
		if !fileExists(old.PHPPoolPath()) {
			t.Fatalf("old pool must survive")
		}
		if fileExists(buildPHPPoolPaths("demo", "8.5").ConfigPath) {
			t.Fatalf("new pool should be removed")
		}
		if st := f.stored("demo"); st.PHPVersion != "8.4" {
			t.Fatalf("stored version = %q", st.PHPVersion)
		}
	})
}

func TestSetRuntimeRollsBackWhenStateSaveFails(t *testing.T) {
	f := newRuntimeFixture(t)
	f.addProject(&State{
		Runtime:       RuntimeNode,
		NodeVersion:   "22",
		OwnershipMode: OwnershipIsolated,
	})
	beforeVhost := f.vhost()

	// Make the state file unwritable by turning it into a directory after the
	// vhost step has started. The reload hook runs during the vhost update.
	statePath := f.svc.statePath("demo")
	f.svc.runner.SetCommandFunc(func(_ context.Context, name string, args []string) (executil.Result, bool, error) {
		if name == "nginx" && len(args) == 2 && args[1] == "reload" && fileExists(statePath) {
			if err := os.Remove(statePath); err != nil {
				t.Fatal(err)
			}
			if err := os.Mkdir(statePath, 0o755); err != nil {
				t.Fatal(err)
			}
		}
		return executil.Result{}, true, nil
	})

	if _, err := f.svc.SetRuntime(context.Background(), SetRuntimeOptions{Name: "demo", Runtime: RuntimePHP, PHPVersion: "8.4"}); err == nil || !strings.Contains(err.Error(), "saving project state") {
		t.Fatalf("err = %v", err)
	}
	if f.vhost() != beforeVhost {
		t.Fatalf("vhost bytes not restored:\n%s", f.vhost())
	}
	if fileExists(buildPHPPoolPaths("demo", "8.4").ConfigPath) {
		t.Fatalf("new pool should be removed")
	}
}

func TestSetRuntimeKeepsDisabledVhostDisabled(t *testing.T) {
	f := newRuntimeFixture(t)
	f.addProject(&State{Runtime: RuntimeStatic, VhostDisabled: true})
	if err := f.svc.disableVhostFile(f.vhostPath); err != nil {
		t.Fatal(err)
	}

	if _, err := f.svc.SetRuntime(context.Background(), SetRuntimeOptions{Name: "demo", Runtime: RuntimeNode, ProxyPort: 4000}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(f.vhost(), "proxy_pass http://127.0.0.1:4000;") {
		t.Fatalf("vhost not rewritten:\n%s", f.vhost())
	}
	if _, err := os.Lstat(filepath.Join(f.svc.nginxEnabled, "abstrax-demo")); !os.IsNotExist(err) {
		t.Fatalf("disabled vhost must stay disabled, err=%v", err)
	}
	if indexOf(f.events, "nginx-reload") >= 0 {
		t.Fatalf("nginx should not be reloaded for a disabled vhost: %v", f.events)
	}
}

func TestSetRuntimeNoVhostOnlyUpdatesState(t *testing.T) {
	f := newRuntimeFixture(t)
	state := f.addProject(&State{Runtime: RuntimeStatic})
	state.VhostPath = ""
	if err := f.svc.saveState(state); err != nil {
		t.Fatal(err)
	}
	before := f.vhost()

	if _, err := f.svc.SetRuntime(context.Background(), SetRuntimeOptions{Name: "demo", Runtime: RuntimeRuby}); err != nil {
		t.Fatal(err)
	}
	if f.vhost() != before {
		t.Fatalf("vhost should not be touched")
	}
	if st := f.stored("demo"); st.Runtime != RuntimeRuby {
		t.Fatalf("runtime = %q", st.Runtime)
	}
}

func TestSetRuntimeDryRunChangesNothing(t *testing.T) {
	f := newRuntimeFixture(t)
	old := f.addProject(&State{
		Runtime:       RuntimePHP,
		PHPVersion:    "8.4",
		OwnershipMode: OwnershipIsolated,
	})
	beforeVhost := f.vhost()
	beforeState := mustRead(t, f.svc.statePath("demo"))

	got, err := f.svc.SetRuntime(context.Background(), SetRuntimeOptions{Name: "demo", Runtime: RuntimeNode, DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if got.Runtime != RuntimeNode || got.PHPPoolName != "" {
		t.Fatalf("preview state wrong: %#v", got)
	}
	if f.vhost() != beforeVhost || mustRead(t, f.svc.statePath("demo")) != beforeState || !fileExists(old.PHPPoolPath()) {
		t.Fatalf("dry-run modified files")
	}
	if len(f.events) != 0 {
		t.Fatalf("dry-run ran commands: %v", f.events)
	}
}

func TestSetRuntimeNoChangeIsNoop(t *testing.T) {
	f := newRuntimeFixture(t)
	f.addProject(&State{Runtime: RuntimeNode, NodeVersion: "22", ProxyPort: 4000})
	beforeState := mustRead(t, f.svc.statePath("demo"))

	got, err := f.svc.SetRuntime(context.Background(), SetRuntimeOptions{Name: "demo", Runtime: RuntimeNode})
	if err != nil {
		t.Fatal(err)
	}
	if got.Runtime != RuntimeNode || len(f.events) != 0 || mustRead(t, f.svc.statePath("demo")) != beforeState {
		t.Fatalf("expected no-op, events=%v", f.events)
	}
}

func TestSetRuntimeValidation(t *testing.T) {
	f := newRuntimeFixture(t)
	f.addProject(&State{Runtime: RuntimeStatic})
	beforeState := mustRead(t, f.svc.statePath("demo"))

	tests := []struct {
		name string
		opts SetRuntimeOptions
		want string
	}{
		{"missing project", SetRuntimeOptions{Name: "nope", Runtime: RuntimeNode}, "not found"},
		{"unknown runtime", SetRuntimeOptions{Name: "demo", Runtime: "go"}, "unsupported runtime"},
		{"empty runtime", SetRuntimeOptions{Name: "demo"}, "unsupported runtime"},
		{"node version on php", SetRuntimeOptions{Name: "demo", Runtime: RuntimePHP, NodeVersion: "22"}, "--node-version"},
		{"php version on node", SetRuntimeOptions{Name: "demo", Runtime: RuntimeNode, PHPVersion: "8.4"}, "--php-version"},
		{"ruby version on static", SetRuntimeOptions{Name: "demo", Runtime: RuntimeStatic, RubyVersion: "3.4"}, "--ruby-version"},
		{"proxy port on php", SetRuntimeOptions{Name: "demo", Runtime: RuntimePHP, ProxyPort: 3000}, "--proxy-port"},
		{"public dir on node", SetRuntimeOptions{Name: "demo", Runtime: RuntimeNode, PublicDir: "public"}, "--public-dir"},
		{"bad port", SetRuntimeOptions{Name: "demo", Runtime: RuntimeNode, ProxyPort: 70000}, "out of range"},
		{"public dir escapes", SetRuntimeOptions{Name: "demo", Runtime: RuntimePHP, PublicDir: "../../etc"}, "escapes"},
		{"public dir absolute", SetRuntimeOptions{Name: "demo", Runtime: RuntimePHP, PublicDir: "/etc"}, "relative"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.svc.SetRuntime(context.Background(), tc.opts)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
	if mustRead(t, f.svc.statePath("demo")) != beforeState || len(f.events) != 0 {
		t.Fatalf("validation failures must not change anything")
	}
}

func mustRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(fmt.Sprintf("read %s: %v", path, err))
	}
	return string(data)
}

func TestSetRuntimeKeepsTLS(t *testing.T) {
	f := newRuntimeFixture(t)
	f.svc.letsEncryptDir = filepath.Join(f.root, "letsencrypt")
	f.addProject(&State{
		Runtime:    RuntimeStatic,
		SSLEnabled: true,
		SSL:        &SSLState{CertificateName: "abstrax-demo", RedirectHTTP: true},
	})
	if !strings.Contains(f.vhost(), "listen 443 ssl") {
		t.Fatalf("fixture vhost has no TLS:\n%s", f.vhost())
	}

	if _, err := f.svc.SetRuntime(context.Background(), SetRuntimeOptions{Name: "demo", Runtime: RuntimeNode, ProxyPort: 4000}); err != nil {
		t.Fatal(err)
	}
	got := f.vhost()
	if !strings.Contains(got, "proxy_pass http://127.0.0.1:4000;") {
		t.Fatalf("vhost not rewritten:\n%s", got)
	}
	if !strings.Contains(got, "listen 443 ssl") || !strings.Contains(got, "live/abstrax-demo/fullchain.pem") {
		t.Fatalf("TLS dropped from vhost:\n%s", got)
	}
}
