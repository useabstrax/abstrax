package project

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	executil "abstrax/internal/exec"
	"abstrax/internal/platform"
)

// confDProvider points the conf.d site path at a temporary directory.
type confDProvider struct {
	platform.Provider
	dir string
}

func (p confDProvider) NginxSiteConfigPath(site string) string {
	return filepath.Join(p.dir, site+".conf")
}

type vhostTestEnv struct {
	svc       *Service
	available string
	enabled   string
	confDir   string
	nginxTest *int // exit code returned by nginx -t
}

func newVhostTestService(t *testing.T, layout platform.NginxLayout) *vhostTestEnv {
	t.Helper()
	root := t.TempDir()
	env := &vhostTestEnv{
		available: filepath.Join(root, "sites-available"),
		enabled:   filepath.Join(root, "sites-enabled"),
		confDir:   filepath.Join(root, "conf.d"),
	}
	exit := 0
	env.nginxTest = &exit

	runner := executil.New(false, false)
	runner.SetCommandFunc(func(_ context.Context, name string, args []string) (executil.Result, bool, error) {
		if name != "nginx" {
			return executil.Result{}, false, nil
		}
		if len(args) == 1 && args[0] == "-t" {
			if exit != 0 {
				return executil.Result{ExitCode: exit, Stderr: "bad config"}, true, nil
			}
			return executil.Result{}, true, nil
		}
		return executil.Result{}, true, nil
	})

	env.svc = &Service{
		runner:         runner,
		provider:       confDProvider{Provider: platform.DebianDefaults(), dir: env.confDir},
		stateDir:       filepath.Join(root, "state"),
		nginxLayout:    layout,
		nginxAvail:     env.available,
		nginxEnabled:   env.enabled,
		nginxConfDir:   env.confDir,
		nginxLogDir:    filepath.Join(root, "log"),
		nginxInstalled: func() bool { return true },
	}
	return env
}

func (e *vhostTestEnv) seed(t *testing.T, state *State) {
	t.Helper()
	state.CreatedAt = time.Now()
	state.UpdatedAt = time.Now()
	if err := e.svc.saveState(state); err != nil {
		t.Fatal(err)
	}
}

func (e *vhostTestEnv) reloads() int {
	n := 0
	for _, cmd := range e.svc.runner.Commands() {
		if len(cmd) == 3 && cmd[0] == "nginx" && cmd[1] == "-s" && cmd[2] == "reload" {
			n++
		}
	}
	return n
}

func directoryState(path string) *State {
	return &State{
		Name:      "demo",
		Path:      path,
		Domains:   []string{"demo.test"},
		WebServer: WebServerNone,
		Runtime:   RuntimeStatic,
	}
}

func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func TestVhostConfigCarriesPortAndWebRoot(t *testing.T) {
	state := &State{Name: "demo", Path: "/var/www/demo", Port: 8080, WebRoot: "/var/www/demo/dist"}
	cfg := state.vhostConfig()
	if cfg.Port != 8080 || cfg.WebRoot != "/var/www/demo/dist" {
		t.Fatalf("cfg port=%d web_root=%q", cfg.Port, cfg.WebRoot)
	}
	conf, err := buildNginxConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(conf, "listen 8080;") || !strings.Contains(conf, "root /var/www/demo/dist;") {
		t.Fatalf("config = %s", conf)
	}
}

func TestStatePortAndWebRootRoundTrip(t *testing.T) {
	data, err := json.Marshal(&State{Name: "demo", Port: 8080, WebRoot: "/x", VhostDisabled: true})
	if err != nil {
		t.Fatal(err)
	}
	var got State
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.Port != 8080 || got.WebRoot != "/x" || !got.VhostDisabled {
		t.Fatalf("round trip = %+v", got)
	}
	data, _ = json.Marshal(&State{Name: "demo"})
	for _, key := range []string{"port", "web_root", "vhost_disabled"} {
		if strings.Contains(string(data), `"`+key+`"`) {
			t.Fatalf("%s should be omitted when empty: %s", key, data)
		}
	}
}

