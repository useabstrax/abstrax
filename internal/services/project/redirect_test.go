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

type redirectEnv struct {
	svc       *Service
	vhostPath string
	nginxFail bool
	commands  [][]string
}

func newRedirectEnv(t *testing.T, withVhost bool) *redirectEnv {
	t.Helper()
	root := t.TempDir()
	env := &redirectEnv{}
	runner := executil.New(false, false)
	runner.SetCommandFunc(func(_ context.Context, name string, args []string) (executil.Result, bool, error) {
		if name != "nginx" {
			return executil.Result{}, false, nil
		}
		env.commands = append(env.commands, append([]string{name}, args...))
		if len(args) > 0 && args[0] == "-t" && env.nginxFail {
			return executil.Result{ExitCode: 1, Stderr: "nginx: bad config"}, true, nil
		}
		return executil.Result{}, true, nil
	})
	env.svc = &Service{
		runner:       runner,
		stateDir:     filepath.Join(root, "state"),
		nginxLayout:  platform.NginxSitesAvailableEnabled,
		nginxAvail:   filepath.Join(root, "available"),
		nginxEnabled: filepath.Join(root, "enabled"),
		nginxLogDir:  filepath.Join(root, "log"),
	}
	state := &State{
		Name:      "example",
		Path:      "/var/www/example",
		Domains:   []string{"example.com"},
		WebServer: WebServerNginx,
		Runtime:   RuntimeStatic,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if withVhost {
		env.vhostPath = filepath.Join(env.svc.nginxAvail, "abstrax-example")
		state.VhostPath = env.vhostPath
		if _, err := env.svc.createNginxVhost(context.Background(), state.vhostConfig()); err != nil {
			t.Fatal(err)
		}
		env.commands = nil
	}
	if err := env.svc.saveState(state); err != nil {
		t.Fatal(err)
	}
	return env
}

func (e *redirectEnv) vhost(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(e.vhostPath)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestValidateRedirect(t *testing.T) {
	good := []Redirect{
		{Source: "/old", Destination: "/new", Code: 301},
		{Source: "/a/b-c_d.html", Destination: "https://example.com/x?y=1#z", Code: 302},
		{Source: "/", Destination: "http://example.com", Code: 307},
		{Source: "/p", Destination: "/q?x=1", Code: 308},
	}
	for _, r := range good {
		if err := validateRedirect(r); err != nil {
			t.Errorf("%+v rejected: %v", r, err)
		}
	}

	bad := map[string]Redirect{
		"no leading slash":    {Source: "old", Destination: "/new", Code: 301},
		"empty source":        {Source: "", Destination: "/new", Code: 301},
		"space in source":     {Source: "/a b", Destination: "/new", Code: 301},
		"newline in source":   {Source: "/a\nb", Destination: "/new", Code: 301},
		"semicolon source":    {Source: "/a;b", Destination: "/new", Code: 301},
		"brace source":        {Source: "/a{", Destination: "/new", Code: 301},
		"close brace source":  {Source: "/a}", Destination: "/new", Code: 301},
		"quote source":        {Source: `/a"`, Destination: "/new", Code: 301},
		"dollar source":       {Source: "/a$b", Destination: "/new", Code: 301},
		"query source":        {Source: "/a?b=1", Destination: "/new", Code: 301},
		"fragment source":     {Source: "/a#b", Destination: "/new", Code: 301},
		"non-ascii source":    {Source: "/é", Destination: "/new", Code: 301},
		"empty destination":   {Source: "/a", Destination: "", Code: 301},
		"relative dest":       {Source: "/a", Destination: "new", Code: 301},
		"scheme-relative":     {Source: "/a", Destination: "//evil.com", Code: 301},
		"ftp dest":            {Source: "/a", Destination: "ftp://example.com/", Code: 301},
		"javascript dest":     {Source: "/a", Destination: "javascript:alert(1)", Code: 301},
		"no host":             {Source: "/a", Destination: "https:///path", Code: 301},
		"credentials":         {Source: "/a", Destination: "https://u:p@example.com/", Code: 301},
		"space in dest":       {Source: "/a", Destination: "/b c", Code: 301},
		"semicolon in dest":   {Source: "/a", Destination: "/b;rewrite ^ /x", Code: 301},
		"injection in dest":   {Source: "/a", Destination: "/b;}server{", Code: 301},
		"newline in dest":     {Source: "/a", Destination: "/b\nlocation / {", Code: 301},
		"variable in dest":    {Source: "/a", Destination: "https://example.com/$request_uri", Code: 301},
		"backslash dest":      {Source: "/a", Destination: `/b\c`, Code: 301},
		"code 200":            {Source: "/a", Destination: "/b", Code: 200},
		"code zero":           {Source: "/a", Destination: "/b", Code: 0},
		"code 404":            {Source: "/a", Destination: "/b", Code: 404},
		"too long source":     {Source: "/" + strings.Repeat("a", maxRedirectFieldLen), Destination: "/b", Code: 301},
		"backtick dest":       {Source: "/a", Destination: "/b`", Code: 301},
		"single quote source": {Source: "/a'", Destination: "/b", Code: 301},
	}
	for name, r := range bad {
		if err := validateRedirect(r); err == nil {
			t.Errorf("%s: %+v accepted", name, r)
		}
	}
}

func TestBuildNginxConfigRendersRedirectsBeforeLocations(t *testing.T) {
	for _, rt := range []Runtime{RuntimeStatic, RuntimePHP, RuntimeNode} {
		conf, err := buildNginxConfig(vhostConfig{
			Name:    "example",
			Path:    "/var/www/example",
			Domains: []string{"example.com"},
			Runtime: rt,
			Redirects: []Redirect{
				{ID: "bbbbbbbb", Source: "/b", Destination: "https://example.org/b", Code: 302},
				{ID: "aaaaaaaa", Source: "/a", Destination: "/new-a", Code: 301},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		a := strings.Index(conf, "    location = /a {\n        return 301 /new-a;\n    }\n")
		b := strings.Index(conf, "    location = /b {\n        return 302 https://example.org/b;\n    }\n")
		catchAll := strings.Index(conf, "    location / {\n")
		if a < 0 || b < 0 || catchAll < 0 {
			t.Fatalf("%s: missing blocks:\n%s", rt, conf)
		}
		if !(a < b && b < catchAll) {
			t.Fatalf("%s: wrong order (a=%d b=%d catch-all=%d):\n%s", rt, a, b, catchAll, conf)
		}
		if rt == RuntimePHP && b > strings.Index(conf, "location ~ \\.php$") {
			t.Fatalf("redirects must precede the PHP location:\n%s", conf)
		}
	}
}

func TestBuildNginxConfigWithoutRedirectsUnchanged(t *testing.T) {
	conf, err := buildNginxConfig(vhostConfig{Name: "example", Path: "/var/www/example"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(conf, "return ") {
		t.Fatalf("unexpected return directive:\n%s", conf)
	}
}

func TestBuildNginxConfigRejectsInjectedRedirect(t *testing.T) {
	_, err := buildNginxConfig(vhostConfig{
		Name: "example",
		Path: "/var/www/example",
		Redirects: []Redirect{
			{ID: "x", Source: "/a", Destination: "/b;}\n}\nserver {", Code: 301},
		},
	})
	if err == nil {
		t.Fatal("injected redirect rendered")
	}
}

func TestStateVhostConfigCarriesRedirects(t *testing.T) {
	state := &State{
		Name: "example",
		Path: "/var/www/example",
		Redirects: []Redirect{
			{ID: "aaaaaaaa", Source: "/a", Destination: "/b", Code: 301},
		},
	}
	cfg := state.vhostConfig()
	if len(cfg.Redirects) != 1 {
		t.Fatalf("vhostConfig dropped redirects: %+v", cfg.Redirects)
	}
	conf, err := buildNginxConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(conf, "return 301 /b;") {
		t.Fatalf("modify-style rewrite lost redirect:\n%s", conf)
	}
}

func TestRedirectCRUD(t *testing.T) {
	env := newRedirectEnv(t, true)
	ctx := context.Background()

	r, err := env.svc.AddRedirect(ctx, AddRedirectOptions{
		Project: "example", Source: "/old", Destination: "/new",
	})
	if err != nil {
		t.Fatal(err)
	}
	if r.Code != 301 || r.ID != redirectID("/old") || len(r.ID) != 8 {
		t.Fatalf("unexpected redirect: %+v", r)
	}
	if !strings.Contains(env.vhost(t), "location = /old {\n        return 301 /new;\n    }") {
		t.Fatalf("vhost missing redirect:\n%s", env.vhost(t))
	}
	if len(env.commands) == 0 || env.commands[0][1] != "-t" {
		t.Fatalf("nginx -t not run: %v", env.commands)
	}

	if _, err := env.svc.AddRedirect(ctx, AddRedirectOptions{
		Project: "example", Source: "/old", Destination: "/other",
	}); err == nil {
		t.Fatal("duplicate source accepted")
	}

	second, err := env.svc.AddRedirect(ctx, AddRedirectOptions{
		Project: "example", Source: "/blog", Destination: "https://blog.example.com/", Code: 302,
	})
	if err != nil {
		t.Fatal(err)
	}

	list, err := env.svc.ListRedirects(ctx, "example")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID > list[1].ID {
		t.Fatalf("list not sorted by ID: %+v", list)
	}

	mod, err := env.svc.ModifyRedirect(ctx, ModifyRedirectOptions{
		Project: "example", ID: r.ID, Destination: "/newer", Code: 308,
	})
	if err != nil {
		t.Fatal(err)
	}
	if mod.ID != r.ID || mod.Source != "/old" || mod.Destination != "/newer" || mod.Code != 308 {
		t.Fatalf("unexpected modify result: %+v", mod)
	}
	if !strings.Contains(env.vhost(t), "return 308 /newer;") || strings.Contains(env.vhost(t), "return 301 /new;") {
		t.Fatalf("vhost not updated:\n%s", env.vhost(t))
	}

	if _, err := env.svc.ModifyRedirect(ctx, ModifyRedirectOptions{
		Project: "example", ID: r.ID, Source: "/blog",
	}); err == nil {
		t.Fatal("modify to a duplicate source accepted")
	}
	if _, err := env.svc.ModifyRedirect(ctx, ModifyRedirectOptions{Project: "example", ID: r.ID}); err == nil {
		t.Fatal("empty modify accepted")
	}
	if _, err := env.svc.ModifyRedirect(ctx, ModifyRedirectOptions{
		Project: "example", ID: "nope", Code: 302,
	}); err == nil {
		t.Fatal("unknown id accepted")
	}

	removed, err := env.svc.RemoveRedirect(ctx, "example", second.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if removed.Source != "/blog" {
		t.Fatalf("removed wrong redirect: %+v", removed)
	}
	if strings.Contains(env.vhost(t), "/blog") {
		t.Fatalf("vhost still has removed redirect:\n%s", env.vhost(t))
	}
	if _, err := env.svc.RemoveRedirect(ctx, "example", second.ID, false); err == nil {
		t.Fatal("removing twice succeeded")
	}

	state, err := env.svc.loadState("example")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Redirects) != 1 || state.Redirects[0].ID != r.ID {
		t.Fatalf("unexpected state: %+v", state.Redirects)
	}
}

func TestRedirectsSurviveModify(t *testing.T) {
	env := newRedirectEnv(t, true)
	ctx := context.Background()
	if _, err := env.svc.AddRedirect(ctx, AddRedirectOptions{
		Project: "example", Source: "/old", Destination: "/new",
	}); err != nil {
		t.Fatal(err)
	}

	// Modify reloads state and regenerates the vhost from it.
	state, err := env.svc.loadState("example")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(env.vhostPath, []byte("# stale\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := env.svc.createNginxVhost(ctx, state.vhostConfig()); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(env.vhost(t), "location = /old {\n        return 301 /new;\n    }") {
		t.Fatalf("redirect lost on vhost rewrite:\n%s", env.vhost(t))
	}
}

func TestRedirectVhostFailureDoesNotSaveState(t *testing.T) {
	env := newRedirectEnv(t, true)
	ctx := context.Background()
	before := env.vhost(t)

	env.nginxFail = true
	_, err := env.svc.AddRedirect(ctx, AddRedirectOptions{
		Project: "example", Source: "/old", Destination: "/new",
	})
	if err == nil || !strings.Contains(err.Error(), "nginx config validation failed") {
		t.Fatalf("expected validation failure, got %v", err)
	}
	state, err := env.svc.loadState("example")
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Redirects) != 0 {
		t.Fatalf("bad state saved: %+v", state.Redirects)
	}
	if got := env.vhost(t); got != before {
		t.Fatalf("previous vhost not restored:\n%s", got)
	}
}

func TestRedirectRequiresVhost(t *testing.T) {
	env := newRedirectEnv(t, false)
	ctx := context.Background()
	_, err := env.svc.AddRedirect(ctx, AddRedirectOptions{
		Project: "example", Source: "/old", Destination: "/new",
	})
	if err == nil || !strings.Contains(err.Error(), "no vhost") {
		t.Fatalf("expected no-vhost error, got %v", err)
	}
	if _, err := env.svc.ModifyRedirect(ctx, ModifyRedirectOptions{Project: "example", ID: "x", Code: 302}); err == nil {
		t.Fatal("modify without vhost succeeded")
	}
	if _, err := env.svc.RemoveRedirect(ctx, "example", "x", false); err == nil {
		t.Fatal("remove without vhost succeeded")
	}
}

func TestRedirectInvalidInputDoesNotTouchVhost(t *testing.T) {
	env := newRedirectEnv(t, true)
	before := env.vhost(t)
	_, err := env.svc.AddRedirect(context.Background(), AddRedirectOptions{
		Project: "example", Source: "/a;b", Destination: "/new",
	})
	if err == nil {
		t.Fatal("invalid source accepted")
	}
	if env.vhost(t) != before || len(env.commands) != 0 {
		t.Fatal("invalid redirect changed nginx")
	}
}

func TestRedirectDryRunWritesNothing(t *testing.T) {
	env := newRedirectEnv(t, true)
	before := env.vhost(t)
	if _, err := env.svc.AddRedirect(context.Background(), AddRedirectOptions{
		Project: "example", Source: "/old", Destination: "/new", DryRun: true,
	}); err != nil {
		t.Fatal(err)
	}
	if env.vhost(t) != before {
		t.Fatal("dry run rewrote the vhost")
	}
	state, _ := env.svc.loadState("example")
	if len(state.Redirects) != 0 {
		t.Fatal("dry run saved state")
	}
}

func TestListRedirectsUnknownProject(t *testing.T) {
	env := newRedirectEnv(t, true)
	if _, err := env.svc.ListRedirects(context.Background(), "missing"); err == nil {
		t.Fatal("expected error")
	}
}
