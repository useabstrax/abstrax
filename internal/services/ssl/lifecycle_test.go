package ssl

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	executil "abstrax/internal/exec"
	"abstrax/internal/services/project"
)

var fixedNow = time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)

type harness struct {
	t       *testing.T
	svc     *Service
	proj    *project.Service
	runner  *executil.Runner
	root    string
	le      string
	handler func(name string, args []string) (executil.Result, error)
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	root := t.TempDir()
	h := &harness{t: t, root: root, le: filepath.Join(root, "le")}

	h.proj = project.New(false, false)
	h.proj.OverrideLayout(project.LayoutOverride{
		StateDir:       filepath.Join(root, "state"),
		NginxAvailable: filepath.Join(root, "available"),
		NginxEnabled:   filepath.Join(root, "enabled"),
		NginxLogDir:    filepath.Join(root, "logs"),
		LetsEncryptDir: h.le,
	})

	h.runner = executil.New(false, false)
	h.runner.SetCommandFunc(func(_ context.Context, name string, args []string) (executil.Result, bool, error) {
		if h.handler != nil {
			res, err := h.handler(name, args)
			return res, true, err
		}
		return executil.Result{}, true, nil
	})
	h.proj.SetRunner(h.runner)

	h.svc = &Service{
		runner:         h.runner,
		projectsSvc:    h.proj,
		letsEncryptDir: h.le,
		installed:      func() bool { return true },
		now:            func() time.Time { return fixedNow },
	}
	return h
}

func (h *harness) addProject(name string, domains []string, mutate func(*project.State)) *project.State {
	h.t.Helper()
	vhost := filepath.Join(h.root, "available", "abstrax-"+name)
	st := &project.State{
		Name:      name,
		Path:      "/var/www/" + name,
		Domains:   domains,
		WebServer: project.WebServerNginx,
		Runtime:   project.RuntimeStatic,
		VhostPath: vhost,
		CreatedAt: fixedNow,
		UpdatedAt: fixedNow,
	}
	if mutate != nil {
		mutate(st)
	}
	if err := os.MkdirAll(filepath.Dir(vhost), 0755); err != nil {
		h.t.Fatal(err)
	}
	if err := h.proj.SaveState(st); err != nil {
		h.t.Fatal(err)
	}
	return st
}