func TestAddVhostCreatesVhostForDirectoryProject(t *testing.T) {
	env := newVhostTestService(t, platform.NginxSitesAvailableEnabled)
	projectDir := t.TempDir()
	env.seed(t, directoryState(projectDir))

	webRoot := filepath.Join(projectDir, "dist")
	state, err := env.svc.AddVhost(context.Background(), AddVhostOptions{
		Name:    "demo",
		Domains: []string{"a.test", "b.test"},
		Port:    8081,
		WebRoot: webRoot,
	})
	if err != nil {
		t.Fatal(err)
	}

	wantPath := filepath.Join(env.available, "abstrax-demo")
	if state.VhostPath != wantPath {
		t.Fatalf("VhostPath = %q, want %q", state.VhostPath, wantPath)
	}
	if state.WebServer != WebServerNginx || state.Port != 8081 || state.WebRoot != webRoot {
		t.Fatalf("state = %+v", state)
	}
	if got := strings.Join(state.Domains, ","); got != "a.test,b.test" {
		t.Fatalf("domains = %q", got)
	}
	if !exists(filepath.Join(env.enabled, "abstrax-demo")) {
		t.Fatal("vhost was not enabled")
	}
	conf, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"listen 8081;", "server_name a.test b.test;", "root " + webRoot + ";"} {
		if !strings.Contains(string(conf), want) {
			t.Fatalf("config missing %q:\n%s", want, conf)
		}
	}
	if env.reloads() == 0 {
		t.Fatal("nginx was not reloaded")
	}

	saved, err := env.svc.loadState("demo")
	if err != nil {
		t.Fatal(err)
	}
	if saved.VhostPath != wantPath || saved.WebServer != WebServerNginx || saved.Port != 8081 {
		t.Fatalf("saved state = %+v", saved)
	}
}

