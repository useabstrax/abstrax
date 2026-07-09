package project_test

import (
	"os"
	"path/filepath"
	"testing"

	"abstrax/internal/platform"
)

// TestRHELNginxSitePaths verifies the RHEL provider resolves conf.d site paths.
func TestRHELNginxSitePaths(t *testing.T) {
	p := platform.RHELDefaults()
	if p.NginxLayout() != platform.NginxConfD {
		t.Fatalf("layout = %q", p.NginxLayout())
	}
	got := p.NginxSiteConfigPath("abstrax-example")
	want := "/etc/nginx/conf.d/abstrax-example.conf"
	if got != want {
		t.Fatalf("NginxSiteConfigPath = %q, want %q", got, want)
	}
}

func TestRHELDisableRenameSemantics(t *testing.T) {
	dir := t.TempDir()
	active := filepath.Join(dir, "abstrax-app.conf")
	disabled := active + ".disabled"
	if err := os.WriteFile(active, []byte("server {}\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(active, disabled); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(active); !os.IsNotExist(err) {
		t.Fatal("active config should be gone after disable rename")
	}
	if _, err := os.Stat(disabled); err != nil {
		t.Fatalf("disabled config missing: %v", err)
	}
	if err := os.Rename(disabled, active); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(active); err != nil {
		t.Fatalf("re-enable failed: %v", err)
	}
}

func TestDebianNginxLayoutPreserved(t *testing.T) {
	p := platform.DebianDefaults()
	if p.NginxLayout() != platform.NginxSitesAvailableEnabled {
		t.Fatalf("layout = %q", p.NginxLayout())
	}
	if p.NginxSitesAvailable() != "/etc/nginx/sites-available" {
		t.Fatalf("available = %q", p.NginxSitesAvailable())
	}
	if p.NginxSitesEnabled() != "/etc/nginx/sites-enabled" {
		t.Fatalf("enabled = %q", p.NginxSitesEnabled())
	}
}

func TestPackageManagerFactoryFamilies(t *testing.T) {
	// Imported indirectly via platform defaults used by services.
	if platform.RHELDefaults().Profile().PackageManager != "dnf" {
		t.Fatal("RHEL defaults should advertise dnf")
	}
	if platform.DebianDefaults().Profile().PackageManager != "apt" {
		t.Fatal("Debian defaults should advertise apt")
	}
	if platform.RHELDefaults().Profile().FirewallStrategy != "firewalld" {
		t.Fatal("RHEL defaults should advertise firewalld")
	}
}
