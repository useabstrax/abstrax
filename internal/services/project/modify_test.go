package project

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	executil "abstrax/internal/exec"
	"abstrax/internal/platform"
)

const staleVhost = "# stale vhost\n"

type modifyFixture struct {
	svc       *Service
	vhostPath string
}

// newModifyFixture returns a service backed by temp state and nginx dirs.
// nginx commands succeed without running. When withVhost is set, the project
// has a vhost file holding staleVhost so a rewrite is visible.
func newModifyFixture(t *testing.T, state State, withVhost bool) *modifyFixture {
	t.Helper()
	root := t.TempDir()
	svc := New(false, false)
	svc.stateDir = filepath.Join(root, "projects")
	svc.nginxLayout = platform.NginxSitesAvailableEnabled
	svc.nginxAvail = filepath.Join(root, "sites-available")
	svc.nginxEnabled = filepath.Join(root, "sites-enabled")
	svc.nginxLogDir = filepath.Join(root, "log")
	svc.runner.SetCommandFunc(func(_ context.Context, name string, _ []string) (executil.Result, bool, error) {
		if name == "nginx" {
			return executil.Result{}, true, nil
		}
		return executil.Result{}, false, nil
	})

	f := &modifyFixture{svc: svc}
	if state.Name == "" {
		state.Name = "example"
	}
	if state.Path == "" {
		state.Path = "/var/www/example"
	}
	if state.WebServer == "" {
		state.WebServer = WebServerNginx
	}
	if withVhost {
		if err := os.MkdirAll(svc.nginxAvail, 0755); err != nil {
			t.Fatal(err)
		}
		f.vhostPath = filepath.Join(svc.nginxAvail, "abstrax-"+state.Name)
		if err := os.WriteFile(f.vhostPath, []byte(staleVhost), 0644); err != nil {
			t.Fatal(err)
		}
		state.VhostPath = f.vhostPath
	}
	state.CreatedAt = time.Now()
	state.UpdatedAt = time.Now()
	if err := svc.saveState(&state); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *modifyFixture) vhost(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(f.vhostPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func (f *modifyFixture) stored(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile(f.svc.statePath("example"))
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestModifyClearDomainsWithoutVhost(t *testing.T) {
	f := newModifyFixture(t, State{
		Domains: []string{"example.com", "www.example.com"},
		Runtime: RuntimeStatic,
	}, false)

	got, err := f.svc.Modify(context.Background(), ModifyOptions{
		Name:    "example",
		Changed: map[string]bool{"domains": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Domains) != 0 {
		t.Fatalf("domains = %v", got.Domains)
	}
	reloaded, err := f.svc.loadState("example")
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Domains) != 0 {
		t.Fatalf("stored domains = %v", reloaded.Domains)
	}
}

func TestModifyDomainsWithBlankEntriesClears(t *testing.T) {
	f := newModifyFixture(t, State{Domains: []string{"example.com"}, Runtime: RuntimeStatic}, false)

	got, err := f.svc.Modify(context.Background(), ModifyOptions{
		Name:    "example",
		Domains: []string{"", " "},
		Changed: map[string]bool{"domains": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Domains) != 0 {
		t.Fatalf("domains = %v", got.Domains)
	}
}

func TestModifyClearDomainsRefusedWithVhost(t *testing.T) {
	f := newModifyFixture(t, State{
		Domains: []string{"example.com"},
		Runtime: RuntimeStatic,
	}, true)

	_, err := f.svc.Modify(context.Background(), ModifyOptions{
		Name:    "example",
		Changed: map[string]bool{"domains": true},
	})
	if err == nil || !strings.Contains(err.Error(), "server_name _") {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(f.stored(t), "example.com") {
		t.Fatalf("domains were cleared:\n%s", f.stored(t))
	}
	if f.vhost(t) != staleVhost {
		t.Fatalf("vhost was rewritten:\n%s", f.vhost(t))
	}
}

func TestModifyRemoveLastDomainRefusedWithVhost(t *testing.T) {
	f := newModifyFixture(t, State{
		Domains: []string{"example.com"},
		Runtime: RuntimeStatic,
	}, true)

	_, err := f.svc.Modify(context.Background(), ModifyOptions{
		Name:         "example",
		RemoveDomain: "example.com",
		Changed:      map[string]bool{"remove-domain": true},
	})
	if err == nil || !strings.Contains(err.Error(), "server_name _") {
		t.Fatalf("err = %v", err)
	}
}

func TestModifyReplaceDomainsWithVhostRewrites(t *testing.T) {
	f := newModifyFixture(t, State{
		Domains: []string{"example.com"},
		Runtime: RuntimeStatic,
	}, true)

	got, err := f.svc.Modify(context.Background(), ModifyOptions{
		Name:    "example",
		Domains: []string{"new.example.com", "www.new.example.com"},
		Changed: map[string]bool{"domains": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(got.Domains, ",") != "new.example.com,www.new.example.com" {
		t.Fatalf("domains = %v", got.Domains)
	}
	if !strings.Contains(f.vhost(t), "server_name new.example.com www.new.example.com;") {
		t.Fatalf("vhost = %s", f.vhost(t))
	}
}

func TestModifyRejectsInvalidDomain(t *testing.T) {
	f := newModifyFixture(t, State{Domains: []string{"example.com"}, Runtime: RuntimeStatic}, false)

	for _, opts := range []ModifyOptions{
		{Domains: []string{"bad domain;"}, Changed: map[string]bool{"domains": true}},
		{AddDomain: "bad domain;", Changed: map[string]bool{"add-domain": true}},
		{AddDomain: "", Changed: map[string]bool{"add-domain": true}},
	} {
		opts.Name = "example"
		if _, err := f.svc.Modify(context.Background(), opts); err == nil {
			t.Fatalf("opts %+v: expected error", opts)
		}
	}
	if !strings.Contains(f.stored(t), "example.com") {
		t.Fatalf("state changed:\n%s", f.stored(t))
	}
}

func TestModifyClearProxyPort(t *testing.T) {
	f := newModifyFixture(t, State{
		Domains:   []string{"example.com"},
		Runtime:   RuntimeNode,
		ProxyPort: 4000,
	}, true)

	got, err := f.svc.Modify(context.Background(), ModifyOptions{
		Name:      "example",
		ProxyPort: 0,
		Changed:   map[string]bool{"proxy-port": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ProxyPort != 0 {
		t.Fatalf("proxy port = %d", got.ProxyPort)
	}
	if strings.Contains(f.stored(t), "proxy_port") {
		t.Fatalf("proxy port stayed in state:\n%s", f.stored(t))
	}
	if strings.Contains(f.vhost(t), ":4000") || f.vhost(t) == staleVhost {
		t.Fatalf("vhost not rewritten:\n%s", f.vhost(t))
	}
}

func TestModifySetAndValidateProxyPort(t *testing.T) {
	f := newModifyFixture(t, State{Domains: []string{"example.com"}, Runtime: RuntimeNode}, true)

	got, err := f.svc.Modify(context.Background(), ModifyOptions{
		Name:      "example",
		ProxyPort: 5000,
		Changed:   map[string]bool{"proxy-port": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.ProxyPort != 5000 || !strings.Contains(f.vhost(t), "proxy_pass http://127.0.0.1:5000;") {
		t.Fatalf("port = %d, vhost:\n%s", got.ProxyPort, f.vhost(t))
	}

	for _, port := range []int{-1, 70000} {
		_, err := f.svc.Modify(context.Background(), ModifyOptions{
			Name:      "example",
			ProxyPort: port,
			Changed:   map[string]bool{"proxy-port": true},
		})
		if err == nil {
			t.Fatalf("port %d: expected error", port)
		}
	}
}

func TestModifyClearPublicDir(t *testing.T) {
	f := newModifyFixture(t, State{
		Domains:   []string{"example.com"},
		Runtime:   RuntimeStatic,
		PublicDir: "public",
	}, true)

	got, err := f.svc.Modify(context.Background(), ModifyOptions{
		Name:      "example",
		PublicDir: "",
		Changed:   map[string]bool{"public-dir": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got.PublicDir != "" {
		t.Fatalf("public dir = %q", got.PublicDir)
	}
	if strings.Contains(f.stored(t), "public_dir") {
		t.Fatalf("public dir stayed in state:\n%s", f.stored(t))
	}
	if !strings.Contains(f.vhost(t), "root /var/www/example;") {
		t.Fatalf("vhost = %s", f.vhost(t))
	}
}

func TestModifyPublicDirRejectsEscape(t *testing.T) {
	f := newModifyFixture(t, State{Runtime: RuntimeStatic, PublicDir: "public"}, false)

	_, err := f.svc.Modify(context.Background(), ModifyOptions{
		Name:      "example",
		PublicDir: "../../etc",
		Changed:   map[string]bool{"public-dir": true},
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestModifyUnchangedFieldsAreLeftAlone(t *testing.T) {
	f := newModifyFixture(t, State{
		Domains:   []string{"example.com"},
		Runtime:   RuntimeNode,
		PublicDir: "public",
		ProxyPort: 4000,
	}, true)

	got, err := f.svc.Modify(context.Background(), ModifyOptions{Name: "example"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Domains) != 1 || got.PublicDir != "public" || got.ProxyPort != 4000 {
		t.Fatalf("state = %+v", got)
	}
	if f.vhost(t) != staleVhost {
		t.Fatalf("vhost rewritten with nothing changed:\n%s", f.vhost(t))
	}
}

func TestModifySkipsVhostRewriteWhenValuesMatch(t *testing.T) {
	f := newModifyFixture(t, State{
		Domains:   []string{"example.com"},
		Runtime:   RuntimeNode,
		ProxyPort: 4000,
	}, true)

	_, err := f.svc.Modify(context.Background(), ModifyOptions{
		Name:      "example",
		Domains:   []string{"example.com"},
		ProxyPort: 4000,
		Changed:   map[string]bool{"domains": true, "proxy-port": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if f.vhost(t) != staleVhost {
		t.Fatalf("vhost rewritten when values matched:\n%s", f.vhost(t))
	}
}

func TestModifyVersionClearSemantics(t *testing.T) {
	svc := New(false, false)
	state := &State{
		Name:        "example",
		Runtime:     RuntimeNode,
		PHPVersion:  "8.3",
		NodeVersion: "20",
		RubyVersion: "3.3",
	}

	err := svc.applyModify(context.Background(), state, ModifyOptions{
		Changed: map[string]bool{"php-version": true, "ruby-version": true, "node-version": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if state.PHPVersion != "" || state.RubyVersion != "" {
		t.Fatalf("inactive runtime versions not cleared: php=%q ruby=%q", state.PHPVersion, state.RubyVersion)
	}
	if state.NodeVersion != DefaultNodeVersion {
		t.Fatalf("active runtime version = %q, want default %q", state.NodeVersion, DefaultNodeVersion)
	}

	err = svc.applyModify(context.Background(), state, ModifyOptions{
		PHPVersion: "8.4",
		Changed:    map[string]bool{"php-version": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if state.PHPVersion != "8.4" {
		t.Fatalf("php version = %q", state.PHPVersion)
	}

	state.NodeVersion = "22"
	if err := svc.applyModify(context.Background(), state, ModifyOptions{}); err != nil {
		t.Fatal(err)
	}
	if state.NodeVersion != "22" || state.PHPVersion != "8.4" {
		t.Fatalf("unchanged versions were touched: %+v", state)
	}
}

func TestModifyTouchesRuntime(t *testing.T) {
	tests := []struct {
		name  string
		state State
		opts  ModifyOptions
		want  bool
	}{
		{"no flags", State{Runtime: RuntimePHP}, ModifyOptions{}, false},
		{"domains only", State{Runtime: RuntimePHP}, ModifyOptions{Changed: map[string]bool{"domains": true}}, false},
		{"php version on php", State{Runtime: RuntimePHP}, ModifyOptions{Changed: map[string]bool{"php-version": true}}, true},
		{"php version on node", State{Runtime: RuntimeNode}, ModifyOptions{Changed: map[string]bool{"php-version": true}}, false},
		{"node version on node", State{Runtime: RuntimeNode}, ModifyOptions{Changed: map[string]bool{"node-version": true}}, true},
		{"ruby version on ruby", State{Runtime: RuntimeRuby}, ModifyOptions{Changed: map[string]bool{"ruby-version": true}}, true},
		{"runtime switch", State{Runtime: RuntimeStatic}, ModifyOptions{Runtime: RuntimePHP}, true},
		{"version on static", State{Runtime: RuntimeStatic}, ModifyOptions{Changed: map[string]bool{"php-version": true}}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := modifyTouchesRuntime(&tt.state, tt.opts); got != tt.want {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestModifyPathRejectsEmptyAndRelative(t *testing.T) {
	f := newModifyFixture(t, State{Runtime: RuntimeStatic}, false)

	for _, path := range []string{"", "  ", "relative/path"} {
		_, err := f.svc.Modify(context.Background(), ModifyOptions{
			Name:    "example",
			Path:    path,
			Changed: map[string]bool{"path": true},
		})
		if err == nil {
			t.Fatalf("path %q: expected error", path)
		}
	}
	if !strings.Contains(f.stored(t), "/var/www/example") {
		t.Fatalf("path changed:\n%s", f.stored(t))
	}
}

func TestModifyPathUpdatesStateOnly(t *testing.T) {
	oldDir := filepath.Join(t.TempDir(), "old")
	newDir := filepath.Join(t.TempDir(), "new")
	if err := os.MkdirAll(oldDir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "index.html"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}
	f := newModifyFixture(t, State{Path: oldDir, Runtime: RuntimeStatic}, false)
	f.svc.SetIdentityResolver(&mockIdentity{})

	got, err := f.svc.Modify(context.Background(), ModifyOptions{
		Name:    "example",
		Path:    newDir,
		Changed: map[string]bool{"path": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(mustEval(filepath.Dir(newDir)), "new"); got.Path != want {
		t.Fatalf("path = %q, want %q", got.Path, want)
	}
	if _, err := os.Stat(newDir); !os.IsNotExist(err) {
		t.Fatalf("new path was created on disk: %v", err)
	}
	if _, err := os.Stat(filepath.Join(oldDir, "index.html")); err != nil {
		t.Fatalf("old files moved: %v", err)
	}
}