func TestAddVhostUsesStoredValues(t *testing.T) {
	env := newVhostTestService(t, platform.NginxSitesAvailableEnabled)
	projectDir := t.TempDir()
	st := directoryState(projectDir)
	st.Port = 9000
	env.seed(t, st)

	state, err := env.svc.AddVhost(context.Background(), AddVhostOptions{Name: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	conf, _ := os.ReadFile(state.VhostPath)
	if !strings.Contains(string(conf), "listen 9000;") || !strings.Contains(string(conf), "server_name demo.test;") {
		t.Fatalf("config = %s", conf)
	}
}

func TestAddVhostRejections(t *testing.T) {
	ctx := context.Background()

	t.Run("missing project", func(t *testing.T) {
		env := newVhostTestService(t, platform.NginxSitesAvailableEnabled)
		if _, err := env.svc.AddVhost(ctx, AddVhostOptions{Name: "nope"}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("already has vhost", func(t *testing.T) {
		env := newVhostTestService(t, platform.NginxSitesAvailableEnabled)
		st := directoryState(t.TempDir())
		st.WebServer = WebServerNginx
		st.VhostPath = filepath.Join(env.available, "abstrax-demo")
		env.seed(t, st)
		_, err := env.svc.AddVhost(ctx, AddVhostOptions{Name: "demo"})
		if err == nil || !strings.Contains(err.Error(), "already has a vhost") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("nginx missing", func(t *testing.T) {
		env := newVhostTestService(t, platform.NginxSitesAvailableEnabled)
		env.svc.nginxInstalled = func() bool { return false }
		env.seed(t, directoryState(t.TempDir()))
		_, err := env.svc.AddVhost(ctx, AddVhostOptions{Name: "demo"})
		if err == nil || !strings.Contains(err.Error(), "nginx is not installed") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("apache", func(t *testing.T) {
		env := newVhostTestService(t, platform.NginxSitesAvailableEnabled)
		st := directoryState(t.TempDir())
		st.WebServer = WebServerApache
		env.seed(t, st)
		if _, err := env.svc.AddVhost(ctx, AddVhostOptions{Name: "demo"}); err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("web root outside project", func(t *testing.T) {
		env := newVhostTestService(t, platform.NginxSitesAvailableEnabled)
		env.seed(t, directoryState(t.TempDir()))
		_, err := env.svc.AddVhost(ctx, AddVhostOptions{Name: "demo", WebRoot: "/etc"})
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestAddVhostNginxTestFailureLeavesProjectUnchanged(t *testing.T) {
	env := newVhostTestService(t, platform.NginxSitesAvailableEnabled)
	env.seed(t, directoryState(t.TempDir()))
	*env.nginxTest = 1

	if _, err := env.svc.AddVhost(context.Background(), AddVhostOptions{Name: "demo"}); err == nil {
		t.Fatal("expected error")
	}
	if exists(filepath.Join(env.available, "abstrax-demo")) || exists(filepath.Join(env.enabled, "abstrax-demo")) {
		t.Fatal("vhost files left behind")
	}
	saved, err := env.svc.loadState("demo")
	if err != nil {
		t.Fatal(err)
	}
	if saved.VhostPath != "" || saved.WebServer != WebServerNone {
		t.Fatalf("state changed: %+v", saved)
	}
}

func TestRemoveVhostKeepsProject(t *testing.T) {
	for _, layout := range []platform.NginxLayout{platform.NginxSitesAvailableEnabled, platform.NginxConfD} {
		t.Run(string(layout), func(t *testing.T) {
			env := newVhostTestService(t, layout)
			projectDir := t.TempDir()
			marker := filepath.Join(projectDir, "index.html")
			if err := os.WriteFile(marker, []byte("hi"), 0644); err != nil {
				t.Fatal(err)
			}
			env.seed(t, directoryState(projectDir))
			added, err := env.svc.AddVhost(context.Background(), AddVhostOptions{Name: "demo"})
			if err != nil {
				t.Fatal(err)
			}
			added.SSLEnabled = true
			if err := env.svc.saveState(added); err != nil {
				t.Fatal(err)
			}
			// A stale disabled copy must go too.
			if err := os.WriteFile(disabledVhostPath(added.VhostPath), []byte("old"), 0644); err != nil {
				t.Fatal(err)
			}

			before := env.reloads()
			state, err := env.svc.RemoveVhost(context.Background(), "demo")
			if err != nil {
				t.Fatal(err)
			}
			if state.VhostPath != "" || state.SSLEnabled || state.VhostDisabled {
				t.Fatalf("state = %+v", state)
			}
			for _, path := range []string{
				added.VhostPath,
				disabledVhostPath(added.VhostPath),
				filepath.Join(env.enabled, "abstrax-demo"),
			} {
				if exists(path) {
					t.Fatalf("%s still exists", path)
				}
			}
			if !exists(marker) {
				t.Fatal("project files were removed")
			}
			saved, err := env.svc.loadState("demo")
			if err != nil {
				t.Fatalf("state was deleted: %v", err)
			}
			if saved.VhostPath != "" || saved.SSLEnabled {
				t.Fatalf("saved = %+v", saved)
			}
			if env.reloads() <= before {
				t.Fatal("nginx was not reloaded")
			}
		})
	}
}

func TestRemoveVhostWithoutVhost(t *testing.T) {
	env := newVhostTestService(t, platform.NginxSitesAvailableEnabled)
	env.seed(t, directoryState(t.TempDir()))
	if _, err := env.svc.RemoveVhost(context.Background(), "demo"); err == nil {
		t.Fatal("expected error")
	}
	if _, err := env.svc.RemoveVhost(context.Background(), "nope"); err == nil {
		t.Fatal("expected error for missing project")
	}
}

func TestEnableDisablePersistVhostDisabled(t *testing.T) {
	for _, layout := range []platform.NginxLayout{platform.NginxSitesAvailableEnabled, platform.NginxConfD} {
		t.Run(string(layout), func(t *testing.T) {
			env := newVhostTestService(t, layout)
			env.seed(t, directoryState(t.TempDir()))
			ctx := context.Background()
			added, err := env.svc.AddVhost(ctx, AddVhostOptions{Name: "demo"})
			if err != nil {
				t.Fatal(err)
			}

			if err := env.svc.Disable(ctx, "demo"); err != nil {
				t.Fatal(err)
			}
			saved, _ := env.svc.loadState("demo")
			if !saved.VhostDisabled {
				t.Fatal("Disable did not persist vhost_disabled")
			}
			if err := env.svc.Enable(ctx, "demo"); err != nil {
				t.Fatal(err)
			}
			saved, _ = env.svc.loadState("demo")
			if saved.VhostDisabled {
				t.Fatal("Enable did not clear vhost_disabled")
			}
			if saved.VhostPath != added.VhostPath {
				t.Fatalf("vhost path changed: %q", saved.VhostPath)
			}
		})
	}
}

func TestModifyKeepsDisabledVhostDisabled(t *testing.T) {
	cases := []struct {
		layout platform.NginxLayout
		active func(env *vhostTestEnv, vhostPath string) bool
	}{
		{
			platform.NginxSitesAvailableEnabled,
			func(env *vhostTestEnv, vhostPath string) bool {
				return exists(filepath.Join(env.enabled, filepath.Base(vhostPath)))
			},
		},
		{
			platform.NginxConfD,
			func(_ *vhostTestEnv, vhostPath string) bool { return exists(vhostPath) },
		},
	}
	for _, tc := range cases {
		t.Run(string(tc.layout), func(t *testing.T) {
			env := newVhostTestService(t, tc.layout)
			env.seed(t, directoryState(t.TempDir()))
			ctx := context.Background()
			added, err := env.svc.AddVhost(ctx, AddVhostOptions{Name: "demo"})
			if err != nil {
				t.Fatal(err)
			}
			if err := env.svc.Disable(ctx, "demo"); err != nil {
				t.Fatal(err)
			}

			before := env.reloads()
			state, err := env.svc.Modify(ctx, ModifyOptions{Name: "demo", AddDomain: "extra.test", Changed: map[string]bool{"add-domain": true}})
			if err != nil {
				t.Fatal(err)
			}
			if !state.VhostDisabled {
				t.Fatal("Modify cleared vhost_disabled")
			}
			if tc.active(env, added.VhostPath) {
				t.Fatal("Modify re-enabled a disabled vhost")
			}
			if env.reloads() != before {
				t.Fatal("Modify reloaded nginx for a disabled vhost")
			}

			// The rewrite is kept and applies when the vhost is enabled again.
			if err := env.svc.Enable(ctx, "demo"); err != nil {
				t.Fatal(err)
			}
			conf, err := os.ReadFile(added.VhostPath)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(conf), "extra.test") {
				t.Fatalf("enabled config lost the modify:\n%s", conf)
			}
		})
	}
}

func TestModifyEnabledVhostStaysEnabled(t *testing.T) {
	env := newVhostTestService(t, platform.NginxSitesAvailableEnabled)
	env.seed(t, directoryState(t.TempDir()))
	ctx := context.Background()
	if _, err := env.svc.AddVhost(ctx, AddVhostOptions{Name: "demo"}); err != nil {
		t.Fatal(err)
	}
	if _, err := env.svc.Modify(ctx, ModifyOptions{Name: "demo", AddDomain: "extra.test", Changed: map[string]bool{"add-domain": true}}); err != nil {
		t.Fatal(err)
	}
	if !exists(filepath.Join(env.enabled, "abstrax-demo")) {
		t.Fatal("vhost no longer enabled")
	}
}

func TestModifyNginxTestFailureRestoresVhost(t *testing.T) {
	env := newVhostTestService(t, platform.NginxSitesAvailableEnabled)
	env.seed(t, directoryState(t.TempDir()))
	ctx := context.Background()
	added, err := env.svc.AddVhost(ctx, AddVhostOptions{Name: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(added.VhostPath)

	*env.nginxTest = 1
	if _, err := env.svc.Modify(ctx, ModifyOptions{Name: "demo", AddDomain: "extra.test", Changed: map[string]bool{"add-domain": true}}); err == nil {
		t.Fatal("expected error")
	}
	after, err := os.ReadFile(added.VhostPath)
	if err != nil {
		t.Fatalf("vhost file lost: %v", err)
	}
	if string(before) != string(after) {
		t.Fatalf("vhost file was not restored:\n%s", after)
	}
	if !exists(filepath.Join(env.enabled, "abstrax-demo")) {
		t.Fatal("vhost link was lost")
	}
}

func TestModifyConfDDoesNotResurrectStaleDisabledCopy(t *testing.T) {
	env := newVhostTestService(t, platform.NginxConfD)
	env.seed(t, directoryState(t.TempDir()))
	ctx := context.Background()
	added, err := env.svc.AddVhost(ctx, AddVhostOptions{Name: "demo"})
	if err != nil {
		t.Fatal(err)
	}
	if err := env.svc.Disable(ctx, "demo"); err != nil {
		t.Fatal(err)
	}
	if _, err := env.svc.Modify(ctx, ModifyOptions{Name: "demo", AddDomain: "extra.test", Changed: map[string]bool{"add-domain": true}}); err != nil {
		t.Fatal(err)
	}
	if exists(added.VhostPath) {
		t.Fatal("active conf.d file exists for a disabled vhost")
	}
	if !exists(disabledVhostPath(added.VhostPath)) {
		t.Fatal("disabled copy missing")
	}
}

func TestAddPersistsPortAndWebRoot(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	setupFilesystem(t, root)

	projectPath := filepath.Join(root, "var", "www", "demo")
	webRoot := filepath.Join(projectPath, "dist")

	svc := New(false, false)
	svc.stateDir = filepath.Join(root, "state")
	svc.SetIdentityResolver(&mockIdentity{homes: testHomes(root)})

	state, err := svc.Add(context.Background(), AddOptions{
		Name:      "demo",
		Path:      projectPath,
		WebServer: WebServerNginx,
		NoVhost:   true,
		Port:      8080,
		WebRoot:   webRoot,
		Runtime:   RuntimeStatic,
		User:      SharedWebUser,
		Group:     SharedWebGroup,
		Yes:       true,
	})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if state.Port != 8080 || state.WebRoot != webRoot {
		t.Fatalf("state port=%d web_root=%q", state.Port, state.WebRoot)
	}
	saved, err := svc.loadState("demo")
	if err != nil {
		t.Fatal(err)
	}
	if saved.Port != 8080 || saved.WebRoot != webRoot {
		t.Fatalf("saved port=%d web_root=%q", saved.Port, saved.WebRoot)
	}
}
