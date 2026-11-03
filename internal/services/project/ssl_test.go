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
)

func TestBuildNginxConfigWithSSLRedirect(t *testing.T) {
	conf, err := buildNginxConfig(vhostConfig{
		Name:    "example",
		Path:    "/var/www/example",
		Domains: []string{"example.com", "www.example.com"},
		SSL:     &SSLState{CertificateName: "abstrax-example", RedirectHTTP: true},
		LogDir:  "/var/log/nginx",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"listen 80;",
		"return 301 https://$host$request_uri;",
		"listen 443 ssl;",
		"ssl_certificate /etc/letsencrypt/live/abstrax-example/fullchain.pem;",
		"ssl_certificate_key /etc/letsencrypt/live/abstrax-example/privkey.pem;",
		"ssl_protocols TLSv1.2 TLSv1.3;",
	} {
		if !strings.Contains(conf, want) {
			t.Fatalf("config missing %q:\n%s", want, conf)
		}
	}
	if strings.Count(conf, "server_name example.com www.example.com;") != 2 {
		t.Fatalf("expected both servers to carry server_name:\n%s", conf)
	}
	// Only the HTTPS server serves the app when HTTP redirects.
	if strings.Count(conf, "root /var/www/example;") != 1 {
		t.Fatalf("redirect server must not serve the site:\n%s", conf)
	}
}

func TestBuildNginxConfigWithSSLNoRedirectServesBoth(t *testing.T) {
	conf, err := buildNginxConfig(vhostConfig{
		Name:           "example",
		Path:           "/var/www/example",
		Domains:        []string{"example.com"},
		SharedLogs:     true,
		SSL:            &SSLState{CertificateName: "abstrax-example"},
		LetsEncryptDir: "/le",
		SSLOptionsPath: "/le/options-ssl-nginx.conf",
		SSLDHParamPath: "/le/ssl-dhparams.pem",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(conf, "return 301") {
		t.Fatalf("unexpected redirect:\n%s", conf)
	}
	if strings.Count(conf, "root /var/www/example;") != 2 {
		t.Fatalf("both servers should serve the site:\n%s", conf)
	}
	for _, want := range []string{
		"ssl_certificate /le/live/abstrax-example/fullchain.pem;",
		"include /le/options-ssl-nginx.conf;",
		"ssl_dhparam /le/ssl-dhparams.pem;",
	} {
		if !strings.Contains(conf, want) {
			t.Fatalf("config missing %q:\n%s", want, conf)
		}
	}
	if strings.Contains(conf, "ssl_protocols") {
		t.Fatalf("inline protocols should be replaced by options include:\n%s", conf)
	}
}

func TestBuildNginxConfigWithoutSSLHasNoTLS(t *testing.T) {
	conf, err := buildNginxConfig(vhostConfig{Name: "example", Path: "/var/www/example", SharedLogs: true})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(conf, "443") || strings.Contains(conf, "ssl_") {
		t.Fatalf("plain config rendered TLS:\n%s", conf)
	}
}

func TestBuildNginxConfigRejectsUnsafeCertName(t *testing.T) {
	for _, name := range []string{"", "../etc", "a/b", "a b", "a;b", "a..b"} {
		_, err := buildNginxConfig(vhostConfig{
			Name: "example", Path: "/x", SharedLogs: true,
			SSL: &SSLState{CertificateName: name},
		})
		if err == nil {
			t.Fatalf("cert name %q: expected error", name)
		}
	}
}

func TestEffectiveSSLLegacyFallback(t *testing.T) {
	st := &State{Name: "example", Domains: []string{"example.com"}, SSLEnabled: true}
	ssl := st.EffectiveSSL()
	if ssl == nil || ssl.CertificateName != "example.com" || !ssl.RedirectHTTP {
		t.Fatalf("EffectiveSSL = %#v", ssl)
	}
	if (&State{Domains: []string{"example.com"}}).EffectiveSSL() != nil {
		t.Fatal("SSL disabled must not render TLS")
	}
	stored := &State{SSLEnabled: true, SSL: &SSLState{CertificateName: "abstrax-example"}}
	if got := stored.EffectiveSSL(); got.CertificateName != "abstrax-example" || got.RedirectHTTP {
		t.Fatalf("EffectiveSSL = %#v", got)
	}
}

func TestSSLStateJSONOmitsZeroTimes(t *testing.T) {
	data, err := json.Marshal(SSLState{CertificateName: "abstrax-x"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "expires_at") || strings.Contains(string(data), "issued_at") {
		t.Fatalf("json = %s", data)
	}
	if !strings.Contains(string(data), `"redirect_http":false`) {
		t.Fatalf("json = %s", data)
	}

	exp := time.Date(2027, 1, 2, 3, 4, 5, 0, time.UTC)
	data, _ = json.Marshal(SSLState{CertificateName: "abstrax-x", ExpiresAt: exp})
	var back SSLState
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if !back.ExpiresAt.Equal(exp) {
		t.Fatalf("round trip = %v", back.ExpiresAt)
	}
}

func TestDiscoverSSLFromVhost(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vhost")
	conf := `server {
    listen 443 ssl;
    ssl_certificate /etc/letsencrypt/live/example.com-0001/fullchain.pem; # managed by Certbot
    ssl_certificate_key /etc/letsencrypt/live/example.com-0001/privkey.pem;
}
server {
    if ($host = example.com) {
        return 301 https://$host$request_uri;
    }
    listen 80;
}
`
	if err := os.WriteFile(path, []byte(conf), 0644); err != nil {
		t.Fatal(err)
	}
	name, redirect, found := DiscoverSSLFromVhost(path)
	if !found || name != "example.com-0001" || !redirect {
		t.Fatalf("got %q %v %v", name, redirect, found)
	}
	if _, _, found := DiscoverSSLFromVhost(filepath.Join(t.TempDir(), "missing")); found {
		t.Fatal("missing file must not be found")
	}
}

func TestAdoptSSLPrefersVhostThenDomain(t *testing.T) {
	svc := &Service{}
	dir := t.TempDir()
	vhost := filepath.Join(dir, "vhost")
	_ = os.WriteFile(vhost, []byte("ssl_certificate /etc/letsencrypt/live/custom/fullchain.pem;\n"), 0644)

	st := &State{Name: "example", Domains: []string{"example.com"}, SSLEnabled: true, VhostPath: vhost}
	if !svc.AdoptSSL(st) || st.SSL == nil || st.SSL.CertificateName != "custom" {
		t.Fatalf("adopt from vhost = %#v", st.SSL)
	}
	if svc.AdoptSSL(st) {
		t.Fatal("second adopt must be a no-op")
	}

	st = &State{Name: "example", Domains: []string{"example.com"}, SSLEnabled: true, VhostPath: filepath.Join(dir, "none")}
	if !svc.AdoptSSL(st) || st.SSL.CertificateName != "example.com" || !st.SSL.RedirectHTTP {
		t.Fatalf("adopt fallback = %#v", st.SSL)
	}

	if svc.AdoptSSL(&State{Domains: []string{"example.com"}}) {
		t.Fatal("SSL disabled must not adopt")
	}
}

// stubNginx answers nginx and records every command.
func stubNginx(r *executil.Runner) {
	r.SetCommandFunc(func(_ context.Context, name string, args []string) (executil.Result, bool, error) {
		return executil.Result{Command: name + " " + strings.Join(args, " ")}, true, nil
	})
}

func newTLSTestService(t *testing.T) (*Service, string) {
	t.Helper()
	root := t.TempDir()
	svc := New(false, false)
	svc.OverrideLayout(LayoutOverride{
		StateDir:       filepath.Join(root, "state"),
		NginxAvailable: filepath.Join(root, "available"),
		NginxEnabled:   filepath.Join(root, "enabled"),
		NginxLogDir:    filepath.Join(root, "logs"),
		LetsEncryptDir: filepath.Join(root, "le"),
	})
	r := executil.New(false, false)
	stubNginx(r)
	svc.SetRunner(r)
	return svc, root
}

func TestModifyKeepsHTTPSForStoredAndLegacyState(t *testing.T) {
	cases := []struct {
		name     string
		state    State
		wantCert string
	}{
		{
			name: "stored ssl",
			state: State{SSLEnabled: true, SSL: &SSLState{
				CertificateName: "abstrax-example", RedirectHTTP: true,
			}},
			wantCert: "abstrax-example",
		},
		{
			name:     "legacy ssl_enabled only",
			state:    State{SSLEnabled: true},
			wantCert: "example.com",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := newTLSTestService(t)
			st := tc.state
			st.Name = "example"
			st.Path = "/var/www/example"
			st.Domains = []string{"example.com"}
			st.WebServer = WebServerNginx
			st.Runtime = RuntimeStatic
			st.VhostPath = filepath.Join(svc.nginxAvail, "abstrax-example")
			if err := svc.saveState(&st); err != nil {
				t.Fatal(err)
			}

			got, err := svc.Modify(context.Background(), ModifyOptions{
				Name:      "example",
				AddDomain: "www.example.com",
				Changed:   map[string]bool{"add-domain": true},
			})
			if err != nil {
				t.Fatal(err)
			}
			if got.SSL == nil || got.SSL.CertificateName != tc.wantCert {
				t.Fatalf("SSL after modify = %#v", got.SSL)
			}
			conf, err := os.ReadFile(got.VhostPath)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(conf), "listen 443 ssl;") ||
				!strings.Contains(string(conf), "/live/"+tc.wantCert+"/fullchain.pem") {
				t.Fatalf("modify dropped HTTPS:\n%s", conf)
			}
			if !strings.Contains(string(conf), "server_name example.com www.example.com;") {
				t.Fatalf("modify did not apply the new domain:\n%s", conf)
			}
			// Adopted metadata is persisted.
			stored, err := svc.loadState("example")
			if err != nil || stored.SSL == nil || stored.SSL.CertificateName != tc.wantCert {
				t.Fatalf("stored state = %#v, err %v", stored, err)
			}
		})
	}
}

func TestRenderVhostRestoresPreviousOnNginxFailure(t *testing.T) {
	svc, _ := newTLSTestService(t)
	r := executil.New(false, false)
	r.SetCommandFunc(func(_ context.Context, name string, args []string) (executil.Result, bool, error) {
		if name == "nginx" && len(args) > 0 && args[0] == "-t" {
			return executil.Result{ExitCode: 1, Stderr: "bad config"}, true, os.ErrInvalid
		}
		return executil.Result{}, true, nil
	})
	svc.SetRunner(r)

	vhost := filepath.Join(svc.nginxAvail, "abstrax-example")
	if err := os.MkdirAll(svc.nginxAvail, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(vhost, []byte("previous\n"), 0644); err != nil {
		t.Fatal(err)
	}
	st := &State{
		Name: "example", Path: "/x", Domains: []string{"example.com"},
		WebServer: WebServerNginx, VhostPath: vhost, SSLEnabled: true,
		SSL: &SSLState{CertificateName: "abstrax-example"},
	}
	if err := svc.RenderVhost(context.Background(), st); err == nil {
		t.Fatal("expected nginx validation error")
	}
	got, _ := os.ReadFile(vhost)
	if string(got) != "previous\n" {
		t.Fatalf("vhost = %q, want previous content restored", got)
	}
}

func TestRenderNginxConfigFindsCertbotTLSFiles(t *testing.T) {
	svc, root := newTLSTestService(t)
	le := filepath.Join(root, "le")
	if err := os.MkdirAll(le, 0755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(le, "options-ssl-nginx.conf"), []byte("#"), 0644)
	_ = os.WriteFile(filepath.Join(le, "ssl-dhparams.pem"), []byte("#"), 0644)

	conf, err := svc.renderNginxConfig(vhostConfig{
		Name: "example", Path: "/x", Domains: []string{"example.com"},
		SSL: &SSLState{CertificateName: "abstrax-example", RedirectHTTP: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(conf, "include "+filepath.Join(le, "options-ssl-nginx.conf")+";") ||
		!strings.Contains(conf, "ssl_dhparam "+filepath.Join(le, "ssl-dhparams.pem")+";") {
		t.Fatalf("config = %s", conf)
	}
}