func (h *harness) writeCert(certName string, notAfter time.Time, dns ...string) {
	h.t.Helper()
	dir := filepath.Join(h.le, "live", certName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cert.pem"), testCertPEM(h.t, notAfter, dns), 0644); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) commands() []string {
	var out []string
	for _, c := range h.runner.Commands() {
		out = append(out, strings.Join(c, " "))
	}
	return out
}

func (h *harness) indexOf(prefix string) int {
	for i, c := range h.commands() {
		if strings.HasPrefix(c, prefix) {
			return i
		}
	}
	return -1
}

func testCertPEM(t *testing.T, notAfter time.Time, dns []string) []byte {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    notAfter.Add(-90 * 24 * time.Hour),
		NotAfter:     notAfter,
		DNSNames:     dns,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func TestAddIssuesCertonlyWithStableNameAndRendersTLS(t *testing.T) {
	h := newHarness(t)
	h.addProject("example", []string{"example.com", "www.example.com"}, nil)
	expiry := fixedNow.Add(89 * 24 * time.Hour)

	h.handler = func(name string, args []string) (executil.Result, error) {
		if name == "certbot" {
			h.writeCert("abstrax-example", expiry, "example.com", "www.example.com")
		}
		return executil.Result{}, nil
	}

	err := h.svc.Add(context.Background(), AddOptions{
		ProjectName:  "example",
		Email:        "admin@example.com",
		RedirectHTTP: true,
		Staging:      true,
	})
	if err != nil {
		t.Fatal(err)
	}

	certbot := h.commands()[h.indexOf("certbot")]
	for _, want := range []string{"certbot certonly --nginx", "--cert-name abstrax-example", "-d example.com", "-d www.example.com", "--staging"} {
		if !strings.Contains(certbot, want) {
			t.Fatalf("certbot command %q missing %q", certbot, want)
		}
	}
	if strings.Contains(certbot, "--redirect") {
		t.Fatalf("certbot must not rewrite nginx: %s", certbot)
	}
	if h.indexOf("nginx -t") < h.indexOf("certbot") {
		t.Fatalf("nginx -t must run after certbot issued: %v", h.commands())
	}

	st, err := h.proj.Info(context.Background(), "example")
	if err != nil {
		t.Fatal(err)
	}
	if !st.SSLEnabled || st.SSL == nil {
		t.Fatalf("state = %#v", st)
	}
	ssl := st.SSL
	if ssl.CertificateName != "abstrax-example" || !ssl.RedirectHTTP || !ssl.Staging {
		t.Fatalf("ssl = %#v", ssl)
	}
	if !reflectEqualStrings(ssl.Domains, []string{"example.com", "www.example.com"}) {
		t.Fatalf("domains = %v", ssl.Domains)
	}
	if !ssl.ExpiresAt.Equal(expiry) || !ssl.IssuedAt.Equal(fixedNow) {
		t.Fatalf("expires %v issued %v", ssl.ExpiresAt, ssl.IssuedAt)
	}

	conf, err := os.ReadFile(st.VhostPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"listen 443 ssl;",
		"ssl_certificate " + h.le + "/live/abstrax-example/fullchain.pem;",
		"ssl_certificate_key " + h.le + "/live/abstrax-example/privkey.pem;",
		"return 301 https://$host$request_uri;",
	} {
		if !strings.Contains(string(conf), want) {
			t.Fatalf("vhost missing %q:\n%s", want, conf)
		}
	}
}

func TestAddReusesStoredCertificateName(t *testing.T) {
	h := newHarness(t)
	h.addProject("example", []string{"example.com"}, func(s *project.State) {
		s.SSLEnabled = true
		s.SSL = &project.SSLState{CertificateName: "example.com-0001", RedirectHTTP: true}
	})
	if err := h.svc.Add(context.Background(), AddOptions{ProjectName: "example", Email: "a@example.com", RedirectHTTP: true}); err != nil {
		t.Fatal(err)
	}
	if got := h.commands()[h.indexOf("certbot")]; !strings.Contains(got, "--cert-name example.com-0001") {
		t.Fatalf("certbot = %s", got)
	}
}

func TestAddVhostFailureKeepsHTTPSOffAndRecordsError(t *testing.T) {
	h := newHarness(t)
	h.addProject("example", []string{"example.com"}, nil)
	h.handler = func(name string, args []string) (executil.Result, error) {
		if name == "nginx" && len(args) > 0 && args[0] == "-t" {
			return executil.Result{ExitCode: 1, Stderr: "cannot load certificate"}, errors.New("exit 1")
		}
		return executil.Result{}, nil
	}
	err := h.svc.Add(context.Background(), AddOptions{ProjectName: "example", Email: "a@example.com", RedirectHTTP: true})
	if err == nil || !strings.Contains(err.Error(), "issued but enabling HTTPS") {
		t.Fatalf("err = %v", err)
	}
	st, _ := h.proj.Info(context.Background(), "example")
	if st.SSLEnabled {
		t.Fatal("ssl_enabled must stay false when nginx rejected the config")
	}
	if st.SSL == nil || !strings.Contains(st.SSL.LastError, "nginx config validation failed") {
		t.Fatalf("ssl = %#v", st.SSL)
	}
}

func TestAddRequiresNginxVhostAndEmail(t *testing.T) {
	h := newHarness(t)
	h.addProject("novhost", []string{"example.com"}, func(s *project.State) { s.VhostPath = "" })
	if err := h.svc.Add(context.Background(), AddOptions{ProjectName: "novhost", Email: "a@example.com"}); err == nil {
		t.Fatal("expected vhost error")
	}
	h.addProject("example", []string{"example.com"}, nil)
	if err := h.svc.Add(context.Background(), AddOptions{ProjectName: "example"}); err == nil {
		t.Fatal("expected email error")
	}
	if h.indexOf("certbot") != -1 {
		t.Fatalf("certbot must not run: %v", h.commands())
	}
}

func TestRemoveUsesStoredCertNameAndOrdersSteps(t *testing.T) {
	h := newHarness(t)
	st := h.addProject("example", []string{"example.com"}, func(s *project.State) {
		s.SSLEnabled = true
		s.SSL = &project.SSLState{CertificateName: "abstrax-example", RedirectHTTP: true}
	})
	h.writeCert("abstrax-example", fixedNow.Add(60*24*time.Hour), "example.com")
	// Start from a TLS vhost so we can see it rewritten.
	if err := h.proj.RenderVhost(context.Background(), st); err != nil {
		t.Fatal(err)
	}
	if conf, _ := os.ReadFile(st.VhostPath); !strings.Contains(string(conf), "443 ssl") {
		t.Fatalf("setup vhost has no TLS:\n%s", conf)
	}
	h.runner = executil.New(false, false) // fresh command log
	h.runner.SetCommandFunc(func(_ context.Context, name string, args []string) (executil.Result, bool, error) {
		return executil.Result{}, true, nil
	})
	h.svc.runner = h.runner
	h.proj.SetRunner(h.runner)

	if err := h.svc.Remove(context.Background(), "example"); err != nil {
		t.Fatal(err)
	}

	cmds := h.commands()
	del := h.indexOf("certbot delete --cert-name abstrax-example --non-interactive")
	test := h.indexOf("nginx -t")
	if del == -1 || test == -1 || test > del {
		t.Fatalf("want nginx -t before certbot delete, got %v", cmds)
	}

	conf, _ := os.ReadFile(st.VhostPath)
	if strings.Contains(string(conf), "ssl") || strings.Contains(string(conf), "443") {
		t.Fatalf("vhost still has TLS:\n%s", conf)
	}
	after, _ := h.proj.Info(context.Background(), "example")
	if after.SSLEnabled || after.SSL != nil {
		t.Fatalf("state after remove = %#v", after)
	}
}

func TestRemoveLegacyFallsBackToVhostCertThenDomain(t *testing.T) {
	t.Run("discovered from vhost", func(t *testing.T) {
		h := newHarness(t)
		st := h.addProject("example", []string{"example.com"}, func(s *project.State) { s.SSLEnabled = true })
		conf := "server {\n    ssl_certificate /etc/letsencrypt/live/example.com-0001/fullchain.pem;\n}\n"
		if err := os.WriteFile(st.VhostPath, []byte(conf), 0644); err != nil {
			t.Fatal(err)
		}
		h.writeCert("example.com-0001", fixedNow.Add(time.Hour*24*60))
		if err := h.svc.Remove(context.Background(), "example"); err != nil {
			t.Fatal(err)
		}
		if h.indexOf("certbot delete --cert-name example.com-0001") == -1 {
			t.Fatalf("commands = %v", h.commands())
		}
	})
	t.Run("first domain", func(t *testing.T) {
		h := newHarness(t)
		h.addProject("example", []string{"example.com"}, func(s *project.State) { s.SSLEnabled = true })
		h.writeCert("example.com", fixedNow.Add(time.Hour*24*60))
		if err := h.svc.Remove(context.Background(), "example"); err != nil {
			t.Fatal(err)
		}
		if h.indexOf("certbot delete --cert-name example.com ") == -1 {
			t.Fatalf("commands = %v", h.commands())
		}
	})
}

func TestRemoveStopsBeforeDeleteWhenNginxRejectsPlainVhost(t *testing.T) {
	h := newHarness(t)
	h.addProject("example", []string{"example.com"}, func(s *project.State) {
		s.SSLEnabled = true
		s.SSL = &project.SSLState{CertificateName: "abstrax-example"}
	})
	h.writeCert("abstrax-example", fixedNow.Add(time.Hour*24*60))
	h.handler = func(name string, args []string) (executil.Result, error) {
		if name == "nginx" && len(args) > 0 && args[0] == "-t" {
			return executil.Result{ExitCode: 1, Stderr: "broken"}, errors.New("exit 1")
		}
		return executil.Result{}, nil
	}
	if err := h.svc.Remove(context.Background(), "example"); err == nil {
		t.Fatal("expected error")
	}
	if h.indexOf("certbot delete") != -1 {
		t.Fatalf("certificate deleted despite nginx failure: %v", h.commands())
	}
	st, _ := h.proj.Info(context.Background(), "example")
	if !st.SSLEnabled || st.SSL == nil {
		t.Fatalf("state must be unchanged: %#v", st)
	}
}

func TestRemoveRestoresHTTPSWhenCertbotDeleteFails(t *testing.T) {
	h := newHarness(t)
	st := h.addProject("example", []string{"example.com"}, func(s *project.State) {
		s.SSLEnabled = true
		s.SSL = &project.SSLState{CertificateName: "abstrax-example", RedirectHTTP: true}
	})
	h.writeCert("abstrax-example", fixedNow.Add(time.Hour*24*60))
	h.handler = func(name string, args []string) (executil.Result, error) {
		if name == "certbot" {
			return executil.Result{Stderr: "boom"}, errors.New("exit 1")
		}
		return executil.Result{}, nil
	}
	err := h.svc.Remove(context.Background(), "example")
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v", err)
	}
	conf, _ := os.ReadFile(st.VhostPath)
	if !strings.Contains(string(conf), "443 ssl") {
		t.Fatalf("HTTPS not restored:\n%s", conf)
	}
	after, _ := h.proj.Info(context.Background(), "example")
	if !after.SSLEnabled {
		t.Fatal("state must keep ssl enabled")
	}
}

func TestRemoveWithoutSSLErrors(t *testing.T) {
	h := newHarness(t)
	h.addProject("plain", []string{"example.com"}, nil)
	if err := h.svc.Remove(context.Background(), "plain"); err == nil {
		t.Fatal("expected error")
	}
}

func TestRenewUsesStoredCertNameAndRefreshesExpiry(t *testing.T) {
	h := newHarness(t)
	h.addProject("example", []string{"example.com"}, func(s *project.State) {
		s.SSLEnabled = true
		s.SSL = &project.SSLState{CertificateName: "abstrax-example", ExpiresAt: fixedNow.Add(24 * time.Hour), LastError: "old"}
	})
	renewed := fixedNow.Add(90 * 24 * time.Hour)
	h.writeCert("abstrax-example", renewed, "example.com")

	if err := h.svc.Renew(context.Background(), RenewOptions{Project: "example"}); err != nil {
		t.Fatal(err)
	}
	if h.indexOf("certbot renew --cert-name abstrax-example") == -1 {
		t.Fatalf("commands = %v", h.commands())
	}
	if h.indexOf("nginx -s reload") == -1 {
		t.Fatalf("nginx not reloaded: %v", h.commands())
	}
	st, _ := h.proj.Info(context.Background(), "example")
	if !st.SSL.ExpiresAt.Equal(renewed) || st.SSL.LastError != "" {
		t.Fatalf("ssl = %#v", st.SSL)
	}
}

func TestRenewDryRunDoesNotTouchState(t *testing.T) {
	h := newHarness(t)
	h.addProject("example", []string{"example.com"}, func(s *project.State) {
		s.SSLEnabled = true
		s.SSL = &project.SSLState{CertificateName: "abstrax-example"}
	})
	if err := h.svc.Renew(context.Background(), RenewOptions{Project: "example", DryRun: true}); err != nil {
		t.Fatal(err)
	}
	if h.indexOf("certbot renew --dry-run --cert-name abstrax-example") == -1 {
		t.Fatalf("commands = %v", h.commands())
	}
	if h.indexOf("nginx") != -1 {
		t.Fatalf("dry run must not reload nginx: %v", h.commands())
	}
}

func TestStatusStructuredFields(t *testing.T) {
	h := newHarness(t)
	h.addProject("fresh", []string{"fresh.test"}, func(s *project.State) {
		s.SSLEnabled = true
		s.SSL = &project.SSLState{CertificateName: "abstrax-fresh", Domains: []string{"fresh.test"}, RedirectHTTP: true}
	})
	h.addProject("soon", []string{"soon.test"}, func(s *project.State) {
		s.SSLEnabled = true
		s.SSL = &project.SSLState{CertificateName: "abstrax-soon", Staging: true}
	})
	h.addProject("old", []string{"old.test"}, func(s *project.State) {
		s.SSLEnabled = true
		s.SSL = &project.SSLState{CertificateName: "abstrax-old"}
	})
	h.addProject("gone", []string{"gone.test"}, func(s *project.State) {
		s.SSLEnabled = true
		s.SSL = &project.SSLState{CertificateName: "abstrax-gone"}
	})
	h.addProject("legacy", []string{"legacy.test"}, func(s *project.State) { s.SSLEnabled = true })
	h.addProject("plain", []string{"plain.test"}, nil)

	h.writeCert("abstrax-fresh", fixedNow.Add(61*24*time.Hour+time.Hour), "fresh.test", "www.fresh.test")
	h.writeCert("abstrax-soon", fixedNow.Add(10*24*time.Hour+time.Hour), "soon.test")
	h.writeCert("abstrax-old", fixedNow.Add(-48*time.Hour), "old.test")
	h.writeCert("legacy.test", fixedNow.Add(40*24*time.Hour), "legacy.test")

	got, err := h.svc.Status(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]CertStatus{}
	for _, cs := range got {
		by[cs.ProjectName] = cs
	}
	if _, ok := by["plain"]; ok {
		t.Fatal("project without SSL must not be listed")
	}

	check := func(project, cert, status string) CertStatus {
		t.Helper()
		cs, ok := by[project]
		if !ok {
			t.Fatalf("no status for %s: %#v", project, got)
		}
		if cs.ProjectName != project || cs.CertificateName != cert || cs.Status != status {
			t.Fatalf("%s = %#v, want cert %s status %s", project, cs, cert, status)
		}
		return cs
	}

	fresh := check("fresh", "abstrax-fresh", StatusValid)
	if fresh.Expiry != fixedNow.Add(61*24*time.Hour+time.Hour).Format(time.RFC3339) {
		t.Fatalf("expiry = %q", fresh.Expiry)
	}
	if fresh.DaysRemaining == nil || *fresh.DaysRemaining != 61 {
		t.Fatalf("days = %v", fresh.DaysRemaining)
	}
	if !reflectEqualStrings(fresh.Domains, []string{"fresh.test", "www.fresh.test"}) || !fresh.RedirectHTTP {
		t.Fatalf("fresh = %#v", fresh)
	}
	if soon := check("soon", "abstrax-soon", StatusExpiring); !soon.Staging || *soon.DaysRemaining != 10 {
		t.Fatalf("soon = %#v", soon)
	}
	if old := check("old", "abstrax-old", StatusExpired); *old.DaysRemaining >= 0 {
		t.Fatalf("old = %#v", old)
	}
	if gone := check("gone", "abstrax-gone", StatusMissing); gone.Expiry != "" || gone.DaysRemaining != nil {
		t.Fatalf("gone = %#v", gone)
	}
	check("legacy", "legacy.test", StatusValid)
}

func TestStatusSingleProjectAndUnknown(t *testing.T) {
	h := newHarness(t)
	h.addProject("example", []string{"example.com"}, func(s *project.State) {
		s.SSLEnabled = true
		s.SSL = &project.SSLState{CertificateName: "abstrax-example"}
	})
	h.writeCert("abstrax-example", fixedNow.Add(90*24*time.Hour), "example.com")

	got, err := h.svc.Status(context.Background(), "example")
	if err != nil || len(got) != 1 || got[0].ProjectName != "example" {
		t.Fatalf("got %#v err %v", got, err)
	}
	if h.indexOf("certbot") != -1 {
		t.Fatalf("status must not scrape certbot when the cert is readable: %v", h.commands())
	}
	if _, err := h.svc.Status(context.Background(), "nope"); err == nil {
		t.Fatal("expected unknown project error")
	}
}

const certbotOutput = `Saved certificates:
  Certificate Name: abstrax-unreadable
    Serial Number: 123
    Domains: unreadable.test www.unreadable.test
    Expiry Date: 2027-01-01 00:00:00+00:00 (VALID: 59 days)
    Certificate Path: /etc/letsencrypt/live/abstrax-unreadable/fullchain.pem
  Certificate Name: shared.example
    Domains: shared.example
    Expiry Date: 2026-11-10 00:00:00+00:00 (VALID: 6 days)
`

func TestStatusFallsBackToCertbotOutput(t *testing.T) {
	h := newHarness(t)
	h.addProject("unreadable", []string{"unreadable.test"}, func(s *project.State) {
		s.SSLEnabled = true
		s.SSL = &project.SSLState{CertificateName: "abstrax-unreadable"}
	})
	h.handler = func(name string, args []string) (executil.Result, error) {
		if name == "certbot" {
			return executil.Result{Stdout: certbotOutput}, nil
		}
		return executil.Result{}, nil
	}

	got, err := h.svc.Status(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %#v", got)
	}
	owned, other := got[0], got[1]
	if owned.ProjectName != "unreadable" || owned.CertificateName != "abstrax-unreadable" ||
		owned.Status != StatusValid || owned.Expiry != "2027-01-01T00:00:00Z" {
		t.Fatalf("owned = %#v", owned)
	}
	if other.ProjectName != "" || other.CertificateName != "shared.example" || other.Status != StatusExpiring {
		t.Fatalf("other = %#v", other)
	}
}

func TestClassifyBoundaries(t *testing.T) {
	now := fixedNow
	cases := []struct {
		after time.Duration
		want  string
	}{
		{-time.Second, StatusExpired},
		{0, StatusExpired},
		{30*24*time.Hour - time.Second, StatusExpiring},
		{30 * 24 * time.Hour, StatusValid},
	}
	for _, tc := range cases {
		if got, _ := classify(now, now.Add(tc.after)); got != tc.want {
			t.Fatalf("after %v: %s, want %s", tc.after, got, tc.want)
		}
	}
}

func reflectEqualStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
